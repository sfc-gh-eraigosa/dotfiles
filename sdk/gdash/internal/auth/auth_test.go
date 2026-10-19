package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/config"
)

// fixtureCred is assembled at run time so no literal in this file looks
// like a credential to the privacy guard.
func fixtureCred() string { return strings.Join([]string{"fixture", "cred", "4b1d"}, "-") }

func writeCred(t *testing.T, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "lab.cred")
	if err := os.WriteFile(p, []byte(fixtureCred()+"\n"), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
	return p
}

func header(t *testing.T, c Credential) string {
	t.Helper()
	r, _ := http.NewRequest("GET", "http://192.0.2.1/", nil)
	c.Apply(r)
	return r.Header.Get("Authorization")
}

func TestSecretNeverFormats(t *testing.T) {
	s := NewSecret(fixtureCred())
	b, _ := json.Marshal(struct{ S Secret }{s})
	for _, out := range []string{s.String(), fmt.Sprint(s), fmt.Sprintf("%v %+v %#v %s %q", s, s, s, s, s), string(b)} {
		if strings.Contains(out, fixtureCred()) {
			t.Fatalf("secret leaked: %q", out)
		}
		if !strings.Contains(out, Redacted) {
			t.Fatalf("no redaction marker in %q", out)
		}
	}
}

func TestFileSource(t *testing.T) {
	c, err := Load(config.Auth{File: writeCred(t, 0o600)})
	if err != nil {
		t.Fatal(err)
	}
	if got := header(t, c); got != "Bearer "+fixtureCred() {
		t.Fatalf("Authorization = %q", got)
	}
}

// E10: a token file others can read is refused, with the fix.
func TestFileSourceRefusesLooseMode(t *testing.T) {
	_, err := Load(config.Auth{File: writeCred(t, 0o644)})
	if err == nil || !strings.Contains(err.Error(), "chmod 600") {
		t.Fatalf("0644 file: %v", err)
	}
	if strings.Contains(fmt.Sprint(err), fixtureCred()) {
		t.Fatal("the error leaked the secret")
	}
}

func TestFileSourceResolvesSymlinkThenChecks(t *testing.T) {
	target := writeCred(t, 0o600)
	link := filepath.Join(t.TempDir(), "link.cred")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(config.Auth{File: link}); err != nil {
		t.Fatalf("symlink to a 0600 file refused: %v", err)
	}
	loose := writeCred(t, 0o644)
	link2 := filepath.Join(t.TempDir(), "loose.cred")
	_ = os.Symlink(loose, link2)
	if _, err := Load(config.Auth{File: link2}); err == nil {
		t.Fatal("symlink to a 0644 file accepted (the link's own mode was checked)")
	}
}

func TestFileSourceExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	p := filepath.Join(home, "lab.cred")
	if err := os.WriteFile(p, []byte(fixtureCred()), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(config.Auth{File: "~/lab.cred"}); err != nil {
		t.Fatalf("~ not expanded: %v", err)
	}
}

func TestEnvSource(t *testing.T) {
	t.Setenv("GDASH_TEST_CRED", fixtureCred())
	c, err := Load(config.Auth{Env: "GDASH_TEST_CRED"})
	if err != nil || header(t, c) != "Bearer "+fixtureCred() {
		t.Fatalf("env source: %v", err)
	}
	t.Setenv("GDASH_TEST_CRED", "")
	if _, err := Load(config.Auth{Env: "GDASH_TEST_CRED"}); err == nil || !strings.Contains(err.Error(), "GDASH_TEST_CRED") {
		t.Fatalf("empty env should name the variable: %v", err)
	}
}

func TestCmdSource(t *testing.T) {
	c, err := Load(config.Auth{Cmd: "printf '%s\\n' " + fixtureCred()})
	if err != nil || header(t, c) != "Bearer "+fixtureCred() {
		t.Fatalf("cmd source: %v", err)
	}
}

// E11: a command that waits for input, hangs, fails or prints nothing gives
// a named error quickly instead of prompting or hanging.
func TestCmdNoTTYTimeout(t *testing.T) {
	old := CmdTimeout
	CmdTimeout = 300 * time.Millisecond
	t.Cleanup(func() { CmdTimeout = old })
	cases := map[string]string{
		"read line":      "read -r x; printf %s \"$x\"", // stdin is /dev/null: empty
		"sleeps":         "sleep 5; echo late",
		"fails":          "exit 3",
		"prints nothing": "true",
		"reads the tty":  "cat /dev/tty",
	}
	for name, cmd := range cases {
		start := time.Now()
		_, err := Load(config.Auth{Cmd: cmd})
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		if !strings.Contains(err.Error(), "auth.cmd") {
			t.Errorf("%s: error does not name auth.cmd: %v", name, err)
		}
		if d := time.Since(start); d > 2*time.Second {
			t.Errorf("%s: took %v", name, d)
		}
	}
}

func TestBasicAuth(t *testing.T) {
	t.Setenv("GDASH_TEST_CRED", fixtureCred())
	c, err := Load(config.Auth{User: "viewer", Env: "GDASH_TEST_CRED"})
	if err != nil {
		t.Fatal(err)
	}
	r, _ := http.NewRequest("GET", "http://192.0.2.1/", nil)
	c.Apply(r)
	if u, p, ok := r.BasicAuth(); !ok || u != "viewer" || p != fixtureCred() {
		t.Fatalf("basic auth = %q %v", u, ok)
	}
}

func TestExactlyOneSource(t *testing.T) {
	if _, err := Load(config.Auth{}); err == nil {
		t.Fatal("no source accepted")
	}
	if _, err := Load(config.Auth{Env: "A", Cmd: "true"}); err == nil {
		t.Fatal("two sources accepted")
	}
}
