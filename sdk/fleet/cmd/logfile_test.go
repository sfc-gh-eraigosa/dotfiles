package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
)

// The pane is an in-memory ring that dies with the process, so an install that
// misbehaved overnight left nothing to read. Every run is written to a file.
func TestRunLogCapturesTheWholeInstall(t *testing.T) {
	dir := t.TempDir()
	r := runner.Fake{Out: map[string]string{"host-a": "Installing sops...\nBuilding gss...\ndone"}}

	msg := beginStream("host-a", "main", answers{}, r, dir)()
	st := msg.(streamStartedMsg).st
	var streamed []string
	for l := range st.lines {
		streamed = append(streamed, l)
	}
	<-st.done

	files, _ := os.ReadDir(dir)
	if len(files) != 1 {
		t.Fatalf("expected exactly one run log, got %v", files)
	}
	body, _ := os.ReadFile(filepath.Join(dir, files[0].Name()))
	s := string(body)

	// The header ties the file to a host, a ref and a time — without it a bare
	// log cannot be placed.
	for _, want := range []string{"host=host-a", "ref=main", "mode=fast-forward", "started="} {
		if !strings.Contains(s, want) {
			t.Fatalf("header missing %q:\n%s", want, s)
		}
	}
	for _, want := range []string{"Installing sops", "Building gss", "done", "# finished"} {
		if !strings.Contains(s, want) {
			t.Fatalf("run log missing %q:\n%s", want, s)
		}
	}
	// Teeing must not swallow or reorder what the pane sees.
	if len(streamed) != 3 {
		t.Fatalf("the tee changed the stream: %v", streamed)
	}
}

// A forced reset is the destructive mode; the log has to say so, because
// that is the run someone will come back to asking what happened.
func TestRunLogRecordsAForcedReset(t *testing.T) {
	dir := t.TempDir()
	r := runner.Fake{Out: map[string]string{"h": "x"}}
	st := beginStream("h", "main", answers{reset: "y"}, r, dir)().(streamStartedMsg).st
	for range st.lines {
	}
	<-st.done
	files, _ := os.ReadDir(dir)
	body, _ := os.ReadFile(filepath.Join(dir, files[0].Name()))
	if !strings.Contains(string(body), "mode=FORCE RESET") {
		t.Fatalf("a destructive run must be labelled:\n%s", body)
	}
}

// One file per host per run: a concurrent wave interleaved into a single file
// would be unreadable, and a run is the unit an operator looks at.
func TestEachHostAndRunGetsItsOwnFile(t *testing.T) {
	dir := t.TempDir()
	r := runner.Fake{Out: map[string]string{"a": "one", "b": "two"}}
	for _, h := range []string{"a", "b"} {
		st := beginStream(h, "main", answers{}, r, dir)().(streamStartedMsg).st
		for range st.lines {
		}
		<-st.done
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 2 {
		t.Fatalf("expected one file per host, got %v", files)
	}
}

// Losing the log must never cost the update.
func TestAnUnwritableLogDirDoesNotBreakTheStream(t *testing.T) {
	r := runner.Fake{Out: map[string]string{"h": "still streams"}}
	for _, dir := range []string{"", "/proc/nonexistent-cannot-mkdir"} {
		st := beginStream("h", "main", answers{}, r, dir)().(streamStartedMsg).st
		var got []string
		for l := range st.lines {
			got = append(got, l)
		}
		<-st.done
		if len(got) != 1 || got[0] != "still streams" {
			t.Fatalf("dir %q: the stream must survive an unusable log dir, got %v", dir, got)
		}
	}
}

// Unbounded logs on a workstation that updates a fleet daily would grow
// forever; the newest are kept.
func TestOldRunLogsArePruned(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 8; i++ {
		name := logFileName("h", base.Add(time.Duration(i)*time.Minute))
		os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600)
	}
	os.WriteFile(filepath.Join(dir, logFileName("other", base)), []byte("x"), 0o600)

	pruneRunLogs(dir, "h", 3)

	files, _ := os.ReadDir(dir)
	var mine, others int
	for _, f := range files {
		if strings.HasSuffix(f.Name(), "__h.log") {
			mine++
		} else {
			others++
		}
	}
	if mine != 3 {
		t.Fatalf("expected 3 kept for the host, got %d", mine)
	}
	if others != 1 {
		t.Fatalf("pruning must not touch another host's logs, got %d", others)
	}
	// The ones kept must be the NEWEST.
	kept, _ := os.ReadDir(dir)
	newest := logFileName("h", base.Add(7*time.Minute))
	var found bool
	for _, f := range kept {
		if f.Name() == newest {
			found = true
		}
	}
	if !found {
		t.Fatalf("pruning kept the wrong end; %s was deleted", newest)
	}
}

// An ssh alias reaches the filesystem, so it must not be able to escape the
// log directory.
func TestAliasCannotEscapeTheLogDirectory(t *testing.T) {
	for _, bad := range []string{"../../etc/passwd", "a/b", "..", "with space", ""} {
		got := safeAlias(bad)
		if strings.ContainsAny(got, `/\`) || got == ".." || got == "" {
			t.Fatalf("safeAlias(%q) = %q is not a safe filename", bad, got)
		}
		if strings.Contains(filepath.Join("/logs", logFileName(bad, time.Now())), "/etc/") {
			t.Fatalf("alias %q escaped the directory", bad)
		}
	}
}

// Names sort chronologically, which is what pruning and "what ran last" rely on.
func TestRunLogNamesSortChronologically(t *testing.T) {
	base := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	a := logFileName("h", base)
	b := logFileName("h", base.Add(time.Second))
	if !(a < b) {
		t.Fatalf("names must sort by time: %q !< %q", a, b)
	}
	if !strings.Contains(a, "20260304T050607Z") {
		t.Fatalf("unexpected name %q", a)
	}
	_ = fmt.Sprint(a)
}
