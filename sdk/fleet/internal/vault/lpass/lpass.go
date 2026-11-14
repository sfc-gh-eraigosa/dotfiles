// Package lpass is the vault.Store adapter over the LastPass CLI (lpass 1.6.x).
//
// Each logical item is a note named "<folder>/fleet:<logical>" whose Notes body
// is the payload. The payload travels only on the child's stdin. Version is the
// lowercase hex sha256 of the exact notes bytes `lpass show --notes` returns.
package lpass

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault"
)

const (
	bin           = "lpass"
	defaultFolder = "fleet"
	itemPrefix    = "fleet:"
)

// Interactive runs a command with the caller's terminal attached.
type Interactive func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, name string, args ...string) error

// Store implements vault.Store over lpass.
type Store struct {
	folder string
	exec   vault.Exec
	// Interactive runs `lpass login`; it defaults to os/exec with the given stdio.
	Interactive Interactive
}

var _ vault.Store = (*Store)(nil)

// New returns a Store for cfg using x to run lpass.
func New(cfg vault.Config, x vault.Exec) *Store {
	f := strings.Trim(cfg.Vault, "/")
	if f == "" {
		f = defaultFolder
	}
	return &Store{folder: f, exec: x, Interactive: runInteractive}
}

func runInteractive(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, name string, args ...string) error {
	c := exec.CommandContext(ctx, name, args...)
	c.Stdin, c.Stdout, c.Stderr = stdin, stdout, stderr
	return c.Run()
}

// Name returns "lpass".
func (*Store) Name() string { return "lpass" }

// escape keeps a logical name's "/" from being read by lpass as a folder.
func escape(s string) string {
	return strings.NewReplacer("%", "%25", "/", "%2F").Replace(s)
}

func unescape(s string) string {
	return strings.NewReplacer("%2F", "/", "%25", "%").Replace(s)
}

func (s *Store) item(name string) string { return s.folder + "/" + itemPrefix + escape(name) }

func version(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// classify maps lpass failures to the vault sentinels.
func classify(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, vault.ErrNotInstalled) {
		return vault.ErrNotInstalled
	}
	m := strings.ToLower(err.Error())
	switch {
	case strings.Contains(m, "could not find decryption key"),
		strings.Contains(m, "not logged in"),
		strings.Contains(m, "lpass login"):
		return fmt.Errorf("%w: %v", vault.ErrSignedOut, err)
	case strings.Contains(m, "could not find specified account"):
		return fmt.Errorf("%w: %v", vault.ErrNotFound, err)
	}
	return err
}

// Ready runs `lpass status -q`.
func (s *Store) Ready(ctx context.Context) error {
	if _, err := s.exec(ctx, bin, nil, "status", "-q"); err != nil {
		if errors.Is(err, vault.ErrNotInstalled) {
			return vault.ErrNotInstalled
		}
		return fmt.Errorf("%w: %v", vault.ErrSignedOut, err)
	}
	return nil
}

// SignIn runs `lpass login --trust $LPASS_USER` on the operator's terminal.
func (s *Store) SignIn(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) error {
	user := os.Getenv("LPASS_USER")
	if user == "" {
		return errors.New("lpass: LPASS_USER is not set; run `lpass login --trust <email>` yourself, then retry")
	}
	return s.Interactive(ctx, stdin, stdout, stderr, bin, "login", "--trust", user)
}

func (s *Store) read(ctx context.Context, name string) ([]byte, error) {
	out, err := s.exec(ctx, bin, nil, "show", "--sync=now", "--notes", s.item(name))
	if err != nil {
		return nil, classify(err)
	}
	// lpass appends exactly one "\n" to the stored notes; strip it so data round-trips.
	return bytes.TrimSuffix(out, []byte("\n")), nil
}

// Get returns the note body and its version.
func (s *Store) Get(ctx context.Context, name string) (vault.Blob, error) {
	out, err := s.read(ctx, name)
	if err != nil {
		return vault.Blob{}, err
	}
	return vault.Blob{Data: vault.Secret(out), Version: version(out)}, nil
}

// Put creates (version "") or updates the item, returning the version a re-read reports.
func (s *Store) Put(ctx context.Context, name string, data vault.Secret, ver string) (string, error) {
	if err := s.Ready(ctx); err != nil {
		return "", err
	}
	cur, err := s.read(ctx, name)
	sub := "edit"
	if ver == "" {
		sub = "add"
		switch {
		case err == nil:
			return "", vault.ErrConflict
		case !errors.Is(err, vault.ErrNotFound):
			return "", err
		}
	} else {
		if err != nil {
			return "", err
		}
		if version(cur) != ver {
			return "", vault.ErrConflict
		}
	}
	if _, err := s.exec(ctx, bin, []byte(data), sub, "--sync=now", "--non-interactive", "--notes", s.item(name)); err != nil {
		return "", classify(err)
	}
	// The re-read is authoritative: it is what the next Get will hash.
	b, err := s.Get(ctx, name)
	if err != nil {
		return "", err
	}
	return b.Version, nil
}

// List returns the logical names of this folder's fleet: items.
func (s *Store) List(ctx context.Context) ([]string, error) {
	out, err := s.exec(ctx, bin, nil, "ls", "--sync=now", s.folder)
	if err != nil {
		return nil, classify(err)
	}
	var names []string
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if i := strings.LastIndex(line, " [id:"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimPrefix(line, s.folder+"/")
		if rest, ok := strings.CutPrefix(line, itemPrefix); ok && rest != "" {
			names = append(names, unescape(rest))
		}
	}
	return names, sc.Err()
}
