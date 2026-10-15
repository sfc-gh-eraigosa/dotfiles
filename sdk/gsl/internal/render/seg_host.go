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
// stripped) cut to MaxLen display columns with an ellipsis. It is meant to sit
// at the far left of the line so a glance across a fleet of panes answers
// "where am I?" first.
type HostSegment struct {
	// Hostname returns the machine's name. Injected for deterministic tests;
	// nil ⇒ os.Hostname.
	Hostname func() (string, error)
	// Probe gathers the hardware evidence. Injected for tests; nil ⇒ probeHost.
	// It is only consulted when Icon is empty.
	Probe func() hostProbe
	// MaxLen caps the hostname at this many display columns (ellipsis
	// included). 0 ⇒ defaultHostMaxLen.
	MaxLen int
	// Icon, when set, is used verbatim instead of the kind-derived glyph
	// (config option "icon").
	Icon string
	// Kind, when a valid host kind, replaces detection (config option "kind").
	Kind string
	// Priority is the DROP priority used by the fit loop.
	Priority int
}

// defaultHostMaxLen is the hostname column budget: long enough for most short
// names, short enough that the segment never crowds the directory next to it.
const defaultHostMaxLen = 11

// NewHostSegment builds a HostSegment from the segment's config options
// (max_len, icon, kind). opts may be nil.
func NewHostSegment(opts map[string]any) *HostSegment {
	s := &HostSegment{
		Icon: optString(opts, "icon", ""),
		Kind: optString(opts, "kind", ""),
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
// per-kind icon ("host_<kind>"), then the generic "host" icon.
func hostGlyph(st style.Style, kind, override string) string {
	if override != "" {
		return override
	}
	if g := glyph(st, "host_"+kind); g != "" {
		return g
	}
	return glyph(st, "host")
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
