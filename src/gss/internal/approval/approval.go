// Package approval implements the gss push/PR-ready approval token — the
// technical safeguard that forces an agent to obtain user permission,
// bound to a specific commit, before a gated operation (design.md →
// "Safety primitives that must survive verbatim").
//
// The token is a file (default ~/.config/gss/approval.token) whose content
// is the HEAD SHA the user approved. Verify reads it, requires it to equal
// the current HEAD, and consumes (deletes) it on success — so a token is
// single-use and can't be replayed against a later commit. This reproduces
// the semantics of the classic cmd/push.go handshake; the --force-autonomous
// path bypasses the check entirely.
package approval

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wenlock/dotfiles/gss/internal/errors"
	"github.com/wenlock/dotfiles/gss/internal/git"
)

// Reason distinguishes the two ways a token check can fail.
type Reason int

const (
	// ReasonMissing — no readable token file.
	ReasonMissing Reason = iota
	// ReasonMismatch — token present but doesn't match the current HEAD
	// (stale or issued for a different commit).
	ReasonMismatch
)

// Error is the typed approval failure. It always wraps
// errors.ErrApprovalTokenMissing ("approval token missing or invalid for a
// gated operation"), so callers can errors.Is the catalog sentinel while
// still inspecting Reason / the SHAs for diagnostics.
type Error struct {
	Reason   Reason
	Expected string // current HEAD (for ReasonMismatch)
	Got      string // token content (for ReasonMismatch)
	Err      error
}

func (e *Error) Error() string {
	switch e.Reason {
	case ReasonMismatch:
		return fmt.Sprintf("approval: token does not match HEAD (expected %s, token had %s)", e.Expected, e.Got)
	default:
		return "approval: missing or unreadable approval token; the agent must obtain user permission before this operation"
	}
}

func (e *Error) Unwrap() error { return e.Err }

// Verifier checks (and issues) HEAD-bound approval tokens.
type Verifier struct {
	// Path is the token file location (e.g. <state_dir>/approval.token).
	Path string
	// Git reads HEAD; injected for testability.
	Git git.Runner
}

// NewVerifier wires the token path and git runner.
func NewVerifier(path string, gitr git.Runner) *Verifier {
	return &Verifier{Path: path, Git: gitr}
}

// Verify enforces the handshake for the repo at repoPath. When
// forceAutonomous is true the check is skipped entirely (no token is read
// or consumed). Otherwise the token must exist and equal the current HEAD;
// on success the token is consumed and nil is returned. Failures return a
// *Error wrapping errors.ErrApprovalTokenMissing; the token is left in
// place on a mismatch so the user can see what was rejected.
func (v *Verifier) Verify(ctx context.Context, repoPath string, forceAutonomous bool) error {
	if forceAutonomous {
		return nil
	}
	head, err := v.headSHA(ctx, repoPath)
	if err != nil {
		return fmt.Errorf("approval: resolve HEAD: %w", err)
	}
	content, err := os.ReadFile(v.Path)
	if err != nil {
		return &Error{Reason: ReasonMissing, Err: errors.ErrApprovalTokenMissing}
	}
	if token := strings.TrimSpace(string(content)); token != head {
		return &Error{Reason: ReasonMismatch, Expected: head, Got: token, Err: errors.ErrApprovalTokenMissing}
	}
	if err := os.Remove(v.Path); err != nil {
		return fmt.Errorf("approval: consume token: %w", err)
	}
	return nil
}

// Issue writes the current HEAD SHA to the token file (creating parent
// dirs), minting an approval bound to the present commit. This is the
// "user grants permission" half of the handshake.
func (v *Verifier) Issue(ctx context.Context, repoPath string) error {
	head, err := v.headSHA(ctx, repoPath)
	if err != nil {
		return fmt.Errorf("approval: resolve HEAD: %w", err)
	}
	if dir := filepath.Dir(v.Path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("approval: mkdir %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(v.Path, []byte(head+"\n"), 0o600); err != nil {
		return fmt.Errorf("approval: write token: %w", err)
	}
	return nil
}

func (v *Verifier) headSHA(ctx context.Context, repoPath string) (string, error) {
	out, err := v.Git.Run(ctx, "-C", repoPath, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
