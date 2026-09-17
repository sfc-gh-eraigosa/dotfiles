// Package feature_test verifies feature start + worker add per
// sdk/gss/docs/plan.md PR-32: identifier validation (PR-08), worker_ref
// emission (PR-07), --description required, spawned_by persisted verbatim,
// unique tuple under the registry lock (PR-18).
package feature_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/feature"
	gitfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git/fake"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/identity"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/registry"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/worktree"
)

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// fakeBackend records Create calls and makes the worktree dir so WORKER.md
// can be written, without invoking real git. removeErr (if set) makes
// Remove fail on every call, after still recording it in removed. afterRemove
// (if set) runs synchronously right after recording the call and before
// returning — used to inject registry-lock contention at the EXACT moment
// after the worktree is gone but before the caller's next Store call, since
// Done's own earlier Store.Load would otherwise contend on the same lock if
// it were held for the whole test (dotfiles#98).
type fakeBackend struct {
	created     []worktree.CreateReq
	removed     []string
	removeErr   error
	afterRemove func()
}

func (b *fakeBackend) Name() string { return "git" }
func (b *fakeBackend) Create(req worktree.CreateReq) (worktree.Info, error) {
	b.created = append(b.created, req)
	if err := os.MkdirAll(req.Path, 0o755); err != nil {
		return worktree.Info{}, err
	}
	return worktree.Info{Path: req.Path, Branch: req.Branch, BaseBranch: req.BaseBranch, Backend: "git"}, nil
}
func (b *fakeBackend) Remove(path string, _ bool) error {
	b.removed = append(b.removed, path)
	if b.afterRemove != nil {
		b.afterRemove()
	}
	return b.removeErr
}
func (b *fakeBackend) List(string) ([]worktree.Info, error) { return nil, nil }
func (b *fakeBackend) Status(string) (worktree.Status, error) {
	return worktree.Status{Clean: true}, nil
}

func newService(t *testing.T) (*feature.Service, *fakeBackend, *registry.Store) {
	t.Helper()
	root := t.TempDir()
	store := registry.NewStore(filepath.Join(root, "registry.json"))
	be := &fakeBackend{}
	svc := &feature.Service{
		Store:        store,
		Backend:      be,
		Git:          &gitfake.Runner{Default: gitfake.Response{Stdout: []byte("abc123\n")}},
		Clock:        fixedClock{t: time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)},
		WorktreeRoot: filepath.Join(root, "wt"),
		NWO:          "octo/proj",
		UserSources: identity.UserSources{Getenv: func(k string) string {
			if k == "USER" {
				return "erai"
			}
			return ""
		}},
	}
	return svc, be, store
}

func TestStart_CreatesFeatureAndFile(t *testing.T) {
	svc, _, store := newService(t)
	dir, err := svc.Start(context.Background(), feature.StartOpts{Name: "auth", Description: "Add login", BaseBranch: "main"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	reg, _ := store.Load()
	if len(reg.Features) != 1 || reg.Features[0].Name != "auth" {
		t.Fatalf("registry features = %+v; want one named auth", reg.Features)
	}
	if reg.Features[0].BaseCommit != "abc123" {
		t.Errorf("base_commit = %q; want abc123 (from rev-parse)", reg.Features[0].BaseCommit)
	}
	data, err := os.ReadFile(filepath.Join(dir, "FEATURE.md"))
	if err != nil {
		t.Fatalf("read FEATURE.md: %v", err)
	}
	if !strings.Contains(string(data), "# Feature: auth") {
		t.Errorf("FEATURE.md missing header:\n%s", data)
	}
}

func TestStart_RejectsDuplicate(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	if _, err := svc.Start(ctx, feature.StartOpts{Name: "auth", Description: "x"}); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	if _, err := svc.Start(ctx, feature.StartOpts{Name: "auth", Description: "y"}); err == nil {
		t.Error("duplicate feature: want error")
	}
}

func TestStart_InvalidName(t *testing.T) {
	svc, _, _ := newService(t)
	if _, err := svc.Start(context.Background(), feature.StartOpts{Name: "Bad Name", Description: "x"}); err == nil {
		t.Error("invalid feature name: want error")
	}
}

// TestStart_RejectsFlagLikeBase pins the dotfiles#96 fix at the third write
// path into a BaseBranch value: `feature start --base` seeds
// DefaultBaseBranch, which worker add falls back to when --base is omitted,
// and from there flows into the same rebase call sites.
func TestStart_RejectsFlagLikeBase(t *testing.T) {
	svc, _, store := newService(t)
	if _, err := svc.Start(context.Background(), feature.StartOpts{Name: "auth", Description: "x", BaseBranch: "--exec=touch /tmp/pwned"}); err == nil {
		t.Error("flag-like --base: want a validation error")
	}
	reg, _ := store.Load()
	if len(reg.Features) != 0 {
		t.Errorf("registry features = %+v; want none (rejected before persisting)", reg.Features)
	}
}
