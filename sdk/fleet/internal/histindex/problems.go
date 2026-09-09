package histindex

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updexec"
)

// authoredProblem matches a line the INSTALLER wrote about its own failure.
// install.sh reports what actually broke this way — "WARNING: could not
// install these apt packages: …" — and it writes it to STDOUT, because it is
// a message to the operator rather than a subprocess's error channel.
//
// This is the whole reason a digest cannot be built on the stdout/stderr
// split. On a host whose sudo was broken, stderr held 74 lines that were two
// distinct messages repeated 37 times each; the five lines that explained
// what the machine was now MISSING were all stdout. Filtering to stderr
// showed the mechanism and hid the consequence.
var authoredProblem = regexp.MustCompile(`^(WARNING|ERROR|FATAL|FAILED)\b`)

// Class sorts a problem by what it means, so triage can work down the list
// instead of reading all of it. Nothing is ever hidden by class — a filter
// that hides is a filter that can hide the one line that mattered.
type Class int

const (
	// ClassFailure is the installer saying something it tried did not work.
	// These are what an operator can act on, so they lead.
	ClassFailure Class = iota
	// ClassStderr is non-benign stderr: usually the mechanical cause under a
	// failure ("sudo: a password is required"), occasionally the only
	// evidence there is.
	ClassStderr
	// ClassAdvisory is a tool announcing news about ITSELF — an available
	// upgrade, a config suggestion. Nothing failed. Grouping these with real
	// failures is what made a healthy host report 34 problems.
	ClassAdvisory
)

// Label names the class with its count, inflected. "stderr" is a stream
// name, not a countable noun, so it never takes an s.
func (c Class) Label(n int) string {
	switch {
	case c == ClassStderr:
		return fmt.Sprintf("%d stderr", n)
	case n == 1 && c == ClassFailure:
		return "1 failure"
	case n == 1:
		return "1 advisory"
	default:
		return fmt.Sprintf("%d %s", n, c)
	}
}

// String names the class for rendering and grouping.
func (c Class) String() string {
	switch c {
	case ClassFailure:
		return "failures"
	case ClassStderr:
		return "stderr"
	default:
		return "advisories"
	}
}

