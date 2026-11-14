package op

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault"
)

const marker = "PRIVATE-MATERIAL-123"

type call struct {
	argv  []string
	stdin []byte
}

// fakeOp is an in-memory 1Password: one vault, items by title.
type fakeOp struct {
	calls    []call
	signedIn bool
	missing  bool
	items    map[string]map[string]any // title -> item doc
	dupes    map[string]bool           // titles that are ambiguous
	listOut  []byte
	// afterWrite, when set, answers `item get` once a create/edit was made.
	afterWrite func() ([]byte, error)
}

func newFake() *fakeOp {
	return &fakeOp{signedIn: true, items: map[string]map[string]any{}, dupes: map[string]bool{}}
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func (f *fakeOp) seed(t *testing.T, title string) {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(fixture(t, "item.json"), &doc); err != nil {
		t.Fatal(err)
	}
	doc["title"] = title
	f.items[title] = doc
}

func payloadOf(doc map[string]any) string {
	for _, x := range doc["fields"].([]any) {
		m := x.(map[string]any)
		if m["label"] == "payload" {
			return m["value"].(string)
		}
	}
	return ""
}

func (f *fakeOp) writes() int {
	n := 0
	for _, c := range f.calls {
		if len(c.argv) > 2 && c.argv[1] == "item" && (c.argv[2] == "create" || c.argv[2] == "edit") {
			n++
		}
	}
	return n
}

func (f *fakeOp) exec(_ context.Context, name string, stdin []byte, args ...string) ([]byte, error) {
	f.calls = append(f.calls, call{append([]string{name}, args...), stdin})
	if f.missing {
		return nil, vault.ErrNotInstalled
	}
	if !f.signedIn {
		return nil, errors.New(`op: exit status 1: [ERROR] 2026/11/13 10:00:00 You are not currently signed in. Please run 'op signin --help' for instructions`)
	}
	switch args[0] {
	case "whoami":
		return []byte("URL: my.1password.com\n"), nil
	case "item":
		switch args[1] {
		case "get":
			if f.afterWrite != nil && f.writes() > 0 {
				return f.afterWrite()
			}
			title := args[2]
			if f.dupes[title] {
				return nil, fmt.Errorf(`op: exit status 1: [ERROR] More than one item matches "%s". Try again and specify the item by its ID:`, title)
			}
			doc, ok := f.items[title]
			if !ok {
				return nil, fmt.Errorf(`op: exit status 1: [ERROR] "%s" isn't an item in the "Private" vault. Specify the item with its UUID, name, or domain.`, title)
			}
			return json.Marshal(doc)
		case "create":
			var doc map[string]any
			if err := json.Unmarshal(stdin, &doc); err != nil {
				return nil, err
			}
			doc["version"] = 1
			f.items[doc["title"].(string)] = doc
			return json.Marshal(doc)
		case "edit":
			var doc map[string]any
			if err := json.Unmarshal(stdin, &doc); err != nil {
				return nil, err
			}
			v, _ := strconv.Atoi(fmt.Sprint(doc["version"]))
			doc["version"] = v + 1
			f.items[args[2]] = doc
			return json.Marshal(doc)
		case "list":
			return f.listOut, nil
		}
	}
	return nil, fmt.Errorf("unexpected %v", args)
}

func newStore(f *fakeOp) *Store { return New(vault.Config{Provider: "op"}, f.exec) }

func TestNameAndDefaultVault(t *testing.T) {
	s := newStore(newFake())
	if s.Name() != "op" || s.vault != "Private" {
		t.Fatal(s.Name(), s.vault)
	}
}

func TestReadyMapsTheVendorErrors(t *testing.T) {
	f := newFake()
	f.missing = true
	if err := newStore(f).Ready(context.Background()); !errors.Is(err, vault.ErrNotInstalled) {
		t.Errorf("missing: %v", err)
	}
	f = newFake()
	f.signedIn = false
	if err := newStore(f).Ready(context.Background()); !errors.Is(err, vault.ErrSignedOut) {
		t.Errorf("signed out: %v", err)
	}
	for _, text := range []string{"You are not currently signed in", "no account found for filter", "account is not signed in", "NOT CURRENTLY SIGNED IN"} {
		s := New(vault.Config{}, func(context.Context, string, []byte, ...string) ([]byte, error) { return nil, errors.New(text) })
		if err := s.Ready(context.Background()); !errors.Is(err, vault.ErrSignedOut) {
			t.Errorf("%q: %v", text, err)
		}
	}
	f = newFake()
	if err := newStore(f).Ready(context.Background()); err != nil {
		t.Errorf("ready: %v", err)
	}
	if want := []string{"op", "whoami"}; !reflect.DeepEqual(f.calls[0].argv, want) {
		t.Errorf("argv = %v", f.calls[0].argv)
	}
}

func TestReadyOtherFailureIsNotSignedOut(t *testing.T) {
	boom := errors.New("network down")
	s := New(vault.Config{}, func(context.Context, string, []byte, ...string) ([]byte, error) { return nil, boom })
	err := s.Ready(context.Background())
	if !errors.Is(err, boom) || errors.Is(err, vault.ErrSignedOut) {
		t.Fatalf("err = %v", err)
	}
}

func TestReadyReturnsContextErrorNotSignedOut(t *testing.T) {
	f := newFake()
	f.signedIn = false
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := newStore(f).Ready(ctx)
	if !errors.Is(err, context.Canceled) || errors.Is(err, vault.ErrSignedOut) {
		t.Fatalf("err = %v", err)
	}
}

func TestGetReturnsPayloadAndVersion(t *testing.T) {
	f := newFake()
	f.seed(t, "fleet:roster")
	b, err := newStore(f).Get(context.Background(), "roster")
	if err != nil {
		t.Fatal(err)
	}
	if string(b.Data) != marker || b.Version != "3" {
		t.Fatalf("blob = %q %q", []byte(b.Data), b.Version)
	}
	want := []string{"op", "item", "get", "fleet:roster", "--vault", "Private", "--format", "json", "--reveal"}
	if !reflect.DeepEqual(f.calls[0].argv, want) {
		t.Errorf("argv = %v", f.calls[0].argv)
	}
}

func TestGetMissingIsErrNotFound(t *testing.T) {
	_, err := newStore(newFake()).Get(context.Background(), "roster")
	if !errors.Is(err, vault.ErrNotFound) {
		t.Fatal(err)
	}
	s := New(vault.Config{}, func(context.Context, string, []byte, ...string) ([]byte, error) {
		return nil, errors.New("[ERROR] x isn't an item")
	})
	if _, err := s.Get(context.Background(), "x"); !errors.Is(err, vault.ErrNotFound) {
		t.Fatal(err)
	}
}

func TestGetSignedOutMaps(t *testing.T) {
	f := newFake()
	f.signedIn = false
	if _, err := newStore(f).Get(context.Background(), "roster"); !errors.Is(err, vault.ErrSignedOut) {
		t.Fatal(err)
	}
}

func TestGetWithoutPayloadFieldIsAnErrorNotNotFound(t *testing.T) {
	f := newFake()
	f.seed(t, "fleet:roster")
	f.items["fleet:roster"]["fields"] = []any{}
	_, err := newStore(f).Get(context.Background(), "roster")
	if err == nil || errors.Is(err, vault.ErrNotFound) || !strings.Contains(err.Error(), "payload") {
		t.Fatal(err)
	}
}

func TestGetBadJSONNeverEchoesStdout(t *testing.T) {
	for _, out := range []string{"not json " + marker, `{"title":"x","version":"abc","fields":[]}`, `{"version":1,"fields":"` + marker + `"}`} {
		s := New(vault.Config{}, func(context.Context, string, []byte, ...string) ([]byte, error) { return []byte(out), nil })
		_, err := s.Get(context.Background(), "roster")
		if err == nil || strings.Contains(err.Error(), marker) {
			t.Fatalf("out=%q err=%v", out, err)
		}
	}
}

func TestPutCreateSendsThePayloadOnStdinOnly(t *testing.T) {
	f := newFake()
	payload := vault.Secret(`{"private":"` + marker + `"}`)
	v, err := newStore(f).Put(context.Background(), "roster", payload, "")
	if err != nil {
		t.Fatal(err)
	}
	if v != "1" {
		t.Errorf("version = %s", v)
	}
	var create *call
	for i := range f.calls {
		joined := strings.Join(f.calls[i].argv, " ")
		if strings.Contains(joined, marker) || strings.Contains(joined, base64.StdEncoding.EncodeToString(payload)) {
			t.Fatalf("payload leaked into argv: %s", joined)
		}
		if len(f.calls[i].argv) > 2 && f.calls[i].argv[2] == "create" {
			create = &f.calls[i]
		}
	}
	if create == nil {
		t.Fatal("no create call")
	}
	want := []string{"op", "item", "create", "--vault", "Private", "--format", "json", "-"}
	if !reflect.DeepEqual(create.argv, want) {
		t.Errorf("argv = %v", create.argv)
	}
	var tmpl struct {
		Title    string   `json:"title"`
		Category string   `json:"category"`
		Tags     []string `json:"tags"`
		Fields   []struct{ Label, Type, Value string }
	}
	if err := json.Unmarshal(create.stdin, &tmpl); err != nil {
		t.Fatal(err)
	}
	if tmpl.Title != "fleet:roster" || tmpl.Category != "SECURE_NOTE" || !reflect.DeepEqual(tmpl.Tags, []string{"fleet"}) ||
		len(tmpl.Fields) != 1 || tmpl.Fields[0].Label != "payload" || tmpl.Fields[0].Type != "CONCEALED" || tmpl.Fields[0].Value != string(payload) {
		t.Errorf("template = %s", create.stdin)
	}
}

func TestPutCreateOverExistingIsConflict(t *testing.T) {
	f := newFake()
	f.seed(t, "fleet:roster")
	_, err := newStore(f).Put(context.Background(), "roster", vault.Secret("new"), "")
	if !errors.Is(err, vault.ErrConflict) || f.writes() != 0 {
		t.Fatalf("err=%v writes=%d", err, f.writes())
	}
}

func TestPutUpdateChecksTheVersionFirst(t *testing.T) {
	f := newFake()
	f.seed(t, "fleet:roster") // version 3
	_, err := newStore(f).Put(context.Background(), "roster", vault.Secret("mine"), "2")
	if !errors.Is(err, vault.ErrConflict) {
		t.Fatalf("err = %v", err)
	}
	if f.writes() != 0 {
		t.Fatal("edit was called despite the conflict")
	}
}

func TestPutUpdateSendsTheFullItemOnStdin(t *testing.T) {
	f := newFake()
	f.seed(t, "fleet:roster")
	newPayload := vault.Secret("NEW-" + marker)
	v, err := newStore(f).Put(context.Background(), "roster", newPayload, "3")
	if err != nil || v != "4" {
		t.Fatalf("v=%s err=%v", v, err)
	}
	var edit *call
	for i := range f.calls {
		if len(f.calls[i].argv) > 2 && f.calls[i].argv[2] == "edit" {
			edit = &f.calls[i]
		}
		if strings.Contains(strings.Join(f.calls[i].argv, " "), "NEW-") {
			t.Fatal("payload in argv")
		}
	}
	want := []string{"op", "item", "edit", "fleet:roster", "--vault", "Private", "--format", "json"}
	if edit == nil || !reflect.DeepEqual(edit.argv, want) {
		t.Fatalf("edit = %+v", edit)
	}
	var doc map[string]any
	if err := json.Unmarshal(edit.stdin, &doc); err != nil {
		t.Fatal(err)
	}
	if payloadOf(doc) != string(newPayload) || doc["id"] != "abcdefghijklmnopqrstuvwxyz" || len(doc["fields"].([]any)) != 2 {
		t.Errorf("edit doc = %s", edit.stdin)
	}
}

func TestPutUpdateOfMissingItemIsNotFound(t *testing.T) {
	f := newFake()
	_, err := newStore(f).Put(context.Background(), "roster", vault.Secret("x"), "3")
	if !errors.Is(err, vault.ErrNotFound) || f.writes() != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestPutSignedOutWritesNothing(t *testing.T) {
	for _, ver := range []string{"", "3"} {
		f := newFake()
		f.signedIn = false
		_, err := newStore(f).Put(context.Background(), "roster", vault.Secret(marker), ver)
		if !errors.Is(err, vault.ErrSignedOut) || f.writes() != 0 {
			t.Fatalf("ver=%q err=%v writes=%d", ver, err, f.writes())
		}
		if len(f.calls) != 1 {
			t.Fatalf("calls after Ready: %d", len(f.calls))
		}
	}
}

func TestPutRefusesAForeignOrNormalisedReRead(t *testing.T) {
	f := newFake()
	f.afterWrite = func() ([]byte, error) { return fixture(t, "item.json"), nil } // someone else's payload, version 3
	v, err := newStore(f).Put(context.Background(), "roster", vault.Secret("mine"), "")
	if !errors.Is(err, vault.ErrConflict) || v != "" || !strings.Contains(err.Error(), "changed by another writer or normalised after write") {
		t.Fatalf("v=%q err=%v", v, err)
	}
}

func TestPutReportsWriteLandedWhenReReadFails(t *testing.T) {
	f := newFake()
	boom := errors.New("network down")
	f.afterWrite = func() ([]byte, error) { return nil, boom }
	v, err := newStore(f).Put(context.Background(), "roster", vault.Secret("mine"), "")
	if v != "" || !errors.Is(err, boom) || !strings.Contains(err.Error(), "written but re-read failed") {
		t.Fatalf("v=%q err=%v", v, err)
	}
}

func TestAmbiguousNameIsAnErrorAndNeverWritten(t *testing.T) {
	f := newFake()
	f.dupes["fleet:roster"] = true
	s := newStore(f)
	if _, err := s.Get(context.Background(), "roster"); err == nil || !strings.Contains(err.Error(), "fleet:roster") || errors.Is(err, vault.ErrNotFound) {
		t.Fatalf("get err = %v", err)
	}
	for _, ver := range []string{"", "3"} {
		if _, err := s.Put(context.Background(), "roster", vault.Secret("y"), ver); err == nil || errors.Is(err, vault.ErrNotFound) || f.writes() != 0 {
			t.Fatalf("ver=%q err=%v writes=%d", ver, err, f.writes())
		}
	}
}

func TestPutRejectsNonUTF8(t *testing.T) {
	f := newFake()
	if _, err := newStore(f).Put(context.Background(), "roster", vault.Secret{0xff, 0xfe}, ""); err == nil || f.writes() != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestPutThenGetRoundTripsBytesAndVersion(t *testing.T) {
	f := newFake()
	s := newStore(f)
	payload := "{\"k\":\"" + marker + "\"}\n\"quoted\" \\ <&>"
	v, err := s.Put(context.Background(), "roster", vault.Secret(payload), "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Get(context.Background(), "roster")
	if err != nil || string(b.Data) != payload || b.Version != v {
		t.Fatalf("data=%q version %s vs %s err=%v", []byte(b.Data), b.Version, v, err)
	}
	v2, err := s.Put(context.Background(), "roster", vault.Secret("two"), v)
	if err != nil || v2 == v {
		t.Fatalf("v2=%s err=%v", v2, err)
	}
}

func TestListFiltersToFleetTitles(t *testing.T) {
	f := newFake()
	f.listOut = fixture(t, "list.json")
	names, err := newStore(f).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"roster", "key/mac1"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("names = %v", names)
	}
	want := []string{"op", "item", "list", "--vault", "Private", "--tags", "fleet", "--format", "json"}
	if !reflect.DeepEqual(f.calls[0].argv, want) {
		t.Errorf("argv = %v", f.calls[0].argv)
	}
}

func TestListErrors(t *testing.T) {
	f := newFake()
	f.listOut = []byte("garbage")
	if _, err := newStore(f).List(context.Background()); err == nil {
		t.Fatal("want parse error")
	}
	f = newFake()
	f.signedIn = false
	if _, err := newStore(f).List(context.Background()); !errors.Is(err, vault.ErrSignedOut) {
		t.Fatal(err)
	}
}

func TestCustomVault(t *testing.T) {
	f := newFake()
	s := New(vault.Config{Provider: "op", Vault: "Team"}, f.exec)
	f.seed(t, "fleet:roster")
	if _, err := s.Get(context.Background(), "roster"); err != nil {
		t.Fatal(err)
	}
	if f.calls[0].argv[5] != "Team" {
		t.Fatal(f.calls[0].argv)
	}
}

func TestSignInRunsOpSignin(t *testing.T) {
	var got []string
	s := newStore(newFake())
	s.Interactive = func(_ context.Context, _ io.Reader, _, _ io.Writer, name string, args ...string) error {
		got = append([]string{name}, args...)
		return nil
	}
	if err := s.SignIn(context.Background(), nil, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if want := []string{"op", "signin"}; !reflect.DeepEqual(got, want) {
		t.Errorf("argv = %v", got)
	}
}

func TestRunInteractiveWiresStdio(t *testing.T) {
	var out bytes.Buffer
	if err := runInteractive(context.Background(), strings.NewReader("hi"), &out, io.Discard, "cat"); err != nil || out.String() != "hi" {
		t.Fatalf("out=%q err=%v", out.String(), err)
	}
}

func TestOtherExecErrorsAreWrappedNotMisclassified(t *testing.T) {
	boom := errors.New("network down")
	s := New(vault.Config{}, func(context.Context, string, []byte, ...string) ([]byte, error) { return nil, boom })
	_, err := s.Get(context.Background(), "roster")
	if !errors.Is(err, boom) || errors.Is(err, vault.ErrNotFound) || errors.Is(err, vault.ErrSignedOut) {
		t.Fatalf("err = %v", err)
	}
}
