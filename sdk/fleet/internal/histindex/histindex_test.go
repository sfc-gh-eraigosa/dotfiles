package histindex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updexec"
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

// problemCapture is the shape a real broken run has: install.sh's own
// WARNING: diagnosis on STDOUT, the mechanical noise underneath it on
// stderr, repeated many times, plus routine chatter that is neither.
const problemCapture = `# fleet update — host=gig started=x
03:47:15 === step dotfiles.install (run) ===
03:47:16 !! From https://github.com/o/r
03:47:16 !! Already on 'main'
03:47:18 !! sudo: a password is required
03:47:18 !! sudo: a password is required
03:47:18 !! sudo: a password is required
03:47:18 WARNING: apt-get update failed; installs may be incomplete.
03:47:19 WARNING: could not install these apt packages: git gh jq
03:47:20 Installing fnm...
# 2026-09-09T03:50:36Z finished
`

// TestProblemsLeadWithTheAuthoredDiagnosis pins the ordering that makes this
// useful. install.sh writes its OWN explanation of what broke to stdout
// ("WARNING: could not install these apt packages: …"); the stderr underneath
// is the mechanical cause, repeated once per failed call. A digest that
// showed only stderr — which is what a stdout/stderr split gives you — would
// print 37 copies of "sudo: a password is required" and never once mention
// that 32 packages are missing. The authored line comes first because it is
// the one a human can act on.
func TestProblemsLeadWithTheAuthoredDiagnosis(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "20260909T034714Z__gig.log", problemCapture)

	c, err := Read(filepath.Join(dir, "20260909T034714Z__gig.log"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	ps := c.Problems()

	if len(ps) != 3 {
		t.Fatalf("got %d problems, want 3 (2 diagnoses + 1 collapsed stderr): %+v", len(ps), ps)
	}
	if !strings.HasPrefix(ps[0].Text, "WARNING: apt-get update failed") {
		t.Errorf("problem 0 = %q, want the first authored WARNING", ps[0].Text)
	}
	if !strings.Contains(ps[1].Text, "could not install these apt packages") {
		t.Errorf("problem 1 = %q, want the second authored WARNING", ps[1].Text)
	}
	if ps[0].Class != ClassFailure || ps[2].Class != ClassStderr {
		t.Errorf("Class must separate install.sh's diagnosis from raw stderr: %+v", ps)
	}
}

// TestProblemsCollapseRepeats pins the collapsing. The same failure repeated
// once per privileged call is ONE problem seen N times, not N problems; a
// digest that listed each occurrence would bury the other findings exactly
// the way the raw log does.
func TestProblemsCollapseRepeats(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "20260909T034714Z__gig.log", problemCapture)

	c, _ := Read(filepath.Join(dir, "20260909T034714Z__gig.log"))
	var sudo *Problem
	for i := range c.Problems() {
		if strings.Contains(c.Problems()[i].Text, "sudo") {
			sudo = &c.Problems()[i]
		}
	}
	if sudo == nil {
		t.Fatal("the repeated sudo failure must be reported")
	}
	if sudo.Count != 3 {
		t.Errorf("Count = %d, want 3 — the three occurrences collapse into one entry", sudo.Count)
	}
	if sudo.First != "03:47:18" {
		t.Errorf("First = %q, want the earliest occurrence", sudo.First)
	}
}

// TestProblemsExcludeRoutineChatter pins that benign stderr and ordinary
// stdout never reach the digest. If a healthy run produced problems, the
// digest would be as useless as the raw log.
func TestProblemsExcludeRoutineChatter(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "20260909T034714Z__gig.log", problemCapture)

	c, _ := Read(filepath.Join(dir, "20260909T034714Z__gig.log"))
	for _, p := range c.Problems() {
		if strings.Contains(p.Text, "From https://") ||
			strings.Contains(p.Text, "Already on") ||
			strings.Contains(p.Text, "Installing fnm") {
			t.Errorf("routine line reached the digest: %q", p.Text)
		}
	}
}

