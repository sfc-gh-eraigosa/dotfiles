package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/tmux-mgr/pkg/agent"
)

// gssRunner runs a gss subcommand and returns combined output. Injectable so
// the worker-add orchestration is unit-testable without a real gss binary.
type gssRunner func(args ...string) ([]byte, error)

func defaultGssRunner(args ...string) ([]byte, error) {
	return exec.Command("gss", args...).CombinedOutput()
}

// workerAddResult is the JSON shape emitted by `gss feature worker add --json`.
type workerAddResult struct {
	WorkerRef    string `json:"worker_ref"`
	Branch       string `json:"branch"`
	WorktreePath string `json:"worktree_path"`
	BaseBranch   string `json:"base_branch"`
}

// gssWorkerAdd ensures the feature exists (best-effort) then adds a worker via
// gss, returning the parsed result. gss owns the worktree + repo identity, so
// this replaces the old local workspace.CreateWorkspace.
//
// NOTE: --pane-id is intentionally not passed: the tmux pane does not exist
// until after the worktree (which this call creates) is known, so the pane id
// is recorded in the tmux-mgr Session instead. spawned_by is informational
// only (resolution #8), so the empty pane_id there is harmless.
func gssWorkerAdd(run gssRunner, feature, purpose, description, user, engine, sessionID, tmuxMgrSession string) (workerAddResult, error) {
	// Ensure the feature exists; ignore an "already exists" error — the
	// worker add below fails clearly if the feature truly can't be created.
	_, _ = run("feature", "start", feature)

	out, err := run(
		"feature", "worker", "add",
		"--feature", feature,
		"--purpose", purpose,
		"--description", description,
		"--user", user,
		"--engine", engine,
		"--session-id", sessionID,
		"--tmux-mgr-session", tmuxMgrSession,
		"--json",
	)
	if err != nil {
		return workerAddResult{}, fmt.Errorf("gss feature worker add: %w\n%s", err, out)
	}
	var res workerAddResult
	if err := json.Unmarshal(out, &res); err != nil {
		return workerAddResult{}, fmt.Errorf("parse gss worker add JSON: %w\n%s", err, out)
	}
	if res.WorkerRef == "" || res.WorktreePath == "" {
		return workerAddResult{}, fmt.Errorf("gss worker add JSON missing worker_ref/worktree_path:\n%s", out)
	}
	return res, nil
}

// engineSessionID reads the engine-native session id from the environment,
// keyed by the detected host.
func engineSessionID(host agent.Assistant, getenv func(string) string) string {
	switch host {
	case agent.AssistantClaude:
		return getenv("CLAUDE_SESSION_ID")
	case agent.AssistantGemini:
		return getenv("GEMINI_SESSION_ID")
	default:
		return ""
	}
}

// shellQuote single-quotes s for safe embedding inside a /bin/sh -c string.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// wrapWithPaneWrap wraps an inner shell command so the agent runs under the
// pane-wrap shim, which auto-checkpoints the worker on the agent's exit
// (PR-54). The inner command is handed to `/bin/sh -c` after the `--`.
func wrapWithPaneWrap(exePath, workerRef, inner string) string {
	return fmt.Sprintf("%s internal pane-wrap --worker-ref %s -- /bin/sh -c %s",
		exePath, shellQuote(workerRef), shellQuote(inner))
}

// cleanupDeps are the injectable seams for cleanupSession.
type cleanupDeps struct {
	run      gssRunner
	killPane func(paneID string) error
	out      io.Writer
}

// cleanupSession tears down a worker (design.md → "What changes" #3 / "What
// goes away" #3-4): gss owns worktree removal via `gss feature done --worker
// <ref>` (forwarding --force) for gss-backed sessions. As of PR-59 tmux-mgr no
// longer removes worktrees directly, so a legacy session (no WorkerRef) is left
// in place with a pointer to `migrate-to-gss`. The tmux pane is killed
// afterwards in both cases; the caller removes the session file.
func cleanupSession(session *agent.Session, force bool, deps cleanupDeps) {
	if session.WorkerRef != "" {
		args := []string{"feature", "done", "--worker", session.WorkerRef}
		if force {
			args = append(args, "--force")
		}
		if out, err := deps.run(args...); err != nil {
			fmt.Fprintf(deps.out, "Warning: gss feature done failed: %s\n%s\n", err, out)
		}
	} else {
		fmt.Fprintf(deps.out, "Session %s has no gss worker ref; leaving worktree %s in place "+
			"(run `tmux-mgr internal migrate-to-gss` to adopt it into gss, then clean up via gss).\n",
			session.SessionID, session.WorktreePath)
	}
	if session.PaneID != "" {
		if err := deps.killPane(session.PaneID); err != nil {
			fmt.Fprintf(deps.out, "Warning: failed to kill pane %s: %s\n", session.PaneID, err)
		}
	}
}
