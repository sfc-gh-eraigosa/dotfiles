package log

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Capture is a per-run plain-text file for output produced by something else —
// a remote install's stdout, a build's console, a migration's report.
//
// It is deliberately NOT a logrus sink. The value of captured output is being
// readable exactly as the tool emitted it; wrapping each line in JSON to log it
// "properly" makes the one thing it is for harder. What IS standardized is the
// file's lifecycle: location, permissions, a header that ties it to a subject,
// per-line timestamps, and retention.
type Capture struct {
	f    *os.File
	path string
	now  func() time.Time
}

// CaptureOptions configures a capture file.
type CaptureOptions struct {
	// Tool names the component; with Dir empty it selects
	// <state>/<tool>/logs. Required unless Dir is set.
	Tool string
	// Dir overrides the directory entirely.
	Dir string
	// Subject is what this run is about — a hostname, a target, a job id. It
	// becomes part of the filename, sanitized.
	Subject string
	// Header is written first, as a `# ` comment, to tie the file to its run.
	Header string
	// Keep is how many files for this subject are retained; 0 → 200.
	Keep int
	// Now is the clock, injected for tests.
	Now func() time.Time
}

// NewCapture opens this run's file. It returns nil when no usable location
// exists — writing to a nil *Capture is safe and does nothing, so losing the
// capture never costs the caller its actual work.
func NewCapture(opts CaptureOptions) *Capture {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	dir := opts.Dir
	if dir == "" {
		if s := StateDir(opts.Tool); s != "" {
			dir = filepath.Join(s, "logs")
		}
	}
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return nil
	}
	at := now()
	path := filepath.Join(dir, CaptureName(opts.Subject, at))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, filePerm)
	if err != nil {
		return nil
	}
	c := &Capture{f: f, path: path, now: now}
	if opts.Header != "" {
		fmt.Fprintf(f, "# %s\n", opts.Header)
	}
	keep := opts.Keep
	if keep <= 0 {
		keep = 200
	}
	Prune(dir, opts.Subject, keep)
	return c
}

// Path is the file's location, for telling an operator where to look. Empty
// for a nil Capture.
func (c *Capture) Path() string {
	if c == nil {
		return ""
	}
	return c.path
}

// WriteLine appends one timestamped line. A nil Capture is a no-op.
func (c *Capture) WriteLine(s string) {
	if c == nil {
		return
	}
	fmt.Fprintf(c.f, "%s %s\n", c.now().UTC().Format("15:04:05"), s)
}

// Close writes a footer and closes the file. A nil Capture is a no-op, and
// Close is safe to call twice.
func (c *Capture) Close(footer string) error {
	if c == nil || c.f == nil {
		return nil
	}
	if footer != "" {
		fmt.Fprintf(c.f, "# %s %s\n", c.now().UTC().Format(time.RFC3339), footer)
	}
	err := c.f.Close()
	c.f = nil
	return err
}

// Tee returns a channel that forwards everything from in while writing it to
// c, closing c when in closes. It is the shape a streaming tool wants: the
// capture is a side effect, never a gate on the data.
func (c *Capture) Tee(in <-chan string, footer string) <-chan string {
	if c == nil {
		return in
	}
	out := make(chan string, 64)
	go func() {
		defer close(out)
		defer func() { _ = c.Close(footer) }()
		for l := range in {
			c.WriteLine(l)
			out <- l
		}
	}()
	return out
}

// CaptureName is the filename for a run: timestamp first, so the directory
// sorts chronologically by name and retention is a lexical operation.
func CaptureName(subject string, at time.Time) string {
	return fmt.Sprintf("%s__%s.log", at.UTC().Format("20060102T150405Z"), SafeName(subject))
}

// SafeName reduces an arbitrary subject to a filename that cannot escape its
// directory. "." and ".." survive a charset filter but are directory entries,
// so they are rejected outright.
func SafeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" || out == "." || out == ".." {
		return "run"
	}
	return out
}

// Prune keeps the newest keep files for subject in dir, leaving other
// subjects' files alone. Unbounded captures on a machine that runs daily grow
// without limit.
func Prune(dir, subject string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	suffix := "__" + SafeName(subject) + ".log"
	var mine []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), suffix) {
			mine = append(mine, e.Name())
		}
	}
	if len(mine) <= keep {
		return
	}
	sort.Strings(mine) // timestamp-prefixed: lexical == chronological
	for _, n := range mine[:len(mine)-keep] {
		_ = os.Remove(filepath.Join(dir, n))
	}
}

// compile-time assurance that a Capture can stand in for a writer sink.
var _ io.Writer = (*captureWriter)(nil)

type captureWriter struct{ c *Capture }

func (w *captureWriter) Write(p []byte) (int, error) {
	for _, l := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		w.c.WriteLine(l)
	}
	return len(p), nil
}

// Writer adapts a Capture to io.Writer, for handing to exec.Cmd.Stdout or
// anything else that writes bytes rather than lines.
func (c *Capture) Writer() io.Writer { return &captureWriter{c: c} }
