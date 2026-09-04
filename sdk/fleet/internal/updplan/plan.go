// Package updplan is the pure, host-blind representation of a `fleet update`
// plan: the fleet.yaml schema, its validation rules, defaults merge, backoff
// math, and the topological order the executor walks. Nothing here touches a
// network, a filesystem, or a clock — every impure edge (the YAML bytes, the
// jitter source) is a parameter, never an ambient call.
package updplan

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Kind is the step kind.
type Kind string

const (
	KindSync   Kind = "sync"
	KindRun    Kind = "run"
	KindGhAuth Kind = "gh-auth"
)

// OnFailure controls whether a step's dependents run after it fails.
type OnFailure string

const (
	OnFailureStop     OnFailure = "stop"
	OnFailureContinue OnFailure = "continue"
)

// Local is a repo's local-changes policy.
type Local string

const (
	LocalSkip   Local = "skip"
	LocalRescue Local = "rescue"
	LocalCarry  Local = "carry"
)

// RetryOn names a failure class a step should be retried for: "transport",
// "timeout", "any", or "exit:<n>" for a specific exit code.
type RetryOn string

const (
	RetryOnTransport RetryOn = "transport"
	RetryOnTimeout   RetryOn = "timeout"
	RetryOnAny       RetryOn = "any"
)

// Backoff is an exponential backoff schedule with an optional +/-50% jitter.
type Backoff struct {
	Initial time.Duration
	Max     time.Duration
	Factor  float64
	Jitter  bool
}

// Wait returns how long to sleep before the attempt after the n-th failed
// one (n is 1-based). rnd, when non-nil, must return a value in [0,1); nil
// is treated as returning 0.5 (the midpoint, i.e. no jitter movement).
func (b Backoff) Wait(n int, rnd func() float64) time.Duration {
	if n < 1 {
		n = 1
	}
	f := 1.0
	for i := 0; i < n-1; i++ {
		f *= b.Factor
	}
	d := time.Duration(float64(b.Initial) * f)
	if b.Max > 0 && d > b.Max {
		d = b.Max
	}
	if b.Jitter {
		r := 0.5
		if rnd != nil {
			r = rnd()
		}
		d = time.Duration(float64(d) * (0.5 + r))
	}
	return d
}

// Retry is a step or default retry policy.
type Retry struct {
	Attempts int
	On       []RetryOn
	Backoff  Backoff
}

// Defaults are the plan-wide defaults every step's timeout/retry merge over.
type Defaults struct {
	Timeout time.Duration
	Retry   Retry
}

// Expect names the exit codes that count as success. Empty means [0].
type Expect struct {
	Exit []int
}

// Step is one node in the plan's DAG.
type Step struct {
	ID          string
	Kind        Kind
	Repo        string
	Run         string
	Interactive bool
	Needs       []string
	Expect      Expect
	OnFailure   OnFailure
	Hostname    string
	Timeout     time.Duration // resolved: defaults merged in by Parse
	Retry       Retry         // resolved: defaults merged in by Parse
}

// Repo is one repository the plan knows how to sync. Path is always the
// REMOTE path after Parse: absolute or "~/…", never relative.
type Repo struct {
	Name     string
	Path     string
	URL      string
	Branches []string
	Local    Local
	Restore  bool
}

// Plan is a fully parsed, defaulted, validated, path-resolved fleet.yaml.
type Plan struct {
	Root     string
	Defaults Defaults
	Repos    map[string]Repo
	Steps    []Step
	Source   string
}

// builtinDefaults are the plan-wide defaults per spec F1: 30m timeout, one
// attempt, retried only on a transport failure, 5s/2x/2m capped/jittered.
func builtinDefaults() Defaults {
	return Defaults{
		Timeout: 30 * time.Minute,
		Retry: Retry{
			Attempts: 1,
			On:       []RetryOn{RetryOnTransport},
			Backoff: Backoff{
				Initial: 5 * time.Second,
				Max:     2 * time.Minute,
				Factor:  2,
				Jitter:  true,
			},
		},
	}
}

// DefaultYAML is the commented starter plan `fleet update init` writes. It
// parses to exactly Default() (modulo Source, which the caller sets).
const DefaultYAML = "# fleet.yaml — the fleet-update plan.\n" +
	"# With no repos/steps overridden this is byte-for-byte today's `fleet update`:\n" +
	"# fetch+ff-only dotfiles under ~/git, then re-run ./install.sh interactively.\n" +
	"version: 1\n" +
	"update:\n" +
	"  # root: where relative repo paths resolve (repos.<name>.path defaults to <name>).\n" +
	"  root: ~/git\n" +
	"  repos:\n" +
	"    dotfiles:\n" +
	"      path: dotfiles\n" +
	"      branches: [main]\n" +
	"      # local: skip|rescue|carry (default skip); restore: true|false (default true)\n" +
	"  steps:\n" +
	"    - id: dotfiles.sync\n" +
	"      kind: sync\n" +
	"      repo: dotfiles\n" +
	"    - id: dotfiles.install\n" +
	"      kind: run\n" +
	"      repo: dotfiles\n" +
	"      run: ./install.sh\n" +
	"      interactive: true\n" +
	"      needs: [dotfiles.sync]\n"

