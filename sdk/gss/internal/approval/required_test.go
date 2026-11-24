package approval

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestTokenRequired(t *testing.T) {
	defer func(l func(string) (string, error), r func(...string) error) { lookPath, runGFF = l, r }(lookPath, runGFF)
	found := func(string) (string, error) { return "/bin/gff", nil }
	missing := func(string) (string, error) { return "", errors.New("not found") }
	// gff answers only the exact key declared in .github/gff/features.yaml
	// (TestFlagDeclared); any other question is an unknown key, exit 2.
	on := func(args ...string) error {
		if len(args) != 2 || args[0] != "enabled" || args[1] != "gss.approval-token.enabled" {
			return errors.New("exit status 2")
		}
		return nil
	}
	off := func(...string) error { return errors.New("exit status 1") }
	cases := []struct {
		name string
		env  string
		look func(string) (string, error)
		run  func(...string) error
		want bool
	}{
		{"default: no gff", "", missing, on, false},
		{"gff says on", "", found, on, true},
		{"gff says off or unknown flag", "", found, off, false},
		{"env on beats gff off", "on", found, off, true},
		{"env true", "TRUE", missing, off, true},
		{"env off cannot turn gff on off", "off", found, on, true},
		{"env 0 cannot either", "0", found, on, true},
		{"env off with gff off: off", "off", found, off, false},
		{"env junk falls through to gff", "maybe", found, on, true},
		{"env trimmed: ' on '", " on ", missing, off, true},
		{"env ' off ' cannot turn gff on off", " off ", found, on, true},
		{"only the ends are trimmed: 'o f f' falls through", "o f f", found, on, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(EnvOverride, c.env)
			lookPath, runGFF = c.look, c.run
			if got := TokenRequired(); got != c.want {
				t.Fatalf("TokenRequired() = %v, want %v", got, c.want)
			}
		})
	}
}

type noGit struct{}

func (noGit) Run(context.Context, string, ...string) ([]byte, error) {
	return nil, errors.New("git must not run when the token is off")
}

// With the token off, a gated verb proceeds with no token file and never
// reads HEAD.
func TestVerifyTokenOff(t *testing.T) {
	v := &Verifier{Path: filepath.Join(t.TempDir(), "approval.token"), Required: func() bool { return false }, Git: noGit{}}
	if err := v.Verify(context.Background(), ".", false); err != nil {
		t.Fatalf("Verify with the token off = %v", err)
	}
}

// NewVerifier wires TokenRequired, so the default (no override, gff off)
// reaches Verify, and an existing token file is left alone (neither read
// against HEAD nor consumed) while the gate is off.
func TestNewVerifierDefaultOff(t *testing.T) {
	defer func(l func(string) (string, error), r func(...string) error) { lookPath, runGFF = l, r }(lookPath, runGFF)
	lookPath = func(string) (string, error) { return "/bin/gff", nil }
	runGFF = func(...string) error { return errors.New("exit status 1") }
	t.Setenv(EnvOverride, "")
	path := filepath.Join(t.TempDir(), "approval.token")
	if err := os.WriteFile(path, []byte("stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	v := NewVerifier(path, noGit{})
	if err := v.Verify(context.Background(), ".", false); err != nil {
		t.Fatalf("Verify with the gate off = %v", err)
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != "stale\n" {
		t.Fatalf("token file changed while off: %q, %v", b, err)
	}
	t.Setenv(EnvOverride, "on")
	if err := NewVerifier(path, noGit{}).Verify(context.Background(), ".", false); err == nil {
		t.Fatal("GSS_REQUIRE_APPROVAL=on did not check the token")
	}
}

// No override and no gff: NewVerifier's default is off, so a gated verb
// runs without a token.
func TestNewVerifierUnsetDefault(t *testing.T) {
	defer func(l func(string) (string, error)) { lookPath = l }(lookPath)
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	t.Setenv(EnvOverride, "")
	v := NewVerifier(filepath.Join(t.TempDir(), "approval.token"), noGit{})
	if err := v.Verify(context.Background(), ".", false); err != nil {
		t.Fatalf("default (no override, no gff) = %v, want off", err)
	}
}
