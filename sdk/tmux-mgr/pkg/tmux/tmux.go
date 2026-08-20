package tmux

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Manager handles tmux operations and state.
type Manager struct {
	Verbose bool
}

// Run executes a tmux command and returns output.
func (m *Manager) Run(args ...string) (string, error) {
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return "", fmt.Errorf("tmux binary not found in PATH. Please ensure tmux is installed and available: %w", err)
	}

	if m.Verbose {
		log.Printf("Executing: %s %s", tmuxPath, strings.Join(args, " "))
	}
	cmd := exec.Command(tmuxPath, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Attach attaches to an existing tmux session interactively.
func (m *Manager) Attach(name string) error {
	if m.Verbose {
		log.Printf("Attaching to session: %s", name)
	}
	c := exec.Command("tmux", "attach-session", "-t", name)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// SetPaneTitle sets the title of the current pane.
func (m *Manager) SetPaneTitle(title string) error {
	_, err := m.Run("select-pane", "-T", title)
	return err
}

// Layout represents a tmux window layout.
type Layout struct {
	Name    string       `json:"name"`
	Windows []WindowInfo `json:"windows"`
}

// WindowInfo represents a single tmux window state.
type WindowInfo struct {
	Name   string `json:"name"`
	Layout string `json:"layout"`
	Index  int    `json:"index"`
}

// SaveLayout captures the current tmux layout to a file.
func (m *Manager) SaveLayout(name string) error {
	out, err := m.Run("list-windows", "-F", "#{window_index}|#{window_name}|#{window_layout}")
	if err != nil {
		return fmt.Errorf("error listing windows: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	var windows []WindowInfo
	for _, line := range lines {
		parts := strings.Split(line, "|")
		if len(parts) == 3 {
			idx, _ := strconv.Atoi(parts[0])
			windows = append(windows, WindowInfo{Index: idx, Name: parts[1], Layout: parts[2]})
		}
	}

	layout := Layout{Name: name, Windows: windows}
	data, err := json.MarshalIndent(layout, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling layout: %w", err)
	}

	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".config", "tmux-mgr", name+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("error writing layout file: %w", err)
	}

	log.Printf("Layout %s saved to %s", name, path)
	return nil
}

// RestoreLayout reapplies a saved layout.
func (m *Manager) RestoreLayout(name string) error {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".config", "tmux-mgr", name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("layout file %s not found: %w", path, err)
	}

	var layout Layout
	if err := json.Unmarshal(data, &layout); err != nil {
		return fmt.Errorf("error unmarshaling layout: %w", err)
	}

	// This function returns an error, so a step that fails must not be swallowed
	// and then reported as "Layout restored".
	for _, win := range layout.Windows {
		for _, step := range [][]string{
			{"select-window", "-t", strconv.Itoa(win.Index)},
			{"select-layout", win.Layout},
			{"rename-window", "-t", strconv.Itoa(win.Index), win.Name},
		} {
			if _, err := m.Run(step...); err != nil {
				return fmt.Errorf("restore layout %s: tmux %s: %w", name, strings.Join(step, " "), err)
			}
		}
	}

	log.Printf("Layout %s restored", name)
	return nil
}

// Capture captures the text content of a pane.
func (m *Manager) Capture(target string) (string, error) {
	out, err := m.Run("capture-pane", "-pt", target)
	if err != nil {
		return "", fmt.Errorf("error capturing pane %s: %w", target, err)
	}
	return out, nil
}

// CreatePane creates a new tmux pane for an agent session and returns the
// resulting pane ID (e.g. "%42") so callers can track liveness later.
//
// symbol is an optional emoji prepended to the pane title (typically the
// `Symbol:` value from a `.ai/agents/<name>.md` definition). When empty the
// default 🤖 is used.
// label is an optional human-readable name (e.g. the agent's Persona) used
// in the pane title in place of the raw session ID. When empty, sessionID is
// used.
func CreatePane(sessionID, worktreePath, command, symbol, label string) (string, error) {
	paneCmd := fmt.Sprintf("cd %s && %s", worktreePath, command)

	target, err := RootPaneID()
	if err != nil {
		return "", fmt.Errorf("cannot create pane: %w", err)
	}

	args := []string{
		"split-window",
		"-h",
		"-l", "30%",
		"-t", target,
		"-P",
		"-F", "#{pane_id}",
		paneCmd,
	}

	tmuxCmd := exec.Command("tmux", args...)

	output, err := tmuxCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to create tmux pane: %w, Output: %s", err, string(output))
	}

	paneID := strings.TrimSpace(string(output))

	if symbol == "" {
		symbol = "🤖"
	}
	if label == "" {
		label = sessionID
	}
	styledTitle := fmt.Sprintf("#[bg=colour33,fg=white,bold] %s %s #[default]", symbol, label)
	// Cosmetic pane decoration: the pane already exists and is returned below,
	// so a styling failure must not fail pane creation.
	_ = exec.Command("tmux", "select-pane", "-t", paneID, "-T", styledTitle).Run()
	_ = exec.Command("tmux", "set-option", "-w", "-t", paneID, "pane-border-status", "top").Run()

	fmt.Printf("Created tmux pane: %s\n", paneID)
	return paneID, nil
}
