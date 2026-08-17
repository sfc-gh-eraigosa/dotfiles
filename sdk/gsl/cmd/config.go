package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage the gsl configuration",
	Long:  "Manage the gsl configuration file (~/.config/gsl/config.json).",
}

var configGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Print the config (or a specific field)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(config.DefaultPath())
		if err != nil {
			return err
		}
		if len(args) == 0 {
			data, err := json.MarshalIndent(cfg, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}
		return printConfigKey(cfg, args[0])
	},
}

func printConfigKey(cfg config.Config, key string) error {
	switch key {
	case "enabled":
		fmt.Println(cfg.Enabled)
	case "style":
		fmt.Println(cfg.Style)
	case "timezone":
		fmt.Println(cfg.Timezone)
	case "time_format":
		fmt.Println(cfg.TimeFormat)
	case "date_format":
		fmt.Println(cfg.DateFormat)
	case "segments":
		data, _ := json.MarshalIndent(cfg.Segments, "", "  ")
		fmt.Println(string(data))
	case "styles":
		data, _ := json.MarshalIndent(cfg.Styles, "", "  ")
		fmt.Println(string(data))
	default:
		return fmt.Errorf("gsl config get: unknown key %q; use 'gsl config get' to see all", key)
	}
	return nil
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a config field",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, value := args[0], args[1]
		cfg, err := config.Load(config.DefaultPath())
		if err != nil {
			return err
		}
		switch key {
		case "style":
			cfg.Style = value
		case "timezone":
			cfg.Timezone = value
		case "time_format":
			cfg.TimeFormat = value
		case "date_format":
			cfg.DateFormat = value
		default:
			return fmt.Errorf("gsl config set: unknown key %q", key)
		}
		if err := config.Save(config.DefaultPath(), cfg); err != nil {
			return err
		}
		fmt.Printf("OK: %s = %s\n", key, value)
		return nil
	},
}

var configEnableCmd = &cobra.Command{
	Use:   "enable [segment]",
	Short: "Enable the master switch or a specific segment",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(config.DefaultPath())
		if err != nil {
			return err
		}
		if len(args) == 0 {
			cfg.EnableMaster()
			if err := config.Save(config.DefaultPath(), cfg); err != nil {
				return err
			}
			fmt.Println("OK: status line enabled")
			return nil
		}
		if err := cfg.EnableSegment(args[0]); err != nil {
			return err
		}
		if err := config.Save(config.DefaultPath(), cfg); err != nil {
			return err
		}
		fmt.Printf("OK: segment %q enabled\n", args[0])
		return nil
	},
}

var configDisableCmd = &cobra.Command{
	Use:   "disable [segment]",
	Short: "Disable the master switch or a specific segment",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(config.DefaultPath())
		if err != nil {
			return err
		}
		if len(args) == 0 {
			cfg.DisableMaster()
			if err := config.Save(config.DefaultPath(), cfg); err != nil {
				return err
			}
			fmt.Println("OK: status line disabled")
			return nil
		}
		if err := cfg.DisableSegment(args[0]); err != nil {
			return err
		}
		if err := config.Save(config.DefaultPath(), cfg); err != nil {
			return err
		}
		fmt.Printf("OK: segment %q disabled\n", args[0])
		return nil
	},
}

var configToggleCmd = &cobra.Command{
	Use:   "toggle <segment>",
	Short: "Toggle a segment on/off",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(config.DefaultPath())
		if err != nil {
			return err
		}
		if err := cfg.ToggleSegment(args[0]); err != nil {
			return err
		}
		if err := config.Save(config.DefaultPath(), cfg); err != nil {
			return err
		}
		// Find the resulting state to report.
		for _, seg := range cfg.Segments {
			if seg.Type == args[0] {
				state := "disabled"
				if seg.Enabled {
					state = "enabled"
				}
				fmt.Printf("OK: segment %q toggled → %s\n", args[0], state)
				return nil
			}
		}
		return nil
	},
}

var configStyleListFlag bool

var configStyleCmd = &cobra.Command{
	Use:   "style [name]",
	Short: "Set the active style or list available styles",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(config.DefaultPath())
		if err != nil {
			return err
		}

		if configStyleListFlag {
			return listStyles(cfg)
		}

		if len(args) == 0 {
			// No args, no --list: show current style.
			fmt.Printf("current style: %s\n", cfg.Style)
			return nil
		}

		newStyle := args[0]
		cfg.Style = newStyle
		if err := config.Save(config.DefaultPath(), cfg); err != nil {
			return err
		}
		fmt.Printf("OK: style set to %q\n", newStyle)
		return nil
	},
}

// listStyles prints all builtin styles and user-defined styles, marking the
// active one with *.
func listStyles(cfg config.Config) error {
	builtins := style.Builtins()

	// Collect all style names (builtins + user-defined).
	names := make([]string, 0, len(builtins))
	for k := range builtins {
		names = append(names, k)
	}
	// Add user styles from cfg.Styles that are not already in builtins.
	for k := range cfg.Styles {
		if _, ok := builtins[k]; !ok {
			names = append(names, k)
		}
	}
	sort.Strings(names)

	var sb strings.Builder
	for _, name := range names {
		marker := "  "
		if name == cfg.Style {
			marker = "* "
		}
		label := "builtin"
		if _, ok := builtins[name]; !ok {
			label = "user"
		}
		sb.WriteString(fmt.Sprintf("%s%-16s (%s)\n", marker, name, label))
	}
	fmt.Print(sb.String())
	return nil
}

func init() {
	configStyleCmd.Flags().BoolVarP(&configStyleListFlag, "list", "l", false, "List available styles")
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configEnableCmd)
	configCmd.AddCommand(configDisableCmd)
	configCmd.AddCommand(configToggleCmd)
	configCmd.AddCommand(configStyleCmd)
	rootCmd.AddCommand(configCmd)
}
