// Package status_test verifies the porcelain status formatter per
// sdk/gss/docs/plan.md PR-14: empty tree → "No changes detected", dirty
// tree lists paths, output byte-identical to classic cmd/status.go.
package status_test

import (
	stderrors "errors"
	"testing"

	gitfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git/fake"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/status"
)

func TestFormat_NoChanges(t *testing.T) {
	for _, porcelain := range []string{"", "\n", "  \n"} {
		got := status.Format("/repo", porcelain)
		want := "No changes detected in /repo.\n"
		if got != want {
			t.Errorf("Format(%q) = %q; want %q", porcelain, got, want)
		}
	}
}

func TestFormat_Dirty(t *testing.T) {
	porcelain := " M a.go\n?? b.txt\n"
	got := status.Format("/repo", porcelain)
	want := "Changes in /repo:\n" +
		" -  M a.go\n" + // note: " - " + " M a.go" (porcelain leading space preserved)
		" - ?? b.txt\n"
	if got != want {
		t.Errorf("Format dirty =\n%q\nwant\n%q", got, want)
	}
}

func TestStatus_ViaRunner(t *testing.T) {
	gitr := &gitfake.Runner{Default: gitfake.Response{Stdout: []byte(" M x.go\n")}}
	s := status.NewService(gitr)
	got, err := s.Status(t.Context(), "/repo")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if got != "Changes in /repo:\n -  M x.go\n" {
		t.Errorf("Status = %q", got)
	}
}

func TestStatus_GitError(t *testing.T) {
	gitr := &gitfake.Runner{Default: gitfake.Response{Err: stderrors.New("not a repo")}}
	s := status.NewService(gitr)
	if _, err := s.Status(t.Context(), "/repo"); err == nil {
		t.Error("Status with git error: err = nil; want error")
	}
}
