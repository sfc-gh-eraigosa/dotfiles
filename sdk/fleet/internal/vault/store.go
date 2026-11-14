// Package vault is the contract between fleet and a password-manager backend
// that holds per-workstation SSH keys. It depends only on the standard library.
package vault

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Exec runs a vendor CLI, feeding stdin and returning stdout. Secrets travel
// only on stdin, never in argv.
type Exec func(ctx context.Context, name string, stdin []byte, args ...string) (stdout []byte, err error)

const redacted = "[redacted]"

// Secret is sensitive bytes. Every formatting path prints "[redacted]"; code
// that needs the bytes converts explicitly with []byte(s).
type Secret []byte

// String implements fmt.Stringer.
func (Secret) String() string { return redacted }

// GoString implements fmt.GoStringer.
func (Secret) GoString() string { return redacted }

// Format implements fmt.Formatter, covering every verb.
func (Secret) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, redacted) }

// MarshalJSON emits the JSON string "[redacted]".
func (Secret) MarshalJSON() ([]byte, error) { return json.Marshal(redacted) }

// Blob is a stored payload plus its opaque version.
type Blob struct {
	Data    Secret
	Version string // opaque; "" only for a blob never stored
}

// Store is one vendor backend.
type Store interface {
	Name() string                                                                // "op" | "lpass"
	Ready(ctx context.Context) error                                             // nil | ErrNotInstalled | ErrSignedOut
	SignIn(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) error // vendor owns the terminal
	Get(ctx context.Context, name string) (Blob, error)                          // ErrNotFound when absent
	// Put stores data. version "" means the item must not exist yet (create);
	// otherwise it must equal the current version. ErrConflict otherwise.
	Put(ctx context.Context, name string, data Secret, version string) (newVersion string, err error)
	List(ctx context.Context) ([]string, error) // logical names of this fleet's items
}

var (
	ErrNotInstalled = errors.New("vault: provider CLI not installed")
	ErrSignedOut    = errors.New("vault: signed out of provider")
	ErrNotFound     = errors.New("vault: item not found")
	ErrConflict     = errors.New("vault: item changed concurrently (version mismatch)")
)

// Config selects and parameterises a provider.
type Config struct {
	Provider    string // "op" | "lpass"
	Vault       string // op: vault name (default "Private"); lpass: folder (default "fleet")
	Workstation string // default: short hostname
	Key         string // default "~/.ssh/id_ed25519"
}
