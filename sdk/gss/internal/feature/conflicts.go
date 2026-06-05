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

// Conflicts surfaces file overlap between the workers of a feature
// (design.md → "gss feature conflicts"). For each worker it reads the files
// changed vs the worker's base (`git diff --name-only base...HEAD`), then
// reports every pair with a non-empty intersection. It is read-only — it
// NEVER attempts to merge or resolve; the actual merge is git's job.
func (s *Service) Conflicts(ctx context.Context, opts ConflictsOpts) (ConflictReport, error) {
	reg, err := s.Store.Load()
	if err != nil {
		return ConflictReport{}, err
	}
	f, err := findFeature(reg, opts.Feature)
	if err != nil {
		return ConflictReport{}, err
	}

	type wf struct {
		ref   string
		files map[string]bool
	}
	var wfs []wf
	for _, w := range f.Workers {
		files, err := s.changedFiles(ctx, w.Worktree, w.BaseBranch)
		if err != nil {
			return ConflictReport{}, fmt.Errorf("feature conflicts: %s: %w", workerRef(f.Name, w), err)
		}
		wfs = append(wfs, wf{ref: workerRef(f.Name, w), files: files})
	}

	report := ConflictReport{Feature: f.Name}
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

// Text renders the report for humans.
func (r ConflictReport) Text() string {
	if len(r.Conflicts) == 0 {
		return fmt.Sprintf("No file conflicts among workers on %s.\n", r.Feature)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Conflicts on %s:\n", r.Feature)
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
