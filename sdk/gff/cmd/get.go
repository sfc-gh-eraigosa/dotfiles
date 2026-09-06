package cmd

import (
	"fmt"
	"strings"

	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Print the effective value of a feature flag",
	Args:  cobra.ExactArgs(1),
	RunE:  runGet,
}

func init() {
	rootCmd.AddCommand(getCmd)
}

func runGet(cmd *cobra.Command, args []string) error {
	r, err := newResolver()
	if err != nil {
		return err
	}

	res, err := r.Resolve(args[0])
	if err != nil {
		return err
	}

	var value string
	switch v := res.Value.GetKind().(type) {
	case *gffv1.Value_BoolValue:
		value = fmt.Sprintf("%v", v.BoolValue)
	case *gffv1.Value_ChoiceValue:
		value = strings.Join(v.ChoiceValue.GetSelected(), ",")
	default:
		value = ""
	}

	fmt.Fprintln(cmd.OutOrStdout(), value)
	return nil
}
