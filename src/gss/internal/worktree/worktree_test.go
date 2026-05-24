// Package worktree_test validates the backend registry and proves the
// contract suite itself works by running it against an in-memory backend
// (src/gss/docs/plan.md PR-20). The real git backend (PR-21) runs the same
// backendtest.RunContractSuite.
package worktree_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/wenlock/dotfiles/gss/internal/worktree"
	"github.com/wenlock/dotfiles/gss/internal/worktree/backendtest"
)

// memBackend is a trivial in-memory worktree.Backend for exercising the
// contract suite without touching the filesystem or git.
type memBackend struct {
	mu    sync.Mutex
	items map[string]worktree.Info
	dirty map[string]bool
}

func newMem() *memBackend {
	return &memBackend{items: map[string]worktree.Info{}, dirty: map[string]bool{}}
}

func (m *memBackend) Name() string { return "mem" }

func (m *memBackend) Create(req worktree.CreateReq) (worktree.Info, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.items[req.Path]; ok {
		return worktree.Info{}, fmt.Errorf("mem: %s already exists", req.Path)
	}
	info := worktree.Info{Path: req.Path, Branch: req.Branch, BaseBranch: req.BaseBranch, Backend: "mem", HeadSHA: "deadbeef"}
	m.items[req.Path] = info
	return info, nil
}

func (m *memBackend) Remove(path string, force bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dirty[path] && !force {
		return fmt.Errorf("mem: %s is dirty; refuse without force", path)
	}
	delete(m.items, path)
	delete(m.dirty, path)
	return nil
}

func (m *memBackend) List(root string) ([]worktree.Info, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []worktree.Info
	for p, i := range m.items {
		if strings.HasPrefix(p, root) {
			out = append(out, i)
		}
	}
	return out, nil
}

func (m *memBackend) Status(path string) (worktree.Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return worktree.Status{Clean: !m.dirty[path]}, nil
}

func (m *memBackend) makeDirty(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dirty[path] = true
}

func TestMemBackend_Contract(t *testing.T) {
	backendtest.RunContractSuite(t, func(t *testing.T) backendtest.Fixture {
		root := t.TempDir()
		mem := newMem()
		n := 0
		return backendtest.Fixture{
			Backend: mem,
			Root:    root,
			NewReq: func() worktree.CreateReq {
				n++
				return worktree.CreateReq{
					Path:       filepath.Join(root, fmt.Sprintf("wt%d", n)),
					Branch:     fmt.Sprintf("feature/x/wt%d", n),
					BaseBranch: "main",
				}
			},
			MakeDirty: mem.makeDirty,
		}
	})
}

func TestRegisterAndOpen(t *testing.T) {
	worktree.Register("mem-test", func() worktree.Backend { return newMem() })

	b, err := worktree.Open("mem-test")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if b.Name() != "mem" {
		t.Errorf("Open(mem-test).Name() = %q; want mem", b.Name())
	}
	found := false
	for _, n := range worktree.Registered() {
		if n == "mem-test" {
			found = true
		}
	}
	if !found {
		t.Errorf("Registered() = %v; want it to include mem-test", worktree.Registered())
	}
}

func TestOpen_Unknown(t *testing.T) {
	if _, err := worktree.Open("does-not-exist-xyz"); err == nil {
		t.Error("Open(unknown): err = nil; want error")
	}
}

func TestRegister_DuplicatePanics(t *testing.T) {
	worktree.Register("dup-test", func() worktree.Backend { return newMem() })
	defer func() {
		if recover() == nil {
			t.Error("duplicate Register: expected panic")
		}
	}()
	worktree.Register("dup-test", func() worktree.Backend { return newMem() })
}
