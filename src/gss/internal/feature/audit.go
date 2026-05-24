package feature

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"os"
	"strings"

	"github.com/wenlock/dotfiles/gss/internal/errors"
	"github.com/wenlock/dotfiles/gss/internal/gh"
	"github.com/wenlock/dotfiles/gss/internal/git"
	"github.com/wenlock/dotfiles/gss/internal/registry"
)

// Audit finding severities (design.md → "gss feature audit").
const (
	SevInfo  = "info"
	SevWarn  = "warn"
	SevError = "error"
)

// Finding is one audit observation about a worker (Worker == "" for
// registry-level findings).
type Finding struct {
	Worker   string `json:"worker,omitempty"`
	Check    string `json:"check"`
	Severity string `json:"severity"`
	Detail   string `json:"detail,omitempty"`
	Remedy   string `json:"remedy,omitempty"`
	Repaired bool   `json:"repaired,omitempty"`

	fix string // internal: target base branch for a registry-local repair
}

// Observer supplies the read-only observations the audit needs. It exposes
// ONLY non-mutating queries — the audit literally cannot create/edit/ready a
// PR or rewrite git refs, which is what makes read-only mode provably
// side-effect-free (design.md → "audit does not call gh pr create/edit/ready
// ever").
type Observer interface {
	WorktreeExists(path string) bool
	BranchExists(ctx context.Context, branch string) bool
	BaseReachable(ctx context.Context, branch, base string) bool
	PRView(ctx context.Context, num int) (gh.PR, bool) // ok == false on 404
}

// AuditOpts configures Audit.
type AuditOpts struct {
	Feature  string // restrict to one feature (empty = all)
	Repair   bool
	RepoPath string // git -C target for the system observer (unused when Service.Observe is set)
}

// AuditReport is the structured result of an audit run.
type AuditReport struct {
	Findings []Finding
	Repaired int
}

// HasErrors reports whether any finding is error-severity (drives a non-zero
// exit code).
func (r AuditReport) HasErrors() bool {
	for _, f := range r.Findings {
		if f.Severity == SevError {
			return true
		}
	}
	return false
}

// Audit walks the registry and surfaces drift between recorded state and
// observed reality (design.md → "gss feature audit"; resolution #20). Default
// mode is read-only. With opts.Repair, it applies the deterministic,
// registry-local fixes (drop dead rows, clear 404 PRs, adopt the authoritative
// PR base, reset stale base refs) under the store lock — it NEVER force-pushes,
// renames a branch, or calls a mutating gh verb. A registry whose schema is
// newer than this build supports is surfaced as an error-severity finding and
// never repaired (Store.Load rejects it at parse time).
func (s *Service) Audit(ctx context.Context, opts AuditOpts) (AuditReport, error) {
	reg, err := s.Store.Load()
	if err != nil {
		if stderrors.Is(err, errors.ErrSchemaMismatch) {
			return AuditReport{Findings: []Finding{{
				Check: "schema-newer", Severity: SevError, Detail: err.Error(),
				Remedy: "upgrade gss; --repair cannot touch a newer schema",
			}}}, nil
		}
		return AuditReport{}, err
	}

	obs := s.Observe
	if obs == nil {
		obs = &systemObserver{git: s.Git, gh: s.GH, repoPath: opts.RepoPath}
	}
	findings := runAudit(ctx, reg, obs, opts.Feature)
	rep := AuditReport{Findings: findings}

	if opts.Repair {
		if err := s.Store.Update(func(r *registry.Registry) error {
			rep.Repaired = applyRepairs(r, rep.Findings)
			return nil
		}); err != nil {
			return rep, err
		}
	}
	return rep, nil
}

