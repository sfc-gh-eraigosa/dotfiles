// Package config loads and merges gss configuration.
//
// gss reads a single YAML file at ~/.config/gss/config.yaml. Every key is
// optional; missing keys take built-in defaults. Values resolve in
// increasing order of precedence (design.md → "Configuration";
// resolutions #3, #5):
//
//	built-in default  →  config.yaml  →  GSS_* env var  →  --flag
//
// Load() applies those layers in order and returns the effective Config.
// On first run, WriteStubIfMissing writes a fully-commented stub with all
// defaults so the user has a documented file to edit.
//
// # Dependency
//
// This package pins gopkg.in/yaml.v3 (dual MIT + Apache-2.0; see
// https://github.com/go-yaml/yaml/blob/v3.0.1/LICENSE) — both licenses are
// on the project's Allowed list (src/GEMINI.md → Library standards).
package config

import (
	"bytes"
	stderrors "errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Config mirrors ~/.config/gss/config.yaml. Each section is a nested struct
// so YAML, env, and flag layers map onto the same shape.
type Config struct {
	Paths    Paths    `yaml:"paths"`
	Worktree Worktree `yaml:"worktree"`
	Tools    Tools    `yaml:"tools"`
	Defaults Defaults `yaml:"defaults"`
	Behavior Behavior `yaml:"behavior"`
	GitHub   GitHub   `yaml:"github"`
	Suffixes Suffixes `yaml:"suffixes"`
}

// Paths holds filesystem locations. Tilde (~) values are stored verbatim;
// expansion is a downstream concern, not config's.
type Paths struct {
	WorktreeRoot string `yaml:"worktree_root"`
	RegistryDir  string `yaml:"registry_dir"`
	StateDir     string `yaml:"state_dir"`
}

// Worktree selects the worktree backend and its free-form options.
type Worktree struct {
	Backend string         `yaml:"backend"`
	Options map[string]any `yaml:"options"`
}

// Tools names the external binaries gss shells out to.
type Tools struct {
	Git string `yaml:"git"`
	GH  string `yaml:"gh"`
}

// Defaults configures new-feature defaults.
type Defaults struct {
	BaseBranch   string `yaml:"base_branch"`
	BranchPrefix string `yaml:"branch_prefix"`
	User         string `yaml:"user"`
	EngineHint   string `yaml:"engine_hint"`
}

// Behavior holds boolean policy switches.
type Behavior struct {
	AutoUpdateRefs     bool `yaml:"auto_update_refs"`
	AutoPromoteOnMerge bool `yaml:"auto_promote_on_merge"`
	ConflictScanOnList bool `yaml:"conflict_scan_on_list"`
	DeleteRemoteOnDone bool `yaml:"delete_remote_on_done"`
	ForceWithLease     bool `yaml:"force_with_lease"`
}

// GitHub holds GitHub-host resolution settings.
type GitHub struct {
	Host                  string `yaml:"host"`
	DefaultRepoResolution string `yaml:"default_repo_resolution"`
}

// Suffixes configures the random suffix-word pool.
type Suffixes struct {
	Wordlist     []string `yaml:"wordlist"`
	WordlistMode string   `yaml:"wordlist_mode"`
}

// Default returns the built-in configuration — the lowest-precedence layer.
// The values mirror the documented stub in design.md → "Configuration".
func Default() Config {
	return Config{
		Paths: Paths{
			WorktreeRoot: "~/.config/gss/worktrees",
			RegistryDir:  "~/.config/gss/worktrees",
			StateDir:     "~/.config/gss",
		},
		Worktree: Worktree{Backend: "git"},
		Tools:    Tools{Git: "git", GH: "gh"},
		Defaults: Defaults{BaseBranch: "main", BranchPrefix: "feature"},
		Behavior: Behavior{
			AutoUpdateRefs:     true,
			AutoPromoteOnMerge: true,
			ConflictScanOnList: true,
			DeleteRemoteOnDone: false,
			ForceWithLease:     true,
		},
		GitHub:   GitHub{DefaultRepoResolution: "gh"},
		Suffixes: Suffixes{Wordlist: []string{}, WordlistMode: "append"},
	}
}

// Flags carries command-line overrides. Pointer fields are sparse: a nil
// pointer means "flag not set, leave the lower layer's value". This is the
// highest-precedence layer.
type Flags struct {
	BaseBranch     *string
	User           *string
	WorktreeRoot   *string
	ForceWithLease *bool
}

// Options controls Load.
type Options struct {
	// Path is the config file to read; "" uses DefaultConfigPath().
	Path string
	// Getenv resolves GSS_* env vars; nil uses os.Getenv. Injected in tests.
	Getenv func(string) string
	// Flags are the command-line overrides, applied last.
	Flags Flags
}

// Load resolves the effective configuration through all four layers. A
// missing config file is not an error — the defaults (plus env/flags) are
// returned. A present-but-malformed file returns a *ParseError.
func Load(opts Options) (Config, error) {
	cfg := Default()

	path := opts.Path
	if path == "" {
		path = DefaultConfigPath()
	}
	switch data, err := os.ReadFile(path); {
	case err == nil:
		if err := cfg.overlayYAML(data, path); err != nil {
			return Config{}, err
		}
	case stderrors.Is(err, fs.ErrNotExist):
		// No file → defaults stand. Not an error.
	default:
		return Config{}, fmt.Errorf("config: read %s: %w", path, err)
	}

	getenv := opts.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	cfg.overlayEnv(getenv)
	cfg.overlayFlags(opts.Flags)
	return cfg, nil
}

// overlayYAML decodes data onto c, leaving keys absent from the YAML at
// their current (default) values. Unknown keys are rejected so typos
// surface as errors rather than silently no-op. An empty file is a no-op.
func (c *Config) overlayYAML(data []byte, path string) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(c); err != nil {
		if stderrors.Is(err, io.EOF) {
			return nil // empty document → all defaults
		}
		return &ParseError{Path: path, Err: err}
	}
	return nil
}

