package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
	"github.com/spf13/cobra"
)

// discoverRow is one concrete host in ~/.ssh/config and whether it is already
// in the fleet. "available" rows are the ones `fleet add <alias>` can adopt.
type discoverRow struct {
	Alias    string `json:"alias"`
	HostName string `json:"hostname,omitempty"`
	InFleet  bool   `json:"in_fleet"`
}

// discoverRows lists every concrete Host block (pattern hosts omitted, as in
// Parse), sorted by alias for deterministic output.
func discoverRows(cfg, marker string) []discoverRow {
	hosts, _ := sshconf.Parse(cfg, marker)
	rows := make([]discoverRow, 0, len(hosts))
	for _, h := range hosts {
		rows = append(rows, discoverRow{Alias: h.Alias, HostName: h.HostName, InFleet: h.Fleet})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Alias < rows[j].Alias })
	return rows
}

func renderDiscoverTable(rows []discoverRow) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-16s %-20s %s\n", "HOST", "HOSTNAME", "STATUS")
	for _, r := range rows {
		host := r.HostName
		if host == "" {
			host = "-"
		}
		status := "available"
		if r.InFleet {
			status = "in-fleet"
		}
		fmt.Fprintf(&b, "%-16s %-20s %s\n", r.Alias, host, status)
	}
	return b.String()
}

func renderDiscoverJSON(rows []discoverRow) string {
	// rows is never nil (discoverRows returns a non-nil slice), so this encodes
	// to [] rather than null when the config has no hosts.
	buf, _ := json.MarshalIndent(rows, "", "  ")
	return string(buf)
}

var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "List ssh-config hosts and which are in the fleet (adopt an available one with `fleet add`)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := readConfig(flagConfig)
		if err != nil {
			return err
		}
		rows := discoverRows(cfg, flagMarker)
		out := cmd.OutOrStdout()
		if flagJSON {
			fmt.Fprintln(out, renderDiscoverJSON(rows))
			return nil
		}
		fmt.Fprint(out, renderDiscoverTable(rows))
		var available int
		for _, r := range rows {
			if !r.InFleet {
				available++
			}
		}
		if available > 0 {
			fmt.Fprintf(out, "\n%d host(s) available — adopt one with `fleet add <alias>`\n", available)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(discoverCmd)
}
