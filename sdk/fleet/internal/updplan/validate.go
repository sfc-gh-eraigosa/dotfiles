package updplan

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ValidRef guards an operator-supplied git ref (branch/tag). Moved verbatim
// from cmd/update.go's validRef: the ref is interpolated into a command run
// over ssh, so it is constrained to the git ref charset (letters, digits,
// and . _ / -). Anything else — shell metacharacters, spaces, command
// substitution — is rejected before it can run.
func ValidRef(ref string) bool {
	if ref == "" {
		return false
	}
	for _, r := range ref {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.', r == '_', r == '/', r == '-':
		default:
			return false
		}
	}
	return true
}

var (
	repoNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	idRe       = repoNameRe
	urlRe      = regexp.MustCompile(`^(https://|ssh://|git@)[A-Za-z0-9._:/@~-]+(\.git)?$`)
	hostnameRe = regexp.MustCompile(`^[A-Za-z0-9.-]+$`)
	shaRe      = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
	pathCharRe = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
)

// ValidRepoName reports whether s is a legal repo name: ^[a-z0-9][a-z0-9._-]*$.
func ValidRepoName(s string) bool { return repoNameRe.MatchString(s) }

// ValidID reports whether s is a legal step id: same charset as ValidRepoName.
func ValidID(s string) bool { return idRe.MatchString(s) }

// ValidPath reports whether s is a legal repo path: [A-Za-z0-9._/-]+, an
// optional leading "~/", no ".." path segment, and no leading "-".
func ValidPath(s string) bool {
	if s == "" {
		return false
	}
	rest := s
	if strings.HasPrefix(rest, "~/") {
		rest = rest[2:]
		if rest == "" {
			return false
		}
	}
	if strings.HasPrefix(rest, "-") {
		return false
	}
	if !pathCharRe.MatchString(rest) {
		return false
	}
	for _, seg := range strings.Split(rest, "/") {
		if seg == ".." {
			return false
		}
	}
	return true
}

// ValidURL reports whether s is a legal repo URL:
// ^(https://|ssh://|git@)[A-Za-z0-9._:/@~-]+(\.git)?$
func ValidURL(s string) bool { return urlRe.MatchString(s) }

// ValidHostname reports whether s is a legal gh-auth hostname: ^[A-Za-z0-9.-]+$.
func ValidHostname(s string) bool { return hostnameRe.MatchString(s) }

// ValidSHA reports whether s is exactly 40 hex characters.
func ValidSHA(s string) bool { return shaRe.MatchString(s) }

// isTagLike is the heuristic for "a tag only as the sole branches entry": we
// cannot tell a tag from a branch syntactically, so an entry is treated as
// tag-like (and therefore forbidden alongside other entries) only when it
// looks unmistakably like one — "v" followed by a digit, or a full
// refs/tags/ path.
func isTagLike(s string) bool {
	if strings.Contains(s, "refs/tags/") {
		return true
	}
	if len(s) >= 2 && (s[0] == 'v' || s[0] == 'V') && s[1] >= '0' && s[1] <= '9' {
		return true
	}
	return false
}

// --- error aggregation -----------------------------------------------------

type errCollector struct {
	errs []error
}

func (c *errCollector) add(err error) {
	if err != nil {
		c.errs = append(c.errs, err)
	}
}

func (c *errCollector) addf(scope, format string, args ...any) {
	c.add(fmt.Errorf("%s: %s", scope, fmt.Sprintf(format, args...)))
}

func (c *errCollector) join() error {
	return errors.Join(c.errs...)
}

// --- defaults ---------------------------------------------------------------

func parseDuration(scope, field, s string, allowEmpty bool) (time.Duration, error) {
	if s == "" {
		if allowEmpty {
			return 0, nil
		}
		return 0, fmt.Errorf("%s: %s: duration required", scope, field)
	}
	if s == "0" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("%s: %s: invalid duration %q: %v", scope, field, s, err)
	}
	if d < 0 {
		return 0, fmt.Errorf("%s: %s: duration must not be negative, got %q", scope, field, s)
	}
	return d, nil
}

func parseRetryOn(scope string, in []wireRetryOn) ([]RetryOn, error) {
	errs := &errCollector{}
	out := make([]RetryOn, 0, len(in))
	for _, w := range in {
		switch w.raw {
		case string(RetryOnTransport), string(RetryOnTimeout), string(RetryOnAny):
			out = append(out, RetryOn(w.raw))
		default:
			if strings.HasPrefix(w.raw, "exit:") {
				var n int
				if _, err := fmt.Sscanf(w.raw, "exit:%d", &n); err != nil || n < 0 || n > 255 {
					errs.addf(scope, "retry.on: invalid exit code token %q", w.raw)
					continue
				}
				out = append(out, RetryOn(w.raw))
				continue
			}
			errs.addf(scope, "retry.on: unknown token %q", w.raw)
		}
	}
	return out, errs.join()
}

