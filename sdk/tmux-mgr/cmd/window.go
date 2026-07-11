package cmd

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/tmux-mgr/pkg/tmux"
	"github.com/spf13/cobra"
)

var windowCmd = &cobra.Command{
	Use:   "window",
	Short: "Manage tmux windows and panes",
}

func init() {
	windowCmd.AddCommand(&cobra.Command{
		Use:   "move [left|right|up|down]",
		Short: "Move focus to a different pane",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			dir := args[0]
			var tmuxArg string
			switch dir {
			case "left":
				tmuxArg = "-L"
			case "right":
				tmuxArg = "-R"
			case "up":
				tmuxArg = "-U"
			case "down":
				tmuxArg = "-D"
			default:
				fmt.Println("Invalid direction. Use left|right|up|down")
				return
			}
			Tmgr.Run("select-pane", tmuxArg)
			log.Printf("Moved focus %s", dir)
		},
	})

	windowCmd.AddCommand(&cobra.Command{
		Use:   "split [horizontal|vertical|left|right|up|down]",
		Short: "Split a tmux window",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			dir := "horizontal"
			if len(args) > 0 {
				dir = args[0]
			}
			var tmuxArg string
			switch dir {
			case "horizontal", "right", "left":
				tmuxArg = "-h"
			case "vertical", "up", "down":
				tmuxArg = "-v"
			default:
				fmt.Println("Invalid direction. Use horizontal|vertical|left|right|up|down")
				return
			}
			target, err := tmux.RootPaneID()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				os.Exit(1)
			}
			Tmgr.Run("split-window", tmuxArg, "-t", target)
			log.Printf("Split window %s", dir)
		},
	})

	windowCmd.AddCommand(&cobra.Command{
		Use:   "new [name]",
		Short: "Create a new tmux window",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			var tmuxArgs []string
			tmuxArgs = append(tmuxArgs, "new-window")
			if len(args) > 0 {
				tmuxArgs = append(tmuxArgs, "-n", args[0])
			}
			Tmgr.Run(tmuxArgs...)
			log.Printf("Created new window")
		},
	})

	windowCmd.AddCommand(&cobra.Command{
		Use:   "kill",
		Short: "Kill the current tmux pane",
		Run: func(cmd *cobra.Command, args []string) {
			Tmgr.Run("kill-pane")
			log.Printf("Killed current pane")
		},
	})

	windowCmd.AddCommand(&cobra.Command{
		Use:   "swap [left|right|up|down]",
		Short: "Swap current pane with a neighbor",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			dir := args[0]
			var tmuxArg string
			switch dir {
			case "left":
				tmuxArg = "-L"
			case "right":
				tmuxArg = "-R"
			case "up":
				tmuxArg = "-U"
			case "down":
				tmuxArg = "-D"
			default:
				fmt.Println("Invalid direction. Use left|right|up|down")
				return
			}
			Tmgr.Run("swap-pane", tmuxArg)
			log.Printf("Swapped pane %s", dir)
		},
	})

	windowCmd.AddCommand(&cobra.Command{
		Use:   "rename [name]",
		Short: "Rename the current tmux window",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			name := args[0]
			Tmgr.Run("rename-window", name)
			log.Printf("Renamed window to %s", name)
		},
	})

	windowCmd.AddCommand(&cobra.Command{
		Use:   "rename-pane [title]",
		Short: "Rename the current tmux pane",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			title := args[0]
			// Ensure pane titles are visible
			Tmgr.Run("set", "-g", "pane-border-status", "top")
			err := Tmgr.SetPaneTitle(title)
			if err != nil {
				log.Printf("Error renaming pane to %s: %v", title, err)
				fmt.Printf("Error renaming pane to %s\n", title)
				return
			}
			log.Printf("Renamed pane to %s", title)
		},
	})

	windowCmd.AddCommand(&cobra.Command{
		Use:   "resize [left|right|up|down|width|height] [val]",
		Short: "Resize a tmux pane",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			dir := args[0]
			val := "5"
			if len(args) > 1 {
				val = args[1]
			}

			if strings.HasSuffix(dir, "%") || strings.HasSuffix(val, "%") {
				vStr := strings.TrimSuffix(val, "%")
				if strings.HasSuffix(dir, "%") {
					vStr = strings.TrimSuffix(dir, "%")
				}
				v, _ := strconv.Atoi(vStr)

				widthStr, _ := Tmgr.Run("display", "-p", "#{terminal_width}")
				heightStr, _ := Tmgr.Run("display", "-p", "#{terminal_height}")
				width, _ := strconv.Atoi(strings.TrimSpace(widthStr))
				height, _ := strconv.Atoi(strings.TrimSpace(heightStr))

				if dir == "width" || dir == "horizontal" || strings.HasSuffix(dir, "%") {
					newW := (width * v) / 100
					Tmgr.Run("resize-pane", "-x", strconv.Itoa(newW))
					log.Printf("Resized width to %d (%d%%)", newW, v)
				} else if dir == "height" || dir == "vertical" {
					newH := (height * v) / 100
					Tmgr.Run("resize-pane", "-y", strconv.Itoa(newH))
					log.Printf("Resized height to %d (%d%%)", newH, v)
				}
				return
			}

			var tmuxArg string
			switch dir {
			case "left":
				tmuxArg = "-L"
			case "right":
				tmuxArg = "-R"
			case "up":
				tmuxArg = "-U"
			case "down":
				tmuxArg = "-D"
			default:
				fmt.Println("Invalid direction. Use left|right|up|down or width/height with %")
				return
			}
			Tmgr.Run("resize-pane", tmuxArg, val)
			log.Printf("Resized %s by %s", dir, val)
		},
	})

	rootCmd.AddCommand(windowCmd)
}
