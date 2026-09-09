package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/tmux-mgr/pkg/agent"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/tmux-mgr/pkg/tmux"
	"github.com/spf13/cobra"
)

func runAgent(cmd *cobra.Command, args []string) {
	// Bare `tmux-mgr agent` just prints usage; a failed write to the help
	// stream has nowhere left to be reported.
	_ = cmd.Help()
}

func runAgentStart(cmd *cobra.Command, args []string) error {
	agentName := args[0]
	taskDescription, _ := cmd.Flags().GetString("task-description")

	if taskDescription == "" {
		return fmt.Errorf("--task-description flag is required")
	}

	host := agent.DetectHost(os.Getenv)

	def, err := agent.LoadDefinition(agentName)
	if err != nil {
		fmt.Printf("Warning: failed to load agent definition for %q: %s (continuing with generalist defaults)\n", agentName, err)
		def = nil
	}
	model := resolveModel(def, host)

	defSummary := "generalist defaults"
	if def != nil {
		defSummary = fmt.Sprintf("definition=%s persona=%q symbol=%q", def.SourcePath, def.Persona, def.Symbol)
	}
	modelSummary := model
	if modelSummary == "" {
		modelSummary = "(inherit host default)"
	}
	fmt.Printf("Starting isolated agent '%s' (assistant=%s model=%s) for task '%s'\n  %s\n", agentName, host, modelSummary, taskDescription, defSummary)

	// gss owns the worktree + repo identity now. Resolve the feature/purpose
	// (defaulting both to the agent name), then create the worker via gss.
	feature, _ := cmd.Flags().GetString("feature")
	if feature == "" {
		feature = agentName
	}
	purpose, _ := cmd.Flags().GetString("purpose")
	if purpose == "" {
		purpose = agentName
	}
	sessionID := fmt.Sprintf("%s-%d", agentName, time.Now().UnixNano())

	wa, err := gssWorkerAdd(defaultGssRunner, feature, purpose, taskDescription,
		os.Getenv("USER"), string(host), engineSessionID(host, os.Getenv), sessionID)
	if err != nil {
		return fmt.Errorf("error creating gss worker: %w", err)
	}
	workspacePath := wa.WorktreePath
	fmt.Printf("Created gss worker %s at %s (branch %s, base %s)\n", wa.WorkerRef, workspacePath, wa.Branch, wa.BaseBranch)

	return spawnAgentPane(host, def, model, agentName, sessionID, wa, taskDescription)
}

// resolveModel selects the model for host, deferring to a configured launcher
// (TMUX_MGR_*_LAUNCHER) by clearing the model — those wrappers may use a
// different model namespace than the bare CLI, so passing tmux-mgr's tier
// mapping would fail.
func resolveModel(def *agent.Definition, host agent.Assistant) string {
	model := agent.SelectModel(def, host)
	if host == agent.AssistantClaude && os.Getenv("TMUX_MGR_CLAUDE_LAUNCHER") != "" {
		return ""
	}
	if agent.NormalizeAssistant(host) == agent.AssistantAntigravity && os.Getenv("TMUX_MGR_ANTIGRAVITY_LAUNCHER") != "" {
		return ""
	}
	return model
}

