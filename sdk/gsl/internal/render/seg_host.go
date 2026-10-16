package render

import (
	"context"
	"os"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
)

// HostSegment renders which machine the status line is on:
//
//	<host-glyph> <hostname>
//
// The glyph follows the detected hardware kind (hostkind.go) — 🧠 for a DGX,
// 🍓 for a Raspberry Pi, … — and the hostname is the short name (domain
// stripped). Its width is FLEXIBLE: on a crowded line it is cut to MinLen
// display columns with an ellipsis; when the fit loop finds slack the name
// grows toward its full length (up to MaxLen when set). It is meant to sit at
// the far left of the line so a glance across a fleet of panes answers
// "where am I?" first.
type HostSegment struct {
	// Hostname returns the machine's name. Injected for deterministic tests;
	// nil ⇒ os.Hostname.
	Hostname func() (string, error)
	// Probe gathers the hardware evidence. Injected for tests; nil ⇒ probeHost.
	// It is only consulted when Icon is empty.
	Probe func() hostProbe
	// MinLen is the hostname's floor in display columns (ellipsis included):
	// what a crowded line shows. 0 ⇒ defaultHostMinLen.
	MinLen int
	// MaxLen is a hard ceiling on the hostname's columns, applied even when
	// the line has room. 0 ⇒ no ceiling (the full name).
	MaxLen int
	// Icon, when set, is used verbatim instead of the kind-derived glyph
	// (config option "icon").
	Icon string
	// Kind, when set, replaces detection (config option "kind"). NewHostSegment
	// drops an unknown value so the Deps fallback and detection still apply.
	Kind string
	// Priority is the DROP priority used by the fit loop.
	Priority int
}

// defaultHostMinLen is the hostname floor: long enough for most short names,
// short enough that the segment never crowds the directory next to it.
const defaultHostMinLen = 11

// NewHostSegment builds a HostSegment from the segment's config options
// (min_len, max_len, icon, kind). opts may be nil.
func NewHostSegment(opts map[string]any) *HostSegment {
	s := &HostSegment{
		Icon: optString(opts, "icon", ""),
	}
	if k := optString(opts, "kind", ""); validHostKind(k) {
		s.Kind = k
	}
	if n := optInt(opts, "min_len", 0); n > 0 {
		s.MinLen = n
	}
	if n := optInt(opts, "max_len", 0); n > 0 {
		s.MaxLen = n
	}
	return s
}

// Render implements Segment. It never panics; a failed or empty hostname lookup
// self-omits.
func (s *HostSegment) Render(ctx context.Context, st style.Style, level int) (text, colorKey string, ok bool) {
	d, ok := s.detect(ctx)
	if !ok {
		return "", "", false
	}
	text, colorKey = d.format(st, level)
	return text, colorKey, text != ""
}

// hostGlyph resolves the icon for kind: an explicit override wins, then the
// per-kind icon ("host_<kind>"), then the generic "host" icon. In ascii mode
// the override only applies when it is itself plain ASCII, so one config key
// cannot break the no-emoji guarantee every other glyph honours.
func hostGlyph(st style.Style, kind, override string) string {
	if override != "" && (st.Glyphs != "ascii" || isASCII(override)) {
		return override
	}
	if g := glyph(st, "host_"+kind); g != "" {
		return g
	}
	return glyph(st, "host")
}

// isASCII reports whether s is printable 7-bit ASCII.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			return false
		}
	}
	return true
}

func (s *HostSegment) hostname() (string, error) {
	if s.Hostname != nil {
		return s.Hostname()
	}
	return os.Hostname()
}

func (s *HostSegment) probe() hostProbe {
	if s.Probe != nil {
		return s.Probe()
	}
	return probeHost()
}
