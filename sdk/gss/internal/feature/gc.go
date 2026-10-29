package feature

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/identity"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/registry"
)

// GCOpts configures `feature gc`.
type GCOpts struct {
	Feature string // restrict to one feature (empty = all of this repo's)
	Apply   bool   // remove the candidates (default: dry run)
}

// GCCandidate is a worker gc would remove (dry run) or did remove (--apply).
type GCCandidate struct {
	Worker  string `json:"worker"`
	PR      int    `json:"pr"`
	PRState string `json:"pr_state"`
	Bytes   int64  `json:"bytes"`
	Action  string `json:"action"`
}

// GCSkip is a worker gc left alone, with the reason.
type GCSkip struct {
	Worker string `json:"worker"`
	Reason string `json:"reason"`
}

// GCResult is the outcome of a gc run. Removed is always non-nil so the JSON
// form prints `[]`, not `null`.
type GCResult struct {
	Candidates []GCCandidate `json:"candidates"`
	Removed    []string      `json:"removed"`
	Skipped    []GCSkip      `json:"skipped"`
	Failed     []GCSkip      `json:"failed"`
	FreedBytes int64         `json:"freed_bytes"`
}

// JSON renders the machine-readable result.
func (r GCResult) JSON() ([]byte, error) { return json.MarshalIndent(r, "", "  ") }

// GC finds workers of THIS repo whose PR is merged or closed, whose worktree
// is clean and fully pushed, and that no other worker bases on (dotfiles#438).
// It is a dry run unless opts.Apply, and then removes through Done without
// force, so every guard Done has still applies.
//
// The registry is shared across repos, so a row is in scope only when its
// worktree lives under <WorktreeRoot>/<NWO>/. Rows of any other repo are not
// examined, not reported, and never written: asking this repo's gh client
// about another repo's PR number would answer for the wrong PR.
func (s *Service) GC(ctx context.Context, opts GCOpts) (GCResult, error) {
	res := GCResult{Candidates: []GCCandidate{}, Removed: []string{}, Skipped: []GCSkip{}, Failed: []GCSkip{}}
	reg, err := s.Store.Load()
	if err != nil {
		return res, err
	}
	if opts.Feature != "" {
		found := false
		for _, f := range reg.Features {
			found = found || f.Name == opts.Feature
		}
		if !found {
			return res, fmt.Errorf("feature gc: no such feature %q", opts.Feature)
		}
	}

	for _, feat := range reg.Features {
		if opts.Feature != "" && feat.Name != opts.Feature {
			continue
		}
		for _, w := range feat.Workers {
			if !s.inThisRepo(w) {
				continue
			}
			ref := workerRef(feat.Name, w)
			cand, reason := s.gcCheck(ctx, feat, w, opts.Apply)
			if reason != "" {
				res.Skipped = append(res.Skipped, GCSkip{Worker: ref, Reason: reason})
				continue
			}
			res.Candidates = append(res.Candidates, cand)
		}
	}

	if !opts.Apply {
		return res, nil
	}
	for _, c := range res.Candidates {
		if _, err := s.Done(ctx, DoneOpts{WorkerRef: c.Worker}); err != nil {
			res.Failed = append(res.Failed, GCSkip{Worker: c.Worker, Reason: err.Error()})
			continue
		}
		res.Removed = append(res.Removed, c.Worker)
		res.FreedBytes += c.Bytes
	}
	return res, nil
}

// inThisRepo reports whether a worker row belongs to the repo this Service is
// bound to, judged by its worktree path.
func (s *Service) inThisRepo(w registry.Worker) bool {
	if s.NWO == "" {
		return true
	}
	root := filepath.Join(s.WorktreeRoot, s.NWO) + string(filepath.Separator)
	return strings.HasPrefix(w.Worktree, root)
}

