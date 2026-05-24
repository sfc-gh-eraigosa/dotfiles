// Package config_test verifies the layered config loader per
// src/gss/docs/plan.md PR-05. TDD-first: this file fails to compile until
// config.go provides Config/Default/Load/Flags/WriteStubIfMissing.
//
// The contract under test is the four-layer precedence
// (built-in -> YAML -> env -> flag), structured errors on malformed YAML,
// first-run stub generation (with an injectable Clock), and the
// marshal/round-trip used by `gss config print`.
package config_test

import (
	stderrors "errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wenlock/dotfiles/gss/internal/config"
)

// noEnv is an empty environment, so tests aren't perturbed by ambient
// GSS_* vars on the dev host.
func noEnv(string) string { return "" }

// mapEnv returns a getenv backed by m.
func mapEnv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// writeTemp writes content to a temp file and returns its path.
func writeTemp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return p
}

func TestDefault(t *testing.T) {
	d := config.Default()
	if d.Defaults.BaseBranch != "main" {
		t.Errorf("default base_branch = %q; want main", d.Defaults.BaseBranch)
	}
	if !d.Behavior.ForceWithLease {
		t.Error("default force_with_lease = false; want true")
	}
	if d.Worktree.Backend != "git" {
		t.Errorf("default backend = %q; want git", d.Worktree.Backend)
	}
	if d.Suffixes.WordlistMode != "append" {
		t.Errorf("default wordlist_mode = %q; want append", d.Suffixes.WordlistMode)
	}
}

func TestLoad_MissingFileUsesDefaults(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.yaml")
	cfg, err := config.Load(config.Options{Path: missing, Getenv: noEnv})
	if err != nil {
		t.Fatalf("Load(missing): unexpected err: %v", err)
	}
	if cfg.Defaults.BaseBranch != "main" {
		t.Errorf("missing file should yield defaults; base_branch = %q", cfg.Defaults.BaseBranch)
	}
}

func TestLoad_YAMLOverlayKeepsOtherDefaults(t *testing.T) {
	path := writeTemp(t, "defaults:\n  base_branch: develop\n")
	cfg, err := config.Load(config.Options{Path: path, Getenv: noEnv})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Defaults.BaseBranch != "develop" {
		t.Errorf("YAML base_branch = %q; want develop", cfg.Defaults.BaseBranch)
	}
	// Untouched keys keep their built-in defaults.
	if cfg.Defaults.BranchPrefix != "feature" || !cfg.Behavior.ForceWithLease {
		t.Errorf("non-overridden keys lost their defaults: %+v", cfg)
	}
}

// TestLoad_PrecedenceLayers is the headline test: built-in < YAML < env <
// flag, checked by adding one layer at a time on the same key.
func TestLoad_PrecedenceLayers(t *testing.T) {
	yamlPath := writeTemp(t, "defaults:\n  base_branch: develop\n")
	env := map[string]string{"GSS_DEFAULTS_BASE_BRANCH": "staging"}
	flagVal := "release"

	tests := []struct {
		name string
		opts config.Options
		want string
	}{
		{"built-in only", config.Options{Path: "/no/such/file", Getenv: noEnv}, "main"},
		{"yaml over built-in", config.Options{Path: yamlPath, Getenv: noEnv}, "develop"},
		{"env over yaml", config.Options{Path: yamlPath, Getenv: mapEnv(env)}, "staging"},
		{"flag over env", config.Options{Path: yamlPath, Getenv: mapEnv(env), Flags: config.Flags{BaseBranch: &flagVal}}, "release"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := config.Load(tc.opts)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.Defaults.BaseBranch != tc.want {
				t.Errorf("base_branch = %q; want %q", cfg.Defaults.BaseBranch, tc.want)
			}
		})
	}
}

