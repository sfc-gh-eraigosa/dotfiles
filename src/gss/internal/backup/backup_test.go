// Package backup_test verifies the safety-branch creator per
// src/gss/docs/plan.md PR-11: fixed branch-name format from an injected
// Clock (byte-identical to classic cmd/backup.go), idempotent rerun via a
// monotonic suffix, and git.Runner usage.
package backup_test

import (
	stderrors "errors"
	"testing"
	"time"

	"github.com/wenlock/dotfiles/gss/internal/backup"
	gitfake "github.com/wenlock/dotfiles/gss/internal/git/fake"
)

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// clockAt builds a fixed clock; the format yields 20260521-123456.
func clockAt() fixedClock {
	return fixedClock{t: time.Date(2026, 5, 21, 12, 34, 56, 0, time.UTC)}
}

// notExist is a scripted "ref not found" response (rev-parse exits non-zero).
var notExist = gitfake.Response{Err: stderrors.New("not a valid ref")}

// exists is a scripted "ref found" response (rev-parse exits 0).
var exists = gitfake.Response{}

func TestCreate_BranchNameFromClock(t *testing.T) {
	// rev-parse(base) → not exist; branch(base) → ok.
	gitr := &gitfake.Runner{Script: []gitfake.Response{notExist, {}}}
	s := backup.NewService(gitr, clockAt())

	name, err := s.Create(t.Context(), "/repo")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if name != "backup/gss-20260521-123456" {
		t.Errorf("name = %q; want backup/gss-20260521-123456", name)
	}
	// Last call must be the branch creation with that name.
	last := gitr.Calls[len(gitr.Calls)-1]
	if !argsContain(last.Args, "branch") || !argsContain(last.Args, name) {
		t.Errorf("final git call = %+v; want `branch %s`", last, name)
	}
}

func TestCreate_IdempotentMonotonicSuffix(t *testing.T) {
	// base exists; base-2 does not → name is base-2.
	gitr := &gitfake.Runner{Script: []gitfake.Response{exists, notExist, {}}}
	s := backup.NewService(gitr, clockAt())

	name, err := s.Create(t.Context(), "/repo")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if name != "backup/gss-20260521-123456-2" {
		t.Errorf("name = %q; want ...-2 (monotonic suffix)", name)
	}
}

func TestCreate_GitError(t *testing.T) {
	// rev-parse → not exist; branch → error.
	gitr := &gitfake.Runner{Script: []gitfake.Response{notExist, {Err: stderrors.New("branch failed")}}}
	s := backup.NewService(gitr, clockAt())
	if _, err := s.Create(t.Context(), "/repo"); err == nil {
		t.Error("Create with git branch failure: err = nil; want error")
	}
}

func argsContain(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}
