// Package updexec builds the remote shell one-liners the executor sends over
// runner.Runner and walks a parsed updplan.Plan per host. Every builder here
// is pure — it takes already-validated updplan values and a few explicit
// parameters, and returns either a string that is safe to hand to a remote
// shell or an error. Nothing here touches the network, a clock, or a
// filesystem; that is exec.go's and the runner's job.
//
// Every builder re-validates its inputs (never trusting that a caller went
// through updplan.Parse) because a hand-built updplan.Repo/Step bypassing
// Parse must never be able to smuggle a shell metacharacter onto the wire.
package updexec

import (
	"fmt"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// ShQuote makes s safe as a single POSIX shell word: it single-quotes s and
// escapes any embedded single quote using the standard POSIX close-backslash-
// quote-reopen idiom. Copied from cmd/tui_cmds.go so updexec (and cmd, via a
// thin alias) share one definition; cmd's copy retires in leaf D/E.
func ShQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// validRepo re-runs the updplan validation checks a Repo must satisfy before
// any of its fields are interpolated into a remote command.
func validRepo(r updplan.Repo) error {
	if !updplan.ValidPath(r.Path) {
		return fmt.Errorf("updexec: repo %q: invalid path %q", r.Name, r.Path)
	}
	if r.Name != "" && !updplan.ValidRepoName(r.Name) {
		return fmt.Errorf("updexec: invalid repo name %q", r.Name)
	}
	return nil
}

func validBranches(r updplan.Repo) error {
	if len(r.Branches) == 0 {
		return fmt.Errorf("updexec: repo %q: no branches", r.Name)
	}
	for i, b := range r.Branches {
		if b == "default" {
			if i != 0 {
				return fmt.Errorf("updexec: repo %q: %q must be first", r.Name, "default")
			}
			continue
		}
		if !updplan.ValidRef(b) {
			return fmt.Errorf("updexec: repo %q: invalid branch %q", r.Name, b)
		}
	}
	return nil
}

// extrasScript builds the EXTRAS loop that fast-forwards (or creates) every
// branch after the first, skipping any that has diverged from its remote
// counterpart rather than clobbering it.
//
// The loop tracks a "fail" flag rather than relying on its own exit status:
// POSIX's `for` reports only its LAST iteration's status, so a failing
// non-last extra (e.g. `git branch --track` against a missing remote
// branch) would otherwise be silently swallowed by a later, successful
// iteration. `skipped(diverged)` is deliberately NOT a failure — it echoes
// and continues.
func extrasScript(extras []string) string {
	if len(extras) == 0 {
		return ""
	}
	return `fail=0; for b in ` + strings.Join(extras, " ") + `; do [ "$b" = "$b1" ] && continue; ` +
		`if git show-ref -q --verify "refs/heads/$b"; then ` +
		`if git merge-base --is-ancestor "$b" "origin/$b"; then git branch -q -f "$b" "origin/$b" && echo "fleet: ff $b" || fail=1; ` +
		`else echo "fleet: skipped(diverged) $b"; fi; ` +
		`else git branch -q --track "$b" "origin/$b" && echo "fleet: created $b" || fail=1; fi; done; [ "$fail" = 0 ]`
}

// syncBody builds the BODY portion of a sync script (no prologue/epilogue),
// for the single-branch, multi-branch, and default-branch forms.
func syncBody(r updplan.Repo, reset bool) string {
	branches := r.Branches
	if branches[0] == "default" {
		extras := extrasScript(branches[1:])
		move := `git merge --ff-only "origin/$b1"`
		if reset {
			move = ResetScript("$b1", `"origin/$b1"`)
		}
		// The fetch MUST stay in the `&&` chain: a `;` here would let a
		// failed fetch fall through into resolving b1 (via a symref/
		// ls-remote fallback) and checking out/merging against stale refs,
		// exiting 0 despite the transport failure never having succeeded.
		body := `git fetch origin && { ` +
			`b1=$(git symbolic-ref -q --short refs/remotes/origin/HEAD); b1=${b1#origin/}; ` +
			`[ -n "$b1" ] || { b1=$(git ls-remote --symref origin HEAD | sed -n 's|^ref: refs/heads/\(.*\)[[:space:]]HEAD$|\1|p') && [ -n "$b1" ] && git remote set-head origin "$b1"; }; ` +
			`[ -n "$b1" ] || { echo 'fleet: cannot resolve the default branch' >&2; exit 3; }; ` +
			`} && git checkout -q "$b1" && ` + move
		if extras != "" {
			body += ` && ` + extras
		}
		return body
	}

	if len(branches) == 1 {
		b1 := branches[0]
		move := "git merge --ff-only FETCH_HEAD"
		if reset {
			move = ResetScript(b1, "FETCH_HEAD")
		}
		return `git fetch origin ` + b1 + ` && git checkout ` + b1 + ` && ` + move
	}

	b1 := branches[0]
	extras := extrasScript(branches[1:])
	move := `git merge --ff-only "origin/$b1"`
	if reset {
		move = ResetScript("$b1", `"origin/$b1"`)
	}
	body := `git fetch origin ` + strings.Join(branches, " ") + ` && b1=` + b1 +
		` && git checkout -q "$b1" && ` + move
	if extras != "" {
		body += ` && ` + extras
	}
	return body
}

// SyncScript builds the full remote sync command: PROLOGUE (record orig,
// optionally stash under local=carry) + BODY (single/multi/default branch
// form) + EPILOGUE (echo when the branch moved; propagate the exit code).
func SyncScript(r updplan.Repo, local updplan.Local, reset bool) (string, error) {
	if err := validRepo(r); err != nil {
		return "", err
	}
	if err := validBranches(r); err != nil {
		return "", err
	}
	switch local {
	case updplan.LocalSkip, updplan.LocalRescue, updplan.LocalCarry:
	default:
		return "", fmt.Errorf("updexec: repo %q: invalid local policy %q", r.Name, local)
	}

	p := r.Path
	prologue := `cd ` + p + ` && orig=$(git symbolic-ref -q --short HEAD || git rev-parse HEAD) && echo "fleet: orig=$orig" && `
	if local == updplan.LocalCarry {
		prologue += `ts=$(date -u +%Y%m%dT%H%M%SZ) && ` +
			`{ [ -z "$(git status --porcelain)" ] || { git stash push -q -u -m "fleet-carry $ts" && echo "fleet: carried stash=$(git rev-parse stash@{0}) from=$orig"; }; } && `
	}

	body := syncBody(r, reset)

	epilogue := `; rc=$?; now=$(git symbolic-ref -q --short HEAD || git rev-parse HEAD); [ "$orig" = "$now" ] || echo "fleet: switched $orig -> $now"; exit $rc`

	return prologue + body + epilogue, nil
}

// ResetScript is today's resetToFetched text, generalised to take the ref to
// check out (checkoutRef) and the ref to hard-reset onto (resetTo): it
// commits everything currently on the clone (tracked AND untracked, via
// `git add -A`) onto a fleet-reset/<ts> branch before hard resetting to
// resetTo, so a reset can never be the thing that loses an operator's work.
//
// resetTo must NOT always be "FETCH_HEAD": that is only correct for the
// single-branch form, where `git fetch origin <b1>` makes FETCH_HEAD exactly
// origin/<b1>. The multi-branch and default forms fetch several refs (or
// none named at all), so FETCH_HEAD there is whatever ref the fetch last
// advertised — typically the ORIGINAL branch's upstream, not origin/$b1 —
// and callers for those forms must pass `"origin/$b1"` instead.
//
// Built by concatenation, never fmt.Sprintf: the shell's date format
// (%Y%m%dT%H%M%SZ) collides with printf verbs.
func ResetScript(checkoutRef, resetTo string) string {
	return `ts=$(date -u +%Y%m%dT%H%M%SZ) && ` +
		`git checkout -q -b "fleet-reset/$ts" && git add -A && ` +
		`{ git -c user.email=fleet@local -c user.name=fleet commit -q -m "fleet pre-reset $ts" || true; } && ` +
		`git checkout -q "` + checkoutRef + `" && git reset --hard ` + resetTo
}

// defaultGhHost is the gh CLI's own default hostname when a step does not
// name one.
const defaultGhHost = "github.com"

// rescueRoot is the fixed remote directory under which rescue worktrees are
// materialised, keyed by repo name.
const rescueRoot = "~/.local/state/fleet/rescue"

// PrecheckScript reports a repo's remote state without changing anything:
// "state=missing" when the clone does not exist, else
// "state=<clean|dirty|in-progress> branch=<name|detached>". Uses -e rather
// than -d so a WORKTREE clone — whose .git is a file, not a directory — is
// still detected.
func PrecheckScript(r updplan.Repo) (string, error) {
	if err := validRepo(r); err != nil {
		return "", err
	}
	p := r.Path
	return `if [ ! -e ` + p + `/.git ]; then echo "state=missing"; else cd ` + p +
		` && g=$(git rev-parse --git-dir) && if [ -e "$g/MERGE_HEAD" ] || [ -d "$g/rebase-merge" ] || [ -d "$g/rebase-apply" ]; then s=in-progress; elif [ -n "$(git status --porcelain)" ]; then s=dirty; else s=clean; fi; b=$(git symbolic-ref -q --short HEAD || echo detached); echo "state=$s branch=$b"; fi`, nil
}

// CloneScript clones a missing repo — the one network call for a "missing"
// precheck. The explicit-branch form passes --branch; the "default" form
// omits it and reads back the branch git itself checked out.
func CloneScript(r updplan.Repo) (string, error) {
	if err := validRepo(r); err != nil {
		return "", err
	}
	if err := validBranches(r); err != nil {
		return "", err
	}
	if !updplan.ValidURL(r.URL) {
		return "", fmt.Errorf("updexec: repo %q: invalid url %q", r.Name, r.URL)
	}
	p := r.Path
	u := ShQuote(r.URL)

	branches := r.Branches
	var head string
	var extras []string
	if branches[0] == "default" {
		head = `mkdir -p "$(dirname ` + p + `)" && git clone -q ` + u + ` ` + p +
			` && cd ` + p + ` && b1=$(git symbolic-ref -q --short HEAD)`
		extras = branches[1:]
	} else {
		head = `mkdir -p "$(dirname ` + p + `)" && git clone -q --branch ` + branches[0] + ` ` + u + ` ` + p +
			` && cd ` + p + ` && b1=` + branches[0]
		extras = branches[1:]
	}

	e := extrasScript(extras)
	if e == "" {
		return head, nil
	}
	return head + ` && ` + e, nil
}

// RescueScript is today's rescueWorktree text, generalised to a repo's path
// and name: it commits everything currently on the clone (tracked AND
// untracked) onto a fleet-rescue/<ts> branch, returns to the original
// branch (now clean), and materialises the rescue branch as its own
// worktree under ~/.local/state/fleet/rescue/<name>/<ts> for the operator to
// inspect. Nothing is ever discarded.
//
// orig uses `git symbolic-ref -q --short HEAD`, falling back to the SHA,
// rather than `git rev-parse --abbrev-ref HEAD`: on a detached checkout the
// latter prints the literal string "HEAD", which makes the later
// `git checkout -q "$orig"` a no-op — the clone stays on the freshly
// created fleet-rescue/<ts> branch, and `git worktree add` for that SAME
// branch then fails, leaving the clone stranded there.
//
// The commit is wrapped `|| true`, mirroring ResetScript: submodule-only
// dirt is not staged by `git add -A`, so a clone whose only change is a
// dirty submodule pointer has nothing to commit — without `|| true` that
// aborts the `&&` chain in the same stranded state.
func RescueScript(r updplan.Repo) (string, error) {
	if err := validRepo(r); err != nil {
		return "", err
	}
	p, n := r.Path, r.Name
	dir := rescueRoot + "/" + n
	return `cd ` + p + ` && ts=$(date -u +%Y%m%dT%H%M%SZ) && ` +
		`orig=$(git symbolic-ref -q --short HEAD || git rev-parse HEAD) && ` +
		`git checkout -q -b "fleet-rescue/$ts" && git add -A && ` +
		`{ git -c user.email=fleet@local -c user.name=fleet commit -q -m "fleet rescue $ts" || true; } && ` +
		`git checkout -q "$orig" && ` +
		`mkdir -p ` + dir + ` && ` +
		`git worktree add ` + dir + `/$ts "fleet-rescue/$ts"`, nil
}

// RestoreScript checks out orig and, when sha is non-empty, applies and
// drops the carried stash — but only after a clean apply. On any failure
// (checkout or apply/drop) the stash is left in place and the exit is a
// distinguishable 4, so the caller can report "stash=<sha> branch=<orig>"
// without ever having lost the operator's carried work.
func RestoreScript(r updplan.Repo, orig, sha string) (string, error) {
	if err := validRepo(r); err != nil {
		return "", err
	}
	if !updplan.ValidRef(orig) && !updplan.ValidSHA(orig) {
		return "", fmt.Errorf("updexec: repo %q: invalid restore orig %q", r.Name, orig)
	}
	if sha != "" && !updplan.ValidSHA(sha) {
		return "", fmt.Errorf("updexec: repo %q: invalid restore sha %q", r.Name, sha)
	}
	p := r.Path
	return `cd ` + p + ` && git checkout -q ` + orig + ` && ` +
		`{ [ -z "` + sha + `" ] || { git stash apply -q ` + sha + ` && git stash drop -q ` + sha + ` && echo "fleet: restored stash=` + sha + `"; }; } || ` +
		`{ echo "fleet: restore-failed stash=` + sha + ` branch=` + orig + `" >&2; exit 4; }`, nil
}

// RunScript is a run step's script: `cd <path> && <run>` when the step
// targets a repo, or the run text verbatim when it does not. run is never
// re-quoted — it is trusted, operator-authored shell, run verbatim exactly
// as the operator wrote it.
func RunScript(st updplan.Step, r *updplan.Repo) (string, error) {
	if st.Run == "" {
		return "", fmt.Errorf("updexec: step %q: run is empty", st.ID)
	}
	if strings.ContainsAny(st.Run, "\x00\n") {
		return "", fmt.Errorf("updexec: step %q: run must not contain NUL or newline", st.ID)
	}
	if r == nil {
		return st.Run, nil
	}
	if err := validRepo(*r); err != nil {
		return "", err
	}
	return `cd ` + r.Path + ` && ` + st.Run, nil
}

// GhAuthCheck reports (via exit 127, or the gh auth status exit) whether gh
// is installed and already authenticated to host, without ever prompting.
func GhAuthCheck(host string) (string, error) {
	if host == "" {
		host = defaultGhHost
	}
	if !updplan.ValidHostname(host) {
		return "", fmt.Errorf("updexec: invalid gh-auth hostname %q", host)
	}
	return `command -v gh >/dev/null 2>&1 || exit 127; gh auth status -h ` + host + ` >/dev/null 2>&1`, nil
}

// GhAuthLogin drives gh's interactive web login flow — it never carries a
// token, so the credential lives only in the browser/device-code exchange
// gh itself performs.
func GhAuthLogin(host string) (string, error) {
	if host == "" {
		host = defaultGhHost
	}
	if !updplan.ValidHostname(host) {
		return "", fmt.Errorf("updexec: invalid gh-auth hostname %q", host)
	}
	return `gh auth login -h ` + host + ` --web --git-protocol https && gh auth setup-git -h ` + host, nil
}
