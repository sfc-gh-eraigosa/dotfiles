package render

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/term"
)

const ulOn, ulOff = "\x1b[4m", "\x1b[24m"

func TestPaintRuns_WrapsExactlyTheSpan(t *testing.T) {
	st := asciiStyle() // Links == "" ⇒ underline
	got := paintRuns(st, "root main PR#21", []LinkSpan{{Start: 10, End: 15, URL: wantPRURL}})
	want := "root main " + osc8Open(wantPRURL) + ulOn + "PR#21" + ulOff + osc8Close
	if got != want {
		t.Errorf("paintRuns = %q, want %q", got, want)
	}
}

func TestPaintRuns_PlainModeHasNoUnderline(t *testing.T) {
	st := asciiStyle()
	st.Links = "plain"
	got := paintRuns(st, "PR#21", []LinkSpan{{0, 5, wantPRURL}})
	if strings.Contains(got, ulOn) || !strings.Contains(got, osc8Open(wantPRURL)) {
		t.Errorf("plain mode: got %q", got)
	}
}

func TestPaintRuns_NoSpansIsIdentity(t *testing.T) {
	if got := paintRuns(asciiStyle(), "abc", nil); got != "abc" {
		t.Errorf("got %q", got)
	}
}

func TestValidateSpans_DropsBadOnes(t *testing.T) {
	spans := validateSpans(5, []LinkSpan{
		{3, 5, "u3"}, {0, 2, "u1"}, {1, 3, "overlap"}, {4, 9, "range"}, {0, 1, ""}, {2, 2, "empty"},
	})
	if len(spans) != 2 || spans[0].URL != "u1" || spans[1].URL != "u3" {
		t.Errorf("validateSpans = %+v", spans)
	}
}

func TestClipSpans(t *testing.T) {
	got := clipSpans([]LinkSpan{{0, 4, "a"}, {5, 9, "b"}, {10, 12, "c"}}, 7)
	if len(got) != 2 || got[1].End != 7 {
		t.Errorf("clipSpans = %+v", got)
	}
}

func TestReanchorSpans_SurvivesANSIStrip(t *testing.T) {
	orig := "x \x1b[38;5;5mPR#21\x1b[38;5;7m y"
	spans := []LinkSpan{{2, 2 + len("\x1b[38;5;5mPR#21\x1b[38;5;7m"), wantPRURL}}
	stripped := term.StripANSI(orig)
	got := reanchorSpans(orig, stripped, spans)
	if len(got) != 1 || stripped[got[0].Start:got[0].End] != "PR#21" {
		t.Errorf("reanchor = %+v over %q", got, stripped)
	}
}

func TestShiftSpans(t *testing.T) {
	got := shiftSpans([]LinkSpan{{0, 2, "glyph"}, {3, 7, "label"}}, 3)
	if len(got) != 1 || got[0].Start != 0 || got[0].End != 4 {
		t.Errorf("shiftSpans = %+v", got)
	}
}

func TestJoin_SpansZeroWidth_BothPaths(t *testing.T) {
	plain := []segmentBlock{{text: "main PR#21", colorKey: "repo_root"}}
	linked := []segmentBlock{{text: "main PR#21", colorKey: "repo_root",
		links: []LinkSpan{{0, 4, "https://github.com/o/r/tree/main"}, {5, 10, wantPRURL}}}}
	for _, st := range []style.Style{asciiStyle(), powerlineStyleFixture()} {
		if term.DisplayWidth(join(st, linked)) != term.DisplayWidth(join(st, plain)) {
			t.Errorf("spans changed width for %s", st.Separator)
		}
		out := join(st, linked)
		if strings.Count(out, "\x1b]8;;") != 4 { // 2 opens + 2 closes
			t.Errorf("unbalanced OSC 8 in %q", out)
		}
	}
}

func TestTruncateToWidth_ClipsSpans(t *testing.T) {
	blocks := []segmentBlock{{text: "a-long-repo-label PR#21", colorKey: "repo_root",
		links: []LinkSpan{{0, 17, "https://github.com/o/r/tree/x"}, {18, 23, wantPRURL}}}}
	out := truncateToWidth(blocks, asciiStyle(), 12)
	if len(out) != 1 || len(out[0].links) != 1 || out[0].links[0].End > len(out[0].text)-len(ellipsis) {
		t.Errorf("truncate: %+v", out)
	}
}

