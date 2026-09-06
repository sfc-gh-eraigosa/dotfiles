package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/engine"
)

// Markdown writes the table that goes into $GITHUB_STEP_SUMMARY: a one-line
// headline with counts, then `| family | key | want | live | kind |`
// (plan §3.5).
func Markdown(w io.Writer, r engine.Report) error {
	if _, err := fmt.Fprintf(w, "### gcfg — %s\n", r.Headline()); err != nil {
		return err
	}
	if r.Clean() {
		_, err := fmt.Fprint(w, "\nNothing to change.\n")
		return err
	}
	if _, err := fmt.Fprint(w, "\n| family | key | want | live | kind |\n| :-- | :-- | :-- | :-- | :-- |\n"); err != nil {
		return err
	}
	for _, f := range r.Findings {
		reason := ""
		if f.Reason != "" {
			reason = " — " + cell(f.Reason)
		}
		if _, err := fmt.Fprintf(w, "| %s | %s | %s | %s | %s%s |\n",
			cell(f.Family), cell(f.Key), cell(redacted(f.Want)), cell(redacted(f.Live)), kindName(f.Kind), reason); err != nil {
			return err
		}
	}
	return nil
}

// cell escapes what would otherwise break a markdown table.
func cell(v any) string {
	s := redacted(v)
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "\n", " ")
	if s == "" {
		return "—"
	}
	return s
}
