package log

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The guarantee every caller relies on: logging can never be the thing that
// breaks the tool. A tool that dies because it could not log is strictly worse
// than one that runs unlogged.
func TestConstructionNeverFails(t *testing.T) {
	for name, opts := range map[string]Options{
		"unwritable path":  {Tool: "x", Path: "/proc/definitely/not/writable.log"},
		"no tool, no path": {},
		"nonsense level":   {Tool: "x", Path: filepath.Join(t.TempDir(), "a.log"), Level: "banana"},
	} {
		l := New(opts)
		if l == nil {
			t.Fatalf("%s: New returned nil", name)
		}
		l.Info("must not panic") // the real assertion
	}
}

func TestLevelComesFromTheToolsOwnEnvVar(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FLEET_LOG_LEVEL", "debug")
	l := New(Options{Tool: "fleet", Path: filepath.Join(dir, "f.log")})
	if l.GetLevel().String() != "debug" {
		t.Fatalf("expected debug from FLEET_LOG_LEVEL, got %s", l.GetLevel())
	}
	// An explicit option still wins over the environment.
	l2 := New(Options{Tool: "fleet", Path: filepath.Join(dir, "g.log"), Level: "warn"})
	if l2.GetLevel().String() != "warning" {
		t.Fatalf("explicit level should win, got %s", l2.GetLevel())
	}
}

// A hyphenated tool must still yield a legal variable name.
func TestEnvNameHandlesHyphenatedTools(t *testing.T) {
	if got := envName("tmux-mgr", "LOG_FILE"); got != "TMUX_MGR_LOG_FILE" {
		t.Fatalf("got %q", got)
	}
}

func TestPathPrecedence(t *testing.T) {
	t.Run("explicit override wins", func(t *testing.T) {
		t.Setenv("FLEET_LOG_FILE", "/tmp/override.log")
		if got := ResolvePath("fleet"); got != "/tmp/override.log" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("XDG_STATE_HOME next", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("FLEET_LOG_FILE", "")
		t.Setenv("XDG_STATE_HOME", dir)
		want := filepath.Join(dir, "fleet", "fleet.log")
		if got := ResolvePath("fleet"); got != want {
			t.Fatalf("got %q want %q", got, want)
		}
		if _, err := os.Stat(filepath.Dir(want)); err != nil {
			t.Fatalf("parent directory not created: %v", err)
		}
	})
	t.Run("no tool means no path", func(t *testing.T) {
		if got := ResolvePath(""); got != "" {
			t.Fatalf("got %q", got)
		}
	})
}

// Logs echo hostnames, paths and command lines, so they must not be
// world-readable.
func TestLogFileIsOwnerOnly(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.log")
	New(Options{Tool: "x", Path: p}).Info("hello")
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != filePerm {
		t.Fatalf("log is %o, want %o", fi.Mode().Perm(), filePerm)
	}
}

func TestRecordsAreStructuredByDefaultAndTextOnRequest(t *testing.T) {
	p := filepath.Join(t.TempDir(), "j.log")
	New(Options{Tool: "x", Path: p}).WithField("host", "nano").Info("updating")
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), `"host":"nano"`) {
		t.Fatalf("expected JSON fields:\n%s", b)
	}
	p2 := filepath.Join(t.TempDir(), "t.log")
	New(Options{Tool: "x", Path: p2, Text: true}).WithField("host", "nano").Info("updating")
	b2, _ := os.ReadFile(p2)
	if strings.Contains(string(b2), `"host":"nano"`) {
		t.Fatalf("Text should not emit JSON:\n%s", b2)
	}
	if !strings.Contains(string(b2), "host=nano") {
		t.Fatalf("expected text fields:\n%s", b2)
	}
}

// The lazy singleton is what most call sites will use, so it must be safe
// before anyone has configured anything.
func TestDefaultIsUsableAndTiedToTheTool(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	ResetDefaultForTest()
	SetDefaultTool("fleet")
	l := Default()
	if l == nil {
		t.Fatal("Default must never be nil")
	}
	l.Info("hello from the default logger")
	if _, err := os.Stat(filepath.Join(dir, "fleet", "fleet.log")); err != nil {
		t.Fatalf("Default should log under the tool's state dir: %v", err)
	}
	// It is a singleton: the second call is the same logger.
	if Default() != l {
		t.Fatal("Default must return the same logger")
	}
}

// With no tool set, Default is still safe — it simply discards.
func TestDefaultWithoutAToolStillWorks(t *testing.T) {
	ResetDefaultForTest()
	SetDefaultTool("")
	Default().Warn("must not panic")
}

func TestStateDirFollowsTheSamePrecedence(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	if got, want := StateDir("fleet"), filepath.Join(dir, "fleet"); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if got := StateDir(""); got != "" {
		t.Fatalf("no tool means no state dir, got %q", got)
	}
	t.Setenv("XDG_STATE_HOME", "")
	if got := StateDir("fleet"); !strings.HasSuffix(got, filepath.Join(".local", "state", "fleet")) {
		t.Fatalf("expected the ~/.local/state fallback, got %q", got)
	}
}

// Rotation is configurable and defaulted; a caller passing nothing still gets
// a bounded file.
func TestRotationDefaultsAreApplied(t *testing.T) {
	if got := nonZero(0, 5); got != 5 {
		t.Fatalf("zero should take the default, got %d", got)
	}
	if got := nonZero(-1, 5); got != 5 {
		t.Fatalf("negative should take the default, got %d", got)
	}
	if got := nonZero(9, 5); got != 9 {
		t.Fatalf("an explicit value should win, got %d", got)
	}
}
