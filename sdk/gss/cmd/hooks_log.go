package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/hooklog"
)

var hooksLogCmd = &cobra.Command{
	Use:   "log <hook> <exit-code|chained>",
	Short: "Append one line for a git hook run (called by the privacy git hooks)",
	Long: `The privacy git hooks (pre-commit, commit-msg, pre-push) call this once per
run: from an EXIT trap with their exit code, and with "chained" just before
they hand off to the repo's own hook. Each call appends one JSON line (time,
host, repo, hook, result, exit code, and skip when PRIVACY_GUARD_SKIP is set)
to $GSS_HOOK_LOG, else $XDG_STATE_HOME/githooks/runs.jsonl, else
~/.local/state/githooks/runs.jsonl. A commit or push with no line means no
hook ran for it.

It always exits 0 and never reads stdin, so it cannot change what a hook
allows or refuses. A problem is printed to stderr, which the hooks discard.`,
	Args: cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if err := writeHookLog(args, time.Now()); err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), err)
		}
	},
}

func writeHookLog(args []string, now time.Time) error {
	if len(args) != 2 {
		return fmt.Errorf("hooks log: want <hook> <exit-code|chained>, got %d args", len(args))
	}
	host, _ := os.Hostname()
	cwd, _ := os.Getwd()
	e, err := hooklog.Build(now, host, cwd, args[0], args[1], os.Getenv)
	if err != nil {
		return err
	}
	return hooklog.Append(hooklog.Path(os.Getenv), e)
}

func init() {
	hooksCmd.AddCommand(hooksLogCmd)
}
