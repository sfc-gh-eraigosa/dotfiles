package stack

import "fmt"

// Retarget is one base-branch change to apply to a worker (and its PR).
type Retarget struct {
	Ref     string
	Branch  string
	OldBase string
	NewBase string
}

// RetargetOnMerge computes the re-targets when `merged` lands (design.md →
// "gss feature merged"): every worker based DIRECTLY on merged.Branch is
// re-targeted onto merged's former base (one stack level collapses).
// Descendants below the direct children keep their parent and are not
// touched. Order follows the input node order (deterministic).
func RetargetOnMerge(nodes []Node, merged Node) []Retarget {
	var out []Retarget
	for _, c := range Children(nodes, merged) {
		out = append(out, Retarget{
			Ref: c.Ref, Branch: c.Branch, OldBase: c.BaseBranch, NewBase: merged.BaseBranch,
		})
	}
	return out
}

// AutoPromoteChild returns the single direct child eligible for
// auto-promote-on-merge, or (Node{}, false). Eligible only when (1) the
// merged worker was the stack bottom (its base is the default branch), (2)
// it had exactly one direct child, and (3) that child's restack_count is 0
// (the lifetime invariant). restackCount looks up a worker_ref's count
// (registry-backed); a nil func treats all counts as 0.
func AutoPromoteChild(nodes []Node, merged Node, defaultBranch string, restackCount func(ref string) int) (Node, bool) {
	if merged.BaseBranch != defaultBranch {
		return Node{}, false
	}
	kids := Children(nodes, merged)
	if len(kids) != 1 {
		return Node{}, false
	}
	if restackCount != nil && restackCount(kids[0].Ref) != 0 {
		return Node{}, false
	}
	return kids[0], true
}

// RestackOnto computes the effect of re-basing root onto newBase (design.md
// → "gss feature restack --onto"): root's base_branch changes to newBase
// (the returned Retarget), and root plus every descendant has its effective
// base moved — so restack_count must increment on each. The affected refs
// are returned in parents-before-children (breadth-first) order so callers
// update PR bases top-down. Cycle-detected (ErrCycle).
func RestackOnto(nodes []Node, root Node, newBase string) (Retarget, []string, error) {
	affected, err := descendants(nodes, root)
	if err != nil {
		return Retarget{}, nil, err
	}
	rt := Retarget{Ref: root.Ref, Branch: root.Branch, OldBase: root.BaseBranch, NewBase: newBase}
	refs := make([]string, len(affected))
	for i, n := range affected {
		refs[i] = n.Ref
	}
	return rt, refs, nil
}

// descendants returns root plus all transitive children in breadth-first
// order (root, its children, then grandchildren — parents before children).
// Returns ErrCycle if the base links cycle.
func descendants(nodes []Node, root Node) ([]Node, error) {
	var out []Node
	seen := map[string]bool{}
	queue := []Node{root}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if seen[cur.Branch] {
			return nil, fmt.Errorf("%w: at %q", ErrCycle, cur.Branch)
		}
		seen[cur.Branch] = true
		out = append(out, cur)
		queue = append(queue, Children(nodes, cur)...)
	}
	return out, nil
}
