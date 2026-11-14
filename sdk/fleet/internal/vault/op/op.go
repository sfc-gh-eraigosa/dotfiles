// Package op is the vault.Store adapter over the 1Password CLI (op v2).
//
// Each logical item is a Secure Note titled "fleet:<logical>" in one vault,
// tagged "fleet", whose payload lives in a CONCEALED custom field labelled
// "payload". The payload travels only on the child's stdin (a JSON item
// template), never in argv and never in a temp file. Version is the item's
// top-level "version" counter rendered as a decimal string.
//
// Session persistence is the operator's concern: `op signin` run from fleet
// cannot export OP_SESSION_* into the operator's shell, so the 1Password
// desktop-app integration or OP_SESSION_* must already be in the environment
// fleet runs under.
//
// Every argv line here is unconfirmed against a live account.
package op

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault"
)

const (
	bin          = "op"
	defaultVault = "Private"
	itemPrefix   = "fleet:"
	tag          = "fleet"
	payloadLabel = "payload"
)

// Interactive runs a command with the caller's terminal attached.
type Interactive func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, name string, args ...string) error

// Store implements vault.Store over op.
type Store struct {
	vault string
	exec  vault.Exec
	// Interactive runs `op signin`; it defaults to os/exec with the given stdio.
	Interactive Interactive
}

var _ vault.Store = (*Store)(nil)

// New returns a Store for cfg using x to run op.
func New(cfg vault.Config, x vault.Exec) *Store {
	v := strings.TrimSpace(cfg.Vault)
	if v == "" {
		v = defaultVault
	}
	return &Store{vault: v, exec: x, Interactive: runInteractive}
}

func runInteractive(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, name string, args ...string) error {
	c := exec.CommandContext(ctx, name, args...)
	c.Stdin, c.Stdout, c.Stderr = stdin, stdout, stderr
	return c.Run()
}

// Name returns "op".
func (*Store) Name() string { return "op" }

func (s *Store) title(name string) string { return itemPrefix + name }

var signedOutTexts = []string{
	"not currently signed in",
	"no account found",
	"account is not signed in",
}

func isSignedOut(err error) bool {
	m := strings.ToLower(err.Error())
	for _, t := range signedOutTexts {
		if strings.Contains(m, t) {
			return true
		}
	}
	return false
}

// classify maps op failures to the vault sentinels. It only ever reads the
// error text (stderr), never stdout.
func classify(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, vault.ErrNotInstalled) {
		return vault.ErrNotInstalled
	}
	if isSignedOut(err) {
		return fmt.Errorf("%w: %v", vault.ErrSignedOut, err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "isn't an item") {
		return fmt.Errorf("%w: %v", vault.ErrNotFound, err)
	}
	return err
}

// Ready runs `op whoami`.
func (s *Store) Ready(ctx context.Context) error {
	if _, err := s.exec(ctx, bin, nil, "whoami"); err != nil {
		if errors.Is(err, vault.ErrNotInstalled) {
			return vault.ErrNotInstalled
		}
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if isSignedOut(err) {
			return fmt.Errorf("%w: %v", vault.ErrSignedOut, err)
		}
		return fmt.Errorf("op whoami: %w", err)
	}
	return nil
}

// SignIn runs `op signin` on the operator's terminal.
func (s *Store) SignIn(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) error {
	return s.Interactive(ctx, stdin, stdout, stderr, bin, "signin")
}

type field struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type item struct {
	Version json.Number `json:"version"`
	Fields  []field     `json:"fields"`
}

// fetched is one `op item get` result: the raw document (kept so an edit can
// send the full item back), the payload and the version.
type fetched struct {
	raw     map[string]json.RawMessage
	payload []byte
	version string
}

func (s *Store) fetch(ctx context.Context, name string) (fetched, error) {
	t := s.title(name)
	out, err := s.exec(ctx, bin, nil, "item", "get", t, "--vault", s.vault, "--format", "json", "--reveal")
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "more than one item matches") {
			return fetched{}, fmt.Errorf("op: %s is ambiguous (multiple items share that title in vault %q)", t, s.vault)
		}
		return fetched{}, classify(err)
	}
	var it item
	if err := json.Unmarshal(out, &it); err != nil {
		return fetched{}, fmt.Errorf("op: %s: unparseable item JSON", t) // never echo stdout
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(out, &raw); err != nil {
		return fetched{}, fmt.Errorf("op: %s: unparseable item JSON", t)
	}
	if _, err := strconv.ParseInt(it.Version.String(), 10, 64); err != nil {
		return fetched{}, fmt.Errorf("op: %s: item has no numeric version", t)
	}
	for _, f := range it.Fields {
		if f.Label == payloadLabel {
			return fetched{raw: raw, payload: []byte(f.Value), version: it.Version.String()}, nil
		}
	}
	return fetched{}, fmt.Errorf("op: %s has no %q field", t, payloadLabel)
}