// TestCleanRunHasNoProblems pins the other end: the run that fixed the host
// must digest to nothing at all.
func TestCleanRunHasNoProblems(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "20260909T034714Z__pi.log", realCapture)

	c, _ := Read(filepath.Join(dir, "20260909T034714Z__pi.log"))
	ps := c.Problems()
	if len(ps) != 1 || !strings.Contains(ps[0].Text, "fatal: could not read Username") {
		t.Fatalf("want only the genuine fatal line, got %+v", ps)
	}
}

// TestUncapturedRunIsNotReportedAsProblemFree pins the distinction between
// "nothing went wrong" and "I could not see what happened". An interactive
// run's output never reaches the capture, so its digest is empty for the
// same reason a perfect run's is — and calling that clean would present an
// entirely unobserved host as verified, which is the exact failure mode
// (a host reporting success it never earned) fleet exists to prevent.
func TestUncapturedRunIsNotReportedAsProblemFree(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "20260909T034714Z__gig.log",
		"# fleet update — host=gig started=x\n"+
			"04:32:23 === step dotfiles.install (run) ===\n"+
			"04:32:23 "+updexec.InteractiveNote+"\n"+
			"# 2026-09-09T04:34:20Z finished\n")

	c, err := Read(filepath.Join(dir, "20260909T034714Z__gig.log"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(c.Problems()) != 0 {
		t.Fatalf("the note itself must not be a problem: %+v", c.Problems())
	}
	if c.Observed {
		t.Error("a run whose output went to the terminal was NOT observed; " +
			"an empty digest here means unseen, not clean")
	}

	clean, _ := Read(filepath.Join(dir, "20260909T034714Z__gig.log"))
	_ = clean
	dir2 := t.TempDir()
	write(t, dir2, "20260909T034714Z__pi.log", realCapture)
	c2, _ := Read(filepath.Join(dir2, "20260909T034714Z__pi.log"))
	if !c2.Observed {
		t.Error("a fully captured run IS observed")
	}
}

// TestProblemsStripColourBeforeGrouping pins that escape sequences do not
// reach the digest. install.sh colourises its own warnings, so the same
// message wrapped in different escapes would group as two distinct problems
// and would render as literal "[1;33m" noise in a summary whose entire job
// is to be readable at a glance. --show still prints the file's own bytes;
// the digest is a summary, and a summary normalises.
func TestProblemsStripColourBeforeGrouping(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "20260909T034714Z__gig.log",
		"# fleet update — host=gig started=x\n"+
			"03:47:18 \x1b[1;33mWARNING: disk is nearly full\x1b[0m\n"+
			"03:47:19 WARNING: disk is nearly full\n"+
			"# 2026-09-09T03:50:36Z finished\n")

	c, _ := Read(filepath.Join(dir, "20260909T034714Z__gig.log"))
	ps := c.Problems()
	if len(ps) != 1 {
		t.Fatalf("the same message in two colourings is ONE problem, got %d: %+v", len(ps), ps)
	}
	if ps[0].Count != 2 {
		t.Errorf("Count = %d, want 2", ps[0].Count)
	}
	if strings.Contains(ps[0].Text, "\x1b") {
		t.Errorf("escape sequences must not reach the digest: %q", ps[0].Text)
	}
}

