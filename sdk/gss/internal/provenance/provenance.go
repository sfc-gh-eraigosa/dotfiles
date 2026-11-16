// Package provenance records where an assistant's work came from, so a later
// escape hunt can go from a line on main to the harness, session and host
// that made it.
//
// Two places carry it:
//
//   - Commit trailers, stamped by the commit-msg privacy hook through
//     `gss provenance stamp` when an assistant made the commit: Built-By,
//     Built-On, Built-Session and Hooks. A commit from an assistant with no
//     Hooks trailer is a commit the global hooks did not see.
//   - A managed "Provenance" section in each PR body gss writes at
//     checkpoint, with the same fields plus the hooks state of the worktree.
//
// The host is never written by name. The short host name is one of the
// identity tokens the privacy guard refuses, so Built-On carries HostID, a
// short hash of it; `gss provenance show --host <name>` maps a name to its
// id on any machine.
package provenance

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

// Env is the environment lookup (os.Getenv in production).
type Env func(string) string

// Info is one machine's and session's provenance.
type Info struct {
	Harness string // claude, agy, a loop model …; "" when no assistant is detected
	Session string // the harness's session id; "" when unknown
	HostID  string // HostID of the short host name
}

// Assistant reports whether an assistant was detected.
func (i Info) Assistant() bool { return i.Harness != "" }

// Detect reads the harness and session from the environment:
//
//   - GSS_BUILT_BY / GSS_BUILT_SESSION, set by whatever launched the
//     builder (the conductor loop, a script), win;
//   - CLAUDECODE=1 is Claude Code, whose session is CLAUDE_CODE_SESSION_ID
//     (the transcript's file name on that host);
//   - AI_AGENT (for example "claude-code_2-1-325_agent") names other agents
//     by the part before the first "_".
func Detect(env Env, hostname string) Info {
	info := Info{HostID: HostID(hostname)}
	switch {
	case env("GSS_BUILT_BY") != "":
		info.Harness = clean(env("GSS_BUILT_BY"))
	case env("CLAUDECODE") == "1":
		info.Harness = "claude"
		info.Session = clean(env("CLAUDE_CODE_SESSION_ID"))
	case env("AI_AGENT") != "":
		name, _, _ := strings.Cut(env("AI_AGENT"), "_")
		info.Harness = clean(strings.TrimSuffix(name, "-code"))
	}
	if s := env("GSS_BUILT_SESSION"); s != "" {
		info.Session = clean(s)
	}
	return info
}

// HostID is "h-" and the first 10 hex digits of the SHA-256 of the
// lower-cased short host name: stable per host, and not the name itself.
func HostID(hostname string) string {
	short, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(hostname)), ".")
	if short == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(short))
	return "h-" + hex.EncodeToString(sum[:])[:10]
}

// Trailers are the commit trailers for info, in a fixed order. Hooks is
// "global": only the global commit-msg hook stamps them, so their presence
// says that hook ran. None when no assistant is detected.
func Trailers(info Info) []string {
	if !info.Assistant() {
		return nil
	}
	out := []string{"Built-By: " + info.Harness}
	if info.HostID != "" {
		out = append(out, "Built-On: "+info.HostID)
	}
	if info.Session != "" {
		out = append(out, "Built-Session: "+info.Session)
	}
	return append(out, "Hooks: global")
}

// Section markers for the PR body, same contract as the stack section.
const (
	markerBegin = "<!-- gss:provenance-begin -->"
	markerEnd   = "<!-- gss:provenance-end -->"
)

var (
	blockRe = regexp.MustCompile(`(?s)<!-- gss:provenance-begin -->.*?<!-- gss:provenance-end -->`)
	strayRe = regexp.MustCompile(`<!--\s*gss:provenance[^>]*-->`)
)

// RenderBody returns body with the provenance section set to info and hooks
// (the worktree's hooks state, e.g. "global" or "local override: .husky").
// An existing section is replaced where it is, so the body's order is kept;
// otherwise the section is appended. Stray markers are stripped first.
// Idempotent.
func RenderBody(body string, info Info, hooks string) string {
	section := renderSection(info, hooks)
	if loc := blockRe.FindStringIndex(body); loc != nil {
		head := strayRe.ReplaceAllString(body[:loc[0]], "")
		tail := blockRe.ReplaceAllString(body[loc[1]:], "")
		tail = strayRe.ReplaceAllString(tail, "")
		return head + section + tail
	}
	clean := strings.TrimRight(strayRe.ReplaceAllString(body, ""), " \t\r\n")
	if clean == "" {
		return section + "\n"
	}
	return clean + "\n\n" + section + "\n"
}

func renderSection(info Info, hooks string) string {
	var b strings.Builder
	b.WriteString(markerBegin + "\n## Provenance\n\n")
	b.WriteString("Written by gss at the last checkpoint.\n\n")
	if info.Assistant() {
		fmt.Fprintf(&b, "- Built by: %s\n", info.Harness)
	} else {
		b.WriteString("- Built by: no assistant detected\n")
	}
	if info.Session != "" {
		fmt.Fprintf(&b, "- Session: `%s`\n", info.Session)
	}
	if info.HostID != "" {
		fmt.Fprintf(&b, "- Host: `%s` (`gss provenance show --host <name>` maps a name to it)\n", info.HostID)
	}
	if hooks != "" {
		fmt.Fprintf(&b, "- Privacy hooks: %s\n", oneLine(hooks))
	}
	b.WriteString(markerEnd)
	return b.String()
}

var unsafeRe = regexp.MustCompile(`[^A-Za-z0-9._:+-]`)

// clean keeps a value to one short token, so an environment variable cannot
// inject text into a trailer or a PR body.
func clean(s string) string {
	s = unsafeRe.ReplaceAllString(strings.TrimSpace(s), "")
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}

// oneLine keeps a free-text value (the hooks state can carry a git error) on
// one list line, without HTML comments that could forge a gss marker.
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	s = strings.NewReplacer("<!--", "", "-->", "").Replace(s)
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