// runAudit is the pure check matrix over a loaded registry + an Observer. It
// performs no I/O of its own; every observation goes through obs.
func runAudit(ctx context.Context, reg registry.Registry, obs Observer, only string) []Finding {
	var fs []Finding

	// Registry-wide branch claims, used for the cross-machine duplicate-branch
	// check (roadmap.md → cross-machine sync failure mode 3): two hosts running
	// `worker add` against the same NWO + name collide on one branch.
	branchClaims := map[string]int{}
	for _, f := range reg.Features {
		if only != "" && f.Name != only {
			continue
		}
		for _, w := range f.Workers {
			branchClaims[w.Branch]++
		}
	}

	for _, f := range reg.Features {
		if only != "" && f.Name != only {
			continue
		}
		owned := map[string]bool{} // branch -> some worker owns it
		for _, w := range f.Workers {
			owned[w.Branch] = true
		}
		for _, w := range f.Workers {
			ref := workerRef(f.Name, w)

			if w.Worktree != "" && !obs.WorktreeExists(w.Worktree) {
				fs = append(fs, Finding{Worker: ref, Check: "worktree-missing", Severity: SevError,
					Detail: w.Worktree, Remedy: "audit --repair drops the row (or restore the path)"})
				continue // a missing worktree makes the git probes meaningless
			}
			if !obs.BranchExists(ctx, w.Branch) {
				fs = append(fs, Finding{Worker: ref, Check: "branch-missing", Severity: SevError,
					Detail: w.Branch, Remedy: "audit --repair drops the row"})
				continue
			}
			if branchClaims[w.Branch] > 1 {
				fs = append(fs, Finding{Worker: ref, Check: "duplicate-branch", Severity: SevError,
					Detail: fmt.Sprintf("branch %q is claimed by %d registry rows", w.Branch, branchClaims[w.Branch]),
					Remedy: "pick one to keep, then gss feature done --force --worker <ref> on the loser (repair won't choose)"})
			}
			if !obs.BaseReachable(ctx, w.Branch, w.BaseBranch) {
				fs = append(fs, Finding{Worker: ref, Check: "base-unreachable", Severity: SevError,
					Detail: fmt.Sprintf("%s cannot reach base %s", w.Branch, w.BaseBranch),
					Remedy: "gss feature restack (repair never rewrites git refs)"})
			}
			if w.BaseBranch != f.DefaultBaseBranch && !owned[w.BaseBranch] {
				fs = append(fs, Finding{Worker: ref, Check: "stale-base", Severity: SevWarn,
					Detail: fmt.Sprintf("base %q is not the default and no worker owns it", w.BaseBranch),
					Remedy: "audit --repair resets base to " + f.DefaultBaseBranch, fix: f.DefaultBaseBranch})
			}
			if w.PRURL != "" {
				num := prNumber(w.PRURL)
				pr, ok := obs.PRView(ctx, num)
				switch {
				case !ok:
					fs = append(fs, Finding{Worker: ref, Check: "pr-404", Severity: SevWarn,
						Detail: fmt.Sprintf("PR #%d (%s) not found", num, w.PRURL),
						Remedy: "audit --repair clears pr_url/pr_state"})
				default:
					if pr.Base != "" && pr.Base != w.BaseBranch {
						fs = append(fs, Finding{Worker: ref, Check: "pr-base-diverged", Severity: SevWarn,
							Detail: fmt.Sprintf("PR base %q != registry base %q", pr.Base, w.BaseBranch),
							Remedy: "audit --repair adopts the PR base (the PR is authoritative)", fix: pr.Base})
					}
					// pr-state reconcile (roadmap.md → cross-machine sync failure
					// mode 5): another host already flipped the PR; the registry's
					// pr_state is stale. Benign — the ready-flip is idempotent — so
					// it is info-severity and repair just adopts the observed state.
					if obsd := observedPRState(pr); obsd != "" && w.PRState != "" && obsd != w.PRState {
						fs = append(fs, Finding{Worker: ref, Check: "pr-state-stale", Severity: SevInfo,
							Detail: fmt.Sprintf("registry pr_state %q but observed %q", w.PRState, obsd),
							Remedy: "audit --repair reconciles pr_state to the observed value", fix: obsd})
					}
				}
			}
			// NOTE: spawned_by is intentionally NOT validated here. Resolution
			// #8 makes it informational-only — never the basis for a control
			// decision — and TestSpawnedByInformationalOnly forbids any code in
			// internal/ from branching on spawned_by.engine.
		}
	}
	return fs
}

// observedPRState maps a gh.PR to the registry pr_state vocabulary: a draft is
// "draft"; otherwise the lower-cased State (open/closed/merged).
func observedPRState(pr gh.PR) string {
	if pr.IsDraft {
		return "draft"
	}
	return strings.ToLower(pr.State)
}

