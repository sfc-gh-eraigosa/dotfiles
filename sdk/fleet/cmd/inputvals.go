package cmd

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// rcInputFlag is the exit a step reports when a gff flag answer could not be
// applied. Distinct so the row says "the switch did not take", not "the
// script failed" — enabling a feature that silently stayed off is the exact
// failure this whole mechanism exists to remove.
const rcInputFlag = 94

// rcInputDiscover is the exit a step reports when an input the host was
// supposed to answer for itself came up empty, or failed its own rule.
// Distinct from a script failure: converging with a blank node name or a
// half-read token is worse than not converging at all.
const rcInputDiscover = 95

// inputValues is what the operator answered: run-scope values once, and
// host-scope values per host alias.
type inputValues struct {
	run  map[string]string
	host map[string]map[string]string
}

func (v inputValues) lookup(host string, in updplan.Input) (string, bool) {
	if in.Scope == updplan.ScopeHost {
		got, ok := v.host[host][in.ID]
		return got, ok
	}
	got, ok := v.run[in.ID]
	return got, ok
}

// inputPreamble builds the shell that puts a step's declared inputs in place:
// exports for plain values, `gff set` for flag-bound answers, and `read -r`
// for confidential ones, which arrive on stdin (see inputStdin).
//
// Confidential values are never written into the command text. The remote
// command line is world-readable through /proc on the host, so a secret in
// argv is a secret published to every local account for the life of the step.
// Reading it from stdin into the shell's own environment keeps it to the
// process and its children, which is where the step needs it.
func inputPreamble(p updplan.Plan, st updplan.Step, host string, vals inputValues) (string, error) {
	ins := p.InputsFor(st)
	if len(ins) == 0 {
		return "", nil
	}
	repo, hasRepo := p.RepoOf(st)

	var b strings.Builder
	for _, in := range ins {
		val, ok := vals.lookup(host, in)

		// Nothing supplied, but the plan says how to find it: let the host
		// answer. For a confidential value this is the best case — it is
		// computed in the remote shell and never travels.
		if !ok && in.DefaultFrom != "" {
			b.WriteString(discoverShell(in))
			continue
		}
		if !ok && in.Default != "" {
			fmt.Fprintf(&b, "export %s=%s; ", in.Env, shQuote(in.Default))
			continue
		}
		if !ok {
			// Nothing supplied and nothing to fall back on. missingInputs
			// has already refused the run; a dry-run of an unanswered plan
			// simply shows the step without it.
			continue
		}

		switch {
		case in.Secret:
			// An interactive step's stdin IS the operator's terminal, so
			// there is nowhere to put a secret except argv. Refuse rather
			// than downgrade silently.
			if st.Interactive {
				return "", fmt.Errorf("step %q is interactive, so input %q (confidential) cannot be delivered: its stdin is the terminal. Make the step batch (fleet primes sudo for it) or have the script prompt for the value itself", st.ID, in.ID)
			}
			tmp := "__fleet_in_" + in.ID
			fmt.Fprintf(&b, "IFS= read -r %s; export %s=\"$%s\"; unset %s; ", tmp, in.Env, tmp, tmp)

		case in.Flag != "":
			if !hasRepo {
				return "", fmt.Errorf("input %q sets gff flag %q but step %q targets no repo — gff resolves a flag from a checkout", in.ID, in.Flag, st.ID)
			}
			// A subshell so the step's own `cd` is not disturbed.
			// repo.Path is emitted UNQUOTED, exactly as every other step
			// script emits it: the path routinely starts with ~, and `cd
			// '~/pg'` looks for a directory literally named "~". Safe because
			// updplan's ValidPath already restricts a repo path to
			// [A-Za-z0-9._/-] plus a leading ~, with no "..".
			fmt.Fprintf(&b, "( cd %s && gff set %s %s ) || { echo %s >&2; exit %d; }; ",
				repo.Path, shQuote(in.Flag), shQuote(val),
				shQuote(fmt.Sprintf("fleet: gff set %s failed (is gff installed, and is the flag declared in that repo?)", in.Flag)),
				rcInputFlag)

		default:
			fmt.Fprintf(&b, "export %s=%s; ", in.Env, shQuote(val))
		}
	}
	return b.String(), nil
}

