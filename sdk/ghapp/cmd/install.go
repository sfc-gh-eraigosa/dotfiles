package cmd

import (
	"fmt"
	"sort"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/ghapp/pkg/ghapp"
	"github.com/spf13/cobra"
)

// recordInstallations lists the App's installations and stores
// account → installation id. Returns the sorted accounts.
func recordInstallations(cmd *cobra.Command, g *globals, app ghapp.App, apps ghapp.Apps) ([]ghapp.Installation, error) {
	insts, err := app.Installations(cmd.Context())
	if err != nil {
		return nil, err
	}
	stored := apps[app.Slug]
	if stored.Installs == nil {
		stored.Installs = map[string]int64{}
	}
	for _, i := range insts {
		stored.Installs[i.Account] = i.ID
	}
	apps[app.Slug] = stored
	if err := g.store().Save(apps); err != nil {
		return nil, err
	}
	sort.Slice(insts, func(a, b int) bool { return insts[a].Account < insts[b].Account })
	return insts, nil
}

func newInstallCmd(g *globals) *cobra.Command {
	var noBrowser bool
	c := &cobra.Command{
		Use:   "install",
		Short: "Open the App's install page, then record where it is installed",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, apps, err := g.selectApp()
			if err != nil {
				return err
			}
			if !noBrowser {
				u := fmt.Sprintf("%s/apps/%s/installations/new", g.webURL, app.Slug)
				if err := openBrowser(u); err != nil {
					return err
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "install the App in the browser, then re-run `ghapp install --no-browser` (or `ghapp status`) to record it\n")
			}
			insts, err := recordInstallations(cmd, g, app, apps)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(insts) == 0 {
				fmt.Fprintln(out, "no installations yet")
				return nil
			}
			fmt.Fprintf(out, "%s is installed on:\n", app.Slug)
			for _, i := range insts {
				fmt.Fprintf(out, "  %-24s id %-8d %s (%s repositories)\n", i.Account, i.ID, i.TargetType, i.RepositorySelection)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&noBrowser, "no-browser", false, "skip opening the install page; only record installations")
	return c
}
