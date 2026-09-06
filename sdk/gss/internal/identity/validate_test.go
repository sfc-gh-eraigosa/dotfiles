// Package identity_test verifies identifier validation and user resolution
// per sdk/gss/docs/plan.md PR-08: the segment regex for feature/user/
// purpose, purpose-vs-wordlist collision, --description cleaning (NFC,
// control-char rejection, code-point count), and user resolution
// precedence. Every reject path wraps errors.ErrInvalidIdent.
package identity_test

import (
	stderrors "errors"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/errors"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/identity"
)

func TestValidateSegment_Valid(t *testing.T) {
	for _, s := range []string{"ab", "api", "login", "feature-1", "a1", "x-y-z", "parallel-worktrees"} {
		if err := identity.ValidateFeature(s); err != nil {
			t.Errorf("ValidateFeature(%q) = %v; want nil", s, err)
		}
	}
}

func TestValidateSegment_Invalid(t *testing.T) {
	bad := []string{"", "a", "1ab", "-ab", "ab-", "AB", "a_b", "ab cd", strings.Repeat("a", 33)}
	for _, s := range bad {
		err := identity.ValidateUser(s)
		if err == nil {
			t.Errorf("ValidateUser(%q) = nil; want error", s)
			continue
		}
		if !stderrors.Is(err, errors.ErrInvalidIdent) {
			t.Errorf("ValidateUser(%q): err = %v; want wrapping ErrInvalidIdent", s, err)
		}
		var ve *errors.ValidationError
		if !stderrors.As(err, &ve) {
			t.Errorf("ValidateUser(%q): err type = %T; want *errors.ValidationError", s, err)
		}
	}
}

func TestValidatePurpose(t *testing.T) {
	for _, s := range []string{"api", "ui", "docs", "tests"} {
		if err := identity.ValidatePurpose(s); err != nil {
			t.Errorf("ValidatePurpose(%q) = %v; want nil", s, err)
		}
	}
	// A wordlist word is a valid segment grammatically but rejected as a
	// purpose (would alias a drawn suffix).
	for _, w := range []string{"moss", "oak", "fern"} {
		if err := identity.ValidatePurpose(w); err == nil {
			t.Errorf("ValidatePurpose(%q) = nil; want rejection (wordlist word)", w)
		} else if !stderrors.Is(err, errors.ErrInvalidIdent) {
			t.Errorf("ValidatePurpose(%q): err = %v; want wrapping ErrInvalidIdent", w, err)
		}
	}
}

func TestValidateDescription_Valid(t *testing.T) {
	got, err := identity.ValidateDescription("Refactor the login flow")
	if err != nil {
		t.Fatalf("ValidateDescription: %v", err)
	}
	if got != "Refactor the login flow" {
		t.Errorf("clean description altered: %q", got)
	}
}

func TestValidateDescription_NFC(t *testing.T) {
	// "e" + combining acute (2 code points) must NFC-fold to "é" (1).
	got, err := identity.ValidateDescription("é")
	if err != nil {
		t.Fatalf("ValidateDescription: %v", err)
	}
	if got != "é" {
		t.Errorf("NFC result = %q (% x); want é (U+00E9)", got, []rune(got))
	}
}

func TestValidateDescription_Strips(t *testing.T) {
	cases := map[string]string{
		"hi\x1b[31mred\x1b[0m":         "hired",      // ANSI stripped
		"todo <!-- gss:stack --> done": "todo  done", // marker stripped
		"a\nb\tc\rd":                   "abcd",       // newlines/tabs/CR stripped
	}
	for in, want := range cases {
		got, err := identity.ValidateDescription(in)
		if err != nil {
			t.Errorf("ValidateDescription(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ValidateDescription(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestValidateDescription_Rejects(t *testing.T) {
	// A bell control char (not whitespace, not space) is rejected.
	if _, err := identity.ValidateDescription("bad\x07bell"); err == nil {
		t.Error("control char: want rejection")
	}
	// Empty after stripping.
	if _, err := identity.ValidateDescription("\n\n"); err == nil {
		t.Error("empty-after-strip: want rejection")
	}
	if _, err := identity.ValidateDescription(""); err == nil {
		t.Error("empty: want rejection")
	}
	// Too long (>240 code points).
	if _, err := identity.ValidateDescription(strings.Repeat("a", 241)); err == nil {
		t.Error("241 runes: want rejection")
	}
}

func TestResolveUser_OverrideWins(t *testing.T) {
	got, err := identity.ResolveUser(identity.UserSources{
		Override: "erai",
		GHLogin:  func() (string, error) { return "ghlogin", nil },
	})
	if err != nil || got != "erai" {
		t.Errorf("ResolveUser(override) = %q, %v; want erai, nil", got, err)
	}
}

func TestResolveUser_InvalidOverrideErrors(t *testing.T) {
	_, err := identity.ResolveUser(identity.UserSources{Override: "BadUser"})
	if err == nil {
		t.Fatal("invalid --user: want error, not fall-through")
	}
	if !stderrors.Is(err, errors.ErrInvalidIdent) {
		t.Errorf("err = %v; want wrapping ErrInvalidIdent", err)
	}
}

func TestResolveUser_Precedence(t *testing.T) {
	// gh login present and valid → wins over email/$USER.
	got, err := identity.ResolveUser(identity.UserSources{
		GHLogin:  func() (string, error) { return "Erai-Bot", nil },
		GitEmail: func() (string, error) { return "jane.doe@example.com", nil },
		Getenv:   func(string) string { return "someuser" },
	})
	if err != nil || got != "erai-bot" {
		t.Errorf("precedence(gh) = %q, %v; want erai-bot, nil", got, err)
	}
}

func TestResolveUser_FallsThroughToEmail(t *testing.T) {
	got, err := identity.ResolveUser(identity.UserSources{
		GHLogin:  func() (string, error) { return "", stderrors.New("not authed") },
		GitEmail: func() (string, error) { return "jane.doe@example.com", nil },
		Getenv:   func(string) string { return "someuser" },
	})
	if err != nil || got != "jane-doe" {
		t.Errorf("fallthrough(email) = %q, %v; want jane-doe, nil", got, err)
	}
}

func TestResolveUser_FallsThroughToEnv(t *testing.T) {
	got, err := identity.ResolveUser(identity.UserSources{
		GHLogin:  func() (string, error) { return "x", nil }, // slugs to "x" → invalid (too short)
		GitEmail: func() (string, error) { return "", stderrors.New("no email") },
		Getenv:   func(k string) string { return map[string]string{"USER": "someuser"}[k] },
	})
	if err != nil || got != "someuser" {
		t.Errorf("fallthrough($USER) = %q, %v; want someuser, nil", got, err)
	}
}

func TestResolveUser_NothingResolves(t *testing.T) {
	_, err := identity.ResolveUser(identity.UserSources{
		Getenv: func(string) string { return "" },
	})
	if err == nil {
		t.Fatal("no sources: want error")
	}
	if !stderrors.Is(err, errors.ErrInvalidIdent) {
		t.Errorf("err = %v; want wrapping ErrInvalidIdent", err)
	}
}
