// Package updplan is the pure, host-blind representation of a `fleet update`
// plan: the fleet.yaml schema, its validation rules, defaults merge, backoff
// math, and the topological order the executor walks. Nothing here touches a
// network, a filesystem, or a clock — every impure edge (the YAML bytes, the
// jitter source) is a parameter, never an ambient call.
package updplan

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
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
	// Everything happens in float64 and the cap is applied BEFORE the
	// conversion to Duration: Initial×Factor^(n-1) overflows int64 at
	// n≈32 for the built-in 5s/2 schedule, and a float→int64 conversion of
	// an out-of-range value is implementation-defined (negative on amd64).
	f := float64(b.Initial)
	limit := float64(b.Max)
	for i := 1; i < n; i++ {
		f *= b.Factor
		if b.Max > 0 && f >= limit {
			f = limit
			break
		}
	}
	if b.Max > 0 && f > limit {
		f = limit
	}
	if b.Jitter {
		r := 0.5
		if rnd != nil {
			r = rnd()
		}
		f *= 0.5 + r
	}
	const maxDur = float64(1 << 62) // comfortably inside int64
	if math.IsNaN(f) || f < 0 {
		return 0
	}
	if math.IsInf(f, 1) || f > maxDur {
		return time.Duration(1 << 62)
	}
	return time.Duration(f)
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

func mustParse(data string) Plan {
	p, err := Parse([]byte(data))
	if err != nil {
		panic(fmt.Sprintf("updplan: DefaultYAML failed to parse: %v", err))
	}
	return p
}

// Default returns the built-in plan: today's `fleet update` behaviour,
// exactly as Parse(DefaultYAML) produces it. It re-parses on every call
// (~35µs) so each caller owns its maps and slices — a process-global value
// would let one caller's --local/--timeout override leak into the next.
func Default() Plan {
	return mustParse(DefaultYAML)
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
		if errors.Is(err, io.EOF) {
			// An empty or comment-only file is a schema problem, not an
			// I/O one; do not let it satisfy errors.Is(err, io.EOF).
			return Plan{}, errors.New("updplan: parse: empty plan file (no YAML document; run `fleet update init`)")
		}
		return Plan{}, fmt.Errorf("updplan: parse: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return Plan{}, errors.New("updplan: parse: multiple YAML documents; a plan file holds exactly one")
	} else if !errors.Is(err, io.EOF) {
		return Plan{}, fmt.Errorf("updplan: parse: second document: %w", err)
	}

	errs := &errCollector{}
	if wf.Version != 1 {
		errs.addf("version", "must be 1, got %d", wf.Version)
	}

	root := strings.TrimSpace(wf.Update.Root)
	if root == "" {
		root = "~/git"
	}
	// root is prefixed onto every relative repo path, so it obeys the same
	// charset rule as path: AND must itself be absolute or ~-relative —
	// otherwise Repo.Path would resolve against the ssh login cwd.
	rootOK := root == "~" || ((strings.HasPrefix(root, "/") || strings.HasPrefix(root, "~/")) && ValidPath(root))
	if !rootOK {
		errs.addf("update", "root: must be an absolute or ~/ path using [A-Za-z0-9._/-], got %q", root)
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
