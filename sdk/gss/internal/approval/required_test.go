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
	on := func(args ...string) error {
		if len(args) != 2 || args[0] != "enabled" || args[1] != FlagKey {
			t.Fatalf("gff args = %v", args)
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
		{"env off beats gff on", "off", found, on, false},
		{"env 0", "0", found, on, false},
		{"env junk falls through to gff", "maybe", found, on, true},
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

// NewVerifier wires TokenRequired, so GSS_REQUIRE_APPROVAL=off reaches Verify,
// and an existing token file is left alone (neither read against HEAD nor
// consumed) while the gate is off.
func TestNewVerifierDefaultOff(t *testing.T) {
	t.Setenv(EnvOverride, "off")
	path := filepath.Join(t.TempDir(), "approval.token")
	if err := os.WriteFile(path, []byte("stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	v := NewVerifier(path, noGit{})
	if err := v.Verify(context.Background(), ".", false); err != nil {
		t.Fatalf("Verify with GSS_REQUIRE_APPROVAL=off = %v", err)
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != "stale\n" {
		t.Fatalf("token file changed while off: %q, %v", b, err)
	}
	t.Setenv(EnvOverride, "on")
	if err := NewVerifier(path, noGit{}).Verify(context.Background(), ".", false); err == nil {
		t.Fatal("GSS_REQUIRE_APPROVAL=on did not check the token")
	}
}
