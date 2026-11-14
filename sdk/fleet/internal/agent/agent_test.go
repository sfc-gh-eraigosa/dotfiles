package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/keystore"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault"
)

// TestMain lets this test binary act as fleet's SSH_ASKPASS program for
// TestLoadRealSshAddWithThrowawayAgent: ssh-add runs os.Args[0] with
// FLEET_ASKPASS_SOCK set, exactly as it would run the fleet binary.
func TestMain(m *testing.M) {
	if sock := os.Getenv("FLEET_ASKPASS_SOCK"); sock != "" {
		if err := AskpassClient(sock, os.Stdout); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

const testPass = "test-passphrase-not-secret"

type exitErr struct {
	code int
	msg  string
}

func (e exitErr) Error() string { return fmt.Sprintf("exit status %d: %s", e.code, e.msg) }
func (e exitErr) ExitCode() int { return e.code }

func TestLoadedParsesSshAddList(t *testing.T) {
	ctx := context.Background()
	const fp = "SHA256:abcDEF123"
	list := "256 SHA256:other comment-a (ED25519)\n256 " + fp + " me@ws (ED25519)\n"

	t.Run("unset socket", func(t *testing.T) {
		t.Setenv("SSH_AUTH_SOCK", "")
		ran := false
		x := func(context.Context, string, []string, ...string) ([]byte, error) { ran = true; return nil, nil }
		_, err := Loaded(ctx, x, fp)
		if !errors.Is(err, ErrNoAgent) {
			t.Fatalf("err = %v, want ErrNoAgent", err)
		}
		if ran {
			t.Fatal("ssh-add ran with no SSH_AUTH_SOCK")
		}
	})

	cases := []struct {
		name    string
		out     string
		err     error
		want    bool
		wantErr error
	}{
		{"present", list, nil, true, nil},
		{"absent", "256 SHA256:other c (ED25519)\n", nil, false, nil},
		{"fingerprint only as comment", "256 SHA256:other " + fp + " (ED25519)\n", nil, false, nil},
		{"no identities", "The agent has no identities.\n", exitErr{1, ""}, false, nil},
		{"dead agent", "", exitErr{2, "Could not open a connection to your authentication agent."}, false, ErrNoAgent},
		{"not installed", "", exec.ErrNotFound, false, ErrNoAgent},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("SSH_AUTH_SOCK", "/nonexistent/fake-agent.sock")
			var gotName string
			var gotArgs []string
			x := func(_ context.Context, name string, _ []string, args ...string) ([]byte, error) {
				gotName, gotArgs = name, args
				return []byte(c.out), c.err
			}
			got, err := Loaded(ctx, x, fp)
			if gotName != "ssh-add" || strings.Join(gotArgs, " ") != "-l" {
				t.Fatalf("ran %s %v, want ssh-add -l", gotName, gotArgs)
			}
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("err = %v, want %v", err, c.wantErr)
				}
				return
			}
			if err != nil || got != c.want {
				t.Fatalf("Loaded = %v, %v; want %v, nil", got, err, c.want)
			}
		})
	}
}

// fakeSshAdd acts as ssh-add: it records env and args, then plays the askpass
// program by dialing FLEET_ASKPASS_SOCK from the env it was handed.
type fakeSshAdd struct {
	mu       sync.Mutex
	name     string
	sock     string
	env      []string
	args     []string
	received string
	skipAsk  bool
	fail     error
}

func (f *fakeSshAdd) exec(_ context.Context, name string, env []string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.name = name
	f.env, f.args = append([]string(nil), env...), append([]string(nil), args...)
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, "FLEET_ASKPASS_SOCK="); ok {
			f.sock = v
		}
	}
	if !f.skipAsk {
		var out bytes.Buffer
		if err := AskpassClient(f.sock, &out); err != nil {
			return nil, err
		}
		f.received = out.String()
	}
	return nil, f.fail
}

