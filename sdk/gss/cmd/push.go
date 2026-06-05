package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/approval"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/backup"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/classic"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/errors"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/mode"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/registry"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/sync"
)

var forceAutonomous bool

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Safely push changes to origin",
	Run: func(cmd *cobra.Command, args []string) {
		path := getRepoPath()
		cwd, _ := os.Getwd()

		// Mode gate (canonical, via internal/mode — PR-26): `gss push` is a
		// classic-mode command and is invalid inside a registered feature
		// worker worktree, even with --force-autonomous.
		reg, _ := loadRegistry()
		ref, inWorker := mode.IsInWorker(cwd, reg)
		if err := classicAllowed(inWorker, forceAutonomous); err != nil {
			fmt.Fprintf(os.Stderr, "Error: 'gss push' is not valid inside feature worker worktree %q; use the gss feature commands.\n", ref)
			os.Exit(errors.ExitCode(err))
		}

		// Wire and run the classic push orchestrator (PR-22).
		runner := git.NewSystemRunner()
		home, _ := os.UserHomeDir()
		tokenPath := filepath.Join(home, ".config", "gss", "approval.token")
		pusher := &classic.Pusher{
			Git:      runner,
			GH:       gh.NewSystemClient(),
			Approval: approval.NewVerifier(tokenPath, runner),
			Backup:   backup.NewService(runner, config.SystemClock{}),
			Sync:     sync.NewService(runner),
			Out:      os.Stdout,
		}
		if err := pusher.Push(context.Background(), classic.PushOpts{
			RepoPath:        path,
			DefaultBranch:   getDefaultBranch(),
			ForceAutonomous: forceAutonomous,
		}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(errors.ExitCode(err))
		}
	},
}

// classicAllowed enforces the mode gate: a classic command is rejected
// (errors.ErrWrongMode) inside a feature worker worktree. --force-autonomous
// bypasses the approval prompt, NOT the mode gate, so force is irrelevant
// here — both worker-mode combinations are refused.
func classicAllowed(inWorker, forceAutonomous bool) error {
	_ = forceAutonomous // documented: force never bypasses the mode gate
	if inWorker {
		return errors.ErrWrongMode
	}
	return nil
}

// loadRegistry best-effort loads the per-repo registry. Returns
// (zero, false) when it can't be found — the v1 norm until feature
// commands populate it. (Preliminary path resolution; PR-26 + the feature
// commands resolve the canonical per-NWO nested path.)
func loadRegistry() (registry.Registry, bool) {
	cfg, err := config.Load(config.Options{})
	if err != nil {
		return registry.Registry{}, false
	}
	path := filepath.Join(expandHome(cfg.Paths.RegistryDir), "registry.json")
	if _, err := os.Stat(path); err != nil {
		return registry.Registry{}, false
	}
	reg, err := registry.NewStore(path).Load()
	if err != nil {
		return registry.Registry{}, false
	}
	return reg, true
}

// expandHome expands a leading ~/ to the user's home directory.
func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

func init() {
	pushCmd.Flags().BoolVar(&forceAutonomous, "force-autonomous", false, "Skip the interactive confirmation prompt (Dangerous)")
	rootCmd.AddCommand(pushCmd)
}
