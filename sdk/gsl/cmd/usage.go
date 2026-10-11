package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/usage"
	"github.com/spf13/cobra"
)

var usageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Show the rate-limit usage the host last reported (5h / 7d)",
	Long: `usage prints the 5-hour and 7-day rate-limit usage that Claude Code or
Antigravity last handed to gsl render — the numbers on the status bar — so a
script can read them. Every render that carries rate limits records them under
${XDG_STATE_HOME:-~/.local/state}/gsl/usage/<host>.json.

The numbers are as fresh as the host's last turn: "seen" says how long ago.
With --host, exits 1 when that host has not reported yet.`,
	Args: cobra.NoArgs,
	RunE: runUsage,
}

var (
	usageHost string
	usageJSON bool
)

func init() {
	usageCmd.Flags().StringVar(&usageHost, "host", "", "only this host: claude or antigravity")
	usageCmd.Flags().BoolVar(&usageJSON, "json", false, "print JSON (an object with --host, else an array)")
	rootCmd.AddCommand(usageCmd)
}

func runUsage(cmd *cobra.Command, args []string) error {
	dir, err := usage.Dir()
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	now := time.Now()

	if usageHost != "" {
		s, err := usage.Read(dir, usageHost)
		if errors.Is(err, usage.ErrNone) {
			cmd.SilenceUsage = true
			return fmt.Errorf("%w — it appears after that host's next turn", err)
		}
		if err != nil {
			return err
		}
		if usageJSON {
			return writeJSON(out, s)
		}
		printSnapshot(out, s, now)
		return nil
	}

	all, err := usage.ReadAll(dir)
	if err != nil {
		return err
	}
	if usageJSON {
		if all == nil {
			all = []usage.Snapshot{}
		}
		return writeJSON(out, all)
	}
	if len(all) == 0 {
		fmt.Fprintln(out, "no usage recorded yet — it appears after the host's next turn")
		return nil
	}
	for _, s := range all {
		printSnapshot(out, s, now)
	}
	return nil
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// printSnapshot writes one host as a line:
//
//	claude       7d 34%  resets Thu Oct 8 09:00 (in 3d 4h)   5h 12%  resets 23:00 (in 1h)   seen 2m ago
func printSnapshot(w io.Writer, s usage.Snapshot, now time.Time) {
	line := fmt.Sprintf("%-12s %s   %s", s.Host, windowText("7d", s.SevenDay, now), windowText("5h", s.FiveHour, now))
	fmt.Fprintf(w, "%s   seen %s ago\n", line, roughDuration(now.Sub(s.CapturedAt)))
}

func windowText(name string, w *usage.Window, now time.Time) string {
	if w == nil {
		return name + " —"
	}
	text := fmt.Sprintf("%s %.0f%%", name, math.Round(w.UsedPercentage))
	if t, err := time.Parse(time.RFC3339, w.ResetsAt); err == nil {
		t = t.Local()
		layout := "15:04"
		if t.Sub(now) > 20*time.Hour {
			layout = "Mon Jan 2 15:04"
		}
		text += fmt.Sprintf("  resets %s (in %s)", t.Format(layout), roughDuration(t.Sub(now)))
	}
	return text
}

// roughDuration is a two-unit duration: 3d 4h, 1h 5m, 2m, 40s.
func roughDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%dd %dh", int(d.Hours())/24, int(d.Hours())%24)
	case d >= time.Hour:
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}