// spawnAgentPane launches the agent for an already-created gss worker: build
// the pane-wrapped invocation (PR-54), create the tmux pane, and persist the
// session. Shared by `agent start` and `feature add-agent`.
func spawnAgentPane(host agent.Assistant, def *agent.Definition, model, agentName, sessionID string, wa workerAddResult, taskDescription string) error {
	executablePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	assistantBinary := host.Binary()
	assistantPath, err := exec.LookPath(assistantBinary)
	if err != nil {
		assistantPath = assistantBinary // fallback; child will surface the error
	}
	inner := buildInvocationCmd(host, assistantPath, executablePath, taskDescription, model)
	invocationCmd := wrapWithPaneWrap(executablePath, wa.WorkerRef, inner)
	symbol, label := "", ""
	if def != nil {
		symbol = def.Symbol
		label = def.Persona
	}
	paneID, err := tmux.CreatePane(sessionID, wa.WorktreePath, invocationCmd, symbol, label)
	if err != nil {
		return fmt.Errorf("error creating tmux pane: %w", err)
	}
	session := agent.Session{
		SessionID:    sessionID,
		AgentName:    agentName,
		Status:       agent.StatusRunning,
		StartTime:    time.Now(),
		WorktreePath: wa.WorktreePath,
		PaneID:       paneID,
		WorkerRef:    wa.WorkerRef,
	}
	if err := agent.SaveSession(session); err != nil {
		fmt.Printf("Warning: Failed to save session state: %s\n", err)
	}
	fmt.Printf("Agent '%s' (worker %s, session %s) started in a new tmux pane.\n", agentName, wa.WorkerRef, sessionID)
	return nil
}

func runAgentExecute(cmd *cobra.Command, args []string) error {
	task, _ := cmd.Flags().GetString("task-description")
	if task == "" {
		return fmt.Errorf("task description is empty")
	}

	// Normalize so a legacy TMUX_MGR_ASSISTANT='gemini' (old panes/wrappers)
	// runs the Antigravity loop.
	host := agent.NormalizeAssistant(agent.Assistant(os.Getenv("TMUX_MGR_ASSISTANT")))
	if host == "" {
		host = agent.AssistantAntigravity
	}
	assistantPath := os.Getenv("TMUX_MGR_ASSISTANT_PATH")

	model := os.Getenv("TMUX_MGR_MODEL")

	fmt.Printf("Agent executing task (assistant=%s model=%s): %s\n", host, displayModel(model), task)

	switch host {
	case agent.AssistantClaude:
		return runClaudeLoop(task, assistantPath, model)
	default:
		return runAntigravityLoop(task, assistantPath, model)
	}
}

func displayModel(model string) string {
	if model == "" {
		return "(inherit host default)"
	}
	return model
}

// resolveLauncher splits a launcher env var into an executable + prefix args
// for use with exec.Command. This lets users wrap an assistant CLI in a
// platform-specific runner without forking the binary or changing flag order.
//
// Example: TMUX_MGR_CLAUDE_LAUNCHER="sf ai claude --" produces
//
//	execPath="sf", prefixArgs=["ai","claude","--"]
//
// so the spawned command becomes:
//
//	sf ai claude -- -p "<task>" --dangerously-skip-permissions
//
// Resolution order: env var (multi-token) > explicit fallbackPath (single
// token, from TMUX_MGR_ASSISTANT_PATH) > defaultExec (e.g. "claude").
func resolveLauncher(envName, defaultExec, fallbackPath string) (string, []string) {
	if l := strings.TrimSpace(os.Getenv(envName)); l != "" {
		tokens := strings.Fields(l)
		if len(tokens) > 0 {
			return tokens[0], tokens[1:]
		}
	}
	if fallbackPath != "" {
		return fallbackPath, nil
	}
	return defaultExec, nil
}

