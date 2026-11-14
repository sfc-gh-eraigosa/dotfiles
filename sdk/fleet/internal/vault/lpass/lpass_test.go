package lpass

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault"
)

const marker = "PRIVATE-MATERIAL-123"

type call struct {
	argv  []string
	stdin []byte
}

// fakeLpass replays lpass output keyed on the subcommand + item.
type fakeLpass struct {
	calls    []call
	loggedIn bool
	notes    map[string]string // full item name -> notes
	lsOut    string
	missing  bool // binary not installed
}

func newFake() *fakeLpass { return &fakeLpass{loggedIn: true, notes: map[string]string{}} }

func (f *fakeLpass) exec(_ context.Context, name string, stdin []byte, args ...string) ([]byte, error) {
	f.calls = append(f.calls, call{append([]string{name}, args...), stdin})
	if f.missing {
		return nil, vault.ErrNotInstalled
	}
	switch args[0] {
	case "status":
		if !f.loggedIn {
			return nil, errors.New("lpass: exit status 1: Not logged in.")
		}
		return nil, nil
	case "show":
		if !f.loggedIn {
			return nil, errors.New("lpass: exit status 1: Error: Could not find decryption key. Perhaps you need to login with `lpass login`.")
		}
		item := args[len(args)-1]
		n, ok := f.notes[item]
		if !ok {
			return nil, errors.New("lpass: exit status 1: Error: Could not find specified account(s).")
		}
		return []byte(n + "\n"), nil // real lpass appends one newline
	case "add", "edit":
		item := args[len(args)-1]
		f.notes[item] = string(stdin)
		return nil, nil
	case "ls":
		return []byte(f.lsOut), nil
	}
	return nil, fmt.Errorf("unexpected %v", args)
}

func (f *fakeLpass) writes() int {
	n := 0
	for _, c := range f.calls {
		if c.argv[1] == "add" || c.argv[1] == "edit" {
			n++
		}
	}
	return n
}

