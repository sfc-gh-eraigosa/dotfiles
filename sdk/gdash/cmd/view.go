package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/app"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/auth"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/grafana"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/render"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/theme"
	"golang.org/x/term"
)

// HTTPTimeout bounds each Grafana request the dashboard makes (spec F1).
const HTTPTimeout = 10 * time.Second

type viewFlags struct{ profile, dashboard, rng, refresh string }

var vf viewFlags

// pickView chooses what to draw: a named view, or an ad hoc --dashboard,
// with --profile/--range/--refresh overriding, and defaults filled in.
func pickView(c config.Config, args []string, f viewFlags) (string, config.View, config.Profile, error) {
	if len(c.Profiles) == 0 {
		return "", config.View{}, config.Profile{}, fmt.Errorf("no Grafana profile yet: gdash config add profile lab --url http://<grafana> --auth-file ~/.config/gdash/lab.cred")
	}
	var name string
	switch {
	case len(args) == 1:
		name = args[0]
		if _, ok := c.Views[name]; !ok {
			return "", config.View{}, config.Profile{}, fmt.Errorf("no view named %q (views: %s)", name, viewNames(c))
		}
	case f.dashboard != "":
		name = f.dashboard
		if c.Views == nil {
			c.Views = map[string]config.View{}
		}
		c.Views[name] = config.View{Dashboard: f.dashboard}
	default:
		return "", config.View{}, config.Profile{}, fmt.Errorf("name a view (views: %s) or pass --dashboard", viewNames(c))
	}
	v := c.Views[name]
	if f.profile != "" {
		v.Profile = f.profile
	}
	if f.rng != "" {
		v.Range = f.rng
	}
	if f.refresh != "" {
		v.Refresh = f.refresh
	}
	c.Views[name] = v
	if err := c.Validate(); err != nil {
		return "", config.View{}, config.Profile{}, err
	}
	v, p, err := c.Resolve(name)
	return name, v, p, err
}

func viewNames(c config.Config) string {
	names := make([]string, 0, len(c.Views))
	for n := range c.Views {
		names = append(names, n)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "none yet; gdash config add view <name> --dashboard <uid>"
	}
	return strings.Join(names, ", ")
}

// runView draws a view full screen until q.
func runView(args []string) error {
	path, err := config.Path()
	if err != nil {
		return err
	}
	c, err := config.Load(path)
	if err != nil {
		return err
	}
	name, v, p, err := pickView(c, args, vf)
	if err != nil {
		return err
	}
	cred, err := auth.Load(p.Auth)
	if err != nil {
		return err
	}
	// The sdk's one color scheme (sdk/libs/tui/theme): NO_COLOR, then
	// GDASH_THEME / SDK_THEME, tmux or 8 colors → dark8, else the background.
	color := theme.Enabled(os.Getenv) && term.IsTerminal(int(os.Stdout.Fd())) //nolint:gosec // Fd fits an int
	pal, _ := theme.Get(theme.Resolve("gdash", os.Getenv, theme.EnvDarkBackground(os.Getenv)))
	m := app.New(app.Options{
		Fetcher: app.NewLive(grafana.NewHTTP(p.URL, cred, HTTPTimeout), v),
		View:    name, Range: v.Range, Refresh: v.Refresh,
		Render: render.Opts{Color: color, Palette: pal},
	})
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}
