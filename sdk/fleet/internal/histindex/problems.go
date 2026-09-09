package histindex

import (
	"regexp"
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"
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

// Problem is one distinct thing that went wrong, however many times it
// happened.
type Problem struct {
	Text string
	// Count is how many times this exact line appeared. A failure repeated
	// once per privileged call is one problem seen N times, not N problems.
	Count int
	// First is the earliest occurrence's timestamp.
	First string
	// Authored distinguishes the installer's own explanation from the raw
	// stderr underneath it. Authored lines are the ones a human can act on.
	Authored bool
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

	add := func(l Line, authored bool) {
		if a, ok := seen[l.Text]; ok {
			a.Count++
			return
		}
		order++
		seen[l.Text] = &acc{
			Problem: Problem{Text: l.Text, Count: 1, First: l.Time, Authored: authored},
			seq:     order,
		}
	}

	for _, l := range c.Lines {
		// Colour is stripped BEFORE matching, not just before printing.
		// install.sh colourises its own warnings, so a leading escape
		// sequence hid the "WARNING:" prefix from the matcher entirely —
		// the coloured half of a message was not merely grouped separately,
		// it was not recognised as a problem at all.
		l.Text = strings.TrimRight(ansi.Strip(l.Text), " \t")
		switch {
		case !l.Stderr && authoredProblem.MatchString(strings.TrimSpace(l.Text)):
			add(l, true)
		case l.Stderr && !updexec.Benign(l.Text):
			add(l, false)
		}
	}

	out := make([]Problem, 0, len(seen))
	accs := make([]*acc, 0, len(seen))
	for _, a := range seen {
		accs = append(accs, a)
	}
	sort.Slice(accs, func(i, j int) bool {
		if accs[i].Authored != accs[j].Authored {
			return accs[i].Authored // authored diagnosis leads
		}
		if accs[i].Authored {
			return accs[i].seq < accs[j].seq // in the order it was written
		}
		if accs[i].Count != accs[j].Count {
			return accs[i].Count > accs[j].Count // then loudest stderr first
		}
		return accs[i].seq < accs[j].seq
	})
	for _, a := range accs {
		out = append(out, a.Problem)
	}
	return out
}
