package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/mcp"
	"github.com/spf13/cobra"
)

// mcpRunner and mcpOptions are the seams the mcp verbs use; tests replace them.
var (
	mcpRunner  mcp.Runner = mcp.NewSystemRunner()
	mcpOptions            = mcp.Options{}
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "MCP server health: the cached status and its out-of-band refresh",
	Long: `The status line never runs ` + "`claude mcp list`" + ` itself (it takes seconds and
dials every server). It reads a per-directory cache, and when that is stale it
starts ` + "`gsl mcp refresh`" + ` in the background. These verbs show and refresh it.`,
}

var mcpStatusJSON bool

var mcpStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Print the cached MCP status for the current directory",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		s, err := mcp.Get(cmd.Context(), cwd, mcpOptions)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "gsl: reading MCP config: %v\n", err)
		}
		if mcpStatusJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(s)
		}
		writeMCPStatus(cmd.OutOrStdout(), s, time.Now())
		return nil
	},
}

var mcpRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Run `claude mcp list` now and rewrite the cache",
	Long: `refresh probes every MCP server (` + "`claude mcp list`" + `, a few seconds) and
rewrites this directory's cache. Run by hand it ignores the failure backoff;
the status line starts it in the background (with ` + mcp.RefreshEnv + ` set), where the
backoff and the single-flight lock apply.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		o := mcpOptions
		background := os.Getenv(mcp.RefreshEnv) != ""
		o.Force = !background
		if err := mcp.Refresh(cmd.Context(), mcpRunner, cwd, o); err != nil {
			return fmt.Errorf("mcp refresh: %w", err)
		}
		if background {
			return nil
		}
		s, _ := mcp.Get(cmd.Context(), cwd, mcpOptions)
		writeMCPStatus(cmd.OutOrStdout(), s, time.Now())
		return nil
	},
}

// writeMCPStatus is the human form: a summary line, then each probed server
// that is not fine, grouped by state.
func writeMCPStatus(w io.Writer, s mcp.Status, now time.Time) {
	if s.FetchedAt.IsZero() {
		fmt.Fprintf(w, "MCP: %d configured, not probed yet — run `gsl mcp refresh`\n", s.Configured)
		return
	}
	age := now.Sub(s.FetchedAt).Round(time.Second)
	stale := ""
	if s.Stale {
		stale = " (stale)"
	}
	fmt.Fprintf(w, "MCP: %d connected · %d failed · %d needs auth · %d configured — probed %s ago%s\n",
		s.Connected, s.Failed, s.NeedsAuth, s.Configured, age, stale)
	byState := map[mcp.State][]string{}
	for _, sv := range s.Servers {
		byState[sv.State] = append(byState[sv.State], sv.Name)
	}
	for _, st := range []mcp.State{mcp.StateFailed, mcp.StateNeedsAuth, mcp.StateConnecting} {
		names := byState[st]
		if len(names) == 0 {
			continue
		}
		sort.Strings(names)
		fmt.Fprintf(w, "  %s: %s\n", st, strings.Join(names, ", "))
	}
}

func init() {
	mcpStatusCmd.Flags().BoolVar(&mcpStatusJSON, "json", false, "print the status as JSON")
	mcpCmd.AddCommand(mcpStatusCmd, mcpRefreshCmd)
	rootCmd.AddCommand(mcpCmd)
}
