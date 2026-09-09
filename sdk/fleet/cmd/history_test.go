package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeCapture drops a capture file into dir the way libs/log names them.
func writeCapture(t *testing.T, dir, stamp, host, body string) {
	t.Helper()
	name := stamp + "__" + host + ".log"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

const capOK = `# fleet update — host=pi started=x
03:47:14 === step dotfiles.sync (sync) ===
03:47:16 !! From github.com:sfc-gh-eraigosa/dotfiles
03:47:16 Already up to date.
# 2026-09-09T03:50:36Z finished
`

const capBad = `# fleet update — host=nano started=x
04:10:00 === step dotfiles.sync (sync) ===
04:10:01 !! fatal: could not read Username
04:10:01 !! error: failed to fetch
# 2026-09-09T04:10:02Z finished
`

const capCut = `# fleet update — host=spark started=x
05:00:00 working
`

func historyFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeCapture(t, dir, "20260909T034714Z", "pi", capOK)
	writeCapture(t, dir, "20260909T041000Z", "nano", capBad)
	writeCapture(t, dir, "20260909T050000Z", "spark", capCut)
	return dir
}

// TestHistoryListsNewestFirstWithOutcome pins the default listing: every run,
// newest first, with the two facts a filename cannot give — whether the run
// finished, and how many non-benign stderr lines it produced. The benign
// fetch header in capOK must NOT be counted, or every healthy run would carry
// a warning and the column would mean nothing.
func TestHistoryListsNewestFirstWithOutcome(t *testing.T) {
	var buf strings.Builder
	if err := runHistory(&buf, nil, historyFixture(t), time.UTC); err != nil {
		t.Fatalf("runHistory: %v", err)
	}
	out := buf.String()

	iSpark := strings.Index(out, "spark")
	iNano := strings.Index(out, "nano")
	iPi := strings.Index(out, "pi ")
	if iSpark < 0 || iNano < 0 || iPi < 0 {
		t.Fatalf("every host must be listed:\n%s", out)
	}
	if !(iSpark < iNano && iNano < iPi) {
		t.Errorf("want newest-first (spark, nano, pi):\n%s", out)
	}

	if !strings.Contains(out, "unfinished") {
		t.Errorf("the capture with no footer must be marked unfinished:\n%s", out)
	}
	if !strings.Contains(out, "2") {
		t.Errorf("nano's two non-benign stderr lines must be counted:\n%s", out)
	}
	if strings.Contains(out, "⚠1") {
		t.Errorf("pi's only stderr line is a benign fetch header and must not count:\n%s", out)
	}
}

// TestHistoryFiltersToOneHost pins the by-host navigation: naming a host
// shows only that host's runs.
func TestHistoryFiltersToOneHost(t *testing.T) {
	var buf strings.Builder
	if err := runHistory(&buf, []string{"nano"}, historyFixture(t), time.UTC); err != nil {
		t.Fatalf("runHistory: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "nano") {
		t.Errorf("want nano's run:\n%s", out)
	}
	if strings.Contains(out, "spark") || strings.Contains(out, "pi ") {
		t.Errorf("naming a host must exclude the others:\n%s", out)
	}
}

// TestHistoryShowPrintsTheRunsContent pins --show: the capture body, with the
// "!! " mark decoded rather than printed literally.
func TestHistoryShowPrintsTheRunsContent(t *testing.T) {
	old := flagHistoryShow
	flagHistoryShow = true
	t.Cleanup(func() { flagHistoryShow = old })

	var buf strings.Builder
	if err := runHistory(&buf, []string{"nano"}, historyFixture(t), time.UTC); err != nil {
		t.Fatalf("runHistory: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "fatal: could not read Username") {
		t.Errorf("want the run's content:\n%s", out)
	}
	if strings.Contains(out, "!! ") {
		t.Errorf("the stderr mark is structure, not text — it must not be printed raw:\n%s", out)
	}
}

// TestHistoryErrorsShowsOnlyStderr pins --errors: the same projection the
// TUI's stderr pane shows.
func TestHistoryErrorsShowsOnlyStderr(t *testing.T) {
	oldS, oldE := flagHistoryShow, flagHistoryErrors
	flagHistoryShow, flagHistoryErrors = true, true
	t.Cleanup(func() { flagHistoryShow, flagHistoryErrors = oldS, oldE })

	var buf strings.Builder
	if err := runHistory(&buf, []string{"nano"}, historyFixture(t), time.UTC); err != nil {
		t.Fatalf("runHistory: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "fatal: could not read Username") {
		t.Errorf("want the stderr lines:\n%s", out)
	}
	if strings.Contains(out, "=== step dotfiles.sync") {
		t.Errorf("--errors must exclude stdout:\n%s", out)
	}
}

// TestHistoryGrepFiltersLines pins --grep against the same content.
func TestHistoryGrepFiltersLines(t *testing.T) {
	oldS, oldG := flagHistoryShow, flagHistoryGrep
	flagHistoryShow, flagHistoryGrep = true, "Username"
	t.Cleanup(func() { flagHistoryShow, flagHistoryGrep = oldS, oldG })

	var buf strings.Builder
	if err := runHistory(&buf, []string{"nano"}, historyFixture(t), time.UTC); err != nil {
		t.Fatalf("runHistory: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "could not read Username") {
		t.Errorf("want the matching line:\n%s", out)
	}
	if strings.Contains(out, "failed to fetch") {
		t.Errorf("--grep must drop non-matching lines:\n%s", out)
	}
}

// TestHistoryEmptyIsSaidNotFailed pins that a machine which has never run an
// update gets a sentence, not an error and not a bare empty table — the same
// rule fleet applies to a missing ~/.ssh/config.
func TestHistoryEmptyIsSaidNotFailed(t *testing.T) {
	var buf strings.Builder
	if err := runHistory(&buf, nil, t.TempDir(), time.UTC); err != nil {
		t.Fatalf("an empty history must not be an error: %v", err)
	}
	if !strings.Contains(strings.ToLower(buf.String()), "no ") {
		t.Errorf("want a sentence explaining there is nothing yet, got:\n%q", buf.String())
	}
}

// TestHistoryUnknownHostNamesWhatExists pins that a typo does not look like
// "this host has never been updated": the message lists the hosts that DO
// have history.
func TestHistoryUnknownHostNamesWhatExists(t *testing.T) {
	var buf strings.Builder
	if err := runHistory(&buf, []string{"nope"}, historyFixture(t), time.UTC); err != nil {
		t.Fatalf("runHistory: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "nano") || !strings.Contains(out, "spark") {
		t.Errorf("an unknown host must name the hosts that do have captures:\n%s", out)
	}
}
