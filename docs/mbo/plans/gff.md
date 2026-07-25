# gff — git fast features — implementation plan

- **Slug:** gff
- **Date:** 2026-07-23
- **Status:** Draft
- **Relates to:** spec `../specs/gff.md` · design `../designs/gff.md` · issue #180 · PR #181

> **For agentic workers:** REQUIRED SUB-SKILL: use `superpowers:subagent-driven-development`
> (recommended) or `superpowers:executing-plans` to implement task-by-task. Steps use
> checkbox (`- [ ]`) syntax. TDD throughout: test → red → code → green → commit.

**Goal:** Build `sdk/gff`, a generic git-persisted feature-flag engine (proto schema,
layered resolver, cobra CLI, bubbletea TUI, Go SDK), and instrument `install.sh` +
the Windows PowerShell phases with per-component flags, all defaulting on.

**Architecture:** proto defines only the *shape* (FeatureSet/Feature/bool/choice);
flag *data* lives in each repo's tracked flag file, discovered by probing
`.gff/features.yaml` then `.github/gff/features.yaml` (dotfiles uses the latter —
keeps the repo root clean). A 5-layer resolver
(system snapshot → user snapshot → live repo file → system override → user override)
computes effective values with winning-layer attribution. Shell consumes flags via
`gff export --shell` env vars + a fail-open `gff_on` helper.

**Tech stack:** Go 1.26.1 (repo `.go-version`), `google.golang.org/protobuf`
(+ system `protoc` with a go.mod-pinned `protoc-gen-go` — raw protoc behind make
targets, **no buf** (rejected as unreliable); protoc needed only to regenerate since
output is committed), cobra, `gopkg.in/yaml.v3`, bubbletea
(+ bubbles/lipgloss, P3 only), bash + PowerShell touch-points.

## Global constraints

- Module path `github.com/sfc-gh-eraigosa/dotfiles/sdk/gff`; go directive `1.26.1`.
- Mirror `sdk/gss`/`sdk/gsl` layout: `cmd/`, `internal/`, `main.go`, `VERSION`,
  `build.sh` stamping `internal/version` via ldflags only (no build vars in `cmd`/`main`).
- **≥90% Go unit-test coverage** for `sdk/gff` — this objective's own bar, enforced in
  `gff-ci.yml` (the repo-wide `sdk/` floor is 60%; gff exceeds it — see §7 validation
  plan). Table-driven tests; no network in tests.
- Generated proto Go code is **committed** under `gen/gff/v1/` (Go package name
  `gffv1`, import path `…/sdk/gff/gen/gff/v1`); regeneration must be clean in CI.
- Bool flags: `true`=on, `false`=off; lint rejects negative names (`no-*`, `not-*`,
  `disable-*`, `disabled-*`, `skip-*`, `off-*`).
- Canonical keys: exactly 3 dotted segments `area.component.feature`, each segment
  `[a-z0-9]+(-[a-z0-9]+)*`.
- Env mangling: uppercase, `.` and `-` → `_` (e.g. `install.windows.wispr-flow` →
  `GFF_INSTALL_WINDOWS_WISPR_FLOW`).
- `gff enabled` / `gff selected` exit codes: 0=on/selected, 1=off/not-selected,
  2=unknown key or option id. All other verbs: 0 ok, 1 error.
- Writes go ONLY to `~/.config/gff/` (config.yaml 0600, sources.yaml). Never write
  repo or system files.
- Every shell edit obeys `docs/mbo/specs/shell-portability.md`; run `make lint-shell`
  and `make lint-portability` before each shell commit. All shell gates FAIL OPEN.
- Per-directory docs rule: new documented dirs get `AGENTS.md` + `CLAUDE.md → AGENTS.md`.
- Commits end with `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`; stage
  files by explicit name.

## 1. Summary & verdict

Four phases, each an independently landable PR: **P1** engine (proto + schema + paths +
resolver + registry + core CLI + SDK + CI), **P2** dotfiles instrumentation
(`.github/gff/features.yaml` inventory, `opt/lib/gff.sh`, install.sh + PowerShell gating),
**P3** TUI, **P4** `gff gen` typed accessors. P1 blocks the rest; P2/P3/P4 are
path-disjoint after P1 (see §6.1). Spec coverage verified in §5.

## 2. File inventory

| Path | Purpose | Implements |
| :-- | :-- | :-- |
| `sdk/gff/go.mod`, `go.sum`, `main.go`, `VERSION`, `build.sh`, `README.md`, `AGENTS.md`, `CLAUDE.md→AGENTS.md`, `LICENSE` | module scaffold, build + version stamping | spec §3 |
| `sdk/gff/internal/version/version.go` | ldflags-stamped version vars | repo sdk convention |
| `sdk/gff/proto/gff/v1/features.proto` | generic schema messages | F1–F3, design §4 |
| `sdk/gff/scripts/genproto.sh` + root `Makefile` (`gff-proto`, `gff-proto-check`) | raw-protoc codegen, go.mod-pinned `protoc-gen-go`, committed output | spec §3 proto |
| `sdk/gff/gen/gff/v1/*.pb.go` | committed generated code | spec §3 proto |
| `sdk/gff/internal/schema/{schema.go,lint.go,schema_test.go,lint_test.go}` | load/validate/lint features files | F1–F3, F5 (format) |
| `sdk/gff/internal/paths/paths.go` (+test) | well-known layer paths, overridable for tests | F4 |
| `sdk/gff/internal/gitx/{gitx.go,gitx_test.go}` | repo-root discovery + `gff.source` redirect; mockable runner | F5 |
| `sdk/gff/internal/resolve/{resolve.go,resolve_test.go}` | 5-layer merge, provenance, unknown-key | F4 |
| `sdk/gff/internal/registry/{registry.go,registry_test.go}` | sources.yaml, exclusive area claims, snapshots | F6 |
| `sdk/gff/cmd/{root,version,get,enabled,set,unset,list,lint,export,install}.go` (+tests) | cobra verbs | F4–F8 |
| `sdk/gff/pkg/gff/{gff.go,gff_test.go}` | public runtime SDK | UC4 |
| `.github/workflows/gff-ci.yml` | go vet+test+cover gate, proto-regen-clean | spec §6 |
| `.github/gff/features.yaml` | dotfiles flag inventory (all on; probe path 2 — repo root stays clean) | F9 |
| `.gitignore` | verify only — `!.github/**` already opts the inventory in (no new rules; `git check-ignore -v` to confirm) | F9 (allowlist gotcha) |
| `sdk/gff/e2e/e2e_test.go`, `sdk/gff/scripts/e2e.sh` | binary-level integration harness (§7.2 scenarios) | §7 validation |
| `sdk/gff/scripts/demo.sh` | narrated end-to-end demo (§7.3) | §7 validation, VD-1 |
| `opt/lib/gff.sh` (+ `opt/lib/gff_test.sh`) | POSIX fail-open `gff_on` helper + test driver | F9, UC1 |
| `install.sh` | gff build block after Go toolchain; `eval export`; per-step gates | F9 |
| `opt/bin/install_windows.sh` | WSLENV pass-through of `GFF_INSTALL_WINDOWS_*` | F9 |
| `opt/Desktop/Apps/scripts/lib/gff.ps1` | `Test-GffOn` PS helper (unset ⇒ on) | F9 |
| `opt/Desktop/Apps/scripts/{setup-apps.ps1,setup-elevated.ps1}` | phase gating | F9 |
| `sdk/gff/internal/tui/{model.go,view.go,tui_test.go}` + `cmd/tui.go` | bubbletea TUI | F10, UC2 |
| `sdk/gff/cmd/gen.go` (+ golden tests) | typed-accessor codegen | UC4 (P4) |

## 3. Interface contracts (frozen)

