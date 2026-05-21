// Package status implements `gss status`: a human-readable rendering of
// `git status --porcelain` (design.md → "Existing features that must
// survive the refactor"). Format reproduces the classic cmd/status.go
// output byte-for-byte — the porcelain lines are passed through verbatim,
// preserving git's own column alignment that slash-command consumers rely
// on.
package status

import (
	"context"
	"fmt"
	"strings"

	"github.com/wenlock/dotfiles/gss/internal/git"
)

// Service renders repo status via an injected git.Runner.
type Service struct {
	Git git.Runner
}

// NewService wires the git runner.
func NewService(gitr git.Runner) *Service { return &Service{Git: gitr} }

// Status runs `git status --porcelain` at repoPath and returns the
// formatted report.
func (s *Service) Status(ctx context.Context, repoPath string) (string, error) {
	out, err := s.Git.Run(ctx, "-C", repoPath, "status", "--porcelain")
	if err != nil {
		return "", fmt.Errorf("status: git status in %s: %w", repoPath, err)
	}
	return Format(repoPath, string(out)), nil
}

// Format renders porcelain output into the classic report. A tree with no
// non-blank porcelain lines prints "No changes detected in <path>."; a
// dirty tree prints "Changes in <path>:" followed by one " - <line>" per
// porcelain line. Output is byte-identical to classic cmd/status.go.
func Format(repoPath, porcelain string) string {
	lines := strings.Split(porcelain, "\n")

	dirty := false
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			dirty = true
			break
		}
	}
	if !dirty {
		return fmt.Sprintf("No changes detected in %s.\n", repoPath)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Changes in %s:\n", repoPath)
	for _, line := range lines {
		if line == "" {
			continue
		}
		fmt.Fprintf(&b, " - %s\n", line)
	}
	return b.String()
}
