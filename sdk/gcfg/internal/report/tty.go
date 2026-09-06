package report

import (
	"fmt"
	"io"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/engine"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/family"
)

// ANSI colours, kept here rather than in a style package: this is the only
// place gcfg colours anything outside the TUI.
const (
	reset  = "\x1b[0m"
	bold   = "\x1b[1m"
	dim    = "\x1b[2m"
	red    = "\x1b[31m"
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	blue   = "\x1b[34m"
)

// colourFor maps a kind to its colour: drift is what you must act on,
// unmanaged is informational, not-honoured is a warning about GitHub, and
// unreadable is about the credential.
func colourFor(k family.Kind) string {
	switch k {
	case family.Drift:
		return red
	case family.Unmanaged:
		return blue
	case family.NotHonoured:
		return yellow
	default:
		return dim
	}
}

// TTY writes the human-readable report: a headline, then one line per
// finding grouped by family.
func TTY(w io.Writer, r engine.Report, o Options) error {
	paint := func(colour, s string) string {
		if o.NoColor {
			return s
		}
		return colour + s + reset
	}
	if r.Clean() {
		_, err := fmt.Fprintf(w, "%s\n", paint(green, r.Headline()))
		return err
	}
	if _, err := fmt.Fprintf(w, "%s\n", paint(bold, r.Headline())); err != nil {
		return err
	}
	lastFamily := ""
	for _, f := range r.Findings {
		if f.Family != lastFamily {
			if _, err := fmt.Fprintf(w, "\n%s\n", paint(bold, f.Family)); err != nil {
				return err
			}
			lastFamily = f.Family
		}
		line := fmt.Sprintf("  %-10s %s", paint(colourFor(f.Kind), kindName(f.Kind)), f.Key)
		switch f.Kind {
		case family.Drift, family.NotHonoured:
			line += fmt.Sprintf("\n             want %s\n             live %s", redacted(f.Want), redacted(f.Live))
		case family.Unmanaged:
			line += fmt.Sprintf("\n             live %s (not declared)", redacted(f.Live))
		}
		if f.Reason != "" {
			line += "\n             " + paint(dim, redacted(f.Reason))
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}
