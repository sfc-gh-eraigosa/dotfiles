// Package backup creates the local safety branch gss makes before a
// destructive operation (design.md → "Safety primitives that must survive
// verbatim"). It reproduces the classic cmd/backup.go branch-name format
// — backup/gss-<YYYYMMDD-HHMMSS> — but takes the timestamp from an
// injected Clock and, if that name already exists (e.g. two runs in the
// same second), appends a monotonic -N suffix so a rerun is idempotent
// rather than failing.
package backup

import (
	"context"
	"fmt"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git"
)

// timeFormat is the fixed timestamp layout, byte-identical to the classic
// cmd/backup.go ("20060102-150405").
const timeFormat = "20060102-150405"

// maxSuffix caps the monotonic-suffix search so a pathological repo can't
// spin Create forever.
const maxSuffix = 1000

// Service creates backup branches via an injected git.Runner and Clock.
type Service struct {
	Git   git.Runner
	Clock config.Clock
}

// NewService wires the dependencies.
func NewService(gitr git.Runner, clock config.Clock) *Service {
	return &Service{Git: gitr, Clock: clock}
}

// Create makes branch backup/gss-<timestamp> at repoPath and returns its
// name. For a fresh timestamp the name is byte-identical to the classic
// command's. If the name already exists, Create appends -2, -3, … until a
// free name is found (idempotent rerun).
func (s *Service) Create(ctx context.Context, repoPath string) (string, error) {
	base := fmt.Sprintf("backup/gss-%s", s.Clock.Now().Format(timeFormat))
	name := base
	for i := 2; s.branchExists(ctx, repoPath, name); i++ {
		if i > maxSuffix {
			return "", fmt.Errorf("backup: exhausted %d suffixes for %s", maxSuffix, base)
		}
		name = fmt.Sprintf("%s-%d", base, i)
	}
	if out, err := s.Git.Run(ctx, "-C", repoPath, "branch", name); err != nil {
		return "", fmt.Errorf("backup: create branch %s: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return name, nil
}

// branchExists reports whether a local branch ref exists. `git rev-parse
// --verify --quiet refs/heads/<name>` exits 0 when present, non-zero when
// not — so a nil error means the branch exists.
func (s *Service) branchExists(ctx context.Context, repoPath, name string) bool {
	_, err := s.Git.Run(ctx, "-C", repoPath, "rev-parse", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}