// TestARunStepWithNoOutputIsUnobserved pins detection that works on
// captures written BEFORE the interactive note existed — which is every
// capture already on disk, and therefore every past run someone would use
// this to investigate. A `run` step whose banner is followed by nothing at
// all produced no output we can see: either it was interactive (ssh -t took
// the terminal) or it was genuinely silent, and the capture cannot tell
// which. Both mean "not observed", so the conservative reading is the
// correct one — the failure direction is admitting we cannot vouch for a
// run, never vouching for one we did not see.
func TestARunStepWithNoOutputIsUnobserved(t *testing.T) {
	dir := t.TempDir()
	// exactly the shape of a real interactive capture: sync output, then an
	// install banner with nothing after it.
	write(t, dir, "20260909T043222Z__gig.log",
		"# fleet update — host=gig started=x\n"+
			"04:32:22 === step dotfiles.sync (sync) ===\n"+
			"04:32:23 Already up to date.\n"+
			"04:32:23 === step dotfiles.install (run) ===\n"+
			"# 2026-09-09T04:34:20Z finished\n")

	c, err := Read(filepath.Join(dir, "20260909T043222Z__gig.log"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if c.Observed {
		t.Error("a run step that produced no captured output means the run was not observed")
	}
}

// TestARunStepWithOutputIsObserved guards the other direction: a step that
// actually produced output must not be written off as unseen, or the flag
// would fire on every healthy run and mean nothing.
func TestARunStepWithOutputIsObserved(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "20260909T043222Z__gig.log",
		"# fleet update — host=gig started=x\n"+
			"04:32:23 === step dotfiles.install (run) ===\n"+
			"04:32:24 Installing packages...\n"+
			"# 2026-09-09T04:34:20Z finished\n")

	c, _ := Read(filepath.Join(dir, "20260909T043222Z__gig.log"))
	if !c.Observed {
		t.Error("a run step with output WAS observed")
	}
}

// TestAdvisoriesAreClassifiedNotHidden pins that a tool announcing news
// about ITSELF is separated from something that actually failed — and that
// it is still reported. npm's install-scripts notice and pip's upgrade
// notice mean nothing went wrong; grouping them with "could not install
// these apt packages" is what made a healthy host show 34 problems. They are
// classified rather than suppressed: a filter that hides is a filter that
// can hide the one line that mattered, and an operator triaging a fleet
// needs to see everything once and decide for themselves.
func TestAdvisoriesAreClassifiedNotHidden(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "20260909T034714Z__gig.log",
		"# fleet update — host=gig started=x\n"+
			"03:47:18 WARNING: could not install these apt packages: git jq\n"+
			// npm, pip and gcloud all write these to STDERR — that is how
			// they arrive in a real capture.
			"03:47:19 !! npm warn install-scripts 2 packages have install scripts\n"+
			"03:47:20 !! [notice] A new release of pip is available: 26.0.1 -> 26.2.1\n"+
			"03:47:21 !! WARNING: Running pip as the 'root' user can result in broken permissions\n"+
			"03:47:22 !! Updates are available for some Google Cloud CLI components.\n"+
			"# 2026-09-09T03:50:36Z finished\n")

	c, _ := Read(filepath.Join(dir, "20260909T034714Z__gig.log"))
	ps := c.Problems()

	if len(ps) != 5 {
		t.Fatalf("every line must still be reported, got %d: %+v", len(ps), ps)
	}

	byClass := map[Class]int{}
	for _, p := range ps {
		byClass[p.Class]++
	}
	if byClass[ClassFailure] != 1 {
		t.Errorf("exactly the apt failure is a failure, got %d: %+v", byClass[ClassFailure], ps)
	}
	if byClass[ClassAdvisory] != 4 {
		t.Errorf("the four tool notices are advisories, got %d: %+v", byClass[ClassAdvisory], ps)
	}

	// failures sort ahead of advisories so triage reads top-down
	if ps[0].Class != ClassFailure {
		t.Errorf("failures must lead: %+v", ps)
	}
}

