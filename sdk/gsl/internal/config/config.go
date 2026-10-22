// Package config manages the gsl configuration file.
//
// Config file location: ${XDG_CONFIG_HOME:-~/.config}/gsl/config.json
//
// Default() returns a fully working configuration with all 5 segments
// enabled. Load() for a missing file returns Default() with no error.
// Save() creates parent directories as needed.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// Segment represents a single status-line segment.
type Segment struct {
	// Type is the segment kind: "host", "dirgit", "repo", "ai", "video", or "time"
	// (KnownSegmentTypes).
	Type string `json:"type"`
	// Enabled controls whether this segment is rendered.
	Enabled bool `json:"enabled"`
	// Priority is the DROP priority used by the render fit loop when text
	// compaction alone cannot make the line fit: the LOWEST priority segment is
	// dropped first, and the highest-priority segment is the last one standing.
	//
	// It is deliberately independent of the segment's POSITION in Segments,
	// which controls only the left-to-right VISUAL order. Before this field
	// existed the fit loop dropped the right-most slice element, so simply
	// reordering the line silently changed which information survived a narrow
	// terminal — the user's cosmetic preference decided what they could still
	// read at 40 columns.
	//
	// 0 (the zero value, i.e. "unset") means "use the built-in default for this
	// segment type" — see DefaultSegmentPriority. That keeps every existing
	// config.json working unchanged.
	Priority int `json:"priority,omitempty"`
	// Options holds segment-specific key-value overrides.
	Options map[string]any `json:"options,omitempty"`
}

// Built-in drop priorities. Higher survives longer; the fit loop drops the
// lowest first. The ordering encodes "what does the user most need to still see
// on a 40-column split pane": which repo/PR you are in beats where you are on
// disk, beats the model, beats the clock (which every OS already shows).
//
// The gaps leave room for a user to interleave a custom value without having to
// renumber the others.
const (
	PriorityVideo = 5  // dropped first: GPU load is nice-to-have
	PriorityTime  = 10 // dropped right after video
	PriorityHost  = 15 // which machine — outlives the clock, not the model
	PriorityAI    = 20 // the legacy all-in-one ai segment (migrates to the four below)
	// The ai segment's pieces (#426): all between host and dirgit, as ai was,
	// shedding the least important first.
	PriorityLimits  = 17
	PriorityMCP     = 18
	PriorityContext = 19
	PriorityModel   = 20
	PriorityDirGit  = 30
	PriorityRepo    = 40 // dropped last
)

// DefaultSegmentPriority returns the built-in drop priority for a segment type.
// Unknown types sort with the first-dropped group.
func DefaultSegmentPriority(segType string) int {
	switch segType {
	case "repo":
		return PriorityRepo
	case "dirgit":
		return PriorityDirGit
	case "ai":
		return PriorityAI
	case "model":
		return PriorityModel
	case "context":
		return PriorityContext
	case "mcp":
		return PriorityMCP
	case "limits":
		return PriorityLimits
	case "time":
		return PriorityTime
	case "video":
		return PriorityVideo
	case "host":
		return PriorityHost
	default:
		return 0
	}
}

// KnownSegmentTypes lists every segment type gsl can render, in the default
// left-to-right order. EnableSegment uses it to add a type that an older
// config.json predates.
var KnownSegmentTypes = []string{"host", "dirgit", "repo", "model", "context", "mcp", "limits", "video", "time"}

// AIParts are the segments the old all-in-one "ai" segment became (#426), in
// its order. "ai" is still accepted: Load migrates it, and the segment verbs
// read it as all four.
var AIParts = []string{"model", "context", "mcp", "limits"}

// EffectivePriority returns the segment's drop priority: the user's explicit
// Priority when set, otherwise the built-in default for its type.
func (s Segment) EffectivePriority() int {
	if s.Priority != 0 {
		return s.Priority
	}
	return DefaultSegmentPriority(s.Type)
}

