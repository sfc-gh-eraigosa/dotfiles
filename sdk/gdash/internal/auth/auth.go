// Package auth loads a profile's Grafana credential from exactly one source
// (a 0600 file, an environment variable, or a command) and keeps it in a
// Secret that never prints (spec F6, E10–E12).
package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/config"
)

// Redacted is what a Secret prints as, everywhere.
const Redacted = "‹redacted›"

// CmdTimeout bounds auth.cmd. A variable so tests can shorten it.
var CmdTimeout = 5 * time.Second

// Secret holds a credential. Every way of printing it prints Redacted.
type Secret struct{ v string }

// NewSecret wraps a credential.
func NewSecret(v string) Secret { return Secret{v: v} }

func (Secret) String() string                  { return Redacted }
func (Secret) GoString() string                { return Redacted }
func (Secret) MarshalJSON() ([]byte, error)    { return []byte(`"` + Redacted + `"`), nil }
func (Secret) MarshalText() ([]byte, error)    { return []byte(Redacted), nil }
func (s Secret) Format(f fmt.State, verb rune) { _, _ = fmt.Fprint(f, Redacted) }
func (s Secret) empty() bool                   { return s.v == "" }

// Credential sets a request's Authorization header.
type Credential interface {
	Apply(r *http.Request)
}

type bearer struct{ s Secret }

func (b bearer) Apply(r *http.Request) { r.Header.Set("Authorization", "Bearer "+b.s.v) }

// Redact removes the token from s (an error body that echoed it, say).
func (b bearer) Redact(s string) string { return redact(s, b.s.v) }

type basic struct {
	user string
	pw   Secret
}

func (b basic) Apply(r *http.Request) { r.SetBasicAuth(b.user, b.pw.v) }

// Redact removes the password, and the header's base64 user:password, from s.
func (b basic) Redact(s string) string {
	return redact(s, b.pw.v, base64.StdEncoding.EncodeToString([]byte(b.user+":"+b.pw.v)))
}

func redact(s string, secrets ...string) string {
	for _, v := range secrets {
		if v != "" {
			s = strings.ReplaceAll(s, v, Redacted)
		}
	}
	return s
}

// Load reads the credential a profile names. With auth.user set it is the
// basic-auth password; otherwise it is a bearer (service-account) token.
func Load(a config.Auth) (Credential, error) {
	var (
		s   Secret
		err error
	)
	switch src := a.Sources(); {
	case len(src) == 0:
		return nil, errors.New("auth: set one of auth.file, auth.env or auth.cmd")
	case len(src) > 1:
		return nil, fmt.Errorf("auth: exactly one of auth.file, auth.env, auth.cmd may be set (found %s)", strings.Join(src, ", "))
	case a.File != "":
		s, err = fromFile(a.File)
	case a.Env != "":
		s, err = fromEnv(a.Env)
	default:
		s, err = fromCmd(a.Cmd)
	}
	if err != nil {
		return nil, err
	}
	if a.User != "" {
		return basic{user: a.User, pw: s}, nil
	}
	return bearer{s: s}, nil
}

func fromFile(p string) (Secret, error) {
	if err := config.CheckPrivateFile(p); err != nil {
		return Secret{}, fmt.Errorf("auth.file: %w", err)
	}
	b, err := os.ReadFile(config.ExpandHome(p)) //nolint:gosec // the user's own credential path, checked above
	if err != nil {
		return Secret{}, fmt.Errorf("auth.file: %w", err)
	}
	s := NewSecret(strings.TrimSpace(string(b)))
	if s.empty() {
		return Secret{}, fmt.Errorf("auth.file: %s is empty", p)
	}
	return s, nil
}

func fromEnv(name string) (Secret, error) {
	s := NewSecret(strings.TrimSpace(os.Getenv(name)))
	if s.empty() {
		return Secret{}, fmt.Errorf("auth.env: $%s is empty or unset", name)
	}
	return s, nil
}

// fromCmd runs the command once, non-interactively: stdin is /dev/null and it
// gets a new session, so it has no controlling terminal to prompt on. Only
// stdout is the credential; stderr is discarded so it can never leak.
func fromCmd(command string) (Secret, error) {
	ctx, cancel := context.WithTimeout(context.Background(), CmdTimeout)
	defer cancel()
	c := exec.CommandContext(ctx, "sh", "-c", command) //nolint:gosec // the user's own configured command
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	c.Stdin = nil // /dev/null
	var out bytes.Buffer
	c.Stdout = &out
	c.WaitDelay = 500 * time.Millisecond
	err := c.Run()
	switch {
	case ctx.Err() != nil:
		return Secret{}, fmt.Errorf("auth.cmd: did not answer within %s (it must not prompt; use a logged-in CLI, auth.file or auth.env)", CmdTimeout)
	case err != nil:
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return Secret{}, fmt.Errorf("auth.cmd: exited %d", ee.ExitCode())
		}
		return Secret{}, fmt.Errorf("auth.cmd: %w", err)
	}
	s := NewSecret(strings.TrimSpace(out.String()))
	if s.empty() {
		return Secret{}, errors.New("auth.cmd: printed nothing")
	}
	return s, nil
}
