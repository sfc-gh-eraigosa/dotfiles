// Package report renders an engine.Report three ways: for a terminal, for a
// GitHub step summary, and for a machine. All three share one redaction
// rule — a value shaped like a credential is never printed, whatever put it
// in the file (design G8).
package report

import (
	"fmt"
	"regexp"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/family"
)

// Options steer the terminal renderer.
type Options struct {
	NoColor bool
}

// secretish matches values that must never reach output. It mirrors the
// linter's list and errs toward over-matching: this is the last gate before
// a value is printed, and a false positive costs a reader nothing while a
// miss publishes a credential. The alphanumeric runs allow underscores,
// which GitHub's own formats do not use but look-alikes often do.
var secretish = []*regexp.Regexp{
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9_]{16,}`),
	regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`),
	regexp.MustCompile(`(?i)\b(?:secret|password|passwd|api[_-]?key)\s*[:=]\s*\S{8,}`),
}

// redacted renders a value for display, replacing anything that looks like
// a credential.
func redacted(v any) string {
	if v == nil {
		return ""
	}
	s := fmt.Sprintf("%v", v)
	for _, re := range secretish {
		s = re.ReplaceAllString(s, "[REDACTED]")
	}
	return s
}

// kindName is the stable name of a finding kind, used in JSON and markdown.
func kindName(k family.Kind) string { return k.String() }
