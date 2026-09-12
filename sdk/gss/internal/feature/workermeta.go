package feature

import (
	"os"
	"regexp"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/registry"
)

// goalHeadingRe matches WORKER.md's "## Goal" heading at any ATX level.
var goalHeadingRe = regexp.MustCompile(`(?i)^#{1,6}\s+goal\s*$`)

// workerProse returns the free-form text a NEW PR body is seeded with: the
// worker's WORKER.md "Goal" and "Decisions & notes" sections, template
// comments stripped. "" when the file or both sections are absent, in which
// case renderPRBody falls back to the one-line registry description.
//
// Read on create only. Once the PR exists its body belongs to whoever edits
// it on GitHub, so later WORKER.md edits are never pushed over it — the same
// rule that keeps feature notes marker-scoped. The WORKER.md template used to
// promise its notes were "rendered verbatim into PR body" while nothing read
// the file; this is the reader that makes the (narrowed) promise true.
//
// Errors are swallowed: prose is decoration, and an unreadable WORKER.md must
// never fail a checkpoint.
func (s *Service) workerProse(w registry.Worker) string {
	if w.Worktree == "" {
		return ""
	}
	raw, err := os.ReadFile(WorkerMetaPath(w.Worktree))
	if err != nil {
		return ""
	}
	return extractWorkerProse(string(raw))
}

// extractWorkerProse composes the PR prose from WORKER.md content: the Goal
// body first (it reads as the PR summary), then the notes under their own
// heading. Split out so the parsing is testable without the filesystem.
func extractWorkerProse(md string) string {
	var parts []string
	if goal := extractSection(md, goalHeadingRe); goal != "" {
		parts = append(parts, goal)
	}
	if notes := extractNotes(md); notes != "" {
		parts = append(parts, "## Decisions & notes\n\n"+notes)
	}
	return strings.Join(parts, "\n\n")
}
