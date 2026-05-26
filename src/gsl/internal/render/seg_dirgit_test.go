package render

import (
	"context"
	"errors"
	"strings"
	"testing"

	gitfake "github.com/wenlock/dotfiles/gsl/internal/git/fake"
)

func TestDirGit_WithGit_ASCII(t *testing.T) {
	st := asciiStyle()

	r := &gitfake.Runner{Script: gitStatusResponses("main")}
	seg := &DirGitSegment{Cwd: "/home/user/project", Git: r, home: "/home/user"}

	got, ok := seg.Render(context.Background(), st)
	if !ok {
		t.Fatal("dirgit: want ok=true")
	}
	// ascii glyphs: [dir] dir, br: branch, *1 staged, !1 unstaged, ?1 untracked,
	// $1 stash, +2 ahead (ascii table: staged=*, unstaged=!, untracked=?,
	// stash=$, ahead=+).
	for _, want := range []string{"[dir]", "project", "br:", "main", "*1", "!1", "?1", "$1", "+2"} {
		if !strings.Contains(got, want) {
			t.Errorf("dirgit ascii output %q missing %q", got, want)
		}
	}
}

func TestDirGit_NotARepo_ShowsCwdOnly(t *testing.T) {
	st := asciiStyle()

	r := &gitfake.Runner{Default: gitfake.Response{Err: errors.New("not a git repo")}}
	seg := &DirGitSegment{Cwd: "/home/user/project", Git: r, home: "/home/user"}

	got, ok := seg.Render(context.Background(), st)
	if !ok {
		t.Fatal("dirgit: want ok=true even outside a repo")
	}
	if !strings.Contains(got, "project") {
		t.Errorf("dirgit: want cwd basename, got %q", got)
	}
	if strings.Contains(got, "br:") {
		t.Errorf("dirgit: should not show branch outside a repo, got %q", got)
	}
}

func TestDirGit_HomeAbbreviation(t *testing.T) {
	st := asciiStyle()
	r := &gitfake.Runner{Default: gitfake.Response{Err: errors.New("nope")}}

	seg := &DirGitSegment{Cwd: "/home/user", Git: r, home: "/home/user"}
	got, _ := seg.Render(context.Background(), st)
	if !strings.Contains(got, "~") {
		t.Errorf("dirgit: home dir should abbreviate to ~, got %q", got)
	}
}

func TestDirGit_CancelledContext_OmitsGitDetail(t *testing.T) {
	st := asciiStyle()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	r := &gitfake.Runner{Script: gitStatusResponses("main")}
	seg := &DirGitSegment{Cwd: "/home/user/project", Git: r, home: "/home/user"}

	got, ok := seg.Render(ctx, st)
	if !ok {
		t.Fatal("dirgit: want ok=true (cwd still renders)")
	}
	if strings.Contains(got, "main") {
		t.Errorf("dirgit: cancelled ctx should omit git detail, got %q", got)
	}
}
