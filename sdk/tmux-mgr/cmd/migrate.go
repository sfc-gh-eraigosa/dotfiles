package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/tmux-mgr/pkg/agent"
)

// migrateGit runs `git -C <dir> <args...>` and returns stdout. Injectable so
// the migrator is unit-testable without a real repo.
type migrateGit func(dir string, args ...string) (string, error)

// migrateDeps are the injectable seams for runMigrateToGss.
type migrateDeps struct {
	sessions []agent.Session           // legacy sessions to process
	runGss   gssRunner                 // gss subcommand runner
	git      migrateGit                // git -C <dir> <args...>
	save     func(agent.Session) error // persist an updated session in place
	user     string                    // gss user (empty → gss resolves)
	out      io.Writer                 // human-readable plan / progress
}

// migrateResult is one row of the migration summary.
type migrateResult struct {
	SessionID string
	WorkerRef string
	Status    string
}

var migrateDryRun bool

// migrateToGssCmd is the one-shot, one-way migrator that adopts pre-refactor
// tmux-mgr sessions (which owned their own worktrees) into gss features. Per
// the design's "What's new" #5 / resolution #21, but using the mechanism the
// PR-55 `worker add --json --base` plumbing made available: rather than
// hand-rolling gss's registry across a module boundary, it shells out to
// `gss feature worker add --base <legacy-branch>`, which mints a canonical gss
// worker branched off the legacy branch's HEAD (so the legacy commits are
// preserved) and keeps gss the sole registry writer.
var migrateToGssCmd = &cobra.Command{
	Use:   "migrate-to-gss",
	Short: "One-shot migrate legacy tmux-mgr sessions into gss features (one-way)",
	Long: `migrate-to-gss adopts every legacy tmux-mgr session
(~/.config/tmux-mgr/sessions/*.json) into a gss feature/worker.

For each session that has no WorkerRef yet it resolves the worktree's current
branch, then runs "gss feature worker add --base <legacy-branch>", which mints
a canonical gss worker branched off the legacy HEAD (commits preserved). The
session JSON is updated in place with the new WorkerRef + worktree path. gss
remains the sole registry writer; tmux-mgr never touches registry.json.

This is one-way (no rollback) and best-effort: a per-session failure is logged
and skipped, the rest continue. Re-runs are idempotent — an already-migrated
session (WorkerRef set) is skipped. --dry-run prints every planned action
without executing. A summary is written to ~/.config/gss/migrate-to-gss.log.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		sessions, err := agent.ListSessions()
		if err != nil {
			return fmt.Errorf("migrate-to-gss: list sessions: %w", err)
		}
		deps := migrateDeps{
			sessions: sessions,
			runGss:   defaultGssRunner,
			git: func(dir string, a ...string) (string, error) {
				out, err := exec.Command("git", append([]string{"-C", dir}, a...)...).Output()
				return string(out), err
			},
			save: agent.SaveSession,
			user: os.Getenv("USER"),
			out:  cmd.OutOrStdout(),
		}
		results := runMigrateToGss(deps, migrateDryRun)
		summary := formatSummary(results)
		fmt.Fprint(cmd.OutOrStdout(), summary)
		if !migrateDryRun {
			if err := appendMigrateLog(summary); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "migrate-to-gss: could not write audit log: %v\n", err)
			}
		}
		return nil
	},
}

// runMigrateToGss processes every legacy session in a deterministic order and
// returns one result row per session. It never aborts the batch: a per-session
// failure is captured in its row and processing continues.
func runMigrateToGss(deps migrateDeps, dryRun bool) []migrateResult {
	var results []migrateResult
	for _, s := range deps.sessions {
		results = append(results, migrateOne(deps, s, dryRun))
	}
	return results
}

func migrateOne(deps migrateDeps, s agent.Session, dryRun bool) migrateResult {
	r := migrateResult{SessionID: s.SessionID}

	if s.WorkerRef != "" { // idempotent: already migrated
		r.WorkerRef = s.WorkerRef
		r.Status = "skipped (already migrated)"
		return r
	}
	if s.WorktreePath == "" {
		r.Status = "skipped (no worktree path)"
		return r
	}

	base, err := resolveLegacyBase(deps.git, s.WorktreePath)
	if err != nil {
		r.Status = fmt.Sprintf("FAILED (resolve branch: %v)", err)
		return r
	}

	feature := deriveFeature(s.AgentName)
	desc := fmt.Sprintf("migrated from tmux-mgr session %s", s.SessionID)
	addArgs := workerAddArgsForMigration(feature, "main", base, deps.user, s.SessionID, desc)

	if dryRun {
		fmt.Fprintf(deps.out, "DRY-RUN %s: gss feature start %s && gss %s\n",
			s.SessionID, feature, strings.Join(addArgs, " "))
		r.Status = fmt.Sprintf("dry-run (→ feature %s, base %s)", feature, base)
		return r
	}

	_, _ = deps.runGss("feature", "start", feature) // best-effort; worker add fails clearly if the feature truly can't exist
	out, err := deps.runGss(addArgs...)
	if err != nil {
		r.Status = fmt.Sprintf("FAILED (gss worker add: %v)", err)
		return r
	}
	var wa workerAddResult
	if jerr := json.Unmarshal(out, &wa); jerr != nil || wa.WorkerRef == "" || wa.WorktreePath == "" {
		r.Status = fmt.Sprintf("FAILED (parse worker add JSON: %v)", jerr)
		return r
	}

	s.WorkerRef = wa.WorkerRef
	s.WorktreePath = wa.WorktreePath // gss owns the new worktree; legacy path retained until manual cleanup
	if serr := deps.save(s); serr != nil {
		r.Status = fmt.Sprintf("FAILED (save session: %v)", serr)
		return r
	}
	r.WorkerRef = wa.WorkerRef
	r.Status = "migrated"
	return r
}

// resolveLegacyBase returns the branch to adopt as the new worker's base: the
// worktree's current branch, or the commit SHA when HEAD is detached.
func resolveLegacyBase(git migrateGit, worktreePath string) (string, error) {
	branch, err := git(worktreePath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	base := strings.TrimSpace(branch)
	if base == "HEAD" || base == "" {
		sha, err := git(worktreePath, "rev-parse", "HEAD")
		if err != nil {
			return "", err
		}
		base = strings.TrimSpace(sha)
	}
	return base, nil
}

// deriveFeature maps a legacy agent name to a synthetic, gss-valid feature
// segment, prefixed "migrated-" so adopted features never collide with active
// ones (design step d).
func deriveFeature(agentName string) string {
	base := kebab(agentName)
	if base == "" {
		return "migrated"
	}
	return "migrated-" + base
}

// kebab lowercases s and collapses every run of non-[a-z0-9] into a single
// hyphen, trimming leading/trailing hyphens.
func kebab(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash && b.Len() > 0 {
			b.WriteByte('-')
			prevDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// workerAddArgsForMigration builds the `gss feature worker add` argv that
// adopts a legacy branch as a new worker. spawned_by is stamped manual; the
// engine session is gone by definition, so session-id/pane-id are the literal
// "migrated" (resolution #8: spawned_by is informational only).
func workerAddArgsForMigration(feature, purpose, base, user, legacyID, description string) []string {
	args := []string{
		"feature", "worker", "add",
		"--feature", feature,
		"--purpose", purpose,
		"--base", base,
		"--description", description,
	}
	if user != "" {
		args = append(args, "--user", user)
	}
	args = append(args,
		"--engine", "manual",
		"--session-id", "migrated",
		"--pane-id", "migrated",
		"--tmux-mgr-session", legacyID,
		"--json",
	)
	return args
}

// formatSummary renders the per-session result table (also written to the
// audit log): "<legacy-id>  →  <worker-ref>  (status)".
func formatSummary(results []migrateResult) string {
	var b strings.Builder
	b.WriteString("migrate-to-gss summary:\n")
	for _, r := range results {
		ref := r.WorkerRef
		if ref == "" {
			ref = "-"
		}
		fmt.Fprintf(&b, "  %s  →  %s  (%s)\n", r.SessionID, ref, r.Status)
	}
	return b.String()
}

// appendMigrateLog appends a timestamped summary block to
// ~/.config/gss/migrate-to-gss.log for audit.
func appendMigrateLog(summary string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".config", "gss")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, "migrate-to-gss.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "=== %s ===\n%s\n", time.Now().UTC().Format(time.RFC3339), summary)
	return err
}

func init() {
	migrateToGssCmd.Flags().BoolVar(&migrateDryRun, "dry-run", false, "Print every planned action without executing")
	internalCmd.AddCommand(migrateToGssCmd)
}
