package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

var testNow = time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)

func TestRenderTableIsWorstFirst(t *testing.T) {
	rows := []Row{
		{Alias: "good", Class: "up-to-date", Commit: "0b8726e", Age: testNow.Add(-time.Hour)},
		{Alias: "stale", Class: "behind", Behind: 24, Commit: "1bc1928", Age: testNow.Add(-14 * 24 * time.Hour)},
		{Alias: "dead", Class: "unreachable"},
	}
	out := renderTable(rows, testNow)
	iDead, iStale, iGood := strings.Index(out, "dead"), strings.Index(out, "stale"), strings.Index(out, "good")
	if iDead >= iStale || iStale >= iGood {
		t.Fatalf("rows not worst-first:\n%s", out)
	}
	if !strings.Contains(out, "behind 24") {
		t.Fatalf("missing behind count:\n%s", out)
	}
}

func TestExitCodeNonZeroWhenAnyHostStale(t *testing.T) {
	if exitCode([]Row{{Class: "up-to-date"}}) != 0 {
		t.Fatal("all up-to-date must exit 0")
	}
	if exitCode([]Row{{Class: "up-to-date"}, {Class: "behind"}}) == 0 {
		t.Fatal("a stale host must exit non-zero")
	}
	if exitCode([]Row{{Class: "unreachable"}}) == 0 {
		t.Fatal("an unreachable host must exit non-zero")
	}
	if exitCode([]Row{{Class: "unknown"}}) == 0 {
		t.Fatal("an unknown host must exit non-zero")
	}
	if exitCode(nil) != 0 {
		t.Fatal("no hosts must exit 0")
	}
}

// fakeBaseline stands in for the local git repo.
type fakeBaseline struct {
	head     string
	ancestor map[string]bool
	behind   map[string]int
}

func (f fakeBaseline) Head() string { return f.head }
func (f fakeBaseline) Probe() string {
	return probeCmdFor("~/.local/state/dotfiles/install-stamp", "~/git/dotfiles")
}
func (f fakeBaseline) Compare(sha string) (bool, int) {
	return f.ancestor[sha], f.behind[sha]
}

func TestCollectClassifiesEachHost(t *testing.T) {
	stampOf := func(sha string) string {
		return "commit=" + sha + "\ninstalled_at=1754700000\nbranch=main\nhostname=h\n"
	}
	cur := strings.Repeat("a", 40)
	old := strings.Repeat("b", 40)
	r := runner.Fake{
		Out: map[string]string{"cur": stampOf(cur), "old": stampOf(old), "bare": ""},
		Err: map[string]error{"dead": runner.ErrFake},
	}
	base := fakeBaseline{head: cur, ancestor: map[string]bool{cur: true, old: true}, behind: map[string]int{old: 24}}
	hosts := []sshconf.Host{{Alias: "cur"}, {Alias: "old"}, {Alias: "bare"}, {Alias: "dead"}}

	got := map[string]Row{}
	for _, row := range collect(hosts, r, base, testNow) {
		got[row.Alias] = row
	}
	if got["cur"].Class != "up-to-date" {
		t.Errorf("cur = %+v", got["cur"])
	}
	if got["old"].Class != "behind" || got["old"].Behind != 24 {
		t.Errorf("old = %+v", got["old"])
	}
	if got["bare"].Class != "unknown" {
		t.Errorf("bare (no stamp) = %+v", got["bare"])
	}
	if got["dead"].Class != "unreachable" {
		t.Errorf("dead = %+v", got["dead"])
	}
	if len(got) != 4 {
		t.Fatalf("every host must appear exactly once, got %d", len(got))
	}
}

func TestRenderJSONIsParseable(t *testing.T) {
	rows := []Row{{Alias: "a", Class: "behind", Behind: 3, Commit: "abc1234", Age: testNow}}
	var parsed []map[string]any
	if err := json.Unmarshal([]byte(renderJSON(rows)), &parsed); err != nil {
		t.Fatalf("renderJSON is not valid JSON: %v", err)
	}
	if parsed[0]["alias"] != "a" || parsed[0]["status"] != "behind" {
		t.Fatalf("unexpected JSON shape: %v", parsed[0])
	}
}

// Bug 3: os.Exit inside RunE bypasses cobra's error path and cleanup, and
// makes the stale-exit path untestable. It must surface as a typed error.
func TestStaleFleetSurfacesAsTypedErrorNotOsExit(t *testing.T) {
	err := exitErrorFor([]Row{{Class: "behind", Behind: 2}})
	if err == nil {
		t.Fatal("a stale fleet must produce an error carrying the exit code")
	}
	var ee exitError
	if !errors.As(err, &ee) || ee.code != 1 {
		t.Fatalf("want exitError{1}, got %#v", err)
	}
	if exitErrorFor([]Row{{Class: "up-to-date"}}) != nil {
		t.Fatal("an all-current fleet must produce no error")
	}
}

// Bug 5: a CORRUPT stamp must not be silently indistinguishable from NO
// stamp — the parser is strict precisely to catch truncated writes.
func TestCorruptStampIsReportedDistinctlyFromNoStamp(t *testing.T) {
	base := fakeBaseline{head: strings.Repeat("a", 40)}
	r := runner.Fake{Out: map[string]string{
		"bare":    "",
		"corrupt": "commit=abc\ninstalled_at=nonsense\n",
	}}
	rows := collect([]sshconf.Host{{Alias: "bare"}, {Alias: "corrupt"}}, r, base, testNow)
	got := map[string]Row{}
	for _, r := range rows {
		got[r.Alias] = r
	}
	if got["bare"].Note != "" {
		t.Fatalf("a missing stamp needs no note, got %q", got["bare"].Note)
	}
	if got["corrupt"].Note == "" {
		t.Fatal("a corrupt stamp must carry a note so it is distinguishable from no stamp")
	}
	if !strings.Contains(renderTable(rows, testNow), got["corrupt"].Note) {
		t.Fatal("the note must reach the rendered table")
	}
}

