package feature

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/errors"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/registry"
)

// ConflictsOpts configures `feature conflicts`.
type ConflictsOpts struct {
	Feature string
	JSON    bool
}

// Conflict is one pair of workers that touched a common file.
type Conflict struct {
	WorkerA string   `json:"worker_a"`
	WorkerB string   `json:"worker_b"`
	Files   []string `json:"files"`
}

// ConflictReport is the result of a conflict scan over one feature.
type ConflictReport struct {
	Feature   string     `json:"feature"`
	Conflicts []Conflict `json:"conflicts"`
}

// Conflicts surfaces file overlap between workers (design.md → "gss feature
// conflicts"). For each worker it reads the files changed vs the worker's
// base (`git diff --name-only base...HEAD`), then reports every pair with a
// non-empty intersection. It is read-only — it NEVER attempts to merge or
// resolve; the actual merge is git's job.
//
// opts.Feature selects one feature by name; "" (the CLI default, per
// --help) pools every worker across every feature in the registry instead
// of erroring (dotfiles#332 — a bare `gss feature conflicts` used to look
// up a feature literally named "" and fail with `no such feature ""`,
// even though the help text always said the empty filter means "all").
// Worker refs are already feature-qualified ("feature/user/purpose[-suffix]"),
// so a cross-feature overlap is reported the same way a same-feature one is
// — no shape change to Conflict or the JSON envelope.
func (s *Service) Conflicts(ctx context.Context, opts ConflictsOpts) (ConflictReport, error) {
	reg, err := s.Store.Load()
	if err != nil {
		return ConflictReport{}, err
	}
	var feats []registry.Feature
	if opts.Feature == "" {
		feats = reg.Features
	} else {
		f, err := findFeature(reg, opts.Feature)
		if err != nil {
			return ConflictReport{}, err
		}
		feats = []registry.Feature{f}
	}

	type wf struct {
		ref   string
		files map[string]bool
	}
	var wfs []wf
	for _, f := range feats {
		for _, w := range f.Workers {
			files, err := s.changedFiles(ctx, w.Worktree, w.BaseBranch)
			if err != nil {
				return ConflictReport{}, fmt.Errorf("feature conflicts: %s: %w", workerRef(f.Name, w), err)
			}
			wfs = append(wfs, wf{ref: workerRef(f.Name, w), files: files})
		}
	}

	report := ConflictReport{Feature: opts.Feature}
	for i := 0; i < len(wfs); i++ {
		for j := i + 1; j < len(wfs); j++ {
			if overlap := intersect(wfs[i].files, wfs[j].files); len(overlap) > 0 {
				report.Conflicts = append(report.Conflicts, Conflict{
					WorkerA: wfs[i].ref, WorkerB: wfs[j].ref, Files: overlap,
				})
			}
		}
	}
	return report, nil
}

func (s *Service) changedFiles(ctx context.Context, worktree, base string) (map[string]bool, error) {
	out, err := s.Git.Run(ctx, "-C", worktree, "diff", "--name-only", base+"...HEAD")
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		if fpath := strings.TrimSpace(line); fpath != "" {
			set[fpath] = true
		}
	}
	return set, nil
}

func intersect(a, b map[string]bool) []string {
	var out []string
	for fpath := range a {
		if b[fpath] {
			out = append(out, fpath)
		}
	}
	sort.Strings(out)
	return out
}

func findFeature(reg registry.Registry, name string) (registry.Feature, error) {
	for _, f := range reg.Features {
		if f.Name == name {
			return f, nil
		}
	}
	return registry.Feature{}, fmt.Errorf("%w: no such feature %q", errors.ErrInvalidIdent, name)
}

// Text renders the report for humans. An empty Feature (the all-features
// scan) reads as "across all features" rather than "on ".
func (r ConflictReport) Text() string {
	label := "on " + r.Feature
	if r.Feature == "" {
		label = "across all features"
	}
	if len(r.Conflicts) == 0 {
		return fmt.Sprintf("No file conflicts among workers %s.\n", label)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Conflicts %s:\n", label)
	for _, c := range r.Conflicts {
		fmt.Fprintf(&b, "  %s <-> %s:\n", c.WorkerA, c.WorkerB)
		for _, fpath := range c.Files {
			fmt.Fprintf(&b, "    - %s\n", fpath)
		}
	}
	return b.String()
}

// MarshalReport renders the report as stable JSON.
func (r ConflictReport) MarshalReport() ([]byte, error) {
	if r.Conflicts == nil {
		r.Conflicts = []Conflict{}
	}
	return json.MarshalIndent(r, "", "  ")
}
