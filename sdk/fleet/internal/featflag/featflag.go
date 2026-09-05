// Package featflag resolves the fleet-update gff flags into fail-open
// Settings. It never imports the gff SDK directly (see gff.go for that
// adapter) so it stays trivially testable with the Static fixture below.
package featflag

import (
	"fmt"
	"path/filepath"
	"reflect"
)

// Flag keys (declared in .github/gff/features.yaml, area `fleet`).
const (
	KeyEnabled = "fleet.update.enabled"
	KeyConfig  = "fleet.update.config"
)

// Source is the minimal gff surface featflag needs. gff.GFF implements it in
// gff.go; Static implements it here for tests.
type Source interface {
	Bool(key string) (bool, error)
	Strings(key string) ([]string, error)
}

// Settings is the resolved, fail-open view of the fleet.update.* flags.
type Settings struct {
	// Enabled is true unless the source explicitly resolved fleet.update.enabled
	// to false. Any error resolving it defaults to true.
	Enabled bool
	// ConfigPath is the fleet.yaml path implied by fleet.update.config, or ""
	// when the caller should fall back to its own default (unset, "home", an
	// error, or an unrecognized selection all map to "").
	ConfigPath string
	// Note explains any fallback taken, empty when both flags resolved cleanly.
	Note string
}

// Static is a fixed-value Source for tests. Err, if set, is returned by both
// Bool and Strings; otherwise a missing key is itself an error.
type Static struct {
	Bools map[string]bool
	Strs  map[string][]string
	Err   error
}

func (s Static) Bool(key string) (bool, error) {
	if s.Err != nil {
		return false, s.Err
	}
	v, ok := s.Bools[key]
	if !ok {
		return false, fmt.Errorf("featflag: unknown key %q", key)
	}
	return v, nil
}

func (s Static) Strings(key string) ([]string, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	v, ok := s.Strs[key]
	if !ok {
		return nil, fmt.Errorf("featflag: unknown key %q", key)
	}
	return v, nil
}

// Resolve reads the fleet.update.* flags from src and returns fail-open
// Settings: no code path here can return Enabled=false except an explicit,
// successfully-resolved `false` for fleet.update.enabled.
func Resolve(src Source, home, repoDir string) Settings {
	if src == nil || isTypedNil(src) {
		return Settings{Enabled: true, Note: "featflag: no source configured, using built-in defaults"}
	}

	settings := Settings{Enabled: true}

	enabled, err := src.Bool(KeyEnabled)
	if err != nil {
		settings.Note = appendNote(settings.Note, fmt.Sprintf("fleet.update.enabled: %v (defaulting to enabled)", err))
	} else {
		settings.Enabled = enabled
	}

	locs, err := src.Strings(KeyConfig)
	if err != nil {
		settings.Note = appendNote(settings.Note, fmt.Sprintf("fleet.update.config: %v (defaulting to caller's config path)", err))
		return settings
	}
	if len(locs) == 0 {
		settings.Note = appendNote(settings.Note, "fleet.update.config: no selection (defaulting to caller's config path)")
		return settings
	}
	if len(locs) > 1 {
		// A SINGLE-mode choice with two selections is a misconfigured flag
		// file; taking the first silently would make YAML option order decide.
		settings.Note = appendNote(settings.Note, fmt.Sprintf("fleet.update.config: %d selections %v where one is expected (defaulting to caller's config path)", len(locs), locs))
		return settings
	}

	switch loc := locs[0]; loc {
	case "home":
		// ConfigPath stays "" - the caller resolves its own $XDG_CONFIG_HOME default.
	case "repo":
		if !filepath.IsAbs(repoDir) {
			// A cwd-relative plan path would read whatever checkout the
			// operator happens to be standing in.
			settings.Note = appendNote(settings.Note, fmt.Sprintf("fleet.update.config: repo location needs an absolute repo dir, got %q (defaulting to caller's config path)", repoDir))
			return settings
		}
		settings.ConfigPath = filepath.Join(repoDir, "opt/etc/fleet/fleet.yaml")
	default:
		settings.Note = appendNote(settings.Note, fmt.Sprintf("fleet.update.config: unrecognized selection %q (defaulting to caller's config path)", loc))
	}

	return settings
}

func appendNote(existing, add string) string {
	if existing == "" {
		return add
	}
	return existing + "; " + add
}

// isTypedNil reports whether src is a nil pointer wrapped in the interface —
// `var g *GFF; Resolve(g, …)` — which `src == nil` cannot see.
func isTypedNil(src Source) bool {
	v := reflect.ValueOf(src)
	return v.Kind() == reflect.Ptr && v.IsNil()
}
