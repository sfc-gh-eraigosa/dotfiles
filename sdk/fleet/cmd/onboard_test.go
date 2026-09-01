package cmd

import (
	"os"
	"strings"
	"testing"
)

// A missing ~/.ssh/config must never be fatal. Before this, `fleet` on a fresh
// machine failed to start at all — and since bare `fleet` opens the dashboard,
// that was the very first thing a new user hit.
func TestMissingConfigIsAnEmptyFleetNotAnError(t *testing.T) {
	got, err := readConfig("/nonexistent/path/to/config")
	if err != nil {
		t.Fatalf("missing config returned an error: %v", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

// Onboarding must never prompt where nobody can answer: a script or CI run
// would hang forever waiting on stdin.
func TestOnboardNeverPromptsWithoutATerminal(t *testing.T) {
	if got := onboardDecision(0, false, false); got != onboardNone {
		t.Fatalf("non-interactive with no hosts = %v, want onboardNone", got)
	}
}

func TestOnboardOffersTheRightStep(t *testing.T) {
	for _, tc := range []struct {
		name        string
		hosts       int
		cfgExists   bool
		interactive bool
		want        onboardStep
	}{
		{"fresh machine, no config", 0, false, true, onboardOfferCreate},
		{"config exists but empty", 0, true, true, onboardOfferScan},
		{"already has hosts", 3, true, true, onboardNone},
		{"has hosts, no config file", 1, false, true, onboardNone},
	} {
		if got := onboardDecision(tc.hosts, tc.cfgExists, tc.interactive); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Look for an existing key ALWAYS before offering to make one — generating a
// second key when a perfectly good one is present is how a machine ends up with
// credentials nobody can account for.
func TestPickIdentityPrefersAnExistingKeyOverGenerating(t *testing.T) {
	for _, tc := range []struct {
		name    string
		present []string
		want    string
	}{
		{"modern key wins", []string{"id_rsa", "id_ed25519"}, "id_ed25519"},
		{"ecdsa beats rsa", []string{"id_rsa", "id_ecdsa"}, "id_ecdsa"},
		{"only rsa", []string{"id_rsa"}, "id_rsa"},
		{"an unconventional name still counts", []string{"github_key"}, "github_key"},
		{"nothing at all", nil, ""},
	} {
		if got := pickIdentity(tc.present); got != tc.want {
			t.Errorf("%s: pickIdentity(%v) = %q, want %q", tc.name, tc.present, got, tc.want)
		}
	}
}

// A conventional key must win over an unconventional one, so a stray test key
// does not shadow the real id_ed25519.
func TestPickIdentityPrefersConventionalNames(t *testing.T) {
	if got := pickIdentity([]string{"zz_random", "id_ed25519"}); got != "id_ed25519" {
		t.Fatalf("got %q, want id_ed25519", got)
	}
}

// The onboarding text has to say what it will do BEFORE it does it: this
// writes the file every other command depends on.
func TestOnboardMessageNamesTheFileAndTheAction(t *testing.T) {
	msg := onboardCreateMessage("/home/u/.ssh/config")
	for _, want := range []string{"/home/u/.ssh/config", "0600"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message %q missing %q", msg, want)
		}
	}
}

// /dev/null IS a character device, so the common ModeCharDevice idiom reports
// it as a terminal — and a script run with `</dev/null` would then be prompted
// at, print a question nobody asked for, and take the EOF as "no". Found by
// running it, not by review: the real check must ask the fd whether it is a
// tty, not what kind of file it is.
func TestDevNullIsNotATerminal(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Skip("no /dev/null")
	}
	defer f.Close()
	if isTerminal(f) {
		t.Fatal("/dev/null reported as a terminal — a script would be prompted at")
	}
}

func TestAPipeIsNotATerminal(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if isTerminal(r) {
		t.Fatal("a pipe reported as a terminal")
	}
}