### 3.1 Proto (`proto/gff/v1/features.proto`) — the whole schema

```proto
syntax = "proto3";
package gff.v1;
option go_package = "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1;gffv1";

// A repo's tracked flag definitions (the repo flag file = YAML/protojson encoding).
message FeatureFile { repeated FeatureSet sets = 1; }
message FeatureSet {
  string area = 1;                 // claimed exclusively per machine
  repeated Feature features = 2;
}
message Feature {
  string path = 1;                 // canonical: area.component.feature
  string description = 2;
  oneof default {
    bool bool_default = 3;         // true is always ON; no negative names
    ChoiceDefault choice_default = 4;
  }
}
enum ChoiceMode {
  CHOICE_MODE_UNSPECIFIED = 0;
  CHOICE_MODE_SINGLE = 1;          // radio: exactly one option selected
  CHOICE_MODE_MULTI = 2;           // checkbox: zero or more options selected
}
message ChoiceDefault {
  ChoiceMode mode = 1;
  repeated ChoiceOption options = 2;
}
message ChoiceOption {
  string id = 1;                   // stable kebab-case slug, unique within the feature
  string description = 2;          // human description of this option
  bool selected = 3;               // default selection state
  oneof value {                    // typed payload; ONE type per feature (lint-enforced)
    int64 int_value = 4;
    double float_value = 5;
    string string_value = 6;
    bool bool_value = 7;
  }
}
// Sparse override value (override files decode into this).
message Value {
  oneof kind { bool bool_value = 1; ChoiceSelection choice_value = 2; }
}
message ChoiceSelection { repeated string selected = 1; } // option ids; exactly 1 when SINGLE
message Source {                   // one registered repo
  string name = 1; string url = 2;
  repeated string areas = 3; string commit = 4;
}
message SourceRegistry { repeated Source sources = 1; }
```

### 3.2 File formats

The repo flag file — `.gff/features.yaml` or `.github/gff/features.yaml`, probed in
that order (`.gff/` wins when both exist); protojson field names, YAML syntax:

```yaml
sets:
  - area: install
    features:
      - path: install.windows.wispr-flow
        description: Wispr Flow MSI + AHK dictation workflow (Windows)
        boolDefault: true
      - path: install.pkg.manager        # single-select choice (radio)
        description: Package manager selection
        choiceDefault:
          mode: CHOICE_MODE_SINGLE
          options:
            - {id: auto, description: Auto-detect, stringValue: auto, selected: true}
            - {id: apt,  description: Debian/Ubuntu apt, stringValue: apt}
            - {id: brew, description: Homebrew, stringValue: brew}
```

Override files (`/var/opt/conf/gff/config.yaml`, `~/.config/gff/config.yaml`) are a
**plain scalar map** for hand-editability (decoded into `map[string]*gffv1.Value`;
bool → BoolValue, string or string-list → ChoiceSelection; anything else = parse error):

```yaml
install.windows.wispr-flow: false
install.pkg.manager: apt            # single-select: exactly one option id
shell.zsh.plugins: [fzf, starship]  # multi-select: zero or more option ids
```

`~/.config/gff/sources.yaml` = YAML encoding of `SourceRegistry`
(`sources: [{name, url, areas, commit}]`). Snapshots are verbatim copies of the
source's features file at `<layer>/gff/<source-name>.yaml`.

### 3.3 Go interfaces

```go
// internal/paths
type Paths struct {
    SystemSnapshotDir string // /opt/conf/gff
    UserSnapshotDir   string // $HOME/opt/conf/gff
    SystemOverride    string // /var/opt/conf/gff/config.yaml
    UserOverride      string // $HOME/.config/gff/config.yaml
    RegistryFile      string // $HOME/.config/gff/sources.yaml
    WorkDir           string // CWD for repo discovery
}
func Default() (Paths, error)

// internal/gitx — mockable like gss's runner
type Runner interface { Output(dir string, args ...string) (string, error) }
func RepoRoot(startDir string) (string, bool)              // walk up to .git (dir OR file)
func SourcePath(r Runner, repoRoot string) string          // `git config gff.source` redirect,
                                                           // else probe .gff/features.yaml,
                                                           // then .github/gff/features.yaml

// internal/schema
func LoadFeatureFile(path string) (*gffv1.FeatureFile, error)   // .yaml|.yml|.json
func LoadOverrides(path string) (map[string]*gffv1.Value, error) // missing file => empty map, nil err
func Lint(f *gffv1.FeatureFile) []Finding                        // Finding{Path, Rule, Msg}

// internal/resolve
type Layer int
const ( LayerNone Layer = iota; LayerSystemSnapshot; LayerUserSnapshot
        LayerRepoLive; LayerSystemOverride; LayerUserOverride )
type Resolved struct {
    Feature *gffv1.Feature // definition (description, default, choice options)
    Value   *gffv1.Value   // effective value
    Layer   Layer          // layer that set the effective value (Def layers => default)
}
var ErrUnknownKey = errors.New("unknown flag key")
var ErrUnknownSource = errors.New("unknown source")
type Resolver struct {
    P paths.Paths; R gitx.Runner
    Source string // "" = CWD discovery; else a registered source name or local repo path
}
func (r *Resolver) All() ([]Resolved, error)          // sorted by Feature.Path
func (r *Resolver) Resolve(key string) (Resolved, error)

// internal/registry
type Registry struct { P paths.Paths }
var ErrAreaClaimed = errors.New("area already claimed")     // wraps owner name
func (g *Registry) Install(repoRoot, name, url, commit string, ff *gffv1.FeatureFile) error
func (g *Registry) Sources() ([]*gffv1.Source, error)

// pkg/gff — public SDK
func Bool(key string) (bool, error)                 // full-chain resolve from Default() paths
func Selected(key string) ([]string, error)         // effective selected option ids
func IsSelected(key, optionID string) (bool, error) // one radio/checkbox state; unknown id = error
// typed payloads of the selected options (feature's value type is lint-homogeneous;
// wrong-type accessor = error naming the actual type):
func IntValues(key string) ([]int64, error)
func FloatValues(key string) ([]float64, error)
func StringValues(key string) ([]string, error)
func BoolValues(key string) ([]bool, error)
```

### 3.4 CLI contract

```
gff get <key>            -> prints "true"|"false"|<id[,id...]>; exit 2 unknown key
gff enabled <key>        -> no output; exit 0 on / 1 off / 2 unknown (choice: exit 2 + stderr)
gff selected <key> <id>  -> no output; exit 0 selected / 1 not / 2 unknown key OR option id
gff set <key> <value>    -> writes ~/.config/gff/config.yaml (0600); bool: true|false;
                            choice: id or comma-list of ids (single mode: exactly one)
gff unset <key>          -> removes key from user override
gff list [--json]        -> table: PATH TYPE VALUE LAYER DESCRIPTION; --json = []Resolved
gff lint [path]          -> lints features file (default: discovered repo file); exit 1 on findings
gff export --format shell|dotenv|json|yaml [-o <file>]
                         -> shell:  eval-able GFF_<MANGLED>=<value> lines (stdout)
                            dotenv: identical KEY=value lines, default -o .env; must
                                    parse with dotenv-family libs (hashicorp
                                    go-envparse, joho/godotenv, python-dotenv, …)
                            json:   full resolved snapshot (list --json shape incl.
                                    typed choice payloads) — the bridge artifact for
                                    non-Go languages
                            yaml:   the same resolved snapshot, YAML encoding (same
                                    protojson-normalize trick as schema — one struct,
                                    two encodings)
                            values are bool literals or comma-joined lint-constrained
                            kebab option ids — injection-safe by construction;
                            --shell is kept as an alias for --format shell
gff install              -> registers CWD repo + snapshot into user layer; exit 1 on area clash
gff version              -> gss-style version block
```

