package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// withPlan swaps the model's plan the way newTUIModel would have set it,
// including the entry it caches.
func withPlan(m tuiModel, p updplan.Plan) tuiModel {
	m.plan = p
	m.entry, m.hasEntry = p.Entry()
	return m
}

func planOf(t *testing.T, yaml string) updplan.Plan {
	t.Helper()
	p, err := updplan.Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	return p
}

const playgroundPlan = `version: 1
update:
  repos:
    playground: { path: ~/github/x/playground, branches: [main] }
  steps:
    - { id: playground.sync, kind: sync, repo: playground }
    - { id: playground.converge, kind: run, repo: playground, run: ./install.sh, needs: [playground.sync] }
`

// The header says WHAT `u` would run here: repo, entry script, and the
// commit the local checkout sits on.
func TestBannerNamesThePlanEntryAndCommit(t *testing.T) {
	m := testModel("a")
	m = withPlan(m, planOf(t, playgroundPlan))
	m.vp = viewport{height: 16, width: 140}
	m.entrySHA = "bb5db6a"
	b := m.banner()
	if !strings.Contains(b, planGlyph+" playground:./install.sh (bb5db6a)") {
		t.Fatalf("banner must name the plan entry and commit:\n%s", b)
	}
}

// Without a resolvable checkout there is no commit to show — not "()".
func TestPlanBadgeOmitsAnUnknownCommit(t *testing.T) {
	m := testModel("a")
	m = withPlan(m, planOf(t, playgroundPlan))
	got := stripANSI(m.planBadge(80))
	if got != planGlyph+" playground:./install.sh" {
		t.Fatalf("planBadge = %q, want no commit suffix", got)
	}
}

// A multi-run plan has one headline; +N says there is more than it shows.
func TestPlanBadgeCountsTheOtherRunSteps(t *testing.T) {
	m := testModel("a")
	m = withPlan(m, planOf(t, `version: 1
update:
  baseline: playground
  repos:
    dotfiles: { path: dotfiles }
    playground: { path: ~/github/x/playground }
  steps:
    - { id: df.install, kind: run, repo: dotfiles, run: ./install.sh }
    - { id: pg.install, kind: run, repo: playground, run: ./install.sh, needs: [df.install] }
`))
	m.entrySHA = "bb5db6a"
	if got := stripANSI(m.planBadge(80)); got != planGlyph+" playground:./install.sh +1 (bb5db6a)" {
		t.Fatalf("planBadge = %q", got)
	}
}

// A plan that runs nothing has nothing to headline.
func TestPlanBadgeIsEmptyForASyncOnlyPlan(t *testing.T) {
	m := testModel("a")
	m = withPlan(m, planOf(t, `version: 1
update:
  repos:
    dotfiles: { path: dotfiles }
  steps:
    - { id: df.sync, kind: sync, repo: dotfiles }
`))
	if got := m.planBadge(80); got != "" {
		t.Fatalf("planBadge = %q, want empty", got)
	}
}

// The host badge outranks the plan badge: on a narrow pane the plan badge is
// what gives way, and the line never overflows the panel.
func TestPlanBadgeGivesWayToTheHostBadge(t *testing.T) {
	for _, w := range []int{60, 80, 100, 140} {
		m := testModel("a")
		m = withPlan(m, planOf(t, playgroundPlan))
		m.entrySHA = "bb5db6a"
		m.vp = viewport{height: 16, width: w}
		m.setLocal(localHost{Name: "host-pi"})
		for _, line := range strings.Split(stripANSI(m.banner()), "\n") {
			if lw := lipgloss.Width(line); lw > w {
				t.Fatalf("width %d: banner line is %d wide: %q", w, lw, line)
			}
		}
		if !strings.Contains(m.banner(), "host-pi") {
			t.Fatalf("width %d: the host badge must survive:\n%s", w, m.banner())
		}
	}
}

// A run: with control characters must not reach the terminal raw.
func TestPlanBadgeStripsControlCharacters(t *testing.T) {
	m := withPlan(testModel("a"), planOf(t, "version: 1\nupdate:\n  repos:\n    dotfiles: { path: dotfiles }\n  steps:\n    - { id: r, kind: run, repo: dotfiles, run: \"a\\rb\\x1b[2Jc\\td\" }\n"))
	got := stripANSI(m.planBadge(80))
	if strings.ContainsAny(got, "\r\t\x1b") || !strings.Contains(got, "dotfiles:a bc d") {
		t.Fatalf("planBadge = %q, want control characters neutralised", got)
	}
}

func TestEntryRefFollowsThePlanBranch(t *testing.T) {
	cases := []struct {
		branches []string
		want     string
	}{
		{[]string{"main"}, "origin/main"},
		{[]string{"release", "main"}, "origin/release"},
		{[]string{"default"}, "origin/HEAD"},
		{nil, ""},
	}
	for _, c := range cases {
		if got := entryRef(updplan.Repo{Branches: c.branches}); got != c.want {
			t.Errorf("entryRef(%v) = %q, want %q", c.branches, got, c.want)
		}
	}
}

// The badge names what `u` DELIVERS: origin/<branch> in the plan path's
// clone, with --update-ref folded in — not the local HEAD, not --repo.
func TestResolveEntrySHAFollowsTheUpdateRef(t *testing.T) {
	p, err := planOf(t, playgroundPlan).WithRef("playground=feat/x")
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Entry()
	var gotDir, gotRef string
	rp := func(dir, ref string) string { gotDir, gotRef = dir, ref; return "1234567890ab" }
	got := resolveEntrySHA(e, ok, "/home/u", rp)
	if got != "1234567" || gotDir != "/home/u/github/x/playground" || gotRef != "origin/feat/x" {
		t.Fatalf("got %q from rev-parse(%q, %q), want 1234567 from the plan path at origin/feat/x", got, gotDir, gotRef)
	}
}

// Nothing resolvable means no (sha).
func TestResolveEntrySHAUnknownIsEmpty(t *testing.T) {
	p := planOf(t, playgroundPlan)
	e, ok := p.Entry()
	rp := func(string, string) string { return "" }
	if got := resolveEntrySHA(e, ok, "/home/u", rp); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

// git -C walks up: a plain directory inside another repo must not answer
// with the parent's commit.
func TestGitRevParseRefusesADirectoryInsideAnotherRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	top := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"-c", "user.email=", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "x"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", top}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if gitRevParse(top, "main") == "" {
		t.Fatal("the repo's own top must resolve")
	}
	sub := filepath.Join(top, "not-a-clone")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := gitRevParse(sub, "main"); got != "" {
		t.Fatalf("gitRevParse(subdir) = %q, want empty", got)
	}
}