func TestLoadPassesThePassphraseOnlyOverTheSocket(t *testing.T) {
	ctx := context.Background()
	t.Setenv("SSH_ASKPASS", "/stale/askpass")
	t.Setenv("FLEET_ASKPASS_SOCK", "")
	for _, goos := range []string{"darwin", "linux"} {
		t.Run(goos, func(t *testing.T) {
			f := &fakeSshAdd{}
			if err := Load(ctx, f.exec, "/usr/local/bin/fleet", "/k/id_ed25519", vault.Secret(testPass), goos); err != nil {
				t.Fatalf("Load: %v", err)
			}
			if f.received != testPass+"\n" {
				t.Fatalf("askpass received %q, want the passphrase plus newline", f.received)
			}
			for _, s := range append(append([]string(nil), f.env...), f.args...) {
				if strings.Contains(s, testPass) {
					t.Fatalf("passphrase leaked into env/argv: %q", s)
				}
			}
			has := func(want string) bool {
				for _, e := range f.env {
					if e == want {
						return true
					}
				}
				return false
			}
			for _, want := range []string{"SSH_ASKPASS=/usr/local/bin/fleet", "SSH_ASKPASS_REQUIRE=force"} {
				if !has(want) {
					t.Errorf("env missing %s", want)
				}
			}
			var socks, askpass int
			for _, e := range f.env {
				if strings.HasPrefix(e, "FLEET_ASKPASS_SOCK=") {
					socks++
					if !strings.HasSuffix(e, "/askpass.sock") {
						t.Errorf("FLEET_ASKPASS_SOCK = %q", e)
					}
				}
				if strings.HasPrefix(e, "SSH_ASKPASS=") {
					askpass++
				}
			}
			if socks != 1 || askpass != 1 {
				t.Errorf("FLEET_ASKPASS_SOCK x%d, SSH_ASKPASS x%d; want exactly one each", socks, askpass)
			}
			wantName, wantArgs := "ssh-add", "/k/id_ed25519"
			if goos == "darwin" {
				// Only Apple's ssh-add knows --apple-use-keychain; Homebrew's
				// upstream openssh, often first in PATH, rejects it.
				wantName, wantArgs = "/usr/bin/ssh-add", "--apple-use-keychain /k/id_ed25519"
			}
			if f.name != wantName {
				t.Errorf("ran %q, want %q", f.name, wantName)
			}
			if got := strings.Join(f.args, " "); got != wantArgs {
				t.Errorf("args = %q, want %q", got, wantArgs)
			}
		})
	}
}

func TestLoadErrors(t *testing.T) {
	ctx := context.Background()
	t.Run("ssh-add fails", func(t *testing.T) {
		f := &fakeSshAdd{fail: exitErr{1, "Bad passphrase, try again"}}
		err := Load(ctx, f.exec, "/fleet", "/k", vault.Secret(testPass), "linux")
		if err == nil || !strings.Contains(err.Error(), "Bad passphrase") {
			t.Fatalf("err = %v, want ssh-add stderr", err)
		}
		if strings.Contains(err.Error(), testPass) {
			t.Fatal("passphrase in error")
		}
	})
	t.Run("empty passphrase", func(t *testing.T) {
		f := &fakeSshAdd{}
		if err := Load(ctx, f.exec, "/fleet", "/k", nil, "linux"); err == nil {
			t.Fatal("empty passphrase accepted")
		}
	})
}

// Apple's ssh-add --apple-use-keychain reads a stored passphrase from the
// keychain and never runs askpass; a zero exit still means the key is loaded.
func TestLoadSucceedsWhenSshAddDoesNotAsk(t *testing.T) {
	for _, goos := range []string{"darwin", "linux"} {
		f := &fakeSshAdd{skipAsk: true}
		if err := Load(context.Background(), f.exec, "/fleet", "/k", vault.Secret(testPass), goos); err != nil {
			t.Fatalf("%s: Load = %v, want nil when ssh-add exits 0 without asking", goos, err)
		}
		if f.sock == "" {
			t.Fatalf("%s: no FLEET_ASKPASS_SOCK handed to ssh-add", goos)
		}
		if _, err := os.Stat(filepath.Dir(f.sock)); !os.IsNotExist(err) {
			t.Fatalf("%s: socket dir still present: %v", goos, err)
		}
	}
}

func TestAskpassServesOnceThenCloses(t *testing.T) {
	s, err := startAskpass(context.Background(), []byte(testPass))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	var out bytes.Buffer
	if err := AskpassClient(s.Sock, &out); err != nil {
		t.Fatalf("first client: %v", err)
	}
	if out.String() != testPass+"\n" {
		t.Fatalf("first client got %q", out.String())
	}
	s.wait()
	if !s.served() {
		t.Fatal("server did not record the read")
	}
	var out2 bytes.Buffer
	if err := AskpassClient(s.Sock, &out2); err == nil {
		t.Fatalf("second client succeeded with %q", out2.String())
	}
	if out2.Len() != 0 {
		t.Fatal("second client received bytes")
	}
}

func TestAskpassHonoursCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s, err := startAskpass(ctx, []byte(testPass))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	cancel()
	done := make(chan struct{})
	go func() { s.wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("server goroutine did not exit on cancel")
	}
	if s.served() {
		t.Fatal("served after cancel")
	}
	if err := AskpassClient(s.Sock, &bytes.Buffer{}); err == nil {
		t.Fatal("client succeeded after cancel")
	}
}

func TestAskpassSocketDirIs0700(t *testing.T) {
	s, err := startAskpass(context.Background(), []byte(testPass))
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(s.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o700 || !fi.IsDir() {
		t.Fatalf("socket dir mode = %v, want drwx------", fi.Mode())
	}
	if filepath.Dir(s.Sock) != s.Dir || filepath.Base(s.Sock) != "askpass.sock" {
		t.Fatalf("sock %q not <dir>/askpass.sock", s.Sock)
	}
	if len(s.Sock) > maxSockPath {
		t.Fatalf("sock path %d bytes > %d", len(s.Sock), maxSockPath)
	}
	_ = s.Close()
	if _, err := os.Stat(s.Dir); !os.IsNotExist(err) {
		t.Fatalf("socket dir not removed: %v", err)
	}
}

func TestSystemExecReportsExitAndStderr(t *testing.T) {
	_, err := SystemExec(context.Background(), "sh", []string{"PATH=" + os.Getenv("PATH")}, "-c", "echo boom >&2; exit 3")
	var ec interface{ ExitCode() int }
	if !errors.As(err, &ec) || ec.ExitCode() != 3 {
		t.Fatalf("err = %v, want exit code 3", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err %q lacks stderr", err)
	}
	out, err := SystemExec(context.Background(), "sh", []string{"FOO=bar"}, "-c", "echo $FOO")
	if err != nil || string(out) != "bar\n" {
		t.Fatalf("explicit env: %q %v", out, err)
	}
}

// TestLoadRealSshAddWithThrowawayAgent drives the real ssh-add against a
// private ssh-agent, with this test binary as the askpass program. goos is
// always "linux" so no test ever writes the macOS keychain.
func TestLoadRealSshAddWithThrowawayAgent(t *testing.T) {
	for _, bin := range []string{"ssh-agent", "ssh-add"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not on PATH", bin)
		}
	}
	// A short dir: macOS limits unix socket paths to ~104 bytes.
	dir, err := os.MkdirTemp("", "fa")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	agentSock := filepath.Join(dir, "agent.sock")
	agent := exec.Command("ssh-agent", "-D", "-a", agentSock)
	if err := agent.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = agent.Process.Kill(); _ = agent.Wait() })
	for i := 0; ; i++ {
		if _, err := os.Stat(agentSock); err == nil {
			break
		}
		if i > 100 {
			t.Fatal("ssh-agent socket never appeared")
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Never the operator's agent: everything below sees only the private one.
	t.Setenv("SSH_AUTH_SOCK", agentSock)

	priv, pub, err := keystore.Generate([]byte(testPass), "fleet-agent-test")
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	if _, err := keystore.Write(keyPath, priv, pub, time.Now()); err != nil {
		t.Fatal(err)
	}
	fp, err := keystore.Fingerprint(pub)
	if err != nil {
		t.Fatal(err)
	}
	self, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if ok, err := Loaded(ctx, SystemExec, fp); err != nil || ok {
		t.Fatalf("before Load: Loaded = %v, %v; want false, nil", ok, err)
	}
	if err := Load(ctx, SystemExec, self, keyPath, vault.Secret("wrong-passphrase"), "linux"); err == nil {
		t.Fatal("Load with a wrong passphrase succeeded")
	}
	if err := Load(ctx, SystemExec, self, keyPath, vault.Secret(testPass), "linux"); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ok, err := Loaded(ctx, SystemExec, fp); err != nil || !ok {
		t.Fatalf("after Load: Loaded = %v, %v; want true, nil", ok, err)
	}
}
