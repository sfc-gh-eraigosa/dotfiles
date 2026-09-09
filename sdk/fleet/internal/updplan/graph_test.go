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