// gcCheck decides one in-scope worker. A non-empty reason means skip. The
// refreshed PR state is written to the registry only when persist is set, so
// a dry run leaves the shared registry byte-identical.
func (s *Service) gcCheck(ctx context.Context, feat registry.Feature, w registry.Worker, persist bool) (GCCandidate, string) {
	ref := workerRef(feat.Name, w)
	pr, state, err := s.observePR(ctx, w)
	if err == errNoPR {
		return GCCandidate{}, errNoPR.Error()
	}
	if err != nil {
		return GCCandidate{}, fmt.Sprintf("cannot refresh PR state from GitHub: %v", err)
	}
	if parsed, perr := identity.ParseWorkerRef(ref); persist && perr == nil {
		s.persistPRState(parsed, pr, state)
	}
	num := pr.Number
	if state != "merged" && state != "closed" {
		return GCCandidate{}, fmt.Sprintf("PR #%d is %s", num, state)
	}
	for _, other := range feat.Workers {
		if other.Branch != w.Branch && other.BaseBranch == w.Branch {
			return GCCandidate{}, fmt.Sprintf("%s still bases on it", workerRef(feat.Name, other))
		}
	}
	out, err := s.gitOut(ctx, w.Worktree, "status", "--porcelain")
	if err != nil {
		return GCCandidate{}, fmt.Sprintf("cannot inspect worktree: %v", err)
	}
	if out != "" {
		return GCCandidate{}, "worktree has uncommitted changes"
	}
	n, err := s.unpushedCommits(ctx, w.Worktree, pr.HeadSHA)
	if err != nil {
		return GCCandidate{}, fmt.Sprintf("cannot check for unpushed commits: %v", err)
	}
	if n > 0 {
		return GCCandidate{}, fmt.Sprintf("%d commit(s) not pushed", n)
	}
	action := "gss feature done " + ref
	if state == "merged" {
		action += " (and delete local branch)"
	}
	return GCCandidate{Worker: ref, PR: num, PRState: state, Bytes: dirSize(w.Worktree), Action: action}, ""
}

// unpushedCommits counts commits on HEAD the remote lacks. A HEAD equal to the
// PR's head SHA is pushed by definition (this holds after GitHub deleted the
// branch and the upstream is gone). Otherwise it counts @{u}..HEAD; with
// neither available it errors, so the caller skips the worker.
func (s *Service) unpushedCommits(ctx context.Context, worktree, prHead string) (int, error) {
	head, err := s.gitOut(ctx, worktree, "rev-parse", "HEAD")
	if err != nil {
		return 0, err
	}
	if prHead != "" && head == prHead {
		return 0, nil
	}
	out, err := s.gitOut(ctx, worktree, "rev-list", "--count", "@{u}..HEAD")
	if err != nil {
		return 0, fmt.Errorf("no upstream and HEAD is not the PR head")
	}
	return strconv.Atoi(strings.TrimSpace(out))
}

// dirSize sums regular-file sizes under dir (symlinks not followed).
func dirSize(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, ierr := d.Info(); ierr == nil && info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total
}

// HumanBytes renders a byte count for the text report.
func HumanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// Text renders the human report: one line per candidate (or removal), one per
// skipped worker, and a footer.
func (r GCResult) Text(apply bool) string {
	var b strings.Builder
	removed := map[string]bool{}
	for _, w := range r.Removed {
		removed[w] = true
	}
	failed := map[string]string{}
	for _, f := range r.Failed {
		failed[f.Worker] = f.Reason
	}
	for _, c := range r.Candidates {
		verb := "would remove"
		switch {
		case removed[c.Worker]:
			verb = "removed"
		case failed[c.Worker] != "":
			verb = "FAILED"
		}
		fmt.Fprintf(&b, "%-12s %s  PR #%d %s  %s  %s\n", verb, c.Worker, c.PR, c.PRState, HumanBytes(c.Bytes), c.Action)
		if reason := failed[c.Worker]; reason != "" {
			fmt.Fprintf(&b, "             %s\n", reason)
		}
	}
	for _, s := range r.Skipped {
		fmt.Fprintf(&b, "%-12s %s  %s\n", "skip", s.Worker, s.Reason)
	}
	if apply {
		fmt.Fprintf(&b, "removed %d worker(s), freed %s, skipped %d, failed %d\n", len(r.Removed), HumanBytes(r.FreedBytes), len(r.Skipped), len(r.Failed))
	} else {
		var total int64
		for _, c := range r.Candidates {
			total += c.Bytes
		}
		fmt.Fprintf(&b, "%d worker(s) would be removed (%s), %d skipped; dry run, pass --apply to remove\n", len(r.Candidates), HumanBytes(total), len(r.Skipped))
	}
	return b.String()
}