func sum(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

func newStore(f *fakeLpass) *Store {
	return New(vault.Config{Provider: "lpass"}, f.exec)
}

func TestNameAndDefaults(t *testing.T) {
	s := newStore(newFake())
	if s.Name() != "lpass" {
		t.Fatal(s.Name())
	}
}

func TestReadyMapsTheVendorErrors(t *testing.T) {
	f := newFake()
	f.missing = true
	if err := newStore(f).Ready(context.Background()); !errors.Is(err, vault.ErrNotInstalled) {
		t.Errorf("missing: %v", err)
	}
	f = newFake()
	f.loggedIn = false
	if err := newStore(f).Ready(context.Background()); !errors.Is(err, vault.ErrSignedOut) {
		t.Errorf("signed out: %v", err)
	}
	f = newFake()
	if err := newStore(f).Ready(context.Background()); err != nil {
		t.Errorf("ready: %v", err)
	}
	want := []string{"lpass", "status", "-q"}
	if !reflect.DeepEqual(f.calls[0].argv, want) {
		t.Errorf("argv = %v", f.calls[0].argv)
	}
}

func TestGetReturnsPayloadAndVersion(t *testing.T) {
	f := newFake()
	f.notes["fleet/fleet:roster"] = marker
	b, err := newStore(f).Get(context.Background(), "roster")
	if err != nil {
		t.Fatal(err)
	}
	if string(b.Data) != marker || b.Version != sum(marker) {
		t.Fatalf("blob = %q %q", []byte(b.Data), b.Version)
	}
	want := []string{"lpass", "show", "--sync=now", "--notes", "fleet/fleet:roster"}
	if !reflect.DeepEqual(f.calls[0].argv, want) {
		t.Errorf("argv = %v", f.calls[0].argv)
	}
}

func TestGetMissingIsErrNotFound(t *testing.T) {
	_, err := newStore(newFake()).Get(context.Background(), "roster")
	if !errors.Is(err, vault.ErrNotFound) {
		t.Fatal(err)
	}
}

func TestGetSignedOutMapsDecryptionKeyError(t *testing.T) {
	f := newFake()
	f.loggedIn = false
	_, err := newStore(f).Get(context.Background(), "roster")
	if !errors.Is(err, vault.ErrSignedOut) {
		t.Fatal(err)
	}
}

func TestLogicalNameWithSlashStaysInsideTheFolder(t *testing.T) {
	f := newFake()
	s := newStore(f)
	if _, err := s.Put(context.Background(), "key/mac1", vault.Secret(marker), ""); err != nil {
		t.Fatal(err)
	}
	item := f.calls[len(f.calls)-1].argv
	if got := item[len(item)-1]; got != "fleet/fleet:key%2Fmac1" {
		t.Fatalf("item = %q", got)
	}
	f.lsOut = "fleet/fleet:key%2Fmac1 [id: 1]\n"
	names, err := s.List(context.Background())
	if err != nil || !reflect.DeepEqual(names, []string{"key/mac1"}) {
		t.Fatalf("names = %v %v", names, err)
	}
}

func TestPutCreateSendsThePayloadOnStdinOnly(t *testing.T) {
	f := newFake()
	payload := vault.Secret(`{"private":"` + marker + `"}`)
	v, err := newStore(f).Put(context.Background(), "roster", payload, "")
	if err != nil {
		t.Fatal(err)
	}
	if v != sum(string(payload)) {
		t.Errorf("version = %s", v)
	}
	var add *call
	for i := range f.calls {
		joined := strings.Join(f.calls[i].argv, " ")
		if strings.Contains(joined, marker) || strings.Contains(joined, base64.StdEncoding.EncodeToString(payload)) {
			t.Fatalf("payload leaked into argv: %s", joined)
		}
		if f.calls[i].argv[1] == "add" {
			add = &f.calls[i]
		}
	}
	if add == nil {
		t.Fatal("no add call")
	}
	want := []string{"lpass", "add", "--sync=now", "--non-interactive", "--notes", "fleet/fleet:roster"}
	if !reflect.DeepEqual(add.argv, want) {
		t.Errorf("argv = %v", add.argv)
	}
	if !bytes.Equal(add.stdin, payload) {
		t.Errorf("stdin = %q", add.stdin)
	}
}

func TestPutCreateOverExistingIsConflict(t *testing.T) {
	f := newFake()
	f.notes["fleet/fleet:roster"] = "old"
	_, err := newStore(f).Put(context.Background(), "roster", vault.Secret("new"), "")
	if !errors.Is(err, vault.ErrConflict) || f.writes() != 0 {
		t.Fatalf("err=%v writes=%d", err, f.writes())
	}
}

func TestPutUpdateSendsEditWithStdin(t *testing.T) {
	f := newFake()
	f.notes["fleet/fleet:roster"] = "old"
	v, err := newStore(f).Put(context.Background(), "roster", vault.Secret(marker), sum("old"))
	if err != nil || v != sum(marker) {
		t.Fatalf("v=%s err=%v", v, err)
	}
	last := f.calls[len(f.calls)-2] // edit, then the authoritative re-read
	want := []string{"lpass", "edit", "--sync=now", "--non-interactive", "--notes", "fleet/fleet:roster"}
	if !reflect.DeepEqual(last.argv, want) || string(last.stdin) != marker {
		t.Errorf("last = %v %q", last.argv, last.stdin)
	}
}

func TestPutDetectsAChangedNoteBeforeEditing(t *testing.T) {
	f := newFake()
	f.notes["fleet/fleet:roster"] = "changed by someone else"
	_, err := newStore(f).Put(context.Background(), "roster", vault.Secret("mine"), sum("what I read"))
	if !errors.Is(err, vault.ErrConflict) {
		t.Fatalf("err = %v", err)
	}
	if f.writes() != 0 {
		t.Fatal("edit was called despite the conflict")
	}
}

func TestPutUpdateOfMissingItemIsNotFound(t *testing.T) {
	f := newFake()
	_, err := newStore(f).Put(context.Background(), "roster", vault.Secret("x"), sum("old"))
	if !errors.Is(err, vault.ErrNotFound) || f.writes() != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestPutSignedOutWritesNothing(t *testing.T) {
	for _, ver := range []string{"", sum("old")} {
		f := newFake()
		f.loggedIn = false
		_, err := newStore(f).Put(context.Background(), "roster", vault.Secret(marker), ver)
		if !errors.Is(err, vault.ErrSignedOut) || f.writes() != 0 {
			t.Fatalf("ver=%q err=%v writes=%d", ver, err, f.writes())
		}
	}
}

func TestListFiltersToThisFleetFolder(t *testing.T) {
	f := newFake()
	f.lsOut = "fleet/fleet:roster [id: 11]\nfleet/fleet:key%2Fmac1 [id: 12]\nfleet/other-note [id: 13]\n  fleet/fleet:inventory [id: 14]\n\n"
	names, err := newStore(f).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"roster", "key/mac1", "inventory"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("names = %v", names)
	}
	argv := []string{"lpass", "ls", "--sync=now", "fleet"}
	if !reflect.DeepEqual(f.calls[0].argv, argv) {
		t.Errorf("argv = %v", f.calls[0].argv)
	}
}

func TestCustomFolder(t *testing.T) {
	f := newFake()
	s := New(vault.Config{Provider: "lpass", Vault: "team"}, f.exec)
	f.notes["team/fleet:roster"] = "x"
	if _, err := s.Get(context.Background(), "roster"); err != nil {
		t.Fatal(err)
	}
}

func TestSignInUsesLPASSUserAndTrust(t *testing.T) {
	t.Setenv("LPASS_USER", "me@example.com")
	var got []string
	s := newStore(newFake())
	s.Interactive = func(_ context.Context, _ io.Reader, _, _ io.Writer, name string, args ...string) error {
		got = append([]string{name}, args...)
		return nil
	}
	if err := s.SignIn(context.Background(), nil, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	want := []string{"lpass", "login", "--trust", "me@example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("argv = %v", got)
	}
}

func TestSignInWithoutUserTellsTheOperator(t *testing.T) {
	os.Unsetenv("LPASS_USER")
	s := newStore(newFake())
	s.Interactive = func(context.Context, io.Reader, io.Writer, io.Writer, string, ...string) error {
		t.Fatal("must not run")
		return nil
	}
	err := s.SignIn(context.Background(), nil, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "lpass login --trust") {
		t.Fatalf("err = %v", err)
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

func TestPutThenGetRoundTripsBytesAndVersion(t *testing.T) {
	f := newFake()
	s := newStore(f)
	payload := "{\"k\":\"" + marker + "\"}\n" // payload itself ends in a newline
	v, err := s.Put(context.Background(), "roster", vault.Secret(payload), "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Get(context.Background(), "roster")
	if err != nil {
		t.Fatal(err)
	}
	if string(b.Data) != payload || b.Version != v {
		t.Fatalf("data=%q version %s vs Put %s", []byte(b.Data), b.Version, v)
	}
}

func TestPutWithTheReturnedVersionSucceeds(t *testing.T) {
	f := newFake()
	s := newStore(f)
	v1, err := s.Put(context.Background(), "roster", vault.Secret("one"), "")
	if err != nil {
		t.Fatal(err)
	}
	v2, err := s.Put(context.Background(), "roster", vault.Secret("two"), v1)
	if err != nil {
		t.Fatalf("false conflict: %v", err)
	}
	if v2 == v1 || f.notes["fleet/fleet:roster"] != "two" {
		t.Fatalf("v2=%s", v2)
	}
}