**Global flag (all read verbs):** `--source <name|path>` scopes resolution to one
source instead of CWD discovery — a registered *name* resolves that source's snapshot
layers (plus its live clone when CWD happens to be inside it); a local *path* is used
as the repo root for the live layer. Unknown name/path ⇒ `ErrUnknownSource`, exit 2.
Purely local; never a network fetch at resolution time.

**Zero-install invocation (first-class, CI-smoked):** every recipe that works with an
installed `gff` must also work as
`go run github.com/sfc-gh-eraigosa/dotfiles/sdk/gff@<tag> <verb> …` — the `main.go`
at the module root makes the module `go run`-able; `tag-sdk-modules.yml` cuts pinned
`sdk/gff/vX.Y.Z` tags (pin one for reproducibility; `@latest` is the convenience
form). Canonical example, needing only a Go toolchain + module proxy:

```sh
eval "$(go run github.com/sfc-gh-eraigosa/dotfiles/sdk/gff@v0.1.0 export --format shell --source dotfiles)"
```

`go run` build/download noise goes to stderr, stdout carries only export lines, so
`eval` stays safe.

### 3.5 Shell contract (`opt/lib/gff.sh`)

```sh
gff_on <key>   # returns 0 (run the step) unless the mangled env var is exactly "false"
GFF_CMD        # optional: how scripts invoke gff for export/refresh (default: gff on PATH);
               # e.g. GFF_CMD="go run github.com/sfc-gh-eraigosa/dotfiles/sdk/gff@v0.1.0"
               # — gff_on itself stays env-only/fail-open and never invokes a binary
```

Windows: bash appends each `GFF_INSTALL_WINDOWS_*` name to `WSLENV` (`/u` flag) before
invoking `powershell.exe`; `lib/gff.ps1` provides `Test-GffOn "install.windows.x"`
(true unless env var is exactly `"false"`).

## 4. TDD build order

Tasks are bite-sized; every code step shows the real code or the exact content sketch a
skilled Go engineer needs. Run all Go commands from `sdk/gff/`.

---

### P1-T1: module scaffold + version

**Files:** create `sdk/gff/{go.mod,main.go,VERSION,build.sh,README.md,AGENTS.md,LICENSE}`,
`sdk/gff/cmd/{root.go,version.go,root_test.go}`, `sdk/gff/internal/version/version.go`;
symlink `sdk/gff/CLAUDE.md → AGENTS.md`.

**Interfaces:** produces `cmd.Execute()`, `cmd.NewRootCmd() *cobra.Command` (all later
verbs register on it in their own files via `rootCmd.AddCommand` in `init()` — copy the
`sdk/gsl/cmd` pattern).

- [ ] Copy `sdk/gsl/build.sh` to `sdk/gff/build.sh`; replace every `gsl` with `gff`.
      `VERSION` = `0.1.0`. `LICENSE` = copy of `sdk/gsl/LICENSE`.
