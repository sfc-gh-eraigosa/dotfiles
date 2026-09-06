package gh

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The fake is what every family and engine test runs against: it serves
// fixtures, records what was asked, and fails loudly on an unexpected call.
func TestFakeServesFixturesAndRecordsCalls(t *testing.T) {
	f := NewFake()
	f.Get("/repos/o/r", 200, `{"full_name":"o/r","has_wiki":false}`)
	f.Get("/repos/o/r/labels", 200, `[{"name":"bug"}]`)

	var repo struct {
		FullName string `json:"full_name"`
		HasWiki  bool   `json:"has_wiki"`
	}
	status, err := f.Do(context.Background(), "GET", "/repos/o/r", nil, &repo)
	if err != nil || status != 200 || repo.FullName != "o/r" || repo.HasWiki {
		t.Fatalf("status=%d repo=%+v err=%v", status, repo, err)
	}
	if _, err := f.Do(context.Background(), "PATCH", "/repos/o/r", map[string]any{"has_wiki": true}, nil); err != nil {
		t.Fatal(err)
	}
	var names []string
	if err := f.Paginate(context.Background(), "/repos/o/r/labels", func(raw json.RawMessage) error {
		var l struct{ Name string }
		_ = json.Unmarshal(raw, &l)
		names = append(names, l.Name)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "bug" {
		t.Fatalf("names = %v", names)
	}

	calls := f.Calls()
	if len(calls) != 3 {
		t.Fatalf("want 3 recorded calls, got %d: %v", len(calls), calls)
	}
	if calls[0].Method != "GET" || calls[0].Path != "/repos/o/r" {
		t.Errorf("call 0 = %+v", calls[0])
	}
	if calls[1].Method != "PATCH" || calls[1].Body["has_wiki"] != true {
		t.Errorf("call 1 = %+v", calls[1])
	}
	// Writes are what apply must prove, so they are also available alone.
	if w := f.Writes(); len(w) != 1 || w[0].Path != "/repos/o/r" {
		t.Errorf("writes = %+v", w)
	}
	if f.Body("PATCH", "/repos/o/r")["has_wiki"] != true {
		t.Errorf("Body helper = %v", f.Body("PATCH", "/repos/o/r"))
	}
}

func TestFakeUnexpectedCallIsAnError(t *testing.T) {
	f := NewFake()
	status, err := f.Do(context.Background(), "GET", "/nope", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "/nope") {
		t.Fatalf("want an error naming the path, got %v (status %d)", err, status)
	}
	if err := f.Paginate(context.Background(), "/nope", func(json.RawMessage) error { return nil }); err == nil {
		t.Fatal("want an error for an unstubbed paginate")
	}
}

func TestFakeCanFailAndServeFromFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "repo.json"), []byte(`{"full_name":"o/r"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	f := NewFake()
	f.GetFile(t, "/repos/o/r", filepath.Join(dir, "repo.json"))
	f.Fail("PATCH", "/repos/o/r", 403, "Resource not accessible by personal access token")

	var repo struct {
		FullName string `json:"full_name"`
	}
	if _, err := f.Do(context.Background(), "GET", "/repos/o/r", nil, &repo); err != nil || repo.FullName != "o/r" {
		t.Fatalf("repo=%+v err=%v", repo, err)
	}
	status, err := f.Do(context.Background(), "PATCH", "/repos/o/r", nil, nil)
	if status != 403 || err == nil || !strings.Contains(err.Error(), "not accessible") {
		t.Fatalf("status=%d err=%v", status, err)
	}
}

// A family that reads a page at a time must see the same pagination the
// real client does.
func TestFakePaginatesMultiplePages(t *testing.T) {
	f := NewFake()
	f.GetPages("/repos/o/r/labels", `[{"name":"a"}]`, `[{"name":"b"},{"name":"c"}]`)
	var got []string
	if err := f.Paginate(context.Background(), "/repos/o/r/labels", func(raw json.RawMessage) error {
		var l struct{ Name string }
		_ = json.Unmarshal(raw, &l)
		got = append(got, l.Name)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "a,b,c" {
		t.Fatalf("got = %v", got)
	}
}

// Fake and REST must satisfy the same interface — that is the whole point
// of the seam.
func TestBothImplementClient(t *testing.T) {
	var _ Client = NewFake()
	var _ Client = NewREST(RESTOpts{Bearer: "t"})
}