// Config is the top-level gsl configuration.
type Config struct {
	// Enabled is the master on/off switch for the entire status line.
	Enabled bool `json:"enabled"`
	// Segments is the ordered list of segments to render.
	Segments []Segment `json:"segments"`
	// Timezone is the IANA timezone used for the time segment.
	Timezone string `json:"timezone"`
	// TimeFormat is the Go time layout string for the clock display.
	TimeFormat string `json:"time_format"`
	// DateFormat is the Go time layout string for the date display.
	DateFormat string `json:"date_format"`
	// Style selects the powerline/separator style ("powerline", "plain").
	Style string `json:"style"`
	// Links selects the hyperlink affordance: "underline" (default; OSC 8 +
	// SGR underline on exactly the linked text), "plain" (OSC 8 only), or
	// "off" (no links at all). Which fields are linked is governed by the
	// gsl.links.* gff flags, not by this key. See EffectiveLinks.
	Links string `json:"links,omitempty"`
	// Palette pins the colour palette ("dark", "light", "dark8",
	// "dark-daltonism"); "" = auto (the host tool's theme, then the terminal).
	// GSL_THEME / SDK_THEME in the environment still win for a session.
	Palette string `json:"palette,omitempty"`
	// Styles holds user-defined style overrides keyed by segment type or
	// element name. Kept as map[string]any for CP1; CP2 will type it further.
	Styles map[string]any `json:"styles,omitempty"`
	// FallbackColumns is the terminal width assumed when NO source knows it —
	// no terminal_width in the payload, neither stdout nor stderr is a tty, and
	// COLUMNS is not exported. See cmd.resolveColumns.
	//
	// DefaultFallbackColumns (120), not 80. 80 is a teletype default that
	// under-serves every modern terminal; when guessing, guess generously — an
	// over-wide guess is corrected by the fit loop's truncation (now a hard
	// guarantee), whereas an under-wide guess silently discards information the
	// user had room for.
	FallbackColumns int `json:"fallback_columns,omitempty"`
}

// DefaultFallbackColumns is the assumed terminal width when nothing knows it.
const DefaultFallbackColumns = 120

// Link affordance modes for Config.Links.
const (
	LinksUnderline = "underline"
	LinksPlain     = "plain"
	LinksOff       = "off"
)

// EffectiveLinks normalizes Links: "" or an unknown value means "underline".
func (c Config) EffectiveLinks() string {
	switch c.Links {
	case LinksPlain, LinksOff:
		return c.Links
	}
	return LinksUnderline
}

// Default returns a fully usable Config with all 6 segments enabled in the
// canonical order: host, dirgit, repo, ai, video, time.
func Default() Config {
	return Config{
		Enabled: true,
		Segments: []Segment{
			{Type: "host", Enabled: true},
			{Type: "dirgit", Enabled: true},
			{Type: "repo", Enabled: true},
			{Type: "model", Enabled: true},
			{Type: "context", Enabled: true},
			{Type: "mcp", Enabled: true},
			{Type: "limits", Enabled: true},
			{Type: "video", Enabled: true},
			{Type: "time", Enabled: true},
		},
		Timezone:        "America/Los_Angeles",
		TimeFormat:      "15:04:05",
		DateFormat:      "2006-01-02",
		Style:           "powerline",
		FallbackColumns: DefaultFallbackColumns,
	}
}

// EffectiveFallbackColumns returns the configured fallback width, or
// DefaultFallbackColumns when it is unset or nonsensical.
func (c Config) EffectiveFallbackColumns() int {
	if c.FallbackColumns > 0 {
		return c.FallbackColumns
	}
	return DefaultFallbackColumns
}

// DefaultPath returns the default config file path, respecting
// XDG_CONFIG_HOME when set.
func DefaultPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(base, "gsl", "config.json")
}

// Load reads the config from path. If path does not exist, Load returns
// Default() with a nil error — missing config is not an error. Any other
// file system error or JSON parse error is returned as a non-nil error.
//
// The file contents are unmarshaled over a Default() base, so a partial
// hand-edited config (e.g. {"enabled": true}) keeps the defaults for fields it
// does not specify while any field it does specify — including explicit
// false/empty values — overrides the default.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Default(), nil
		}
		return Config{}, fmt.Errorf("config: read %s: %w", path, err)
	}
	c := Default()
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("config: parse %s: %w", path, err)
	}
	c.migrateAI()
	c.migrateVideo()
	return c, nil
}

// migrateAI replaces an "ai" entry with its four parts, in its place (#426):
// the same on/off state and explicit priority; model_url goes to model,
// usage_url to context and limits (the fields that linked to it).
func (c *Config) migrateAI() {
	i := slices.IndexFunc(c.Segments, func(s Segment) bool { return s.Type == "ai" })
	if i < 0 {
		return
	}
	ai := c.Segments[i]
	parts := make([]Segment, 0, len(AIParts))
	for _, t := range AIParts {
		if c.findSegment(t) != nil {
			continue // already split (hand-edited): keep that one
		}
		p := Segment{Type: t, Enabled: ai.Enabled, Priority: ai.Priority}
		pick := func(keys ...string) {
			for _, k := range keys {
				if v, ok := ai.Options[k]; ok {
					if p.Options == nil {
						p.Options = map[string]any{}
					}
					p.Options[k] = v
				}
			}
		}
		switch t {
		case "model":
			pick("model_url")
		case "context", "limits":
			pick("usage_url")
		}
		parts = append(parts, p)
	}
	c.Segments = slices.Concat(c.Segments[:i:i], parts, c.Segments[i+1:])
}

