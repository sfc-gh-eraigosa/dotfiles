// Package config manages the gsl configuration file.
//
// Config file location: ${XDG_CONFIG_HOME:-~/.config}/gsl/config.json
//
// Default() returns a fully working configuration with all 4 segments
// enabled. Load() for a missing file returns Default() with no error.
// Save() creates parent directories as needed.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Segment represents a single status-line segment.
type Segment struct {
	// Type is the segment kind: "dirgit", "repo", "ai", or "time".
	Type string `json:"type"`
	// Enabled controls whether this segment is rendered.
	Enabled bool `json:"enabled"`
	// Options holds segment-specific key-value overrides.
	Options map[string]any `json:"options,omitempty"`
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
	// Styles holds user-defined style overrides keyed by segment type or
	// element name. Kept as map[string]any for CP1; CP2 will type it further.
	Styles map[string]any `json:"styles,omitempty"`
}

// Default returns a fully usable Config with all 4 segments enabled in the
// canonical order: dirgit, repo, ai, time.
func Default() Config {
	return Config{
		Enabled: true,
		Segments: []Segment{
			{Type: "dirgit", Enabled: true},
			{Type: "repo", Enabled: true},
			{Type: "ai", Enabled: true},
			{Type: "time", Enabled: true},
		},
		Timezone:   "America/Los_Angeles",
		TimeFormat: "15:04:05",
		DateFormat: "2006-01-02",
		Style:      "powerline",
	}
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
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Default(), nil
		}
		return Config{}, fmt.Errorf("config: read %s: %w", path, err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("config: parse %s: %w", path, err)
	}
	return c, nil
}

// Save writes c to path as indented JSON, creating any missing parent
// directories. It overwrites an existing file.
func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("config: create dir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
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

// EnableSegment sets the Enabled flag on the segment identified by segType.
// Returns an error if no such segment exists.
func (c *Config) EnableSegment(segType string) error {
	seg := c.findSegment(segType)
	if seg == nil {
		return fmt.Errorf("config: unknown segment type %q", segType)
	}
	seg.Enabled = true
	return nil
}

// DisableSegment clears the Enabled flag on the segment identified by segType.
// Returns an error if no such segment exists.
func (c *Config) DisableSegment(segType string) error {
	seg := c.findSegment(segType)
	if seg == nil {
		return fmt.Errorf("config: unknown segment type %q", segType)
	}
	seg.Enabled = false
	return nil
}

// ToggleSegment flips the Enabled flag on the segment identified by segType.
// Returns an error if no such segment exists.
func (c *Config) ToggleSegment(segType string) error {
	seg := c.findSegment(segType)
	if seg == nil {
		return fmt.Errorf("config: unknown segment type %q", segType)
	}
	seg.Enabled = !seg.Enabled
	return nil
}
