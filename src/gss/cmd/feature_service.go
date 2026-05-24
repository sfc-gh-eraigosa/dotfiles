package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wenlock/dotfiles/gss/internal/approval"
	"github.com/wenlock/dotfiles/gss/internal/config"
	"github.com/wenlock/dotfiles/gss/internal/errors"
	"github.com/wenlock/dotfiles/gss/internal/feature"
	"github.com/wenlock/dotfiles/gss/internal/gh"
	"github.com/wenlock/dotfiles/gss/internal/git"
	"github.com/wenlock/dotfiles/gss/internal/identity"
	"github.com/wenlock/dotfiles/gss/internal/mode"
	"github.com/wenlock/dotfiles/gss/internal/registry"
	"github.com/wenlock/dotfiles/gss/internal/repo"
	wtgit "github.com/wenlock/dotfiles/gss/internal/worktree/git"
)

// newFeatureService builds the fully-wired feature.Service from config + the
// resolved repo: the system git/gh runners, the git worktree backend bound to
// the repo, the registry store, the approval verifier, and the user-resolution
// sources. Shared by every gss feature leaf that mutates worktrees (start,
// worker add, checkpoint, …). Read-only `list` uses newRegistryStore instead,
// so it never needs origin/NWO resolution.
func newFeatureService() (*feature.Service, error) {
	cfg, err := config.Load(config.Options{})
	if err != nil {
		return nil, err
	}
	repoPath := getRepoPath()
	runner := git.NewSystemRunner()
	ghc := gh.NewSystemClient()
	regDir := expandHome(cfg.Paths.RegistryDir)

	nwo, err := repo.NewResolver(ghc, runner, repo.NewCache(regDir)).Resolve(context.Background(), "")
	if err != nil {
		return nil, err
	}

	home, _ := os.UserHomeDir()
	tokenPath := filepath.Join(home, ".config", "gss", "approval.token")

	return &feature.Service{
		Store:        registry.NewStore(filepath.Join(regDir, "registry.json")),
		Backend:      wtgit.NewBackend(repoPath, runner),
		Git:          runner,
		GH:           ghc,
		Approval:     approval.NewVerifier(tokenPath, runner),
		Clock:        config.SystemClock{},
		WorktreeRoot: expandHome(cfg.Paths.WorktreeRoot),
		NWO:          nwo.String(),
		BranchPrefix: "feature",
		UserSources: identity.UserSources{
			// GHLogin is intentionally nil: the gh.Client has no CurrentUser
			// verb in v1, so user resolution falls through git email → $USER.
			// Wiring the gh login step needs a small internal/gh addition.
			GitEmail: func() (string, error) {
				out, err := runner.Run(context.Background(), "-C", repoPath, "config", "user.email")
				return strings.TrimSpace(string(out)), err
			},
			Getenv: os.Getenv,
		},
	}, nil
}

// newRegistryStore builds just the registry Store from config — the minimal
// dependency for read-only feature commands (list), avoiding origin/NWO
// resolution that those commands don't need.
func newRegistryStore() (*registry.Store, error) {
	cfg, err := config.Load(config.Options{})
	if err != nil {
		return nil, err
	}
	return registry.NewStore(filepath.Join(expandHome(cfg.Paths.RegistryDir), "registry.json")), nil
}

// fail prints err to stderr and exits with its mapped gss exit code — the
// shared error path for the feature leaves (mirrors cmd/push.go).
func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(errors.ExitCode(err))
}

// currentWorkerRef resolves the worker_ref of the worktree containing cwd
// (the per-worker default for checkpoint/pr/rebase). Returns ErrWrongMode if
// cwd is not inside a registered worker worktree.
func currentWorkerRef() (string, error) {
	reg, _ := loadRegistry()
	cwd, _ := os.Getwd()
	ref, ok := mode.IsInWorker(cwd, reg)
	if !ok {
		return "", errors.ErrWrongMode
	}
	return ref, nil
}
