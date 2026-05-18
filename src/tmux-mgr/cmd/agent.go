package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/eraigosa/dotfiles/src/tmux-mgr/pkg/agent"
	"github.com/eraigosa/dotfiles/src/tmux-mgr/pkg/tmux"
	"github.com/eraigosa/dotfiles/src/tmux-mgr/pkg/workspace"
	"github.com/spf13/cobra"
)

func runAgent(cmd *cobra.Command, args []string) {
	cmd.Help()
}

func runAgentStart(cmd *cobra.Command, args []string) error {
	agentName := args[0]
	taskDescription, _ := cmd.Flags().GetString("task-description")

	if taskDescription == "" {
		return fmt.Errorf("--task-description flag is required")
	}

	host := agent.DetectHost(os.Getenv)
	fmt.Printf("Starting isolated agent '%s' (assistant=%s) for task '%s'...\n", agentName, host, taskDescription)

	workspacePath, sessionID, err := workspace.CreateWorkspace(agentName)
	if err != nil {
		return fmt.Errorf("error creating workspace: %w", err)
	}

	executablePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	assistantBinary := string(host)
	assistantPath, err := exec.LookPath(assistantBinary)
	if err != nil {
		assistantPath = assistantBinary // fallback; child will surface the error
	}

	invocationCmd := buildInvocationCmd(host, assistantPath, executablePath, taskDescription)

	paneID, err := tmux.CreatePane(sessionID, workspacePath, invocationCmd)
	if err != nil {
		return fmt.Errorf("error creating tmux pane: %w", err)
	}

	session := agent.Session{
		SessionID:    sessionID,
		AgentName:    agentName,
		Status:       agent.StatusRunning,
		StartTime:    time.Now(),
		WorktreePath: workspacePath,
		PaneID:       paneID,
		RepoRoot:     currentRepoRoot(),
	}

	if err := agent.SaveSession(session); err != nil {
		fmt.Printf("Warning: Failed to save session state: %s\n", err)
	}

	fmt.Printf("Agent '%s' with session ID '%s' started in a new tmux pane.\n", agentName, sessionID)
	return nil
}

func runAgentExecute(cmd *cobra.Command, args []string) error {
	task, _ := cmd.Flags().GetString("task-description")
	if task == "" {
		return fmt.Errorf("task description is empty")
	}

	host := agent.Assistant(os.Getenv("TMUX_MGR_ASSISTANT"))
	if host == "" {
		host = agent.AssistantGemini
	}
	assistantPath := os.Getenv("TMUX_MGR_ASSISTANT_PATH")
	// Back-compat: respect legacy GEMINI_PATH if the new var is absent and host is gemini.
	if assistantPath == "" && host == agent.AssistantGemini {
		assistantPath = os.Getenv("GEMINI_PATH")
	}

	fmt.Printf("Agent executing task (assistant=%s): %s\n", host, task)

	switch host {
	case agent.AssistantClaude:
		return runClaudeLoop(task, assistantPath)
	default:
		return runGeminiLoop(task, assistantPath)
	}
}

// buildInvocationCmd produces the shell string used to launch `tmux-mgr agent execute`
// inside a freshly-created tmux pane. Exposed at package scope so it can be unit-tested
// without spawning tmux.
func buildInvocationCmd(host agent.Assistant, assistantPath, executablePath, taskDescription string) string {
	// Escape single quotes in the task description to prevent shell injection issues.
	escapedTask := strings.ReplaceAll(taskDescription, "'", "'\\''")
	return fmt.Sprintf(
		"TMUX_MGR_ASSISTANT='%s' TMUX_MGR_ASSISTANT_PATH='%s' %s agent execute --task-description '%s'",
		host, assistantPath, executablePath, escapedTask,
	)
}

// buildInstruction produces the prompt handed to the spawned assistant. The shape differs
// per host: Gemini uses its @generalist extension prefix; Claude takes a plain task plus
// the RESULT.md mandate.
func buildInstruction(host agent.Assistant, task string) string {
	if host == agent.AssistantClaude {
		return fmt.Sprintf(
			"Execute the following task in the current working directory: %s. When finished, you MUST write the final result to RESULT.md in the current directory using the Write tool, then exit.",
			task,
		)
	}
	return fmt.Sprintf(
		"@generalist Execute the following task: %s. When finished, you MUST ensure the final result is written to RESULT.md in the current directory, then exit.",
		task,
	)
}

func runGeminiLoop(task, geminiPath string) error {
	if geminiPath == "" {
		geminiPath = "gemini"
	}

	instruction := buildInstruction(agent.AssistantGemini, task)

	// List of models to try. Empty string means the default model configured in the CLI.
	// Ordered by preference and fallback availability.
	models := []string{
		"", // Default model
		"gemini-3.1-pro-preview",
		"gemini-2.5-pro",
		"gemini-2.5-flash",
	}

	for _, model := range models {
		if model == "" {
			fmt.Println("Starting cognitive loop with default model...")
		} else {
			fmt.Printf("Starting cognitive loop with fallback model: %s...\n", model)
		}

		execArgs := []string{"-y", "-p", instruction}
		if model != "" {
			execArgs = append([]string{"-m", model}, execArgs...)
		}

		c := exec.Command(geminiPath, execArgs...)

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
			logFile.Close()
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

func runClaudeLoop(task, claudePath string) error {
	if claudePath == "" {
		claudePath = "claude"
	}

	instruction := buildInstruction(agent.AssistantClaude, task)

	fmt.Println("Starting cognitive loop with Claude...")

	c := exec.Command(claudePath, "-p", instruction, "--dangerously-skip-permissions")

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
		logFile.Close()
	}

	if runErr != nil {
		return fmt.Errorf("agent execution failed: %w", runErr)
	}

	fmt.Println("Task complete. Exiting.")
	return nil
}

// currentRepoRoot returns the absolute git toplevel for the current working
// directory, or "" if the cwd is not inside a git repository (or git is
// unavailable). It is used both to tag new sessions and to default the
// `agent list` filter to the current repo.
func currentRepoRoot() string {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func runAgentList(cmd *cobra.Command, args []string) {
	all, _ := cmd.Flags().GetBool("all")

	filter := ""
	if !all {
		filter = currentRepoRoot()
	}

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

	fmt.Printf("Cleaning up session '%s'...\n", sessionID)

	// Remove the git worktree
	fmt.Printf("Removing worktree at: %s\n", session.WorktreePath)
	gitRootCmd := exec.Command("git", "rev-parse", "--show-toplevel")
	gitRootBytes, err := gitRootCmd.Output()
	if err == nil {
		gitRoot := string(gitRootBytes)
		if len(gitRoot) > 0 {
			gitRoot = gitRoot[:len(gitRoot)-1] // Remove newline
		}

		wtCmd := exec.Command("git", "worktree", "remove", "--force", session.WorktreePath)
		wtCmd.Dir = gitRoot
		if wtOut, err := wtCmd.CombinedOutput(); err != nil {
			fmt.Printf("Warning: Failed to remove git worktree: %s\nOutput: %s\n", err, string(wtOut))
		}
	}

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
	startCmd.MarkFlagRequired("task-description")

	executeCmd.Flags().StringP("task-description", "d", "", "The task description for the agent.")
	executeCmd.MarkFlagRequired("task-description")

	listCmd.Flags().BoolP("all", "a", false, "Show sessions from every repo (and global sessions). Default scopes to current repo.")
}
