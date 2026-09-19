package cmd

import (
	"os"
	"os/exec"
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

// --- pre-fill from the host, and post-fill checks --------------------------

func discoveryPlan(t *testing.T) updplan.Plan {
	t.Helper()
	p, err := updplan.Parse([]byte(`
version: 1
update:
  inputs:
    - id: node
      prompt: "which k3s node is this host?"
      description: "The name this machine is registered as in k3s/cluster.yaml."
      scope: host
      env: CONVERGE_NODE
      default_from: make -s -C ~/pg k3s-node
      validate: '^[a-z][a-z0-9]*$'
    - id: joinkey
      type: password
      env: K3S_TOKEN
      default_from: sudo -n cat /var/lib/rancher/k3s/server/node-token
    - id: lane
      default: fast
  repos:
    pg: {path: ~/pg}
  steps:
    - {id: s, kind: run, repo: pg, run: ./install.sh}
`))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAnInputThatTheHostCanAnswerIsNotAskedFor(t *testing.T) {
	p := discoveryPlan(t)
	if missing := missingInputs(p, []string{"h1"}, inputValues{}); len(missing) != 0 {
		t.Fatalf("an operator was asked for values the host can find: %v", missing)
	}
}

func TestDiscoveredValuesAreComputedOnTheHost(t *testing.T) {
	p := discoveryPlan(t)
	st, _ := p.Step("s")
	pre, err := inputPreamble(p, st, "h1", inputValues{})
	if err != nil {
		t.Fatal(err)
	}

	// The command runs in the remote shell and its output becomes the value.
	if !strings.Contains(pre, "make -s -C ~/pg k3s-node") {
		t.Errorf("preamble does not run the discovery command: %q", pre)
	}
	if !strings.Contains(pre, "export CONVERGE_NODE") {
		t.Errorf("preamble does not export the discovered value: %q", pre)
	}
	// Discovery that finds nothing must FAIL the step, not export an empty
	// string and let the script converge the wrong thing.
	if !strings.Contains(pre, "exit 95") {
		t.Errorf("preamble does not fail when discovery comes up empty: %q", pre)
	}
	// The post-fill check: a discovered value is held to the same rule.
	if !strings.Contains(pre, "grep -qE") {
		t.Errorf("a discovered value is not validated on the host: %q", pre)
	}

	// A confidential value discovered on the host never travels at all —
	// nothing on stdin, nothing in the command but the lookup itself.
	if !strings.Contains(pre, "node-token") {
		t.Errorf("the confidential lookup is not in the preamble: %q", pre)
	}
	if got := inputStdin(p, st, "h1", inputValues{}); got != "" {
		t.Errorf("a host-discovered confidential value still travelled: %q", got)
	}

	// A static default needs no command at all.
	if !strings.Contains(pre, "export LANE='fast'") {
		t.Errorf("static default not applied: %q", pre)
	}
}

func TestAnOperatorValueBeatsDiscovery(t *testing.T) {
	p := discoveryPlan(t)
	st, _ := p.Step("s")
	vals := inputValues{host: map[string]map[string]string{"h1": {"node": "spark1"}}}
	pre, err := inputPreamble(p, st, "h1", vals)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pre, "export CONVERGE_NODE='spark1'") {
		t.Errorf("the supplied value was not used: %q", pre)
	}
	if strings.Contains(pre, "k3s-node") {
		t.Errorf("discovery still ran even though a value was supplied: %q", pre)
	}
}

func TestSuppliedValuesAreCheckedBeforeTheFleetIsTouched(t *testing.T) {
	p := discoveryPlan(t)
	_, err := parseInputFlags(p, []string{"h1:node=NOT-A-NODE"})
	if err == nil {
		t.Fatal("a value that fails its own validate rule was accepted")
	}
	if !strings.Contains(err.Error(), "validate") && !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("error %q does not explain the rule", err)
	}
	// ...and a good one still passes.
	if _, err := parseInputFlags(p, []string{"h1:node=jetson1"}); err != nil {
		t.Fatalf("a valid value was rejected: %v", err)
	}
}

func TestListInputsExplainsEachValue(t *testing.T) {
	p := discoveryPlan(t)
	var b strings.Builder
	listInputs(&b, p)
	out := b.String()
	for _, want := range []string{
		"node", "which k3s node is this host?",
		"registered as in k3s/cluster.yaml", // the description
		"per host", "found on the host",     // scope and where it comes from
		"joinkey", "confidential",
		"lane", "fast",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("--list-inputs output does not mention %q:\n%s", want, out)
		}
	}
}

func TestGeneratedShellSurvivesAwkwardText(t *testing.T) {
	// Every message fleet builds ends up inside the remote command, so any
	// plan text that reaches one has to be quoted as a WHOLE unit. Embedding
	// an already-quoted fragment inside a quoted echo pushes it back OUT of
	// the quotes, where the shell expands it — a regexp full of $ and * is
	// the worst possible thing to hand an unsuspecting shell.
	p, err := updplan.Parse([]byte(`
version: 1
update:
  inputs:
    - id: awkward
      env: AWKWARD
      default_from: "printf '%s' \"it's-here\""
      validate: "^it's-[a-z]+$"
    - id: flagged
      type: bool
      flag: "a.b.c"
  repos:
    r: {path: ~/r}
  steps:
    - {id: s, kind: run, repo: r, run: ./x.sh}
`))
	if err != nil {
		t.Fatal(err)
	}
	st, _ := p.Step("s")
	vals := inputValues{run: map[string]string{"flagged": "true"}}
	pre, err := inputPreamble(p, st, "h1", vals)
	if err != nil {
		t.Fatal(err)
	}

	// sh -n parses without running: the one check that actually proves the
	// generated text is valid shell rather than merely looking like it.
	cmd := exec.Command("sh", "-n")
	cmd.Stdin = strings.NewReader(pre + "true\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated preamble is not valid shell: %v\n%s\n--- preamble ---\n%s", err, out, pre)
	}
}

