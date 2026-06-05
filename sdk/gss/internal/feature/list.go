package feature

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/errors"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/identity"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/registry"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/stack"
)

// ListOpts configures `feature list`.
type ListOpts struct {
	Feature      string // filter to one feature; "" = all
	Tree         bool   // render stack relationships (indent by depth)
	JSON         bool
	WithSessions bool // reserved for v1.1
}

// List renders the registry's features + workers. Description is shown by
// default; --tree indents by stack depth; --json emits a stable schema.
// `--with sessions` is reserved for v1.1 and errors. (Reconciliation
// against live git/gh is the caller's concern via registry.Reconciler.)
func (s *Service) List(_ context.Context, opts ListOpts) (string, error) {
	if opts.WithSessions {
		return "", fmt.Errorf("feature list --with sessions: not yet implemented (reserved for v1.1)")
	}
	reg, err := s.Store.Load()
	if err != nil {
		return "", err
	}
	feats := reg.Features
	if opts.Feature != "" {
		var filtered []registry.Feature
		for _, f := range feats {
			if f.Name == opts.Feature {
				filtered = append(filtered, f)
			}
		}
		if len(filtered) == 0 {
			return "", fmt.Errorf("%w: no such feature %q", errors.ErrInvalidIdent, opts.Feature)
		}
		feats = filtered
	}
	if opts.JSON {
		return renderJSON(feats)
	}
	return renderText(feats, opts.Tree), nil
}

func workerRef(featureName string, w registry.Worker) string {
	return identity.WorkerRef{Feature: featureName, User: w.User, Purpose: w.Purpose, Suffix: w.Suffix}.String()
}

func renderText(feats []registry.Feature, tree bool) string {
	var b strings.Builder
	for _, f := range feats {
		fmt.Fprintf(&b, "Feature: %s (base: %s)\n", f.Name, f.DefaultBaseBranch)
		if f.Description != "" {
			fmt.Fprintf(&b, "  %s\n", f.Description)
		}
		if tree {
			renderTree(&b, f)
		} else {
			renderFlat(&b, f)
		}
	}
	return b.String()
}

func renderFlat(b *strings.Builder, f registry.Feature) {
	for _, w := range f.Workers {
		fmt.Fprintf(b, "  - %s — %s%s\n", workerRef(f.Name, w), w.Description, stateSuffix(w))
	}
}

func renderTree(b *strings.Builder, f registry.Feature) {
	nodes := make([]stack.Node, len(f.Workers))
	for i, w := range f.Workers {
		nodes[i] = stack.Node{Ref: workerRef(f.Name, w), Branch: w.Branch, BaseBranch: w.BaseBranch}
	}
	for i, w := range f.Workers {
		depth := 0
		if chain, err := stack.Ancestors(nodes, nodes[i]); err == nil {
			depth = len(chain) - 1
		}
		indent := strings.Repeat("  ", depth+1)
		fmt.Fprintf(b, "%s- %s — %s%s\n", indent, workerRef(f.Name, w), w.Description, stateSuffix(w))
	}
}

func stateSuffix(w registry.Worker) string {
	if w.PRState != "" {
		return " [" + w.PRState + "]"
	}
	return ""
}

// JSON view — a stable schema for tooling. Snapshot-tested.
type jsonFeature struct {
	Name              string       `json:"name"`
	DefaultBaseBranch string       `json:"default_base_branch"`
	Description       string       `json:"description,omitempty"`
	Workers           []jsonWorker `json:"workers"`
}

type jsonWorker struct {
	Ref         string `json:"worker_ref"`
	Branch      string `json:"branch"`
	BaseBranch  string `json:"base_branch"`
	Description string `json:"description"`
	PRState     string `json:"pr_state,omitempty"`
	PRURL       string `json:"pr_url,omitempty"`
}

func renderJSON(feats []registry.Feature) (string, error) {
	out := make([]jsonFeature, 0, len(feats))
	for _, f := range feats {
		jf := jsonFeature{Name: f.Name, DefaultBaseBranch: f.DefaultBaseBranch, Description: f.Description}
		ws := append([]registry.Worker(nil), f.Workers...)
		sort.SliceStable(ws, func(i, j int) bool { return workerRef(f.Name, ws[i]) < workerRef(f.Name, ws[j]) })
		for _, w := range ws {
			jf.Workers = append(jf.Workers, jsonWorker{
				Ref: workerRef(f.Name, w), Branch: w.Branch, BaseBranch: w.BaseBranch,
				Description: w.Description, PRState: w.PRState, PRURL: w.PRURL,
			})
		}
		out = append(out, jf)
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", fmt.Errorf("feature list: marshal json: %w", err)
	}
	return string(data) + "\n", nil
}
