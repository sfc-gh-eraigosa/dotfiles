// Package backendtest provides the shared contract suite every worktree
// backend must pass (src/gss/docs/plan.md PR-20). A backend implementation
// proves wire-compatibility by calling RunContractSuite with a fixture
// factory; the git backend (PR-21) and any future backend run the same
// suite.
package backendtest

import (
	"testing"

	"github.com/wenlock/dotfiles/gss/internal/worktree"
)

// Fixture is a ready-to-test backend plus the environment knowledge the
// suite needs. The factory builds it (e.g. the git backend sets up a temp
// repo); the suite stays backend-agnostic.
type Fixture struct {
	// Backend under test.
	Backend worktree.Backend
	// Root is the directory List should enumerate.
	Root string
	// NewReq returns a fresh CreateReq with a unique path/branch under Root,
	// valid for this backend's environment.
	NewReq func() worktree.CreateReq
	// MakeDirty, if non-nil, makes the worktree at path report Status.Clean
	// == false, enabling the dirty-removal subtest.
	MakeDirty func(path string)
}

// RunContractSuite exercises the Backend contract against a fresh fixture
// per subtest (so state never leaks between cases).
func RunContractSuite(t *testing.T, newFixture func(t *testing.T) Fixture) {
	t.Helper()

	t.Run("NameNonEmpty", func(t *testing.T) {
		f := newFixture(t)
		if f.Backend.Name() == "" {
			t.Error("Backend.Name() returned empty")
		}
	})

	t.Run("CreateStatusList", func(t *testing.T) {
		f := newFixture(t)
		req := f.NewReq()
		info, err := f.Backend.Create(req)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if info.Path != req.Path || info.Branch != req.Branch {
			t.Errorf("Create Info = %+v; want Path/Branch from req %+v", info, req)
		}
		if info.Backend != f.Backend.Name() {
			t.Errorf("Info.Backend = %q; want %q", info.Backend, f.Backend.Name())
		}

		st, err := f.Backend.Status(req.Path)
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		if !st.Clean {
			t.Errorf("fresh worktree Status.Clean = false; want true")
		}

		list, err := f.Backend.List(f.Root)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if !containsPath(list, req.Path) {
			t.Errorf("List(%q) = %+v; want it to include %q", f.Root, list, req.Path)
		}
	})

	t.Run("RemoveThenGone", func(t *testing.T) {
		f := newFixture(t)
		req := f.NewReq()
		if _, err := f.Backend.Create(req); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := f.Backend.Remove(req.Path, false); err != nil {
			t.Fatalf("Remove(clean): %v", err)
		}
		list, err := f.Backend.List(f.Root)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if containsPath(list, req.Path) {
			t.Errorf("List still includes %q after Remove", req.Path)
		}
	})

	t.Run("IdempotentRecreateAfterRemove", func(t *testing.T) {
		f := newFixture(t)
		req := f.NewReq()
		if _, err := f.Backend.Create(req); err != nil {
			t.Fatalf("Create 1: %v", err)
		}
		if err := f.Backend.Remove(req.Path, false); err != nil {
			t.Fatalf("Remove: %v", err)
		}
		if _, err := f.Backend.Create(req); err != nil {
			t.Errorf("re-Create after Remove: %v; want success (idempotent retry)", err)
		}
	})

	t.Run("RemoveRefusesDirtyWithoutForce", func(t *testing.T) {
		f := newFixture(t)
		if f.MakeDirty == nil {
			t.Skip("fixture does not support MakeDirty")
		}
		req := f.NewReq()
		if _, err := f.Backend.Create(req); err != nil {
			t.Fatalf("Create: %v", err)
		}
		f.MakeDirty(req.Path)
		if err := f.Backend.Remove(req.Path, false); err == nil {
			t.Error("Remove(dirty, force=false) = nil; want refusal")
		}
		if err := f.Backend.Remove(req.Path, true); err != nil {
			t.Errorf("Remove(dirty, force=true) = %v; want success", err)
		}
	})
}

func containsPath(list []worktree.Info, path string) bool {
	for _, i := range list {
		if i.Path == path {
			return true
		}
	}
	return false
}