// migrateVideo is a one-time migration for the video segment, which postdates
// saved configs: a loaded segments array replaces the defaults, so without this
// no existing user would ever see it. When the array has no video entry at all
// it is inserted ENABLED immediately before time (or at the end). A user who
// turned it off has an explicit disabled entry, which is left alone. Other
// missing segment types keep the ensureSegment behaviour (added disabled).
func (c *Config) migrateVideo() {
	if c.findSegment("video") != nil {
		return
	}
	at := len(c.Segments)
	for i, seg := range c.Segments {
		if seg.Type == "time" {
			at = i
			break
		}
	}
	c.Segments = slices.Insert(c.Segments, at, Segment{Type: "video", Enabled: true})
}

// Save writes c to path as indented JSON, creating any missing parent
// directories. It overwrites an existing file.
func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("config: create dir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("config: write %s: %w", path, err)
	}
	return nil
}

// EnableMaster sets the master Enabled flag to true.
func (c *Config) EnableMaster() {
	c.Enabled = true
}

// DisableMaster sets the master Enabled flag to false.
func (c *Config) DisableMaster() {
	c.Enabled = false
}

// findSegment returns a pointer to the first segment with the given type, or
// nil if not found.
func (c *Config) findSegment(segType string) *Segment {
	for i := range c.Segments {
		if c.Segments[i].Type == segType {
			return &c.Segments[i]
		}
	}
	return nil
}

// ensureSegment returns the segment entry for segType, inserting one when a
// KNOWN type is missing from Segments: a config.json saved before the type
// existed (Load replaces the whole array). The inserted entry starts disabled —
// the verb that asked for it decides its state — and goes where the canonical
// order (KnownSegmentTypes) puts it: before the first existing segment that
// canonically follows it, else at the end. Returns an error only for a type
// gsl does not render.
func (c *Config) ensureSegment(segType string) (*Segment, error) {
	if seg := c.findSegment(segType); seg != nil {
		return seg, nil
	}
	rank := slices.Index(KnownSegmentTypes, segType)
	if rank < 0 {
		return nil, fmt.Errorf("config: unknown segment type %q", segType)
	}
	at := len(c.Segments)
	for i, seg := range c.Segments {
		if r := slices.Index(KnownSegmentTypes, seg.Type); r > rank {
			at = i
			break
		}
	}
	c.Segments = slices.Insert(c.Segments, at, Segment{Type: segType})
	return &c.Segments[at], nil
}

// EnableSegment sets the Enabled flag on the segment identified by segType,
// inserting a known-but-missing type first (see ensureSegment).
func (c *Config) EnableSegment(segType string) error {
	if segType == "ai" { // the old name: all four parts
		for _, t := range AIParts {
			if err := c.EnableSegment(t); err != nil {
				return err
			}
		}
		return nil
	}
	seg, err := c.ensureSegment(segType)
	if err != nil {
		return err
	}
	seg.Enabled = true
	return nil
}

// DisableSegment clears the Enabled flag on the segment identified by segType,
// inserting a known-but-missing type first so the file records the choice.
func (c *Config) DisableSegment(segType string) error {
	if segType == "ai" { // the old name: all four parts
		for _, t := range AIParts {
			if err := c.DisableSegment(t); err != nil {
				return err
			}
		}
		return nil
	}
	seg, err := c.ensureSegment(segType)
	if err != nil {
		return err
	}
	seg.Enabled = false
	return nil
}

// ToggleSegment flips the Enabled flag on the segment identified by segType. A
// known-but-missing type is effectively off, so its first toggle turns it on.
func (c *Config) ToggleSegment(segType string) error {
	if segType == "ai" { // the old name: all four parts
		for _, t := range AIParts {
			if err := c.ToggleSegment(t); err != nil {
				return err
			}
		}
		return nil
	}
	seg, err := c.ensureSegment(segType)
	if err != nil {
		return err
	}
	seg.Enabled = !seg.Enabled
	return nil
}
