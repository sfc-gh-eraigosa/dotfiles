package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
)

// TestDefaultIsUsable verifies that Default() returns a config that passes
// basic sanity checks: enabled, all 4 segments present and enabled, valid
// timezone and style defaults.
func TestDefaultIsUsable(t *testing.T) {
	c := config.Default()

	if !c.Enabled {
		t.Error("Default().Enabled should be true")
	}
	if c.Style != "powerline" {
		t.Errorf("Default().Style = %q; want 'powerline'", c.Style)
	}
	if c.Timezone != "America/Los_Angeles" {
		t.Errorf("Default().Timezone = %q; want 'America/Los_Angeles'", c.Timezone)
	}

	// All 4 segments in order.
	wantTypes := []string{"dirgit", "repo", "ai", "time"}
	if len(c.Segments) != len(wantTypes) {
		t.Fatalf("Default() has %d segments; want %d", len(c.Segments), len(wantTypes))
	}
	for i, seg := range c.Segments {
		if seg.Type != wantTypes[i] {
			t.Errorf("Segments[%d].Type = %q; want %q", i, seg.Type, wantTypes[i])
		}
		if !seg.Enabled {
			t.Errorf("Segments[%d] (%s) should be enabled by default", i, seg.Type)
		}
	}
}

// TestLoadMissingFileReturnsDefaults verifies that loading a nonexistent path
// returns Default() and no error.
func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	c, err := config.Load(filepath.Join(t.TempDir(), "nonexistent", "config.json"))
	if err != nil {
		t.Fatalf("Load(missing) returned error: %v; want nil", err)
	}
	d := config.Default()
	if c.Style != d.Style {
		t.Errorf("Load(missing).Style = %q; want %q", c.Style, d.Style)
	}
	if len(c.Segments) != len(d.Segments) {
		t.Errorf("Load(missing).Segments len = %d; want %d", len(c.Segments), len(d.Segments))
	}
}

// TestSaveAndLoadRoundTrip verifies that Save then Load restores the same
// config values.
func TestSaveAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gsl", "config.json")

	c := config.Default()
	c.Style = "plain"
	c.Timezone = "UTC"
	c.Enabled = false

	if err := config.Save(path, c); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Style != c.Style {
		t.Errorf("Style: got %q; want %q", loaded.Style, c.Style)
	}
	if loaded.Timezone != c.Timezone {
		t.Errorf("Timezone: got %q; want %q", loaded.Timezone, c.Timezone)
	}
	if loaded.Enabled != c.Enabled {
		t.Errorf("Enabled: got %v; want %v", loaded.Enabled, c.Enabled)
	}
	if len(loaded.Segments) != len(c.Segments) {
		t.Errorf("Segments len: got %d; want %d", len(loaded.Segments), len(c.Segments))
	}
}

// TestSaveCreatesParentDir verifies that Save creates missing parent directories.
func TestSaveCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "config.json")
	if err := config.Save(path, config.Default()); err != nil {
		t.Fatalf("Save(nested path) returned error: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file should exist after Save: %v", err)
	}
}

// TestToggleMasterEnable tests EnableMaster/DisableMaster helpers.
func TestToggleMasterEnable(t *testing.T) {
	c := config.Default()
	c.Enabled = true

	c.DisableMaster()
	if c.Enabled {
		t.Error("DisableMaster should set Enabled to false")
	}
	c.EnableMaster()
	if !c.Enabled {
		t.Error("EnableMaster should set Enabled to true")
	}
}

// TestToggleSegment tests enabling, disabling, and toggling a segment by type.
func TestToggleSegment(t *testing.T) {
	c := config.Default()

	// All segments start enabled.
	if err := c.DisableSegment("repo"); err != nil {
		t.Fatalf("DisableSegment('repo'): %v", err)
	}
	seg := findSegment(t, c, "repo")
	if seg.Enabled {
		t.Error("repo segment should be disabled")
	}

	if err := c.EnableSegment("repo"); err != nil {
		t.Fatalf("EnableSegment('repo'): %v", err)
	}
	seg = findSegment(t, c, "repo")
	if !seg.Enabled {
		t.Error("repo segment should be enabled")
	}

	// ToggleSegment flips state.
	initialState := seg.Enabled
	if err := c.ToggleSegment("repo"); err != nil {
		t.Fatalf("ToggleSegment('repo'): %v", err)
	}
	seg = findSegment(t, c, "repo")
	if seg.Enabled == initialState {
		t.Errorf("ToggleSegment should flip state; initial=%v, after=%v", initialState, seg.Enabled)
	}
}

