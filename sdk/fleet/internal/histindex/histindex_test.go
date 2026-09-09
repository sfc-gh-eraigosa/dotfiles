package histindex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestScanReadsHostAndTimeNewestFirst pins the listing order and the
// filename decode. Captures are named <UTC>__<subject>.log by libs/log, so
// the host and the run time are both recoverable without opening the file —
// which is what makes listing a few hundred runs cheap.
func TestScanReadsHostAndTimeNewestFirst(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "20260908T120000Z__alpha.log", "# h\n")
	write(t, dir, "20260909T030000Z__beta.log", "# h\n")
	write(t, dir, "20260907T010000Z__alpha.log", "# h\n")
	write(t, dir, "notes.txt", "not a capture")

	runs, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("got %d runs, want 3 (notes.txt must be ignored): %+v", len(runs), runs)
	}

	want := []struct {
		host string
		at   time.Time
	}{
		{"beta", time.Date(2026, 9, 9, 3, 0, 0, 0, time.UTC)},
		{"alpha", time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)},
		{"alpha", time.Date(2026, 9, 7, 1, 0, 0, 0, time.UTC)},
	}
	for i, w := range want {
		if runs[i].Host != w.host || !runs[i].At.Equal(w.at) {
			t.Errorf("run %d = %s@%s, want %s@%s", i, runs[i].Host, runs[i].At, w.host, w.at)
		}
	}
}

// TestScanDecodesAHostContainingTheSeparator pins that the decode is
// positional, not a split on "__". SafeName permits underscores, so a host
// literally named "a__b" produces "<ts>__a__b.log"; splitting on the first
// "__" would report the host as "a" and splitting on the last as "b".
func TestScanDecodesAHostContainingTheSeparator(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "20260908T120000Z__a__b.log", "# h\n")

	runs, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(runs) != 1 || runs[0].Host != "a__b" {
		t.Fatalf("got %+v, want one run for host a__b", runs)
	}
}

// TestScanIgnoresAnythingItCannotDecode pins that a stray file never becomes
// a bogus row: a missing directory is an empty history, not an error, because
// "no runs yet" is the normal state on a fresh machine.
func TestScanIgnoresAnythingItCannotDecode(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "not-a-timestamp__alpha.log", "# h\n")
	write(t, dir, "20260908T120000Z-no-separator.log", "# h\n")

	runs, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("got %+v, want none", runs)
	}

	if runs, err := Scan(filepath.Join(dir, "does-not-exist")); err != nil || len(runs) != 0 {
		t.Fatalf("a missing dir must be an empty history, got %v / %+v", err, runs)
	}
}

// realCapture mirrors the shape libs/log + updexec actually write: a "# "
// header, "HH:MM:SS text" body lines with stderr marked "!! ", and a
// "# <iso> finished" footer.
const realCapture = `# fleet update — host=pi plan=built-in default mode=fast-forward started=2026-09-08T20:47:14-07:00
03:47:14 === step dotfiles.sync (sync) ===
03:47:14 state=clean branch=main
03:47:16 !! From github.com:sfc-gh-eraigosa/dotfiles
03:47:16 !!  * branch            main       -> FETCH_HEAD
03:47:16 Already up to date.
03:47:20 !! fatal: could not read Username
# 2026-09-09T03:50:36Z finished
`

// TestReadSplitsHeaderBodyAndFooter pins the decode of a capture's three
// parts, and that the "!! " stderr mark is turned back into structure rather
// than left in the text. The mark exists so a post-mortem can tell a warning
// from progress; a reader that kept it as literal text would make every
// stderr line sort and grep differently from its stdout neighbours.
func TestReadSplitsHeaderBodyAndFooter(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "20260909T034714Z__pi.log", realCapture)

	c, err := Read(filepath.Join(dir, "20260909T034714Z__pi.log"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if !strings.Contains(c.Header, "host=pi") {
		t.Errorf("header = %q, want it to carry the run's metadata", c.Header)
	}
	if !strings.Contains(c.Footer, "finished") {
		t.Errorf("footer = %q, want the finished marker", c.Footer)
	}
	if len(c.Lines) != 6 {
		t.Fatalf("got %d body lines, want 6: %+v", len(c.Lines), c.Lines)
	}

	first := c.Lines[0]
	if first.Time != "03:47:14" || first.Stderr || first.Text != "=== step dotfiles.sync (sync) ===" {
		t.Errorf("line 0 = %+v, want the stdout step banner with its time split off", first)
	}

	fetch := c.Lines[2]
	if !fetch.Stderr || fetch.Text != "From github.com:sfc-gh-eraigosa/dotfiles" {
		t.Errorf("line 2 = %+v, want stderr with the !! mark removed", fetch)
	}
}

// TestReadCountsOnlyNonBenignStderrAsAWarning pins that the count matches
// what the TUI's error pane badges. Git writes its whole fetch progress to
// stderr on a completely healthy run, so counting raw stderr lines would put
// a warning badge on every successful update and make the signal worthless.
// updexec.Benign is the SAME classifier the TUI uses — deliberately shared,
// so the CLI and the dashboard can never disagree about what an error is.
func TestReadCountsOnlyNonBenignStderrAsAWarning(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "20260909T034714Z__pi.log", realCapture)

	c, err := Read(filepath.Join(dir, "20260909T034714Z__pi.log"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if got := c.Stderr(); len(got) != 3 {
		t.Errorf("Stderr() returned %d lines, want all 3 marked ones", len(got))
	}
	if c.Warnings != 1 {
		t.Errorf("Warnings = %d, want 1 — only the `fatal:` line is non-benign; "+
			"git's two fetch-progress lines are routine", c.Warnings)
	}
}

// TestSummarizeReportsFinishAndWarnings pins the two facts the listing shows
// that a filename cannot supply. A capture with no "finished" footer is a run
// that was killed or is still going — distinct from one that finished badly,
// and the listing must not conflate them.
func TestSummarizeReportsFinishAndWarnings(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "20260909T034714Z__pi.log", realCapture)
	cut := "# fleet update — host=nano started=x\n03:47:14 working\n"
	write(t, dir, "20260909T034715Z__nano.log", cut)

	runs, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	got := map[string]Summary{}
	for _, r := range runs {
		s, err := Summarize(r)
		if err != nil {
			t.Fatalf("Summarize(%s): %v", r.Host, err)
		}
		got[r.Host] = s
	}

	if !got["pi"].Finished || got["pi"].Warnings != 1 {
		t.Errorf("pi = %+v, want finished with 1 warning", got["pi"])
	}
	if got["nano"].Finished {
		t.Errorf("nano = %+v, want NOT finished — it has no footer", got["nano"])
	}
}
