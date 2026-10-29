package cmd

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

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
    playground: { path: ~/github/x/playground }
  steps:
    - { id: playground.sync, kind: sync, repo: playground }
    - { id: playground.converge, kind: run, repo: playground, run: ./install.sh, needs: [playground.sync] }
`

// The header says WHAT `u` would run here: repo, entry script, and the
// commit the local checkout sits on.
func TestBannerNamesThePlanEntryAndCommit(t *testing.T) {
	m := testModel("a")
	m.plan = planOf(t, playgroundPlan)
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
	m.plan = planOf(t, playgroundPlan)
	got := stripANSI(m.planBadge(80))
	if got != planGlyph+" playground:./install.sh" {
		t.Fatalf("planBadge = %q, want no commit suffix", got)
	}
}

// A multi-run plan has one headline; +N says there is more than it shows.
func TestPlanBadgeCountsTheOtherRunSteps(t *testing.T) {
	m := testModel("a")
	m.plan = planOf(t, `version: 1
update:
  baseline: playground
  repos:
    dotfiles: { path: dotfiles }
    playground: { path: ~/github/x/playground }
  steps:
    - { id: df.install, kind: run, repo: dotfiles, run: ./install.sh }
    - { id: pg.install, kind: run, repo: playground, run: ./install.sh, needs: [df.install] }
`)
	m.entrySHA = "bb5db6a"
	if got := stripANSI(m.planBadge(80)); got != planGlyph+" playground:./install.sh +1 (bb5db6a)" {
		t.Fatalf("planBadge = %q", got)
	}
}

// A plan that runs nothing has nothing to headline.
func TestPlanBadgeIsEmptyForASyncOnlyPlan(t *testing.T) {
	m := testModel("a")
	m.plan = planOf(t, `version: 1
update:
  repos:
    dotfiles: { path: dotfiles }
  steps:
    - { id: df.sync, kind: sync, repo: dotfiles }
`)
	if got := m.planBadge(80); got != "" {
		t.Fatalf("planBadge = %q, want empty", got)
	}
}

// The host badge outranks the plan badge: on a narrow pane the plan badge is
// what gives way, and the line never overflows the panel.
func TestPlanBadgeGivesWayToTheHostBadge(t *testing.T) {
	for _, w := range []int{60, 80, 100, 140} {
		m := testModel("a")
		m.plan = planOf(t, playgroundPlan)
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

// expandLocalPath resolves a plan repo path on THIS machine.
func TestExpandLocalPath(t *testing.T) {
	cases := map[string]string{
		"~/git/dotfiles": "/home/u/git/dotfiles",
		"~":              "/home/u",
		"/srv/repo":      "/srv/repo",
	}
	for in, want := range cases {
		if got := expandLocalPath(in, "/home/u"); got != want {
			t.Errorf("expandLocalPath(%q) = %q, want %q", in, got, want)
		}
	}
}
