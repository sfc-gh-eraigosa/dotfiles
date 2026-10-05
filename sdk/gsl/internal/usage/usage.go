// Package usage records the host's rate-limit usage — the 5h and 7d windows
// gsl already renders — so other tools can read it without a payload of their
// own.
//
// The host (Claude Code or Antigravity) hands gsl its usage on stdin after
// every turn and nowhere else; the status bar was the only place it surfaced.
// Each render that carries rate limits writes one snapshot per host to
// ${XDG_STATE_HOME:-$HOME/.local/state}/gsl/usage/<host>.json, and `gsl usage`
// reads them back. All sessions of one host share an account, so the newest
// write is the account's current state.
package usage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/payload"
)

// Host names, as used in the snapshot and its file name.
const (
	HostClaude      = "claude"
	HostAntigravity = "antigravity"
)

// Window is one rate-limit window: how much of it is used and when it resets.
type Window struct {
	UsedPercentage float64 `json:"used_percentage"`
	// ResetsAt is RFC3339 UTC, or "" when the host did not say.
	ResetsAt string `json:"resets_at,omitempty"`
}

// Snapshot is the last usage a host reported.
type Snapshot struct {
	Host       string    `json:"host"`
	Model      string    `json:"model,omitempty"`
	CapturedAt time.Time `json:"captured_at"`
	FiveHour   *Window   `json:"five_hour,omitempty"`
	SevenDay   *Window   `json:"seven_day,omitempty"`
}

// FromPayload builds a snapshot from a parsed payload. ok is false when the
// payload carries no usable rate-limit window — nothing to record.
func FromPayload(p payload.Payload, now time.Time) (s Snapshot, ok bool) {
	s = Snapshot{Host: HostClaude, CapturedAt: now.UTC()}
	if p.IsAntigravity() {
		s.Host = HostAntigravity
	}
	if p.Model != nil {
		switch {
		case p.Model.ID != nil && *p.Model.ID != "":
			s.Model = *p.Model.ID
		case p.Model.DisplayName != nil:
			s.Model = *p.Model.DisplayName
		}
	}
	if p.RateLimits != nil {
		s.FiveHour = window(p.RateLimits.FiveHour)
		s.SevenDay = window(p.RateLimits.SevenDay)
	}
	return s, s.FiveHour != nil || s.SevenDay != nil
}

func window(w *payload.RateWindow) *Window {
	if w == nil || w.UsedPercentage == nil {
		return nil
	}
	out := &Window{UsedPercentage: *w.UsedPercentage}
	if w.ResetsAt != nil {
		out.ResetsAt = w.ResetsAt.String()
	}
	return out
}

// Dir is where snapshots live: ${XDG_STATE_HOME:-$HOME/.local/state}/gsl/usage.
func Dir() (string, error) {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return "", fmt.Errorf("usage: no state directory: %w", err)
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "gsl", "usage"), nil
}

// Write stores s as <dir>/<host>.json, atomically: concurrent sessions render
// at the same time, and a reader must never see half a file.
func Write(dir string, s Snapshot) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+s.Host+"-*.json")
	if err != nil {
		return err
	}
	// On failure, the temp file is cleaned up best-effort; the write error is
	// the one worth returning.
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, s.Host+".json"))
}

// ErrNone means no snapshot has been recorded for the host yet.
var ErrNone = errors.New("no usage recorded yet")

// Read returns the snapshot for one host.
func Read(dir, host string) (Snapshot, error) {
	var s Snapshot
	data, err := os.ReadFile(filepath.Join(dir, host+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return s, fmt.Errorf("%s: %w", host, ErrNone)
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("%s: %w", host, err)
	}
	return s, nil
}

// ReadAll returns every recorded host's snapshot, sorted by host.
func ReadAll(dir string) ([]Snapshot, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Snapshot
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".json") {
			continue
		}
		s, err := Read(dir, strings.TrimSuffix(name, ".json"))
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Host < out[j].Host })
	return out, nil
}
