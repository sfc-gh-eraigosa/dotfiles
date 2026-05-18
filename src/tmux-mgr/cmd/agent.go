package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	taskID, _ := cmd.Flags().GetString("task-id")

	if taskID == "" {
		return fmt.Errorf("--task-id flag is required")
	}

	fmt.Printf("Starting isolated agent '%s' for task '%s'...\n", agentName, taskID)

	workspacePath, sessionID, err := workspace.CreateWorkspace(agentName)
	if err != nil {
		return fmt.Errorf("error creating workspace: %w", err)
	}

	// Construct the native Gemini invocation command using the correct CLI syntax.
	// We use -r to associate with the task-id (session) and -p for non-interactive mode.
	// We use --yolo to allow the sub-agent to work autonomously in its isolated environment.
	// We explicitly tell the sub-agent to write its summary to RESULT.md for fan-in.
	instruction := fmt.Sprintf("@%s Execute task %s. When finished, you MUST write your final summary to RESULT.md in the current directory and then exit.", agentName, taskID)
	invocationCmd := fmt.Sprintf("gemini -r %s -y -p '%s'", taskID, instruction)

	// Create the tmux pane and run the command
	if err := tmux.CreatePane(sessionID, workspacePath, invocationCmd); err != nil {
		return fmt.Errorf("error creating tmux pane: %w", err)
	}

	// Save the session details for lifecycle management (cleanup)
	session := agent.Session{
		SessionID:    sessionID,
		AgentName:    agentName,
		Status:       "RUNNING",
		StartTime:    time.Now(),
		WorktreePath: workspacePath,
	}

	if err := agent.SaveSession(session); err != nil {
		fmt.Printf("Warning: Failed to save session state: %s\n", err)
	}

	fmt.Printf("Agent '%s' with session ID '%s' started in a new tmux pane.\n", agentName, sessionID)
	return nil
}

func runAgentList(cmd *cobra.Command, args []string) {
	sessions, err := agent.ListSessions()
	if err != nil {
		fmt.Printf("Error listing sessions: %s\n", err)
		return
	}

	if len(sessions) == 0 {
		fmt.Println("No active agent sessions found.")
		return
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

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists all active agent sessions.",
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
	agentCmd.AddCommand(listCmd)
	agentCmd.AddCommand(completeCmd)
	agentCmd.AddCommand(cleanupCmd)

	startCmd.Flags().StringP("task-id", "t", "", "The native Gemini tracker task ID for the agent.")
	startCmd.MarkFlagRequired("task-id")
}
