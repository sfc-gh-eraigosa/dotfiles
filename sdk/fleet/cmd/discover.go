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

// adoptAll marks every available host in ONE pass, returning the new config
// and the aliases adopted (table order). A single pass matters: N separate
// `fleet add` runs would take N backups and rewrite the file N times, widening
// the window in which a partial write costs SSH access to every machine.
//
// It returns the config unchanged when nothing is available, so the caller can
// skip the write — and therefore the backup — entirely.
func adoptAll(cfg, marker string) (string, []string, error) {
	next := cfg
	var adopted []string
	for _, r := range discoverRows(cfg, marker) {
		if r.InFleet {
			continue
		}
		marked, err := sshconf.Mark(next, r.Alias, marker)
		if err != nil {
			return "", nil, fmt.Errorf("adopting %s: %w", r.Alias, err)
		}
		next = marked
		adopted = append(adopted, r.Alias)
	}
	if len(adopted) == 0 {
		return cfg, nil, nil
	}
	return next, adopted, nil
}

var (
	discoverAddAll bool
	discoverYes    bool
	discoverDryRun bool
)

var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "List ssh-config hosts and which are in the fleet (`--add-all` adopts every available one)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := readConfig(flagConfig)
		if err != nil {
			return err
		}
		rows := discoverRows(cfg, flagMarker)
		out := cmd.OutOrStdout()
		if flagJSON && !discoverAddAll {
			fmt.Fprintln(out, renderDiscoverJSON(rows))
			return nil
		}
		fmt.Fprint(out, renderDiscoverTable(rows))

		var available []string
		for _, r := range rows {
			if !r.InFleet {
				available = append(available, r.Alias)
			}
		}
		if !discoverAddAll {
			if len(available) > 0 {
				fmt.Fprintf(out, "\n%d host(s) available — adopt one with `fleet add <alias>`,"+
					" or all of them with `fleet discover --add-all`\n", len(available))
			}
			return nil
		}

		if len(available) == 0 {
			fmt.Fprintln(out, "\nnothing to adopt — every ssh-config host is already in the fleet")
			return nil
		}
		// Name every host before touching anything: this edits the file every
		// remote command depends on.
		fmt.Fprintf(out, "\nwill adopt %d host(s): %s\n", len(available), strings.Join(available, ", "))

		next, adopted, err := adoptAll(cfg, flagMarker)
		if err != nil {
			return err
		}
		if discoverDryRun {
			return applyConfig(out, flagConfig, next, true)
		}
		if !discoverYes && !askYesNo(cmd, "adopt them into the fleet?") {
			fmt.Fprintln(out, "nothing changed")
			return nil
		}
		if err := applyConfig(out, flagConfig, next, false); err != nil {
			return err
		}
		fmt.Fprintf(out, "adopted %d host(s) into the fleet: %s\n", len(adopted), strings.Join(adopted, ", "))
		return nil
	},
}

func init() {
	discoverCmd.Flags().BoolVar(&discoverAddAll, "add-all", false, "adopt every available ssh-config host into the fleet")
	discoverCmd.Flags().BoolVar(&discoverYes, "yes", false, "skip the confirmation prompt (non-interactive)")
	discoverCmd.Flags().BoolVar(&discoverDryRun, "dry-run", false, "print the resulting config without writing")
	rootCmd.AddCommand(discoverCmd)
}
