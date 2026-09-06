package cmd

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/ghapp/pkg/ghapp"
	"github.com/spf13/cobra"
)

// check prints one doctor line; every check runs so the whole picture shows.
type checker struct {
	cmd    *cobra.Command
	failed bool
}

func (c *checker) ok(name, detail string) {
	fmt.Fprintf(c.cmd.OutOrStdout(), "ok    %-14s %s\n", name, detail)
}

func (c *checker) fail(name string, err error) {
	c.failed = true
	fmt.Fprintf(c.cmd.OutOrStdout(), "FAIL  %-14s %v\n", name, err)
}

func newDoctorCmd(g *globals) *cobra.Command {
	var repo string
	c := &cobra.Command{
		Use:   "doctor",
		Short: "Check the store, key, App JWT, installations, and (with --repo) a real token",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, apps, err := g.selectApp()
			if err != nil {
				return err
			}
			ck := &checker{cmd: cmd}
			if st, err := os.Stat(g.configDir); err != nil {
				ck.fail("store", err)
			} else if m := st.Mode().Perm(); m&0o077 != 0 {
				ck.fail("store", fmt.Errorf("%s is %04o; must be 0700", g.configDir, m))
			} else {
				ck.ok("store", fmt.Sprintf("%s %04o", g.configDir, m))
			}
			if mode := pemMode(app.PEMPath); strings.HasSuffix(mode, "ok") {
				ck.ok("pem", fmt.Sprintf("%s %s", app.PEMPath, mode))
			} else {
				ck.fail("pem", fmt.Errorf("%s %s", app.PEMPath, mode))
			}
			info, err := app.Info(cmd.Context())
			if err != nil {
				ck.fail("jwt", err)
			} else {
				ck.ok("jwt", fmt.Sprintf("App %s (id %d) %s", info.Slug, info.ID, info.HTMLURL))
			}
			insts, err := recordInstallations(cmd, g, app, apps)
			if err != nil {
				ck.fail("installations", err)
			} else {
				accs := make([]string, 0, len(insts))
				for _, i := range insts {
					accs = append(accs, fmt.Sprintf("%s=%d", i.Account, i.ID))
				}
				sort.Strings(accs)
				ck.ok("installations", fmt.Sprintf("%d (%s)", len(insts), strings.Join(accs, ", ")))
			}
			if repo != "" {
				owner, name, err := splitRepo(repo)
				if err != nil {
					return err
				}
				if inst, err := installationFor(cmd, g, app, apps, owner); err != nil {
					ck.fail("token", err)
				} else if tok, err := app.Token(cmd.Context(), inst, ghapp.TokenScope{Repositories: []string{name}}); err != nil {
					ck.fail("token", err)
				} else {
					ck.ok("token", fmt.Sprintf("minted for installation %d, %s", inst, tok))
					if perms, err := ghapp.RepoAccess(cmd.Context(), nil, g.apiURL, tok, repo); err != nil {
						ck.fail("repo", err)
					} else {
						ps := make([]string, 0, len(perms))
						for k, v := range perms {
							if v {
								ps = append(ps, k)
							}
						}
						sort.Strings(ps)
						ck.ok("repo", fmt.Sprintf("%s reachable; permissions: %s", repo, strings.Join(ps, ",")))
					}
				}
			}
			if ck.failed {
				return errors.New("ghapp doctor: one or more checks failed")
			}
			return nil
		},
	}
	c.Flags().StringVar(&repo, "repo", "", "also mint a token for owner/repo and probe it")
	return c
}
