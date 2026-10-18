package cmd

import (
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/video"
	"github.com/spf13/cobra"
)

// videoRefreshCmd is the hidden entry point the video segment re-execs
// (detached) to refresh the GPU cache; see internal/video.
var videoRefreshCmd = &cobra.Command{
	Use:    "video-refresh",
	Short:  "Refresh the cached GPU reading (internal; spawned by the video segment)",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		dir, err := video.Dir()
		if err != nil {
			return err
		}
		c := video.Cache{Dir: dir}
		return c.Refresh(video.Collector{Runner: video.SystemRunner{}})
	},
}

func init() { rootCmd.AddCommand(videoRefreshCmd) }
