// Package stack computes parent/child relationships within a stack of
// worker branches (design.md → "Stacked PRs"). A stack is a chain where
// each worker's base branch points at another worker's branch; the bottom
// is the worker based on the default branch (e.g. main).
//
// It is pure logic over a minimal Node (no registry/git/gh dependency, no
// external calls) — callers build []Node from registry workers.
package stack

import (
	stderrors "errors"
	"fmt"
)

// ErrCycle is returned when the base-branch links form a cycle.
var ErrCycle = stderrors.New("stack: cycle detected in base-branch links")

// Node is one worker in a stack.
type Node struct {
	Ref        string // worker_ref, for display/diagnostics
	Branch     string // this worker's branch
	BaseBranch string // the branch this one is based on
}

// index maps branch name → node for O(1) parent lookup.
func index(nodes []Node) map[string]Node {
	m := make(map[string]Node, len(nodes))
	for _, n := range nodes {
		m[n.Branch] = n
	}
	return m
}

// Parent returns the node whose Branch == n.BaseBranch, if present in the
// set (i.e. n is stacked on another worker rather than on the trunk).
func Parent(nodes []Node, n Node) (Node, bool) {
	p, ok := index(nodes)[n.BaseBranch]
	return p, ok
}

// Children returns the nodes based directly on n (n.Branch == child.BaseBranch),
// preserving input order.
func Children(nodes []Node, n Node) []Node {
	var out []Node
	for _, c := range nodes {
		if c.BaseBranch == n.Branch {
			out = append(out, c)
		}
	}
	return out
}

// IsBottom reports whether n is the bottom of its stack: its base is the
// default branch, or its base is not another node in the set.
func IsBottom(nodes []Node, n Node, defaultBranch string) bool {
	if n.BaseBranch == defaultBranch {
		return true
	}
	_, hasParent := index(nodes)[n.BaseBranch]
	return !hasParent
}

// Ancestors returns the chain from the bottom of the stack up to n
// (inclusive), ordered bottom → n. A node with no in-set parent is its own
// bottom. Returns ErrCycle if the base links cycle.
func Ancestors(nodes []Node, n Node) ([]Node, error) {
	idx := index(nodes)
	var chain []Node // n → … → bottom (we reverse at the end)
	seen := map[string]bool{}
	cur := n
	for {
		if seen[cur.Branch] {
			return nil, fmt.Errorf("%w: at %q", ErrCycle, cur.Branch)
		}
		seen[cur.Branch] = true
		chain = append(chain, cur)
		parent, ok := idx[cur.BaseBranch]
		if !ok {
			break // cur is the bottom
		}
		cur = parent
	}
	// Reverse to bottom → n order.
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, nil
}

// Bottom returns the bottom-most ancestor of n. Returns ErrCycle on a cycle.
func Bottom(nodes []Node, n Node) (Node, error) {
	chain, err := Ancestors(nodes, n)
	if err != nil {
		return Node{}, err
	}
	return chain[0], nil
}
