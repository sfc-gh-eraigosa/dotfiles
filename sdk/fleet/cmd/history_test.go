package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updexec"
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
	if iSpark >= iNano || iNano >= iPi {
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

// TestProblemsDigestsNewestRunPerHost pins the fleet-wide answer to "what is
// broken and where". With no host named it walks each host's NEWEST run only
// — older runs are history, not the current state — and prints that run's
// digest, so one command replaces reading four logs.
func TestProblemsDigestsNewestRunPerHost(t *testing.T) {
	old := flagHistoryProblems
	flagHistoryProblems = true
	t.Cleanup(func() { flagHistoryProblems = old })

	dir := t.TempDir()
	writeCapture(t, dir, "20260909T034714Z", "gig", capBad)
	writeCapture(t, dir, "20260909T034715Z", "pi", capOK)
	// an OLDER broken run for pi, which must NOT be reported: pi's newest is clean
	writeCapture(t, dir, "20260901T000000Z", "pi", capBad)

	var buf strings.Builder
	if err := runHistory(&buf, nil, dir, time.UTC); err != nil {
		t.Fatalf("runHistory: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "could not read Username") {
		t.Errorf("gig's problem must be reported:\n%s", out)
	}
	if !strings.Contains(out, "clean") {
		t.Errorf("a host whose newest run is clean must be SAID to be clean, not omitted:\n%s", out)
	}
	if strings.Count(out, "could not read Username") != 1 {
		t.Errorf("only the newest run per host is digested; pi's older broken run must not appear:\n%s", out)
	}
}

// TestProblemsCollapseRepeatsInOutput pins that the rendered digest carries
// the multiplier rather than repeating a line — the whole reason to have a
// digest instead of `--errors`.
func TestProblemsCollapseRepeatsInOutput(t *testing.T) {
	old := flagHistoryProblems
	flagHistoryProblems = true
	t.Cleanup(func() { flagHistoryProblems = old })

	dir := t.TempDir()
	writeCapture(t, dir, "20260909T034714Z", "gig", `# fleet update — host=gig started=x
03:47:18 !! sudo: a password is required
03:47:18 !! sudo: a password is required
03:47:18 !! sudo: a password is required
03:47:19 WARNING: could not install these apt packages: git gh jq
# 2026-09-09T03:50:36Z finished
`)

	var buf strings.Builder
	if err := runHistory(&buf, []string{"gig"}, dir, time.UTC); err != nil {
		t.Fatalf("runHistory: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "3×") {
		t.Errorf("a repeated failure must render with its multiplier:\n%s", out)
	}
	if strings.Count(out, "sudo: a password is required") != 1 {
		t.Errorf("the repeated line must appear once, not three times:\n%s", out)
	}
	if !strings.Contains(out, "could not install these apt packages") {
		t.Errorf("the authored diagnosis must be shown:\n%s", out)
	}
}

// TestUncapturedRunIsNotCalledClean pins the fleet-wide view's honesty. An
// interactive run captures none of install.sh's output, so its digest is
// empty for the same reason a perfect run's is. Printing "clean" there would
// report a host as verified on the strength of a file we know is missing the
// only part that mattered — the same "reported success it never earned"
// failure fleet exists to catch.
func TestUncapturedRunIsNotCalledClean(t *testing.T) {
	old := flagHistoryProblems
	flagHistoryProblems = true
	t.Cleanup(func() { flagHistoryProblems = old })

	dir := t.TempDir()
	writeCapture(t, dir, "20260909T043222Z", "gig",
		"# fleet update — host=gig started=x\n"+
			"04:32:23 === step dotfiles.install (run) ===\n"+
			"04:32:23 "+updexec.InteractiveNote+"\n"+
			"# 2026-09-09T04:34:20Z finished\n")

	var buf strings.Builder
	if err := runHistory(&buf, nil, dir, time.UTC); err != nil {
		t.Fatalf("runHistory: %v", err)
	}
	out := buf.String()

	if strings.Contains(out, "clean") {
		t.Errorf("an unobserved run must never be reported clean:\n%s", out)
	}
	if !strings.Contains(out, "not captured") {
		t.Errorf("it must say the output was not captured:\n%s", out)
	}
}

// TestDigestGroupsByClassAndKeepsEverything pins the rendering contract the
// operator asked for: classify, never hide. Advisories are labelled and put
// last so triage reads top-down, but they are still printed, and a
// continuation line is still shown as detail under its parent.
func TestDigestGroupsByClassAndKeepsEverything(t *testing.T) {
	old := flagHistoryProblems
	flagHistoryProblems = true
	t.Cleanup(func() { flagHistoryProblems = old })

	dir := t.TempDir()
	writeCapture(t, dir, "20260909T034714Z", "gig",
		"# fleet update — host=gig started=x\n"+
			"03:47:18 WARNING: could not install these apt packages: git jq\n"+
			"03:47:19 !! npm warn install-scripts 2 packages have install scripts\n"+
			"03:47:20 !! Updates are available for some Google Cloud CLI components. To install them,\n"+
			"03:47:20 !! please run:\n"+
			"# 2026-09-09T03:50:36Z finished\n")

	var buf strings.Builder
	if err := runHistory(&buf, []string{"gig"}, dir, time.UTC); err != nil {
		t.Fatalf("runHistory: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "failures") || !strings.Contains(out, "advisories") {
		t.Errorf("output must label the classes:\n%s", out)
	}
	// nothing is hidden
	for _, want := range []string{
		"could not install these apt packages",
		"npm warn install-scripts",
		"Updates are available for some Google Cloud CLI components",
		"please run:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("every line must still appear; missing %q:\n%s", want, out)
		}
	}
	// failures lead advisories
	if strings.Index(out, "could not install") > strings.Index(out, "npm warn") {
		t.Errorf("failures must be listed before advisories:\n%s", out)
	}
}
