package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/feature"
)

var (
	checkpointWorker   string
	checkpointAuto     bool
	checkpointDryRun   bool
	checkpointBodyFrom string
	checkpointRefresh  bool
)

var featureCheckpointCmd = &cobra.Command{
	Use:   "checkpoint",
	Short: "Rebase, push, and create/update the worker's draft PR",
	Long: `Checkpoint the current worker: fetch, rebase on its base branch, render the
PR body, and create (first time) or update its draft PR, refreshing the stack
section across the feature. Resolves the worker from cwd unless --worker is
given. --auto is the non-interactive variant (safe for hooks): silent on
no-op, WIP-commits tracked changes, draft-only, and skips (non-zero) on any
condition that would prompt.

The PR body's free-form prose is written once, when the PR is created, and
kept on every later checkpoint. To replace it, pass --body-from <file> (the
file becomes the prose; "-" reads stdin) or --refresh-body (WORKER.md's Goal
and notes again). The gss-managed stack and feature-notes sections are
re-rendered either way.`,
	Run: func(cmd *cobra.Command, args []string) {
		svc, err := newFeatureService()
		if err != nil {
			fail(err)
		}
		ref := checkpointWorker
		if ref == "" {
			ref, err = currentWorkerRef()
			if err != nil {
				fail(err)
			}
		}

		if checkpointAuto && (checkpointBodyFrom != "" || checkpointRefresh) {
			fail(fmt.Errorf("checkpoint: --auto does not take --body-from or --refresh-body"))
		}
		if checkpointAuto {
			res, err := svc.AutoCheckpoint(context.Background(), feature.AutoOpts{WorkerRef: ref, DryRun: checkpointDryRun})
			if err != nil {
				fail(err) // skip conditions return an error; WORKER.md log already written
			}
			printAutoResult(cmd, res)
			return
		}

		body, err := readBodyFrom(cmd, checkpointBodyFrom)
		if err != nil {
			fail(err)
		}
		res, err := svc.Checkpoint(context.Background(), feature.CheckpointOpts{WorkerRef: ref, Body: body, RefreshBody: checkpointRefresh})
		if err != nil {
			fail(err)
		}
		verb := "updated"
		if res.Created {
			verb = "created"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Checkpoint %s: PR %s (%s) for %s\n", verb, res.PRURL, res.PRState, res.Ref)
	},
}

func printAutoResult(cmd *cobra.Command, r feature.AutoResult) {
	switch {
	case len(r.Planned) > 0:
		fmt.Fprintf(cmd.OutOrStdout(), "auto-checkpoint %s (dry-run):\n", r.Ref)
		for _, p := range r.Planned {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", p)
		}
	case r.NoOp:
		// Silent on no-op, per design.
	case r.Committed:
		fmt.Fprintf(cmd.OutOrStdout(), "auto-checkpoint %s: committed WIP and updated the draft PR\n", r.Ref)
	default:
		fmt.Fprintf(cmd.OutOrStdout(), "auto-checkpoint %s: updated the draft PR\n", r.Ref)
	}
}

func init() {
	f := featureCheckpointCmd.Flags()
	f.StringVar(&checkpointWorker, "worker", "", "Worker ref (default: resolved from cwd)")
	f.BoolVar(&checkpointAuto, "auto", false, "Non-interactive variant safe for hooks")
	f.BoolVar(&checkpointDryRun, "dry-run", false, "With --auto: print the plan, change nothing")
	f.StringVar(&checkpointBodyFrom, "body-from", "", "Replace the PR body's prose with this file (\"-\" = stdin); managed sections kept")
	f.BoolVar(&checkpointRefresh, "refresh-body", false, "Replace the PR body's prose with WORKER.md's Goal and notes again")
	featureCheckpointCmd.MarkFlagsMutuallyExclusive("body-from", "refresh-body")
	featureCmd.AddCommand(featureCheckpointCmd)
}

// readBodyFrom returns the contents of --body-from: "" when unset, stdin for
// "-", else the file. An empty file is an error, so a typo cannot blank a
// PR's description.
func readBodyFrom(cmd *cobra.Command, path string) (string, error) {
	if path == "" {
		return "", nil
	}
	var raw []byte
	var err error
	if path == "-" {
		raw, err = io.ReadAll(cmd.InOrStdin())
	} else {
		raw, err = os.ReadFile(path)
	}
	if err != nil {
		return "", fmt.Errorf("checkpoint --body-from: %w", err)
	}
	if strings.TrimSpace(string(raw)) == "" {
		return "", fmt.Errorf("checkpoint --body-from %s: the file is empty", path)
	}
	return string(raw), nil
}
