package render

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/gh"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/git"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/mcp"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/observe"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/payload"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
	"github.com/sirupsen/logrus"
)

// segmentTypeName returns the concrete type name of a Segment for log
// records. Pointer receivers are unwrapped so "*seg_ai.Segment" becomes
// "Segment". Best-effort: returns "unknown" when reflection cannot
// resolve a name (e.g. nil interface).
func segmentTypeName(s Segment) string {
	if s == nil {
		return "unknown"
	}
	t := reflect.TypeOf(s)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if name := t.Name(); name != "" {
		return name
	}
	return fmt.Sprintf("%T", s)
}

// segmentDeadline is the per-segment hard cap. Each segment runs in its own
// goroutine under a child context with this deadline; a segment that exceeds
// it is dropped while the rest still render. The detection packages apply
// their own tighter budgets (git ~800 ms, mcp ~500 ms); this is the outer
// safety net so a single misbehaving segment cannot hang the line.
const segmentDeadline = 1000 * time.Millisecond

// Deps bundles everything BuildSegments needs to construct the concrete
// segments. Injecting it (rather than reaching for globals) keeps render
// testable: tests pass fake runners, a fixed clock, a fixture payload, and a
// testdata registry path.
type Deps struct {
	// Payload is the parsed Claude stdin payload (empty struct in CLI mode).
	Payload payload.Payload
	// Cwd is the working directory. When empty, segments fall back to
	// os.Getwd() where appropriate.
	Cwd string
	// Branch is the current git branch (used by the repo segment for PR/name
	// lookup). The caller supplies it (e.g. from git.Status); may be empty.
	Branch string
	// RegistryPath is the gss registry path for repo.PR.
	RegistryPath string

	// Runners (injected seams). Any may be nil; the affected detail is omitted.
	Git git.Runner
	GH  gh.Runner
	MCP mcp.Runner

	// MCPOpts is forwarded to mcp.ActiveCount (cache file / clock).
	MCPOpts mcp.ActiveCountOptions

	// Clock returns the current time for the time segment. nil ⇒ time.Now.
	Clock func() time.Time
}

// BuildSegments constructs the ordered list of ENABLED segments from
// cfg.Segments, preserving config order and skipping disabled entries.
// Unknown segment types are skipped. The returned slice is ready to pass to
// Render.
func BuildSegments(cfg config.Config, deps Deps) []Segment {
	segs := make([]Segment, 0, len(cfg.Segments))
	for _, sc := range cfg.Segments {
		if !sc.Enabled {
			continue
		}
		switch sc.Type {
		case "dirgit":
			segs = append(segs, NewDirGitSegment(deps.Cwd, deps.Git))
		case "repo":
			segs = append(segs, NewRepoSegment(deps.Git, deps.GH, deps.Branch, deps.RegistryPath, sc.Options))
		case "ai":
			segs = append(segs, NewAISegment(deps.Payload, deps.Cwd, deps.MCP, deps.MCPOpts))
		case "time":
			segs = append(segs, NewTimeSegment(deps.Clock, cfg.Timezone, cfg.TimeFormat, cfg.DateFormat))
		default:
			// Unknown segment type: skip silently.
		}
	}
	return segs
}

// Render produces the final status line.
//
//   - Master off (cfg.Enabled == false) ⇒ "" (empty), regardless of segments.
//   - The supplied segs are run CONCURRENTLY, one goroutine each, sharing a
//     parent context derived from ctx. Each goroutine wraps the parent in a
//     per-segment deadline (segmentDeadline) so one slow segment cannot stall
//     the line; a segment that times out, returns ok=false, or panics is
//     dropped and the remaining segments still render.
//   - Surviving segment blocks (raw text + colorKey) are assembled IN INPUT
//     ORDER (the order in segs, which BuildSegments derived from config order)
//     and passed to the color-aware join layer.
//
// Render never hangs (bounded by segmentDeadline per segment, run in parallel)
// and never panics (each segment goroutine recovers).
//
// compactLevel is forwarded to every segment. 0 = full detail. Levels 1–3
// are reserved for PHASE 2 (dynamic width compaction); pass 0 until then.
func Render(ctx context.Context, cfg config.Config, st style.Style, segs []Segment) string {
	return RenderAt(ctx, cfg, st, segs, 0)
}

// RenderAt is like Render but accepts an explicit compactLevel (0 = full
// detail). Callers that implement the PHASE 2 fit loop use this entry point
// to format cached detection results at escalating compaction levels.
func RenderAt(ctx context.Context, cfg config.Config, st style.Style, segs []Segment, compactLevel int) string {
	if !cfg.Enabled {
		return ""
	}
	if len(segs) == 0 {
		return ""
	}

	type result struct {
		text     string
		colorKey string
		ok       bool
	}
	results := make([]result, len(segs))

	var wg sync.WaitGroup
	wg.Add(len(segs))
	for i, seg := range segs {
		go func(idx int, s Segment) {
			defer wg.Done()
			// Recover so a panicking segment is dropped, not fatal.
			defer func() {
				if r := recover(); r != nil {
					observe.Default().WithFields(logrus.Fields{
						"event":   "segment.panic",
						"segment": segmentTypeName(s),
						"panic":   fmt.Sprintf("%v", r),
					}).Warn("segment panicked; dropping")
					results[idx] = result{ok: false}
				}
			}()

			sctx, cancel := context.WithTimeout(ctx, segmentDeadline)
			defer cancel()

			text, colorKey, ok := s.Render(sctx, st, compactLevel)
			if sctx.Err() == context.DeadlineExceeded {
				observe.Default().WithFields(logrus.Fields{
					"event":       "segment.timeout",
					"segment":     segmentTypeName(s),
					"deadline_ms": segmentDeadline.Milliseconds(),
				}).Warn("segment exceeded per-segment deadline; dropping")
				results[idx] = result{ok: false}
				return
			}
			results[idx] = result{text: text, colorKey: colorKey, ok: ok}
		}(i, seg)
	}
	wg.Wait()

	blocks := make([]segmentBlock, 0, len(segs))
	for _, r := range results {
		if r.ok && r.text != "" {
			blocks = append(blocks, segmentBlock{text: r.text, colorKey: r.colorKey})
		}
	}
	if len(blocks) == 0 {
		return ""
	}
	return join(st, blocks)
}
