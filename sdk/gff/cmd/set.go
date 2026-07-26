package cmd

import (
	"fmt"
	"strings"

	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/overrides"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
	"github.com/spf13/cobra"
)

var setCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a feature flag in the user override file (~/.config/gff/config.yaml)",
	Long: `Set writes the user override file with mode 0600.

For bool flags, value must be "true" or "false".
For choice flags, value is a single option id or a comma-separated list of ids
(CHOICE_MODE_SINGLE accepts exactly one; CHOICE_MODE_MULTI accepts any subset).`,
	Args: cobra.ExactArgs(2),
	RunE: runSet,
}

func init() {
	rootCmd.AddCommand(setCmd)
}

func runSet(_ *cobra.Command, args []string) error {
	key := args[0]
	rawVal := args[1]

	r, err := newResolver()
	if err != nil {
		return err
	}

	// Validate the key exists.
	res, err := r.Resolve(key)
	if err != nil {
		return err
	}

	// Parse the value based on flag type.
	var v *gffv1.Value

	switch res.Feature.Default.(type) {
	case *gffv1.Feature_BoolDefault:
		switch rawVal {
		case "true":
			v = &gffv1.Value{Kind: &gffv1.Value_BoolValue{BoolValue: true}}
		case "false":
			v = &gffv1.Value{Kind: &gffv1.Value_BoolValue{BoolValue: false}}
		default:
			return fmt.Errorf("set: key %q is a bool flag; value must be \"true\" or \"false\", got %q", key, rawVal)
		}

	case *gffv1.Feature_ChoiceDefault:
		ids := strings.Split(rawVal, ",")
		for i := range ids {
			ids[i] = strings.TrimSpace(ids[i])
		}
		v = &gffv1.Value{Kind: &gffv1.Value_ChoiceValue{
			ChoiceValue: &gffv1.ChoiceSelection{Selected: ids},
		}}

	default:
		return fmt.Errorf("set: key %q has unknown flag type", key)
	}

	// Validate the value via the resolver (checks option ids and single-mode arity)
	// by building a temporary Resolver with a fake user override.
	// The cleanest approach: use the resolver's internal validation by calling
	// resolve.ValidateValue directly — but since that's not exported, we rely on
	// the fact that the resolver will reject bad overrides when we load them.
	// Instead, we validate using the exported path: build a fresh resolver that
	// has the candidate value in its user override and call Resolve on it.
	// This avoids duplicating the choice-validation logic.
	if err := validateValueForKey(r, res, key, v); err != nil {
		return err
	}

	return overrides.Write(r.P, key, v)
}

// validateValueForKey validates that v is a legal override for key by
// delegating to the resolver's choice-validation logic via a scratch override.
func validateValueForKey(r *resolve.Resolver, res resolve.Resolved, key string, v *gffv1.Value) error {
	cd := res.Feature.GetChoiceDefault()
	if cd == nil {
		// Bool: nothing further to validate (already parsed as true/false above).
		return nil
	}

	// Build valid option id set.
	validIDs := make(map[string]bool, len(cd.GetOptions()))
	for _, opt := range cd.GetOptions() {
		validIDs[opt.GetId()] = true
	}

	choiceVal := v.GetChoiceValue()
	if choiceVal == nil {
		return fmt.Errorf("set: key %q: expected choice value", key)
	}

	// Validate each id.
	for _, id := range choiceVal.GetSelected() {
		if !validIDs[id] {
			return fmt.Errorf("set: key %q: %w: option id %q not defined", key, resolve.ErrUnknownOption, id)
		}
	}

	// CHOICE_MODE_SINGLE: at most one id. Arity violation is a value error
	// (exit 1 per plan §7.2 IA-2), NOT a definition error — no exit-2 sentinel.
	if cd.GetMode() == gffv1.ChoiceMode_CHOICE_MODE_SINGLE && len(choiceVal.GetSelected()) > 1 {
		return fmt.Errorf("set: key %q: CHOICE_MODE_SINGLE requires exactly one id, got %v",
			key, choiceVal.GetSelected())
	}

	return nil
}
