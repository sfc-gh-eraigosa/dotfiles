package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

func inputsPlan(t *testing.T, interactive bool) updplan.Plan {
	t.Helper()
	conv := "    - {id: pg.converge, kind: run, repo: pg, run: ./install.sh, needs: [pg.sync]"
	if interactive {
		conv += ", interactive: true"
	}
	conv += "}"
	p, err := updplan.Parse([]byte(`
version: 1
update:
  inputs:
    - {id: node, scope: host, env: CONVERGE_NODE}
    - {id: k3s_join, type: password, scope: run, env: K3S_TOKEN, needed_by: [pg.converge]}
    - {id: ollama, type: bool, scope: host, flag: converge.ollama.enabled}
  repos:
    pg: {path: ~/pg, stamp: ~/.local/state/pg/install-stamp}
  steps:
    - {id: pg.sync, kind: sync, repo: pg}
` + conv + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestInputPreambleExportsAndSetsFlags(t *testing.T) {
	p := inputsPlan(t, false)
	st, _ := p.Step("pg.converge")
	fake := "s3" + "kr1t"
	vals := inputValues{
		run:  map[string]string{},
		host: map[string]map[string]string{"h1": {"node": "spark1", "ollama": "true"}},
	}
	vals.run["k3s_join"] = fake
	pre, err := inputPreamble(p, st, "h1", vals)
	if err != nil {
		t.Fatal(err)
	}

	// Non-secret values are exported. MUST be `export`, never the `VAR=x cmd`
	// prefix form, which scopes to the `cd` and never reaches the script.
	if !strings.Contains(pre, "export CONVERGE_NODE='spark1';") {
		t.Errorf("preamble does not export the host value: %q", pre)
	}
	// A confidential value is READ FROM STDIN, never written into the command.
	if strings.Contains(pre, fake) {
		t.Fatalf("a confidential value reached the command text: %q", pre)
	}
	if !strings.Contains(pre, "read -r") || !strings.Contains(pre, "export K3S_TOKEN=") {
		t.Errorf("preamble does not read the confidential value from stdin: %q", pre)
	}
	// A flag-bound answer becomes gff state on the host, in that repo.
	if !strings.Contains(pre, "gff set 'converge.ollama.enabled' 'true'") {
		t.Errorf("preamble does not set the gff flag: %q", pre)
	}
	// The repo path must be UNQUOTED — `cd '~/pg'` looks for a directory
	// literally named "~", which is how this bug presented the first time.
	if !strings.Contains(pre, "cd ~/pg && gff set") {
		t.Errorf("gff set must cd, unquoted, into the repo that declares the flag: %q", pre)
	}
	// Every producer terminates its own text into valid shell.
	if !strings.HasSuffix(pre, "; ") && !strings.HasSuffix(pre, "&& ") {
		t.Errorf("preamble is not terminated: %q", pre)
	}

	// The stdin payload carries the confidential values, in read order.
	if got := inputStdin(p, st, "h1", vals); got != fake+"\n" {
		t.Errorf("stdin = %q", got)
	}
	// A step that needs none gets neither.
	sync, _ := p.Step("pg.sync")
	if pre, err := inputPreamble(p, sync, "h1", vals); err != nil || pre != "" {
		t.Errorf("sync step preamble = %q (%v)", pre, err)
	}
	if got := inputStdin(p, sync, "h1", vals); got != "" {
		t.Errorf("sync step stdin = %q", got)
	}
}

func TestInputPreambleRefusesAConfidentialValueOnAnInteractiveStep(t *testing.T) {
	// An interactive step hands the terminal to the operator over `ssh -t`,
	// so stdin IS the terminal — there is nowhere to write a secret that is
	// not argv. Say so rather than silently exporting it into the command.
	p := inputsPlan(t, true)
	st, _ := p.Step("pg.converge")
	if !st.Interactive {
		t.Fatal("fixture is not interactive")
	}
	_, err := inputPreamble(p, st, "h1", inputValues{run: map[string]string{"k3s_join": "x"}})
	if err == nil {
		t.Fatal("a confidential input on an interactive step was accepted")
	}
	if !strings.Contains(err.Error(), "interactive") {
		t.Fatalf("error %q does not explain why", err)
	}
}

func TestParseInputFlagFormsAndSecretSafety(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "tok")
	if err := os.WriteFile(file, []byte("from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLEET_TEST_TOKEN", "from-env")
	p := inputsPlan(t, false)

	vals, err := parseInputFlags(p, []string{
		"h1:node=spark1",
		"k3s_join=@" + file,
		"h2:ollama=false",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := vals.host["h1"]["node"]; got != "spark1" {
		t.Errorf("host value = %q", got)
	}
	if got := vals.run["k3s_join"]; got != "from-file" {
		t.Errorf("@file value = %q (trailing newline must be trimmed)", got)
	}
	if got := vals.host["h2"]["ollama"]; got != "false" {
		t.Errorf("bool value = %q", got)
	}

	if vals, err := parseInputFlags(p, []string{"k3s_join=env:FLEET_TEST_TOKEN"}); err != nil {
		t.Fatal(err)
	} else if vals.run["k3s_join"] != "from-env" {
		t.Errorf("env: value = %q", vals.run["k3s_join"])
	}

	// A literal secret on the command line is world-readable in /proc for as
	// long as the process lives. Refuse it and name the safe forms.
	_, err = parseInputFlags(p, []string{"k3s_join=literal"})
	if err == nil {
		t.Fatal("a literal confidential value on argv was accepted")
	}
	for _, want := range []string{"@", "env:"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name the %q form", err, want)
		}
	}

	for _, bad := range []string{"nosuch=x", "node", "h1:nosuch=x"} {
		if _, err := parseInputFlags(p, []string{bad}); err == nil {
			t.Errorf("accepted bad --input %q", bad)
		}
	}
}

func TestMissingInputsAreNamed(t *testing.T) {
	p := inputsPlan(t, false)
	missing := missingInputs(p, []string{"h1", "h2"}, inputValues{
		run:  map[string]string{},
		host: map[string]map[string]string{"h1": {"node": "spark1", "ollama": "true"}},
	})
	// k3s_join (run) once; node+ollama for h2 only.
	joined := strings.Join(missing, " ")
	for _, want := range []string{"k3s_join", "h2:node", "h2:ollama"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing list %v does not name %q", missing, want)
		}
	}
	if strings.Contains(joined, "h1:node") {
		t.Errorf("an answered input was reported missing: %v", missing)
	}
}

func TestDryRunShowsTheInputPreambleWithoutTheSecret(t *testing.T) {
	// --dry-run is the trust boundary: it must show that a `gff set` will
	// CHANGE the host, and that a value arrives from stdin — while never
	// printing the confidential value itself.
	p := inputsPlan(t, false)
	fake := "s3" + "kr1t"
	vals := inputValues{
		run:  map[string]string{},
		host: map[string]map[string]string{"h1": {"node": "spark1", "ollama": "true"}},
	}
	vals.run["k3s_join"] = fake

	var b strings.Builder
	if err := printDryRunFor(&b, p, "", false, "h1", vals); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if strings.Contains(out, fake) {
		t.Fatalf("--dry-run printed a confidential value:\n%s", out)
	}
	for _, want := range []string{
		"export CONVERGE_NODE='spark1'",
		"gff set 'converge.ollama.enabled' 'true'",
		"read -r __fleet_in_k3s_join",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("--dry-run does not show %q:\n%s", want, out)
		}
	}
}
