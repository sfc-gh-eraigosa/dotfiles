package sshconf

import (
	"fmt"
	"strings"
)

// blockRange locates the [start,end) line range of alias's Host block.
// end is the line index of the next Host line, or len(lines).
func blockRange(lines []string, alias string) (start, end int, ok bool) {
	start = -1
	for i, l := range lines {
		code, _ := splitComment(strings.TrimSpace(l))
		if !isHostLine(code) {
			continue
		}
		if start >= 0 {
			return start, i, true
		}
		if strings.Fields(code)[1] == alias {
			start = i
		}
	}
	if start >= 0 {
		return start, len(lines), true
	}
	return 0, 0, false
}

// normalize guarantees a non-empty config ends with exactly one newline.
// Every edit path runs through this: an add->purge round-trip that silently
// dropped the trailing newline showed up as a spurious "\ No newline at end
// of file" in every later diff — a collateral edit, which this package must
// never make.
func normalize(cfg string) string {
	trimmed := strings.TrimRight(cfg, "\n")
	if strings.TrimSpace(trimmed) == "" {
		return ""
	}
	return trimmed + "\n"
}

// render emits a marked Host block. Only non-empty fields are written, so a
// caller cannot accidentally blank a directive by omitting it.
func render(h Host, marker string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Host %s  %s\n", h.Alias, marker)
	for _, kv := range []struct{ k, v string }{
		{"HostName", h.HostName}, {"User", h.User},
		{"Port", h.Port}, {"IdentityFile", h.Identity},
	} {
		if kv.v != "" {
			fmt.Fprintf(&b, "    %s %s\n", kv.k, kv.v)
		}
	}
	return b.String()
}

// Add upserts a fleet-marked Host block. Re-adding an identical host is a
// no-op on the file's bytes (idempotent), and no other block is touched.
func Add(cfg string, h Host, marker string) (string, error) {
	if strings.TrimSpace(h.Alias) == "" {
		return "", fmt.Errorf("sshconf: alias is required")
	}
	if _, _, exists := blockRange(strings.Split(cfg, "\n"), h.Alias); exists {
		purged, err := Purge(cfg, h.Alias)
		if err != nil {
			return "", err
		}
		cfg = purged
	}
	body := strings.TrimRight(cfg, "\n")
	if strings.TrimSpace(body) == "" {
		return normalize(render(h, marker)), nil
	}
	return normalize(body + "\n\n" + render(h, marker)), nil
}

// Unmark removes the fleet marker but keeps the Host block. Leaving the fleet
// must never cost the operator SSH access to the machine (spec F10).
func Unmark(cfg, alias, marker string) (string, error) {
	lines := strings.Split(cfg, "\n")
	start, end, ok := blockRange(lines, alias)
	if !ok {
		return "", fmt.Errorf("sshconf: host %q not found", alias)
	}
	out := make([]string, 0, len(lines))
	for i, l := range lines {
		if i < start || i >= end {
			out = append(out, l)
			continue
		}
		code, comment := splitComment(strings.TrimSpace(l))
		if !hasMarker(comment, marker) {
			out = append(out, l)
			continue
		}
		if code == "" {
			continue // comment-only marker line: drop it
		}
		// Trailing marker on a directive: keep the directive, drop the comment.
		out = append(out, strings.TrimRight(l[:strings.Index(l, "#")], " \t"))
	}
	return normalize(strings.Join(out, "\n")), nil
}

// Mark adds the fleet marker to an existing Host block in place, keeping every
// directive. It is the inverse of Unmark and the basis of "adopting" a host
// already present in ~/.ssh/config: no HostName/User need be re-typed. Marking
// an already-marked block is a no-op (idempotent); an unknown alias is an error.
func Mark(cfg, alias, marker string) (string, error) {
	lines := strings.Split(cfg, "\n")
	start, end, ok := blockRange(lines, alias)
	if !ok {
		return "", fmt.Errorf("sshconf: host %q not found", alias)
	}
	for i := start; i < end; i++ {
		_, comment := splitComment(strings.TrimSpace(lines[i]))
		if hasMarker(comment, marker) {
			return normalize(cfg), nil // already in the fleet
		}
	}
	if _, comment := splitComment(strings.TrimSpace(lines[start])); comment == "" {
		// No comment on the Host line: append the marker there.
		lines[start] = strings.TrimRight(lines[start], " \t") + "  " + marker
	} else {
		// The Host line already carries a comment; add an own-line marker just
		// below it so the operator's comment is left untouched. Parse detects a
		// marker on any comment line in the block, and Unmark removes it.
		out := append([]string{}, lines[:start+1]...)
		out = append(out, "    "+marker)
		lines = append(out, lines[start+1:]...)
	}
	return normalize(strings.Join(lines, "\n")), nil
}

// Purge deletes the whole Host block — for a machine that is genuinely gone.
func Purge(cfg, alias string) (string, error) {
	lines := strings.Split(cfg, "\n")
	start, end, ok := blockRange(lines, alias)
	if !ok {
		return "", fmt.Errorf("sshconf: host %q not found", alias)
	}
	// Absorb blank lines the block left behind so repeated add/purge cycles
	// don't accumulate whitespace.
	for end < len(lines) && strings.TrimSpace(lines[end]) == "" {
		end++
	}
	for start > 0 && strings.TrimSpace(lines[start-1]) == "" {
		start--
	}
	out := append(append([]string{}, lines[:start]...), lines[end:]...)
	return normalize(strings.Join(out, "\n")), nil
}

// Update rewrites ONLY the modelled directives of an existing block, in place,
// preserving every unmodelled line, comment, marker, and their order.
//
// Add is unusable for this: it purges the block and re-renders from the struct,
// which would silently take an operator's ProxyCommand with it. That difference
// is the whole reason this function exists — a HostName refresh must not be
// destructive.
//
// An empty field means "leave it alone", never "blank it": omission is not an
// instruction to delete.
func Update(cfg string, h Host) (string, error) {
	if strings.TrimSpace(h.Alias) == "" {
		return "", fmt.Errorf("sshconf: alias is required")
	}
	lines := strings.Split(cfg, "\n")
	start, end, ok := blockRange(lines, h.Alias)
	if !ok {
		return "", fmt.Errorf("sshconf: host %q not found", h.Alias)
	}

	want := []struct{ k, v string }{
		{"HostName", h.HostName}, {"User", h.User},
		{"Port", h.Port}, {"IdentityFile", h.Identity},
	}
	seen := map[string]bool{}

	out := append([]string{}, lines[:start+1]...)
	for i := start + 1; i < end; i++ {
		code, comment := splitComment(strings.TrimSpace(lines[i]))
		f := strings.Fields(code)
		replaced := false
		if len(f) >= 2 {
			for _, kv := range want {
				if kv.v == "" || !strings.EqualFold(f[0], kv.k) {
					continue
				}
				line := "    " + kv.k + " " + kv.v
				if comment != "" {
					// A trailing marker or note on the directive is the
					// operator's, not ours to discard.
					line += "  " + comment
				}
				out = append(out, line)
				seen[kv.k], replaced = true, true
				break
			}
		}
		if !replaced {
			out = append(out, lines[i])
		}
	}
	// Append any modelled directive the block did not already carry.
	for _, kv := range want {
		if kv.v != "" && !seen[kv.k] {
			out = append(out, "    "+kv.k+" "+kv.v)
		}
	}
	out = append(out, lines[end:]...)
	return normalize(strings.Join(out, "\n")), nil
}