- [ ] `go.mod`: module `github.com/sfc-gh-eraigosa/dotfiles/sdk/gff`, `go 1.26.1`.
- [ ] `internal/version/version.go`: vars `Version, Commit, BuildDate, Dirty = "dev","none","unknown","false"`
      + `func String() string` formatting them (mirror gsl's).
- [ ] Failing test `cmd/root_test.go`:

```go
func TestVersionCommand(t *testing.T) {
    var out bytes.Buffer
    cmd := NewRootCmd()
    cmd.SetOut(&out); cmd.SetErr(&out)
    cmd.SetArgs([]string{"version"})
    if err := cmd.Execute(); err != nil { t.Fatal(err) }
    if !strings.Contains(out.String(), "gff") { t.Fatalf("want version output, got %q", out.String()) }
}
```

- [ ] Run `go test ./cmd/` → FAIL (NewRootCmd undefined).
- [ ] Implement `cmd/root.go` (`NewRootCmd` returns cobra root, Use `gff`, Short
      "git fast features — layered feature flags persisted in git") + `cmd/version.go`
      printing `version.String()`; `main.go` calls `cmd.Execute()`.
- [ ] `go test ./... && go vet ./...` → PASS. `bash build.sh` → installs `~/opt/bin/gff`.
- [ ] Commit: `feat(gff): scaffold sdk/gff module with cobra root + version`

### P1-T2: proto schema + committed codegen (raw protoc, no buf)

**Files:** create `sdk/gff/proto/gff/v1/features.proto` (§3.1 verbatim),
`sdk/gff/scripts/genproto.sh`, committed `sdk/gff/gen/gff/v1/*.pb.go`; modify root
`Makefile` (add `gff-proto`, `gff-proto-check` targets) and `.gitignore` (ignore
`sdk/gff/.bin/` with a comment — `!sdk/**` would otherwise track the plugin binary).

**Interfaces:** produces package `gen/gffv1` (all messages in §3.1).

- [ ] Deps: `go get google.golang.org/protobuf@latest && go get -tool google.golang.org/protobuf/cmd/protoc-gen-go@latest`
      — the `-tool` entry pins protoc-gen-go's version in go.mod. System `protoc`
      (`apt-get install protobuf-compiler` / `brew install protobuf`) is required only
      for REGENERATION; contributors build from the committed output.
- [ ] `scripts/genproto.sh` — raw protoc, module-pinned plugin, no buf:

```bash
#!/usr/bin/env bash
set -euo pipefail
cd "$(cd "$(dirname "$0")/.." && pwd -P)"
command -v protoc >/dev/null 2>&1 || {
  echo "protoc not found — apt-get install protobuf-compiler / brew install protobuf" >&2
  exit 1
}
GOBIN="${PWD}/.bin" go install google.golang.org/protobuf/cmd/protoc-gen-go  # version from go.mod
PATH="${PWD}/.bin:${PATH}" protoc \
  --proto_path=proto \
  --go_out=gen --go_opt=paths=source_relative \
  proto/gff/v1/features.proto
```

  With `paths=source_relative` + the `;gffv1` go_package suffix, output lands at
  `gen/gff/v1/features.pb.go`, package `gffv1` — no post-processing.
- [ ] Root `Makefile` targets: `gff-proto` → `bash sdk/gff/scripts/genproto.sh`;
      `gff-proto-check` → runs `gff-proto` then `git diff --exit-code -- sdk/gff/gen/`.
- [ ] Write `features.proto` exactly as §3.1. `make gff-proto` →
      `gen/gff/v1/features.pb.go` exists; `go build ./...` PASS.
- [ ] Sanity: `go vet ./...` PASS (round-trip covered by P1-T3 tests).
- [ ] `make gff-proto-check` → clean (regeneration idempotent).
- [ ] Commit: `feat(gff): proto v1 schema + raw-protoc codegen behind make gff-proto (committed output)`

### P1-T3: schema load + lint

**Files:** create `internal/schema/{schema.go,lint.go,schema_test.go,lint_test.go}`.

**Interfaces:** consumes `gen/gffv1`; produces §3.3 `LoadFeatureFile`, `LoadOverrides`, `Lint`.

- [ ] Failing tests first — `schema_test.go` (temp-dir fixtures):

```go
func TestLoadFeatureFileYAML(t *testing.T) {
    p := writeFile(t, "features.yaml", `
sets:
  - area: install
    features:
      - {path: install.ai.claude, description: Claude CLI, boolDefault: true}
      - path: install.pkg.manager
        description: Package manager
        choiceDefault:
          mode: CHOICE_MODE_SINGLE
          options:
            - {id: auto, description: Auto-detect, stringValue: auto, selected: true}
            - {id: apt, description: Debian/Ubuntu apt, stringValue: apt}
`)
    ff, err := LoadFeatureFile(p)
    if err != nil { t.Fatal(err) }
    if got := ff.Sets[0].Features[0].GetBoolDefault(); !got { t.Fatal("bool default") }
    cd := ff.Sets[0].Features[1].GetChoiceDefault()
    if cd.Mode != gffv1.ChoiceMode_CHOICE_MODE_SINGLE { t.Fatal("mode") }
    if cd.Options[1].Id != "apt" || cd.Options[1].GetStringValue() != "apt" { t.Fatal("choice option") }
    if !cd.Options[0].Selected { t.Fatal("default selection") }
}
func TestLoadOverrides(t *testing.T) {
    p := writeFile(t, "config.yaml",
        "install.ai.claude: false\ninstall.pkg.manager: apt\nshell.zsh.plugins: [fzf, starship]\n")
    o, err := LoadOverrides(p)
    if err != nil { t.Fatal(err) }
    if o["install.ai.claude"].GetBoolValue() != false { t.Fatal("bool override") }
    if got := o["install.pkg.manager"].GetChoiceValue().Selected; len(got) != 1 || got[0] != "apt" { t.Fatal("single choice override") }
    if got := o["shell.zsh.plugins"].GetChoiceValue().Selected; len(got) != 2 { t.Fatal("multi choice override") }
}
func TestLoadOverridesMissingFile(t *testing.T) { // sparse layer absent => empty, no error
    o, err := LoadOverrides(filepath.Join(t.TempDir(), "nope.yaml"))
    if err != nil || len(o) != 0 { t.Fatalf("want empty/nil, got %v/%v", o, err) }
}
```

  `lint_test.go` table test — cases (rule, input, wantFinding): duplicate path;
  negative name each prefix (`install.ai.no-claude` etc.); depth 2 (`install.claude`)
  and 4; uppercase/underscore segment; choice: empty `options`, duplicate/non-kebab
  option ids, mixed value types within one feature, `single` mode with zero or two
  default-selected options, `mode` unspecified; path not starting with the set's
  `area.`; clean file ⇒ no findings.
- [ ] `go test ./internal/schema/` → FAIL.
- [ ] Implement. `LoadFeatureFile`: read file; if `.json` → `protojson.Unmarshal`;
      else `yaml.Unmarshal` into `any`, normalize map keys to strings
      (`map[string]any` recursively, ints→strings for option keys), `json.Marshal`,
      `protojson.Unmarshal` with `DiscardUnknown: false`. `LoadOverrides`:
      `os.IsNotExist ⇒ (map{}, nil)`; yaml scalar map; bool→BoolValue,
      int→ChoiceValue, else error naming the key. `Lint`: iterate with a
      `map[string]bool` seen-set; regex `^[a-z0-9]+(-[a-z0-9]+)*$` per segment;
      negative prefixes from Global Constraints on the last segment.
- [ ] `go test ./internal/schema/ -cover` → PASS, ≥90% for this package.
- [ ] Commit: `feat(gff): schema loader (yaml/json protojson) + lint rules`

### P1-T4: paths + git discovery

**Files:** create `internal/paths/paths.go` (+`paths_test.go`), `internal/gitx/{gitx.go,gitx_test.go}`.

**Interfaces:** produces §3.3 `Paths`, `Default()`, `Runner`, `RepoRoot`, `SourcePath`.

- [ ] Failing tests: `Default()` fields end with the exact §3.3 suffixes (use
      `os.UserHomeDir`); `RepoRoot`: temp dir tree `a/b/c` with `.git` **dir** at `a`
      → found from `c`; `.git` **file** (worktree case: `gitdir: ...`) at `a` → found;
      no `.git` anywhere → `("", false)`. `SourcePath`: fake Runner returning
      `custom/flags.yaml` → joined path; Runner returning error → probe order: repo
      with only `.gff/features.yaml` → that path; only `.github/gff/features.yaml` →
      that path; both present → `.gff/features.yaml` wins; neither → `.gff/features.yaml`
      (resolver treats the missing live layer as absent).

```go
type fakeRunner struct{ out string; err error }
func (f fakeRunner) Output(dir string, args ...string) (string, error) { return f.out, f.err }
```

- [ ] Implement. `RepoRoot`: `os.Stat(filepath.Join(dir, ".git"))` accepting dir or
      file, walking to filesystem root. `SourcePath`: `r.Output(repoRoot, "config",
      "--get", "gff.source")`, trim; relative ⇒ join to repoRoot. Real runner execs
      `git -C <dir> <args...>` via `os/exec`.
- [ ] `go test ./internal/paths/ ./internal/gitx/` → PASS.
- [ ] Commit: `feat(gff): well-known paths + git-style repo discovery with gff.source redirect`

### P1-T5: resolver (the core)

**Files:** create `internal/resolve/{resolve.go,resolve_test.go}`.

**Interfaces:** consumes schema/paths/gitx; produces §3.3 `Resolver`, `Resolved`,
`Layer`, `ErrUnknownKey`.

- [ ] Failing matrix test. Helper builds a full fake world in `t.TempDir()`:

```go
// world lays out: sysSnap/gff/src.yaml, userSnap/gff/src.yaml, repo/.gff/features.yaml,
// sysOverride config.yaml, userOverride config.yaml — each optional per case.
type world struct{ sysSnap, userSnap, repo, sysOvr, usrOvr string } // file contents ("" = absent)
func newResolver(t *testing.T, w world) *Resolver { /* writes files, returns Resolver with Paths pointing at temp dirs, WorkDir=repo */ }
```

  Cases (each asserts `.Value` AND `.Layer`):
  1. key only in system snapshot ⇒ default, `LayerSystemSnapshot`
  2. same key in user snapshot too ⇒ user snapshot def wins, `LayerUserSnapshot`
  3. live repo file redefines default ⇒ `LayerRepoLive`
  4. system override flips ⇒ `LayerSystemOverride`
  5. user override flips back ⇒ `LayerUserOverride`
  6. override for UNKNOWN key ⇒ ignored by `All()`, `Resolve` ⇒ `ErrUnknownKey`
  7. choice flag: default selection wins with no override; override to valid id(s) ⇒
     that selection; override naming an unknown id, or two ids on a `single`-mode
     flag ⇒ error naming the key and the offending ids
  8. no repo (WorkDir outside any git repo) ⇒ snapshots+overrides still resolve
  9. `All()` sorted by path; sparse overrides never invent keys
  10. `Source: <local path>` resolves that repo's live file even though WorkDir is
      elsewhere; `Source: <registered name>` resolves from that source's snapshot
      from any CWD; unknown name/path ⇒ `ErrUnknownSource`
- [ ] `go test ./internal/resolve/` → FAIL.
- [ ] Implement: load def layers in order (every `*.yaml|*.json` in SystemSnapshotDir,
      then UserSnapshotDir, then live file via `gitx.RepoRoot(P.WorkDir)`+`SourcePath`);
      later def layer replaces a path's `(Feature, defLayer)`. Then apply the two
      override maps in order; validate choice ids + mode arity against the winning def. Effective
      value = default unless overridden; `Layer` = winning layer.
- [ ] `go test ./internal/resolve/ -cover` → PASS, ≥95% here (this is the heart).
- [ ] Commit: `feat(gff): 5-layer resolver with provenance + choice validation`

### P1-T6: registry + install

**Files:** create `internal/registry/{registry.go,registry_test.go}`.

**Interfaces:** consumes paths/schema; produces §3.3 `Registry`, `ErrAreaClaimed`.

- [ ] Failing tests: fresh install writes `sources.yaml` (name/url/areas/commit) and
      snapshot `<UserSnapshotDir>/<name>.yaml` byte-identical to the source file;
      re-install same name refreshes commit + snapshot (no dup entry); second repo
      claiming an owned area ⇒ `ErrAreaClaimed` and error text contains owner name;
      `Sources()` on missing registry ⇒ empty, nil error.
- [ ] Implement (yaml encode of `SourceRegistry` via the same protojson-normalize
      trick as schema; `os.MkdirAll` + atomic write temp+rename).
- [ ] `go test ./internal/registry/` → PASS. Commit:
      `feat(gff): machine source registry with exclusive area claims + snapshots`

### P1-T7: read verbs — get / enabled / list / lint

**Files:** create `cmd/{get.go,enabled.go,selected.go,list.go,lint.go}` + `cmd/read_test.go`.

**Interfaces:** consumes Resolver; produces CLI contract §3.4. All verbs build
`resolve.Resolver{P: paths.Default(), R: gitx.ExecRunner{}}`; tests inject temp
paths via an unexported `newResolver` hook variable (`var newResolver = defaultResolver`
in root.go, swapped in tests — same pattern gss uses for its runner).

- [ ] Failing tests: `gff get k` prints `true\n` / choice prints `apt\n` (comma-joined
      ids when multi); `selected k apt` on/off/unknown-id (exit 0/1/2); unknown key ⇒
      exit-2 (assert via returned `*ExitError`-style sentinel: root maps
      `resolve.ErrUnknownKey` to `SilenceUsage` + `os.Exit(2)` in `main.go`; in tests
      assert the sentinel error); `enabled` on/off/unknown; `list` table contains
      `install.ai.claude  bool  true  default(user-snapshot)`-style row and
      `--json` unmarshals; `lint` on a bad file exits non-zero listing findings;
      root persistent flag `--source`: `get --source <path>` resolves a second temp
      repo from an unrelated CWD, `--source <registered-name>` resolves via its
      snapshot, unknown source ⇒ exit-2 sentinel (`ErrUnknownSource` maps like
      `ErrUnknownKey` in main.go).
- [ ] Implement the four files (~30 lines each). Exit-code mapping lives ONLY in
      `main.go`: `errors.Is(err, resolve.ErrUnknownKey) ⇒ 2`, else non-nil ⇒ 1.
- [ ] `go test ./cmd/` → PASS. Commit: `feat(gff): get/enabled/list/lint verbs`

### P1-T8: write verbs — set / unset

**Files:** create `cmd/{set.go,unset.go}` + `cmd/write_test.go`.

- [ ] Failing tests: `set k false` creates `~cfg/config.yaml` mode 0600 containing
      only that key; `set` choice with an unknown id, or two ids on a `single`-mode
      flag ⇒ error, file untouched; `set` unknown
      key ⇒ ErrUnknownKey; `unset` removes key, keeps others; round-trip
      `set→get` agrees; NO test writes outside `t.TempDir()`.
- [ ] Implement: read-modify-write `LoadOverrides` map + yaml marshal, atomic
      temp+rename, `os.Chmod(0600)`; validate via Resolver before writing.
- [ ] PASS → Commit: `feat(gff): set/unset writing the user override only`

### P1-T9: export + install verbs

**Files:** create `cmd/{export.go,install.go}` + `cmd/export_test.go`, golden file
`cmd/testdata/export.golden`.

- [ ] Failing tests: mangling table (`install.windows.wispr-flow` →
      `GFF_INSTALL_WINDOWS_WISPR_FLOW`); golden: world with 3 flags (one overridden
      false, one choice) ⇒ exact lines sorted:

```
GFF_INSTALL_AI_CLAUDE=true
GFF_INSTALL_PKG_MANAGER=apt
GFF_INSTALL_WINDOWS_WISPR_FLOW=false
```

      (values are bool literals or option ids; ids are lint-constrained
      `^[a-z0-9]+(-[a-z0-9]+)*$` so no quoting/injection vector exists; assert
      description text can contain `$(rm -rf)` without appearing);
      `install` in a temp repo registers + snapshots (delegates to registry; assert via
      `Sources()`), outside a repo ⇒ clear error. Format tests: `--format dotenv -o
      <tmp>/.env` writes the same lines as shell and round-trips through
      `hashicorp/go-envparse` (test dep only — parity with dotenv-family parsers);
      `--format json` output unmarshals into `[]Resolved` and carries choice option
      ids + typed values; `--format yaml` round-trips to the same `[]Resolved` as the
      json form (equality assert); `--shell` alias behaves as `--format shell`.
- [ ] Implement `export.go` (`--format shell|dotenv|json|yaml`, `-o`, `--shell` alias) + `install.go`
      (name = repo dir basename; url = `git config --get remote.origin.url` via
      gitx Runner, tolerate absence; commit = `rev-parse --short HEAD`).
- [ ] PASS → Commit: `feat(gff): shell export + repo install verbs`

### P1-T10: public SDK + CI + coverage gate

**Files:** create `pkg/gff/{gff.go,gff_test.go}`, `.github/workflows/gff-ci.yml`.

- [ ] Failing test: `pkg/gff` `Bool`/`Selected`/`IsSelected`/`StringValues` agree with a Resolver over the same
      temp world (SDK takes an optional `WithPaths(p)` functional option so the test
      can point it at temp dirs; default = `paths.Default()`).
- [ ] Implement thin wrapper. PASS.
- [ ] `gff-ci.yml`: on PR paths `sdk/gff/**`: setup-go from `.go-version`, run
      `go run . version` (zero-install entrypoint smoke — proves the module stays
      `go run`-able), then `go vet ./... && go test ./... -coverprofile=cover.out`, fail if
      `go tool cover -func=cover.out | tail -1` < 90%, then
      `sudo apt-get install -y protobuf-compiler` and `make gff-proto-check` (regen clean).
- [ ] Full `go test ./... -cover` ≥90% total. Commit:
      `feat(gff): public SDK + CI (vet, tests, coverage gate, proto-regen check)`
      → all green + `bash build.sh` installs a working `gff version`.

### P1-T11: binary-level e2e harness (happy path + adversarial)

**Files:** create `sdk/gff/e2e/e2e_test.go` (build tag `e2e`), `sdk/gff/scripts/e2e.sh`;
modify `.github/workflows/gff-ci.yml` (add `e2e` job) and root `Makefile` (`gff-e2e`).

Unit tests exercise packages; this harness exercises the **compiled binary** the way a
user does. `scripts/e2e.sh` builds `gff` into a temp dir, then runs
`go test -tags e2e ./e2e/`, which drives the binary via `os/exec` against a fake
`$HOME` and temp git repos (real `git`, zero network). Every §7.2 scenario (IH-\*,
IA-\*) is one named subtest — the scenario list in §7.2 is the authoritative spec.

- [ ] Failing subtests first: happy-path chain IH-1…IH-10 as ordered subtests sharing
      one world; adversarial IA-1…IA-12 each in an isolated world.
- [ ] `make gff-e2e` runs the same thing locally; CI `e2e` job runs after the unit job.
- [ ] Commit: `test(gff): binary-level e2e harness — happy path + adversarial suite`
      → **P1 done-when gate:** P1-T10 all green **plus** e2e harness green.

---

### P2-T1: dotfiles flag inventory (`.github/gff/features.yaml`)

**Files:** create `.github/gff/features.yaml` (probe path 2 — keeps the repo root
clean per design; no root `.gff/` in this repo).

- [ ] Allowlist check FIRST: `git check-ignore -v .github/gff/features.yaml` must show
      it NOT ignored (`!.github/**` already opts it in — expect no new `.gitignore`
      rules; if a deeper deny rule surprises us, add a narrow `!` rule with a comment).
      Then create the file and confirm `git status --short -- .github/gff/` shows it.
- [ ] Author the inventory — ONE `sets:` entry, `area: install`, every feature
      `boolDefault: true`, description = one plain sentence naming the install.sh
      block it gates. Full key list (43 flags — the enumeration deliverable):

```
install.system.wsl-interop      install.runtime.goenv        install.sdk.gss
install.system.jetson           install.runtime.pyenv        install.sdk.tmux-mgr
install.system.gitrepos         install.runtime.rbenv        install.sdk.wol
install.system.nano-profile     install.runtime.nvm          install.sdk.gsl
install.shell.profiles          install.runtime.fnm          install.sdk.gff
install.shell.default-zsh       install.ai.skills            install.fonts.nerd-font
install.pkg.common-core         install.ai.antigravity       install.network.sshd
install.pkg.brewfile            install.ai.claude            install.windows.desktop-deploy
install.tools.sops              install.ai.google-cli        install.windows.wsl-platform
install.tools.yq                install.ai.plugins           install.windows.nerd-font
install.tools.k8s               install.ai.teams             install.windows.terminal-themes
install.tools.snowflake                                      install.windows.apps
install.tools.docker                                         install.windows.wispr-flow
install.tools.git-aliases                                    install.windows.copilot-key
                                                             install.windows.ahk-autostart
                                                             install.windows.claude-rc-autostart
                                                             install.windows.sshd
                                                             install.windows.portproxy
```

- [ ] Verify: `~/opt/bin/gff lint .github/gff/features.yaml` → exit 0, no findings; and
      from repo root `gff list` shows all 43 with `LayerRepoLive` defaults.
- [ ] Commit: `feat(gff): enumerate dotfiles install components as flags (all on)`

### P2-T2: shell helper `opt/lib/gff.sh`

**Files:** create `opt/lib/gff.sh`, `opt/lib/gff_test.sh` (mirror
`ai/hooks/safety_guard_test.sh`'s assert style).

- [ ] Write the test driver first — cases: var unset ⇒ `gff_on` returns 0;
      `=true` ⇒ 0; `=false` ⇒ 1; `=FALSE` ⇒ 0 (only exact lowercase `false` disables);
      key mangling (`install.windows.wispr-flow` reads
      `GFF_INSTALL_WINDOWS_WISPR_FLOW`); `gff_skip_msg` echoes
      `SKIP (gff: <key>=false)`.
- [ ] Implement (POSIX-only — dash-safe, no `[[`, no arrays):

```sh
# shellcheck shell=bash
# gff.sh — fail-open feature-flag gate for install scripts.
# Usage: gff_on <area.component.feature>  (0 = run the step)
gff_on() {
  _gff_key=$(printf '%s' "$1" | tr '[:lower:]' '[:upper:]' | tr '.-' '__')
  eval "_gff_val=\${GFF_${_gff_key}:-}"
  [ "${_gff_val}" != "false" ]
}
gff_skip_msg() { echo "SKIP (gff: $1=false)"; }
```

- [ ] `bash opt/lib/gff_test.sh` all pass; `sh opt/lib/gff_test.sh` (dash) all pass;
      `make lint-shell && make lint-portability` clean.
- [ ] Commit: `feat(gff): fail-open gff_on shell gate helper + test driver`

### P2-T3: instrument install.sh (Linux/common)

**Files:** modify `install.sh`; modify `sdk/gff/build.sh` nothing (already installs to
`~/opt/bin`).

- [ ] Insert the gff bootstrap immediately AFTER the goenv/Go block (line ~309),
      BEFORE pyenv:

```bash
# build gff first so every later step can be feature-flag gated (fail-open:
# if the build fails or gff is absent, all steps run — flags only ever skip).
if gff_bootstrap_ok=false; command -v go >/dev/null 2>&1 && [ -f "${BASE_DIR}/sdk/gff/build.sh" ]; then
  bash "${BASE_DIR}/sdk/gff/build.sh" && gff_bootstrap_ok=true || echo "WARNING: gff build failed; all components will run."
fi
if [ "$gff_bootstrap_ok" = "true" ] && [ -x "${HOME}/opt/bin/gff" ]; then
  eval "$(cd "${BASE_DIR}" && "${HOME}/opt/bin/gff" export --shell 2>/dev/null || true)"
fi
. "${BASE_DIR}/opt/lib/gff.sh"
```

- [ ] Wrap each existing block in-place (NO reordering, NO logic changes inside) with
      its P2-T1 key. Pattern, using sops as the exemplar:

```bash
if gff_on install.tools.sops; then
  if [ -f "${BASE_DIR}/opt/scripts/system/install_sops.sh" ]; then
    echo "Installing sops..."
    "${BASE_DIR}/opt/scripts/system/install_sops.sh" || echo "WARNING: sops install reported problems; continuing."
  fi
else gff_skip_msg install.tools.sops; fi
```

  Blocks BEFORE the bootstrap point (`install.system.wsl-interop`, `install.system.jetson`,
  `install.shell.profiles`, `install.ai.skills`, antigravity/claude config,
  `install.pkg.*`, `install.shell.default-zsh`, tools, docker, git-aliases, goenv)
  still get `gff_on` gates — they read env from a PREVIOUS run's flags only if the
  caller pre-exported; document with one comment line at the top: flags for
  pre-bootstrap steps take effect via `gff export` in the calling shell or on the
  next run. (`install.sdk.gff` gates only the LATER duplicate build guard —
  the bootstrap build itself is never gated.)
- [ ] Verify: `bash -n install.sh`; `make lint-shell && make lint-portability` clean;
      manual: `GFF_INSTALL_TOOLS_SOPS=false bash -c '. opt/lib/gff.sh; gff_on install.tools.sops || gff_skip_msg install.tools.sops'` prints the SKIP line.
- [ ] Commit: `feat(install): gate every install.sh component behind gff flags (fail-open)`

### P2-T4: Windows pass-through + PS gating

**Files:** modify `opt/bin/install_windows.sh`; create
`opt/Desktop/Apps/scripts/lib/gff.ps1`; modify
`opt/Desktop/Apps/scripts/{setup-apps.ps1,setup-elevated.ps1}`.

- [ ] `install_windows.sh`: top-level `gff_on install.windows.desktop-deploy || { gff_skip_msg …; exit 0; }`;
      before each `powershell.exe` invocation, build `WSLENV` so `GFF_INSTALL_WINDOWS_*`
      crosses into Windows:

```bash
_gff_wslenv="${WSLENV:-}"
for _v in $(env | sed -n 's/^\(GFF_INSTALL_WINDOWS_[A-Z_]*\)=.*/\1/p'); do
  case ":${_gff_wslenv}:" in *":${_v}/u:"*) : ;; *) _gff_wslenv="${_gff_wslenv:+${_gff_wslenv}:}${_v}/u" ;; esac
done
export WSLENV="${_gff_wslenv}"
```

- [ ] `lib/gff.ps1`:

```powershell
function Test-GffOn([string]$Key) {
    $var = 'GFF_' + ($Key.ToUpper() -replace '[.-]', '_')
    $val = [Environment]::GetEnvironmentVariable($var)
    return $val -ne 'false'   # fail-open: unset/anything-else => on
}
```

- [ ] Gate `setup-apps.ps1` phases: WSL platform (`install.windows.wsl-platform`),
      Nerd Font, Terminal themes, winget apps; gate `setup-elevated.ps1` items:
      Wispr Flow MSI (`install.windows.wispr-flow`), PowerToys Copilot remap
      (`install.windows.copilot-key`), AHK autostart task
      (`install.windows.ahk-autostart`); gate the standalone scripts' invocation sites
      for `claude-rc-autostart`, `sshd`, `portproxy`. Each disabled phase prints
      `SKIP (gff: <key>=false)`.
- [ ] Verify: `make lint-shell && make lint-portability` (bash file);
      `pwsh -NoProfile -Command ". opt/Desktop/Apps/scripts/lib/gff.ps1; Test-GffOn 'install.windows.wispr-flow'"`
      → True; with `$env:GFF_INSTALL_WINDOWS_WISPR_FLOW='false'` → False (if pwsh
      unavailable in WSL, this check moves to the P2-T5 human run).
- [ ] Commit: `feat(install): gff gating for Windows setup phases via WSLENV pass-through`

### P2-T5: human-evidenced acceptance (spec §6)

- [ ] On WSL: `gff set install.windows.wispr-flow false`, run `install.sh` in a real
      terminal, capture the `SKIP (gff: install.windows.wispr-flow=false)` line;
      `gff unset install.windows.wispr-flow`; paste evidence into PR #181 (or the P2
      leaf PR). **P2 done-when gate.**

---

### P3-T1: TUI

**Files:** create `internal/tui/{model.go,view.go,tui_test.go}`, `cmd/tui.go`; modify
`cmd/root.go` (bare `gff` with no args + a TTY runs the TUI, else help).

- [ ] Deps: `go get github.com/charmbracelet/bubbletea github.com/charmbracelet/lipgloss github.com/charmbracelet/x/exp/teatest@latest`
- [ ] Failing teatest tests: initial frame lists areas collapsed; navigating
      (`enter` on area → components → features) shows a feature row containing
      description + `default`/`override` + winning layer; pressing `space` on a bool
      writes ONLY the user override file (temp paths world) and the row flips;
      `space` on a choice opens the option picker — radio list for `single` mode,
      checkbox list for `multi` (from `ChoiceDefault.Options`, showing id +
      description + typed value); `q` after no toggles writes nothing (mtime unchanged).
- [ ] Implement: `Model{items []resolve.Resolved, cursor, expanded map[string]bool, w io.Writer}`;
      reuse `cmd`'s resolver hook; writes go through the same code path as `gff set`
      (extract `internal/overrides.Write(paths, key, value)` from P1-T8 if not already
      shared — refactor `set.go` to call it, tests stay green).
- [ ] `go test ./... -cover` ≥90% overall still. Commit:
      `feat(gff): bubbletea TUI — browse, provenance, toggle` → **P3 done-when gate.**

### P4-T1: `gff gen` typed accessors

**Files:** create `cmd/gen.go`, `cmd/gen_test.go`, `cmd/testdata/gen.golden`.

- [ ] Failing golden test: for the P1-T9 world, `gff gen --pkg gffgen --out <tmp>`
      writes `<tmp>/gffgen.go` matching the golden — for each flag a var chain
      `var Install = struct{ Ai struct{ Claude BoolFlag } … }` with
      `func (f BoolFlag) Bool() (bool, error)` delegating to `pkg/gff` by literal key
      string; segment names Title-cased, dashes camel-cased (`wispr-flow` → `WisprFlow`).
      Golden compiles: test runs `go vet` on a scratch module embedding the output.
- [ ] Implement using `text/template` + `go/format.Source`.
- [ ] PASS → Commit: `feat(gff): gen — typed accessor codegen` → **P4 done-when gate.**

### VD-1: scripted end-to-end demo

**Files:** create `sdk/gff/scripts/demo.sh` (shell-portability-lint clean).

A narrated, re-runnable walkthrough proving the whole story on a real machine; each
step echoes what it is about to prove. It runs against a scratch `$HOME`
(`GFF_DEMO_HOME` temp dir) so it never touches real config. Steps are §7.3's script.

- [ ] Write `demo.sh`; run it on WSL; paste the full transcript into PR #181 (or the
      leaf PR) as the demo evidence.
