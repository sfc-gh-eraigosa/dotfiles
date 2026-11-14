package vault

import (
	"encoding/json"
	"fmt"
	"time"
)

// SchemaV1 is the schema id carried by every encoded blob.
const SchemaV1 = "fleet-vault/v1"

// KeyPair is one workstation's SSH key material.
type KeyPair struct {
	Private, Passphrase Secret
	Public, Fingerprint string
	Created             time.Time
}

// KeyState is a roster entry's lifecycle state.
type KeyState string

const (
	KeyPending KeyState = "pending"
	KeyActive  KeyState = "active"
	KeyRetired KeyState = "retired"
)

// RosterEntry tracks one key of one workstation.
type RosterEntry struct {
	Workstation string
	Public      string
	Fingerprint string
	State       KeyState
	Changed     time.Time
	VerifiedOn  []string
}

// Roster is the fleet-wide key roster.
type Roster struct{ Entries []RosterEntry }

// Inventory is the #fleet Host blocks, as ssh_config text.
type Inventory struct{ Config string }

// KnownHosts holds known_hosts lines for fleet aliases.
type KnownHosts struct{ Lines []string }

type envelope struct {
	Schema string `json:"schema"`
	Kind   string `json:"kind"`
}

// keyPairWire carries secrets as []byte so encoding/json base64-encodes them
// (Secret.MarshalJSON deliberately redacts).
type keyPairWire struct {
	envelope
	Private     []byte    `json:"private"`
	Passphrase  []byte    `json:"passphrase"`
	Public      string    `json:"public"`
	Fingerprint string    `json:"fingerprint"`
	Created     time.Time `json:"created"`
}

type rosterWire struct {
	envelope
	Entries []RosterEntry `json:"entries"`
}

type inventoryWire struct {
	envelope
	Config string `json:"config"`
}

type knownHostsWire struct {
	envelope
	Lines []string `json:"lines"`
}

func env(kind string) envelope { return envelope{Schema: SchemaV1, Kind: kind} }

// Encode serialises a KeyPair, Roster, Inventory or KnownHosts (value or
// pointer) into a schema-tagged blob. The result is a Secret so it stays
// redacted when printed.
func Encode(v any) (Secret, error) {
	var w any
	switch x := v.(type) {
	case KeyPair:
		w = keyPairWire{env("KeyPair"), []byte(x.Private), []byte(x.Passphrase), x.Public, x.Fingerprint, x.Created}
	case *KeyPair:
		return Encode(*x)
	case Roster:
		w = rosterWire{env("Roster"), x.Entries}
	case *Roster:
		return Encode(*x)
	case Inventory:
		w = inventoryWire{env("Inventory"), x.Config}
	case *Inventory:
		return Encode(*x)
	case KnownHosts:
		w = knownHostsWire{env("KnownHosts"), x.Lines}
	case *KnownHosts:
		return Encode(*x)
	default:
		return nil, fmt.Errorf("vault: cannot encode %T", v)
	}
	b, err := json.Marshal(w)
	if err != nil {
		return nil, err
	}
	return Secret(b), nil
}

func check(b Secret, kind string) error {
	var e envelope
	if err := json.Unmarshal(b, &e); err != nil {
		return fmt.Errorf("vault: decode %s: %w", kind, err)
	}
	if e.Schema != SchemaV1 {
		return fmt.Errorf("vault: unsupported schema %q (want %q)", e.Schema, SchemaV1)
	}
	if e.Kind != kind {
		return fmt.Errorf("vault: wrong kind: expected %q, got %q", kind, e.Kind)
	}
	return nil
}

// Decode parses a blob into T, verifying schema and kind.
func Decode[T KeyPair | Roster | Inventory | KnownHosts](b Secret) (T, error) {
	var out T
	var kind string
	switch any(out).(type) {
	case KeyPair:
		kind = "KeyPair"
	case Roster:
		kind = "Roster"
	case Inventory:
		kind = "Inventory"
	default:
		kind = "KnownHosts"
	}
	if err := check(b, kind); err != nil {
		return out, err
	}
	var err error
	switch p := any(&out).(type) {
	case *KeyPair:
		var w keyPairWire
		if err = json.Unmarshal(b, &w); err == nil {
			*p = KeyPair{Secret(w.Private), Secret(w.Passphrase), w.Public, w.Fingerprint, w.Created}
		}
	case *Roster:
		var w rosterWire
		if err = json.Unmarshal(b, &w); err == nil {
			p.Entries = w.Entries
		}
	case *Inventory:
		var w inventoryWire
		if err = json.Unmarshal(b, &w); err == nil {
			p.Config = w.Config
		}
	case *KnownHosts:
		var w knownHostsWire
		if err = json.Unmarshal(b, &w); err == nil {
			p.Lines = w.Lines
		}
	}
	if err != nil {
		var zero T
		return zero, fmt.Errorf("vault: decode %s: %w", kind, err)
	}
	return out, nil
}
