package observe_test

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/observe"
)

func TestNew_WritesJSONLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gsl.log")
	l := observe.New(observe.Options{Path: path, Level: "debug"})
	l.WithField("event", "test").Warn("hello")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	line := strings.TrimSpace(string(data))
	var rec map[string]any
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		t.Fatalf("not JSON: %q (%v)", line, err)
	}
	if rec["msg"] != "hello" {
		t.Fatalf("msg: want hello, got %v", rec["msg"])
	}
	if rec["event"] != "test" {
		t.Fatalf("event: want test, got %v", rec["event"])
	}
	if rec["level"] != "warning" {
		t.Fatalf("level: want warning, got %v", rec["level"])
	}
}

func TestNew_NoopOnUnwritablePath(t *testing.T) {
	// A path whose parent directory cannot be created → must NOT panic
	// and must return a usable logger.
	l := observe.New(observe.Options{Path: "/proc/cannot/write/here.log"})
	if l == nil {
		t.Fatal("New returned nil")
	}
	if l.Out != io.Discard {
		t.Fatalf("expected io.Discard output for unwritable path, got %T", l.Out)
	}
	l.Warn("should be swallowed")
}

func TestNew_LevelFilter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gsl.log")
	l := observe.New(observe.Options{Path: path, Level: "warn"})
	l.Info("info-msg")
	l.Warn("warn-msg")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if strings.Contains(string(data), "info-msg") {
		t.Fatalf("info should have been filtered, got: %s", data)
	}
	if !strings.Contains(string(data), "warn-msg") {
		t.Fatalf("warn missing: %s", data)
	}
}

func TestNew_LevelFromEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gsl.log")
	t.Setenv("GSL_LOG_LEVEL", "error")
	l := observe.New(observe.Options{Path: path})
	l.Warn("warn-msg")
	l.Error("err-msg")

	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "warn-msg") {
		t.Fatalf("warn should be filtered by GSL_LOG_LEVEL=error: %s", data)
	}
	if !strings.Contains(string(data), "err-msg") {
		t.Fatalf("err-msg missing: %s", data)
	}
}

func TestDefault_IsSingleton(t *testing.T) {
	observe.ResetDefaultForTest()
	a := observe.Default()
	b := observe.Default()
	if a != b {
		t.Fatal("Default() must return the same instance")
	}
}

func TestDefault_ResetReturnsFreshInstance(t *testing.T) {
	a := observe.Default()
	observe.ResetDefaultForTest()
	b := observe.Default()
	if a == b {
		t.Fatal("ResetDefaultForTest should yield a fresh Default instance")
	}
}

func TestNew_WriterIsLumberjack(t *testing.T) {
	dir := t.TempDir()
	l := observe.New(observe.Options{Path: filepath.Join(dir, "gsl.log")})
	if l.Out == os.Stderr || l.Out == os.Stdout || l.Out == io.Discard {
		t.Fatalf("logger Out should be a rotated file writer (lumberjack), got %T", l.Out)
	}
}