- [ ] Post-P3 addendum: a ~30-second TUI segment (browse → toggle → winning-layer
      provenance) captured and linked from the PR.
- [ ] Commit: `docs(gff): end-to-end demo script + recorded evidence`
      → **VD-1 done-when gate:** transcript on the PR; P2-T5's human-evidenced
      wispr-flow SKIP run remains the real-install proof alongside it.

---

## 5. Verification mapping (spec §5 → test)

| Spec rule | Test |
| :-- | :-- |
| F1/F2 lint (dup, negative, depth, casing) | `internal/schema/lint_test.go` table |
| F3 choice validation | `lint_test.go` (def side) + `resolve_test.go` case 7 + `write_test.go` (set side) |
| F4 layer matrix, unknown ⇒ exit 2 | `resolve_test.go` cases 1–9; `cmd/read_test.go` sentinel |
| F5 discovery + `gff.source` redirect, worktree `.git` file | `internal/gitx/gitx_test.go` |
| F6 exclusive claims, refresh, moved clone | `internal/registry/registry_test.go` |
| F7 export mangling, injection-safety, choice id CSV | `cmd/export_test.go` + `testdata/export.golden` |
| F8 user-override-only writes, 0600, no-dir | `cmd/write_test.go` |
| F9 gating fail-open (unset/true/false/FALSE), skip msg | `opt/lib/gff_test.sh` (bash AND dash) |
| F9 Windows pass-through | P2-T4 pwsh check or P2-T5 human run |
| F10 TUI toggle/no-write-on-quit | `internal/tui/tui_test.go` (teatest) |
| F11 zero-install go-run; `--source` scoping | `gff-ci.yml` `go run . version` smoke; `resolve_test.go` case 10; `cmd/read_test.go` `--source` cases |
| Proto regen clean; ≥90% cover | `.github/workflows/gff-ci.yml` |
| UC1 end-to-end wispr-flow skip | P2-T5 human-evidenced run |