// TestContinuationLinesFoldIntoTheirParent pins that a multi-line message is
// ONE problem. All three shapes here are real, taken from live captures, and
// each one was previously counted as several unrelated problems — which is
// most of why a healthy host reported 34.
//
// Nothing is dropped: continuations are attached as detail, so the full text
// is still there to read. The rules are deliberately shallow — an unfolded
// line merely stands on its own, which is the old behaviour, so a miss costs
// tidiness rather than information.
func TestContinuationLinesFoldIntoTheirParent(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "20260909T034714Z__pi.log",
		"# fleet update — host=pi started=x\n"+
			// (a) an unfinished sentence continues: "…To install them," -> "please run:" -> the command
			"03:47:10 !! Updates are available for some Google Cloud CLI components.  To install them,\n"+
			"03:47:10 !! please run:\n"+
			"03:47:10 !!   $ gcloud components update\n"+
			// (b) indentation AFTER the severity tag marks a continuation
			"03:47:20 WARNING: Wayland needs the keyd GNOME extension:\n"+
			"03:47:20 WARNING:   ln -s /usr/local/share/keyd/gnome-extension-45 \\\n"+
			"03:47:20 WARNING:         ~/.local/share/gnome-shell/extensions/keyd\n"+
			// (c) a repeated tool tag is one message
			// goenv writes to stderr, as it does in a real capture
			"03:47:30 !! goenv: WARNING: System 'go' found at /usr/bin/go\n"+
			"03:47:30 !! goenv: Since your shims are at the end of PATH, system 'go' will be used\n"+
			"03:47:30 !! goenv: To fix this, add the following to your ~/.goenvrc:\n"+
			"# 2026-09-09T03:50:36Z finished\n")

	c, _ := Read(filepath.Join(dir, "20260909T034714Z__pi.log"))
	ps := c.Problems()

	if len(ps) != 3 {
		var got []string
		for _, p := range ps {
			got = append(got, p.Text)
		}
		t.Fatalf("three messages, want 3 problems, got %d:\n%s", len(ps), strings.Join(got, "\n"))
	}

	byText := map[string]Problem{}
	for _, p := range ps {
		byText[p.Text] = p
	}
	gcloud, ok := byText["Updates are available for some Google Cloud CLI components.  To install them,"]
	if !ok || len(gcloud.Detail) != 2 {
		t.Errorf("gcloud message must carry its 2 continuation lines: %+v", gcloud)
	}
	keyd, ok := byText["WARNING: Wayland needs the keyd GNOME extension:"]
	if !ok || len(keyd.Detail) != 2 {
		t.Errorf("keyd message must carry its 2 indented lines: %+v", keyd)
	}
	goenv, ok := byText["goenv: WARNING: System 'go' found at /usr/bin/go"]
	if !ok || len(goenv.Detail) != 2 {
		t.Errorf("goenv message must carry its 2 same-tag lines: %+v", goenv)
	}
}

// TestSeverityTagIsNotATagForFolding guards the rule above: "WARNING:" is a
// severity marker every unrelated failure shares, so folding on it would
// swallow every warning into whichever one came first.
func TestSeverityTagIsNotATagForFolding(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "20260909T034714Z__pi.log",
		"# fleet update — host=pi started=x\n"+
			"03:47:10 WARNING: apt-get update failed; installs may be incomplete.\n"+
			"03:47:11 WARNING: could not install these apt packages: git jq\n"+
			"# 2026-09-09T03:50:36Z finished\n")

	c, _ := Read(filepath.Join(dir, "20260909T034714Z__pi.log"))
	if ps := c.Problems(); len(ps) != 2 {
		t.Fatalf("two unrelated warnings are two problems, got %d: %+v", len(ps), ps)
	}
}

// TestNearDuplicatesCollapseIntoOne pins the last of the three noise
// sources. Nine ollama personas failed for ONE reason — a base model that
// was never pulled — and listed as nine problems they crowded out
// everything else on the host. They differ only by an identifier in the
// middle, so they are one problem seen nine times.
func TestNearDuplicatesCollapseIntoOne(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	b.WriteString("# fleet update — host=pi started=x\n")
	for _, name := range []string{
		"ai-ci-aiarch", "architecture-adversary", "architecture-em",
		"architecture-principal", "architecture-secarch", "architecture-sysarch",
	} {
		b.WriteString("03:47:30 WARNING: ollama create teams-" + name +
			" failed (base model 'qwen3.8:27b' likely not pulled) — Modelfile still written\n")
	}
	b.WriteString("# 2026-09-09T03:50:36Z finished\n")
	write(t, dir, "20260909T034714Z__pi.log", b.String())

	c, _ := Read(filepath.Join(dir, "20260909T034714Z__pi.log"))
	ps := c.Problems()

	if len(ps) != 1 {
		t.Fatalf("six spellings of one failure are one problem, got %d: %+v", len(ps), ps)
	}
	if ps[0].Count != 6 {
		t.Errorf("Count = %d, want 6", ps[0].Count)
	}
	if !strings.Contains(ps[0].Text, "ollama create teams-") ||
		!strings.Contains(ps[0].Text, "likely not pulled") {
		t.Errorf("the collapsed text must keep both the shared head and tail: %q", ps[0].Text)
	}
	if !strings.Contains(ps[0].Text, "…") {
		t.Errorf("the varying middle must be elided, not invented: %q", ps[0].Text)
	}
}

