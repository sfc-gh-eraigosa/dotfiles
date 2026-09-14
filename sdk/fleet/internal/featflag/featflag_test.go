package featflag

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDefaultsWhenSourceErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	src := Static{Err: errors.New("boom")}

	got := Resolve(src, t.TempDir(), t.TempDir())

	if !got.Enabled {
		t.Fatalf("Enabled = false, want true on source error")
	}
	if got.ConfigPath != "" {
		t.Fatalf("ConfigPath = %q, want empty", got.ConfigPath)
	}
	if got.Note == "" {
		t.Fatalf("Note is empty, want a non-empty explanation")
	}
	if !strings.Contains(got.Note, "boom") {
		t.Fatalf("Note = %q, want it to mention the error", got.Note)
	}
}

func TestResolveHonoursDisabled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	src := Static{Bools: map[string]bool{KeyEnabled: false}}

	got := Resolve(src, t.TempDir(), t.TempDir())

	if got.Enabled {
		t.Fatalf("Enabled = true, want false")
	}
}

func TestResolveMapsHomeToEmptyPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	src := Static{Strs: map[string][]string{KeyConfig: {"home"}}}

	got := Resolve(src, t.TempDir(), t.TempDir())

	if got.ConfigPath != "" {
		t.Fatalf("ConfigPath = %q, want empty for 'home'", got.ConfigPath)
	}
}

func TestResolveMapsRepoUnderRepoDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := t.TempDir()
	src := Static{Strs: map[string][]string{KeyConfig: {"repo"}}}

	got := Resolve(src, t.TempDir(), repoDir)

	want := filepath.Join(repoDir, "opt/etc/fleet/fleet.yaml")
	if got.ConfigPath != want {
		t.Fatalf("ConfigPath = %q, want %q", got.ConfigPath, want)
	}
}

func TestResolveUnknownKeyIsFailOpen(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	sentinel := errors.New("unknown key")
	src := Static{Err: errors.New("wrapped: " + sentinel.Error())}

	got := Resolve(src, t.TempDir(), t.TempDir())

	if !got.Enabled {
		t.Fatalf("Enabled = false, want true (fail open on unknown key)")
	}
}

func TestResolveUnknownLocationIsFailOpen(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	src := Static{Strs: map[string][]string{KeyConfig: {"elsewhere"}}}

	got := Resolve(src, t.TempDir(), t.TempDir())

	if got.ConfigPath != "" {
		t.Fatalf("ConfigPath = %q, want empty for unknown location", got.ConfigPath)
	}
	if got.Note == "" {
		t.Fatalf("Note is empty, want an explanation for the unknown location")
	}
}

func TestResolveEmptyConfigSelectionIsFailOpen(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	src := Static{Strs: map[string][]string{KeyConfig: {}}}

	got := Resolve(src, t.TempDir(), t.TempDir())

	if got.ConfigPath != "" {
		t.Fatalf("ConfigPath = %q, want empty for an empty selection", got.ConfigPath)
	}
	if got.Note == "" {
		t.Fatalf("Note is empty, want an explanation for the empty selection")
	}
}

func TestResolveNilSourceUsesDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	got := Resolve(nil, t.TempDir(), t.TempDir())

	if !got.Enabled {
		t.Fatalf("Enabled = false, want true for a nil source")
	}
	if got.ConfigPath != "" {
		t.Fatalf("ConfigPath = %q, want empty for a nil source", got.ConfigPath)
	}
	if got.Note == "" {
		t.Fatalf("Note is empty, want an explanation for a nil source")
	}
}

func TestStaticMissingKeyReturnsError(t *testing.T) {
	src := Static{}

	if _, err := src.Bool(KeyEnabled); err == nil {
		t.Fatalf("Bool: want error for missing key")
	}
	if _, err := src.Strings(KeyConfig); err == nil {
		t.Fatalf("Strings: want error for missing key")
	}
}

// fleet.update.sudo-timestamp-global lets the TUI's background lane install a
// sudoers drop-in on a host where the supplied password does not reach
// install.sh's children. That mutates the host, so unlike the other flags it
// is FAIL-CLOSED: only an explicit, resolved `true` turns it on.
func TestResolveSudoTimestampGlobalIsFailClosed(t *testing.T) {
	for name, src := range map[string]Source{
		"source error":   Static{Err: errors.New("boom")},
		"key missing":    Static{Bools: map[string]bool{KeyEnabled: true}},
		"explicit false": Static{Bools: map[string]bool{KeySudoGlobal: false}},
	} {
		if Resolve(src, t.TempDir(), t.TempDir()).SudoTimestampGlobal {
			t.Fatalf("%s: SudoTimestampGlobal must be false", name)
		}
	}
	got := Resolve(Static{Bools: map[string]bool{KeySudoGlobal: true}}, t.TempDir(), t.TempDir())
	if !got.SudoTimestampGlobal {
		t.Fatal("an explicit true must turn it on")
	}
	if !got.Enabled {
		t.Fatal("the other flags keep their fail-open defaults alongside it")
	}
}
