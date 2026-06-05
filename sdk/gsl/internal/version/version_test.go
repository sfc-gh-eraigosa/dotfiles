package version_test

import (
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/version"
)

func TestGetDefaults(t *testing.T) {
	// When linker vars are empty (test context), Get() must return the
	// fallback strings, not empty strings.
	info := version.Get()
	if info.Version == "" {
		t.Error("Version should not be empty; expected fallback 'dev'")
	}
	if info.Commit == "" {
		t.Error("Commit should not be empty; expected fallback 'none'")
	}
	if info.BuildDate == "" {
		t.Error("BuildDate should not be empty; expected fallback 'unknown'")
	}
	if info.Dirty == "" {
		t.Error("Dirty should not be empty; expected fallback 'false'")
	}
}

func TestGetFallbackValues(t *testing.T) {
	info := version.Get()
	if info.Version != "dev" {
		t.Errorf("expected Version fallback 'dev', got %q", info.Version)
	}
	if info.Commit != "none" {
		t.Errorf("expected Commit fallback 'none', got %q", info.Commit)
	}
	if info.BuildDate != "unknown" {
		t.Errorf("expected BuildDate fallback 'unknown', got %q", info.BuildDate)
	}
	if info.Dirty != "false" {
		t.Errorf("expected Dirty fallback 'false', got %q", info.Dirty)
	}
}

func TestGetReturnedFields(t *testing.T) {
	info := version.Get()
	// All fields should be non-empty strings.
	fields := map[string]string{
		"Version":   info.Version,
		"Commit":    info.Commit,
		"BuildDate": info.BuildDate,
		"Dirty":     info.Dirty,
	}
	for name, val := range fields {
		if val == "" {
			t.Errorf("field %s is empty; expected a non-empty fallback", name)
		}
	}
}
