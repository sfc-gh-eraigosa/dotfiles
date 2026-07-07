// Package theme provides detection-only logic for resolving a palette name
// from the host tool's settings file or terminal environment variables.
//
// It returns a palette NAME string (e.g. "dark", "light", "dark-daltonism",
// "dark8"). The actual palette definitions live in internal/style, which owns
// all color truth and merge logic.
//
// Security hardening (F5 / SEC-5 / SEC-7):
//
//   - Settings files are read via Lstat + type check (reject FIFOs, sockets,
//     device files) before opening.
//   - Symlinks are resolved; the resolved target must remain under home.
//   - The file body is bounded to 256 KiB with io.LimitReader.
//   - Any error at any stage degrades gracefully: readField returns "" so
//     Resolve falls through to the next priority level. The status line always
//     renders.
package theme

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxSettingsBytes = 256 * 1024 // 256 KiB

// readClaudeTheme reads the `theme` field from <home>/.claude/settings.json.
// Returns "" on any error (missing file, bad type, anomalous file).
// The returned value is the raw string from the JSON (e.g. "dark", "light",
// "system", "dark-daltonism").
func readClaudeTheme(home string) string {
	path := filepath.Join(home, ".claude", "settings.json")
	data, err := readSettingsFile(home, path)
	if err != nil {
		return ""
	}
	if len(data) == 0 {
		return ""
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return ""
	}

	raw, ok := obj["theme"]
	if !ok {
		return ""
	}

	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		return ""
	}
	return v
}

// readAntigravityTheme reads the `ui.theme` field for the Antigravity CLI.
// It reads the Antigravity settings file <home>/.gemini/antigravity-cli/
// settings.json when it exists; only when that file is absent does it fall
// back to the legacy Gemini CLI file <home>/.gemini/settings.json (Antigravity
// deliberately reuses the ~/.gemini directory). Gating the fallback on file
// absence — not on an empty ui.theme — avoids a second open+parse on every
// statusline render in the steady state where the Antigravity file exists but
// simply lacks ui.theme. Returns "" on any error. The value is a free-form
// string.
func readAntigravityTheme(home string) string {
	agyPath := filepath.Join(home, ".gemini", "antigravity-cli", "settings.json")
	if _, err := os.Lstat(agyPath); !errors.Is(err, os.ErrNotExist) {
		// The Antigravity file exists (or is anomalous): it is authoritative,
		// even when it lacks ui.theme.
		return readUITheme(home, agyPath)
	}
	// Legacy Gemini CLI settings file.
	return readUITheme(home, filepath.Join(home, ".gemini", "settings.json"))
}

// readUITheme reads the `ui.theme` field from a settings.json at path.
// Returns "" on any error (missing file, bad type, anomalous file).
func readUITheme(home, path string) string {
	data, err := readSettingsFile(home, path)
	if err != nil {
		return ""
	}
	if len(data) == 0 {
		return ""
	}

	// Parse top-level object.
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return ""
	}

	// Dig into "ui" sub-object.
	uiRaw, ok := obj["ui"]
	if !ok {
		return ""
	}

	var ui map[string]json.RawMessage
	if err := json.Unmarshal(uiRaw, &ui); err != nil {
		return ""
	}

	themeRaw, ok := ui["theme"]
	if !ok {
		return ""
	}

	var v string
	if err := json.Unmarshal(themeRaw, &v); err != nil {
		return ""
	}
	return v
}

// readSettingsFile reads path safely:
//
//  1. Lstat — reject non-regular files (FIFOs, sockets, devices).
//  2. Resolve symlinks — reject targets outside home.
//  3. Open + LimitReader at 256 KiB.
//
// Returns (nil, nil) for a missing file (not an error — degrade gracefully).
// Returns (nil, err) for any other problem.
func readSettingsFile(home, path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil // missing → degrade
		}
		return nil, fmt.Errorf("theme: lstat %s: %w", path, err)
	}

	mode := info.Mode()

	// Reject non-regular files before we try to open them. A FIFO open would
	// block; a socket/device could have side-effects.
	if mode&os.ModeNamedPipe != 0 ||
		mode&os.ModeSocket != 0 ||
		mode&os.ModeDevice != 0 ||
		mode&os.ModeCharDevice != 0 {
		return nil, fmt.Errorf("theme: %s is not a regular file (mode %v)", path, mode)
	}

	// If it's a symlink, resolve and validate the target is under home.
	if mode&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return nil, fmt.Errorf("theme: resolve symlink %s: %w", path, err)
		}
		// Clean both paths for a reliable prefix check.
		cleanHome := filepath.Clean(home) + string(filepath.Separator)
		cleanResolved := filepath.Clean(resolved)
		if !strings.HasPrefix(cleanResolved+string(filepath.Separator), cleanHome) &&
			cleanResolved != filepath.Clean(home) {
			return nil, fmt.Errorf("theme: symlink %s points outside home (%s)", path, resolved)
		}
		// Re-stat the resolved target to confirm it's a regular file.
		targetInfo, err := os.Stat(resolved)
		if err != nil {
			return nil, fmt.Errorf("theme: stat symlink target %s: %w", resolved, err)
		}
		if !targetInfo.Mode().IsRegular() {
			return nil, fmt.Errorf("theme: symlink target %s is not a regular file", resolved)
		}
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("theme: open %s: %w", path, err)
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxSettingsBytes+1))
	if err != nil {
		return nil, fmt.Errorf("theme: read %s: %w", path, err)
	}
	if len(data) > maxSettingsBytes {
		return nil, fmt.Errorf("theme: %s exceeds 256 KiB limit", path)
	}
	return data, nil
}
