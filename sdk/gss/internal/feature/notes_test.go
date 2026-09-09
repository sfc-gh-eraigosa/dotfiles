package feature

import "testing"

// TestExtractNotes_TemplateInstructionsAreNotPublished guards the case that
// made the feature unsafe to ship naively: the FEATURE.md template carries
// its authoring guidance in HTML comments, which must never reach a PR body.
func TestExtractNotes_TemplateInstructionsAreNotPublished(t *testing.T) {
	md := "# Feature: x\n\n## Decisions & notes\n<!-- append freely; surfaced in every worker's PR body -->\n"
	if got := extractNotes(md); got != "" {
		t.Errorf("extractNotes = %q; want empty (comment-only section)", got)
	}
}

func TestExtractNotes_ReadsSectionBody(t *testing.T) {
	md := `# Feature: x

## Goal

not this

## Decisions & notes
<!-- append freely -->

We chose X over Y because Z.

## Workers

not this either
`
	want := "We chose X over Y because Z."
	if got := extractNotes(md); got != want {
		t.Errorf("extractNotes = %q; want %q", got, want)
	}
}

// TestExtractNotes_StopsAtNextHeading pins the section boundary: notes must
// not swallow the sections that follow them.
func TestExtractNotes_StopsAtNextHeading(t *testing.T) {
	md := "## Decisions & notes\n\nkeep me\n\n## Workers\n\n- drop me\n"
	if got := extractNotes(md); got != "keep me" {
		t.Errorf("extractNotes = %q; want %q", got, "keep me")
	}
}

func TestExtractNotes_AbsentSection(t *testing.T) {
	if got := extractNotes("# Feature: x\n\n## Goal\n\nsomething\n"); got != "" {
		t.Errorf("extractNotes = %q; want empty", got)
	}
}

// TestExtractNotes_AcceptsAndSpelling keeps a hand-edited heading working.
func TestExtractNotes_AcceptsAndSpelling(t *testing.T) {
	if got := extractNotes("## Decisions and Notes\n\nkeep\n"); got != "keep" {
		t.Errorf("extractNotes = %q; want %q", got, "keep")
	}
}
