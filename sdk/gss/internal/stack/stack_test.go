// Package stack_test verifies parent/child computation per
// sdk/gss/docs/plan.md PR-27: parent/child walk, bottom-of-stack
// identification, and cycle detection.
package stack_test

import (
	stderrors "errors"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/stack"
)

// a 3-deep stack: api (base main) ← ui ← docs
func chain() []stack.Node {
	return []stack.Node{
		{Ref: "auth/alice/api", Branch: "feature/auth/alice/api", BaseBranch: "main"},
		{Ref: "auth/alice/ui", Branch: "feature/auth/alice/ui", BaseBranch: "feature/auth/alice/api"},
		{Ref: "auth/bob/docs", Branch: "feature/auth/bob/docs", BaseBranch: "feature/auth/alice/ui"},
	}
}

func node(nodes []stack.Node, ref string) stack.Node {
	for _, n := range nodes {
		if n.Ref == ref {
			return n
		}
	}
	panic("no node " + ref)
}

func TestParent(t *testing.T) {
	ns := chain()
	if p, ok := stack.Parent(ns, node(ns, "auth/alice/ui")); !ok || p.Ref != "auth/alice/api" {
		t.Errorf("Parent(ui) = %+v, %v; want api", p, ok)
	}
	if _, ok := stack.Parent(ns, node(ns, "auth/alice/api")); ok {
		t.Error("Parent(api) should be absent (based on main)")
	}
}

func TestChildren(t *testing.T) {
	ns := chain()
	kids := stack.Children(ns, node(ns, "auth/alice/api"))
	if len(kids) != 1 || kids[0].Ref != "auth/alice/ui" {
		t.Errorf("Children(api) = %+v; want [ui]", kids)
	}
	if k := stack.Children(ns, node(ns, "auth/bob/docs")); len(k) != 0 {
		t.Errorf("Children(docs) = %+v; want none (top of stack)", k)
	}
}

func TestIsBottom(t *testing.T) {
	ns := chain()
	if !stack.IsBottom(ns, node(ns, "auth/alice/api"), "main") {
		t.Error("api (base main) should be bottom")
	}
	if stack.IsBottom(ns, node(ns, "auth/alice/ui"), "main") {
		t.Error("ui should not be bottom")
	}
	// A node whose base is outside the set and not the default is its own bottom.
	orphan := stack.Node{Ref: "x", Branch: "b/x", BaseBranch: "some/external"}
	if !stack.IsBottom([]stack.Node{orphan}, orphan, "main") {
		t.Error("node with out-of-set base should be its own bottom")
	}
}

func TestAncestorsAndBottom(t *testing.T) {
	ns := chain()
	chainUp, err := stack.Ancestors(ns, node(ns, "auth/bob/docs"))
	if err != nil {
		t.Fatalf("Ancestors: %v", err)
	}
	wantOrder := []string{"auth/alice/api", "auth/alice/ui", "auth/bob/docs"}
	if len(chainUp) != 3 {
		t.Fatalf("chain len = %d; want 3", len(chainUp))
	}
	for i, ref := range wantOrder {
		if chainUp[i].Ref != ref {
			t.Errorf("chain[%d] = %q; want %q (bottom→node order)", i, chainUp[i].Ref, ref)
		}
	}
	b, err := stack.Bottom(ns, node(ns, "auth/bob/docs"))
	if err != nil || b.Ref != "auth/alice/api" {
		t.Errorf("Bottom(docs) = %+v, %v; want api", b, err)
	}
}

func TestCycleDetection(t *testing.T) {
	cyc := []stack.Node{
		{Ref: "a", Branch: "br/a", BaseBranch: "br/b"},
		{Ref: "b", Branch: "br/b", BaseBranch: "br/a"},
	}
	if _, err := stack.Ancestors(cyc, cyc[0]); !stderrors.Is(err, stack.ErrCycle) {
		t.Errorf("Ancestors(cycle) err = %v; want ErrCycle", err)
	}
	if _, err := stack.Bottom(cyc, cyc[0]); !stderrors.Is(err, stack.ErrCycle) {
		t.Errorf("Bottom(cycle) err = %v; want ErrCycle", err)
	}
}
