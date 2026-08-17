package cmd

import (
	"fmt"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/spf13/cobra"
)

// UpdateResult reports what happened to one host.
type UpdateResult struct {
	Skipped bool
	Reason  string
}

// validRef guards the operator-supplied update target. The ref is interpolated
// into a command that runs over ssh, so it is constrained to the git ref
// charset (letters, digits, and . _ / -). Anything else — shell metacharacters,
// spaces, command substitution — is rejected before it can run.
func validRef(ref string) bool {
	if ref == "" {
		return false
	}
	for _, r := range ref {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.', r == '_', r == '/', r == '-':
		default:
			return false
		}
	}
	return true
}

// remoteUpdateScript is the update command for one host, parameterised by the
// git ref to move to (default `main`). Fetch, switch to the ref, fast-forward
// it, then re-run install.sh. Shared by the headless path and the TUI so there
// is exactly one definition of "update a host". Callers pass only refs that
// have passed validRef.
func remoteUpdateScript(ref string) string {
	return fmt.Sprintf(
		"cd ~/git/dotfiles && git fetch origin && git checkout %[1]s && "+
			"git pull --ff-only origin %[1]s && ./install.sh", ref)
}

// rescueWorktree preserves uncommitted work before a --force update.
//
// It commits the dirty state (tracked AND untracked, via `git add -A`) onto a
// rescue branch, returns the clone to its original branch — now clean, so
// pull --ff-only can proceed — and materialises the rescue branch as its own
// worktree for the operator to inspect. Nothing is ever discarded.
//
// The plan originally specified `git stash push -u` + `git branch <n> stash@{0}`.
// Verified against git 2.47.3 (evidence/task12/rescue-verification.txt): that
// approach recovers tracked modifications but SILENTLY LOSES untracked files,
// because a stash commit's tree excludes them (they live in stash^3). An
// operator inspecting the rescue worktree would conclude their untracked work
// was gone. The temp-commit form below recovers both.
const rescueWorktree = `cd ~/git/dotfiles && ts=$(date -u +%Y%m%dT%H%M%SZ) && ` +
	`orig=$(git rev-parse --abbrev-ref HEAD) && ` +
	`git checkout -q -b "fleet-rescue/$ts" && git add -A && ` +
	`git -c user.email=fleet@local -c user.name=fleet commit -q -m "fleet rescue $ts" && ` +
	`git checkout -q "$orig" && ` +
	`mkdir -p ~/.local/state/dotfiles/rescue && ` +
	`git worktree add ~/.local/state/dotfiles/rescue/$ts "fleet-rescue/$ts"`

// updateHost fetches, fast-forwards and re-runs install.sh on one host.
// A dirty clone is SKIPPED by default — never mutate a machine carrying local
// work — and with --force the work is rescued first, not thrown away.
func updateHost(r runner.Runner, host, ref string, force bool) (UpdateResult, error) {
	dirty, err := r.Run(host, "git -C ~/git/dotfiles status --porcelain")
	if err != nil {
		return UpdateResult{}, fmt.Errorf("%s: checking clone state: %w", host, err)
	}
	if strings.TrimSpace(dirty) != "" {
		if !force {
			return UpdateResult{
				Skipped: true,
				Reason:  "clone is dirty; re-run with --force to preserve local work in a rescue worktree",
			}, nil
		}
		if _, err := r.Run(host, rescueWorktree); err != nil {
			return UpdateResult{}, fmt.Errorf("%s: rescuing local work (refusing to continue): %w", host, err)
		}
	}
	return UpdateResult{}, r.RunInteractive(host, remoteUpdateScript(ref))
}

var (
	updateForce bool
	updateRef   string
)

var updateCmd = &cobra.Command{
	Use:   "update <host>...",
	Short: "Pull and re-run install.sh on a host, interactively",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !validRef(updateRef) {
			return fmt.Errorf("invalid --ref %q: must be a git ref (letters, digits, . _ / -)", updateRef)
		}
		r := runner.Exec{}
		var failures int
		// One host at a time: interactive sessions cannot share a terminal.
		for _, host := range args {
			fmt.Fprintf(cmd.OutOrStdout(), "\n=== %s ===\n", host)
			res, err := updateHost(r, host, updateRef, updateForce)
			switch {
			case err != nil:
				fmt.Fprintf(cmd.ErrOrStderr(), "FAIL %s: %v\n", host, err)
				failures++
			case res.Skipped:
				fmt.Fprintf(cmd.OutOrStdout(), "SKIP %s: %s\n", host, res.Reason)
				failures++
			default:
				fmt.Fprintf(cmd.OutOrStdout(), "ok   %s\n", host)
			}
		}
		if failures > 0 {
			return fmt.Errorf("%d host(s) not updated", failures)
		}
		return nil
	},
}

func init() {
	updateCmd.Flags().BoolVar(&updateForce, "force", false, "rescue uncommitted work into a worktree, then update")
	updateCmd.Flags().StringVar(&updateRef, "ref", "main", "git ref (branch or tag) to update the host to")
	rootCmd.AddCommand(updateCmd)
}
