package cmd

import (
	"errors"
	"fmt"

	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
	"github.com/spf13/cobra"
)

// errNotSelected is the exit-1 sentinel returned by `selected` when the option
// is not currently selected. main.go maps this to os.Exit(1) silently.
var errNotSelected = errors.New("option not selected")

var selectedCmd = &cobra.Command{
	Use:   "selected <key> <option-id>",
	Short: "Exit 0 if the option is selected, 1 if not, 2 on unknown key or option id",
	Args:  cobra.ExactArgs(2),
	RunE:  runSelected,
}

func init() {
	rootCmd.AddCommand(selectedCmd)
}

func runSelected(_ *cobra.Command, args []string) error {
	key := args[0]
	optID := args[1]

	r, err := newResolver()
	if err != nil {
		return err
	}

	res, err := r.Resolve(key)
	if err != nil {
		return err
	}

	// Must be a choice flag.
	cd := res.Feature.GetChoiceDefault()
	if cd == nil {
		return fmt.Errorf("selected: key %q is not a choice flag: %w", key, resolve.ErrWrongFlagType)
	}

	// Validate that the option id exists in the definition.
	validIDs := make(map[string]bool, len(cd.GetOptions()))
	for _, opt := range cd.GetOptions() {
		validIDs[opt.GetId()] = true
	}
	if !validIDs[optID] {
		return fmt.Errorf("selected: key %q: %w: option id %q not defined", key, resolve.ErrUnknownOption, optID)
	}

	// Check if the option id is in the effective selection.
	choiceVal, ok := res.Value.GetKind().(*gffv1.Value_ChoiceValue)
	if !ok {
		return errNotSelected
	}
	for _, id := range choiceVal.ChoiceValue.GetSelected() {
		if id == optID {
			return nil
		}
	}
	return errNotSelected
}
