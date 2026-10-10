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

func (f *fake) count(name, first string) int {
	n := 0
	for _, c := range f.calls {
		if c.name == name && len(c.args) > 0 && c.args[0] == first {
			n++
		}
	}
	return n
}

// fpByBlob fingerprints a key line by the blob it carries, the way
// ssh-keygen -lf does: the host field does not change the fingerprint.
func fpByBlob(c call) (string, error) {
	switch {
	case strings.Contains(c.stdin, "NEWED"):
		return "256 SHA256:newed x (ED25519)\n", nil
	case strings.Contains(c.stdin, "NEWRSA"):
		return "3072 SHA256:newrsa x (RSA)\n", nil
	case strings.Contains(c.stdin, "OLD"):
		return "256 SHA256:old x (ECDSA)\n", nil
	}
	return "", errors.New("unknown key")
}

func sshG(extra string) func(call) (string, error) {
	return func(call) (string, error) {
		return "user me\nhostname 192.168.0.210\nport 22\nhashknownhosts no\ncheckhostip no\n" +
			"userknownhostsfile /h/.ssh/known_hosts /h/.ssh/known_hosts2\n" + extra, nil
	}
}

func TestResolveReadsEveryUserKnownHostsFile(t *testing.T) {
	f := &fake{answers: map[string]func(call) (string, error){"ssh -G": sshG("")}}
	got, err := Resolve(f.exec, "gig")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.KnownHosts, []string{"/h/.ssh/known_hosts", "/h/.ssh/known_hosts2"}) {
		t.Fatalf("KnownHosts = %v: a stale key in the second file still fails ssh", got.KnownHosts)
	}
	if !reflect.DeepEqual(got.Names(), []string{"192.168.0.210"}) {
		t.Fatalf("Names = %v", got.Names())
	}
}

// ssh looks a host up — and stores it — under HostKeyAlias when one is set,
// unbracketed even off port 22. Working on the HostName would leave the stale
// key exactly where ssh keeps finding it.
func TestResolveHonoursHostKeyAlias(t *testing.T) {
	f := &fake{answers: map[string]func(call) (string, error){"ssh -G": func(call) (string, error) {
		return "hostname 10.0.0.9\nport 2222\nhostkeyalias jump-prod\nuserknownhostsfile /kh\n", nil
	}}}
	got, err := Resolve(f.exec, "gig")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Names(), []string{"jump-prod"}) {
		t.Fatalf("Names = %v", got.Names())
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

// With CheckHostIP on and a DNS HostName, ssh also checks (and records) the
// key under the resolved address; a stale IP entry must go with the name's.
func TestCheckHostIPAddsTheResolvedAddresses(t *testing.T) {
	restore := lookupHost
	lookupHost = func(h string) ([]string, error) {
		if h != "nas.local" {
			t.Fatalf("resolved %q", h)
		}
		return []string{"10.0.0.7"}, nil
	}
	t.Cleanup(func() { lookupHost = restore })
	f := &fake{answers: map[string]func(call) (string, error){"ssh -G": func(call) (string, error) {
		return "hostname nas.local\nport 22\ncheckhostip yes\nuserknownhostsfile /kh\n", nil
	}}}
	got, err := Resolve(f.exec, "nas")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Names(), []string{"nas.local", "10.0.0.7"}) {
		t.Fatalf("Names = %v", got.Names())
	}
}

