package render

import (
	"context"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
)

// Snapshot is every known segment detected ONCE, so the config studio can
// re-fit any selection, order, style or width on each keystroke with no
// further I/O (the detect/format split exists for exactly this).
type Snapshot struct {
	byType map[string]segmentData
	deps   Deps
	st     style.Style
}

// DetectAll detects every segment type — the ones cfg lists, in its order,
// plus any known type it lacks — with all of them enabled.
func DetectAll(ctx context.Context, cfg config.Config, st style.Style, deps Deps) Snapshot {
	all := cfg
	all.Enabled = true
	all.Segments = nil
	seen := map[string]bool{}
	for _, s := range cfg.Segments {
		s.Enabled = true
		all.Segments = append(all.Segments, s)
		seen[s.Type] = true
	}
	for _, t := range config.KnownSegmentTypes {
		if !seen[t] {
			all.Segments = append(all.Segments, config.Segment{Type: t, Enabled: true})
		}
	}
	// Build each type on its own so every Segment is known by its type
	// (BuildSegments skips disabled/section-off/unknown ones), then detect
	// them all at once — concurrently, as a render does.
	var segs []Segment
	var types []string
	for _, sc := range all.Segments {
		one := all
		one.Segments = []config.Segment{sc}
		for _, seg := range BuildSegments(one, deps) {
			segs = append(segs, seg)
			types = append(types, sc.Type)
		}
	}
	datas := Detect(ctx, all, st, segs)
	snap := Snapshot{byType: map[string]segmentData{}, deps: deps, st: st}
	for i, t := range types {
		if i < len(datas) && datas[i] != nil {
			snap.byType[t] = datas[i]
		}
	}
	return snap
}

// Line is the status line cfg would render at cols: its enabled segments, in
// its order, fitted and painted with st. "" when the master switch is off.
func (s Snapshot) Line(cfg config.Config, st style.Style, cols int) string {
	if !cfg.Enabled {
		return ""
	}
	var datas []segmentData
	for _, sc := range cfg.Segments {
		if !sc.Enabled {
			continue
		}
		if sc.Type == "time" { // its timezone and formats are config: re-detect (no I/O)
			if d := s.redetect(cfg, sc); d != nil {
				datas = append(datas, d)
			}
			continue
		}
		if d, ok := s.byType[sc.Type]; ok {
			datas = append(datas, d)
		}
	}
	return Fit(datas, st, cols)
}

// redetect detects one segment against cfg with the snapshot's deps.
func (s Snapshot) redetect(cfg config.Config, sc config.Segment) segmentData {
	one := cfg
	one.Segments = []config.Segment{sc}
	segs := BuildSegments(one, s.deps)
	if len(segs) == 0 {
		return nil
	}
	if datas := Detect(context.Background(), one, s.st, segs); len(datas) == 1 {
		return datas[0]
	}
	return nil
}
