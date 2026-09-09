package cmd

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"os"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/schema"
	"github.com/spf13/cobra"
)

// writeFile writes body to path, refusing to clobber unless force.
func writeFile(path string, body []byte, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%w: %s already exists — re-run with --force to replace it", ErrUsage, path)
		}
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

func newExportCmd(g *Globals) *cobra.Command {
	var out string
	var force bool
	var only []string
	c := &cobra.Command{
		Use:   "export",
		Short: "Write the live settings out as a gcfg.yaml",
		Long: `Reads every family and writes what is live to the settings file, so a repo
that already exists can adopt gcfg without hand-writing anything. A family
the credential cannot read is reported and left out, never guessed at.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			target, client, err := g.resolve(cmd.Context())
			if err != nil {
				return err
			}
			file, findings, err := newEngine().Export(cmd.Context(), client, famTarget(target), g.options(only, false))
			if err != nil {
				return engineErr(err)
			}
			body, err := file.Bytes()
			if err != nil {
				return err
			}
			body = append([]byte(header(target.String())), body...)
			for _, f := range findings {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", f)
			}
			if out == "-" {
				_, err := cmd.OutOrStdout().Write(body)
				return err
			}
			path := g.File
			if out != "" {
				path = out
			}
			if err := writeFile(path, body, force); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s (%s)\n", path, target)
			return nil
		},
	}
	c.Flags().StringVar(&out, "out", "", "write here instead of the settings file; - for stdout")
	c.Flags().BoolVar(&force, "force", false, "replace an existing file")
	c.Flags().StringSliceVar(&only, "only", nil, "limit to these families")
	return c
}

// header is the comment every generated file carries, so the next reader
// knows where it came from and how to check it.
func header(target string) string {
	return fmt.Sprintf(`# GitHub settings for %s, managed by gcfg.
#
# Verify:  gcfg verify          Apply: gcfg apply
# Schema:  gcfg schema --out .github/gcfg.schema.json
#
# Only the keys present here are managed; anything absent is left alone
# (ownership: declared). Secrets are declared by name only, never by value.
`, target)
}

func newInitCmd(g *Globals) *cobra.Command {
	var from string
	var force bool
	c := &cobra.Command{
		Use:   "init",
		Short: "Write a starter gcfg.yaml (optionally copied from another repo)",
		RunE: func(cmd *cobra.Command, args []string) error {
			var body []byte
			target, err := g.resolveTarget()
			if err != nil {
				return err
			}
			if from == "" {
				f := schema.Default()
				b, err := f.Bytes()
				if err != nil {
					return err
				}
				body = append([]byte(header(target.String())), b...)
			} else {
				owner, repo, err := parseTarget(from)
				if err != nil {
					return fmt.Errorf("%w: --from wants owner/repo: %v", ErrUsage, err)
				}
				client, _, err := g.client(cmd.Context(), target)
				if err != nil {
					return err
				}
				var content struct {
					Content  string `json:"content"`
					Encoding string `json:"encoding"`
				}
				path := fmt.Sprintf("/repos/%s/%s/contents/.github/gcfg.yaml", owner, repo)
				if _, err := client.Do(cmd.Context(), http.MethodGet, path, nil, &content); err != nil {
					return fmt.Errorf("%w: reading %s/%s: %v", ErrUsage, owner, repo, err)
				}
				decoded, err := base64.StdEncoding.DecodeString(cleanBase64(content.Content))
				if err != nil {
					return fmt.Errorf("%w: %s/%s returned an unreadable file: %v", ErrUsage, owner, repo, err)
				}
				if _, _, err := schema.Parse(decoded, from); err != nil {
					return fmt.Errorf("%w: %s/%s has an invalid gcfg.yaml: %v", ErrUsage, owner, repo, err)
				}
				body = decoded
			}
			if err := writeFile(g.File, body, force); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s — review it, then `gcfg verify`\n", g.File)
			return nil
		},
	}
	c.Flags().StringVar(&from, "from", "", "copy owner/repo's .github/gcfg.yaml as the starting point")
	c.Flags().BoolVar(&force, "force", false, "replace an existing file")
	return c
}

// cleanBase64 drops the line breaks GitHub puts in contents responses.
func cleanBase64(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\n' && s[i] != '\r' {
			out = append(out, s[i])
		}
	}
	return string(out)
}
