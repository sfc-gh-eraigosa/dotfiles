package cmd

import (
	"errors"
	"io"

	"github.com/spf13/cobra"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/shwords"
)

// maxArgvInput is the most stdin hooks argv reads; a longer command is not
// parsed (exit 3), and the hook refuses it.
const maxArgvInput = 1 << 20

// argvFormat is the output format; "lines" is the only one. The hook passes
// it so that a gss too old to have argv fails on the flag (an unknown
// subcommand of hooks prints help and exits 0).
var argvFormat string

var hooksArgvCmd = &cobra.Command{
	Use:   "argv",
	Short: "Print a shell command's simple commands as bash sees them (called by the safety_guard hook)",
	Long: `Reads a bash command on stdin and prints one line per simple command, with
quotes, escapes, backslash-newlines and comments resolved, $'...' decoded,
paths cleaned (a/x/../b is a/b), each redirection glued to its target
(">/x"), and the strings given to bash -c, sh -c, eval, ssh and a here-
document fed to a shell parsed again. A parameter or command substitution
stays a $ marker; the commands inside a substitution get lines of their own.

The safety_guard hook runs its gss and gff checks on these lines as well as on
the command's text, so a spelling the text match cannot follow is still
seen (#474). Exit 2 when the command does not parse, after the lines of the
statements before the error (bash runs those). Exit 3 when the command is too
large to read through (over 1 MiB, or past the work budget): the hook refuses
it rather than judge part of it.

--format=lines (the default, and the only format) is what the hook passes:
a gss without argv rejects the flag instead of printing help.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if argvFormat != "lines" {
			hooksExit(2)
			return nil
		}
		src, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), maxArgvInput+1))
		if err == nil && len(src) > maxArgvInput {
			err = shwords.ErrTooBig // over 1 MiB
		}
		if err == nil {
			err = shwords.Print(cmd.OutOrStdout(), string(src))
		}
		switch {
		case errors.Is(err, shwords.ErrTooBig):
			hooksExit(3)
		case err != nil:
			hooksExit(2)
		}
		return nil
	},
}

func init() {
	hooksArgvCmd.Flags().StringVar(&argvFormat, "format", "lines", "output format (lines)")
	hooksCmd.AddCommand(hooksArgvCmd)
}
