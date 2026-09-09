package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/version"
	"github.com/spf13/cobra"
)

// VersionInfo is the shape returned by `gsl version --json`.
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
		v := version.Get()
		info := VersionInfo{
			Version:     v.Version,
			Commit:      v.Commit,
			Dirty:       v.Dirty,
			BuildDate:   v.BuildDate,
			Description: "gsl — Go Status Line for Claude Code and Antigravity CLI",
			Path:        execPath,
		}

		if versionJSON {
			data, _ := json.MarshalIndent(info, "", "  ")
			fmt.Println(string(data))
			return
		}

		fmt.Printf("gsl v%s\n", info.Version)
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
