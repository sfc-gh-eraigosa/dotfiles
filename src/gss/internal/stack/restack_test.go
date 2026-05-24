// Package stack_test verifies re-target math per src/gss/docs/plan.md
// PR-29: single-hop re-target on merge, recursive walk for multi-level
// stacks (parents before children), cycle prevention, and auto-promote
// eligibility.
package stack_test

import (
	stderrors "errors"
	"testing"

	"github.com/wenlock/dotfiles/gss/internal/stack"
)

// linear stack: api (base main) <- ui <- docs
func linear() []stack.Node {
	return []stack.Node{
		{Ref: "api", Branch: "b/api", BaseBranch: "main"},
		{Ref: "ui", Branch: "b/ui", BaseBranch: "b/api"},
		{Ref: "docs", Branch: "b/docs", BaseBranch: "b/ui"},
	}
}

func nodeRef(ns []stack.Node, ref string) stack.Node {
	for _, n := range ns {
		if n.Ref == ref {
			return n
		}
	}
	panic("no node " + ref)
}

func TestRetargetOnMerge_SingleHop(t *testing.T) {
	ns := linear()
	rts := stack.RetargetOnMerge(ns, nodeRef(ns, "api"))
	if len(rts) != 1 {
		t.Fatalf("retargets = %d; want 1 (only the direct child)", len(rts))
	}
	if rts[0].Ref != "ui" || rts[0].NewBase != "main" || rts[0].OldBase != "b/api" {
		t.Errorf("retarget = %+v; want ui b/api->main", rts[0])
	}
}

func TestRetargetOnMerge_FanOut(t *testing.T) {
	ns := []stack.Node{
		{Ref: "api", Branch: "b/api", BaseBranch: "main"},
		{Ref: "ui", Branch: "b/ui", BaseBranch: "b/api"},
		{Ref: "cli", Branch: "b/cli", BaseBranch: "b/api"},
	}
	rts := stack.RetargetOnMerge(ns, nodeRef(ns, "api"))
	if len(rts) != 2 {
		t.Fatalf("retargets = %d; want 2 (both direct children)", len(rts))
	}
	for _, rt := range rts {
		if rt.NewBase != "main" {
			t.Errorf("%s NewBase = %q; want main", rt.Ref, rt.NewBase)
		}
	}
}

func TestRetargetOnMerge_TopHasNoChildren(t *testing.T) {
	ns := linear()
	if rts := stack.RetargetOnMerge(ns, nodeRef(ns, "docs")); len(rts) != 0 {
		t.Errorf("merging the top should retarget nothing; got %+v", rts)
	}
}

func TestRestackOnto_RecursiveAffectedOrdered(t *testing.T) {
	ns := linear()
	rt, refs, err := stack.RestackOnto(ns, nodeRef(ns, "ui"), "develop")
	if err != nil {
		t.Fatalf("RestackOnto: %v", err)
	}
	if rt.Ref != "ui" || rt.NewBase != "develop" {
		t.Errorf("root retarget = %+v; want ui->develop", rt)
	}
	// ui + its descendant docs, parents before children.
	want := []string{"ui", "docs"}
	if len(refs) != len(want) {
		t.Fatalf("affected = %v; want %v", refs, want)
	}
	for i := range want {
		if refs[i] != want[i] {
			t.Errorf("affected[%d] = %q; want %q (parents before children)", i, refs[i], want[i])
		}
	}
}

func TestRestackOnto_RootOnly(t *testing.T) {
	ns := linear()
	_, refs, err := stack.RestackOnto(ns, nodeRef(ns, "docs"), "main")
	if err != nil {
		t.Fatalf("RestackOnto: %v", err)
	}
	if len(refs) != 1 || refs[0] != "docs" {
		t.Errorf("affected = %v; want just [docs]", refs)
	}
}

func TestRestackOnto_Cycle(t *testing.T) {
	cyc := []stack.Node{
		{Ref: "a", Branch: "b/a", BaseBranch: "b/b"},
		{Ref: "b", Branch: "b/b", BaseBranch: "b/a"},
	}
	if _, _, err := stack.RestackOnto(cyc, cyc[0], "main"); !stderrors.Is(err, stack.ErrCycle) {
		t.Errorf("RestackOnto(cycle) err = %v; want ErrCycle", err)
	}
}

func TestAutoPromoteChild(t *testing.T) {
	ns := linear() // api(bottom, base main) <- ui <- docs
	zero := func(string) int { return 0 }

	// Eligible: api is bottom, exactly one child (ui), restack_count 0.
	if child, ok := stack.AutoPromoteChild(ns, nodeRef(ns, "api"), "main", zero); !ok || child.Ref != "ui" {
		t.Errorf("eligible case = %+v, %v; want ui, true", child, ok)
	}
	// Disqualified: child restacked.
	restacked := func(ref string) int {
		if ref == "ui" {
			return 2
		}
		return 0
	}
	if _, ok := stack.AutoPromoteChild(ns, nodeRef(ns, "api"), "main", restacked); ok {
		t.Error("restack_count>0 child must not be eligible")
	}
	// Disqualified: not the bottom (ui's base is b/api, not main).
	if _, ok := stack.AutoPromoteChild(ns, nodeRef(ns, "ui"), "main", zero); ok {
		t.Error("non-bottom merge must not auto-promote")
	}
	// Disqualified: fan-out (2 children).
	fan := []stack.Node{
		{Ref: "api", Branch: "b/api", BaseBranch: "main"},
		{Ref: "ui", Branch: "b/ui", BaseBranch: "b/api"},
		{Ref: "cli", Branch: "b/cli", BaseBranch: "b/api"},
	}
	if _, ok := stack.AutoPromoteChild(fan, fan[0], "main", zero); ok {
		t.Error("fan-out must not auto-promote")
	}
}
