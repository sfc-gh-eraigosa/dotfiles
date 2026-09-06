// Package cmd wires the ghapp CLI verbs. NewRootCmd builds a fresh command
// tree each call (tests run many in one process); exit-code mapping lives
// only in main.go.
package cmd

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/ghapp/pkg/ghapp"
	"github.com/spf13/cobra"
)

// ErrUsage marks a usage / no-credential error; main maps it to exit 2.
var ErrUsage = errors.New("usage")

// openBrowser is the test seam for the browser opener.
var openBrowser = ghapp.OpenBrowser

// globals are the persistent flags every verb reads.
type globals struct {
	configDir string
	apiURL    string
	webURL    string
	app       string
}

func (g *globals) store() ghapp.FileStore { return ghapp.FileStore{Dir: g.configDir} }

// selectApp loads the store and picks the App: --app when given, the only
// one when there is one, otherwise a usage error listing the slugs.
func (g *globals) selectApp() (ghapp.App, ghapp.Apps, error) {
	apps, err := g.store().Load()
	if err != nil {
		return ghapp.App{}, nil, err
	}
	if len(apps) == 0 {
		return ghapp.App{}, nil, fmt.Errorf("%w: no GitHub App in %s — run `ghapp create` first", ErrUsage, g.configDir)
	}
	slugs := make([]string, 0, len(apps))
	for s := range apps {
		slugs = append(slugs, s)
	}
	sort.Strings(slugs)
	slug := g.app
	switch {
	case slug != "":
		if _, ok := apps[slug]; !ok {
			return ghapp.App{}, nil, fmt.Errorf("%w: unknown app %q (have: %s)", ErrUsage, slug, strings.Join(slugs, ", "))
		}
	case len(apps) == 1:
		slug = slugs[0]
	default:
		return ghapp.App{}, nil, fmt.Errorf("%w: several apps stored (%s); pick one with --app", ErrUsage, strings.Join(slugs, ", "))
	}
	return apps[slug].With(ghapp.Options{BaseURL: g.apiURL}), apps, nil
}

// NewRootCmd returns a fresh root command with every verb attached.
func NewRootCmd() *cobra.Command {
	g := &globals{}
	root := &cobra.Command{
		Use:           "ghapp",
		Short:         "GitHub App credential toolkit — create an App by manifest, mint installation tokens",
		SilenceUsage:  true,
		SilenceErrors: true,
		// Bare `ghapp` prints help; there is no TUI.
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	pf := root.PersistentFlags()
	pf.StringVar(&g.configDir, "config-dir", ghapp.DefaultDir(), "where apps.json and the PEMs live (0700)")
	pf.StringVar(&g.apiURL, "api-url", "https://api.github.com", "GitHub REST API base URL")
	pf.StringVar(&g.webURL, "web-url", "https://github.com", "GitHub web base URL")
	pf.StringVar(&g.app, "app", "", "App slug when more than one is stored")
	_ = pf.MarkHidden("api-url")
	_ = pf.MarkHidden("web-url")
	root.AddCommand(newVersionCmd(), newCreateCmd(g), newInstallCmd(g), newTokenCmd(g), newStatusCmd(g), newDoctorCmd(g))
	return root
}
