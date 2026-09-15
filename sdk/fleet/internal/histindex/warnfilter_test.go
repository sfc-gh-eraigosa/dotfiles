package histindex

import (
	"path/filepath"
	"testing"
)

// gcloudNag is the exact three-line self-update nag gcloud writes to stderr
// on every invocation once a component update is available. Only the first
// line matches advisoryPatterns on its own text — the other two ("please
// run:" and the indented command) look like ordinary stderr in isolation,
// which is why folding them requires state, not a per-line regexp.
var gcloudNag = []Line{
	{Time: "03:47:10", Stderr: true, Text: "Updates are available for some Google Cloud CLI components.  To install them,"},
	{Time: "03:47:10", Stderr: true, Text: "please run:"},
	{Time: "03:47:10", Stderr: true, Text: "  $ gcloud components update"},
}

// TestWarnFilterFoldsTheGcloudNagToZero pins Change B's headline case: three
// lines, one advisory message, zero warnings — live streaming order.
func TestWarnFilterFoldsTheGcloudNagToZero(t *testing.T) {
	var f WarnFilter
	got := 0
	for _, l := range gcloudNag {
		if f.Warn(l) {
			got++
		}
	}
	if got != 0 {
		t.Fatalf("Warn() counted %d of the gcloud nag's 3 lines, want 0", got)
	}
}

// TestWarnFilterFoldsTheGcloudNagInACapture pins the same fold through the
// file-backed path: Read must count 0 warnings for a capture containing
// nothing but the nag, matching the live-streaming result above so the
// badge and the WARN column can never disagree about this exact message.
func TestWarnFilterFoldsTheGcloudNagInACapture(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "20260909T034714Z__gig.log",
		"# fleet update — host=gig\n"+
			"03:47:10 !! Updates are available for some Google Cloud CLI components.  To install them,\n"+
			"03:47:10 !! please run:\n"+
			"03:47:10 !!   $ gcloud components update\n"+
			"# 2026-09-09T03:50:36Z finished\n")

	c, err := Read(filepath.Join(dir, "20260909T034714Z__gig.log"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Warnings != 0 {
		t.Errorf("Warnings = %d, want 0 — the whole nag is one advisory, not three warnings", c.Warnings)
	}
}

// TestWarnFilterKeepsPerHostState pins that giving each host its own
// WarnFilter is what makes interleaving safe: a second host's lines landing
// between an advisory's continuation lines must not break the fold, because
// each host's filter only ever sees that host's own lines in order.
func TestWarnFilterKeepsPerHostState(t *testing.T) {
	var gig, pi WarnFilter

	// gig's advisory line 1, then pi's unrelated real warning arrives
	// in between, then gig's own continuation lines.
	if gig.Warn(gcloudNag[0]) {
		t.Fatalf("gig's advisory line 1 counted as a warning")
	}
	if !pi.Warn(Line{Stderr: true, Text: "fatal: could not read Username for 'https://github.com'"}) {
		t.Fatalf("pi's real warning, interleaved between gig's lines, must still count")
	}
	if gig.Warn(gcloudNag[1]) {
		t.Fatalf("gig's continuation line 2 counted as a warning despite pi's line landing in between")
	}
	if gig.Warn(gcloudNag[2]) {
		t.Fatalf("gig's continuation line 3 counted as a warning despite pi's line landing in between")
	}
}

// TestWarnFilterCountsARealWarningRightAfterAnAdvisory pins that folding is
// bounded to what actually LOOKS LIKE a continuation: a real, independent
// warning arriving straight after an advisory is not swallowed by it just
// because it comes next.
func TestWarnFilterCountsARealWarningRightAfterAnAdvisory(t *testing.T) {
	var f WarnFilter
	if f.Warn(Line{Stderr: true, Text: "[notice] A new release of pip is available: 26.0.1 -> 26.2.1"}) {
		t.Fatalf("the pip notice itself must not count")
	}
	if !f.Warn(Line{Stderr: true, Text: "fatal: could not read Username for 'https://github.com'"}) {
		t.Fatalf("a real warning right after an advisory must still count")
	}
}

// TestWarnFilterExemptionRequiresAPrecedingAdvisory pins the boundary the
// spec calls out explicitly: the "ends with , or :" continuation shape is
// only an exemption when the line it follows was itself an advisory. A real
// warning that happens to end with ':' must not turn its own next line into
// a free pass.
func TestWarnFilterExemptionRequiresAPrecedingAdvisory(t *testing.T) {
	var f WarnFilter
	if !f.Warn(Line{Stderr: true, Text: "ERROR: could not resolve the following packages:"}) {
		t.Fatalf("the real warning ending in ':' must itself count")
	}
	if !f.Warn(Line{Stderr: true, Text: "  some-package"}) {
		t.Fatalf("a line after a REAL (non-advisory) warning must not be exempted just because the prior line ended in ':'")
	}
}

// TestWarnFilterExcludesNpmWarnAndPipNotice pins the two other advisory
// shapes the spec names. Before Change B these counted as warnings in the
// LIVE path (updexec.Benign has no notion of advisories — only
// histindex.Problems did), which is exactly the badge/digest disagreement
// this filter exists to close.
func TestWarnFilterExcludesNpmWarnAndPipNotice(t *testing.T) {
	var f WarnFilter
	cases := []Line{
		{Stderr: true, Text: "npm warn install-scripts 2 packages have install scripts"},
		{Stderr: true, Text: "[notice] A new release of pip is available: 26.0.1 -> 26.2.1"},
	}
	for _, l := range cases {
		if f.Warn(l) {
			t.Errorf("Warn(%q) = true, want false — it is an advisory", l.Text)
		}
	}
}

// TestWarnFilterIgnoresStdout pins that only stderr lines are ever counted —
// the same rule updexec.Benign's callers have always relied on.
func TestWarnFilterIgnoresStdout(t *testing.T) {
	var f WarnFilter
	if f.Warn(Line{Stderr: false, Text: "WARNING: this is stdout, not stderr"}) {
		t.Fatalf("a stdout line must never be counted as a warning")
	}
}
