package gh

// RepoInfo is the gss view of the current repository, populated from
// `gh repo view --json name,owner,nameWithOwner,defaultBranchRef` (see
// ParseRepoInfo).
//
// Field mapping to gh's --json keys:
//
//	Owner         <- owner.login
//	Name          <- name
//	NameWithOwner <- nameWithOwner   (the canonical "<owner>/<repo>" NWO)
//	DefaultBranch <- defaultBranchRef.name
//
// NameWithOwner is the value the registry caches as the resolved NWO (see
// design.md → "GitHub interaction" pre-flight); DefaultBranch is the base
// classic-mode push/PR targets unless overridden.
type RepoInfo struct {
	Owner         string
	Name          string
	NameWithOwner string
	DefaultBranch string
}