func parseBackoff(scope string, w wireBackoff, fallback Backoff) (Backoff, error) {
	errs := &errCollector{}
	b := fallback

	if w.Initial != "" {
		d, err := parseDuration(scope, "retry.backoff.initial", w.Initial, false)
		errs.add(err)
		b.Initial = d
	}
	if w.Max != "" {
		d, err := parseDuration(scope, "retry.backoff.max", w.Max, false)
		errs.add(err)
		b.Max = d
	}
	if w.Factor != nil {
		if *w.Factor < 1 {
			errs.addf(scope, "retry.backoff.factor: must be >= 1, got %v", *w.Factor)
		} else {
			b.Factor = *w.Factor
		}
	}
	if w.Jitter != nil {
		b.Jitter = *w.Jitter
	}
	return b, errs.join()
}

func parseRetry(scope string, w wireRetry, hasRetry bool, fallback Retry) (Retry, error) {
	if !hasRetry {
		return fallback, nil
	}
	errs := &errCollector{}
	r := fallback

	if w.Attempts != nil {
		if *w.Attempts < 1 {
			errs.addf(scope, "retry.attempts: must be >= 1, got %d", *w.Attempts)
		} else {
			r.Attempts = *w.Attempts
		}
	}
	if len(w.On) > 0 {
		on, err := parseRetryOn(scope, w.On)
		errs.add(err)
		r.On = on
	}
	b, err := parseBackoff(scope, w.Backoff, r.Backoff)
	errs.add(err)
	r.Backoff = b

	return r, errs.join()
}

func parseDefaults(w wireDefaults) (Defaults, error) {
	errs := &errCollector{}
	d := builtinDefaults()

	if w.Timeout != "" {
		t, err := parseDuration("update.defaults", "timeout", w.Timeout, false)
		errs.add(err)
		d.Timeout = t
	}
	r, err := parseRetry("update.defaults", w.Retry, hasWireRetry(w.Retry), d.Retry)
	errs.add(err)
	d.Retry = r

	return d, errs.join()
}

func hasWireRetry(w wireRetry) bool {
	return w.Attempts != nil || len(w.On) > 0 || w.Backoff.Initial != "" ||
		w.Backoff.Max != "" || w.Backoff.Factor != nil || w.Backoff.Jitter != nil
}

// --- repos -------------------------------------------------------------------

func parseRepos(in map[string]wireRepo, root string) (map[string]Repo, error) {
	errs := &errCollector{}
	out := make(map[string]Repo, len(in))

	for name, w := range in {
		scope := fmt.Sprintf("repo %q", name)
		if !ValidRepoName(name) {
			errs.addf(scope, "name: invalid repo name %q", name)
			continue
		}

		r := Repo{Name: name}

		path := w.Path
		if path == "" {
			path = name
		}
		if !ValidPath(path) {
			errs.addf(scope, "path: invalid path %q", path)
		}
		r.Path = resolveRepoPath(root, path)

		if w.URL != "" {
			if !ValidURL(w.URL) {
				errs.addf(scope, "url: invalid url %q", w.URL)
			}
			r.URL = w.URL
		}

		branches := w.Branches
		if len(branches) == 0 {
			branches = []string{"default"}
		}
		if err := validateBranches(scope, branches); err != nil {
			errs.add(err)
		}
		r.Branches = branches

		local := Local(w.Local)
		if local == "" {
			local = LocalSkip
		}
		switch local {
		case LocalSkip, LocalRescue, LocalCarry:
		default:
			errs.addf(scope, "local: invalid value %q", w.Local)
			local = LocalSkip
		}
		r.Local = local

		r.Restore = true
		if w.Restore != nil {
			r.Restore = *w.Restore
		}

		out[name] = r
	}

	return out, errs.join()
}

func resolveRepoPath(root, path string) string {
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "/") {
		return path
	}
	return strings.TrimSuffix(root, "/") + "/" + path
}

func validateBranches(scope string, branches []string) error {
	errs := &errCollector{}
	seen := make(map[string]bool, len(branches))

	for i, b := range branches {
		if b == "" {
			errs.addf(scope, "branches[%d]: empty branch", i)
			continue
		}
		if b == "default" {
			if i != 0 {
				errs.addf(scope, "branches[%d]: \"default\" must be first", i)
			}
		} else if !ValidRef(b) {
			errs.addf(scope, "branches[%d]: invalid ref %q", i, b)
		}
		if seen[b] {
			errs.addf(scope, "branches[%d]: duplicate branch %q", i, b)
		}
		seen[b] = true
	}

	if len(branches) > 1 {
		for i, b := range branches {
			if isTagLike(b) {
				errs.addf(scope, "branches[%d]: tag %q must be the sole entry", i, b)
			}
		}
	}

	return errs.join()
}

// --- steps -------------------------------------------------------------------

