package cmd

import (
	"fmt"
	"os"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/schema"
	"github.com/spf13/cobra"
)

func newSchemaCmd(g *Globals) *cobra.Command {
	var out string
	c := &cobra.Command{
		Use:   "schema",
		Short: "Print the JSON Schema for gcfg.yaml (editors use it to complete the file)",
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := schema.JSONSchema()
			if err != nil {
				return err
			}
			if out != "" {
				if err := os.WriteFile(out, b, 0o644); err != nil {
					return fmt.Errorf("gcfg: writing %s: %w", out, err)
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "wrote %s\n", out)
				return nil
			}
			_, err = cmd.OutOrStdout().Write(b)
			return err
		},
	}
	c.Flags().StringVar(&out, "out", "", "write to this path instead of stdout")
	return c
}
