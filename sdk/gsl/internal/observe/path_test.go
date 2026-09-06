package observe_test

import (
	"path/filepath"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/observe"
)

func TestResolveLogPath_OverrideEnv(t *testing.T) {
	dir := t.TempDir()
	custom := filepath.Join(dir, "custom.log")
	t.Setenv("GSL_LOG_FILE", custom)
	got := observe.ResolveLogPath()
	if got != custom {
		t.Fatalf("ResolveLogPath: want %q, got %q", custom, got)
	}
}

func TestResolveLogPath_XDGStateHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GSL_LOG_FILE", "")
	t.Setenv("XDG_STATE_HOME", dir)
	t.Setenv("HOME", t.TempDir())
	want := filepath.Join(dir, "gsl", "gsl.log")
	if got := observe.ResolveLogPath(); got != want {
		t.Fatalf("ResolveLogPath: want %q, got %q", want, got)
	}
}

func TestResolveLogPath_DefaultLocalState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GSL_LOG_FILE", "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", home)
	want := filepath.Join(home, ".local", "state", "gsl", "gsl.log")
	if got := observe.ResolveLogPath(); got != want {
		t.Fatalf("ResolveLogPath: want %q, got %q", want, got)
	}
}

func TestResolveLogPath_NoHome(t *testing.T) {
	t.Setenv("GSL_LOG_FILE", "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "")
	if got := observe.ResolveLogPath(); got != "" {
		t.Fatalf("ResolveLogPath: want empty, got %q", got)
	}
}