// buildInvocationCmd produces the shell string used to launch `tmux-mgr agent execute`
// inside a freshly-created tmux pane. Exposed at package scope so it can be unit-tested
// without spawning tmux.
//
// model is the resolved Claude/Antigravity model ID (or "" to let the spawned CLI
// inherit its default). It is forwarded via TMUX_MGR_MODEL so the child
// `agent execute` process can attach the right CLI flag.
func buildInvocationCmd(host agent.Assistant, assistantPath, executablePath, taskDescription, model string) string {
	escapedTask := strings.ReplaceAll(taskDescription, "'", "'\\''")
	modelEnv := ""
	if model != "" {
		modelEnv = fmt.Sprintf("TMUX_MGR_MODEL='%s' ", model)
	}
	// Forward launcher env vars from the parent process. tmux runs the pane
	// command via /bin/sh -c, which does NOT source the user's shell rc, so
	// vars set in ~/.zshrc.local would otherwise be lost in the spawned pane.
	extraEnv := ""
	for _, name := range []string{"TMUX_MGR_CLAUDE_LAUNCHER", "TMUX_MGR_ANTIGRAVITY_LAUNCHER"} {
		if v := os.Getenv(name); v != "" {
			escaped := strings.ReplaceAll(v, "'", "'\\''")
			extraEnv += fmt.Sprintf("%s='%s' ", name, escaped)
		}
	}
	return fmt.Sprintf(
		"%s%sTMUX_MGR_ASSISTANT='%s' TMUX_MGR_ASSISTANT_PATH='%s' %s agent execute --task-description '%s'",
		extraEnv, modelEnv, host, assistantPath, executablePath, escapedTask,
	)
}

// buildInstruction produces the prompt handed to the spawned assistant. Both
// hosts take a plain task plus the RESULT.md mandate; Claude's wording names
// its Write tool explicitly. (The Gemini-era @generalist extension prefix
// retired with Gemini CLI — Antigravity has no such extension syntax.)
func buildInstruction(host agent.Assistant, task string) string {
	if host == agent.AssistantClaude {
		return fmt.Sprintf(
			"Execute the following task in the current working directory: %s. When finished, you MUST write the final result to RESULT.md in the current directory using the Write tool, then exit.",
			task,
		)
	}
	return fmt.Sprintf(
		"Execute the following task: %s. When finished, you MUST ensure the final result is written to RESULT.md in the current directory, then exit.",
		task,
	)
}

func runAntigravityLoop(task, agyPath, requestedModel string) error {
	// Launcher resolution mirrors runClaudeLoop — see TMUX_MGR_ANTIGRAVITY_LAUNCHER.
	execPath, prefixArgs := resolveLauncher("TMUX_MGR_ANTIGRAVITY_LAUNCHER", "agy", agyPath)

	instruction := buildInstruction(agent.AssistantAntigravity, task)

	// When the caller requested a specific model (via TMUX_MGR_MODEL), try it
	// first. The rest of the list provides quota fallback. The empty string
	// means "use the CLI's default model".
	models := []string{
		"", // Default model
		"gemini-3.1-pro-preview",
		"gemini-2.5-pro",
		"gemini-2.5-flash",
	}
	if requestedModel != "" {
		models = append([]string{requestedModel}, models...)
	}

	for _, model := range models {
		if model == "" {
			fmt.Println("Starting cognitive loop with default model...")
		} else {
			fmt.Printf("Starting cognitive loop with fallback model: %s...\n", model)
		}

		execArgs := []string{"-p", instruction, "--dangerously-skip-permissions"}
		if model != "" {
			execArgs = append([]string{"--model", model}, execArgs...)
		}
		execArgs = append(append([]string{}, prefixArgs...), execArgs...)

		c := exec.Command(execPath, execArgs...)

		logFile, err := os.OpenFile("agent.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		var outputBuf strings.Builder

		if err == nil {
			c.Stdout = io.MultiWriter(os.Stdout, logFile, &outputBuf)
			c.Stderr = io.MultiWriter(os.Stderr, logFile, &outputBuf)
		} else {
			c.Stdout = io.MultiWriter(os.Stdout, &outputBuf)
			c.Stderr = io.MultiWriter(os.Stderr, &outputBuf)
		}
		c.Stdin = os.Stdin

		runErr := c.Run()
		if logFile != nil {
			// A failed Close truncates the agent transcript, which is the whole
			// point of the log — say so rather than dropping it.
			if err := logFile.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "warning: closing agent log: %v\n", err)
			}
		}

		if runErr == nil {
			fmt.Println("Task complete. Exiting.")
			return nil
		}

		output := outputBuf.String()
		if strings.Contains(output, "exhausted your capacity") || strings.Contains(output, "QUOTA_EXHAUSTED") || strings.Contains(output, "429") {
			fmt.Printf("Model quota exhausted. Retrying with next available model...\n")
			continue
		}

		return fmt.Errorf("agent execution failed: %w", runErr)
	}

	return fmt.Errorf("all models exhausted or failed")
}

