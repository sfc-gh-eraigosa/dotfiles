package feature

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/wenlock/dotfiles/gss/internal/errors"
	"github.com/wenlock/dotfiles/gss/internal/gh"
	"github.com/wenlock/dotfiles/gss/internal/identity"
	"github.com/wenlock/dotfiles/gss/internal/registry"
	"github.com/wenlock/dotfiles/gss/internal/stack"
)

// CheckpointOpts configures `feature checkpoint`.
type CheckpointOpts struct {
	WorkerRef string // feature/user/purpose[-suffix]
}

// CheckpointResult reports the outcome.
type CheckpointResult struct {
	Ref     string
	PRURL   string
	PRState string
	Created bool // PR was created (vs edited)
}

// Checkpoint fetches + rebases the worker on its base, renders the PR body
// (with the stack section), and either opens a draft PR (first time) or
// force-pushes + edits the existing PR, then records PR state in the
// registry (design.md → "gss feature checkpoint"). A rebase conflict is
// aborted cleanly and surfaced as errors.ErrRebaseConflict.
func (s *Service) Checkpoint(ctx context.Context, opts CheckpointOpts) (CheckpointResult, error) {
	ref, err := identity.ParseWorkerRef(opts.WorkerRef)
	if err != nil {
		return CheckpointResult{}, err
	}
	reg, err := s.Store.Load()
	if err != nil {
		return CheckpointResult{}, err
	}
	fi, wi := findWorker(reg, ref)
	if fi < 0 {
		return CheckpointResult{}, fmt.Errorf("%w: no such worker %q", errors.ErrInvalidIdent, opts.WorkerRef)
	}
	feat := reg.Features[fi]
	w := feat.Workers[wi]
	base := w.BaseBranch

	if out, err := s.Git.Run(ctx, "-C", w.Worktree, "fetch", "origin"); err != nil {
		return CheckpointResult{}, fmt.Errorf("checkpoint: fetch: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if out, err := s.Git.Run(ctx, "-C", w.Worktree, "rebase", "origin/"+base); err != nil {
		_, _ = s.Git.Run(ctx, "-C", w.Worktree, "rebase", "--abort") // clean abort; user resolves
		return CheckpointResult{}, fmt.Errorf("%w: rebase onto origin/%s: %s", errors.ErrRebaseConflict, base, strings.TrimSpace(string(out)))
	}

	body := renderPRBody(feat, ref)
	title := fmt.Sprintf("%s: %s", ref.Feature, ref.Purpose) // first H1 of WORKER.md

	res := CheckpointResult{Ref: ref.String()}
	if w.PRURL == "" {
		pr, err := s.GH.PRCreate(ctx, gh.PRCreateOpts{Title: title, Body: body, Base: base, Head: w.Branch, Draft: true})
		if err != nil {
			return CheckpointResult{}, fmt.Errorf("checkpoint: pr create: %w", err)
		}
		res.Created = true
		res.PRURL = pr.URL
		res.PRState = "draft"
	} else {
		if out, err := s.Git.Run(ctx, "-C", w.Worktree, "push", "--force-with-lease", "origin", w.Branch); err != nil {
			return CheckpointResult{}, fmt.Errorf("checkpoint: force-push: %w: %s", err, strings.TrimSpace(string(out)))
		}
		if err := s.GH.PREdit(ctx, prNumber(w.PRURL), gh.PREditOpts{Base: base, Body: body}); err != nil {
			return CheckpointResult{}, fmt.Errorf("checkpoint: pr edit: %w", err)
		}
		res.PRURL = w.PRURL
		res.PRState = w.PRState
	}

	// Persist PR state.
	if err := s.Store.Update(func(r *registry.Registry) error {
		f, wIdx := findWorker(*r, ref)
		if f < 0 {
			return nil
		}
		r.Features[f].Workers[wIdx].PRURL = res.PRURL
		r.Features[f].Workers[wIdx].PRState = res.PRState
		return nil
	}); err != nil {
		return CheckpointResult{}, err
	}
	return res, nil
}

// findWorker returns the (featureIndex, workerIndex) of the worker matching
// ref, or (-1, -1).
func findWorker(reg registry.Registry, ref identity.WorkerRef) (int, int) {
	for fi := range reg.Features {
		if reg.Features[fi].Name != ref.Feature {
			continue
		}
		for wi := range reg.Features[fi].Workers {
			w := reg.Features[fi].Workers[wi]
			if w.User == ref.User && w.Purpose == ref.Purpose && w.Suffix == ref.Suffix {
				return fi, wi
			}
		}
	}
	return -1, -1
}

// renderPRBody builds the PR body: the worker's description as free text
// plus the stack section (design.md → "PR body — stack section").
func renderPRBody(f registry.Feature, here identity.WorkerRef) string {
	view := stack.StackView{Feature: f.Name}
	desc := ""
	for _, w := range f.Workers {
		isHere := w.User == here.User && w.Purpose == here.Purpose && w.Suffix == here.Suffix
		if isHere {
			desc = w.Description
		}
		leaf := w.Purpose
		if w.Suffix != "" {
			leaf = w.Purpose + "-" + w.Suffix
		}
		view.Entries = append(view.Entries, stack.Entry{
			PRNumber: prNumber(w.PRURL),
			Ref:      w.User + "/" + leaf,
			Base:     w.BaseBranch,
			Here:     isHere,
		})
	}
	return stack.RenderBody(desc, view)
}

var prURLNumRe = regexp.MustCompile(`/pull/(\d+)`)

func prNumber(url string) int {
	m := prURLNumRe.FindStringSubmatch(url)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}
