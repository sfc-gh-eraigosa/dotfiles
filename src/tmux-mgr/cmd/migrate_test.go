package cmd

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/eraigosa/dotfiles/src/tmux-mgr/pkg/agent"
)

// --- fakes -----------------------------------------------------------------

// recordingGss returns a gssRunner that records every call and replies with
// addJSON for `worker add`, optionally failing when failOn says so.
func recordingGss(calls *[][]string, addJSON string, failOn func(args []string) error) gssRunner {
	return func(args ...string) ([]byte, error) {
		*calls = append(*calls, append([]string{}, args...))
		if failOn != nil {
			if err := failOn(args); err != nil {
				return []byte("gss boom"), err
			}
		}
		if slices.Contains(args, "add") {
			return []byte(addJSON), nil
		}
		return nil, nil
	}
}

// fakeGit returns abbrevBranch for `rev-parse --abbrev-ref HEAD` and a fixed
// SHA otherwise; err (if set) fails every call.
func fakeGit(abbrevBranch string, err error) migrateGit {
	return func(dir string, args ...string) (string, error) {
		if err != nil {
			return "", err
		}
		if slices.Contains(args, "--abbrev-ref") {
			return abbrevBranch + "\n", nil
		}
		return "deadbeefcafef00d\n", nil
	}
}

const sampleAddJSON = `{
  "worker_ref": "migrated-generalist/erai/main",
  "branch": "feature/migrated-generalist/erai/main",
  "worktree_path": "/wt/owner/repo/migrated-generalist/erai/main",
  "base_branch": "generalist-1700000000000"
}`

// --- pure helpers ----------------------------------------------------------

