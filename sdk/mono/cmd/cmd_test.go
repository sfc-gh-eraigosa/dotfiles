package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/app"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/config"
)

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return out.String(), err
}

func withConfig(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("MONO_CONFIG", p)
	return p
}

func TestConfigVerbsEndToEnd(t *testing.T) {
	p := withConfig(t)
	if out, err := run(t, "config", "get", "panes.gsl.every"); err != nil || strings.TrimSpace(out) != "1s" {
		t.Fatalf("get on missing file = %q %v", out, err)
	}
	if _, err := run(t, "config", "set", "panes.gpu.cmd", "nvitop"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "config", "add", "logs", "--size", "15%", "--at", "1", "--", "journalctl", "-f"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "config", "move", "logs", "down"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "config", "remove", "gsl"); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, pn := range cfg.Panes {
		got = append(got, pn.Name+":"+pn.Cmd)
	}
	if strings.Join(got, ",") != "htop:htop,gpu:nvitop,logs:journalctl -f" {
		t.Fatalf("config after edits = %v", got)
	}
	if out, _ := run(t, "config", "path"); strings.TrimSpace(out) != p {
		t.Fatalf("path = %q", out)
	}
}

func TestConfigSetRejectsAndWritesNothing(t *testing.T) {
	p := withConfig(t)
	if _, err := run(t, "config", "set", "panes.htop.size", "0%"); err == nil || !strings.Contains(err.Error(), "panes.htop.size") {
		t.Fatalf("want a panes.htop.size error, got %v", err)
	}
	if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a rejected set wrote the config file")
	}
	if _, err := run(t, "config", "move", "htop", "sideways"); err == nil {
		t.Fatal("bad direction accepted")
	}
}

type fakeEnv struct{}

func (fakeEnv) LookPath(b string) (string, error) {
	if b == "htop" || b == "gsl" {
		return "/bin/" + b, nil
	}
	return "", errors.New("missing")
}
func (fakeEnv) Run(context.Context, string) bool { return false }
func (fakeEnv) DRMDrivers() []string             { return nil }

func TestDoctorExplainsEachPane(t *testing.T) {
	withConfig(t)
	prev := doctorEnv
	doctorEnv = fakeEnv{}
	defer func() { doctorEnv = prev }()
	out, err := run(t, "doctor", "--rows", "40")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"htop   60%   35", "(not shown)  nvtop not installed; nvitop not installed", "gsl    5%    3        gsl status"} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor output missing %q:\n%s", want, out)
		}
	}
}

func TestConfigArgvRefusesUnderTest(t *testing.T) {
	if argv := configArgv(); argv != nil {
		t.Fatalf("configArgv() = %v under go test; re-exec'ing a test binary runs the suite again", argv)
	}
}

// TestKeyTablesAgree: the README's key table, --help and the overlay all come
// from (or must match) app.Keys.
func TestKeyTablesAgree(t *testing.T) {
	readme, err := os.ReadFile("../README.md")
	if err != nil {
		t.Fatal(err)
	}
	help := keysHelp()
	for _, b := range app.Keys("ctrl+b") {
		keys := strings.Join(b.Keys, "/")
		if !strings.Contains(help, b.Help) {
			t.Errorf("--help is missing %q", b.Help)
		}
		row := "| `" + strings.ReplaceAll(keys, "/", "` `") + "` | " + b.Help + " |"
		if !strings.Contains(string(readme), row) {
			t.Errorf("README key table is missing the row %s", row)
		}
	}
}

func TestVersionJSON(t *testing.T) {
	out, err := run(t, "version", "--json")
	if err != nil || !strings.Contains(out, `"version":"dev"`) {
		t.Fatalf("version --json = %q %v", out, err)
	}
	versionJSON = false
}
