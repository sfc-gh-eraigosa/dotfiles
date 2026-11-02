package cmd

import (
	"fmt"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/config"
	"github.com/spf13/cobra"
)

// edit loads the config, applies f, validates, and saves only if all of that
// worked — a refused change writes nothing.
func edit(f func(c *config.Config) error) error {
	path, err := config.Path()
	if err != nil {
		return err
	}
	c, err := config.Load(path)
	if err != nil {
		return err
	}
	if err := f(&c); err != nil {
		return err
	}
	if err := c.Validate(); err != nil {
		return err
	}
	return config.Save(path, c)
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Read and change gdash's config (profiles and views)",
	Long: `gdash's config names Grafana profiles (a URL and where its credential
comes from) and views (a dashboard, its range and refresh). It never holds a
credential: a profile points at a 0600 file, an environment variable, or a
command that prints one.`,
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the config file's path",
	Args:  cobra.NoArgs,
	RunE: func(c *cobra.Command, _ []string) error {
		p, err := config.Path()
		if err != nil {
			return err
		}
		fmt.Fprintln(c.OutOrStdout(), p)
		return nil
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Print one value, e.g. views.ci.dashboard",
	Args:  cobra.ExactArgs(1),
	RunE: func(c *cobra.Command, args []string) error {
		p, err := config.Path()
		if err != nil {
			return err
		}
		cfg, err := config.Load(p)
		if err != nil {
			return err
		}
		v, err := cfg.Get(args[0])
		if err != nil {
			return err
		}
		fmt.Fprintln(c.OutOrStdout(), v)
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set one value, e.g. views.ci.vars.node All",
	Args:  cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		return edit(func(c *config.Config) error { return c.Set(args[0], args[1]) })
	},
}

var configAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a profile or a view",
}

var addProfile struct{ url, file, env, cmd, user string }

var configAddProfileCmd = &cobra.Command{
	Use:   "profile <name> --url URL (--auth-file F | --auth-env VAR | --auth-cmd CMD) [--user U]",
	Short: "Add (or replace) a Grafana profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		a := addProfile
		addProfile = struct{ url, file, env, cmd, user string }{}
		return edit(func(c *config.Config) error {
			name := args[0]
			sets := [][2]string{{"url", a.url}, {"auth.env", a.env}, {"auth.cmd", a.cmd}, {"auth.user", a.user}, {"auth.file", a.file}}
			if c.Profiles != nil {
				delete(c.Profiles, name)
			}
			for _, s := range sets {
				if s[1] == "" {
					continue
				}
				if err := c.Set("profiles."+name+"."+s[0], s[1]); err != nil {
					return err
				}
			}
			if _, ok := c.Profiles[name]; !ok {
				return fmt.Errorf("profile %s: --url is required", name)
			}
			if c.DefaultProfile == "" {
				c.DefaultProfile = name
			}
			return nil
		})
	},
}

var addView struct{ profile, dashboard, rng, refresh string }

var configAddViewCmd = &cobra.Command{
	Use:   "view <name> --dashboard UID|TITLE [--profile P] [--range 1h] [--refresh 30s]",
	Short: "Add (or replace) a view",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		v := addView
		addView = struct{ profile, dashboard, rng, refresh string }{}
		return edit(func(c *config.Config) error {
			if c.Views == nil {
				c.Views = map[string]config.View{}
			}
			c.Views[args[0]] = config.View{Profile: v.profile, Dashboard: v.dashboard, Range: v.rng, Refresh: v.refresh}
			return nil
		})
	},
}

var configRemoveCmd = &cobra.Command{
	Use:   "remove profile|view <name>",
	Short: "Remove a profile or a view",
	Args:  cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		return edit(func(c *config.Config) error {
			switch args[0] {
			case "profile":
				if _, ok := c.Profiles[args[1]]; !ok {
					return fmt.Errorf("no profile named %q", args[1])
				}
				delete(c.Profiles, args[1])
				if c.DefaultProfile == args[1] {
					c.DefaultProfile = ""
				}
			case "view":
				if _, ok := c.Views[args[1]]; !ok {
					return fmt.Errorf("no view named %q", args[1])
				}
				delete(c.Views, args[1])
			default:
				return fmt.Errorf("remove what? %q (want profile or view)", args[0])
			}
			return nil
		})
	},
}

func init() {
	f := configAddProfileCmd.Flags()
	f.StringVar(&addProfile.url, "url", "", "Grafana base URL, e.g. http://192.168.0.128:30300")
	f.StringVar(&addProfile.file, "auth-file", "", "file holding the token (must be 0600)")
	f.StringVar(&addProfile.env, "auth-env", "", "environment variable holding the token")
	f.StringVar(&addProfile.cmd, "auth-cmd", "", "command that prints the token (must not prompt)")
	f.StringVar(&addProfile.user, "user", "", "basic auth user (the source then holds the password)")
	v := configAddViewCmd.Flags()
	v.StringVar(&addView.dashboard, "dashboard", "", "dashboard uid or exact title (see gdash list)")
	v.StringVar(&addView.profile, "profile", "", "profile to use (default: default_profile)")
	v.StringVar(&addView.rng, "range", "", "time range, e.g. 1h, 7d (default: the dashboard's own, else 1h)")
	v.StringVar(&addView.refresh, "refresh", "", "refresh interval, e.g. 30s (default 30s, floor 5s)")
	configAddCmd.AddCommand(configAddProfileCmd, configAddViewCmd)
	configCmd.AddCommand(configPathCmd, configGetCmd, configSetCmd, configAddCmd, configRemoveCmd)
	rootCmd.AddCommand(configCmd)
}
