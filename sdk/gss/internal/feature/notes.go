package feature

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// notesHeadingRe matches the "Decisions & notes" heading at any ATX level,
// tolerating the "and"/"&" spelling so a hand-edited FEATURE.md still works.
var notesHeadingRe = regexp.MustCompile(`(?i)^#{1,6}\s+decisions\s*(&|and)\s*notes\s*$`)

// anyHeadingRe matches any ATX heading — used to find where the section ends.
var anyHeadingRe = regexp.MustCompile(`^#{1,6}\s+`)

// htmlCommentRe strips HTML comments, which is how the FEATURE.md template
// carries its "append freely" instructions. Those are guidance for the author
// and must not leak into published PR bodies.
var htmlCommentRe = regexp.MustCompile(`(?s)<!--.*?-->`)

// featureNotes returns the body of FEATURE.md's "Decisions & notes" section
// for feat, or "" when the file, the section, or its content is absent.
//
// The FEATURE.md template promises these notes are "surfaced in every
// worker's PR body". Nothing read the file, so the promise was false; this is
// the reader that makes it true.
//
// Errors are deliberately swallowed: notes are decoration on a PR body, and
// an unreadable FEATURE.md must never fail a checkpoint that has already
// pushed commits.
func (s *Service) featureNotes(featureName string) string {
	if s.WorktreeRoot == "" || s.NWO == "" || featureName == "" {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(s.WorktreeRoot, s.NWO, featureName, "FEATURE.md"))
	if err != nil {
		return ""
	}
	return extractNotes(string(raw))
}

// extractNotes pulls the "Decisions & notes" section body out of FEATURE.md
// content. Exported behaviour lives on featureNotes; this is split out so the
// parsing is testable without touching the filesystem.
func extractNotes(md string) string {
	lines := strings.Split(md, "\n")
	start := -1
	for i, ln := range lines {
		if notesHeadingRe.MatchString(strings.TrimSpace(ln)) {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return ""
	}

	end := len(lines)
	for i := start; i < len(lines); i++ {
		if anyHeadingRe.MatchString(strings.TrimSpace(lines[i])) {
			end = i
			break
		}
	}

	section := strings.Join(lines[start:end], "\n")
	section = htmlCommentRe.ReplaceAllString(section, "")
	return strings.TrimSpace(section)
}