// TestDistinctFailuresAreNotMerged guards it. These two share a long tag and
// both end in a period, but they are entirely different problems; merging
// them would erase one. Collapsing must need a substantial shared HEAD and
// TAIL, not merely a common prefix.
func TestDistinctFailuresAreNotMerged(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "20260909T034714Z__pi.log",
		"# fleet update — host=pi started=x\n"+
			"03:47:10 !! install_herdr: /home/u/.config/herdr/config.toml is hand-edited; leaving it alone.\n"+
			"03:47:11 !! install_herdr: another herdr is earlier in PATH: /home/u/.local/bin/herdr; remove it.\n"+
			"03:47:12 WARNING: apt-get update failed; installs may be incomplete.\n"+
			"03:47:13 WARNING: could not install these apt packages: git jq\n"+
			"# 2026-09-09T03:50:36Z finished\n")

	c, _ := Read(filepath.Join(dir, "20260909T034714Z__pi.log"))
	if ps := c.Problems(); len(ps) != 4 {
		var got []string
		for _, p := range ps {
			got = append(got, p.Text)
		}
		t.Fatalf("four distinct problems must stay four, got %d:\n%s", len(ps), strings.Join(got, "\n"))
	}
}

// TestFailureIsDecidedByContentNotStream pins that "WARNING: … failed" is a
// failure wherever it was written. install.sh sends some of its own warnings
// to stdout and others to stderr — on one host every install failure arrived
// on stdout, on another every one arrived on stderr — so keying the class off
// the stream classified a host's real failures as raw stderr and left the
// failures group empty on exactly the hosts that had them.
//
// The stream still decides one thing: an unrecognised stderr line is
// cause-level evidence (ClassStderr), because stderr is where subprocesses
// report. It just cannot outrank an explicit WARNING:/ERROR: marker.
func TestFailureIsDecidedByContentNotStream(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "20260909T034714Z__pi.log",
		"# fleet update — host=pi started=x\n"+
			"03:47:10 WARNING: ollama create teams-x failed (base model not pulled)\n"+
			"03:47:11 !! WARNING: cannot detect the focused window on this wayland session.\n"+
			"03:47:12 !! sudo: a password is required\n"+
			"# 2026-09-09T03:50:36Z finished\n")

	c, _ := Read(filepath.Join(dir, "20260909T034714Z__pi.log"))
	got := map[string]Class{}
	for _, p := range c.Problems() {
		got[p.Text] = p.Class
	}

	if cl := got["WARNING: ollama create teams-x failed (base model not pulled)"]; cl != ClassFailure {
		t.Errorf("a stdout WARNING is a failure, got %v", cl)
	}
	if cl := got["WARNING: cannot detect the focused window on this wayland session."]; cl != ClassFailure {
		t.Errorf("a stderr WARNING is ALSO a failure — the stream must not decide, got %v", cl)
	}
	if cl := got["sudo: a password is required"]; cl != ClassStderr {
		t.Errorf("an unmarked stderr line stays cause-level evidence, got %v", cl)
	}
}

// TestClassLabelsAgreeWithTheirCount keeps the headline from reading as a
// template bug. "stderr" is invariant; the other two inflect.
func TestClassLabelsAgreeWithTheirCount(t *testing.T) {
	for _, tc := range []struct {
		class Class
		n     int
		want  string
	}{
		{ClassFailure, 1, "1 failure"},
		{ClassFailure, 3, "3 failures"},
		{ClassAdvisory, 1, "1 advisory"},
		{ClassAdvisory, 2, "2 advisories"},
		{ClassStderr, 1, "1 stderr"},
		{ClassStderr, 4, "4 stderr"},
	} {
		if got := tc.class.Label(tc.n); got != tc.want {
			t.Errorf("Label(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}