// --- the fixes: defaults for flags, up-front refusals, fail-loud lanes ----

func TestAFlagInputHonoursItsDefaultWithGffSet(t *testing.T) {
	// A flag-bound input's default must land where an answer lands — in the
	// host's gff state — not be exported as an env var nobody reads.
	p, err := updplan.Parse([]byte(`
version: 1
update:
  inputs:
    - {id: ollama, type: bool, flag: converge.ollama.enabled, default: "false"}
    - {id: gpu, type: bool, flag: converge.gpu.enabled, default_from: "test -e /dev/nvidia0 && echo true || echo false"}
  repos:
    pg: {path: ~/pg}
  steps:
    - {id: s, kind: run, repo: pg, run: ./install.sh}
`))
	if err != nil {
		t.Fatal(err)
	}
	st, _ := p.Step("s")
	pre, err := inputPreamble(p, st, "h1", inputValues{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pre, "gff set 'converge.ollama.enabled' 'false'") {
		t.Errorf("the static default did not become gff state: %q", pre)
	}
	if strings.Contains(pre, "export OLLAMA=") {
		t.Errorf("a flag input's default was exported as an env var instead: %q", pre)
	}
	// A discovered value sets the flag to what the host found.
	if !strings.Contains(pre, `gff set 'converge.gpu.enabled' "$GPU"`) {
		t.Errorf("the discovered value did not reach gff set: %q", pre)
	}
}

func TestUndeliverableInputsAreRefusedUpFront(t *testing.T) {
	// A flag input on a run step with no repo: gff resolves a flag from a
	// checkout, so there is nowhere to set it. Refused before a host is
	// contacted — the live lane must never find this out mid-fleet.
	p, err := updplan.Parse([]byte(`
version: 1
update:
  inputs:
    - {id: gpu, type: bool, flag: k3s.gpu}
  repos:
    pg: {path: ~/pg}
  steps:
    - {id: s, kind: run, run: ./bootstrap.sh}
`))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateInputDelivery(p, inputValues{run: map[string]string{"gpu": "true"}}); err == nil {
		t.Fatal("a flag input on a repo-less step was accepted")
	}
	// And should a lane ever reach the preamble with such a plan anyway,
	// the step FAILS with the reason rather than running without its inputs.
	st, _ := p.Step("s")
	pre := inputPreambleOrFail(p, st, "h1", inputValues{run: map[string]string{"gpu": "true"}})
	if !strings.Contains(pre, "exit 94") || !strings.Contains(pre, "targets no repo") {
		t.Errorf("an undeliverable input did not fail the step loudly: %q", pre)
	}
}

func TestASecretTheHostFindsIsAllowedOnAnInteractiveStep(t *testing.T) {
	// README's own pattern: a join token read from the host's disk. Nothing
	// travels, so an interactive step is no obstacle — unless the operator
	// actually supplies a value, which would then need a channel.
	p, err := updplan.Parse([]byte(`
version: 1
update:
  inputs:
    - {id: joinkey, type: password, env: K3S_TOKEN, default_from: "sudo -n cat /var/lib/rancher/k3s/server/node-token"}
  repos:
    pg: {path: ~/pg}
  steps:
    - {id: s, kind: run, repo: pg, run: ./install.sh, interactive: true}
`))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateInputDelivery(p, inputValues{}); err != nil {
		t.Fatalf("a host-discovered secret was refused on an interactive step: %v", err)
	}
	if err := validateInputDelivery(p, inputValues{run: map[string]string{"joinkey": "x"}}); err == nil {
		t.Fatal("a supplied secret for an interactive step was accepted")
	}
}

func TestBoolInputsAreCheckedBeforeTheFleetIsTouched(t *testing.T) {
	p := inputsPlan(t, false)
	if _, err := parseInputFlags(p, []string{"h1:ollama=yes"}); err == nil {
		t.Fatal("a non-boolean value for a bool input was accepted")
	}
	var b strings.Builder
	listInputs(&b, p)
	if !strings.Contains(b.String(), "true, false") {
		t.Errorf("--list-inputs does not name the bool literals:\n%s", b.String())
	}
}

func TestDryRunPrintsOnceUnlessAnInputIsPerHost(t *testing.T) {
	// A plan with no per-host input previews identically on every host, so
	// it is printed once — a twelve-host fleet must not get twelve copies.
	plain, err := updplan.Parse([]byte("version: 1\nupdate:\n  inputs:\n    - {id: lane, default: fast}\n  repos:\n    r: {path: ~/r}\n  steps:\n    - {id: s, kind: run, repo: r, run: ./x.sh}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if plain.HasHostInputs() {
		t.Fatal("no input is per host here")
	}
	if !inputsPlan(t, false).HasHostInputs() {
		t.Fatal("the inputs fixture declares per-host inputs")
	}
}
