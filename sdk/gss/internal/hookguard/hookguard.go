// Package hookguard keeps a repo-local core.hooksPath from silently switching
// off the global privacy git hooks.
//
// install.sh copies the privacy hooks (pre-commit, commit-msg, pre-push) into
// ~/.config/git/hooks and points the GLOBAL core.hooksPath there. Git lets a
// repo's own config win over the global value, so a checkout that sets
// core.hooksPath locally runs none of those hooks, and nothing says so. That
// is how a whole feature was once pushed with the guard off.
//
// Inspect reads the value git will actually use for a repo and sorts it into
// one of three kinds:
//
//   - OK: no override (unset, no global value to override, or the same path
//     as the global value).
//   - Redundant: the repo points at its own hooks dir (<git common dir>/hooks
//     or <git dir>/hooks). The global hooks already run that dir's hook of the
//     same name after their check, so unsetting the value loses nothing.
//   - Foreign: anything else (.husky/_, /dev/null, another tool's dir). Those
//     hooks matter to someone, so gss never removes them; the owner chains
//     them instead.
//
// Check turns a non-OK result into a refusal; Runner and Approver apply Check
// in front of every push gss makes. Fix unsets only the Redundant kind, and
// only in the repo's own config (never global config, never a hook file).
package hookguard

import (
	"context"
	stderrors "errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git"
)

// Key is the git config key this package guards.
const Key = "core.hooksPath"

// Kind classifies a repo's effective core.hooksPath.
type Kind int

const (
	// OK means the global hooks run for this repo.
	OK Kind = iota
	// Redundant means the repo points at its own hooks dir; safe to unset.
	Redundant
	// Foreign means the repo points somewhere else; a person must chain it.
	Foreign
)

func (k Kind) String() string {
	switch k {
	case Redundant:
		return "redundant"
	case Foreign:
		return "foreign"
	default:
		return "ok"
	}
}

// ErrOverride is wrapped by every refusal Check returns, so callers can test
// for it with errors.Is.
var ErrOverride = stderrors.New("a repo-local core.hooksPath switches off the global privacy git hooks")

// errNotRepo marks an Inspect of a directory git does not see as a repo.
var errNotRepo = stderrors.New("not a git repository")

// Result is what Inspect found for one repo.
type Result struct {
	// Repo is the directory Inspect was given.
	Repo string
	// Value is the core.hooksPath git uses for Repo, as written in config
	// ("" when unset).
	Value string
	// Scope is where Value comes from: local, worktree, global, system or
	// command (git config --show-scope).
	Scope string
	// Global is the global core.hooksPath ("" when unset).
	Global string
	// HooksDir is the repo's own hooks dir (<git common dir>/hooks).
	HooksDir string
	Kind     Kind
	// Values are every core.hooksPath set in Scope, in the order git reads
	// them; Value is the last, the one git uses.
	Values []string
	// ownOnly: every one of Values points at the repo's own hooks dir.
	ownOnly bool
}

// Fixable reports whether Fix can remove the override: the Redundant kind,
// set in this repo's own config, with no other value in that scope that
// unsetting would also remove.
func (r Result) Fixable() bool {
	return r.Kind == Redundant && r.ownOnly && (r.Scope == "local" || r.Scope == "worktree")
}

// Inspect reads the effective and global core.hooksPath for the repo at dir
// and classifies them. `git config` honours worktree config and includes, so
// the value is the one git itself will use when a hook runs.
func Inspect(ctx context.Context, r git.Runner, dir string) (Result, error) {
	res := Result{Repo: dir}

	common, err := revParsePath(ctx, r, dir, "--git-common-dir")
	if err != nil {
		return res, fmt.Errorf("hookguard: %s: %w: %v", dir, errNotRepo, err)
	}
	gitDir, _ := revParsePath(ctx, r, dir, "--git-dir")
	top, _ := revParsePath(ctx, r, dir, "--show-toplevel")
	res.HooksDir = filepath.Join(common, "hooks")

	// Every value in every scope, last one wins. git exits 1 when the key is
	// unset; any other failure means the config could not be read, and the
	// caller must not take that for "no override".
	all, err := configGet(ctx, r, dir, "--show-scope", "--get-all")
	if err != nil {
		return res, err
	}
	type entry struct{ scope, val string }
	var entries []entry
	// Lines are "<scope>\t<value>"; the value may be empty (an explicit
	// core.hooksPath = "" still overrides the global one), so only the line end
	// is trimmed.
	for _, line := range strings.Split(all, "\n") {
		if line = strings.TrimRight(line, "\r"); line == "" {
			continue
		}
		scope, val, ok := strings.Cut(line, "\t")
		if !ok {
			scope, val = "", line
		}
		entries = append(entries, entry{scope, val})
	}
	if n := len(entries); n > 0 {
		res.Scope, res.Value = entries[n-1].scope, entries[n-1].val
		for _, e := range entries {
			if e.scope == res.Scope {
				res.Values = append(res.Values, e.val)
			}
		}
	}
	// The global value is the last global line (a second global file or an
	// include may set it twice; git uses the last).
	for _, e := range entries {
		if e.scope == "global" {
			res.Global = e.val
		}
	}

	// No global value means the privacy hooks are not installed globally,
	// so there is nothing for a local value to switch off.
	if len(entries) == 0 || res.Global == "" {
		return res, nil
	}
	base := top
	if base == "" {
		base = gitDir // bare repo: hooks run with the git dir as cwd
	}
	eff := resolve(res.Value, base)
	if eff == resolve(res.Global, base) {
		return res, nil
	}
	own := []string{res.HooksDir}
	if gitDir != "" {
		own = append(own, filepath.Join(gitDir, "hooks"))
	}
	isOwn := func(p string) bool {
		for _, h := range own {
			if p == canonical(h) {
				return true
			}
		}
		return false
	}
	res.Kind = Foreign
	if isOwn(eff) {
		res.Kind = Redundant
	}
	res.ownOnly = true
	for _, v := range res.Values {
		if !isOwn(resolve(v, base)) {
			res.ownOnly = false
		}
	}
	return res, nil
}

