package cmd

import (
	"fmt"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/ghapp/pkg/ghapp"
	"github.com/spf13/cobra"
)

// splitRepo validates owner/repo.
func splitRepo(s string) (owner, repo string, err error) {
	parts := strings.Split(s, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("%w: --repo wants owner/repo, got %q", ErrUsage, s)
	}
	return parts[0], parts[1], nil
}

// parsePermissions turns k=v pairs into the permissions map.
func parsePermissions(kv []string) (map[string]string, error) {
	if len(kv) == 0 {
		return nil, nil
	}
	out := map[string]string{}
	for _, p := range kv {
		k, v, ok := strings.Cut(p, "=")
		if !ok || k == "" || v == "" {
			return nil, fmt.Errorf("%w: --permissions wants name=read|write, got %q", ErrUsage, p)
		}
		out[k] = v
	}
	return out, nil
}

// installationFor resolves owner → installation id, discovering and
// recording it from the API when the store does not know it yet.
func installationFor(cmd *cobra.Command, g *globals, app ghapp.App, apps ghapp.Apps, owner string) (int64, error) {
	if id, ok := apps[app.Slug].Installs[owner]; ok {
		return id, nil
	}
	insts, err := recordInstallations(cmd, g, app, apps)
	if err != nil {
		return 0, err
	}
	for _, i := range insts {
		if i.Account == owner {
			return i.ID, nil
		}
	}
	return 0, fmt.Errorf("ghapp: %s has no installation for %q — run `ghapp install`", app.Slug, owner)
}

func newTokenCmd(g *globals) *cobra.Command {
	var repo, org string
	var perms []string
	c := &cobra.Command{
		Use:   "token",
		Short: "Mint an installation token; prints ONLY the token on stdout",
		Long: `Mints a short-lived installation token for --repo owner/repo (scoped to that
repository) or --org name (organization-wide). Use it like
    GH_TOKEN=$(ghapp token --repo owner/repo) gcfg verify
Nothing else is written to stdout, so the value never mixes with diagnostics.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if (repo == "") == (org == "") {
				return fmt.Errorf("%w: exactly one of --repo owner/repo or --org name is required", ErrUsage)
			}
			var owner string
			scope := ghapp.TokenScope{}
			if repo != "" {
				o, r, err := splitRepo(repo)
				if err != nil {
					return err
				}
				owner = o
				scope.Repositories = []string{r}
			} else {
				owner = org
			}
			p, err := parsePermissions(perms)
			if err != nil {
				return err
			}
			scope.Permissions = p
			app, apps, err := g.selectApp()
			if err != nil {
				return err
			}
			inst, err := installationFor(cmd, g, app, apps, owner)
			if err != nil {
				return err
			}
			tok, err := app.Token(cmd.Context(), inst, scope)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), tok.Value)
			return nil
		},
	}
	c.Flags().StringVar(&repo, "repo", "", "owner/repo to scope the token to")
	c.Flags().StringVar(&org, "org", "", "organization (or user) installation to use")
	c.Flags().StringArrayVar(&perms, "permissions", nil, "name=read|write (repeatable); default: everything the installation grants")
	return c
}
