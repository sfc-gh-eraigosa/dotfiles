package render

// seg_host_data.go — hostData segmentData + HostSegment.detect().
//
// The host segment has no compaction ladder: its text is already capped at
// MaxLen columns at level 0, so every level renders the same width (format is
// trivially non-increasing). When the line still does not fit, the fit loop
// drops the whole segment by priority instead.

import (
	"context"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
)

// hostData is the detect-once intermediate for the host segment.
type hostData struct {
	// name is the short hostname, already cut to the column budget.
	name string
	// kind is the detected (or overridden) host kind.
	kind string
	// icon is the explicit glyph override ("" ⇒ derive from kind).
	icon string
	// prio is the drop priority (config.Segment.EffectivePriority).
	prio int
}

// priority implements prioritized.
func (d *hostData) priority() int {
	if d.prio != 0 {
		return d.prio
	}
	return config.PriorityHost
}

// detect implements detectable for HostSegment: one hostname lookup plus the
// file probe (skipped when the icon is pinned). No subprocesses.
func (s *HostSegment) detect(_ context.Context) (segmentData, bool) {
	raw, err := s.hostname()
	if err != nil {
		return nil, false
	}
	name := shortHostname(raw)
	if name == "" {
		return nil, false
	}
	maxLen := s.MaxLen
	if maxLen <= 0 {
		maxLen = defaultHostMaxLen
	}

	kind := s.Kind
	if !validHostKind(kind) {
		kind = ""
	}
	if kind == "" && s.Icon == "" {
		kind = classifyHost(s.probe())
	}

	return &hostData{
		name: truncateText(name, maxLen),
		kind: kind,
		icon: s.Icon,
		prio: s.Priority,
	}, true
}

// shortHostname strips the domain ("spark1.local" → "spark1") and surrounding
// whitespace.
func shortHostname(h string) string {
	h = strings.TrimSpace(h)
	if i := strings.IndexByte(h, '.'); i >= 0 {
		h = h[:i]
	}
	return h
}

// format implements segmentData.format. Pure; identical at every level.
func (d *hostData) format(st style.Style, _ int) (text, colorKey string) {
	var b strings.Builder
	if g := hostGlyph(st, d.kind, d.icon); g != "" {
		b.WriteString(g)
		b.WriteString(" ")
	}
	b.WriteString(d.name)
	return b.String(), "host"
}
