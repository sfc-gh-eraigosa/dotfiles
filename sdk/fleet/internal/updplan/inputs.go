package updplan

import (
	"fmt"
	"regexp"
	"strings"
)

// An Input is a value the plan needs before it can run, declared so fleet can
// ask for it instead of a step silently going without (#351 feature 3).
//
// Three kinds of gap made this necessary, all found building a real
// multi-repo convergence plan:
//
//   - a per-HOST value (which cluster node this machine is) that no shared
//     plan file can hardcode;
//   - a SECRET (a join token) that must not reach argv or a config file;
//   - a feature SWITCH whose answer belongs in that host's gff state, not in
//     the step's environment at all.
type Input struct {
	ID     string
	Prompt string
	Type   InputType
	// Secret values never touch argv or a file: they are written to the
	// remote shell's stdin and read into the environment there. Implied by
	// type: password.
	Secret bool
	Scope  InputScope
	// Env is the variable the value arrives as. Defaults to the id upshouted
	// (node -> NODE), so the common case declares nothing.
	Env string
	// Flag, when set, makes the answer a gff setting on the host
	// (`gff set <Flag> <value>`) rather than an environment variable. This is
	// what lets a plan turn a repo's own feature on per host.
	Flag    string
	Options []string
	Default string
	// NeededBy limits the input to named steps. Empty means every run step —
	// a sync step runs git, not the plan's script, and has no use for it.
	NeededBy []string
}

type InputType string

const (
	InputText     InputType = "text"
	InputPassword InputType = "password"
	InputBool     InputType = "bool"
	InputChoice   InputType = "choice"
)

type InputScope string

const (
	// ScopeRun is asked once for the whole run: a join token, a release name.
	ScopeRun InputScope = "run"
	// ScopeHost is asked once per host: which node this machine is.
	ScopeHost InputScope = "host"
)

var (
	inputIDRe  = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	inputEnvRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
)

// InputsFor returns the inputs a step should be given.
func (p Plan) InputsFor(st Step) []Input {
	var out []Input
	for _, in := range p.Inputs {
		if len(in.NeededBy) == 0 {
			// Unqualified inputs go to run steps only.
			if st.Kind == KindRun {
				out = append(out, in)
			}
			continue
		}
		for _, id := range in.NeededBy {
			if id == st.ID {
				out = append(out, in)
				break
			}
		}
	}
	return out
}

// HasInputs reports whether the plan declares any input at all, so callers
// can skip the whole collection path on the overwhelmingly common plan that
// declares none.
func (p Plan) HasInputs() bool { return len(p.Inputs) > 0 }

// parseInputs validates and defaults the inputs block. stepIDs is every
// declared step id, so needed_by typos are caught here rather than presenting
// as an input that silently never reaches anything.
func parseInputs(in []wireInput, stepIDs map[string]bool) ([]Input, error) {
	errs := &errCollector{}
	seen := make(map[string]bool, len(in))
	out := make([]Input, 0, len(in))

	for i, w := range in {
		scope := fmt.Sprintf("inputs[%d]", i)
		if w.ID != "" {
			scope = fmt.Sprintf("inputs.%s", w.ID)
		}

		if !inputIDRe.MatchString(w.ID) {
			errs.addf(scope, "id: must match %s, got %q", inputIDRe, w.ID)
			continue
		}
		if seen[w.ID] {
			errs.addf(scope, "id: duplicate input %q", w.ID)
			continue
		}
		seen[w.ID] = true

		v := Input{ID: w.ID, Prompt: w.Prompt, Flag: strings.TrimSpace(w.Flag), Default: w.Default, NeededBy: w.NeededBy}

		v.Type = InputType(strings.TrimSpace(w.Type))
		if v.Type == "" {
			v.Type = InputText
		}
		switch v.Type {
		case InputText, InputPassword, InputBool, InputChoice:
		default:
			errs.addf(scope, "type: must be text|password|bool|choice, got %q", w.Type)
		}

		v.Scope = InputScope(strings.TrimSpace(w.Scope))
		if v.Scope == "" {
			v.Scope = ScopeRun
		}
		switch v.Scope {
		case ScopeRun, ScopeHost:
		default:
			errs.addf(scope, "scope: must be run|host, got %q", w.Scope)
		}

		if w.Secret || v.Type == InputPassword {
			v.Secret = true
		}

		if v.Type == InputChoice {
			if len(w.Options) == 0 {
				errs.addf(scope, "options: a choice input must list its options")
			}
			v.Options = w.Options
			if v.Default != "" && len(w.Options) > 0 && !contains(w.Options, v.Default) {
				errs.addf(scope, "default: %q is not one of the options %v", v.Default, w.Options)
			}
		} else if len(w.Options) > 0 {
			errs.addf(scope, "options: only a choice input has options")
		}

		v.Env = strings.TrimSpace(w.Env)
		if v.Env == "" {
			v.Env = strings.ToUpper(v.ID)
		}
		if !inputEnvRe.MatchString(v.Env) {
			errs.addf(scope, "env: must match %s, got %q", inputEnvRe, v.Env)
		}

		// A gff config file is plain text on disk. Writing a confidential
		// value there would undo the one guarantee this flag makes, so the
		// combination is refused rather than quietly downgraded.
		if v.Flag != "" && v.Secret {
			errs.addf(scope, "flag: a secret input cannot be stored as a gff flag — gff config is plain text on disk")
		}

		for _, id := range v.NeededBy {
			if !stepIDs[id] {
				errs.addf(scope, "needed_by: %q is not a declared step", id)
			}
		}

		out = append(out, v)
	}
	return out, errs.join()
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
