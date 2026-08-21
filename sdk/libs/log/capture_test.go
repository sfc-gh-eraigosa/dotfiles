package log

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixedClock() func() time.Time {
	t0 := time.Date(2026, 8, 20, 15, 7, 18, 0, time.UTC)
	return func() time.Time { return t0 }
}

func TestCaptureWritesHeaderLinesAndFooter(t *testing.T) {
	dir := t.TempDir()
	c := NewCapture(CaptureOptions{Dir: dir, Subject: "host-nano",
		Header: "fleet update — host=host-nano ref=main", Now: fixedClock()})
	if c == nil {
		t.Fatal("expected a capture")
	}
	c.WriteLine("Installing 28 core packages via apt...")
	c.WriteLine("done")
	if err := c.Close("finished"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(c.Path())
	s := string(b)
	for _, want := range []string{
		"# fleet update — host=host-nano ref=main",
		"15:07:18 Installing 28 core packages",
		"15:07:18 done",
		"finished",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("capture missing %q:\n%s", want, s)
		}
	}
	// Raw output stays raw — the whole point of a capture is reading it as the
	// tool emitted it, so it must not be JSON-wrapped.
	if strings.Contains(s, `{"level"`) {
		t.Fatalf("captured output must not be structured:\n%s", s)
	}
}

// Losing a capture must never cost the caller its actual work, so an
// unusable location yields a nil Capture that is safe to use.
func TestNilCaptureIsSafe(t *testing.T) {
	c := NewCapture(CaptureOptions{Dir: "/proc/cannot/mkdir/here", Subject: "x"})
	if c != nil {
		t.Fatal("expected nil for an unusable directory")
	}
	c.WriteLine("must not panic")
	if err := c.Close("footer"); err != nil {
		t.Fatalf("Close on nil should be a no-op, got %v", err)
	}
	if c.Path() != "" {
		t.Fatal("nil capture has no path")
	}
	// Tee must pass the stream straight through rather than dropping it.
	in := make(chan string, 2)
	in <- "a"
	in <- "b"
	close(in)
	var got []string
	for l := range c.Tee(in, "done") {
		got = append(got, l)
	}
	if len(got) != 2 {
		t.Fatalf("a nil capture must not swallow the stream, got %v", got)
	}
}

// Tee is a side effect on the stream, never a gate: everything in comes out,
// in order, and the file is closed when the input ends.
func TestTeeForwardsEverythingAndCloses(t *testing.T) {
	dir := t.TempDir()
	c := NewCapture(CaptureOptions{Dir: dir, Subject: "h", Now: fixedClock()})
	in := make(chan string, 3)
	for _, l := range []string{"one", "two", "three"} {
		in <- l
	}
	close(in)

	var got []string
	for l := range c.Tee(in, "finished") {
		got = append(got, l)
	}
	if strings.Join(got, ",") != "one,two,three" {
		t.Fatalf("tee changed the stream: %v", got)
	}
	b, _ := os.ReadFile(c.Path())
	for _, want := range []string{"one", "two", "three", "finished"} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("file missing %q:\n%s", want, b)
		}
	}
}

// A subject reaches the filesystem, so it must not escape the directory.
func TestSubjectCannotEscapeTheDirectory(t *testing.T) {
	for _, bad := range []string{"../../etc/passwd", "a/b", "..", ".", "", "with space"} {
		got := SafeName(bad)
		if strings.ContainsAny(got, `/\`) || got == "" || got == "." || got == ".." {
			t.Fatalf("SafeName(%q) = %q is not a safe filename", bad, got)
		}
	}
	dir := t.TempDir()
	c := NewCapture(CaptureOptions{Dir: dir, Subject: "../../escape", Now: fixedClock()})
	if c == nil {
		t.Fatal("expected a capture")
	}
	_ = c.Close("")
	if filepath.Dir(c.Path()) != dir {
		t.Fatalf("capture escaped its directory: %s", c.Path())
	}
}

func TestNamesSortChronologically(t *testing.T) {
	t0 := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	if a, b := CaptureName("h", t0), CaptureName("h", t0.Add(time.Second)); !(a < b) {
		t.Fatalf("names must sort by time: %q !< %q", a, b)
	}
}

// Unbounded captures on a machine that runs daily grow without limit; the
// NEWEST are kept, and another subject's files are untouched.
func TestPruneKeepsNewestPerSubject(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 8; i++ {
		os.WriteFile(filepath.Join(dir, CaptureName("h", base.Add(time.Duration(i)*time.Minute))), []byte("x"), filePerm)
	}
	os.WriteFile(filepath.Join(dir, CaptureName("other", base)), []byte("x"), filePerm)

	Prune(dir, "h", 3)

	entries, _ := os.ReadDir(dir)
	var mine, others int
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), "__h.log") {
			mine++
		} else {
			others++
		}
	}
	if mine != 3 || others != 1 {
		t.Fatalf("kept %d for h and %d others, want 3 and 1", mine, others)
	}
	newest := CaptureName("h", base.Add(7*time.Minute))
	if _, err := os.Stat(filepath.Join(dir, newest)); err != nil {
		t.Fatalf("pruning kept the wrong end; %s was deleted", newest)
	}
}

func TestCaptureFileIsOwnerOnly(t *testing.T) {
	dir := t.TempDir()
	c := NewCapture(CaptureOptions{Dir: dir, Subject: "h", Now: fixedClock()})
	_ = c.Close("")
	fi, err := os.Stat(c.Path())
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != filePerm {
		t.Fatalf("capture is %o, want %o", fi.Mode().Perm(), filePerm)
	}
}

// Writer lets a Capture stand in wherever bytes are written (exec.Cmd.Stdout).
func TestWriterSplitsIntoTimestampedLines(t *testing.T) {
	dir := t.TempDir()
	c := NewCapture(CaptureOptions{Dir: dir, Subject: "h", Now: fixedClock()})
	w := c.Writer()
	if _, err := w.Write([]byte("alpha\nbeta\n")); err != nil {
		t.Fatal(err)
	}
	_ = c.Close("")
	b, _ := os.ReadFile(c.Path())
	if strings.Count(string(b), "15:07:18 ") != 2 {
		t.Fatalf("expected two timestamped lines:\n%s", b)
	}
}