// discoverShell builds the remote lookup for an input the host answers for
// itself: run the command, refuse an empty result, check it against the
// input's own rule, then export it.
//
// The value is assigned and exported in the remote shell, never interpolated
// into the command text, so a confidential one is not exposed by the host's
// own /proc — which is the whole reason to prefer discovery over sending it.
func discoverShell(in updplan.Input) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s=\"$(%s 2>/dev/null || true)\"; ", in.Env, in.DefaultFrom)
	// Every message is quoted as ONE unit. Interpolating an already-quoted
	// fragment into a quoted echo pushes it back OUT of the quotes, where the
	// shell expands it — and the text most likely to land here is a regexp,
	// full of $ and * and (. `sh -n` rejected exactly that; see
	// TestGeneratedShellSurvivesAwkwardText.
	fmt.Fprintf(&b, "[ -n \"$%s\" ] || { echo %s >&2; exit %d; }; ",
		in.Env,
		shQuote(fmt.Sprintf("fleet: input %s: this host found nothing (%s) and no --input was given", in.ID, describeSource(in))),
		rcInputDiscover)
	if in.Validate != nil {
		// grep, not a shell case: the rule is a regexp, and a glob would
		// quietly accept values the plan's own validate would reject.
		fmt.Fprintf(&b, "printf '%%s' \"$%s\" | grep -qE %s || { echo %s >&2; exit %d; }; ",
			in.Env, shQuote(in.Validate.String()),
			shQuote(fmt.Sprintf("fleet: input %s: what this host found does not match %s", in.ID, in.Validate.String())),
			rcInputDiscover)
	}
	fmt.Fprintf(&b, "export %s; ", in.Env)
	return b.String()
}

// describeSource names where a value comes from, for a message an operator
// reads when it did not arrive.
func describeSource(in updplan.Input) string {
	if in.DefaultFrom != "" {
		return "tried: " + in.DefaultFrom
	}
	return "no discovery declared"
}

