package updplan

import "testing"

// TestOrderIsTopologicalAndStable: two independent chains interleave by
// declaration index, and every need precedes its dependent.
func TestOrderIsTopologicalAndStable(t *testing.T) {
	yaml := `version: 1
update:
  repos: {}
  steps:
    - { id: a1, kind: run, run: echo hi }
    - { id: b1, kind: run, run: echo hi }
    - { id: a2, kind: run, run: echo hi, needs: [a1] }
    - { id: b2, kind: run, run: echo hi, needs: [b1] }
`
	p, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	for i := 0; i < 20; i++ {
		order := p.Order()
		ids := make([]string, len(order))
		pos := make(map[string]int, len(order))
		for i, st := range order {
			ids[i] = st.ID
			pos[st.ID] = i
		}

		want := []string{"a1", "b1", "a2", "b2"}
		if len(ids) != len(want) {
			t.Fatalf("Order() = %v, want %v", ids, want)
		}
		for j := range want {
			if ids[j] != want[j] {
				t.Fatalf("Order() = %v, want %v (declaration-index-stable)", ids, want)
			}
		}

		for _, st := range order {
			for _, need := range st.Needs {
				if pos[need] >= pos[st.ID] {
					t.Fatalf("need %q does not precede dependent %q in %v", need, st.ID, ids)
				}
			}
		}
	}
}

// TestDependentsIsTransitive: a->b->c, a->d: Dependents("a") == [b c d] in
// Order() order; Dependents("c") == [].
func TestDependentsIsTransitive(t *testing.T) {
	yaml := `version: 1
update:
  repos: {}
  steps:
    - { id: a, kind: run, run: echo hi }
    - { id: b, kind: run, run: echo hi, needs: [a] }
    - { id: c, kind: run, run: echo hi, needs: [b] }
    - { id: d, kind: run, run: echo hi, needs: [a] }
`
	p, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	got := p.Dependents("a")
	want := []string{"b", "c", "d"}
	if len(got) != len(want) {
		t.Fatalf("Dependents(a) = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Dependents(a) = %v, want %v", got, want)
		}
	}

	if got := p.Dependents("c"); len(got) != 0 {
		t.Errorf("Dependents(c) = %v, want []", got)
	}
}

// TestLastStepUsingRepo: r.sync -> r.build -> other.sync ->
// LastStepUsing("r") == "r.build".
func TestLastStepUsingRepo(t *testing.T) {
	yaml := `version: 1
update:
  repos:
    r: { path: r, branches: [main] }
    other: { path: other, branches: [main] }
  steps:
    - { id: r.sync, kind: sync, repo: r }
    - { id: r.build, kind: run, run: echo hi, repo: r, needs: [r.sync] }
    - { id: other.sync, kind: sync, repo: other, needs: [r.build] }
`
	p, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	got, ok := p.LastStepUsing("r")
	if !ok {
		t.Fatal("LastStepUsing(r) not found")
	}
	if got != "r.build" {
		t.Errorf("LastStepUsing(r) = %q, want r.build", got)
	}
}

func parseEntryPlan(t *testing.T, yaml string) Plan {
	t.Helper()
	p, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	return p
}

// TestEntryIsTheBaselineRepoLastRunStep: the header names what the plan will
// run — the baseline repo's last run step in execution order, not the sync.
func TestEntryIsTheBaselineRepoLastRunStep(t *testing.T) {
	p := parseEntryPlan(t, `version: 1
update:
  repos:
    playground: { path: ~/github/x/playground }
  steps:
    - { id: playground.sync, kind: sync, repo: playground }
    - { id: playground.converge, kind: run, repo: playground, run: ./install.sh, needs: [playground.sync] }
`)
	e, ok := p.Entry()
	if !ok {
		t.Fatal("Entry() ok = false, want true")
	}
	if e.Repo.Name != "playground" || e.Run != "./install.sh" || e.Others != 0 {
		t.Fatalf("Entry() = %+v, want playground ./install.sh +0", e)
	}
}

// TestEntryCountsTheOtherRunSteps: a multi-repo plan still has one headline,
// and Others says there is more than it shows.
func TestEntryCountsTheOtherRunSteps(t *testing.T) {
	p := parseEntryPlan(t, `version: 1
update:
  baseline: playground
  repos:
    dotfiles: { path: dotfiles }
    playground: { path: ~/github/x/playground }
  steps:
    - { id: pg.install, kind: run, repo: playground, run: ./install.sh, needs: [df.install] }
    - { id: df.install, kind: run, repo: dotfiles, run: ./install.sh }
    - { id: pg.post, kind: run, repo: playground, run: make check, needs: [pg.install] }
`)
	e, ok := p.Entry()
	if !ok || e.Repo.Name != "playground" || e.Run != "make check" || e.Others != 2 {
		t.Fatalf("Entry() = %+v ok=%v, want playground `make check` +2", e, ok)
	}
}

// TestEntryWithoutABaselineRunFallsBackToTheLastRun: no run step on the
// baseline repo still names whatever the plan runs last.
func TestEntryWithoutABaselineRunFallsBackToTheLastRun(t *testing.T) {
	p := parseEntryPlan(t, `version: 1
update:
  baseline: dotfiles
  repos:
    dotfiles: { path: dotfiles }
    tools: { path: tools }
  steps:
    - { id: df.sync, kind: sync, repo: dotfiles }
    - { id: tools.build, kind: run, repo: tools, run: make, needs: [df.sync] }
`)
	e, ok := p.Entry()
	if !ok || e.Repo.Name != "tools" || e.Run != "make" || e.Others != 0 {
		t.Fatalf("Entry() = %+v ok=%v, want tools make +0", e, ok)
	}
}

// TestEntryOfTheBuiltInPlanIsDotfilesInstall: the default plan is exactly
// today's fetch → ff-only → ./install.sh.
func TestEntryOfTheBuiltInPlanIsDotfilesInstall(t *testing.T) {
	e, ok := Default().Entry()
	if !ok || e.Repo.Name != "dotfiles" || e.Run != "./install.sh" {
		t.Fatalf("Default().Entry() = %+v ok=%v, want dotfiles ./install.sh", e, ok)
	}
}

// TestEntryOfASyncOnlyPlanIsNothing: no run step means nothing to name.
func TestEntryOfASyncOnlyPlanIsNothing(t *testing.T) {
	p := parseEntryPlan(t, `version: 1
update:
  repos:
    dotfiles: { path: dotfiles }
  steps:
    - { id: df.sync, kind: sync, repo: dotfiles }
`)
	if e, ok := p.Entry(); ok {
		t.Fatalf("Entry() = %+v, want ok=false for a plan with no run step", e)
	}
}
