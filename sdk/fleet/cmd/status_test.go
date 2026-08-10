package cmd

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
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
	if !(iDead < iStale && iStale < iGood) {
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