func TestDeriveFeature(t *testing.T) {
	cases := map[string]string{
		"generalist":     "migrated-generalist",
		"My Cool Agent!": "migrated-my-cool-agent",
		"  spaced  ":     "migrated-spaced",
		"UPPER":          "migrated-upper",
		"already-kebab":  "migrated-already-kebab",
		"":               "migrated",
		"!!!":            "migrated",
	}
	for in, want := range cases {
		if got := deriveFeature(in); got != want {
			t.Errorf("deriveFeature(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestWorkerAddArgsForMigration(t *testing.T) {
	args := workerAddArgsForMigration("migrated-gen", "main", "gen-123", "erai", "sess-1", "desc here")
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"feature worker add",
		"--feature migrated-gen",
		"--purpose main",
		"--base gen-123",
		"--user erai",
		"--engine manual",
		"--session-id migrated",
		"--pane-id migrated",
		"--tmux-mgr-session sess-1",
		"--json",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q\n  got: %s", want, joined)
		}
	}
}

func TestWorkerAddArgsForMigration_OmitsEmptyUser(t *testing.T) {
	args := workerAddArgsForMigration("f", "main", "b", "", "s", "d")
	if slices.Contains(args, "--user") {
		t.Errorf("empty user must not emit --user flag\n  got: %v", args)
	}
}

// --- orchestration ---------------------------------------------------------

func TestRunMigrate_SkipsAlreadyMigrated(t *testing.T) {
	var calls [][]string
	deps := migrateDeps{
		sessions: []agent.Session{{SessionID: "s1", AgentName: "generalist", WorktreePath: "/wt/old", WorkerRef: "feat/erai/main"}},
		runGss:   recordingGss(&calls, sampleAddJSON, nil),
		git:      fakeGit("whatever", nil),
		save:     func(agent.Session) error { return nil },
		out:      &bytes.Buffer{},
	}
	res := runMigrateToGss(deps, false)
	if len(res) != 1 || !strings.Contains(res[0].Status, "already migrated") {
		t.Fatalf("want already-migrated skip; got %+v", res)
	}
	if len(calls) != 0 {
		t.Errorf("gss must not be called for an already-migrated session; got %v", calls)
	}
}

func TestRunMigrate_DryRunCallsNoMutators(t *testing.T) {
	var calls [][]string
	var saved []agent.Session
	var out bytes.Buffer
	deps := migrateDeps{
		sessions: []agent.Session{{SessionID: "s1", AgentName: "generalist", WorktreePath: "/wt/old"}},
		runGss:   recordingGss(&calls, sampleAddJSON, nil),
		git:      fakeGit("generalist-1700000000000", nil),
		save:     func(s agent.Session) error { saved = append(saved, s); return nil },
		out:      &out,
	}
	res := runMigrateToGss(deps, true)
	if len(calls) != 0 {
		t.Errorf("dry-run must not call gss; got %v", calls)
	}
	if len(saved) != 0 {
		t.Errorf("dry-run must not save sessions; got %v", saved)
	}
	if !strings.Contains(res[0].Status, "dry-run") {
		t.Errorf("want dry-run status; got %q", res[0].Status)
	}
	if !strings.Contains(out.String(), "migrated-generalist") || !strings.Contains(out.String(), "generalist-1700000000000") {
		t.Errorf("dry-run plan should name the feature + base branch\n  got: %s", out.String())
	}
}

func TestRunMigrate_MigratesAndUpdatesSession(t *testing.T) {
	var calls [][]string
	var saved []agent.Session
	deps := migrateDeps{
		sessions: []agent.Session{{SessionID: "s1", AgentName: "generalist", WorktreePath: "/wt/old", Status: agent.StatusCompleted}},
		runGss:   recordingGss(&calls, sampleAddJSON, nil),
		git:      fakeGit("generalist-1700000000000", nil),
		save:     func(s agent.Session) error { saved = append(saved, s); return nil },
		user:     "erai",
		out:      &bytes.Buffer{},
	}
	res := runMigrateToGss(deps, false)
	if res[0].Status != "migrated" {
		t.Fatalf("want migrated; got %q", res[0].Status)
	}
	// gss called: feature start, then worker add --base <legacy branch>.
	if len(calls) != 2 {
		t.Fatalf("want 2 gss calls (start + worker add); got %d: %v", len(calls), calls)
	}
	if calls[0][0] != "feature" || calls[0][1] != "start" {
		t.Errorf("first gss call should be `feature start`; got %v", calls[0])
	}
	add := strings.Join(calls[1], " ")
	if !strings.Contains(add, "worker add") || !strings.Contains(add, "--base generalist-1700000000000") {
		t.Errorf("worker add should adopt the legacy branch as --base; got %v", calls[1])
	}
	// session JSON updated in place: WorkerRef + new WorktreePath.
	if len(saved) != 1 {
		t.Fatalf("expected one saved session; got %d", len(saved))
	}
	if saved[0].WorkerRef != "migrated-generalist/erai/main" {
		t.Errorf("saved WorkerRef = %q; want migrated-generalist/erai/main", saved[0].WorkerRef)
	}
	if saved[0].WorktreePath != "/wt/owner/repo/migrated-generalist/erai/main" {
		t.Errorf("saved WorktreePath = %q; want the new gss path", saved[0].WorktreePath)
	}
}

func TestRunMigrate_DetachedHeadFallsBackToSHA(t *testing.T) {
	var calls [][]string
	deps := migrateDeps{
		sessions: []agent.Session{{SessionID: "s1", AgentName: "gen", WorktreePath: "/wt/old"}},
		runGss:   recordingGss(&calls, sampleAddJSON, nil),
		git:      fakeGit("HEAD", nil), // detached
		save:     func(agent.Session) error { return nil },
		out:      &bytes.Buffer{},
	}
	runMigrateToGss(deps, false)
	add := strings.Join(calls[1], " ")
	if !strings.Contains(add, "--base deadbeefcafef00d") {
		t.Errorf("detached HEAD should use the commit SHA as --base; got %v", calls[1])
	}
}

func TestRunMigrate_PartialFailureContinues(t *testing.T) {
	var calls [][]string
	gitErr := fakeGit("", errBoom)
	gitOK := fakeGit("gen-9", nil)
	// per-session git: first fails, second ok.
	gitForSession := func(dir string, args ...string) (string, error) {
		if strings.Contains(dir, "bad") {
			return gitErr(dir, args...)
		}
		return gitOK(dir, args...)
	}
	deps := migrateDeps{
		sessions: []agent.Session{
			{SessionID: "bad1", AgentName: "gen", WorktreePath: "/wt/bad"},
			{SessionID: "ok1", AgentName: "gen", WorktreePath: "/wt/good"},
		},
		runGss: recordingGss(&calls, sampleAddJSON, nil),
		git:    gitForSession,
		save:   func(agent.Session) error { return nil },
		out:    &bytes.Buffer{},
	}
	res := runMigrateToGss(deps, false)
	if len(res) != 2 {
		t.Fatalf("both sessions must be processed; got %d", len(res))
	}
	if !strings.Contains(res[0].Status, "FAILED") {
		t.Errorf("first session should FAIL; got %q", res[0].Status)
	}
	if res[1].Status != "migrated" {
		t.Errorf("second session should still migrate; got %q", res[1].Status)
	}
}

func TestRunMigrate_NoWorktreePathSkips(t *testing.T) {
	var calls [][]string
	deps := migrateDeps{
		sessions: []agent.Session{{SessionID: "s1", AgentName: "gen", WorktreePath: ""}},
		runGss:   recordingGss(&calls, sampleAddJSON, nil),
		git:      fakeGit("gen-1", nil),
		save:     func(agent.Session) error { return nil },
		out:      &bytes.Buffer{},
	}
	res := runMigrateToGss(deps, false)
	if !strings.Contains(res[0].Status, "no worktree path") {
		t.Errorf("want no-worktree skip; got %q", res[0].Status)
	}
	if len(calls) != 0 {
		t.Errorf("gss must not be called when there is no worktree; got %v", calls)
	}
}

func TestRunMigrate_GssAddErrorFails(t *testing.T) {
	var calls [][]string
	failAdd := func(args []string) error {
		if slices.Contains(args, "add") {
			return errBoom
		}
		return nil
	}
	deps := migrateDeps{
		sessions: []agent.Session{{SessionID: "s1", AgentName: "gen", WorktreePath: "/wt"}},
		runGss:   recordingGss(&calls, sampleAddJSON, failAdd),
		git:      fakeGit("gen-1", nil),
		save:     func(agent.Session) error { return nil },
		out:      &bytes.Buffer{},
	}
	res := runMigrateToGss(deps, false)
	if !strings.Contains(res[0].Status, "FAILED (gss worker add") {
		t.Errorf("want gss-add failure; got %q", res[0].Status)
	}
}

func TestRunMigrate_BadJSONFails(t *testing.T) {
	var calls [][]string
	deps := migrateDeps{
		sessions: []agent.Session{{SessionID: "s1", AgentName: "gen", WorktreePath: "/wt"}},
		runGss:   recordingGss(&calls, "this is not json", nil),
		git:      fakeGit("gen-1", nil),
		save:     func(agent.Session) error { return nil },
		out:      &bytes.Buffer{},
	}
	res := runMigrateToGss(deps, false)
	if !strings.Contains(res[0].Status, "FAILED (parse worker add JSON") {
		t.Errorf("want parse failure; got %q", res[0].Status)
	}
}

func TestRunMigrate_SaveErrorFails(t *testing.T) {
	var calls [][]string
	deps := migrateDeps{
		sessions: []agent.Session{{SessionID: "s1", AgentName: "gen", WorktreePath: "/wt"}},
		runGss:   recordingGss(&calls, sampleAddJSON, nil),
		git:      fakeGit("gen-1", nil),
		save:     func(agent.Session) error { return errBoom },
		out:      &bytes.Buffer{},
	}
	res := runMigrateToGss(deps, false)
	if !strings.Contains(res[0].Status, "FAILED (save session") {
		t.Errorf("want save failure; got %q", res[0].Status)
	}
}

func TestFormatSummary(t *testing.T) {
	s := formatSummary([]migrateResult{
		{SessionID: "s1", WorkerRef: "feat/erai/main", Status: "migrated"},
		{SessionID: "s2", WorkerRef: "", Status: "FAILED (x)"},
	})
	if !strings.Contains(s, "s1  →  feat/erai/main  (migrated)") {
		t.Errorf("summary row malformed:\n%s", s)
	}
	if !strings.Contains(s, "s2  →  -  (FAILED (x))") {
		t.Errorf("empty worker ref should render as '-':\n%s", s)
	}
}

func TestMigrateCmd_Wired(t *testing.T) {
	if !hasSub(internalCmd, "migrate-to-gss") {
		t.Error("internal migrate-to-gss not wired")
	}
}

var errBoom = &boomError{}

type boomError struct{}

func (*boomError) Error() string { return "boom" }