// Get returns the payload field and the item version.
func (s *Store) Get(ctx context.Context, name string) (vault.Blob, error) {
	f, err := s.fetch(ctx, name)
	if err != nil {
		return vault.Blob{}, err
	}
	return vault.Blob{Data: vault.Secret(f.payload), Version: f.version}, nil
}

// Put creates (version "") or updates the item, returning the version a
// re-read reports. The version check is best-effort: op has no
// compare-and-swap, so a race window exists between the read and the write.
func (s *Store) Put(ctx context.Context, name string, data vault.Secret, ver string) (string, error) {
	if err := s.Ready(ctx); err != nil {
		return "", err
	}
	if !utf8.Valid(data) {
		return "", errors.New("op: payload is not valid UTF-8; a JSON field cannot carry it")
	}
	cur, err := s.fetch(ctx, name)
	if ver == "" {
		switch {
		case err == nil:
			return "", vault.ErrConflict
		case !errors.Is(err, vault.ErrNotFound):
			return "", err
		}
		tmpl, merr := json.Marshal(map[string]any{
			"title":    s.title(name),
			"category": "SECURE_NOTE",
			"tags":     []string{tag},
			"fields":   []field2{{Label: payloadLabel, Type: "CONCEALED", Value: string(data)}},
		})
		if merr != nil {
			return "", merr
		}
		if _, err := s.exec(ctx, bin, tmpl, "item", "create", "--vault", s.vault, "--format", "json", "-"); err != nil {
			return "", classify(err)
		}
	} else {
		if err != nil {
			return "", err
		}
		if cur.version != ver {
			return "", vault.ErrConflict
		}
		doc, merr := replacePayload(cur.raw, data)
		if merr != nil {
			return "", merr
		}
		if _, err := s.exec(ctx, bin, doc, "item", "edit", s.title(name), "--vault", s.vault, "--format", "json"); err != nil {
			return "", classify(err)
		}
	}
	b, err := s.Get(ctx, name)
	if err != nil {
		return "", fmt.Errorf("op: %s written but re-read failed (the write may have landed): %w", name, err)
	}
	if !bytes.Equal(b.Data, data) {
		return "", fmt.Errorf("%w: op: %s changed by another writer or normalised after write", vault.ErrConflict, name)
	}
	return b.Version, nil
}

type field2 struct {
	Label string `json:"label"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

// replacePayload returns the item document with the payload field's value
// replaced and every other key and field untouched.
func replacePayload(raw map[string]json.RawMessage, data vault.Secret) ([]byte, error) {
	var fields []map[string]json.RawMessage
	if err := json.Unmarshal(raw["fields"], &fields); err != nil {
		return nil, errors.New("op: item has unreadable fields")
	}
	val, err := json.Marshal(string(data))
	if err != nil {
		return nil, err
	}
	done := false
	for _, f := range fields {
		var l string
		if json.Unmarshal(f["label"], &l) == nil && l == payloadLabel {
			f["value"] = val
			done = true
		}
	}
	if !done {
		return nil, errors.New("op: item has no payload field")
	}
	nf, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	out := make(map[string]json.RawMessage, len(raw))
	for k, v := range raw {
		out[k] = v
	}
	out["fields"] = nf
	return json.Marshal(out)
}

// List returns the logical names of this vault's fleet: items.
func (s *Store) List(ctx context.Context) ([]string, error) {
	out, err := s.exec(ctx, bin, nil, "item", "list", "--vault", s.vault, "--tags", tag, "--format", "json")
	if err != nil {
		return nil, classify(err)
	}
	var items []struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, errors.New("op: unparseable item list JSON")
	}
	var names []string
	for _, it := range items {
		if rest, ok := strings.CutPrefix(it.Title, itemPrefix); ok && rest != "" {
			names = append(names, rest)
		}
	}
	return names, nil
}
