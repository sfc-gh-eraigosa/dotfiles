package herdrmach

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

// F1a: the captured herdr output parses into one fully populated Machine.
func TestParseCapturedMachineList(t *testing.T) {
	b, err := os.ReadFile("testdata/machine-list.json")
	if err != nil {
		t.Fatal(err)
	}
	ms, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	want := []Machine{{
		ID: "eb4c99883c148f76937cc8bdfd81d539", Label: "bigbox", Target: "bigbox",
		Session: "default", Enabled: true, Selected: false,
	}}
	if !reflect.DeepEqual(ms, want) {
		t.Fatalf("got %+v\nwant %+v", ms, want)
	}

	empty, err := Parse([]byte("[]"))
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty array: got %#v, %v; want empty non-nil slice, nil", empty, err)
	}
}

// F1b: new fields are tolerated; anything that is not an array is an error,
// never an empty list that would read as "nothing saved".
func TestParseRejectsNonArrayAndToleratesNewFields(t *testing.T) {
	ms, err := Parse([]byte(`[{"target":"a","color":"red"}]`))
	if err != nil || len(ms) != 1 || ms[0].Target != "a" {
		t.Fatalf("unknown field: got %+v, %v", ms, err)
	}
	for _, bad := range []string{`{"error":"no server"}`, `"no server running"`, ``, `null`} {
		if ms, err := Parse([]byte(bad)); err == nil {
			t.Errorf("Parse(%q) = %+v, nil; want an error", bad, ms)
		}
	}
}

type fakeRunner struct {
	out  []byte
	err  error
	argv []string
}

func (f *fakeRunner) Output(_ context.Context, argv []string) ([]byte, error) {
	f.argv = argv
	return f.out, f.err
}

// F1c: a runner failure is returned with the argv named, never as an empty list.
func TestListSurfacesRunnerErrors(t *testing.T) {
	f := &fakeRunner{err: errors.New("exit status 7")}
	ms, err := List(context.Background(), f)
	if err == nil || ms != nil {
		t.Fatalf("got %+v, %v; want nil, error", ms, err)
	}
	if !strings.Contains(err.Error(), "herdr machine list --json") || !strings.Contains(err.Error(), "exit status 7") {
		t.Fatalf("error %q should name the argv and the cause", err)
	}

	f = &fakeRunner{out: []byte(`[{"target":"a"}]`)}
	ms, err = List(context.Background(), f)
	if err != nil || len(ms) != 1 {
		t.Fatalf("got %+v, %v", ms, err)
	}
	if want := []string{"herdr", "machine", "list", "--json"}; !reflect.DeepEqual(f.argv, want) {
		t.Fatalf("argv %q, want %q", f.argv, want)
	}

	f = &fakeRunner{out: []byte(`oops`)}
	if _, err := List(context.Background(), f); err == nil {
		t.Fatal("unparseable output must be an error")
	}
}

// F2a: a host is saved when a machine targets it, whatever its label.
func TestSavedMatchesTargetNotLabel(t *testing.T) {
	ms := []Machine{{ID: "1", Label: "renamed", Target: "a"}, {ID: "2", Label: "c", Target: "b"}}
	if m, ok := Saved(ms, "a"); !ok || m.ID != "1" {
		t.Fatalf("Saved(a) = %+v, %v", m, ok)
	}
	if _, ok := Saved(ms, "c"); ok {
		t.Fatal("matched on label")
	}
	if _, ok := Saved(nil, "a"); ok {
		t.Fatal("matched in an empty list")
	}
}

// F3a: the alias is one inert argv element; no shell is ever involved.
func TestAddArgvIsInertToMetacharacters(t *testing.T) {
	alias := "x; rm -rf ~"
	got := AddArgv(alias)
	want := []string{"herdr", "machine", "add", alias, "--label", alias}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for _, a := range got {
		if a == "sh" || a == "-c" {
			t.Fatalf("argv goes through a shell: %q", got)
		}
	}
}
