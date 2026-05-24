// Package registry_test verifies the registry schema per
// src/gss/docs/plan.md PR-17: struct round-trip, restack_count default,
// unknown-field preservation (v1.1 forward-compat), byte-stable re-write,
// and rejection of a newer schema_version.
package registry_test

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	stderrors "errors"

	"github.com/wenlock/dotfiles/gss/internal/errors"
	"github.com/wenlock/dotfiles/gss/internal/registry"
)

func canonical() registry.Registry {
	return registry.Registry{
		SchemaVersion: 1,
		Features: []registry.Feature{{
			Name:              "parallel-worktrees",
			StartedAt:         "2026-05-17T10:34:00Z",
			BaseCommit:        "abc123",
			DefaultBaseBranch: "main",
			Description:       "Move worktree mechanics into gss",
			Workers: []registry.Worker{
				{
					User: "eraigosa", Purpose: "api", Suffix: "",
					Branch:     "feature/parallel-worktrees/eraigosa/api",
					Worktree:   "/wt/eraigosa/dotfiles/parallel-worktrees/eraigosa/api",
					BaseBranch: "main", Backend: "git",
					StartedAt:   "2026-05-17T10:34:00Z",
					Description: "Implement worker add + registry writes",
					SpawnedBy: &registry.SpawnedBy{
						Engine: "claude", SessionID: "c1a2b3", StartedAt: "2026-05-17T10:34:00Z",
					},
					PRURL: "https://github.com/me/dotfiles/pull/42", PRState: "draft",
				},
				{
					User: "eraigosa", Purpose: "ui", Suffix: "moss",
					Branch:     "feature/parallel-worktrees/eraigosa/ui-moss",
					Worktree:   "/wt/eraigosa/dotfiles/parallel-worktrees/eraigosa/ui-moss",
					BaseBranch: "feature/parallel-worktrees/eraigosa/api", Backend: "git",
					StartedAt:   "2026-05-17T11:02:00Z",
					Description: "Wire feature list output",
				},
			},
		}},
	}
}

func TestRoundTrip(t *testing.T) {
	reg := canonical()
	data, err := registry.Marshal(reg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := registry.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(reg, got) {
		t.Errorf("round-trip mismatch:\n got %+v\nwant %+v", got, reg)
	}
}

func TestByteStability(t *testing.T) {
	data1, err := registry.Marshal(canonical())
	if err != nil {
		t.Fatalf("Marshal 1: %v", err)
	}
	reg, err := registry.Unmarshal(data1)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	data2, err := registry.Marshal(reg)
	if err != nil {
		t.Fatalf("Marshal 2: %v", err)
	}
	if !bytes.Equal(data1, data2) {
		t.Errorf("re-write not byte-stable:\n--- first ---\n%s\n--- second ---\n%s", data1, data2)
	}
}

func TestRestackCountDefaultsZero(t *testing.T) {
	// A worker JSON with no restack_count must default to 0.
	js := `{"schema_version":1,"features":[{"name":"f","started_at":"t","base_commit":"c","default_base_branch":"main","workers":[{"user":"u","purpose":"p","suffix":"","branch":"b","worktree":"w","base_branch":"main","backend":"git","started_at":"t","description":"d"}]}]}`
	reg, err := registry.Unmarshal([]byte(js))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got := reg.Features[0].Workers[0].RestackCount; got != 0 {
		t.Errorf("RestackCount = %d; want 0 (default)", got)
	}
}

func TestUnknownFieldsPreserved(t *testing.T) {
	// A worker carries a v1.1 "sessions" array this build doesn't model.
	js := `{"schema_version":1,"features":[{"name":"f","started_at":"t","base_commit":"c","default_base_branch":"main","workers":[{"user":"u","purpose":"p","suffix":"","branch":"b","worktree":"w","base_branch":"main","backend":"git","started_at":"t","description":"d","sessions":[{"engine":"gemini","session_id":"resume-1"}]}]}]}`
	reg, err := registry.Unmarshal([]byte(js))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	w := reg.Features[0].Workers[0]
	if _, ok := w.Extra["sessions"]; !ok {
		t.Fatalf("unknown 'sessions' field not captured into Extra: %+v", w.Extra)
	}
	out, err := registry.Marshal(reg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(out), "sessions") || !strings.Contains(string(out), "resume-1") {
		t.Errorf("re-write dropped the preserved 'sessions' data:\n%s", out)
	}
	// And it survives a second round-trip.
	reg2, err := registry.Unmarshal(out)
	if err != nil {
		t.Fatalf("Unmarshal 2: %v", err)
	}
	if _, ok := reg2.Features[0].Workers[0].Extra["sessions"]; !ok {
		t.Error("'sessions' lost on second round-trip")
	}
}

func TestSchemaVersionRejected(t *testing.T) {
	js := `{"schema_version":2,"features":[]}`
	_, err := registry.Unmarshal([]byte(js))
	if err == nil {
		t.Fatal("schema_version 2: err = nil; want rejection")
	}
	if !stderrors.Is(err, errors.ErrSchemaMismatch) {
		t.Errorf("err = %v; want wrapping ErrSchemaMismatch", err)
	}
}

func TestSchemaVersionDefaultsToSupported(t *testing.T) {
	reg, err := registry.Unmarshal([]byte(`{"features":[]}`))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if reg.SchemaVersion != registry.SupportedSchemaVersion {
		t.Errorf("absent schema_version = %d; want normalised to %d", reg.SchemaVersion, registry.SupportedSchemaVersion)
	}
}
