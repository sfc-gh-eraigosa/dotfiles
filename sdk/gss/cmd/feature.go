package cmd

import "github.com/spf13/cobra"

// featureCmd is the parent of the `gss feature` subtree (design.md → "Command
// surface"): the worktree-based, registry-backed workflow for stacked feature
// branches and the AI/automation workers that drive them.
//
// Per the dotfiles cmd-leaf decision the layout stays FLAT — every leaf is in
// package cmd and attaches itself to featureCmd (or the nested `worker` group)
// via its own init() in a sibling file, rather than living under a
// cmd/gss/feature/ subpackage with an exported Register(parent). The leaves
// land across PR-45..47:
//
//	start, worker {add,update}, list   (PR-45)
//	checkpoint, conflicts, pr, rebase, restack (PR-46)
//	done, merged, audit                (PR-47)
//
// With no leaves attached yet, `gss feature --help` shows usage and an empty
// command list; each subsequent PR fills it in.
var featureCmd = &cobra.Command{
	Use:   "feature",
	Short: "Manage stacked feature-branch worktrees (the worker workflow)",
	Long: `Manage stacked feature branches as isolated git worktrees.

A "feature" groups one or more "workers" — each a dedicated worktree on its
own branch — recorded in the gss registry. Unlike the classic 'gss push' /
'gss pr' flow (which operates on the current checkout), the feature commands
operate by worker reference (feature/user/purpose) and maintain the stack:
parent/child base branches, PR stack sections, and re-targeting when a parent
merges.

Run a subcommand for the specific verb; see 'gss feature <verb> --help' for
flags. Classic 'gss push'/'pr'/'sync' are refused inside a worker worktree —
use these commands there instead.`,
}

func init() {
	rootCmd.AddCommand(featureCmd)
}