func runClaudeLoop(task, claudePath, model string) error {
	// Launcher resolution: prefer TMUX_MGR_CLAUDE_LAUNCHER (multi-token, e.g.
	// "sf ai claude --") for hosts that wrap claude in a platform runner;
	// otherwise use the explicit claudePath; otherwise default to "claude".
	execPath, prefixArgs := resolveLauncher("TMUX_MGR_CLAUDE_LAUNCHER", "claude", claudePath)

	instruction := buildInstruction(agent.AssistantClaude, task)

	if model == "" {
		fmt.Println("Starting cognitive loop with Claude (default model)...")
	} else {
		fmt.Printf("Starting cognitive loop with Claude (model=%s)...\n", model)
	}

	claudeArgs := append([]string{}, prefixArgs...)
	if model != "" {
		claudeArgs = append(claudeArgs, "--model", model)
	}
	claudeArgs = append(claudeArgs, "-p", instruction, "--dangerously-skip-permissions")
	c := exec.Command(execPath, claudeArgs...)

	logFile, err := os.OpenFile("agent.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		c.Stdout = io.MultiWriter(os.Stdout, logFile)
		c.Stderr = io.MultiWriter(os.Stderr, logFile)
	} else {
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
	}
	c.Stdin = os.Stdin

	runErr := c.Run()
	if logFile != nil {
		if err := logFile.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: closing agent log: %v\n", err)
		}
	}

	if runErr != nil {
		return fmt.Errorf("agent execution failed: %w", runErr)
	}

	fmt.Println("Task complete. Exiting.")
	return nil
}

func runAgentList(cmd *cobra.Command, args []string) {
	all, _ := cmd.Flags().GetBool("all")

	// ListSessionsFiltered now scopes by gss feature (re-derived from each
	// session's WorkerRef) rather than the dropped RepoRoot. Until runAgentStart
	// records a WorkerRef (later Batch J PR), there is nothing to scope by, so
	// list all sessions; --all is retained for the forthcoming feature scope.
	filter := ""
	_ = all

	sessions, err := agent.ListSessionsFiltered(agent.DefaultPaneChecker, filter)
	if err != nil {
		fmt.Printf("Error listing sessions: %s\n", err)
		return
	}

	if len(sessions) == 0 {
		if filter != "" {
			fmt.Printf("No active agent sessions found for repo %s. Use --all to see every session.\n", filter)
		} else {
			fmt.Println("No active agent sessions found.")
		}
		return
	}

	if filter != "" {
		fmt.Printf("Sessions scoped to repo %s (use --all to see every session):\n", filter)
	}
	fmt.Printf("%-30s %-15s %-10s %s\n", "SESSION ID", "AGENT NAME", "STATUS", "START TIME")
	fmt.Println("--------------------------------------------------------------------------------")
	for _, s := range sessions {
		fmt.Printf("%-30s %-15s %-10s %s\n", s.SessionID, s.AgentName, s.Status, s.StartTime.Format(time.RFC822))
	}
}

func runAgentComplete(cmd *cobra.Command, args []string) {
	sessionID := args[0]
	session, err := agent.LoadSession(sessionID)
	if err != nil {
		fmt.Printf("Error loading session '%s': %s\n", sessionID, err)
		return
	}

	resultPath := filepath.Join(session.WorktreePath, "RESULT.md")
	content, err := os.ReadFile(resultPath)
	if err != nil {
		fmt.Printf("Error reading result file (it might not be ready yet): %s\n", err)
		return
	}

	fmt.Printf("--- Result for Session %s ---\n", sessionID)
	fmt.Println(string(content))
}

