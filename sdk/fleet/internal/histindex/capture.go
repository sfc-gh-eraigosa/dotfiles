package histindex

import (
	"os"
	"strings"

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

	for _, l := range c.Lines {
		if l.Stderr && !updexec.Benign(l.Text) {
			c.Warnings++
		}
	}
	return c, nil
}

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