func TestLoad_EnvForceWithLeaseFalse(t *testing.T) {
	cfg, err := config.Load(config.Options{
		Path:   "/no/such/file",
		Getenv: mapEnv(map[string]string{"GSS_BEHAVIOR_FORCE_WITH_LEASE": "false"}),
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Behavior.ForceWithLease {
		t.Error("GSS_BEHAVIOR_FORCE_WITH_LEASE=false did not override the default true")
	}
}

func TestLoad_EnvWorktreeRoot(t *testing.T) {
	cfg, err := config.Load(config.Options{
		Path:   "/no/such/file",
		Getenv: mapEnv(map[string]string{"GSS_WORKTREE_ROOT": "/tmp/wt"}),
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Paths.WorktreeRoot != "/tmp/wt" {
		t.Errorf("worktree_root = %q; want /tmp/wt", cfg.Paths.WorktreeRoot)
	}
}

func TestLoad_InvalidYAMLStructuredError(t *testing.T) {
	path := writeTemp(t, "defaults:\n  base_branch: : : :\n  bad indent\n")
	_, err := config.Load(config.Options{Path: path, Getenv: noEnv})
	if err == nil {
		t.Fatal("Load(malformed yaml): err = nil; want a parse error")
	}
	var pe *config.ParseError
	if !stderrors.As(err, &pe) {
		t.Fatalf("err type = %T (%v); want *config.ParseError", err, err)
	}
	if pe.Path != path {
		t.Errorf("ParseError.Path = %q; want %q", pe.Path, path)
	}
	if pe.Unwrap() == nil {
		t.Error("ParseError.Unwrap() = nil; want the underlying yaml error")
	}
}

func TestLoad_UnknownFieldRejected(t *testing.T) {
	path := writeTemp(t, "defaults:\n  base_brunch: typo\n")
	_, err := config.Load(config.Options{Path: path, Getenv: noEnv})
	if err == nil {
		t.Error("Load(unknown key): err = nil; want rejection of the typo'd key")
	}
}

func TestLoad_EmptyFileUsesDefaults(t *testing.T) {
	path := writeTemp(t, "")
	cfg, err := config.Load(config.Options{Path: path, Getenv: noEnv})
	if err != nil {
		t.Fatalf("Load(empty file): %v", err)
	}
	if !marshalEqual(t, cfg, config.Default()) {
		t.Error("empty file should yield Default()")
	}
}

func TestLoad_AllFlagsOverride(t *testing.T) {
	base, user, root := "rel", "erai", "/tmp/wt"
	fwl := false
	cfg, err := config.Load(config.Options{
		Path:   "/no/such/file",
		Getenv: noEnv,
		Flags: config.Flags{
			BaseBranch:     &base,
			User:           &user,
			WorktreeRoot:   &root,
			ForceWithLease: &fwl,
		},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Defaults.BaseBranch != "rel" || cfg.Defaults.User != "erai" ||
		cfg.Paths.WorktreeRoot != "/tmp/wt" || cfg.Behavior.ForceWithLease {
		t.Errorf("flag overrides not all applied: %+v", cfg)
	}
}

func TestDefaultConfigPath(t *testing.T) {
	got := config.DefaultConfigPath()
	suffix := filepath.Join("gss", "config.yaml")
	if !contains(got, suffix) {
		t.Errorf("DefaultConfigPath() = %q; want it to end in %q", got, suffix)
	}
}

func TestParseError_Error(t *testing.T) {
	withPath := &config.ParseError{Path: "/etc/gss.yaml", Err: stderrors.New("boom")}
	if !contains(withPath.Error(), "/etc/gss.yaml") || !contains(withPath.Error(), "boom") {
		t.Errorf("ParseError.Error() = %q; want path + cause", withPath.Error())
	}
	bare := &config.ParseError{Err: stderrors.New("boom")}
	if contains(bare.Error(), "  ") {
		t.Errorf("bare ParseError.Error() malformed: %q", bare.Error())
	}
	if !stderrors.Is(withPath, withPath.Err) {
		t.Error("ParseError should unwrap to its cause")
	}
}

// fixedClock is a deterministic Clock for stub-stamping tests.
type fixedClock struct{ t time.Time }

func (f fixedClock) Now() time.Time { return f.t }

func TestWriteStubIfMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.yaml")
	clock := fixedClock{t: time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)}

	created, err := config.WriteStubIfMissing(path, clock)
	if err != nil {
		t.Fatalf("WriteStubIfMissing: %v", err)
	}
	if !created {
		t.Fatal("first call returned created=false; want true")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read stub: %v", err)
	}
	if !contains(string(data), "2026-05-21T12:00:00Z") {
		t.Errorf("stub missing injected timestamp; got:\n%s", data)
	}

	// The stub must round-trip: loading it yields the built-in defaults.
	cfg, err := config.Load(config.Options{Path: path, Getenv: noEnv})
	if err != nil {
		t.Fatalf("Load(stub): %v", err)
	}
	if !marshalEqual(t, cfg, config.Default()) {
		t.Error("stub did not round-trip to Default()")
	}

	// Idempotent: a second call must not overwrite and must report created=false.
	created2, err := config.WriteStubIfMissing(path, clock)
	if err != nil {
		t.Fatalf("WriteStubIfMissing (2nd): %v", err)
	}
	if created2 {
		t.Error("second call returned created=true; want false (file already present)")
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	d := config.Default()
	out, err := d.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	path := writeTemp(t, string(out))
	got, err := config.Load(config.Options{Path: path, Getenv: noEnv})
	if err != nil {
		t.Fatalf("Load(marshalled): %v", err)
	}
	if !marshalEqual(t, got, d) {
		t.Errorf("round-trip mismatch:\n--- got ---\n%s", out)
	}
}

// marshalEqual compares two configs by their YAML encoding, which
// normalises nil-vs-empty slice/map differences.
func marshalEqual(t *testing.T, a, b config.Config) bool {
	t.Helper()
	ab, err := a.Marshal()
	if err != nil {
		t.Fatalf("marshal a: %v", err)
	}
	bb, err := b.Marshal()
	if err != nil {
		t.Fatalf("marshal b: %v", err)
	}
	return string(ab) == string(bb)
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