## 6. Integration & rollout

- `install.sh` builds gff via the P2-T3 bootstrap block; `sdk-auto-bump.yml` and
  `tag-sdk-modules.yml` cover `sdk/gff` automatically (path-filtered on `sdk/**`).
- Docs: `sdk/gff/AGENTS.md` (+`CLAUDE.md` symlink) written in P1-T1; add a `sdk/gff`
  line to root `CLAUDE.md` Repository Structure and `opt/bin/AGENTS.md` is untouched
  (gff installs to `~/opt/bin` but is sdk-owned, like gss).
- Rollback per design §6: revert P2 commit(s) to restore unconditional installs;
  P1/P3/P4 are additive.
- After merge: close #180 only when all four leaves land; update `docs/mbo/index.md`
  state per leaf.

### 6.1 Build leaves / DAG

| Leaf | Owns (paths) | Consumes (in-edges) | `done-when` gate | Blocking? |
| :-- | :-- | :-- | :-- | :-- |
| `p1-engine` | `sdk/gff/**` (excl. `internal/tui`, `cmd/gen.go`), `.github/workflows/gff-ci.yml` | — | gff-ci green: vet+tests, ≥90% cover, regen clean, e2e harness green; `build.sh` installs working binary | **yes (base)** |
| `p2-instrument` | `.github/gff/**`, `opt/lib/gff.sh*`, `install.sh`, `opt/bin/install_windows.sh`, `opt/Desktop/Apps/scripts/{lib/gff.ps1,setup-apps.ps1,setup-elevated.ps1}` | `p1-engine` (§3.4 CLI, §3.5 shell contract) | lint-shell + lint-portability clean; gff lint clean; P2-T5 human evidence | no |
| `vd-demo` | `sdk/gff/scripts/demo.sh` | `p1-engine` + `p2-instrument` (TUI segment additionally `p3-tui`) | demo transcript + evidence posted on the PR (§7.3, VD-1) | no |
| `p3-tui` | `sdk/gff/internal/tui/**`, `sdk/gff/cmd/tui.go` | `p1-engine` (§3.3 Resolver, overrides writer) | teatest suite green; overall cover ≥90% | no |
| `p4-gen` | `sdk/gff/cmd/gen.go`, `cmd/gen_test.go`, `cmd/testdata/gen.golden` | `p1-engine` (§3.3 pkg/gff) | golden test green; generated output vets | no |

