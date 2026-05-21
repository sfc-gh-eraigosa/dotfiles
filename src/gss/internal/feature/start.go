// Package feature orchestrates the `gss feature` worktree workflow,
// composing registry + worktree + identity + tmpl. This file is the
// Service plus `feature start` (design.md → "gss feature start"); worker
// add/update live in worker.go.
package feature

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wenlock/dotfiles/gss/internal/config"
	"github.com/wenlock/dotfiles/gss/internal/errors"
	"github.com/wenlock/dotfiles/gss/internal/gh"
	"github.com/wenlock/dotfiles/gss/internal/git"
	"github.com/wenlock/dotfiles/gss/internal/identity"
	"github.com/wenlock/dotfiles/gss/internal/registry"
	"github.com/wenlock/dotfiles/gss/internal/tmpl"
	"github.com/wenlock/dotfiles/gss/internal/worktree"
)

// Service creates features and workers. Dependencies are injected so the
// orchestration is testable with a temp registry Store, a fake worktree
// Backend, the git fake, and the gh fake.
type Service struct {
	Store        *registry.Store
	Backend      worktree.Backend
	Git          git.Runner
	GH           gh.Client
	Approval     approver // approval-token verifier for `pr --ready` (PR-37)
	Observe      Observer // read-only audit observer (PR-42); nil → built from Git/GH/os
	Clock        config.Clock
	WorktreeRoot string               // expanded worktrees root
	NWO          string               // "owner/repo"
	BranchPrefix string               // default "feature"
	UserSources  identity.UserSources // Override is set per WorkerAdd call
}

func (s *Service) branchPrefix() string {
	if s.BranchPrefix != "" {
		return s.BranchPrefix
	}
	return "feature"
}

func (s *Service) now() string { return s.Clock.Now().UTC().Format(time.RFC3339) }

// StartOpts configures `feature start`.
type StartOpts struct {
	Name        string
	Description string
	BaseBranch  string // default "main"
	Goal        string
}

// Start validates the feature name, refuses a duplicate, captures the base
// commit (origin/<base>), writes the feature row under the registry lock,
// and renders FEATURE.md into the feature directory. Returns the feature
// directory path.
func (s *Service) Start(ctx context.Context, opts StartOpts) (string, error) {
	if err := identity.ValidateFeature(opts.Name); err != nil {
		return "", err
	}
	base := opts.BaseBranch
	if base == "" {
		base = "main"
	}

	_, _ = s.Git.Run(ctx, "fetch", "origin") // best-effort
	baseCommit := ""
	if out, err := s.Git.Run(ctx, "rev-parse", "origin/"+base); err == nil {
		baseCommit = strings.TrimSpace(string(out))
	}
	started := s.now()

	err := s.Store.Update(func(r *registry.Registry) error {
		for _, f := range r.Features {
			if f.Name == opts.Name {
				return fmt.Errorf("%w: feature %q already exists", errors.ErrInvalidIdent, opts.Name)
			}
		}
		r.Features = append(r.Features, registry.Feature{
			Name:              opts.Name,
			StartedAt:         started,
			BaseCommit:        baseCommit,
			DefaultBaseBranch: base,
			Description:       opts.Description,
		})
		return nil
	})
	if err != nil {
		return "", err
	}

	featDir := filepath.Join(s.WorktreeRoot, s.NWO, opts.Name)
	if err := os.MkdirAll(featDir, 0o755); err != nil {
		return "", fmt.Errorf("feature: mkdir %s: %w", featDir, err)
	}
	content, err := tmpl.RenderEmbeddedFeature(tmpl.FeatureData{
		Name: opts.Name, Description: opts.Description, StartedAt: started, BaseBranch: base,
	})
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(featDir, "FEATURE.md"), []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("feature: write FEATURE.md: %w", err)
	}
	return featDir, nil
}
