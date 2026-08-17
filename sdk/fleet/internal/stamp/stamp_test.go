package stamp

import (
	"strings"
	"testing"
)

// repeat — deliberately not named `strings`, which would shadow the package.
func repeat(c string, n int) string { return strings.Repeat(c, n) }

func TestParseWellFormed(t *testing.T) {
	in := "commit=" + repeat("a", 40) + "\ninstalled_at=1754700000\nbranch=main\nhostname=box\n"
	s, err := Parse(in)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if s.Branch != "main" || s.Hostname != "box" {
		t.Fatalf("got %+v", s)
	}
	if s.InstalledAt.Unix() != 1754700000 {
		t.Fatalf("InstalledAt = %v", s.InstalledAt)
	}
	if s.Commit != repeat("a", 40) {
		t.Fatalf("Commit = %q", s.Commit)
	}
}

func TestParseEmptyIsError(t *testing.T) {
	if _, err := Parse(""); err == nil {
		t.Fatal("expected an error for an empty stamp")
	}
}

func TestParseShortCommitIsError(t *testing.T) {
	if _, err := Parse("commit=abc\ninstalled_at=1754700000\n"); err == nil {
		t.Fatal("expected an error for a short commit sha")
	}
}

func TestParseMissingInstalledAtIsError(t *testing.T) {
	if _, err := Parse("commit=" + repeat("a", 40) + "\n"); err == nil {
		t.Fatal("expected an error when installed_at is missing")
	}
}

// A truncated write (power loss mid-install) must not look like a valid stamp.
func TestParseTruncatedIsError(t *testing.T) {
	if _, err := Parse("commit=" + repeat("a", 17)); err == nil {
		t.Fatal("expected an error for a truncated stamp")
	}
}

func TestParseIgnoresUnknownKeysAndBlankLines(t *testing.T) {
	in := "\ncommit=" + repeat("b", 40) + "\n\nfuture_key=whatever\ninstalled_at=1754700001\n"
	s, err := Parse(in)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if s.Commit != repeat("b", 40) {
		t.Fatalf("Commit = %q", s.Commit)
	}
}
