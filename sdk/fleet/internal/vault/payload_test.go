package vault

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPayloadRoundTripsEveryKind(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	kp := KeyPair{Private: Secret("PRIVATE-MATERIAL-123"), Passphrase: Secret("PASS-MARKER-456"), Public: "ssh-ed25519 AAAA", Fingerprint: "SHA256:abc", Created: ts}
	ro := Roster{Entries: []RosterEntry{{Workstation: "ws", Public: "pub", Fingerprint: "fp", State: KeyActive, Changed: ts, VerifiedOn: []string{"h1"}}}}
	inv := Inventory{Config: "Host a\n  HostName 1.2.3.4\n"}
	kh := KnownHosts{Lines: []string{"a ssh-ed25519 AAAA", "b ssh-ed25519 BBBB"}}

	check := func(kind string, v any, enc Secret) {
		t.Helper()
		s := string(enc)
		if !strings.Contains(s, `"schema":"fleet-vault/v1"`) || !strings.Contains(s, `"kind":"`+kind+`"`) {
			t.Fatalf("%s: bad envelope %s", kind, s)
		}
	}

	e, err := Encode(kp)
	if err != nil {
		t.Fatal(err)
	}
	check("KeyPair", kp, e)
	if bytes.Contains(e, []byte("PRIVATE-MATERIAL-123")) {
		t.Fatal("private must be base64, not plaintext")
	}
	got, err := Decode[KeyPair](e)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Private, kp.Private) || !bytes.Equal(got.Passphrase, kp.Passphrase) {
		t.Fatalf("secret bytes lost: %q %q", []byte(got.Private), []byte(got.Passphrase))
	}
	if !reflect.DeepEqual(got, kp) {
		t.Fatalf("keypair mismatch")
	}

	e, _ = Encode(ro)
	check("Roster", ro, e)
	if g, err := Decode[Roster](e); err != nil || !reflect.DeepEqual(g, ro) {
		t.Fatalf("roster: %v", err)
	}
	e, _ = Encode(inv)
	check("Inventory", inv, e)
	if g, err := Decode[Inventory](e); err != nil || !reflect.DeepEqual(g, inv) {
		t.Fatalf("inventory: %v", err)
	}
	e, _ = Encode(kh)
	check("KnownHosts", kh, e)
	if g, err := Decode[KnownHosts](e); err != nil || !reflect.DeepEqual(g, kh) {
		t.Fatalf("known hosts: %v", err)
	}
}

func TestEncodeRedactsWhenPrinted(t *testing.T) {
	e, _ := Encode(KeyPair{Private: Secret("PRIVATE-MATERIAL-123")})
	if strings.Contains(e.String(), "PRIVATE") {
		t.Fatal("not redacted")
	}
}

func TestEncodeRejectsUnknownType(t *testing.T) {
	if _, err := Encode(42); err == nil {
		t.Fatal("want error")
	}
	if _, err := Encode(&Roster{}); err != nil {
		t.Fatalf("pointer should work: %v", err)
	}
}

func TestDecodeRejectsAWrongKindOrSchema(t *testing.T) {
	e, _ := Encode(KeyPair{Public: "x"})
	_, err := Decode[Roster](e)
	if err == nil || !strings.Contains(err.Error(), "Roster") || !strings.Contains(err.Error(), "KeyPair") {
		t.Fatalf("want kind error naming both, got %v", err)
	}
	v2 := Secret(`{"schema":"fleet-vault/v2","kind":"Roster"}`)
	_, err = Decode[Roster](v2)
	if err == nil || !strings.Contains(err.Error(), "fleet-vault/v2") {
		t.Fatalf("want schema error, got %v", err)
	}
	if _, err = Decode[Roster](Secret("not json")); err == nil {
		t.Fatal("want parse error")
	}
	if _, err = Decode[KeyPair](Secret(`{"schema":"fleet-vault/v1","kind":"KeyPair","private":"!!!"}`)); err == nil {
		t.Fatal("want base64 error")
	}
}

func TestEncodedFieldNamesArePinned(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	cases := map[string]struct {
		v    any
		keys []string
	}{
		"KeyPair":    {KeyPair{Created: ts}, []string{"private", "passphrase", "public", "fingerprint", "created"}},
		"Roster":     {Roster{Entries: []RosterEntry{{State: KeyActive, VerifiedOn: []string{"h"}}}}, []string{"entries", "workstation", "public", "fingerprint", "state", "changed", "verified_on"}},
		"Inventory":  {Inventory{}, []string{"config"}},
		"KnownHosts": {KnownHosts{Lines: []string{"x"}}, []string{"lines"}},
	}
	for name, c := range cases {
		e, err := Encode(c.v)
		if err != nil {
			t.Fatal(err)
		}
		for _, k := range c.keys {
			if !strings.Contains(string(e), `"`+k+`":`) {
				t.Errorf("%s: key %q missing in %s", name, k, e)
			}
		}
	}
}
