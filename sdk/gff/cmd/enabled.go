package cmd

import (
	"errors"
	"fmt"

	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
	"github.com/spf13/cobra"
)

// errOff is the exit-1 sentinel returned by `enabled` when the flag is off.
// main.go maps this to os.Exit(1) silently (no stderr output).
var errOff = errors.New("flag is off")

// IsExit1Silent returns true when err is one of the silent exit-1 sentinels
// (flag off / not selected). main.go uses this to distinguish from other errors.
func IsExit1Silent(err error) bool {
	return errors.Is(err, errOff) || errors.Is(err, errNotSelected)
}

var enabledCmd = &cobra.Command{
	Use:   "enabled <key>",
	Short: "Exit 0 if the flag is on, 1 if off, 2 on type/key error",
	Args:  cobra.ExactArgs(1),
	RunE:  runEnabled,
}

func init() {
	rootCmd.AddCommand(enabledCmd)
}

func runEnabled(cmd *cobra.Command, args []string) error {
	r, err := newResolver()
	if err != nil {
		return err
	}

	res, err := r.Resolve(args[0])
	if err != nil {
		return err
	}

	// Choice keys cannot be queried with `enabled`.
	if res.Feature.GetChoiceDefault() != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "enabled: key %q is a choice flag; use `get` or `selected`\n", args[0])
		return resolve.ErrWrongFlagType
	}

	boolVal, ok := res.Value.GetKind().(*gffv1.Value_BoolValue)
	if !ok || !boolVal.BoolValue {
		return errOff
	}
	return nil
}