// overlayEnv applies GSS_* overrides. Only a non-empty value overrides; an
// unset or empty var leaves the lower layer intact. Bool vars that don't
// parse are ignored (the lower layer wins).
func (c *Config) overlayEnv(getenv func(string) string) {
	setStr(getenv, "GSS_WORKTREE_ROOT", &c.Paths.WorktreeRoot)
	setStr(getenv, "GSS_REGISTRY_DIR", &c.Paths.RegistryDir)
	setStr(getenv, "GSS_STATE_DIR", &c.Paths.StateDir)
	setStr(getenv, "GSS_WORKTREE_BACKEND", &c.Worktree.Backend)
	setStr(getenv, "GSS_TOOLS_GIT", &c.Tools.Git)
	setStr(getenv, "GSS_TOOLS_GH", &c.Tools.GH)
	setStr(getenv, "GSS_DEFAULTS_BASE_BRANCH", &c.Defaults.BaseBranch)
	setStr(getenv, "GSS_DEFAULTS_BRANCH_PREFIX", &c.Defaults.BranchPrefix)
	setStr(getenv, "GSS_DEFAULTS_USER", &c.Defaults.User)
	setStr(getenv, "GSS_DEFAULTS_ENGINE_HINT", &c.Defaults.EngineHint)
	setBool(getenv, "GSS_BEHAVIOR_AUTO_UPDATE_REFS", &c.Behavior.AutoUpdateRefs)
	setBool(getenv, "GSS_BEHAVIOR_AUTO_PROMOTE_ON_MERGE", &c.Behavior.AutoPromoteOnMerge)
	setBool(getenv, "GSS_BEHAVIOR_CONFLICT_SCAN_ON_LIST", &c.Behavior.ConflictScanOnList)
	setBool(getenv, "GSS_BEHAVIOR_DELETE_REMOTE_ON_DONE", &c.Behavior.DeleteRemoteOnDone)
	setBool(getenv, "GSS_BEHAVIOR_FORCE_WITH_LEASE", &c.Behavior.ForceWithLease)
	setStr(getenv, "GSS_GITHUB_HOST", &c.GitHub.Host)
	setStr(getenv, "GSS_GITHUB_DEFAULT_REPO_RESOLUTION", &c.GitHub.DefaultRepoResolution)
}

