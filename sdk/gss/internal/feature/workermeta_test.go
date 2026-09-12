package feature

import (
	"strings"
	"testing"
)

const workerMD = `# auth: api

- **Description**: endpoints
- **Branch**: feature/auth/erai/api

## Goal
Ship the endpoints.

## Decisions & notes
<!-- append freely; seeds the PR body -->
- chose REST over gRPC

## Open questions
- none
`

func TestExtractWorkerProse_GoalThenNotes(t *testing.T) {
	got := extractWorkerProse(workerMD)
	want := "Ship the endpoints.\n\n## Decisions & notes\n\n- chose REST over gRPC"
	if got != want {
		t.Errorf("extractWorkerProse =\n%q\nwant\n%q", got, want)
	}
}

func TestExtractWorkerProse_TemplateInstructionsAreNotPublished(t *testing.T) {
	got := extractWorkerProse(workerMD)
	if strings.Contains(got, "append freely") || strings.Contains(got, "<!--") {
		t.Errorf("template comment leaked into PR prose: %q", got)
	}
	if strings.Contains(got, "Open questions") || strings.Contains(got, "Description") {
		t.Errorf("sections other than Goal/notes leaked: %q", got)
	}
}

func TestExtractWorkerProse_UntouchedTemplateIsEmpty(t *testing.T) {
	// A worker added and checkpointed without editing WORKER.md: both
	// sections hold only the template's comments → "" → description fallback.
	md := "# f: p\n\n## Goal\n<!-- describe this worker's goal -->\n\n## Decisions & notes\n<!-- append freely -->\n\n## Open questions\n"
	if got := extractWorkerProse(md); got != "" {
		t.Errorf("extractWorkerProse(template) = %q; want \"\"", got)
	}
}

func TestExtractWorkerProse_NotesOnly(t *testing.T) {
	md := "## Goal\n\n## Decisions & notes\n- a\n- b\n"
	if got := extractWorkerProse(md); got != "## Decisions & notes\n\n- a\n- b" {
		t.Errorf("notes-only = %q", got)
	}
}