// inputStdin is the payload the preamble's `read -r` calls consume: one
// confidential value per line, in the same order inputPreamble emits them.
func inputStdin(p updplan.Plan, st updplan.Step, host string, vals inputValues) string {
	if st.Interactive {
		return ""
	}
	var b strings.Builder
	for _, in := range p.InputsFor(st) {
		if !in.Secret {
			continue
		}
		if val, ok := vals.lookup(host, in); ok {
			// (a host-discovered value never gets here: lookup misses and the
			// preamble computes it remotely instead)
			b.WriteString(val)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// parseInputFlags reads --input values in three forms:
//
//	id=value        a literal (refused for a confidential input)
//	id=@path        read from a local file
//	id=env:NAME     read from the local environment
//	host:id=…       any of the above, for one host (scope: host)
//
// A confidential value must never be a literal: argv is world-readable via
// /proc for the life of the process, which is precisely what declaring it
// confidential asks fleet to avoid.
func parseInputFlags(p updplan.Plan, raw []string) (inputValues, error) {
	vals := inputValues{run: map[string]string{}, host: map[string]map[string]string{}}
	byID := make(map[string]updplan.Input, len(p.Inputs))
	for _, in := range p.Inputs {
		byID[in.ID] = in
	}

	for _, item := range raw {
		key, spec, ok := strings.Cut(item, "=")
		if !ok {
			return vals, fmt.Errorf("--input %q: expected [<host>:]<id>=<value>", item)
		}
		host, id := "", key
		if h, rest, isHost := strings.Cut(key, ":"); isHost {
			host, id = h, rest
		}
		in, known := byID[id]
		if !known {
			return vals, fmt.Errorf("--input %q: the plan declares no input %q", item, id)
		}

		val, err := resolveInputSpec(in, spec)
		if err != nil {
			return vals, fmt.Errorf("--input %q: %w", item, err)
		}
		// Checked here, before a single host is contacted: a typo that only
		// surfaces on the third host has already half-converged a fleet.
		if in.Validate != nil && !in.Validate.MatchString(val) {
			if in.Secret {
				return vals, fmt.Errorf("--input %q: the value does not match validate %q", item, in.Validate.String())
			}
			return vals, fmt.Errorf("--input %q: %q does not match validate %q", item, val, in.Validate.String())
		}
		if len(in.Options) > 0 && !containsStr(in.Options, val) {
			return vals, fmt.Errorf("--input %q: %q is not one of %v", item, val, in.Options)
		}

		if in.Scope == updplan.ScopeHost {
			if host == "" {
				return vals, fmt.Errorf("--input %q: %q is a per-host input — use <host>:%s=…", item, id, id)
			}
			if vals.host[host] == nil {
				vals.host[host] = map[string]string{}
			}
			vals.host[host][id] = val
			continue
		}
		if host != "" {
			return vals, fmt.Errorf("--input %q: %q is asked once per run, not per host", item, id)
		}
		vals.run[id] = val
	}
	return vals, nil
}

func resolveInputSpec(in updplan.Input, spec string) (string, error) {
	switch {
	case strings.HasPrefix(spec, "@"):
		b, err := os.ReadFile(strings.TrimPrefix(spec, "@"))
		if err != nil {
			return "", err
		}
		return strings.TrimRight(string(b), "\r\n"), nil
	case strings.HasPrefix(spec, "env:"):
		name := strings.TrimPrefix(spec, "env:")
		val, ok := os.LookupEnv(name)
		if !ok {
			return "", fmt.Errorf("$%s is not set", name)
		}
		return val, nil
	default:
		if in.Secret {
			return "", fmt.Errorf("%q is confidential, so it cannot be a literal on the command line (argv is world-readable via /proc) — use @<file> or env:<NAME>", in.ID)
		}
		return spec, nil
	}
}

// planHasConfidentialInput reports whether any declared input is delivered
// over stdin, which is what decides whether a lane needs a Stdin producer.
func planHasConfidentialInput(p updplan.Plan) bool {
	for _, in := range p.Inputs {
		if in.Secret {
			return true
		}
	}
	return false
}

// validateInputDelivery refuses, before anything runs, the one combination
// that cannot be delivered: a confidential value for an interactive step,
// whose stdin is the operator's terminal. Checked up front so the failure
// names the plan's mistake instead of surfacing mid-fleet as a step error.
func validateInputDelivery(p updplan.Plan) error {
	for _, st := range p.Steps {
		if !st.Interactive {
			continue
		}
		for _, in := range p.InputsFor(st) {
			if in.Secret {
				return fmt.Errorf("plan %s: input %q is confidential but step %q is interactive — an interactive step's stdin is the terminal, so there is nowhere to put the value except argv. Make the step batch (fleet primes sudo for it), or have its script prompt for the value itself", p.Source, in.ID, st.ID)
			}
		}
	}
	return nil
}

// missingInputs names every declared value the run has no answer for, so a
// run fails before it starts rather than halfway through the fleet.
func missingInputs(p updplan.Plan, hosts []string, vals inputValues) []string {
	var out []string
	for _, in := range p.Inputs {
		// An input the plan can answer on its own — a static default, or a
		// lookup the host performs — is not something to ask an operator for.
		if in.HasSource() {
			continue
		}
		if in.Scope == updplan.ScopeRun {
			if _, ok := vals.run[in.ID]; !ok {
				out = append(out, in.ID)
			}
			continue
		}
		for _, h := range hosts {
			if _, ok := vals.host[h][in.ID]; !ok {
				out = append(out, h+":"+in.ID)
			}
		}
	}
	sort.Strings(out)
	return out
}

func containsStr(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

// listInputs explains every value the plan needs: what it is for, whether it
// is asked once or per host, and where it comes from if the operator says
// nothing. Answers "what do I have to supply, and why" without reading the
// plan file.
func listInputs(w io.Writer, p updplan.Plan) {
	if !p.HasInputs() {
		fmt.Fprintf(w, "plan %s declares no inputs.\n", p.Source)
		return
	}
	fmt.Fprintf(w, "inputs declared by %s:\n\n", p.Source)
	for _, in := range p.Inputs {
		fmt.Fprintf(w, "  %s", in.ID)
		if in.Prompt != "" {
			fmt.Fprintf(w, " — %s", in.Prompt)
		}
		fmt.Fprintln(w)

		scope := "once per run"
		if in.Scope == updplan.ScopeHost {
			scope = "once per host"
		}
		kind := string(in.Type)
		if in.Secret {
			kind = "confidential"
		}
		fmt.Fprintf(w, "      %s, %s", kind, scope)
		if in.Flag != "" {
			fmt.Fprintf(w, ", sets gff flag %s", in.Flag)
		} else {
			fmt.Fprintf(w, ", arrives as $%s", in.Env)
		}
		fmt.Fprintln(w)

		switch {
		case in.DefaultFrom != "":
			fmt.Fprintf(w, "      default: found on the host (%s)\n", in.DefaultFrom)
		case in.Default != "":
			fmt.Fprintf(w, "      default: %s\n", in.Default)
		default:
			fmt.Fprintf(w, "      required: no default, supply it with --input\n")
		}
		if len(in.Options) > 0 {
			fmt.Fprintf(w, "      one of: %s\n", strings.Join(in.Options, ", "))
		}
		if in.Validate != nil {
			fmt.Fprintf(w, "      must match: %s\n", in.Validate.String())
		}
		for _, line := range strings.Split(strings.TrimSpace(in.Description), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				fmt.Fprintf(w, "      %s\n", line)
			}
		}
		fmt.Fprintln(w)
	}
}