// overlayFlags applies the sparse command-line overrides (highest layer).
func (c *Config) overlayFlags(f Flags) {
	if f.BaseBranch != nil {
		c.Defaults.BaseBranch = *f.BaseBranch
	}
	if f.User != nil {
		c.Defaults.User = *f.User
	}
	if f.WorktreeRoot != nil {
		c.Paths.WorktreeRoot = *f.WorktreeRoot
	}
	if f.ForceWithLease != nil {
		c.Behavior.ForceWithLease = *f.ForceWithLease
	}
}

func setStr(getenv func(string) string, key string, dst *string) {
	if v := getenv(key); v != "" {
		*dst = v
	}
}

func setBool(getenv func(string) string, key string, dst *bool) {
	if v := getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			*dst = b
		}
	}
}

// Marshal renders the effective config back to YAML — the data `gss config
// print` writes to stdout.
func (c Config) Marshal() ([]byte, error) {
	out, err := yaml.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("config: marshal: %w", err)
	}
	return out, nil
}

// DefaultConfigPath is ~/.config/gss/config.yaml on every platform (we
// pin .config rather than os.UserConfigDir so macOS doesn't diverge to
// ~/Library/Application Support, per the design's documented path).
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".config", "gss", "config.yaml")
	}
	return filepath.Join(home, ".config", "gss", "config.yaml")
}

// WriteStubIfMissing writes a fully-commented default config to path when
// no file exists there, creating parent dirs as needed. It returns
// (true, nil) when it created the file and (false, nil) when a file was
// already present. The clock stamps the generated-at header.
func WriteStubIfMissing(path string, clock Clock) (bool, error) {
	switch _, err := os.Stat(path); {
	case err == nil:
		return false, nil // already present
	case stderrors.Is(err, fs.ErrNotExist):
		// fall through and create
	default:
		return false, fmt.Errorf("config: stat %s: %w", path, err)
	}

	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return false, fmt.Errorf("config: mkdir %s: %w", dir, err)
		}
	}
	ts := clock.Now().UTC().Format(time.RFC3339)
	content := fmt.Sprintf("# gss configuration — generated %s\n%s", ts, stubBody)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return false, fmt.Errorf("config: write %s: %w", path, err)
	}
	return true, nil
}

// ParseError wraps a YAML decode failure with the offending path so
// callers can present a structured, actionable message.
type ParseError struct {
	Path string
	Err  error
}

func (e *ParseError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("config: parse %s: %v", e.Path, e.Err)
	}
	return fmt.Sprintf("config: parse: %v", e.Err)
}

func (e *ParseError) Unwrap() error { return e.Err }

// stubBody is the commented default config (sans the generated-at header,
// which WriteStubIfMissing prepends). The uncommented values equal
// Default(), so the stub round-trips: parsing it yields the defaults.
const stubBody = `#
# All keys optional. Shown values are the built-in defaults.

paths:
  worktree_root: ~/.config/gss/worktrees      # parent of <owner>/<repo>/...
  registry_dir:  ~/.config/gss/worktrees      # registry.json sits beside worktrees
  state_dir:     ~/.config/gss                # for approval.token, caches, etc.

worktree:
  backend: git                                # git | overlayfs (future)
  options:                                    # per-backend, free-form map
    # git backend takes no options today.

tools:
  # Only tools gss itself shells out to.
  git: git
  gh:  gh

defaults:
  base_branch:   main          # default_base_branch for new features
  branch_prefix: feature       # branches: <prefix>/<name>/<user>/<purpose>...
  user:                        # override the auto-detected git/gh username
  engine_hint:                 # advisory only; gss does not launch agents

behavior:
  auto_update_refs:        true   # set rebase.updateRefs=true on every worktree
  auto_promote_on_merge:   true   # promote single direct child draft->ready on merge
  conflict_scan_on_list:   true   # gss feature list shows overlap
  delete_remote_on_done:   false  # rely on GitHub auto-delete-on-merge instead
  force_with_lease:        true   # use --force-with-lease on rebase pushes

github:
  host:                          # default: inferred from gh auth status
  default_repo_resolution: gh    # gh | origin | manual

suffixes:
  wordlist: []                   # extra words appended to the built-in 256
  wordlist_mode: append          # append | replace
`
