package cmd

import (
	"fmt"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/gh"
	"github.com/spf13/cobra"
)

func newAuthCmd(g *Globals) *cobra.Command {
	c := &cobra.Command{
		Use:   "auth",
		Short: "Credentials: which one gcfg would use, and what it can do",
		RunE:  func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	c.AddCommand(newAuthStatusCmd(g))
	return c
}

func newAuthStatusCmd(g *Globals) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the credential gcfg would use and where it comes from",
		Long: `Walks the credential chain (GH_TOKEN, GITHUB_TOKEN, gh login, GitHub App)
and reports which source answers first, for which target. The token value is
never printed — only its source and a redacted fingerprint.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := g.resolveTarget()
			if err != nil {
				return err
			}
			_, src, err := g.client(cmd.Context(), target)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "target: %s\n", target)
			fmt.Fprintf(out, "source: %s\n", src)
			fmt.Fprintf(out, "token:  *** (never printed; %s supplies it)\n", src)
			if src == gh.SourceApp {
				fmt.Fprintln(out, "note:   installation tokens expire in an hour; gcfg mints a fresh one per run")
			}
			return nil
		},
	}
}
