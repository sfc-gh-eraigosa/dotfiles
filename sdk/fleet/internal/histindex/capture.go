package histindex

import (
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updexec"
)

// timeWidth is the "HH:MM:SS" prefix libs/log stamps on every captured line.
const timeWidth = 8

// Line is one line of a capture, with the two things the file encodes as
// text turned back into structure: the timestamp prefix and the "!! " stderr
// mark. Keeping the mark in Text would make an error line grep, sort and
// align differently from its stdout neighbours — the mark exists to carry a
// distinction, so a reader has to decode it, not preserve it.
type Line struct {
	Time   string // "03:47:16"; empty for a line with no stamp
	Text   string
	Stderr bool
}

// Capture is one parsed run.
type Capture struct {
	Header string // the "# fleet update — host=… " preamble
	Footer string // the "# <iso> finished" trailer, empty if the run was cut off
	Lines  []Line
	// Warnings counts NON-BENIGN stderr lines — the same number the TUI
	// badges a row with. See Read.
	Warnings int
	// Observed is false when some of this run's output never reached the
	// file — today, when it contains an interactive step whose output went
	// to the terminal instead.
	//
	// It exists so a reader can tell "nothing went wrong" from "I could not
	// see what happened". Both produce an empty problem digest, and
	// presenting the second as the first would mark a host verified that was
	// never actually observed.
	Observed bool
}

// Stderr is the error projection: the same lines the TUI's stderr pane
// shows, in wire order. It is a filter over the one tagged buffer rather
// than a second parse, so the two views can never disagree.
func (c Capture) Stderr() []Line {
	out := make([]Line, 0, len(c.Lines))
	for _, l := range c.Lines {
		if l.Stderr {
			out = append(out, l)
		}
	}
	return out
}

// Read parses one capture file.
//
// Warnings counts only stderr lines updexec.Benign rejects. Counting raw
// stderr would badge every healthy run: git writes its entire fetch progress
// to stderr, so a clean update produces several stderr lines and no problem
// at all. Benign is the classifier the TUI already uses — shared on purpose,
// so `fleet history --errors` and the dashboard's error pane can never
// disagree about what counts as an error.
func Read(path string) (Capture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Capture{}, err
	}

	var c Capture
	raw := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	for i, ln := range raw {
		// "# " marks the header (first line) and the footer (last).
		if strings.HasPrefix(ln, "# ") {
			text := strings.TrimPrefix(ln, "# ")
			switch {
			case i == 0:
				c.Header = text
			case i == len(raw)-1:
				c.Footer = text
			}
			continue
		}
		c.Lines = append(c.Lines, parseLine(ln))
	}

	c.Observed = observed(c.Lines)
	for _, l := range c.Lines {
		if l.Stderr && !updexec.Benign(clean(l.Text)) {
			c.Warnings++
		}
	}
	return c, nil
}

// clean strips terminal colour (and the trailing whitespace it leaves)
// before ANY classification. install.sh colourises its own output, and a
// leading escape sequence hides the "WARNING:" prefix — and the shape of a
// benign git line — from every matcher.
//
// Read strips for the same reason Problems does: the listing's WARN column
// and the --problems digest must agree about what counts as an error, and
// one of them stripping while the other did not made them disagree on
// precisely the colourised lines. The stripped text is used for MATCHING
// only; Lines keeps what the host actually wrote, so --show is unchanged.
func clean(s string) string { return strings.TrimRight(ansi.Strip(s), " \t") }

// parseLine splits the timestamp prefix and the stderr mark off one body
// line. A line without a stamp keeps its whole text — a remote command that
// emitted a bare newline, or output written before the first stamp, must
// survive rather than be dropped for not matching the shape.
func parseLine(ln string) Line {
	out := Line{Text: ln}
	if len(ln) > timeWidth && ln[timeWidth] == ' ' && isClock(ln[:timeWidth]) {
		out.Time = ln[:timeWidth]
		out.Text = ln[timeWidth+1:]
	}
	if rest, ok := strings.CutPrefix(out.Text, updexec.StderrMark); ok {
		out.Stderr = true
		out.Text = rest
	}
	return out
}

// isClock reports whether s is "HH:MM:SS" — digits in the right places,
// which is enough to tell a stamp from a line that merely starts with eight
// characters and a space.
func isClock(s string) bool {
	for i, r := range s {
		if i == 2 || i == 5 {
			if r != ':' {
				return false
			}
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Summary is a Run plus the facts that require opening the file. Scan stays
// filename-only so listing is cheap; a caller that wants these pays for them
// explicitly, and only for the rows it is about to show.
type Summary struct {
	Run
	Warnings int
	Finished bool
	Lines    int
}

// Summarize reads one run's file for its result. Finished is whether the
// capture carries its "finished" footer: a run killed mid-flight (a timeout,
// a closed laptop) leaves the file without one, which is a DIFFERENT outcome
// from a run that completed with errors, and the listing must not merge them.
func Summarize(r Run) (Summary, error) {
	c, err := Read(r.Path)
	if err != nil {
		return Summary{}, err
	}
	return Summary{
		Run:      r,
		Warnings: c.Warnings,
		Finished: c.Footer != "",
		Lines:    len(c.Lines),
	}, nil
}

// runBanner matches the executor's step banner for a `run` step — the one
// kind that can hand the terminal to the remote command.
var runBanner = regexp.MustCompile(`^=== step .* \(run\)( |=)`)

// stepBanner matches any step banner.
var stepBanner = regexp.MustCompile(`^=== step `)

// observed reports whether this capture holds everything the run produced.
//
// It is false when a `run` step's banner is followed by no output at all.
// That is what an INTERACTIVE step leaves behind: ssh -t hands the terminal
// to the remote command, so a two-minute ./install.sh that did the whole job
// writes a banner and nothing else. Detecting it structurally rather than by
// looking for InteractiveNote is deliberate — the note is recent, and every
// capture already on disk predates it, which is precisely the set of past
// runs someone would open this tool to investigate.
//
// A genuinely silent batch step reads the same way and is also reported
// unobserved. That is the conservative direction: the cost is admitting we
// cannot vouch for a run, versus vouching for one nobody saw.
func observed(lines []Line) bool {
	for i, l := range lines {
		// The explicit marker, written by any executor new enough to emit
		// it. Checked as well as the structural rule below, not instead of
		// it: the note IS content, so a banner followed by the note looks
		// "observed" to a purely structural test.
		if l.Text == updexec.InteractiveNote {
			return false
		}
		if !runBanner.MatchString(l.Text) {
			continue
		}
		// Anything before the next banner counts as this step's output.
		if i+1 >= len(lines) || stepBanner.MatchString(lines[i+1].Text) {
			return false
		}
	}
	return true
}
