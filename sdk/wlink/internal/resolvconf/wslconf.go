package resolvconf

import "strings"

// wlink owns exactly ONE key in wsl.conf. Everything else in that file is
// someone else's configuration — [boot] systemd, [user] default, [interop]
// settings — and must come back byte-identical. The Set/Remove round trip is
// asserted by test for that reason: it is the property the undo path rests on.
const (
	networkSection = "[network]"
	generateKey    = "generateResolvConf"
	generateLine   = generateKey + " = false"
)

func isSectionHeader(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]")
}

func isNetworkHeader(line string) bool {
	return strings.EqualFold(strings.TrimSpace(line), networkSection)
}

func isGenerateKey(line string) bool {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(strings.ToLower(t), strings.ToLower(generateKey)) {
		return false
	}
	rest := strings.TrimSpace(t[len(generateKey):])
	return strings.HasPrefix(rest, "=")
}

// splitKeepingTrailingNewline splits into lines while remembering whether the
// content ended with a newline, so rejoining does not silently add or drop one.
func splitKeepingTrailingNewline(content string) (lines []string, trailingNewline bool) {
	if content == "" {
		return nil, false
	}
	trailingNewline = strings.HasSuffix(content, "\n")
	body := content
	if trailingNewline {
		body = body[:len(body)-1]
	}
	return strings.Split(body, "\n"), trailingNewline
}

func join(lines []string, trailingNewline bool) string {
	if len(lines) == 0 {
		return ""
	}
	out := strings.Join(lines, "\n")
	if trailingNewline {
		out += "\n"
	}
	return out
}

// SetGenerateResolvConf ensures `generateResolvConf = false` under [network],
// leaving every other line untouched.
//
// Handles all five shapes a real wsl.conf comes in: the key already present
// (with any value), a [network] section without the key, a [network] section
// that is last in the file, no [network] section at all, and an empty file.
// Idempotent, because install.sh re-runs.
func SetGenerateResolvConf(content string) string {
	// The input's trailing newline is deliberately ignored: the output is always
	// newline-terminated. A config file whose last line lacks one is a nuisance
	// for every later editor, and wsl.conf is ours to leave tidy.
	lines, _ := splitKeepingTrailingNewline(content)
	if len(lines) == 0 {
		return networkSection + "\n" + generateLine + "\n"
	}

	out := make([]string, 0, len(lines)+3)
	inNetwork, done, sawNetwork := false, false, false

	for _, line := range lines {
		if isSectionHeader(line) {
			// Leaving [network] without having written the key: write it as the
			// section's last line, before the next header.
			if inNetwork && !done {
				out = append(out, generateLine)
				done = true
			}
			inNetwork = isNetworkHeader(line)
			sawNetwork = sawNetwork || inNetwork
			out = append(out, line)
			continue
		}
		if inNetwork && isGenerateKey(line) {
			if !done {
				out = append(out, generateLine)
				done = true
			}
			continue // drop any duplicate occurrences
		}
		out = append(out, line)
	}

	switch {
	case inNetwork && !done:
		// File ended inside [network]. Insert before any trailing blank lines so
		// the key lands in the section rather than after it.
		i := len(out)
		for i > 0 && strings.TrimSpace(out[i-1]) == "" {
			i--
		}
		out = append(out[:i], append([]string{generateLine}, out[i:]...)...)
	case !sawNetwork:
		if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
			out = append(out, "")
		}
		out = append(out, networkSection, generateLine)
	}
	return join(out, true)
}

// RemoveGenerateResolvConf deletes the key, and drops a [network] section left
// empty by that removal.
//
// This is the repair path when no snapshot exists (EC-16). Leaving an empty
// [network] behind would mean the file never returns to its stock shape, so a
// later diff against a pristine machine would show a difference wlink caused.
func RemoveGenerateResolvConf(content string) string {
	lines, trailing := splitKeepingTrailingNewline(content)
	if len(lines) == 0 {
		return content
	}

	out := make([]string, 0, len(lines))
	inNetwork := false
	networkHeaderAt := -1
	networkHasOtherContent := false

	flushSection := func() {
		// The section is empty if nothing but blanks followed its header.
		if networkHeaderAt >= 0 && !networkHasOtherContent {
			trimmed := out[:networkHeaderAt]
			// Also drop the blank line that separated it from the section above.
			for len(trimmed) > 0 && strings.TrimSpace(trimmed[len(trimmed)-1]) == "" {
				trimmed = trimmed[:len(trimmed)-1]
			}
			out = trimmed
		}
		networkHeaderAt, networkHasOtherContent = -1, false
	}

	for _, line := range lines {
		if isSectionHeader(line) {
			flushSection()
			inNetwork = isNetworkHeader(line)
			out = append(out, line)
			if inNetwork {
				networkHeaderAt = len(out) - 1
			}
			continue
		}
		if inNetwork {
			if isGenerateKey(line) {
				continue
			}
			if strings.TrimSpace(line) != "" {
				networkHasOtherContent = true
			}
		}
		out = append(out, line)
	}
	flushSection()

	// Trailing blank lines left by a removed section are noise.
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return join(out, trailing)
}
