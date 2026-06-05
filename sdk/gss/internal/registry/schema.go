// Package registry defines the on-disk shape of gss's per-repo
// registry.json — the active features and their workers (design.md →
// "Filesystem layout", "Description and provenance fields"). This file is
// the schema + JSON (de)serialisation; locking and atomic writes land in
// PR-18, reconciliation in PR-19.
//
// # Forward compatibility
//
// A v1 gss must not destroy fields a future v1.1 writes (e.g. a worker's
// append-only `sessions` array). So Worker captures any unknown JSON keys
// into Extra and re-emits them on write — a load→save round-trip by an
// older binary preserves newer data.
package registry

import (
	"encoding/json"
)

// Registry is the root of registry.json.
type Registry struct {
	SchemaVersion int       `json:"schema_version"`
	Features      []Feature `json:"features"`
}

// Feature is one named workstream and its workers.
type Feature struct {
	Name              string   `json:"name"`
	StartedAt         string   `json:"started_at"`
	BaseCommit        string   `json:"base_commit"`
	DefaultBaseBranch string   `json:"default_base_branch"`
	Description       string   `json:"description,omitempty"`
	Workers           []Worker `json:"workers"`
}

// SpawnedBy records the AI-session provenance captured at worker creation.
// Write-once; never overwritten by later checkpoints/rebases.
type SpawnedBy struct {
	Engine         string `json:"engine"`
	SessionID      string `json:"session_id,omitempty"`
	PaneID         string `json:"pane_id,omitempty"`
	TmuxMgrSession string `json:"tmux_mgr_session,omitempty"`
	StartedAt      string `json:"started_at,omitempty"`
}

// Worker is a single worktree's registry row. Unknown JSON keys are
// preserved across a load→save cycle via Extra (see package doc).
type Worker struct {
	User         string     `json:"user"`
	Purpose      string     `json:"purpose"`
	Suffix       string     `json:"suffix"`
	Branch       string     `json:"branch"`
	Worktree     string     `json:"worktree"`
	BaseBranch   string     `json:"base_branch"`
	Backend      string     `json:"backend"`
	RestackCount int        `json:"restack_count,omitempty"`
	StartedAt    string     `json:"started_at"`
	Description  string     `json:"description"`
	SpawnedBy    *SpawnedBy `json:"spawned_by,omitempty"`
	PRURL        string     `json:"pr_url,omitempty"`
	PRState      string     `json:"pr_state,omitempty"`

	// Extra holds JSON keys not modelled by this struct, so a v1 binary
	// preserves fields written by a newer schema. Not emitted when empty.
	Extra map[string]json.RawMessage `json:"-"`
}

// workerKnownKeys are the JSON keys Worker models directly; everything else
// in a worker object is captured into Extra.
var workerKnownKeys = []string{
	"user", "purpose", "suffix", "branch", "worktree", "base_branch",
	"backend", "restack_count", "started_at", "description", "spawned_by",
	"pr_url", "pr_state",
}

// UnmarshalJSON decodes the known fields and stashes any remaining keys in
// Extra.
func (w *Worker) UnmarshalJSON(data []byte) error {
	type alias Worker // no methods → no recursion
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*w = Worker(a)

	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	for _, k := range workerKnownKeys {
		delete(m, k)
	}
	if len(m) > 0 {
		w.Extra = m
	} else {
		w.Extra = nil
	}
	return nil
}

// MarshalJSON emits the known fields plus any preserved Extra keys. Known
// keys always win over Extra (Extra can never shadow a modelled field).
func (w Worker) MarshalJSON() ([]byte, error) {
	type alias Worker
	b, err := json.Marshal(alias(w))
	if err != nil {
		return nil, err
	}
	if len(w.Extra) == 0 {
		return b, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	for k, v := range w.Extra {
		if _, exists := m[k]; !exists {
			m[k] = v
		}
	}
	return json.Marshal(m)
}
