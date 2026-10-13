package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A step's preamble runs over a plain, non-login ssh command. On a host whose
// PATH is set only in an interactive rc (~/.zshrc), the per-user tool dirs are
// missing there, so `gff` — installed in ~/opt/bin — is "command not found":
// every flag-bound input failed with rcInputFlag and the step never ran
// (playground.converge on wslgiga1, 2026-10-11). The preamble must put the
// same dirs ProbePath gives `fleet status` on PATH, for its own gff calls and
// for the step script that runs after it in the same shell.
func TestInputPreambleFindsGffOutsideALoginShell(t *testing.T) {
	p := inputsPlan(t, false)
	st, _ := p.Step("pg.converge")
	vals := inputValues{
		run:  map[string]string{"k3s_join": "tok"},
		host: map[string]map[string]string{"h1": {"node": "spark1", "ollama": "true"}},
	}
	pre, err := inputPreamble(p, st, "h1", vals)
	if err != nil {
		t.Fatal(err)
	}

	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "pg"), 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(home, "opt", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(home, "gff.log")
	fake := "#!/bin/sh\necho \"$@\" >> " + log + "\n[ \"$1\" = get ] && echo false\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "gff"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}

	// What a non-login ssh command gets: system dirs only.
	c := exec.Command("sh", "-c", pre+"command -v gff")
	c.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin"}
	c.Dir = home
	c.Stdin = strings.NewReader(inputStdin(p, st, "h1", vals))
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("preamble failed without a login PATH: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), filepath.Join(bin, "gff")) {
		t.Errorf("the step script cannot find gff after the preamble; got %q", out)
	}
	got, _ := os.ReadFile(log)
	if !strings.Contains(string(got), "set converge.ollama.enabled true") {
		t.Errorf("gff set did not run through ~/opt/bin/gff; gff saw %q", got)
	}
}
