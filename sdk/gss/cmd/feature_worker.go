package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/feature"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/registry"
)

// featureWorkerCmd is the nested `gss feature worker` group. `add` lands here
// in PR-45; `update` is deferred — it needs an internal Service.WorkerUpdate
// (design.md → worker update), which Batch G did not implement.
var featureWorkerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Add or update a worker (a worktree) within a feature",
}

var (
	workerFeature        string
	workerPurpose        string
	workerDesc           string
	workerBase           string
	workerUser           string
	workerForce          bool
	workerGoal           string
	workerJSON           bool
	workerEngine         string
	workerSessionID      string
	workerPaneID         string
	workerTmuxMgrSession string
)

// buildSpawnedBy assembles the spawned_by provenance from the per-flag values,
// or nil when none are set (a human-started worker). The cmd stamps
// started_at; spawned_by is informational only (resolution #8).
func buildSpawnedBy(engine, sessionID, paneID, tmuxMgrSession, startedAt string) *registry.SpawnedBy {
	if engine == "" && sessionID == "" && paneID == "" && tmuxMgrSession == "" {
		return nil
	}
	return &registry.SpawnedBy{
		Engine: engine, SessionID: sessionID, PaneID: paneID,
		TmuxMgrSession: tmuxMgrSession, StartedAt: startedAt,
	}
}

// workerAddJSON renders the machine-readable result that callers (e.g.
// tmux-mgr) parse: worker_ref + the worktree/branch/base it created.
func workerAddJSON(res feature.WorkerResult) ([]byte, error) {
	return json.MarshalIndent(struct {
		WorkerRef    string `json:"worker_ref"`
		Branch       string `json:"branch"`
		WorktreePath string `json:"worktree_path"`
		BaseBranch   string `json:"base_branch"`
	}{
		WorkerRef:    res.Ref.String(),
		Branch:       res.Branch,
		WorktreePath: res.Worktree,
		BaseBranch:   res.Base,
	}, "", "  ")
}

var featureWorkerAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a worker worktree to a feature",
	Long: `Add a worker: allocate a unique worker_ref, create the worktree on its own
branch (based on the feature's default base unless --base overrides), and seed
WORKER.md. --description is required.`,
	Run: func(cmd *cobra.Command, args []string) {
		svc, err := newFeatureService()
		if err != nil {
			fail(err)
		}
		res, err := svc.WorkerAdd(context.Background(), feature.WorkerAddOpts{
			Feature:     workerFeature,
			Purpose:     workerPurpose,
			Description: workerDesc,
			BaseBranch:  workerBase,
			User:        workerUser,
			ForceSuffix: workerForce,
			Goal:        workerGoal,
			SpawnedBy:   buildSpawnedBy(workerEngine, workerSessionID, workerPaneID, workerTmuxMgrSession, time.Now().UTC().Format(time.RFC3339)),
		})
		if err != nil {
			fail(err)
		}
		if workerJSON {
			data, err := workerAddJSON(res)
			if err != nil {
				fail(err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return
		}
		fmt.Fprintf(cmd.OutOrStdout(),
			"Added worker %s\n  branch:   %s\n  worktree: %s\n  base:     %s\n",
			res.Ref.String(), res.Branch, res.Worktree, res.Base)
	},
}

func init() {
	f := featureWorkerAddCmd.Flags()
	f.StringVar(&workerFeature, "feature", "", "Feature name (required)")
	f.StringVar(&workerPurpose, "purpose", "", "Worker purpose, e.g. api/ui (required)")
	f.StringVar(&workerDesc, "description", "", "Worker description (required)")
	f.StringVar(&workerBase, "base", "", "Base branch (default: the feature's default base)")
	f.StringVar(&workerUser, "user", "", "User (default: resolved from git email / $USER)")
	f.BoolVar(&workerForce, "force-suffix", false, "Force a random suffix even when the bare ref is free")
	f.StringVar(&workerGoal, "goal", "", "Worker goal (seeds WORKER.md)")
	f.BoolVar(&workerJSON, "json", false, "Emit the created worker as JSON (worker_ref/branch/worktree_path/base_branch)")
	f.StringVar(&workerEngine, "engine", "", "spawned_by engine (e.g. claude/gemini/manual)")
	f.StringVar(&workerSessionID, "session-id", "", "spawned_by session id")
	f.StringVar(&workerPaneID, "pane-id", "", "spawned_by tmux pane id")
	f.StringVar(&workerTmuxMgrSession, "tmux-mgr-session", "", "spawned_by tmux-mgr session record id")
	_ = featureWorkerAddCmd.MarkFlagRequired("feature")
	_ = featureWorkerAddCmd.MarkFlagRequired("purpose")
	_ = featureWorkerAddCmd.MarkFlagRequired("description")

	featureWorkerCmd.AddCommand(featureWorkerAddCmd)
	featureCmd.AddCommand(featureWorkerCmd)
}