func TestTreeURL(t *testing.T) {
	if got := TreeURL("https://github.com/o/r", "feature/a b#1"); got != "https://github.com/o/r/tree/feature/a%20b%231" {
		t.Errorf("TreeURL = %q", got)
	}
	if TreeURL("https://gitlab.com/o/r", "main") != "" || TreeURL("https://github.com/o/r", "") != "" {
		t.Error("tree URL must be GitHub-only and need a branch")
	}
}

func TestFileURL(t *testing.T) {
	if got := FileURL("/home/u/my dir"); got != "file:///home/u/my%20dir" {
		t.Errorf("FileURL = %q", got)
	}
	if FileURL("") != "" || FileURL("rel") != "" {
		t.Error("relative or empty cwd must yield no URL")
	}
}

func TestTimeURL_Placeholders(t *testing.T) {
	loc, _ := time.LoadLocation("America/Los_Angeles")
	ts := time.Date(2026, 9, 5, 8, 0, 0, 0, loc)
	got := TimeURL("x?tz={tz}&c={tz_city}&i={iso_utc}&u={unix}&k={keep}", ts, loc)
	want := "x?tz=America/Los_Angeles&c=Los_Angeles&i=20260905T150000Z&u=" + strconv.FormatInt(ts.Unix(), 10) + "&k={keep}"
	if got != want {
		t.Errorf("TimeURL = %q want %q", got, want)
	}
	if TimeURL("", ts, loc) != "" || TimeURL(DefaultTimeURL, ts, time.UTC) != "https://time.is/UTC" {
		t.Error("empty template ⇒ no URL; UTC city = UTC")
	}
}

func TestModelFamily(t *testing.T) {
	cases := map[[2]string]string{
		{"claude-fable-5-1", "Fable"}:                              "fable",
		{"claude-opus-4-8", "Claude Opus 4.8 (1M context)"}:        "opus",
		{"", "Sonnet 5"}:                                           "sonnet",
		{"", "claude-haiku-4-5"}:                                   "haiku",
		{"claude-mythos-5-1", ""}:                                  "mythos",
		{"Gemini 3.5 Flash (Medium)", "Gemini 3.5 Flash (Medium)"}: "gemini",
		{"", "Mystery Model"}:                                      "",
		{"", ""}:                                                   "",
	}
	for in, want := range cases {
		if got := modelFamily(in[0], in[1]); got != want {
			t.Errorf("modelFamily(%q, %q) = %q want %q", in[0], in[1], got, want)
		}
	}
}

func TestModelURL(t *testing.T) {
	if got := ModelURL("", "claude-fable-5-1", "Fable"); got != "https://www.anthropic.com/claude/fable" {
		t.Errorf("built-in map: %q", got)
	}
	if got := ModelURL("", "Gemini 3.5 Flash (Medium)", "Gemini 3.5 Flash (Medium)"); got != DefaultGeminiModelURL {
		t.Errorf("gemini built-in: %q", got)
	}
	if got := ModelURL("", "", "Mystery"); got != "" {
		t.Errorf("unknown family must yield no URL, got %q", got)
	}
	if got := ModelURL("https://x/{family}/{model_id}/{display_name}", "claude-opus-5", "Opus 5"); got != "https://x/opus/claude-opus-5/Opus%205" {
		t.Errorf("template: %q", got)
	}
	if got := ModelURL("https://x/{family}", "", "Mystery"); got != "" {
		t.Errorf("template with unknown family must yield no URL, got %q", got)
	}
}

func TestVSCodeDevURLs(t *testing.T) {
	if got := VSCodeDevPRURL("https://github.com/o/r", 279); got != "https://vscode.dev/github/o/r/pull/279/changes" {
		t.Errorf("PR: %q", got)
	}
	if got := VSCodeDevTreeURL("https://github.com/o/r", "feature/a b"); got != "https://vscode.dev/github/o/r/tree/feature/a%20b" {
		t.Errorf("tree: %q", got)
	}
	if VSCodeDevPRURL("https://gitlab.com/o/r", 1) != "" || VSCodeDevTreeURL("https://github.com/o/r", "") != "" || VSCodeDevPRURL("https://github.com/o/r", 0) != "" {
		t.Error("non-GitHub, empty branch, or PR 0 must yield no URL")
	}
}