func parseSteps(in []wireStep, defs Defaults, repos map[string]Repo) ([]Step, error) {
	errs := &errCollector{}
	out := make([]Step, 0, len(in))
	seenID := make(map[string]bool, len(in))

	for _, w := range in {
		scope := fmt.Sprintf("step %q", w.ID)
		if w.ID == "" || !ValidID(w.ID) {
			errs.addf("step", "id: invalid step id %q", w.ID)
		} else if seenID[w.ID] {
			errs.addf(scope, "id: duplicate step id %q", w.ID)
		}
		seenID[w.ID] = true

		st := Step{ID: w.ID}

		switch Kind(w.Kind) {
		case KindSync, KindRun, KindGhAuth:
			st.Kind = Kind(w.Kind)
		default:
			errs.addf(scope, "kind: unknown kind %q", w.Kind)
		}

		st.Repo = w.Repo
		switch st.Kind {
		case KindSync:
			if w.Repo == "" {
				errs.addf(scope, "repo: sync step requires repo")
			} else if _, ok := repos[w.Repo]; !ok {
				errs.addf(scope, "repo: unknown repo %q", w.Repo)
			}
		case KindGhAuth:
			if w.Repo != "" {
				errs.addf(scope, "repo: gh-auth step must not set repo")
			}
			if w.Hostname != "" && !ValidHostname(w.Hostname) {
				errs.addf(scope, "hostname: invalid hostname %q", w.Hostname)
			}
		case KindRun:
			if w.Repo != "" {
				if _, ok := repos[w.Repo]; !ok {
					errs.addf(scope, "repo: unknown repo %q", w.Repo)
				}
			}
		}
		st.Hostname = w.Hostname

		if st.Kind == KindRun {
			if w.Run == "" {
				errs.addf(scope, "run: run step requires run")
			} else if strings.ContainsAny(w.Run, "\x00\n") {
				errs.addf(scope, "run: must not contain NUL or newline")
			}
			st.Run = w.Run
		} else if w.Run != "" {
			errs.addf(scope, "run: only run steps may set run")
		}

		interactive := false
		if w.Interactive != nil {
			interactive = *w.Interactive
			if st.Kind != KindRun && interactive {
				errs.addf(scope, "interactive: only run steps may be interactive")
			}
		}
		st.Interactive = interactive

		st.Needs = append([]string(nil), w.Needs...)

		exit := w.Expect.Exit
		if len(exit) == 0 {
			exit = []int{0}
		}
		for _, e := range exit {
			if e < 0 || e > 255 {
				errs.addf(scope, "expect.exit: code %d out of range 0..255", e)
			}
		}
		st.Expect = Expect{Exit: exit}

		of := OnFailure(w.OnFailure)
		if of == "" {
			of = OnFailureStop
		}
		switch of {
		case OnFailureStop, OnFailureContinue:
		default:
			errs.addf(scope, "on_failure: invalid value %q", w.OnFailure)
			of = OnFailureStop
		}
		st.OnFailure = of

		hasExplicitTimeout := w.Timeout != nil
		timeout := defs.Timeout
		if interactive && !hasExplicitTimeout {
			timeout = 0
		}
		if hasExplicitTimeout {
			t, err := parseDuration(scope, "timeout", *w.Timeout, false)
			errs.add(err)
			timeout = t
		}
		st.Timeout = timeout

		if interactive && w.Retry != nil {
			errs.addf(scope, "retry: not allowed on an interactive step")
		}
		retry, err := parseRetry(scope, derefRetry(w.Retry), w.Retry != nil, defs.Retry)
		errs.add(err)
		st.Retry = retry

		out = append(out, st)
	}

	// needs: unknown / self / cycle
	for _, st := range out {
		for _, n := range st.Needs {
			scope := fmt.Sprintf("step %q", st.ID)
			if n == st.ID {
				errs.addf(scope, "needs: step cannot need itself")
				continue
			}
			if !seenID[n] {
				errs.addf(scope, "needs: unknown step %q", n)
			}
		}
	}
	if err := detectCycle(out); err != nil {
		errs.add(err)
	}

	return out, errs.join()
}

func derefRetry(w *wireRetry) wireRetry {
	if w == nil {
		return wireRetry{}
	}
	return *w
}

func detectCycle(steps []Step) error {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int, len(steps))
	byID := make(map[string]Step, len(steps))
	for _, st := range steps {
		byID[st.ID] = st
	}

	var visit func(id string, path []string) error
	visit = func(id string, path []string) error {
		switch color[id] {
		case gray:
			return fmt.Errorf("update.steps: cycle detected: %s -> %s", strings.Join(path, " -> "), id)
		case black:
			return nil
		}
		color[id] = gray
		st, ok := byID[id]
		if ok {
			for _, n := range st.Needs {
				if _, ok := byID[n]; !ok {
					continue // reported separately as an unknown need
				}
				if err := visit(n, append(path, id)); err != nil {
					return err
				}
			}
		}
		color[id] = black
		return nil
	}

	for _, st := range steps {
		if color[st.ID] == white {
			if err := visit(st.ID, nil); err != nil {
				return err
			}
		}
	}
	return nil
}
