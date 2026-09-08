package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
)

func pemMode(path string) string {
	st, err := os.Stat(path)
	if err != nil {
		return "MISSING"
	}
	m := st.Mode().Perm()
	if m&0o077 != 0 {
		return fmt.Sprintf("%04o (WRONG: must be 0600)", m)
	}
	return fmt.Sprintf("%04o ok", m)
}

func newStatusCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show stored Apps, key file modes, and recorded installations (no secrets)",
		RunE: func(cmd *cobra.Command, args []string) error {
			apps, err := g.store().Load()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(apps) == 0 {
				fmt.Fprintf(out, "no GitHub App stored in %s — run `ghapp create`\n", g.configDir)
				return fmt.Errorf("%w: nothing to show", ErrUsage)
			}
			if g.app != "" {
				if _, ok := apps[g.app]; !ok {
					return fmt.Errorf("%w: unknown app %q", ErrUsage, g.app)
				}
			}
			slugs := make([]string, 0, len(apps))
			for s := range apps {
				if g.app == "" || g.app == s {
					slugs = append(slugs, s)
				}
			}
			sort.Strings(slugs)
			fmt.Fprintf(out, "store: %s\n", g.configDir)
			for _, s := range slugs {
				a := apps[s]
				fmt.Fprintf(out, "%s (id %d)\n  key: %s  %s\n", a.Slug, a.ID, a.PEMPath, pemMode(a.PEMPath))
				if len(a.Installs) == 0 {
					fmt.Fprintln(out, "  installations: none recorded (run `ghapp install`)")
					continue
				}
				accounts := make([]string, 0, len(a.Installs))
				for acc := range a.Installs {
					accounts = append(accounts, acc)
				}
				sort.Strings(accounts)
				fmt.Fprintln(out, "  installations:")
				for _, acc := range accounts {
					fmt.Fprintf(out, "    %-24s id %d\n", acc, a.Installs[acc])
				}
			}
			return nil
		},
	}
}
