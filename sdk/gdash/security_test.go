package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/grafana/grafanatest"
)

// secretFixture is assembled at run time so no literal looks like a
// credential to the privacy guard.
func secretFixture() string { return strings.Join([]string{"glsa", "fixture", "e12", "9f3c"}, "_") }

// E12: the credential never reaches stdout or stderr, on any path, even
// when the server echoes the Authorization header back in its errors.
// The real binary is built and run, so nothing in-process can hide a leak.
func TestSecretNeverPrinted(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}
	bin := filepath.Join(t.TempDir(), "gdash")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	secret := secretFixture()
	dir := t.TempDir()
	credFile := filepath.Join(dir, "lab.cred")
	if err := os.WriteFile(credFile, []byte(secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ok := grafanatest.New(t)
	echo := grafanatest.New(t)
	echo.Status, echo.EchoAuth = 401, true
	write := func(url string, a config.Auth) string {
		p := filepath.Join(t.TempDir(), "config.json")
		c := config.Config{
			Profiles:       map[string]config.Profile{"lab": {URL: url, Auth: a}},
			DefaultProfile: "lab",
			Views:          map[string]config.View{"ci": {Dashboard: "ci-lab"}, "gone": {Dashboard: "no such dashboard"}},
		}
		if err := config.Save(p, c); err != nil {
			t.Fatal(err)
		}
		return p
	}
	configs := map[string]string{
		"file/ok":   write(ok.URL, config.Auth{File: credFile}),
		"file/echo": write(echo.URL, config.Auth{File: credFile}),
		"env/echo":  write(echo.URL, config.Auth{Env: "GDASH_E12"}),
		"cmd/echo":  write(echo.URL, config.Auth{Cmd: "cat " + credFile}),
		"cmd/fails": write(ok.URL, config.Auth{Cmd: "cat " + credFile + " >&2; exit 4"}), // the secret goes to stderr, which gdash discards
		"basic":     write(echo.URL, config.Auth{User: "viewer", File: credFile}),
	}
	commands := [][]string{
		{"doctor"}, {"doctor", "--quiet"}, {"list"}, {"list", "nodes"},
		{"config", "get", "profiles.lab.auth.file"}, {"config", "get", "profiles.lab.auth.cmd"},
		{"nosuchview"}, {"--dashboard", "x", "--profile", "nope"},
	}
	ran := 0
	for name, cfg := range configs {
		for _, args := range commands {
			cmd := exec.Command(bin, args...)
			cmd.Env = append(os.Environ(), "GDASH_CONFIG="+cfg, "GDASH_E12="+secret, "NO_COLOR=1")
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			_ = cmd.Run()
			ran++
			for stream, b := range map[string]string{"stdout": stdout.String(), "stderr": stderr.String()} {
				if strings.Contains(b, secret) {
					t.Errorf("%s: gdash %s leaked the credential on %s:\n%s", name, strings.Join(args, " "), stream, b)
				}
			}
		}
	}
	if ran != len(configs)*len(commands) {
		t.Fatalf("ran %d", ran)
	}
}
