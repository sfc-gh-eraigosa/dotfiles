package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/tmux-mgr/pkg/agent"
)

// tmuxFeatureCmd groups feature-level convenience verbs that wrap the gss
// feature commands (design.md → tmux-mgr refactor "What's new" #2-4). gss owns
// the registry/worktrees/PRs; these verbs are ergonomic shells over it.
var tmuxFeatureCmd = &cobra.Command{
	Use:   "feature",
	Short: "Feature-level convenience verbs wrapping gss feature commands",
	Run:   func(cmd *cobra.Command, args []string) { _ = cmd.Help() },
}

var (
	featStartBase   string
	featStartDesc   string
	addAgentPurpose string
	addAgentTask    string
)

// --- feature start ---

var featStartCmd = &cobra.Command{
	Use:   "start <name>",
	Short: "Create a gss feature (wraps `gss feature start`)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return gssFeatureStart(defaultGssRunner, os.Stdout, args[0], featStartBase, featStartDesc)
	},
}

func gssFeatureStart(run gssRunner, out io.Writer, name, base, description string) error {
	args := []string{"feature", "start", name}
	if base != "" {
		args = append(args, "--base", base)
	}
	if description != "" {
		args = append(args, "--description", description)
	}
	o, err := run(args...)
	if len(o) > 0 {
		fmt.Fprint(out, string(o))
	}
	if err != nil {
		return fmt.Errorf("gss feature start: %w\n%s", err, o)
	}
	return nil
}

// --- feature add-agent ---

var featAddAgentCmd = &cobra.Command{
	Use:   "add-agent <feature>",
	Short: "Add a gss worker to a feature and spawn an agent pane",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if addAgentPurpose == "" {
			return fmt.Errorf("add-agent: --purpose is required")
		}
		if addAgentTask == "" {
			return fmt.Errorf("add-agent: --task-description is required")
		}
		feature := args[0]
		host := agent.DetectHost(os.Getenv)
		def, _ := agent.LoadDefinition(addAgentPurpose)
		model := resolveModel(def, host)
		sessionID := fmt.Sprintf("%s-%d", addAgentPurpose, time.Now().UnixNano())

		wa, err := gssWorkerAdd(defaultGssRunner, feature, addAgentPurpose, addAgentTask,
			os.Getenv("USER"), string(host), engineSessionID(host, os.Getenv), sessionID)
		if err != nil {
			return fmt.Errorf("add-agent: %w", err)
		}
		return spawnAgentPane(host, def, model, addAgentPurpose, sessionID, wa, addAgentTask)
	},
}

// --- feature status ---

var featStatusCmd = &cobra.Command{
	Use:   "status [feature]",
	Short: "Show gss feature state + agent pane liveness",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		feature := ""
		if len(args) == 1 {
			feature = args[0]
		}
		out, err := featureStatus(defaultGssRunner, agent.DefaultPaneChecker, feature)
		fmt.Fprint(os.Stdout, out)
		return err
	},
}

// featureStatus composes gss's view (`feature list --tree`, plus
// `feature conflicts` when a feature is named) with tmux-mgr's pane liveness
// (the sessions whose WorkerRef belongs to the feature). gss owns the data;
// tmux-mgr adds the "is the agent still running?" annotation.
func featureStatus(run gssRunner, isAlive agent.PaneChecker, feature string) (string, error) {
	var b strings.Builder

	listArgs := []string{"feature", "list", "--tree"}
	if feature != "" {
		listArgs = append(listArgs, "--feature", feature)
	}
	b.WriteString("== gss feature list ==\n")
	if out, err := run(listArgs...); err == nil {
		b.Write(out)
	} else {
		fmt.Fprintf(&b, "(gss feature list failed: %v)\n", err)
	}

	if feature != "" {
		if out, err := run("feature", "conflicts", "--feature", feature); err == nil && len(out) > 0 {
			b.WriteString("\n== gss conflicts ==\n")
			b.Write(out)
		}
	}

	sessions, err := agent.ListSessions()
	if err != nil {
		return b.String(), err
	}
	b.WriteString("\n== tmux-mgr agent panes ==\n")
	any := false
	for _, s := range sessions {
		if s.WorkerRef == "" {
			continue
		}
		if feature != "" && agent.FeatureOf(s.WorkerRef) != feature {
			continue
		}
		state := "dead"
		if isAlive(s.PaneID) {
			state = "alive"
		}
		fmt.Fprintf(&b, "  %s  pane=%s (%s)  session=%s\n", s.WorkerRef, s.PaneID, state, s.SessionID)
		any = true
	}
	if !any {
		b.WriteString("  (no agent panes for this feature)\n")
	}
	return b.String(), nil
}

func init() {
	featStartCmd.Flags().StringVar(&featStartBase, "base", "", "Base branch for the feature (default: main)")
	featStartCmd.Flags().StringVar(&featStartDesc, "description", "", "One-line feature description")

	featAddAgentCmd.Flags().StringVar(&addAgentPurpose, "purpose", "", "Worker purpose (required)")
	featAddAgentCmd.Flags().StringVarP(&addAgentTask, "task-description", "d", "", "Agent task description (required)")

	tmuxFeatureCmd.AddCommand(featStartCmd)
	tmuxFeatureCmd.AddCommand(featAddAgentCmd)
	tmuxFeatureCmd.AddCommand(featStatusCmd)
	rootCmd.AddCommand(tmuxFeatureCmd)
}
