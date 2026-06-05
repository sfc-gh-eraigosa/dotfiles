// Package observe owns the gsl structured-log writer and the helpers used
// to resolve its on-disk location.
//
// The log file path is chosen by ResolveLogPath using this precedence:
//
//  1. $GSL_LOG_FILE — verbatim override (used by tests and power users).
//  2. $XDG_STATE_HOME/gsl/gsl.log — when XDG_STATE_HOME is set.
//  3. $HOME/.local/state/gsl/gsl.log — when its parent can be created.
//  4. $HOME/.cache/gsl/gsl.log — final fallback.
//  5. "" — caller switches to a no-op (io.Discard) logger.
//
// Construction in this package is total: callers never receive an error,
// and a logger is always usable. This keeps the gsl hot path free of
// observability-induced failure modes.
package observe

import (
	"os"
	"path/filepath"
)

// ResolveLogPath chooses the gsl log file path per the package precedence
// rules. The returned path's parent directory is created on success. An
// empty string means no usable path was found; the caller MUST fall back
// to a no-op logger.
func ResolveLogPath() string {
	if v := os.Getenv("GSL_LOG_FILE"); v != "" {
		return v
	}
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		p := filepath.Join(xdg, "gsl", "gsl.log")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err == nil {
			return p
		}
		return ""
	}
	home := os.Getenv("HOME")
	if home == "" {
		return ""
	}
	primary := filepath.Join(home, ".local", "state", "gsl", "gsl.log")
	if err := os.MkdirAll(filepath.Dir(primary), 0o755); err == nil {
		return primary
	}
	fallback := filepath.Join(home, ".cache", "gsl", "gsl.log")
	if err := os.MkdirAll(filepath.Dir(fallback), 0o755); err == nil {
		return fallback
	}
	return ""
}