(`p3-tui` and `p4-gen` both add one `AddCommand` line in `cmd/` — each registers from
its own file via `init()`, so no shared-file edits; `p2/p3/p4` are pairwise disjoint
and can run in parallel once `p1-engine` merges.)

## 7. End-to-end validation plan

Three cumulative layers of proof: **unit** (≥90% coverage — every feature's logic),
**integration** (the compiled binary + real git + real files: P1-T11 harness), and
**demo** (a human-followable script proving the whole story: VD-1, plus P2-T5's real
install run). A feature counts as *built* only when all three of its §7.4 rows are
green.

### 7.1 Coverage targets (unit)

| Scope | Bar | Enforced by |
| :-- | :-- | :-- |
| `internal/resolve` | ≥95% | P1-T5 gate |
| `internal/schema` | ≥90% | P1-T3 gate |
| `sdk/gff` overall | **≥90%** | `gff-ci.yml` cover gate (P1-T10) |
| repo-wide `sdk/` floor | 60% | unchanged; gff deliberately exceeds it |

Rationale: `resolve` + `schema` hold nearly all branching logic, so they carry the
highest bars; `cmd/` is thin cobra wiring whose remaining paths (exit codes, flag
parsing, stdout hygiene) the e2e harness exercises through the real binary.

### 7.2 Integration scenarios (P1-T11, compiled binary, fake `$HOME`, real git)

**Happy path (IH-\*, ordered subtests sharing one world — this IS the end-to-end
happy-path proof):**

1. **IH-1** `gff lint` on an authored flag file (bools + one radio + one checkbox
   choice with typed values) ⇒ exit 0.
2. **IH-2** `gff install` in repo A ⇒ `sources.yaml` + snapshot written; `gff list`
   works from `$HOME`.
3. **IH-3** `get`/`enabled` on a default-true bool from a foreign CWD ⇒ `true` / exit 0.
4. **IH-4** `selected` on the default choice option ⇒ exit 0; `get` prints the id(s).
5. **IH-5** `set` bool `false` ⇒ ONLY the user override file changes (0600);
   `list --json` shows `layer=user-override`.
6. **IH-6** `set` choice — single: one id; multi: two ids — round-trips through `get`.
7. **IH-7** `export --format shell` evals cleanly in bash AND dash; `gff_on` then
   skips the false key and runs the true key.
8. **IH-8** `export --format dotenv -o .env` parses with go-envparse; `json` and
   `yaml` forms unmarshal to identical `[]Resolved` incl. typed payloads.
9. **IH-9** `unset` ⇒ default restored; winning layer reverts to snapshot/repo.
10. **IH-10** zero-install + cross-repo: `go run . <verb>` (module-local stand-in for
    `go run <module>@<tag>`) and `--source <name>`/`--source <path>` from a foreign CWD.

**Adversarial / negative (IA-\*, isolated worlds — errors must be *clean*: correct
exit code, message names the offender, zero partial writes):**

1. **IA-1** unknown key ⇒ exit 2 on `get`/`enabled`/`set`; unknown option id ⇒ exit 2
   on `selected`.
2. **IA-2** `set` with two ids on a `single`-mode choice ⇒ exit 1; override file
   byte-identical before/after.
3. **IA-3** malformed flag file (truncated mid-list, bad indent) ⇒ `lint` and every
   read verb fail naming file+line; never a panic/stacktrace.
4. **IA-4** malformed override yaml ⇒ read verbs error cleanly (not silently skipped —
   masking a user's typo is worse than failing); other layers unaffected afterward.
5. **IA-5** injection attempts: description containing `$(rm -rf /tmp/pwned)` never
   reaches export output; option id `evil;rm` rejected by lint; exported bytes assert
   against a `[A-Z0-9_=,.\n-]`-only set.
6. **IA-6** second repo claiming an owned area ⇒ `ErrAreaClaimed` naming the owner;
   registry file unchanged.
7. **IA-7** corrupt `sources.yaml` ⇒ verbs degrade with a clear error — and the shell
   gate stays fail-open (a broken gff still runs every step).
8. **IA-8** read-only `~/.config` ⇒ `set` exits 1, no temp-file litter.
9. **IA-9** `HOME` unset ⇒ clear error; nothing written to CWD.
10. **IA-10** `--source` with an unknown name and with a non-repo path ⇒ exit 2.
11. **IA-11** 10 concurrent `set` calls ⇒ final override is valid yaml equal to one
    of the written values (atomic temp+rename; no interleaved corruption).
12. **IA-12** `gff.source` redirect pointing at a missing file / outside the repo ⇒
    clean error; no path-traversal surprises.

Shell-side negatives live in `opt/lib/gff_test.sh` (F9, P2): unset var ⇒ run; exactly
`"false"` ⇒ skip; `"FALSE"`/`"0"`/garbage ⇒ run (fail-open is literal-false only);
missing binary ⇒ run.

### 7.3 Demo script (VD-1) — "flags for a fresh repo in two minutes"

`sdk/gff/scripts/demo.sh`, narrated, scratch `$HOME`, re-runnable:

1. Scaffold a demo repo; author a flag file with 1 bool + 1 radio choice + 1 checkbox
   choice (typed values shown).
2. `lint` → `install` → `list` (point out the winning-layer/provenance column).
3. Gate a toy script with `gff_on`; `set` the bool off; rerun shows the SKIP line;
   flip it back. (Post-P3: same toggle via the TUI, captured.)
4. `export` all four formats; eval the shell form in dash; parse the `.env`.
5. A second repo claims the same area ⇒ the rejection message (the guardrail moment).
6. Finale from an empty directory with no gff on PATH:
   `eval "$(go run <module>@<tag> export --format shell --source demo)"`.

Real-install evidence: P2-T5's human-evidenced `install.sh` run with
`install.windows.wispr-flow=false` showing the SKIP line, posted to the PR.

### 7.4 Feature → proof matrix

| Feature | Unit | Integration | Demo |
| :-- | :-- | :-- | :-- |
| F1 keys + lint | `lint_test.go` table | IH-1, IA-3 | step 2 |
| F2 bool semantics | schema/resolve tests | IH-3, IH-5 | steps 2–3 |
| F3 choice (modes, ids, typed values) | lint/resolve/write tests | IH-4, IH-6, IA-1, IA-2 | steps 1, 3 |
| F4 layered resolution + provenance | resolve matrix 1–10 | IH-5, IH-9 | step 2 |
| F5 discovery + redirect | `gitx_test.go` | IH-3 (foreign CWD), IA-12 | step 2 |
| F6 registry + claims | `registry_test.go` | IH-2, IA-6, IA-7 | step 5 |
| F7 export formats + injection safety | export golden | IH-7, IH-8, IA-5 | steps 4, 6 |
| F8 write path (0600, user-only) | `write_test.go` | IH-5, IA-8, IA-11 | step 3 |
| F9 fail-open gating | `gff_test.sh` (bash + dash) | IH-7, IA-7 | P2-T5 evidence |
| F10 TUI | teatest goldens | (visual — teatest is the harness) | post-P3 capture |
| F11 go-run + `--source` | CI smoke, read tests | IH-10, IA-10 | step 6 |

### 7.5 Validation done-when

- `gff-ci.yml` fully green: vet, unit tests with **≥90%** coverage, `e2e` job (all
  IH-\* and IA-\* subtests), proto-regen clean, `go run .` smoke.
- Demo transcript (VD-1) and P2-T5 real-install evidence posted on the PR(s).
- Every §7.4 row checked off in the leaf PR descriptions — a feature without all
  three proofs is not done.

> Produced via `superpowers:writing-plans`. Execute with
> `superpowers:subagent-driven-development` / `executing-plans`, TDD throughout.
> Update `../index.md` state as it moves.