// advisoryPatterns match tools reporting on themselves. Kept SHORT and
// specific on purpose: an unrecognised line stays a failure, the same
// conservative default updexec.Benign uses. A false advisory costs a
// misplaced line in a list the operator can still see; a false failure costs
// only a glance.
var advisoryPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^npm warn `),
	regexp.MustCompile(`^\[notice\]`),
	regexp.MustCompile(`^Updates are available for some .* components`),
	regexp.MustCompile(`^WARNING: Running pip as the '?root'? user`),
}

// classify decides what a line means, from its CONTENT first.
//
// An explicit WARNING:/ERROR: marker makes it a failure wherever it was
// written. install.sh sends some of its own warnings to stdout and others to
// stderr — on one host every install failure arrived on stdout, on another
// every one arrived on stderr — so letting the stream decide left the
// failures group empty on exactly the hosts that had failures.
//
// The stream still decides the remainder: an unrecognised stderr line is
// cause-level evidence, because stderr is where subprocesses report. It just
// cannot outrank an explicit marker.
func classify(text string, stderr bool) Class {
	t := strings.TrimSpace(text)
	for _, re := range advisoryPatterns {
		if re.MatchString(t) {
			return ClassAdvisory
		}
	}
	if authoredProblem.MatchString(t) {
		return ClassFailure
	}
	if stderr {
		return ClassStderr
	}
	return ClassFailure
}

// Problem is one distinct thing that went wrong, however many times it
// happened.
type Problem struct {
	Text string
	// Count is how many times this exact line appeared. A failure repeated
	// once per privileged call is one problem seen N times, not N problems.
	Count int
	// First is the earliest occurrence's timestamp.
	First string
	// Class is what this line means — see Class.
	Class Class
	// Detail holds the continuation lines of a multi-line message. They are
	// attached rather than dropped: the whole text stays readable, it just
	// stops being counted as several unrelated problems.
	Detail []string
}

// severityTag matches a leading severity marker. It is stripped before the
// indentation test, so "WARNING:   ln -s …" is recognised as indented — but
// it is deliberately NOT usable as a folding tag, since every unrelated
// failure shares it and folding on it would swallow them all into the first.
var severityTag = regexp.MustCompile(`^(WARNING|WARN|ERROR|FATAL|NOTICE|W|E):`)

// toolTag matches a tool prefixing its own name to every line of one
// message ("goenv: …", "npm warn allow-scripts …"). Two adjacent lines
// sharing one belong to the same message.
var toolTag = regexp.MustCompile(`^(npm warn [a-z-]+|[a-z][a-z0-9_.-]*:)`)

// continues reports whether cur is a continuation of prev.
//
// Three signals, all drawn from real captures:
//
//   - prev ends mid-sentence (", " / ":" / "\") — gcloud's
//     "…To install them," / "please run:" / "  $ gcloud components update".
//   - cur is indented once its severity tag is removed — keyd's
//     "WARNING:   ln -s …" under "WARNING: Wayland needs …".
//   - both carry the same tool tag AND the same timestamp — goenv's
//     "goenv: …" block, without merging a tool's separate remarks.
//
// A miss costs tidiness (the line stands alone, as before), never
// information, so the rules stay shallow on purpose.
func continues(prev, cur Line) bool {
	if prev.Text == "" {
		return false
	}
	p := strings.TrimRight(prev.Text, " ")
	if strings.HasSuffix(p, ",") || strings.HasSuffix(p, ":") || strings.HasSuffix(p, "\\") {
		return true
	}
	// TWO spaces, not one: "WARNING: text" always has one separating space,
	// so a single-space test made every warning a continuation of the
	// warning before it and collapsed the whole list into its first entry.
	// Real continuations are visibly indented ("WARNING:   ln -s …").
	if body := severityTag.ReplaceAllString(cur.Text, ""); body != cur.Text && strings.HasPrefix(body, "  ") {
		return true
	}
	// Same tool tag AND the same timestamp. The tag alone is not enough:
	// one tool emits many INDEPENDENT messages ("install_herdr: config is
	// hand-edited" and "install_herdr: another herdr is earlier in PATH" are
	// two different problems), and folding on the tag merged them. A
	// multi-line message is written in one go, so sharing a second is the
	// signal that separates a continuation from the tool's next remark.
	pt := toolTag.FindString(strings.TrimSpace(prev.Text))
	ct := toolTag.FindString(strings.TrimSpace(cur.Text))
	if ct != "" && ct == pt && prev.Time == cur.Time && !severityTag.MatchString(ct) {
		return true
	}
	return false
}

// Problems is the digest: every distinct problem in the capture, the
// installer's own diagnosis first (in the order it was written), then
// non-benign stderr with the loudest first.
//
// Ordering is the point. Raw order buries the authored explanation under
// whatever repeated most; count order alone leads with the mechanism rather
// than the consequence. Leading with what the installer SAID broke, then
// showing what it saw, is the order someone debugging actually reads in.
func (c Capture) Problems() []Problem {
	type acc struct {
		Problem
		seq int
	}
	seen := map[string]*acc{}
	var order int

	add := func(l Line) {
		if a, ok := seen[l.Text]; ok {
			a.Count++
			return
		}
		order++
		seen[l.Text] = &acc{
			Problem: Problem{Text: l.Text, Count: 1, First: l.Time, Class: classify(l.Text, l.Stderr)},
			seq:     order,
		}
	}

	var last *acc
	var lastLine Line
	for _, l := range c.Lines {
		// Colour is stripped BEFORE matching, not just before printing.
		// install.sh colourises its own warnings, so a leading escape
		// sequence hid the "WARNING:" prefix from the matcher entirely —
		// the coloured half of a message was not merely grouped separately,
		// it was not recognised as a problem at all.
		l.Text = clean(l.Text)
		problem := (!l.Stderr && authoredProblem.MatchString(strings.TrimSpace(l.Text))) ||
			(l.Stderr && !updexec.Benign(l.Text))
		if !problem {
			last, lastLine = nil, Line{}
			continue
		}
		// A line already recorded as its own problem is a REPEAT, not a
		// continuation — checked first because a repeated tagged line
		// ("sudo: a password is required" three times) satisfies the
		// same-tool-tag rule and would otherwise fold into itself and
		// lose its count.
		if _, dup := seen[l.Text]; !dup && last != nil && continues(lastLine, l) {
			last.Detail = append(last.Detail, l.Text)
			lastLine = l
			continue
		}
		add(l)
		last, lastLine = seen[l.Text], l
	}

	out := make([]Problem, 0, len(seen))
	accs := make([]*acc, 0, len(seen))
	for _, a := range seen {
		accs = append(accs, a)
	}
	sort.Slice(accs, func(i, j int) bool {
		if accs[i].Class != accs[j].Class {
			return accs[i].Class < accs[j].Class // failures, stderr, advisories
		}
		if accs[i].Class == ClassFailure {
			return accs[i].seq < accs[j].seq // in the order it was written
		}
		if accs[i].Count != accs[j].Count {
			return accs[i].Count > accs[j].Count // then loudest first
		}
		return accs[i].seq < accs[j].seq
	})
	for _, a := range accs {
		out = append(out, a.Problem)
	}
	return collapseNearDuplicates(out)
}

// minAffix is how much shared head AND tail two lines need before they are
// treated as one failure with a varying middle. Twelve characters is enough
// to require a real shared sentence: "WARNING: " alone is nine, so the two
// unrelated apt warnings can never merge, and two remarks from one tool that
// merely both end in a period share a one-character tail.
const minAffix = 12

// minAffixRatio additionally requires the shared parts to dominate the line.
// Without it, two long messages that happen to share a boilerplate opening
// and closing would collapse despite differing throughout the middle — where
// the meaning is.
const minAffixRatio = 0.5

// collapseNearDuplicates merges problems that differ only by an identifier:
// nine ollama personas that failed for one reason, five memory files skipped
// for one reason. Listed separately they crowd out everything else on the
// host; merged they are one line with a count.
//
// The varying middle is ELIDED with "…" rather than replaced by one of the
// values — printing a single representative would state that a specific
// persona failed when what is known is that nine did.
func collapseNearDuplicates(ps []Problem) []Problem {
	type cluster struct {
		Problem
		prefix, suffix string
	}
	var out []cluster

	for _, p := range ps {
		merged := false
		for i := range out {
			if out[i].Class != p.Class {
				continue
			}
			pre := commonPrefix(out[i].prefix, p.Text)
			suf := commonSuffix(out[i].suffix, p.Text)
			shortest := min(len(out[i].prefix)+len(out[i].suffix), len(p.Text))
			if len(pre) < minAffix || len(suf) < minAffix ||
				len(pre)+len(suf) > len(p.Text) ||
				float64(len(pre)+len(suf)) < minAffixRatio*float64(shortest) {
				continue
			}
			out[i].prefix, out[i].suffix = pre, suf
			out[i].Count += p.Count
			out[i].Text = pre + "…" + suf
			// The merged entry's OWN continuation lines come with it. The
			// elided middle already costs the reader the identifier; also
			// dropping the detail under it would delete text nothing else
			// shows, in a view whose whole contract is that it classifies
			// rather than hides.
			out[i].Detail = append(out[i].Detail, p.Detail...)
			if p.First < out[i].First {
				out[i].First = p.First
			}
			merged = true
			break
		}
		if !merged {
			out = append(out, cluster{Problem: p, prefix: p.Text, suffix: p.Text})
		}
	}

	res := make([]Problem, 0, len(out))
	for _, c := range out {
		res = append(res, c.Problem)
	}
	return res
}

// commonPrefix and commonSuffix compare BYTES but cut on RUNE boundaries.
// Two different runes can share leading bytes ("…" is E2 80 A6 and "—" is
// E2 80 94), so a byte-exact cut can land inside one and put half a rune
// into the elided text — mojibake in the single line the digest exists to
// make readable. Backing off to the nearest boundary costs at most a byte
// or two of shared affix.
func commonPrefix(a, b string) string {
	n := min(len(a), len(b))
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	for i > 0 && i < len(a) && !utf8.RuneStart(a[i]) {
		i--
	}
	return a[:i]
}

func commonSuffix(a, b string) string {
	n := min(len(a), len(b))
	i := 0
	for i < n && a[len(a)-1-i] == b[len(b)-1-i] {
		i++
	}
	for i > 0 && !utf8.RuneStart(a[len(a)-i]) {
		i--
	}
	return a[len(a)-i:]
}
