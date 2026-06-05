package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/tmux-mgr/pkg/agent"
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

func TestBuildInvocationCmd_Claude(t *testing.T) {
	got := buildInvocationCmd(agent.AssistantClaude, "/usr/local/bin/claude", "/usr/local/bin/tmux-mgr", "Refactor auth", "")

	wantParts := []string{
		"TMUX_MGR_ASSISTANT='claude'",
		"TMUX_MGR_ASSISTANT_PATH='/usr/local/bin/claude'",
		"/usr/local/bin/tmux-mgr agent execute",
		"--task-description 'Refactor auth'",
	}
	for _, want := range wantParts {
		if !strings.Contains(got, want) {
			t.Errorf("buildInvocationCmd missing %q\n  got: %s", want, got)
		}
	}
	if strings.Contains(got, "TMUX_MGR_MODEL=") {
		t.Errorf("empty model should not emit TMUX_MGR_MODEL\n  got: %s", got)
	}
}

func TestBuildInvocationCmd_Gemini(t *testing.T) {
	got := buildInvocationCmd(agent.AssistantGemini, "/usr/local/bin/gemini", "/usr/local/bin/tmux-mgr", "say hi", "")

	wantParts := []string{
		"TMUX_MGR_ASSISTANT='gemini'",
		"TMUX_MGR_ASSISTANT_PATH='/usr/local/bin/gemini'",
	}
	for _, want := range wantParts {
		if !strings.Contains(got, want) {
			t.Errorf("buildInvocationCmd missing %q\n  got: %s", want, got)
		}
	}
}

func TestBuildInvocationCmd_EscapesSingleQuotes(t *testing.T) {
	got := buildInvocationCmd(agent.AssistantClaude, "claude", "tmux-mgr", "it's fine", "")
	if !strings.Contains(got, `it'\''s fine`) {
		t.Errorf("buildInvocationCmd did not escape single quote\n  got: %s", got)
	}
}

func TestBuildInvocationCmd_PropagatesModel(t *testing.T) {
	got := buildInvocationCmd(agent.AssistantClaude, "claude", "tmux-mgr", "do work", "claude-haiku-4-5-20251001")
	if !strings.Contains(got, "TMUX_MGR_MODEL='claude-haiku-4-5-20251001'") {
		t.Errorf("expected TMUX_MGR_MODEL env to be forwarded\n  got: %s", got)
	}
	// Env vars should precede the binary path so they apply to the child process.
	modelIdx := strings.Index(got, "TMUX_MGR_MODEL=")
	binIdx := strings.Index(got, "tmux-mgr agent execute")
	if modelIdx < 0 || binIdx < 0 || modelIdx > binIdx {
		t.Errorf("TMUX_MGR_MODEL must be exported before the binary invocation\n  got: %s", got)
	}
}

func TestBuildInstruction_Claude(t *testing.T) {
	got := buildInstruction(agent.AssistantClaude, "Write hello to RESULT.md")
	if strings.Contains(got, "@generalist") {
		t.Errorf("Claude instruction must not include Gemini-specific @generalist prefix\n  got: %s", got)
	}
	if !strings.Contains(got, "RESULT.md") {
		t.Errorf("Claude instruction must mandate RESULT.md\n  got: %s", got)
	}
	if !strings.Contains(got, "Write hello to RESULT.md") {
		t.Errorf("Claude instruction must embed the task\n  got: %s", got)
	}
}

func TestBuildInstruction_Gemini(t *testing.T) {
	got := buildInstruction(agent.AssistantGemini, "Write hello to RESULT.md")
	if !strings.Contains(got, "@generalist") {
		t.Errorf("Gemini instruction must include @generalist prefix\n  got: %s", got)
	}
	if !strings.Contains(got, "RESULT.md") {
		t.Errorf("Gemini instruction must mandate RESULT.md\n  got: %s", got)
	}
}
