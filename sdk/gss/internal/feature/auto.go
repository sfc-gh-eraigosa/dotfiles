package feature

import (
	"context"
	stderrors "errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/errors"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/identity"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/registry"
)

// AutoOpts configures `feature checkpoint --auto`.
type AutoOpts struct {
	WorkerRef string // explicit; never depends on cwd
	DryRun    bool
}

// AutoResult reports what auto-checkpoint did.
type AutoResult struct {
	Ref       string
	NoOp      bool     // nothing to do (clean + already pushed)
	Committed bool     // made a WIP commit
	Planned   []string // --dry-run plan (no execution)
	Skipped   string   // skip reason, if a prompt-condition was hit
}

// AutoCheckpoint is the non-interactive checkpoint variant safe to call
// from a process hook (design.md → "gss feature checkpoint --auto"):
//   - explicit --worker (no cwd dependency);
//   - silent no-op when clean and already pushed;
//   - auto-commits TRACKED changes only (never `git add -A`), listing
//     untracked files in the commit body;
//   - never prompts: on detached HEAD / rebase conflict it skips, writes a
//     diagnostic to WORKER.md, and returns a non-zero (error) result;
//   - only ever touches DRAFT PRs (a ready PR gets a body refresh, no new
//     commits pushed);
//   - --dry-run prints the plan without executing.
func (s *Service) AutoCheckpoint(ctx context.Context, opts AutoOpts) (AutoResult, error) {
	ref, err := identity.ParseWorkerRef(opts.WorkerRef)
	if err != nil {
		return AutoResult{}, err
	}
	reg, err := s.Store.Load()
	if err != nil {
		return AutoResult{}, err
	}
	fi, wi := findWorker(reg, ref)
	if fi < 0 {
		return AutoResult{}, fmt.Errorf("%w: no such worker %q", errors.ErrInvalidIdent, opts.WorkerRef)
	}
	w := reg.Features[fi].Workers[wi]
	res := AutoResult{Ref: ref.String()}

	branch, _ := s.gitOut(ctx, w.Worktree, "rev-parse", "--abbrev-ref", "HEAD")
	if branch == "" || branch == "HEAD" {
		return s.autoSkip(w, "detached HEAD; skipping auto-checkpoint")
	}
	porcelain, _ := s.gitOut(ctx, w.Worktree, "status", "--porcelain")
	tracked, untracked := splitPorcelain(porcelain)
	local, _ := s.gitOut(ctx, w.Worktree, "rev-parse", "HEAD")
	remote, rerr := s.gitOut(ctx, w.Worktree, "rev-parse", "origin/"+w.Branch)
	if rerr != nil {
		remote = ""
	}

	needCommit := len(tracked) > 0
	needPush := remote == "" || local != remote || needCommit

	if opts.DryRun {
		if needCommit {
			res.Planned = append(res.Planned, fmt.Sprintf("commit WIP (%d tracked file(s))", len(tracked)))
		}
		if needPush {
			res.Planned = append(res.Planned, "checkpoint (push + update PR)")
		}
		if len(res.Planned) == 0 {
			res.NoOp = true
		}
		return res, nil
	}

	if !needCommit && !needPush {
		res.NoOp = true // silent
		return res, nil
	}

	if needCommit {
		addArgs := append([]string{w.Worktree, "add", "--"}, tracked...)
		if _, err := s.Git.Run(ctx, "-C", addArgs...); err != nil {
			return s.autoSkip(w, "git add failed: "+err.Error())
		}
		commitArgs := []string{w.Worktree, "commit", "-m", "chore(wip): auto-checkpoint @ " + s.now()}
		if len(untracked) > 0 {
			commitArgs = append(commitArgs, "-m", "untracked (not added):\n"+strings.Join(untracked, "\n"))
		}
		if _, err := s.Git.Run(ctx, "-C", commitArgs...); err != nil {
			return s.autoSkip(w, "wip commit failed")
		}
		res.Committed = true
	}

	// Only ever touch draft PRs: a ready PR gets a body refresh, never new
	// pushed commits (avoids surprising reviewers).
	if w.PRURL != "" {
		if pr, err := s.GH.PRView(ctx, prNumber(w.PRURL)); err == nil && !pr.IsDraft {
			if err := s.GH.PREdit(ctx, prNumber(w.PRURL), gh.PREditOpts{Body: renderPRBody(reg.Features[fi], ref)}); err != nil {
				return res, err
			}
			return res, nil
		}
	}

	// Delegate the rebase + push + PR to Checkpoint. A rebase conflict is a
	// skip (diagnostic + non-zero), not a hard failure.
	if _, err := s.Checkpoint(ctx, CheckpointOpts{WorkerRef: opts.WorkerRef}); err != nil {
		if stderrors.Is(err, errors.ErrRebaseConflict) {
			return s.autoSkip(w, "rebase conflict; resolve in the worktree then re-run")
		}
		return res, err
	}
	return res, nil
}

func (s *Service) gitOut(ctx context.Context, worktree string, sub ...string) (string, error) {
	full := append([]string{worktree}, sub...)
	out, err := s.Git.Run(ctx, "-C", full...)
	return strings.TrimSpace(string(out)), err
}

// autoSkip records a one-line diagnostic in the worker's WORKER.md and
// returns a non-zero (error) result so the caller can surface it.
func (s *Service) autoSkip(w registry.Worker, reason string) (AutoResult, error) {
	s.appendAutoLog(w.Worktree, reason)
	return AutoResult{Skipped: reason}, fmt.Errorf("auto-checkpoint skipped: %s", reason)
}

func (s *Service) appendAutoLog(worktree, reason string) {
	path := filepath.Join(worktree, "WORKER.md")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(fmt.Sprintf("\n## Auto-checkpoint log\n- %s %s\n", s.now(), reason))
}

// splitPorcelain partitions `git status --porcelain` lines into tracked
// changes and untracked ("?? ") paths.
func splitPorcelain(p string) (tracked, untracked []string) {
	for _, line := range strings.Split(p, "\n") {
		if len(line) < 4 {
			continue
		}
		// Porcelain v1: "XY <path>" — XY is 2 status columns; TrimSpace after
		// column 2 yields the path robustly regardless of the separator.
		path := strings.TrimSpace(line[2:])
		if strings.HasPrefix(line, "?? ") {
			untracked = append(untracked, path)
		} else {
			tracked = append(tracked, path)
		}
	}
	return tracked, untracked
}