// TestToggleSegmentUnknownType verifies that operating on a nonexistent type
// returns an error.
func TestToggleSegmentUnknownType(t *testing.T) {
	c := config.Default()
	if err := c.EnableSegment("nonexistent"); err == nil {
		t.Error("EnableSegment('nonexistent') should return an error")
	}
	if err := c.DisableSegment("nonexistent"); err == nil {
		t.Error("DisableSegment('nonexistent') should return an error")
	}
	if err := c.ToggleSegment("nonexistent"); err == nil {
		t.Error("ToggleSegment('nonexistent') should return an error")
	}
}

// TestLoadWritesValidJSON verifies the saved file is valid JSON.
func TestLoadWritesValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := config.Save(path, config.Default()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Errorf("saved file is not valid JSON: %v", err)
	}
}

// TestDefaultTimeFormats verifies sensible default format strings exist.
func TestDefaultTimeFormats(t *testing.T) {
	c := config.Default()
	if c.TimeFormat == "" {
		t.Error("TimeFormat should not be empty in Default()")
	}
	if c.DateFormat == "" {
		t.Error("DateFormat should not be empty in Default()")
	}
}

// TestLoadMalformedJSONReturnsError verifies that loading a file containing
// invalid JSON returns a non-nil error and does not panic. (Finding #10)
func TestLoadMalformedJSONReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{invalid json`), 0o644); err != nil {
		t.Fatalf("setup: WriteFile: %v", err)
	}

	_, err := config.Load(path)
	if err == nil {
		t.Error("Load(malformed JSON) returned nil error; want non-nil")
	}
}

// TestLoadPartialConfigMergesDefaults verifies that a partial hand-edited
// config keeps the defaults for fields it does not specify, while still
// honoring the fields it does specify. (Finding #4)
func TestLoadPartialConfigMergesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// User wrote only the master enable flag; everything else should default.
	if err := os.WriteFile(path, []byte(`{"enabled": true}`), 0o644); err != nil {
		t.Fatalf("setup: WriteFile: %v", err)
	}

	c, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load(partial): %v", err)
	}

	d := config.Default()
	if !c.Enabled {
		t.Error("Load(partial).Enabled = false; want true")
	}
	if len(c.Segments) != len(d.Segments) {
		t.Fatalf("Load(partial).Segments len = %d; want %d (defaults)", len(c.Segments), len(d.Segments))
	}
	for i, seg := range c.Segments {
		if seg.Type != d.Segments[i].Type {
			t.Errorf("Segments[%d].Type = %q; want %q (default)", i, seg.Type, d.Segments[i].Type)
		}
	}
	if c.Timezone != d.Timezone {
		t.Errorf("Load(partial).Timezone = %q; want %q (default)", c.Timezone, d.Timezone)
	}
	if c.TimeFormat != d.TimeFormat {
		t.Errorf("Load(partial).TimeFormat = %q; want %q (default)", c.TimeFormat, d.TimeFormat)
	}
	if c.DateFormat != d.DateFormat {
		t.Errorf("Load(partial).DateFormat = %q; want %q (default)", c.DateFormat, d.DateFormat)
	}
	if c.Style != d.Style {
		t.Errorf("Load(partial).Style = %q; want %q (default)", c.Style, d.Style)
	}
}

// TestLoadPartialConfigHonorsExplicitOverrides verifies that fields the user
// explicitly writes — including ones that override defaults — win over the
// merged Default() base. (Finding #4)
func TestLoadPartialConfigHonorsExplicitOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"style":"plain","timezone":"UTC"}`), 0o644); err != nil {
		t.Fatalf("setup: WriteFile: %v", err)
	}

	c, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load(partial overrides): %v", err)
	}

	if c.Style != "plain" {
		t.Errorf("Load.Style = %q; want %q (explicit override)", c.Style, "plain")
	}
	if c.Timezone != "UTC" {
		t.Errorf("Load.Timezone = %q; want %q (explicit override)", c.Timezone, "UTC")
	}
	// Unspecified fields keep defaults.
	if c.TimeFormat != config.Default().TimeFormat {
		t.Errorf("Load.TimeFormat = %q; want default", c.TimeFormat)
	}
}

// findSegment is a test helper that locates a segment by type or fails the test.
func findSegment(t *testing.T, c config.Config, segType string) config.Segment {
	t.Helper()
	for _, s := range c.Segments {
		if s.Type == segType {
			return s
		}
	}
	t.Fatalf("segment type %q not found in config", segType)
	return config.Segment{}
}
