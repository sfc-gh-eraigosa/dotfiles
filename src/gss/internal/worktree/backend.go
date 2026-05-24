// Package worktree defines the backend abstraction gss uses to materialize
// and manage worktrees (design.md → "Worktree backend abstraction";
// resolution #13). Every caller talks to a worktree through the Backend
// interface, never to `git worktree` directly, so an alternative backend
// (overlayfs, tmpfs, …) can be added later without touching callers.
//
// The interface is scoped to worktree lifecycle only — commits, pushes, PR
// plumbing and rebases operate on a normal git repo at the worktree path,
// a contract every backend must preserve.
package worktree

// CreateReq describes a worktree to materialize.
type CreateReq struct {
	Path       string // absolute target path; the backend owns the inode
	Branch     string // branch to materialize at HEAD
	BaseBranch string // upstream branch (tracking + later rebase)
	BaseCommit string // optional pinned commit; "" → tip of BaseBranch
}

// Info is a backend's view of a materialized worktree.
type Info struct {
	Path       string
	Branch     string
	HeadSHA    string
	BaseBranch string
	Backend    string // "git", "overlayfs", …
}

// Status summarises a worktree's working state for `gss feature list`,
// `conflicts`, and `checkpoint --auto`.
type Status struct {
	Clean     bool
	Staged    []string
	Dirty     []string // tracked, modified, not staged
	Untracked []string
	Ahead     int // commits ahead of BaseBranch on origin
	Behind    int
}

// Backend is the worktree lifecycle interface. Implementations register
// themselves via Register and are obtained via Open.
type Backend interface {
	// Name returns the backend ID persisted in registry.json so a later
	// gss run knows which backend created a given worktree.
	Name() string

	// Create materializes a worktree per req. Implementations must be
	// idempotent enough that a partial failure can be retried after Remove.
	Create(req CreateReq) (Info, error)

	// Remove tears down the worktree. It must refuse when Status reports
	// the worktree dirty, unless force is true.
	Remove(path string, force bool) error

	// List enumerates the worktrees this backend manages under root.
	List(root string) ([]Info, error)

	// Status returns the working-state summary for the worktree at path.
	Status(path string) (Status, error)
}
