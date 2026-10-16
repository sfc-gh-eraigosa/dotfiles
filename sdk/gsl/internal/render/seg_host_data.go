package render

// seg_host_data.go — hostData segmentData + HostSegment.detect().
//
// The host segment has no compaction ladder; it is FLEXIBLE instead. Every
// level renders the hostname at its floor (MinLen columns, ellipsized), which
// is what the goldens and the plain Render path show. Once the fit loop finds
// a level where the whole line fits, it hands the leftover columns back
// (flexible.withSlack) and the name grows toward its full length — capped by
// MaxLen when one is set. When even the floor does not fit, the fit loop drops
// the whole segment by priority.

import (
	"context"
	"net"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
)

// hostData is the detect-once intermediate for the host segment.
type hostData struct {
	// name is the full short hostname (domain stripped, not yet cut).
	name string
	// floor is the column budget with no slack (MinLen).
	floor int
	// cap is the hard column ceiling, 0 = none (MaxLen).
	cap int
	// slack is the extra width the fit loop granted beyond floor.
	slack int
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
	floor := s.MinLen
	if floor <= 0 {
		floor = defaultHostMinLen
	}

	kind := s.Kind
	if !validHostKind(kind) {
		kind = ""
	}
	if kind == "" && s.Icon == "" {
		kind = classifyHost(s.probe())
	}

	return &hostData{
		name:  name,
		floor: floor,
		cap:   s.MaxLen,
		kind:  kind,
		icon:  s.Icon,
		prio:  s.Priority,
	}, true
}

// shortHostname strips the domain ("spark1.local" → "spark1") and surrounding
// whitespace. An IP literal (minimal containers and cloud images sometimes
// report one) is kept whole: "10.0.0.5" must not become "10".
func shortHostname(h string) string {
	h = strings.TrimSpace(h)
	if net.ParseIP(h) != nil {
		return h
	}
	if i := strings.IndexByte(h, '.'); i >= 0 {
		h = h[:i]
	}
	return h
}

// budget is the hostname column budget: floor plus any granted slack, never
// above cap when one is set.
func (d *hostData) budget() int {
	n := d.floor + d.slack
	if d.cap > 0 && n > d.cap {
		n = d.cap
	}
	return n
}

// withSlack implements flexible: a copy allowed extra more columns.
func (d *hostData) withSlack(extra int) segmentData {
	c := *d
	c.slack = extra
	return &c
}

// format implements segmentData.format. Pure; identical at every level — the
// width varies with the granted slack, not the compaction level.
func (d *hostData) format(st style.Style, _ int) (text, colorKey string) {
	var b strings.Builder
	if g := hostGlyph(st, d.kind, d.icon); g != "" {
		b.WriteString(g)
		b.WriteString(" ")
	}
	b.WriteString(truncateText(d.name, d.budget()))
	return b.String(), "host"
}