func runAgentCleanup(cmd *cobra.Command, args []string) {
	sessionID := args[0]
	session, err := agent.LoadSession(sessionID)
	if err != nil {
		fmt.Printf("Error loading session '%s': %s\n", sessionID, err)
		return
	}

	force, _ := cmd.Flags().GetBool("force")
	fmt.Printf("Cleaning up session '%s'...\n", sessionID)

	// gss owns worktree teardown via `gss feature done --worker <ref>`. As of
	// PR-59 tmux-mgr no longer removes worktrees directly; a legacy session
	// (no WorkerRef) is left in place with a pointer to `migrate-to-gss`.
	cleanupSession(session, force, cleanupDeps{
		run:      defaultGssRunner,
		killPane: func(paneID string) error { return exec.Command("tmux", "kill-pane", "-t", paneID).Run() },
		out:      os.Stdout,
	})

	// Delete the session file
	if err := agent.DeleteSession(sessionID); err != nil {
		fmt.Printf("Warning: Failed to delete session file: %s\n", err)
	} else {
		fmt.Println("Session state cleared.")
	}

	fmt.Println("Cleanup complete.")
}

// agentCmd represents the agent command
var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage and coordinate AI agents within tmux.",
	Long: `The agent command is the entry point for all agent-related operations.
It allows you to start, list, and manage the lifecycle of AI agents,
each running in its own isolated tmux pane and git worktree.`,
	Run: runAgent,
}

var startCmd = &cobra.Command{
	Use:   "start [agent-name]",
	Short: "Starts a new agent session in a tmux pane.",
	Long:  `Starts a new agent session in a dedicated, isolated tmux pane and git worktree.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentStart,
}

var executeCmd = &cobra.Command{
	Use:   "execute",
	Short: "Internal command for an agent to execute its task.",
	Long:  "This command is not intended for direct user interaction. It runs the agent's cognitive loop.",
	Args:  cobra.NoArgs,
	RunE:  runAgentExecute,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists active agent sessions for the current repo (use --all for every session).",
	Run:   runAgentList,
}

var completeCmd = &cobra.Command{
	Use:   "complete [session-id]",
	Short: "Retrieves the final summary from RESULT.md in the agent's worktree.",
	Args:  cobra.ExactArgs(1),
	Run:   runAgentComplete,
}

var cleanupCmd = &cobra.Command{
	Use:   "cleanup [session-id]",
	Short: "Cleans up an agent session (removes tmux pane, worktree, and session tracking).",
	Args:  cobra.ExactArgs(1),
	Run:   runAgentCleanup,
}

func init() {
	rootCmd.AddCommand(agentCmd)
	agentCmd.AddCommand(startCmd)
	agentCmd.AddCommand(executeCmd)
	agentCmd.AddCommand(listCmd)
	agentCmd.AddCommand(completeCmd)
	agentCmd.AddCommand(cleanupCmd)

	startCmd.Flags().StringP("task-description", "d", "", "A natural language description of the agent's task.")
	// Cannot fail: the flag is registered on the line above.
	_ = startCmd.MarkFlagRequired("task-description")
	startCmd.Flags().String("feature", "", "gss feature to add the worker to (default: the agent name; auto-created)")
	startCmd.Flags().String("purpose", "", "gss worker purpose (default: the agent name)")

	cleanupCmd.Flags().Bool("force", false, "Forward --force to `gss feature done` (remove despite dirty/dependents/open PR)")

	executeCmd.Flags().StringP("task-description", "d", "", "The task description for the agent.")
	// Cannot fail: the flag is registered on the line above.
	_ = executeCmd.MarkFlagRequired("task-description")

	listCmd.Flags().BoolP("all", "a", false, "Show sessions from every repo (and global sessions). Default scopes to current repo.")
}
