package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withConfig(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "gdash", "config.json")
	t.Setenv("GDASH_CONFIG", p)
	return p
}

func TestConfigVerbsEndToEnd(t *testing.T) {
	p := withConfig(t)
	if out, err := run(t, "config", "path"); err != nil || strings.TrimSpace(out) != p {
		t.Fatalf("config path = %q %v", out, err)
	}
	cred := filepath.Join(t.TempDir(), "lab.cred")
	if err := os.WriteFile(cred, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "config", "add", "profile", "lab", "--url", "http://192.0.2.128:30300", "--auth-file", cred); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "config", "add", "view", "ci", "--dashboard", "ci-lab", "--refresh", "15s"); err != nil {
		t.Fatal(err)
	}
	if out, _ := run(t, "config", "get", "default_profile"); strings.TrimSpace(out) != "lab" {
		t.Fatalf("first profile should become the default, got %q", out)
	}
	if _, err := run(t, "config", "set", "views.ci.vars.node", "All"); err != nil {
		t.Fatal(err)
	}
	if out, _ := run(t, "config", "get", "views.ci.vars.node"); strings.TrimSpace(out) != "All" {
		t.Fatalf("vars round trip = %q", out)
	}
	if _, err := run(t, "config", "remove", "view", "ci"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "config", "get", "views.ci.dashboard"); err == nil {
		t.Fatal("removed view still readable")
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o600 {
		t.Fatalf("config written %v", st.Mode().Perm())
	}
}

// E13: the CLI refuses an inline credential and writes nothing.
func TestConfigSetRefusesInlineCredential(t *testing.T) {
	p := withConfig(t)
	_, err := run(t, "config", "set", "profiles.lab.auth.token", "x")
	if err == nil || !strings.Contains(err.Error(), "auth.file") {
		t.Fatalf("inline credential: %v", err)
	}
	if _, statErr := os.Stat(p); statErr == nil {
		t.Fatal("a refused set still wrote the config file")
	}
}

func TestConfigSetRejectsInvalidResult(t *testing.T) {
	withConfig(t)
	if _, err := run(t, "config", "set", "views.ci.refresh", "soon"); err == nil {
		t.Fatal("an invalid config was saved")
	}
}

func TestConfigAddProfileNeedsOneSource(t *testing.T) {
	withConfig(t)
	if _, err := run(t, "config", "add", "profile", "lab", "--url", "http://h"); err == nil {
		t.Fatal("profile without a credential source accepted")
	}
}
