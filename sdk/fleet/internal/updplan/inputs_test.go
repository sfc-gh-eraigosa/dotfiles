package updplan

import (
	"strings"
	"testing"
)

// A plan can declare the values it needs before it can run: a per-host node
// name, a join token, a feature switch (#351 feature 3). Without this a step
// that needs one cannot get it at all — wireStep has no env: field, and the
// TUI's answer form is a hardcoded four.

func TestInputsParse(t *testing.T) {
	p, err := Parse([]byte(`
version: 1
update:
  inputs:
    - id: k3s_token
      prompt: "k3s join token"
      type: password
      scope: run
      env: K3S_TOKEN
      needed_by: [pg.converge]
    - id: node
      prompt: "this host's k3s node"
      scope: host
      env: CONVERGE_NODE
    - id: ollama
      prompt: "Run ollama here?"
      type: bool
      scope: host
      flag: converge.ollama.enabled
    - id: lane
      type: choice
      options: [fast, slow]
      default: fast
  repos:
    pg: {path: ~/pg, stamp: ~/.local/state/pg/install-stamp}
  steps:
    - {id: pg.sync, kind: sync, repo: pg}
    - {id: pg.converge, kind: run, repo: pg, run: ./install.sh, needs: [pg.sync]}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Inputs) != 4 {
		t.Fatalf("got %d inputs", len(p.Inputs))
	}

	tok := p.Inputs[0]
	if !tok.Secret {
		t.Error("type: password must imply secret — it is the whole reason to declare it that way")
	}
	if tok.Scope != ScopeRun || tok.Env != "K3S_TOKEN" {
		t.Errorf("token = %+v", tok)
	}
	if len(tok.NeededBy) != 1 || tok.NeededBy[0] != "pg.converge" {
		t.Errorf("needed_by = %v", tok.NeededBy)
	}

	node := p.Inputs[1]
	if node.Scope != ScopeHost {
		t.Errorf("node scope = %v, want host", node.Scope)
	}
	if node.Secret {
		t.Error("a plain text input must not be secret")
	}

	if got := p.Inputs[2].Flag; got != "converge.ollama.enabled" {
		t.Errorf("flag = %q", got)
	}
	if p.Inputs[2].Type != InputBool {
		t.Errorf("type = %v", p.Inputs[2].Type)
	}

	// env defaults to the upshouted id, so the common case needs no `env:`.
	if got := p.Inputs[3].Env; got != "LANE" {
		t.Errorf("default env = %q, want LANE", got)
	}
}

func TestInputsValidation(t *testing.T) {
	cases := []struct{ name, yaml, want string }{
		{"duplicate id", "\n    - {id: a}\n    - {id: a}\n", "duplicate"},
		{"bad id", "\n    - {id: \"No-Dashes\"}\n", "id"},
		{"unknown type", "\n    - {id: a, type: quantum}\n", "type"},
		{"unknown scope", "\n    - {id: a, scope: galaxy}\n", "scope"},
		{"choice without options", "\n    - {id: a, type: choice}\n", "options"},
		{"default not an option", "\n    - {id: a, type: choice, options: [x], default: y}\n", "default"},
		{"needed_by unknown step", "\n    - {id: a, needed_by: [nope]}\n", "needed_by"},
		{"bad env name", "\n    - {id: a, env: \"lower case\"}\n", "env"},
		// A secret must never be written into a gff config file — that is a
		// plain-text store, and the whole point of `secret:` is that the value
		// only ever lives in memory and on stdin.
		{"secret bound to a flag", "\n    - {id: a, type: password, flag: x.y.z}\n", "secret"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse([]byte("version: 1\nupdate:\n  inputs:" + c.yaml +
				"  repos:\n    r: {path: ~/r}\n  steps:\n    - {id: s, kind: sync, repo: r}\n"))
			if err == nil {
				t.Fatalf("accepted an invalid input (%s)", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q does not mention %q", err, c.want)
			}
		})
	}
}

func TestInputsForStep(t *testing.T) {
	p, err := Parse([]byte(`
version: 1
update:
  inputs:
    - {id: everywhere}
    - {id: only_converge, needed_by: [pg.converge]}
  repos:
    pg: {path: ~/pg}
  steps:
    - {id: pg.sync, kind: sync, repo: pg}
    - {id: pg.converge, kind: run, repo: pg, run: ./install.sh, needs: [pg.sync]}
`))
	if err != nil {
		t.Fatal(err)
	}
	conv, _ := p.Step("pg.converge")
	sync, _ := p.Step("pg.sync")

	// No needed_by = every RUN step. A sync step runs git, not the plan's
	// script, so handing it the plan's values would be noise at best.
	got := ids(p.InputsFor(conv))
	if got != "everywhere,only_converge" {
		t.Fatalf("converge inputs = %q", got)
	}
	if got := ids(p.InputsFor(sync)); got != "" {
		t.Fatalf("sync step got inputs %q, want none", got)
	}
}

func ids(in []Input) string {
	var b []string
	for _, i := range in {
		b = append(b, i.ID)
	}
	return strings.Join(b, ",")
}

// --- friendliness: describe it, find it, check it -------------------------

func TestInputDescriptionAndDiscovery(t *testing.T) {
	p, err := Parse([]byte(`
version: 1
update:
  inputs:
    - id: node
      prompt: "which k3s node is this host?"
      description: |
        The name this machine is registered as in k3s/cluster.yaml. Registry
        entries target it through placement.node, so it must match exactly.
      scope: host
      env: CONVERGE_NODE
      default_from: make -s -C ~/pg k3s-node
      validate: '^[a-z][a-z0-9]*$'
  repos:
    pg: {path: ~/pg}
  steps:
    - {id: s, kind: run, repo: pg, run: ./install.sh}
`))
	if err != nil {
		t.Fatal(err)
	}
	in := p.Inputs[0]
	if !strings.Contains(in.Description, "placement.node") {
		t.Errorf("description = %q", in.Description)
	}
	if in.DefaultFrom != "make -s -C ~/pg k3s-node" {
		t.Errorf("default_from = %q", in.DefaultFrom)
	}
	if in.Validate == nil || !in.Validate.MatchString("jetson1") || in.Validate.MatchString("Jetson-1") {
		t.Errorf("validate regexp did not compile as expected: %v", in.Validate)
	}
	// An input that can find its own value is not something the operator has
	// to be asked for.
	if !in.HasSource() {
		t.Error("an input with default_from must report that it has a source")
	}
}

func TestInputValidationOfTheNewFields(t *testing.T) {
	cases := []struct{ name, yaml, want string }{
		{"bad regexp", "\n    - {id: a, validate: \"([\"}\n", "validate"},
		{"default fails its own validate", "\n    - {id: a, default: \"ZZZ\", validate: \"^[a-z]+$\"}\n", "default"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse([]byte("version: 1\nupdate:\n  inputs:" + c.yaml +
				"  repos:\n    r: {path: ~/r}\n  steps:\n    - {id: s, kind: run, repo: r, run: ./x}\n"))
			if err == nil {
				t.Fatalf("accepted %s", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q does not mention %q", err, c.want)
			}
		})
	}
}
