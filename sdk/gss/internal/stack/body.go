package stack

import (
	"fmt"
	"regexp"
	"strings"
)

// Stack-section markers. gss rewrites only the content between them, so the
// section is idempotent and never disturbs free-form PR body text
// (design.md → "PR body — stack section").
const (
	markerBegin = "<!-- gss:stack-begin -->"
	markerEnd   = "<!-- gss:stack-end -->"
)

// managedBlockRe matches a complete begin..end managed section.
var managedBlockRe = regexp.MustCompile(`(?s)<!-- gss:stack-begin -->.*?<!-- gss:stack-end -->`)

// strayMarkerRe matches any leftover gss:stack-* marker comment. After the
// managed block is removed, any remaining such token was authored by the
// user (or an attacker) and is stripped before stitching — defence against
// PR-body marker injection (security-review #7).
var strayMarkerRe = regexp.MustCompile(`<!--\s*gss:stack[^>]*-->`)

// Feature-notes markers. FEATURE.md's "Decisions & notes" is shared across a
// feature's workers, so it is mirrored into each PR body as a second managed
// section — same idempotent begin/end contract as the stack section.
const (
	notesBegin = "<!-- gss:notes-begin -->"
	notesEnd   = "<!-- gss:notes-end -->"
)

var notesBlockRe = regexp.MustCompile(`(?s)<!-- gss:notes-begin -->.*?<!-- gss:notes-end -->`)

// strayNotesRe mirrors strayMarkerRe for the notes markers.
var strayNotesRe = regexp.MustCompile(`<!--\s*gss:notes[^>]*-->`)

// RenderNotes returns body with the feature-notes section injected/updated,
// leaving all unmanaged text untouched. Empty notes removes the section, so
// clearing FEATURE.md's notes clears them from every PR. Idempotent:
// RenderNotes(RenderNotes(b, n), n) == RenderNotes(b, n).
func RenderNotes(body, notes string) string {
	clean := notesBlockRe.ReplaceAllString(body, "")
	clean = strayNotesRe.ReplaceAllString(clean, "")
	clean = strings.TrimRight(clean, " \t\r\n")

	notes = strings.TrimSpace(notes)
	if notes == "" {
		return clean
	}

	section := notesBegin + "\n## Feature notes\n\n" + notes + "\n" + notesEnd
	if clean == "" {
		return section + "\n"
	}
	return clean + "\n\n" + section + "\n"
}

// Entry is one row of the rendered stack section, bottom→top.
type Entry struct {
	PRNumber int    // 0 → "(no PR yet)"
	Ref      string // user/purpose[-suffix] display
	Base     string // base branch
	Here     bool   // this row is the current PR
}

// StackView is the data for RenderBody.
type StackView struct {
	Feature string
	Entries []Entry // ordered bottom→top
}

// RenderBody returns body with the stack section injected/updated. It first
// removes any existing managed block AND any stray gss:stack markers from
// the user content (injection defence), then appends the freshly rendered
// section. Idempotent: RenderBody(RenderBody(b, v), v) == RenderBody(b, v).
func RenderBody(body string, view StackView) string {
	clean := managedBlockRe.ReplaceAllString(body, "")
	clean = strayMarkerRe.ReplaceAllString(clean, "")
	clean = strings.TrimRight(clean, " \t\r\n")

	section := renderSection(view)
	if clean == "" {
		return section + "\n"
	}
	return clean + "\n\n" + section + "\n"
}

func renderSection(v StackView) string {
	hereIdx := -1
	for i, e := range v.Entries {
		if e.Here {
			hereIdx = i
		}
	}

	var b strings.Builder
	b.WriteString(markerBegin + "\n")
	b.WriteString("## Stack\n\n")
	fmt.Fprintf(&b, "This PR is part of a stack on **%s**.\n\n", v.Feature)
	for i, e := range v.Entries {
		row := entryText(e)
		switch {
		case e.Here:
			fmt.Fprintf(&b, "- **%s** ← you are here\n", row)
		case i == hereIdx-1:
			fmt.Fprintf(&b, "- %s ← parent of this PR\n", row)
		default:
			fmt.Fprintf(&b, "- %s\n", row)
		}
	}
	b.WriteString("\nReview bottom-up. Merge bottom-up; gss will re-target the rest of the stack when a parent merges.\n")
	b.WriteString(markerEnd)
	return b.String()
}

func entryText(e Entry) string {
	num := "(no PR yet)"
	if e.PRNumber > 0 {
		num = fmt.Sprintf("#%d", e.PRNumber)
	}
	return fmt.Sprintf("%s — %s (base: `%s`)", num, e.Ref, e.Base)
}