func TestParseFingerprintsReadsKeygenListing(t *testing.T) {
	got := parseFingerprints("256 SHA256:abc 10.0.0.1 (ED25519)\n3072 SHA256:def 10.0.0.1 (RSA)\n\n")
	want := []Key{{Type: "ED25519", Fingerprint: "SHA256:abc"}, {Type: "RSA", Fingerprint: "SHA256:def"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestKnownSearchesEveryExistingFileAndName(t *testing.T) {
	dir := t.TempDir()
	kh, kh2 := filepath.Join(dir, "known_hosts"), filepath.Join(dir, "known_hosts2")
	_ = os.WriteFile(kh, nil, 0o600)
	_ = os.WriteFile(kh2, nil, 0o600)
	f := &fake{answers: map[string]func(call) (string, error){
		"ssh-keygen -F": func(c call) (string, error) {
			if c.args[3] == kh2 {
				return "# Host 10.0.0.1 found: line 3\n|1|xx|yy ecdsa-sha2-nistp256 OLD\n", nil
			}
			return "", errors.New("exit status 1")
		},
		"ssh-keygen -lf": fpByBlob,
	}}
	got, err := Known(f.exec, Target{HostName: "10.0.0.1", Port: "22", KnownHosts: []string{kh, kh2, filepath.Join(dir, "absent")}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []Key{{Type: "ECDSA", Fingerprint: "SHA256:old"}}) {
		t.Fatalf("got %v", got)
	}
	if n := f.count("ssh-keygen", "-F"); n != 2 {
		t.Fatalf("searched %d files, want the 2 that exist", n)
	}
}

func TestScanFingerprintsEachKeyAndKeepsItsBlob(t *testing.T) {
	f := &fake{answers: map[string]func(call) (string, error){
		"ssh-keyscan -p": func(c call) (string, error) {
			return "[10.0.0.1]:2222 ssh-ed25519 NEWED\n[10.0.0.1]:2222 ssh-rsa NEWRSA\n", nil
		},
		"ssh-keygen -lf": fpByBlob,
	}}
	got, err := Scan(f.exec, Target{HostName: "10.0.0.1", Port: "2222"})
	if err != nil {
		t.Fatal(err)
	}
	want := []Presented{
		{Key: Key{Type: "ED25519", Fingerprint: "SHA256:newed"}, Blob: "ssh-ed25519 NEWED"},
		{Key: Key{Type: "RSA", Fingerprint: "SHA256:newrsa"}, Blob: "ssh-rsa NEWRSA"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestScanWithNoAnswerIsAnError(t *testing.T) {
	f := &fake{}
	if _, err := Scan(f.exec, Target{HostName: "10.0.0.1", Port: "22"}); err == nil {
		t.Fatal("an empty keyscan must not look like a host with no keys")
	}
}

// The entries are written under the names ssh LOOKS UP (HostKeyAlias, every
// CheckHostIP address) — not under whatever host field keyscan printed.
func TestEntriesUseTheLookupNames(t *testing.T) {
	ps := []Presented{{Key: Key{Type: "ED25519"}, Blob: "ssh-ed25519 NEWED"}}
	got, err := Entries((&fake{}).exec, Target{KeyAlias: "jump-prod", HostName: "10.0.0.9", Port: "2222"}, ps)
	if err != nil {
		t.Fatal(err)
	}
	if got != "jump-prod ssh-ed25519 NEWED\n" {
		t.Fatalf("entries %q", got)
	}
	got, _ = Entries((&fake{}).exec, Target{HostName: "nas.local", Port: "22", IPs: []string{"10.0.0.7"}}, ps)
	if got != "nas.local ssh-ed25519 NEWED\n10.0.0.7 ssh-ed25519 NEWED\n" {
		t.Fatalf("entries %q", got)
	}
}

func TestEntriesAreHashedWhenTheClientHashes(t *testing.T) {
	f := &fake{answers: map[string]func(call) (string, error){
		"ssh-keygen -H": func(c call) (string, error) {
			p := c.args[2]
			b, _ := os.ReadFile(p)
			return "", os.WriteFile(p, []byte(strings.ReplaceAll(string(b), "10.0.0.9 ", "|1|salt|hash ")), 0o600)
		},
	}}
	got, err := Entries(f.exec, Target{HostName: "10.0.0.9", Port: "22", Hash: true},
		[]Presented{{Blob: "ssh-ed25519 NEWED"}})
	if err != nil {
		t.Fatal(err)
	}
	if got != "|1|salt|hash ssh-ed25519 NEWED\n" {
		t.Fatalf("entries %q", got)
	}
}

func TestOnlyFingerprintKeepsThePinnedKey(t *testing.T) {
	ps := []Presented{
		{Key: Key{Type: "ED25519", Fingerprint: "SHA256:a"}},
		{Key: Key{Type: "RSA", Fingerprint: "SHA256:b"}},
	}
	got := OnlyFingerprint(ps, "SHA256:a")
	if len(got) != 1 || got[0].Key.Fingerprint != "SHA256:a" {
		t.Fatalf("got %+v", got)
	}
}

func TestCompare(t *testing.T) {
	edA := Key{Type: "ED25519", Fingerprint: "SHA256:a"}
	edB := Key{Type: "ED25519", Fingerprint: "SHA256:b"}
	rsa := Key{Type: "RSA", Fingerprint: "SHA256:r"}
	ecd := Key{Type: "ECDSA", Fingerprint: "SHA256:e"}
	cases := []struct {
		name              string
		stored, presented []Key
		want              Verdict
	}{
		{"first contact", nil, []Key{edA}, New},
		{"same", []Key{edA}, []Key{edA}, Same},
		{"changed", []Key{edB}, []Key{edA}, Changed},
		// ssh prefers the key types it already knows, so an ED25519 mismatch
		// fails the connection even though the RSA key still matches — and
		// calling that "already trusted" would wave a swapped key through.
		{"one type changed, another matches", []Key{edB, rsa}, []Key{edA, rsa}, Changed},
		// Stored keys exist, none of a presented type: BatchMode ssh refuses.
		{"no common type", []Key{ecd}, []Key{edA}, Changed},
		// A presented type with no stored key is fine while every shared type matches.
		{"extra presented type", []Key{rsa}, []Key{edA, rsa}, Same},
	}
	for _, c := range cases {
		if got := Compare(c.stored, c.presented); got != c.want {
			t.Errorf("%s: Compare = %v want %v", c.name, got, c.want)
		}
	}
}

func TestHasFingerprint(t *testing.T) {
	keys := []Key{{Type: "ED25519", Fingerprint: "SHA256:a"}}
	if !HasFingerprint(keys, "SHA256:a") || HasFingerprint(keys, "SHA256:b") || HasFingerprint(keys, "") {
		t.Fatal("HasFingerprint must match exactly and never match empty")
	}
}

func TestReplaceBacksUpEveryFileRemovesFromEachThenAppendsToTheFirst(t *testing.T) {
	dir := t.TempDir()
	kh, kh2 := filepath.Join(dir, "known_hosts"), filepath.Join(dir, "known_hosts2")
	_ = os.WriteFile(kh, []byte("old-line\n"), 0o600)
	_ = os.WriteFile(kh2, []byte("old-2\n"), 0o600)
	f := &fake{}
	tg := Target{HostName: "10.0.0.1", Port: "22", KnownHosts: []string{kh, kh2}}
	backups, err := Replace(f.exec, tg, "new-line\n", "20261010T000000Z")
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 2 {
		t.Fatalf("backups %v", backups)
	}
	if b, _ := os.ReadFile(backups[1]); string(b) != "old-2\n" {
		t.Fatalf("backup %s holds %q", backups[1], b)
	}
	if !strings.HasPrefix(filepath.Base(backups[0]), "known_hosts.fleet-bak-") {
		t.Fatalf("backup name %s", backups[0])
	}
	if n := f.count("ssh-keygen", "-R"); n != 2 {
		t.Fatalf("removed from %d files, want 2", n)
	}
	if b, _ := os.ReadFile(kh); !strings.HasSuffix(string(b), "new-line\n") {
		t.Fatalf("known_hosts now %q", b)
	}
	if b, _ := os.ReadFile(kh2); strings.Contains(string(b), "new-line") {
		t.Fatal("new keys go to the first file only")
	}
}

// ssh-keygen -R exits 255 on a missing file, which made first contact on a
// fresh machine impossible.
func TestReplaceOnAFreshMachineCreatesTheFile(t *testing.T) {
	kh := filepath.Join(t.TempDir(), "ssh", "known_hosts")
	f := &fake{}
	backups, err := Replace(f.exec, Target{HostName: "10.0.0.1", Port: "22", KnownHosts: []string{kh}}, "new-line\n", "ts")
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 0 || f.count("ssh-keygen", "-R") != 0 {
		t.Fatalf("backups %v, removals %d on a file that did not exist", backups, f.count("ssh-keygen", "-R"))
	}
	if b, _ := os.ReadFile(kh); string(b) != "new-line\n" {
		t.Fatalf("known_hosts %q", b)
	}
	if st, _ := os.Stat(filepath.Dir(kh)); st.Mode().Perm() != 0o700 {
		t.Fatalf("created dir mode %v", st.Mode().Perm())
	}
}

func TestReplaceStopsBeforeAppendingWhenRemovalFails(t *testing.T) {
	kh := filepath.Join(t.TempDir(), "known_hosts")
	_ = os.WriteFile(kh, []byte("old-line\n"), 0o600)
	f := &fake{answers: map[string]func(call) (string, error){
		"ssh-keygen -R": func(call) (string, error) { return "", errors.New("boom") },
	}}
	if _, err := Replace(f.exec, Target{HostName: "10.0.0.1", Port: "22", KnownHosts: []string{kh}}, "new-line\n", "ts"); err == nil {
		t.Fatal("want error")
	}
	if b, _ := os.ReadFile(kh); strings.Contains(string(b), "new-line") {
		t.Fatal("must not append a new key beside an old one it failed to remove")
	}
}

// The old key is already gone when the append runs; a failed write must put
// it back rather than leave the host with no entry at all.
func TestReplaceRestoresTheBackupWhenTheAppendFails(t *testing.T) {
	kh := filepath.Join(t.TempDir(), "known_hosts")
	_ = os.WriteFile(kh, []byte("old-line\n"), 0o600)
	f := &fake{answers: map[string]func(call) (string, error){
		"ssh-keygen -R": func(c call) (string, error) { return "", os.WriteFile(c.args[3], nil, 0o600) },
	}}
	restore := appendFile
	appendFile = func(string, string) error { return errors.New("disk full") }
	t.Cleanup(func() { appendFile = restore })
	if _, err := Replace(f.exec, Target{HostName: "10.0.0.1", Port: "22", KnownHosts: []string{kh}}, "new-line\n", "ts"); err == nil {
		t.Fatal("want error")
	}
	if b, _ := os.ReadFile(kh); string(b) != "old-line\n" {
		t.Fatalf("known_hosts after a failed append %q, want the original back", b)
	}
}
