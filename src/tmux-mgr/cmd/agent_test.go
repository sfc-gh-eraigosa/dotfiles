package cmd

import (
	"bytes"
	"testing"
)

func executeCommand(args ...string) (string, error) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(args)

	err := rootCmd.Execute()
	return buf.String(), err
}

func TestAgentCmd_Help(t *testing.T) {
	output, err := executeCommand("agent", "--help")
	if err != nil {
		t.Errorf("agent help failed: %v", err)
	}
	if len(output) == 0 {
		t.Error("Expected output from agent help")
	}
}

func TestAgentStartCmd_NoTaskID(t *testing.T) {
	// Should fail because --task-id is required
	_, err := executeCommand("agent", "start", "generalist")
	if err == nil {
		t.Error("Expected error when starting agent without task-id")
	}
}

func TestAgentListCmd(t *testing.T) {
    // Should run successfully even if no sessions exist
    _, err := executeCommand("agent", "list")
    if err != nil {
        t.Errorf("agent list failed: %v", err)
    }
}

func TestAgentCleanupCmd_NoArgs(t *testing.T) {
    // Should fail because session-id is required
    _, err := executeCommand("agent", "cleanup")
    if err == nil {
        t.Error("Expected error when calling cleanup without args")
    }
}
