package approval

import (
	"context"
	"errors"
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
