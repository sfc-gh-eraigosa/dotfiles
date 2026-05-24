package identity

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/wenlock/dotfiles/gss/internal/errors"
)

// segmentRe is the shared grammar for the feature / user / purpose
// segments (design.md → "Validation grammar"): 2–32 lowercase ASCII,
// starts with a letter, ends alphanumeric, hyphens allowed mid-string.
var segmentRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}[a-z0-9]$`)

// maxDescriptionRunes bounds --description at 240 code points.
const maxDescriptionRunes = 240

// ansiRe matches an ANSI CSI escape sequence (ESC [ … final-byte).
var ansiRe = regexp.MustCompile("\x1b\\[[\x00-\x3f]*[\x40-\x7e]")

// markerRe matches a reserved gss marker token, e.g. <!-- gss:stack -->.
// Such tokens are stripped from descriptions before persistence so user
// content can't forge the markers gss uses in rendered PR bodies.
var markerRe = regexp.MustCompile(`<!--\s*gss:[^>]*-->`)

// ValidateSegment enforces segmentRe for a named identifier segment,
// returning a *errors.ValidationError (which wraps errors.ErrInvalidIdent)
// on failure so every reject path is errors.Is-recognisable.
func ValidateSegment(kind, s string) error {
	if !segmentRe.MatchString(s) {
		return errors.NewValidationError(kind, "must match ^[a-z][a-z0-9-]{0,30}[a-z0-9]$ (2–32 lowercase ASCII, hyphens mid-string)")
	}
	return nil
}

// ValidateFeature validates a <feature> segment.
func ValidateFeature(s string) error { return ValidateSegment("feature", s) }

// ValidateUser validates a <user> segment.
func ValidateUser(s string) error { return ValidateSegment("user", s) }

// ValidateDescription cleans and validates a --description value
// (design.md → "Validation grammar"): NFC-normalise, strip ANSI escapes,
// gss marker tokens, and newlines/tabs, reject any other control
// character, and require 1–240 code points. It returns the cleaned string
// to persist.
func ValidateDescription(s string) (string, error) {
	s = norm.NFC.String(s)
	s = ansiRe.ReplaceAllString(s, "")
	s = markerRe.ReplaceAllString(s, "")
	// Strip the explicitly-listed whitespace controls (newlines/tabs/CR).
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return -1
		}
		return r
	}, s)
	// Any remaining control character (other than space) is a reject.
	for _, r := range s {
		if r != ' ' && unicode.IsControl(r) {
			return "", errors.NewValidationError("description", "contains a disallowed control character")
		}
	}
	n := utf8.RuneCountInString(s)
	if n < 1 || n > maxDescriptionRunes {
		return "", errors.NewValidationError("description", fmt.Sprintf("must be 1..%d code points, got %d", maxDescriptionRunes, n))
	}
	return s, nil
}
