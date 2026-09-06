package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/ghapp/pkg/ghapp"
	"github.com/spf13/cobra"
)

// defaultManifest is the App gcfg needs: repo administration + metadata.
// Others create their own from a --manifest file (README).
func defaultManifest(name string) ghapp.Manifest {
	return ghapp.Manifest{
		Name:        name,
		Description: "Settings-as-code for this account's repositories (gcfg)",
		Permissions: map[string]string{"administration": "write", "metadata": "read", "contents": "read"},
	}
}

func newCreateCmd(g *globals) *cobra.Command {
	var (
		name, manifestPath, org, hookURL string
		port                             int
		force, noBrowser                 bool
		timeout                          time.Duration
	)
	c := &cobra.Command{
		Use:   "create",
		Short: "Create a GitHub App via the manifest flow and store its key (0600)",
		Long: `Serves a one-page form on localhost, opens it in your browser, and lets GitHub
redirect back with a code that is exchanged for the App id + private key.
The key lands in --config-dir as <slug>.pem (0600); nothing secret is printed.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			m := defaultManifest(name)
			if manifestPath != "" {
				b, err := os.ReadFile(manifestPath)
				if err != nil {
					return fmt.Errorf("%w: reading --manifest: %v", ErrUsage, err)
				}
				if err := json.Unmarshal(b, &m); err != nil {
					return fmt.Errorf("%w: parsing --manifest: %v", ErrUsage, err)
				}
				if name != "" {
					m.Name = name
				}
			}
			if m.Name == "" {
				return fmt.Errorf("%w: --name (or a manifest with a name) is required", ErrUsage)
			}
			if hookURL != "" {
				m.HookURL = hookURL
			}
			opener := openBrowser
			if noBrowser {
				opener = func(u string) error {
					fmt.Fprintf(cmd.ErrOrStderr(), "open this URL in a browser: %s\n", u)
					return nil
				}
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			fmt.Fprintf(cmd.ErrOrStderr(), "waiting for GitHub to hand back the App (up to %s)…\n", timeout)
			app, err := ghapp.Create(ctx, m, ghapp.CreateOpts{
				Store: g.store(), Org: org, Port: port, OpenBrowser: opener,
				WebURL: g.webURL, APIURL: g.apiURL, Force: force,
			})
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "created App %s (id %d)\n", app.Slug, app.ID)
			fmt.Fprintf(out, "  key:  %s (0600)\n", app.PEMPath)
			fmt.Fprintf(out, "  next: ghapp install --app %s   # install it on your account/org, then `ghapp token --repo owner/repo`\n", app.Slug)
			return nil
		},
	}
	c.Flags().StringVar(&name, "name", "", "App name (GitHub derives the slug)")
	c.Flags().StringVar(&manifestPath, "manifest", "", "JSON manifest file (name, url, description, hook_url, public, permissions, events)")
	c.Flags().StringVar(&org, "org", "", "create the App under this organization")
	c.Flags().StringVar(&hookURL, "hook-url", "", "webhook URL (omit for no webhook)")
	c.Flags().IntVar(&port, "port", 8479, "localhost callback port (next ports are tried when busy; 0 = any)")
	c.Flags().BoolVar(&force, "force", false, "replace an App with the same slug in the store")
	c.Flags().BoolVar(&noBrowser, "no-browser", false, "print the URL instead of opening a browser")
	c.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "how long to wait for the callback")
	return c
}
