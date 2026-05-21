package identity

import "github.com/wenlock/dotfiles/gss/internal/errors"

// ValidatePurpose validates a <purpose> segment. Beyond the shared segment
// grammar, a purpose MUST NOT be a built-in suffix wordlist word — that
// would make `purpose` indistinguishable from a drawn `-suffix` after the
// two are joined (design.md → "Validation grammar").
func ValidatePurpose(s string) error {
	if err := ValidateSegment("purpose", s); err != nil {
		return err
	}
	if isWordlistWord(s) {
		return errors.NewValidationError("purpose", "must not be a suffix wordlist word")
	}
	return nil
}
