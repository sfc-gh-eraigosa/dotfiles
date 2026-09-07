package cmd

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// build.sh hands over `git remote get-url origin` VERBATIM, so both remote
// spellings must resolve to the same web URL — parsing it here rather than in
// shell is what makes that testable.
func TestCommitURLAcceptsEveryRemoteSpelling(t *testing.T) {
	want := "https://github.com/acme/dotfiles/commit/abc1234"
	for _, remote := range []string{
		"git@github.com:acme/dotfiles.git",
		"git@github.com:acme/dotfiles",
		"ssh://git@github.com/acme/dotfiles.git",
		"https://github.com/acme/dotfiles.git",
		"https://github.com/acme/dotfiles",
		"  https://github.com/acme/dotfiles.git\n",
	} {
		if got := commitURL(remote, "abc1234"); got != want {
			t.Fatalf("commitURL(%q) = %q, want %q", remote, got, want)
		}
	}
}

// A dev build has no commit to link to. Emitting a link to /commit/none would
// hand the operator a 404 that LOOKS authoritative.
func TestCommitURLNeedsARealSHA(t *testing.T) {
	for _, commit := range []string{"", "none", "dev", "abc123", "not-a-sha", "abc1234-dirty"} {
		if got := commitURL("https://github.com/acme/dotfiles", commit); got != "" {
			t.Fatalf("commitURL(commit=%q) = %q, want no link", commit, got)
		}
	}
}

// Only GitHub's path shape is known here. A GitLab remote would need
// /-/commit/, so guessing would produce a broken link rather than none.
func TestCommitURLOnlyLinksKnownForges(t *testing.T) {
	for _, remote := range []string{
		"", "git@gitlab.com:acme/dotfiles.git", "https://example.com/acme/dotfiles",
		"git@github.com:acme", "https://github.com/acme", "not a url",
	} {
		if got := commitURL(remote, "abc1234"); got != "" {
			t.Fatalf("commitURL(%q) = %q, want no link", remote, got)
		}
	}
}

// The banner's commit must be clickable, and the URL must cost zero cells —
// a link that is MEASURED would blow the panel's width budget.
func TestBannerVersionLinksTheCommitAtZeroWidth(t *testing.T) {
	v, c, r := Version, Commit, Repo
	defer func() { Version, Commit, Repo = v, c, r }()
	Version, Commit, Repo = "9.9.9", "abc1234", "git@github.com:acme/dotfiles.git"

	got := bannerVersion()
	if !strings.Contains(got, "\x1b]8;;https://github.com/acme/dotfiles/commit/abc1234\x1b\\") {
		t.Fatalf("banner version must carry an OSC 8 link to the commit: %q", got)
	}
	if w, want := lipgloss.Width(got), lipgloss.Width(versionString()); w != want {
		t.Fatalf("linked version is %d cells wide, plain is %d — the URL is being measured", w, want)
	}
	// The closing empty-URL OSC is not optional: without it every cell painted
	// after the commit stays clickable.
	if !strings.Contains(got, "abc1234\x1b]8;;\x1b\\") {
		t.Fatalf("the link must be closed after the commit: %q", got)
	}
}

// An un-injected binary must render exactly what it always rendered.
func TestBannerVersionIsPlainWithoutASHA(t *testing.T) {
	v, c := Version, Commit
	defer func() { Version, Commit = v, c }()
	Version, Commit = "dev", "none"
	if got := bannerVersion(); got != versionString() {
		t.Fatalf("bannerVersion() = %q, want the plain %q", got, versionString())
	}
}

// Repo is injected by ldflags exactly like Version/Commit; unexporting or
// renaming it breaks the injection silently.
func TestRepoIsAnLdflagsTarget(t *testing.T) {
	if Repo == "" {
		t.Fatal("Repo must carry a default remote so an un-injected build still links")
	}
	if commitURL(Repo, "abc1234") == "" {
		t.Fatalf("the default Repo (%q) must resolve to a commit URL", Repo)
	}
}
