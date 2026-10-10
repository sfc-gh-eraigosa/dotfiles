package hostkey

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// call is one local command the package asked to run.
type call struct {
	name  string
	args  []string
	stdin string
}

// fake answers by command name + first arg, and records every call so a test
// can assert what would have touched known_hosts.
type fake struct {
	calls   []call
	answers map[string]func(c call) (string, error)
}

func (f *fake) exec(name, stdin string, args ...string) (string, error) {
	c := call{name: name, args: args, stdin: stdin}
	f.calls = append(f.calls, c)
	key := name
	if len(args) > 0 {
		key += " " + args[0]
	}
	if fn, ok := f.answers[key]; ok {
		return fn(c)
	}
	return "", nil
}

func TestResolveReadsHostPortFileAndHashingFromSSHG(t *testing.T) {
	f := &fake{answers: map[string]func(call) (string, error){
		"ssh -G": func(call) (string, error) {
			return "user me\nhostname 192.168.0.210\nport 2222\nhashknownhosts yes\n" +
				"userknownhostsfile /h/.ssh/known_hosts /h/.ssh/known_hosts2\n", nil
		},
	}}
	got, err := Resolve(f.exec, "gig")
	if err != nil {
		t.Fatal(err)
	}
	want := Target{Alias: "gig", HostName: "192.168.0.210", Port: "2222", KnownHosts: "/h/.ssh/known_hosts", Hash: true}
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestNamesCoverTheBracketedFormOnlyOffPort22(t *testing.T) {
	if got := (Target{HostName: "10.0.0.1", Port: "22"}).Names(); !reflect.DeepEqual(got, []string{"10.0.0.1"}) {
		t.Fatalf("port 22: %v", got)
	}
	if got := (Target{HostName: "10.0.0.1", Port: "2222"}).Names(); !reflect.DeepEqual(got, []string{"[10.0.0.1]:2222"}) {
		t.Fatalf("port 2222: %v", got)
	}
}

func TestParseFingerprintsReadsKeygenListing(t *testing.T) {
	got := parseFingerprints("256 SHA256:abc 10.0.0.1 (ED25519)\n3072 SHA256:def 10.0.0.1 (RSA)\n\n")
	want := []Key{{Type: "ED25519", Fingerprint: "SHA256:abc"}, {Type: "RSA", Fingerprint: "SHA256:def"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestKnownFingerprintsOnlyTheMatchingKeyLines(t *testing.T) {
	f := &fake{answers: map[string]func(call) (string, error){
		"ssh-keygen -F": func(call) (string, error) {
			return "# Host 10.0.0.1 found: line 25\n|1|xx|yy ecdsa-sha2-nistp256 AAAA\n", nil
		},
		"ssh-keygen -lf": func(c call) (string, error) {
			if strings.Contains(c.stdin, "# Host") {
				t.Errorf("comment line leaked into the fingerprint input: %q", c.stdin)
			}
			return "256 SHA256:old 10.0.0.1 (ECDSA)\n", nil
		},
	}}
	got, err := Known(f.exec, Target{HostName: "10.0.0.1", Port: "22", KnownHosts: "/kh"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []Key{{Type: "ECDSA", Fingerprint: "SHA256:old"}}) {
		t.Fatalf("got %v", got)
	}
}

func TestKnownTreatsNotFoundAsNoKeys(t *testing.T) {
	// ssh-keygen -F exits 1 with no output when the host is absent.
	f := &fake{answers: map[string]func(call) (string, error){
		"ssh-keygen -F": func(call) (string, error) { return "", errors.New("exit status 1") },
	}}
	got, err := Known(f.exec, Target{HostName: "10.0.0.1", Port: "22", KnownHosts: "/kh"})
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestScanHashesWhenTheClientHashes(t *testing.T) {
	f := &fake{answers: map[string]func(call) (string, error){
		"ssh-keyscan -H": func(call) (string, error) { return "|1|a|b ssh-ed25519 AAAA\n", nil },
		"ssh-keygen -lf": func(call) (string, error) { return "256 SHA256:new 10.0.0.1 (ED25519)\n", nil },
	}}
	raw, keys, err := Scan(f.exec, Target{HostName: "10.0.0.1", Port: "22", Hash: true})
	if err != nil {
		t.Fatal(err)
	}
	if raw != "|1|a|b ssh-ed25519 AAAA\n" || len(keys) != 1 || keys[0].Fingerprint != "SHA256:new" {
		t.Fatalf("raw %q keys %v", raw, keys)
	}
}

func TestScanWithNoAnswerIsAnError(t *testing.T) {
	f := &fake{}
	if _, _, err := Scan(f.exec, Target{HostName: "10.0.0.1", Port: "22"}); err == nil {
		t.Fatal("an empty keyscan must not look like a host with no keys")
	}
}

func TestCompare(t *testing.T) {
	a := Key{Type: "ED25519", Fingerprint: "SHA256:a"}
	b := Key{Type: "ED25519", Fingerprint: "SHA256:b"}
	cases := []struct {
		stored, presented []Key
		want              Verdict
	}{
		{nil, []Key{a}, New},
		{[]Key{a}, []Key{a}, Same},
		{[]Key{b}, []Key{a}, Changed},
	}
	for _, c := range cases {
		if got := Compare(c.stored, c.presented); got != c.want {
			t.Errorf("Compare(%v,%v)=%v want %v", c.stored, c.presented, got, c.want)
		}
	}
}

func TestHasFingerprint(t *testing.T) {
	keys := []Key{{Type: "ED25519", Fingerprint: "SHA256:a"}}
	if !HasFingerprint(keys, "SHA256:a") || HasFingerprint(keys, "SHA256:b") || HasFingerprint(keys, "") {
		t.Fatal("HasFingerprint must match exactly and never match empty")
	}
}

func TestReplaceBacksUpRemovesThenAppends(t *testing.T) {
	dir := t.TempDir()
	kh := filepath.Join(dir, "known_hosts")
	if err := os.WriteFile(kh, []byte("old-line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := &fake{}
	backup, err := Replace(f.exec, Target{HostName: "10.0.0.1", Port: "22", KnownHosts: kh}, "new-line\n", "20261010T000000Z")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(backup); string(b) != "old-line\n" {
		t.Fatalf("backup %s holds %q", backup, b)
	}
	if !strings.HasPrefix(filepath.Base(backup), "known_hosts.fleet-bak-") {
		t.Fatalf("backup name %s", backup)
	}
	if len(f.calls) != 1 || !reflect.DeepEqual(f.calls[0].args, []string{"-R", "10.0.0.1", "-f", kh}) {
		t.Fatalf("calls %+v", f.calls)
	}
	if b, _ := os.ReadFile(kh); !strings.HasSuffix(string(b), "new-line\n") {
		t.Fatalf("known_hosts now %q", b)
	}
}

func TestReplaceStopsBeforeAppendingWhenRemovalFails(t *testing.T) {
	dir := t.TempDir()
	kh := filepath.Join(dir, "known_hosts")
	_ = os.WriteFile(kh, []byte("old-line\n"), 0o600)
	f := &fake{answers: map[string]func(call) (string, error){
		"ssh-keygen -R": func(call) (string, error) { return "", errors.New("boom") },
	}}
	if _, err := Replace(f.exec, Target{HostName: "10.0.0.1", Port: "22", KnownHosts: kh}, "new-line\n", "ts"); err == nil {
		t.Fatal("want error")
	}
	if b, _ := os.ReadFile(kh); strings.Contains(string(b), "new-line") {
		t.Fatal("must not append a new key beside an old one it failed to remove")
	}
}