// applyRepairs performs the deterministic registry-local fixes for repairable
// findings, marking each Finding.Repaired and returning the count applied. It
// mutates r in place and is only ever called inside Store.Update.
func applyRepairs(r *registry.Registry, findings []Finding) int {
	n := 0
	for i := range findings {
		f := &findings[i]
		var ok bool
		switch f.Check {
		case "worktree-missing", "branch-missing":
			ok = removeWorkerByRef(r, f.Worker)
		case "pr-404":
			ok = mutateWorker(r, f.Worker, func(w *registry.Worker) { w.PRURL = ""; w.PRState = "" })
		case "pr-base-diverged", "stale-base":
			target := f.fix
			ok = mutateWorker(r, f.Worker, func(w *registry.Worker) { w.BaseBranch = target })
		case "pr-state-stale":
			target := f.fix
			ok = mutateWorker(r, f.Worker, func(w *registry.Worker) { w.PRState = target })
		}
		// duplicate-branch and base-unreachable are intentionally absent: both
		// need human judgement (which row to keep / a git rewrite), so repair
		// reports them but never acts.
		if ok {
			f.Repaired = true
			n++
		}
	}
	return n
}

func mutateWorker(r *registry.Registry, ref string, fn func(*registry.Worker)) bool {
	for fi := range r.Features {
		for wi := range r.Features[fi].Workers {
			if workerRef(r.Features[fi].Name, r.Features[fi].Workers[wi]) == ref {
				fn(&r.Features[fi].Workers[wi])
				return true
			}
		}
	}
	return false
}

func removeWorkerByRef(r *registry.Registry, ref string) bool {
	for fi := range r.Features {
		ws := r.Features[fi].Workers
		for wi := range ws {
			if workerRef(r.Features[fi].Name, ws[wi]) == ref {
				r.Features[fi].Workers = append(ws[:wi], ws[wi+1:]...)
				return true
			}
		}
	}
	return false
}

// Text renders the report as a human-readable list.
func (r AuditReport) Text() string {
	if len(r.Findings) == 0 {
		return "audit: no findings; registry is consistent with observed state\n"
	}
	var b strings.Builder
	for _, f := range r.Findings {
		ref := f.Worker
		if ref == "" {
			ref = "(registry)"
		}
		mark := ""
		if f.Repaired {
			mark = " [repaired]"
		}
		fmt.Fprintf(&b, "%-5s %s  %s: %s%s\n", strings.ToUpper(f.Severity), ref, f.Check, f.Detail, mark)
		if f.Remedy != "" && !f.Repaired {
			fmt.Fprintf(&b, "      remedy: %s\n", f.Remedy)
		}
	}
	if r.Repaired > 0 {
		fmt.Fprintf(&b, "\nrepaired %d finding(s)\n", r.Repaired)
	}
	return b.String()
}

// JSON renders the report for machine consumers (--json).
func (r AuditReport) JSON() ([]byte, error) {
	return json.MarshalIndent(struct {
		Findings  []Finding `json:"findings"`
		Repaired  int       `json:"repaired"`
		HasErrors bool      `json:"has_errors"`
	}{Findings: r.Findings, Repaired: r.Repaired, HasErrors: r.HasErrors()}, "", "  ")
}

// systemObserver is the production Observer: os.Stat for worktrees, git for
// branch/base reachability, gh for PR existence. Strictly read-only.
type systemObserver struct {
	git      git.Runner
	gh       gh.Client
	repoPath string
}

func (o *systemObserver) WorktreeExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

func (o *systemObserver) BranchExists(ctx context.Context, branch string) bool {
	_, err := o.git.Run(ctx, "-C", o.repoPath, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

func (o *systemObserver) BaseReachable(ctx context.Context, branch, base string) bool {
	if base == "" {
		return true
	}
	_, err := o.git.Run(ctx, "-C", o.repoPath, "merge-base", branch, base)
	return err == nil
}

func (o *systemObserver) PRView(ctx context.Context, num int) (gh.PR, bool) {
	if num <= 0 {
		return gh.PR{}, false
	}
	pr, err := o.gh.PRView(ctx, num)
	return pr, err == nil
}