// configGet reads Key with the given git config flags. Unset (git exits 1)
// is "" and no error; any other failure is an error.
func configGet(ctx context.Context, r git.Runner, dir string, flags ...string) (string, error) {
	args := append(append([]string{"-C", dir, "config"}, flags...), Key)
	out, err := r.Run(ctx, args[0], args[1:]...)
	if err != nil {
		var ee *exec.ExitError
		if stderrors.As(err, &ee) && ee.ExitCode() == 1 {
			return "", nil
		}
		return "", fmt.Errorf("hookguard: read %s in %s: %w: %s", Key, dir, err, strings.TrimSpace(string(out)))
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}

// Check returns nil when the global hooks will run for the repo at dir, and
// a refusal wrapping ErrOverride when a repo-level core.hooksPath replaces
// them. The message names the value, says the privacy guard will not run,
// and gives the fix.
func Check(ctx context.Context, r git.Runner, dir string) error {
	res, err := Inspect(ctx, r, dir)
	if stderrors.Is(err, errNotRepo) {
		// Not a repo (or git missing): the push itself will fail with git's
		// own, clearer error. The guard has nothing to judge.
		return nil
	}
	if err != nil {
		// The config could not be read, so whether the privacy hooks run is
		// unknown: refuse rather than guess.
		return fmt.Errorf("gss refuses to push %s: cannot tell whether the privacy hooks run: %w", dir, err)
	}
	if res.Kind == OK {
		return nil
	}
	return fmt.Errorf("%w\n%s", ErrOverride, Explain(res))
}

// Explain renders the refusal text for a non-OK result.
func Explain(res Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "gss refuses to push %s:\n", res.Repo)
	fmt.Fprintf(&b, "  core.hooksPath = %q (%s config), but the global value is %q.\n", res.Value, scopeName(res.Scope), res.Global)
	b.WriteString("  Git uses the repo's value, so the privacy guard (pre-commit, commit-msg, pre-push) will not run here.\n")
	b.WriteString("  Fix: ")
	b.WriteString(Advice(res))
	return b.String()
}

// Advice is the one-line fix for a non-OK result.
func Advice(res Result) string {
	switch {
	case res.Kind == Redundant && !res.ownOnly && len(res.Values) > 1:
		return fmt.Sprintf("%s config holds %d core.hooksPath values (%s) and git uses the last; gss will not remove the others with it. Edit them by hand: git -C %s config --%s --edit", scopeName(res.Scope), len(res.Values), strings.Join(res.Values, ", "), res.Repo, unsetScope(res.Scope))
	case res.Fixable():
		return fmt.Sprintf("gss hooks check --fix %s  (the value points at the repo's own hooks dir, which the global hooks already run after their check, so removing it loses nothing)", res.Repo)
	case res.Kind == Redundant:
		return fmt.Sprintf("the value points at the repo's own hooks dir but comes from %s config; remove it there (gss only edits a repo's own config)", scopeName(res.Scope))
	default:
		return fmt.Sprintf("make %s/<hook> run the hooks in %q (the global hooks exec that file after the privacy check), then: git -C %s config --%s --unset core.hooksPath",
			res.HooksDir, res.Value, res.Repo, unsetScope(res.Scope))
	}
}

// Fix unsets a Fixable override in the repo's own config and re-inspects.
// Anything else is returned unchanged: Fix never touches global config, a
// foreign value, or a hook file.
func Fix(ctx context.Context, r git.Runner, res Result) (Result, error) {
	if !res.Fixable() {
		return res, nil
	}
	if out, err := r.Run(ctx, "-C", res.Repo, "config", "--"+res.Scope, "--unset-all", Key); err != nil {
		return res, fmt.Errorf("hookguard: unset %s in %s: %w: %s", Key, res.Repo, err, strings.TrimSpace(string(out)))
	}
	return Inspect(ctx, r, res.Repo)
}

func scopeName(scope string) string {
	if scope == "" {
		return "repo"
	}
	return scope
}

func unsetScope(scope string) string {
	if scope == "worktree" {
		return "worktree"
	}
	return "local"
}

// revParsePath runs `git rev-parse --path-format=absolute <flag>` in dir.
func revParsePath(ctx context.Context, r git.Runner, dir, flag string) (string, error) {
	out, err := r.Run(ctx, "-C", dir, "rev-parse", "--path-format=absolute", flag)
	if err != nil {
		return "", err
	}
	p := strings.TrimSpace(string(out))
	if p == "" {
		return "", fmt.Errorf("empty %s", flag)
	}
	return p, nil
}

// resolve turns a core.hooksPath value into a canonical absolute path the way
// git reads it: a leading ~ is the home dir, a relative path is relative to
// the directory hooks run in (the worktree top, or the git dir when bare).
func resolve(val, base string) string {
	if val == "~" || strings.HasPrefix(val, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			val = filepath.Join(home, strings.TrimPrefix(val, "~"))
		}
	}
	if !filepath.IsAbs(val) && base != "" {
		val = filepath.Join(base, val)
	}
	return canonical(val)
}

// canonical cleans p and resolves symlinks when the path exists.
func canonical(p string) string {
	p = filepath.Clean(p)
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	return p
}