var defaultPlan = mustParse(DefaultYAML)

func mustParse(data string) Plan {
	p, err := Parse([]byte(data))
	if err != nil {
		panic(fmt.Sprintf("updplan: DefaultYAML failed to parse: %v", err))
	}
	return p
}

// Default returns the built-in plan: today's `fleet update` behaviour,
// exactly as Parse(DefaultYAML) produces it.
func Default() Plan {
	return defaultPlan
}

// Step looks up a step by id.
func (p Plan) Step(id string) (Step, bool) {
	for _, st := range p.Steps {
		if st.ID == id {
			return st, true
		}
	}
	return Step{}, false
}

// RepoOf returns the repo a step targets, if any.
func (p Plan) RepoOf(st Step) (Repo, bool) {
	if st.Repo == "" {
		return Repo{}, false
	}
	r, ok := p.Repos[st.Repo]
	return r, ok
}

// --- wire (YAML) representation -------------------------------------------------

type wireFile struct {
	Version int        `yaml:"version"`
	Update  wireUpdate `yaml:"update"`
}

type wireUpdate struct {
	Root     string              `yaml:"root"`
	Defaults wireDefaults        `yaml:"defaults"`
	Repos    map[string]wireRepo `yaml:"repos"`
	Steps    []wireStep          `yaml:"steps"`
}

type wireDefaults struct {
	Timeout string    `yaml:"timeout"`
	Retry   wireRetry `yaml:"retry"`
}

type wireRetry struct {
	Attempts *int          `yaml:"attempts"`
	On       []wireRetryOn `yaml:"on"`
	Backoff  wireBackoff   `yaml:"backoff"`
}

type wireBackoff struct {
	Initial string   `yaml:"initial"`
	Max     string   `yaml:"max"`
	Factor  *float64 `yaml:"factor"`
	Jitter  *bool    `yaml:"jitter"`
}

// wireRetryOn accepts either a string token ("transport", "timeout", "any")
// or a bare integer exit code, per spec F1.
type wireRetryOn struct {
	raw string
}

func (w *wireRetryOn) UnmarshalYAML(value *yaml.Node) error {
	switch value.Tag {
	case "!!int":
		var n int
		if err := value.Decode(&n); err != nil {
			return err
		}
		w.raw = fmt.Sprintf("exit:%d", n)
	default:
		var s string
		if err := value.Decode(&s); err != nil {
			return err
		}
		w.raw = s
	}
	return nil
}

type wireRepo struct {
	Path     string   `yaml:"path"`
	URL      string   `yaml:"url"`
	Branches []string `yaml:"branches"`
	Local    string   `yaml:"local"`
	Restore  *bool    `yaml:"restore"`
}

type wireExpect struct {
	Exit []int `yaml:"exit"`
}

type wireStep struct {
	ID          string     `yaml:"id"`
	Kind        string     `yaml:"kind"`
	Repo        string     `yaml:"repo"`
	Run         string     `yaml:"run"`
	Interactive *bool      `yaml:"interactive"`
	Hostname    string     `yaml:"hostname"`
	Needs       []string   `yaml:"needs"`
	Expect      wireExpect `yaml:"expect"`
	OnFailure   string     `yaml:"on_failure"`
	Timeout     *string    `yaml:"timeout"` // pointer: nil = unset, distinct from "0"
	Retry       *wireRetry `yaml:"retry"`
}

// Parse decodes, defaults, validates, and path-resolves a fleet.yaml plan.
// Source is left "" — the caller (loadPlan) records where the bytes came
// from.
func Parse(data []byte) (Plan, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var wf wireFile
	if err := dec.Decode(&wf); err != nil {
		return Plan{}, fmt.Errorf("updplan: parse: %w", err)
	}

	errs := &errCollector{}
	if wf.Version != 1 {
		errs.addf("version", "must be 1, got %d", wf.Version)
	}

	root := strings.TrimSpace(wf.Update.Root)
	if root == "" {
		root = "~/git"
	}

	defs, err := parseDefaults(wf.Update.Defaults)
	errs.add(err)

	repos, err := parseRepos(wf.Update.Repos, root)
	errs.add(err)

	steps, err := parseSteps(wf.Update.Steps, defs, repos)
	errs.add(err)

	if err := errs.join(); err != nil {
		return Plan{}, err
	}

	return Plan{
		Root:     root,
		Defaults: defs,
		Repos:    repos,
		Steps:    steps,
		Source:   "",
	}, nil
}
