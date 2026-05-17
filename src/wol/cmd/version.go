package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	Version   = "dev"
	Commit    = "none"
	Dirty     = "false"
	BuildDate = "unknown"
)

type VersionInfo struct {
	Version     string `json:"version"`
	Commit      string `json:"commit"`
	Dirty       string `json:"dirty"`
	BuildDate   string `json:"build_date"`
	Description string `json:"description"`
	Path        string `json:"path"`
}

var versionJSON bool

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Run: func(cmd *cobra.Command, args []string) {
		execPath, _ := os.Executable()
		info := VersionInfo{
			Version:     Version,
			Commit:      Commit,
			Dirty:       Dirty,
			BuildDate:   BuildDate,
			Description: "wol - A simple utility to send Wake-on-LAN magic packets",
			Path:        execPath,
		}

		if versionJSON {
			data, _ := json.MarshalIndent(info, "", "  ")
			fmt.Println(string(data))
			return
		}

		fmt.Printf("wol v%s\n", info.Version)
		fmt.Printf("  Commit:      %s\n", info.Commit)
		fmt.Printf("  Dirty:       %s\n", info.Dirty)
		fmt.Printf("  Build Date:  %s\n", info.BuildDate)
		fmt.Printf("  Description: %s\n", info.Description)
		fmt.Printf("  Location:    %s\n", info.Path)
	},
}

func init() {
	versionCmd.Flags().BoolVar(&versionJSON, "json", false, "Output version info as JSON")
	rootCmd.AddCommand(versionCmd)
}
