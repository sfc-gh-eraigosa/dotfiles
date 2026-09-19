package updplan

import (
	"fmt"
	"regexp"
	"slices"
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
	// Description is the longer explanation: what the value is for, where to
	// find it, what happens if it is wrong. A prompt has to be one line; this
	// does not.
	Description string
	Type        InputType
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
	// DefaultFrom is a command run ON THE HOST to find the value when the
	// operator supplied none — the k3s join token already on disk, the node
	// name this machine is registered as. A host that can answer for itself
	// should never be asked.
	//
	// For a confidential input this is the better path, not just the friendlier
	// one: the value is computed in the remote shell and never travels at all.
	DefaultFrom string
	// Validate is a regexp the value must match. Checked locally for an
	// operator-supplied value, so a typo fails before the fleet is touched
	// rather than on the third host — and on the HOST for a discovered one,
	// through `grep -E`. The same text runs through both engines, so the
	// parser restricts it to the POSIX ERE subset they agree on (see
	// validateRe).
	Validate *regexp.Regexp
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
	// goOnlyRe spots syntax Go's RE2 accepts that POSIX `grep -E` does not:
	// `(?i)` / `(?:` groups and the `\d` `\w` `\s` `\b` classes (GNU grep
	// rejects `\d`, BSD and busybox grep differ again). A rule using them
	// would pass every local check and then fail — or silently pass — on
	// the host, so it is refused at parse time with the reason.
	goOnlyRe = regexp.MustCompile(`\(\?|\\[A-Za-z]`)
)

// ValidInputEnv reports whether s is a name the preamble may emit as a
// shell variable. Every remote-string builder re-validates its inputs
// (AGENTS.md's "run: is verbatim" invariant), so a hand-built Input that
// bypassed Parse still cannot smuggle a metacharacter.
func ValidInputEnv(s string) bool { return inputEnvRe.MatchString(s) }

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
		if slices.Contains(in.NeededBy, st.ID) {
			out = append(out, in)
		}
	}
	return out
}

// HasSource reports whether the input can produce a value without asking:
// a static default, or a command the host can answer with.
func (in Input) HasSource() bool { return in.Default != "" || in.DefaultFrom != "" }

// HasInputs reports whether the plan declares any input at all, so callers
// can skip the whole collection path on the overwhelmingly common plan that
// declares none.
func (p Plan) HasInputs() bool { return len(p.Inputs) > 0 }

// HasHostInputs reports whether any input resolves differently per host,
// which is what decides whether a preview has to be shown once per host.
func (p Plan) HasHostInputs() bool {
	for _, in := range p.Inputs {
		if in.Scope == ScopeHost {
			return true
		}
	}
	return false
}

// Check is the ONE rule for what a value of this input may be — the bool
// literals, the choice's options, the validate regexp — shared by the parser
// (for `default:`) and by the CLI (for --input), so a value cannot pass one
// and fail the other.
func (in Input) Check(val string) error {
	if in.Type == InputBool && val != "true" && val != "false" {
		return fmt.Errorf("%q is not a bool — use true or false", val)
	}
	if len(in.Options) > 0 && !slices.Contains(in.Options, val) {
		return fmt.Errorf("%q is not one of %v", val, in.Options)
	}
	if in.Validate != nil && !in.Validate.MatchString(val) {
		return fmt.Errorf("%q does not match validate %q", val, in.Validate.String())
	}
	return nil
}

// parseInputs validates and defaults the inputs block. steps is every
// declared step, so a needed_by typo — or a needed_by naming a step that
// never receives inputs — is caught here rather than presenting as a value
// the operator is made to supply and that silently reaches nothing.
func parseInputs(in []wireInput, steps []Step) ([]Input, error) {
	kinds := make(map[string]Kind, len(steps))
	for _, st := range steps {
		kinds[st.ID] = st.Kind
	}
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
		} else if len(w.Options) > 0 {
			errs.addf(scope, "options: only a choice input has options")
		}

		v.Description = strings.TrimSpace(w.Description)
		v.DefaultFrom = strings.TrimSpace(w.DefaultFrom)

		if w.Validate != "" {
			if m := goOnlyRe.FindString(w.Validate); m != "" {
				errs.addf(scope, "validate: %q is Go-only regexp syntax — the rule also runs through `grep -E` on the host, so use POSIX ERE (e.g. [0-9] for \\d, [[:space:]] for \\s)", m)
			}
			re, err := regexp.Compile(w.Validate)
			if err != nil {
				errs.addf(scope, "validate: %v", err)
			} else {
				v.Validate = re
			}
		}
		// A default that its own rules reject is a plan bug that would
		// otherwise only surface on the host that fell back to it. Checked
		// through Check so a default is held to exactly what --input is.
		if v.Default != "" {
			if err := v.Check(v.Default); err != nil {
				errs.addf(scope, "default: %v", err)
			}
		}
		// A plan file is plain text, usually committed. A confidential value
		// written into it as `default:` would then be exported as a literal
		// on every host's command line — the one delivery the secret flag
		// exists to prevent. The host-side `default_from:` is the way to give
		// a secret a fallback: it is computed there and never travels.
		if v.Secret && v.Default != "" {
			errs.addf(scope, "default: a secret input cannot carry a static default — it would travel as a literal; use default_from so the host computes it")
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
			kind, ok := kinds[id]
			switch {
			case !ok:
				errs.addf(scope, "needed_by: %q is not a declared step", id)
			case kind != KindRun:
				// Only a run step's script receives the preamble and stdin
				// (updexec.Console gates both on KindRun); a value bound to a
				// sync or gh-auth step would be demanded and then dropped.
				errs.addf(scope, "needed_by: %q is a %s step — only a run step receives inputs", id, kind)
			}
		}

		out = append(out, v)
	}
	return out, errs.join()
}
