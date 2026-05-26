package repo

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DefaultRegistryPath returns the canonical path to the gss worktrees
// registry file, honouring $XDG_CONFIG_HOME (falling back to $HOME/.config).
func DefaultRegistryPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(base, "gss", "worktrees", "registry.json")
}

// ErrUnsupportedSchema is returned by LoadRegistry when the file exists
// but its schema_version is not 1. Callers must treat this as "unusable
// registry — fall back to gh". It is a distinct sentinel so callers can
// distinguish it from ErrRegistryNotFound if they need to.
var ErrUnsupportedSchema = errors.New("repo: registry schema_version is not supported")

// ErrRegistryNotFound is returned when the registry file does not exist.
// Callers should fall back to gh.PR.
var ErrRegistryNotFound = errors.New("repo: registry file not found")

// Registry is the in-memory representation of the parsed registry.json.
type Registry struct {
	Features []Feature
}

// Feature corresponds to one element of the top-level "features" array.
type Feature struct {
	Name    string
	Workers []Worker
}

// Worker corresponds to one element of a feature's "workers" array.
type Worker struct {
	Branch   string
	Worktree string
	PRUrl    string // OPTIONAL: absent on uncheckpointed workers
	PRState  string // OPTIONAL: absent when PRUrl is absent
}

// rawRegistry is used only for JSON unmarshalling (schema guard lives here).
type rawRegistry struct {
	SchemaVersion int          `json:"schema_version"`
	Features      []rawFeature `json:"features"`
}

type rawFeature struct {
	Name    string      `json:"name"`
	Workers []rawWorker `json:"workers"`
}

type rawWorker struct {
	Branch   string `json:"branch"`
	Worktree string `json:"worktree"`
	PRUrl    string `json:"pr_url"`
	PRState  string `json:"pr_state"`
}

// LoadRegistry reads and parses the gss worktrees registry at path.
//
// Return semantics:
//
//   - (nil, ErrRegistryNotFound): file absent — caller should fall back to gh.
//   - (nil, ErrUnsupportedSchema): file present but schema_version != 1 —
//     registry is unusable; caller should fall back to gh.
//   - (nil, <other error>): unreadable or malformed JSON.
//   - (*Registry, nil): success.
func LoadRegistry(path string) (*Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrRegistryNotFound
		}
		return nil, err
	}

	var raw rawRegistry
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	if raw.SchemaVersion != 1 {
		return nil, ErrUnsupportedSchema
	}

	reg := &Registry{
		Features: make([]Feature, 0, len(raw.Features)),
	}
	for _, rf := range raw.Features {
		f := Feature{Name: rf.Name, Workers: make([]Worker, 0, len(rf.Workers))}
		for _, rw := range rf.Workers {
			f.Workers = append(f.Workers, Worker{
				Branch:   rw.Branch,
				Worktree: rw.Worktree,
				PRUrl:    rw.PRUrl,
				PRState:  rw.PRState,
			})
		}
		reg.Features = append(reg.Features, f)
	}
	return reg, nil
}

// WorkerMatch is the result of a successful Match call.
type WorkerMatch struct {
	// Feature is the parent feature's "name" field.
	Feature string
	// PRNumber is the trailing integer parsed from the worker's pr_url.
	// Zero when HasPR is false.
	PRNumber int
	// PRState is copied verbatim from the worker's "pr_state" field.
	// Empty string when HasPR is false.
	PRState string
	// HasPR is true when the worker had a pr_url that parsed successfully.
	HasPR bool
}

// Match searches reg for a worker whose worktree path equals toplevel OR
// whose branch equals branch. Toplevel match takes precedence over branch
// match so that the most-specific anchor wins. Returns (WorkerMatch, true)
// on the first hit, or (WorkerMatch{}, false) when nothing matches.
func Match(reg *Registry, toplevel, branch string) (*WorkerMatch, bool) {
	if reg == nil {
		return nil, false
	}

	// Two-pass: first look for a toplevel match (more specific), then branch.
	for _, f := range reg.Features {
		for _, w := range f.Workers {
			if w.Worktree == toplevel {
				return buildMatch(f.Name, w), true
			}
		}
	}
	for _, f := range reg.Features {
		for _, w := range f.Workers {
			if w.Branch == branch {
				return buildMatch(f.Name, w), true
			}
		}
	}
	return nil, false
}

// buildMatch constructs a WorkerMatch from a feature name and a matched Worker.
func buildMatch(featureName string, w Worker) *WorkerMatch {
	m := &WorkerMatch{Feature: featureName}
	if w.PRUrl != "" {
		if n := parsePRNumber(w.PRUrl); n > 0 {
			m.PRNumber = n
			m.PRState = w.PRState
			m.HasPR = true
		}
	}
	return m
}

// parsePRNumber extracts the trailing integer from a GitHub PR URL of the
// form ".../pull/<number>". Returns 0 when the URL does not end with a
// recognisable integer.
func parsePRNumber(prURL string) int {
	// Strip trailing slash, then take the last path segment.
	u := strings.TrimRight(prURL, "/")
	idx := strings.LastIndex(u, "/")
	if idx < 0 {
		return 0
	}
	seg := u[idx+1:]
	n, err := strconv.Atoi(seg)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}