// Regression: SilenceErrors (needed so the exitError path prints nothing)
// must not swallow REAL errors. Execute() is responsible for printing them.
func TestSilenceErrorsDoesNotHideRealErrors(t *testing.T) {
	if !statusCmd.SilenceErrors {
		t.Skip("SilenceErrors not set; nothing to guard")
	}
	src, err := os.ReadFile("root.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `fmt.Fprintln(os.Stderr, "Error:", err)`) {
		t.Fatal("Execute() must print non-exitError errors, or SilenceErrors makes them vanish")
	}
}

// --- the probe follows the PLAN's baseline repo (#351 feature 2) -----------

func TestProbeCmdForUsesTheDeclaredStampAndRepo(t *testing.T) {
	got := probeCmdFor("~/.local/state/playground/install-stamp", "~/github/org/playground")
	for _, want := range []string{
		"cat ~/.local/state/playground/install-stamp",
		"git -C ~/github/org/playground rev-parse --abbrev-ref HEAD",
		probeDelim,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("probe %q lacks %q", got, want)
		}
	}
	// It must not carry the dotfiles defaults once a plan says otherwise —
	// that was the bug this replaces (two hardcoded consts).
	if strings.Contains(got, "state/dotfiles") || strings.Contains(got, "git/dotfiles") {
		t.Fatalf("probe still hardcodes dotfiles: %q", got)
	}
}

func TestBuiltInPlanProbeIsByteIdenticalToTheOldConstants(t *testing.T) {
	// The regression guard for every existing user: with no plan of their
	// own, the command that goes over the wire must not change at all.
	r, ok := updplan.Default().BaselineRepo()
	if !ok {
		t.Fatal("built-in plan has no baseline repo")
	}
	want := "cat ~/.local/state/dotfiles/install-stamp 2>/dev/null; echo '" + probeDelim + "'; " +
		"git -C ~/git/dotfiles rev-parse --abbrev-ref HEAD 2>/dev/null || true"
	if got := probeCmdFor(r.Stamp, r.Path); got != want {
		t.Fatalf("probe changed for the built-in plan:\n got %q\nwant %q", got, want)
	}
}

func TestBaselineTargetFromPlanRefusesARepoWithNoStamp(t *testing.T) {
	// "Plan must declare it": a baseline repo with no stamp has no status
	// data, and saying so beats inventing a path no entry point writes.
	p, err := updplan.Parse([]byte("version: 1\nupdate:\n  repos:\n    solo: {path: ~/x}\n  steps:\n    - {id: s, kind: sync, repo: solo}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := baselineTarget(p); err == nil {
		t.Fatal("a baseline repo with no stamp was accepted")
	} else if !strings.Contains(err.Error(), "stamp") {
		t.Fatalf("error %q does not name the missing field", err)
	}

	// ...and an ambiguous plan says which field to add, not "unknown".
	amb, err := updplan.Parse([]byte("version: 1\nupdate:\n  repos:\n    a: {path: ~/a}\n    b: {path: ~/b}\n  steps:\n    - {id: s, kind: sync, repo: a}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := baselineTarget(amb); err == nil || !strings.Contains(err.Error(), "baseline") {
		t.Fatalf("an ambiguous plan should point at `baseline:`, got %v", err)
	}
}

func TestBaselineTargetFromPlanReturnsStampAndPath(t *testing.T) {
	p, err := updplan.Parse([]byte("version: 1\nupdate:\n  baseline: pg\n  repos:\n    pg: {path: ~/github/org/pg, stamp: ~/.local/state/pg/install-stamp}\n  steps:\n    - {id: s, kind: sync, repo: pg}\n"))
	if err != nil {
		t.Fatal(err)
	}
	stamp, path, err := baselineTarget(p)
	if err != nil {
		t.Fatal(err)
	}
	if stamp != "~/.local/state/pg/install-stamp" || path != "~/github/org/pg" {
		t.Fatalf("baselineTarget = %q %q", stamp, path)
	}
}

func TestAPreExistingDotfilesPlanKeepsItsStampWithoutDeclaringOne(t *testing.T) {
	// Every plan written before this feature — the tracked team plan, and any
	// hand-written ~/.config/fleet/fleet.yaml — has a `dotfiles` repo and no
	// `stamp:`. Requiring one there would turn `fleet status` into a hard
	// failure for every existing user on their next upgrade.
	p, err := updplan.Parse([]byte("version: 1\nupdate:\n  root: ~/git\n  repos:\n    dotfiles: {path: dotfiles, branches: [main]}\n  steps:\n    - {id: s, kind: sync, repo: dotfiles}\n"))
	if err != nil {
		t.Fatal(err)
	}
	stamp, path, err := baselineTarget(p)
	if err != nil {
		t.Fatalf("a pre-existing dotfiles plan must still work: %v", err)
	}
	if stamp != "~/.local/state/dotfiles/install-stamp" {
		t.Fatalf("legacy stamp = %q", stamp)
	}
	if path != "~/git/dotfiles" {
		t.Fatalf("legacy path = %q", path)
	}
	// The compatibility shim is for the historical repo NAME only — any other
	// repo still has to declare its stamp.
	other, err := updplan.Parse([]byte("version: 1\nupdate:\n  repos:\n    playground: {path: ~/x}\n  steps:\n    - {id: s, kind: sync, repo: playground}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := baselineTarget(other); err == nil {
		t.Fatal("a non-dotfiles repo with no stamp was accepted")
	}
}
