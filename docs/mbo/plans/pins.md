# pins — version pin management — implementation plan

- **Slug:** pins
- **Date:** 2026-10-10
- **Status:** Approved
- **Relates to:** spec [`../specs/pins.md`](../specs/pins.md) · design [`../designs/pins.md`](../designs/pins.md) · trio [`./pins/`](./pins/)

## 1. Summary & verdict
This adds a version manifest, a shell resolver, converging installers, a `fleet pins` subcommand
(status/bump/apply), a PINS column in `fleet status`, a pins rollout plan for `fleet update --file`,
and a weekly drift-report workflow. It ships as a single PR (feature `pins`, worker `build`). Two subagents run in
parallel in one worktree on disjoint files (shell leaf L1 and Go leaf L2) against the frozen contracts in §3. The integration leaf L3
follows.

## 2. File inventory

| Path | Purpose | Implements |
| :-- | :-- | :-- |
| `opt/profiles/versions.tsv` | the manifest | F1 |
| `opt/lib/pins.sh`, `opt/lib/pins_test.sh` | `pins_want`, `pins_latest`, `pins_strip_v` | F2 |
| `opt/scripts/system/install_{sops,yq,k8s_tools}.sh` (+ new `_test.sh` for sops/yq/k8s) | converge | F3 |
| `opt/scripts/git/install_gitleaks.sh` (+ existing test) | converge | F3 |
| `opt/scripts/system/install_{uv,glow,rust}.sh`, `macos-keys-linux.sh`, `opt/scripts/network/vault-setup.sh`, `sdk/gsl/scripts/install_nerd_font_{linux,macos}.sh`, `install.sh` (Go block) | defaults from manifest | F4 |
| `.github/gff/features.yaml` | `pins.track` choice flag | F12 |
| `sdk/fleet/internal/pins/{manifest,version,upstream,installed}.go` + tests | core | F1,F5,F6,F7 |
| `sdk/fleet/cmd/pins.go` + `pins_test.go` | `fleet pins status|bump|apply` | F5–F8 |
| `sdk/fleet/cmd/status.go` (+tests) | probe §3 + PINS column + JSON | F9 |
| `opt/etc/fleet/pins.yaml`; `install.sh` chmod line | rollout plan | F10 |
| `.github/workflows/pins-report.yml` | weekly issue upsert | F11 |
| `.go-version` | 1.26.1 → 1.26.3 (matches go.mod files; fixes Security Scan) | drift fix |
| `sdk/fleet/README.md`, `sdk/fleet/AGENTS.md`, `opt/profiles/AGENTS.md`, `docs/mbo/index.md` | docs | — |

## 3. Interface contracts (FROZEN: L1 and L2 build against these)

### 3.1 Manifest `opt/profiles/versions.tsv`
Whitespace-separated. `#` comments and blank lines are ignored and preserved. Columns:
`tool track version upstream installer probe…`

- `tool`: `[a-z0-9-]+`, unique.
- `track`: `pin` | `floor` | `latest`.
- `version`: semver without a leading `v`, `-` (only for `latest`), or `@<repo-relative path>` (the file's trimmed content).
- `upstream`: `gh:<owner>/<repo>` | `go:` | `text:<url>` | `-`.
- `installer`: repo-relative script path, or `-` (report-only).
- `probe`: rest of line, a shell command whose output contains the version; `-` = none.

Seed rows:
```
go          pin     @.go-version  go:                          -                                     go version
uv          pin     0.12.16       gh:astral-sh/uv              opt/scripts/system/install_uv.sh      uv --version
glow        pin     3.0.0         gh:charmbracelet/glow        opt/scripts/system/install_glow.sh    glow --version
rust        floor   1.96.0        gh:rust-lang/rust            opt/scripts/system/install_rust.sh    rustc --version
sops        pin     3.13.1        gh:getsops/sops              opt/scripts/system/install_sops.sh    sops --version
yq          pin     4.44.6        gh:mikefarah/yq              opt/scripts/system/install_yq.sh      yq --version
gitleaks    pin     8.21.2        gh:gitleaks/gitleaks         opt/scripts/git/install_gitleaks.sh   gitleaks version
kubectl     pin     1.37.0        text:https://dl.k8s.io/release/stable.txt  opt/scripts/system/install_k8s_tools.sh  kubectl version --client
helm        pin     4.2.4         gh:helm/helm                 opt/scripts/system/install_k8s_tools.sh  helm version --short
kind        pin     0.33.0        gh:kubernetes-sigs/kind      opt/scripts/system/install_k8s_tools.sh  kind version
herdr       latest  -             -                            opt/scripts/system/install_herdr.sh   herdr --version
keyd        pin     2.6.0         gh:rvaiya/keyd               -                                     keyd --version
nerd-fonts  pin     3.4.0         gh:ryanoasis/nerd-fonts      -                                     -
vault       pin     1.21.4        gh:hashicorp/vault           -                                     vault version
```

### 3.2 Shell `opt/lib/pins.sh` (sourced; bash 3.2 + dash-safe body, no `set -e`)
- `pins_manifest` → prints the manifest path: `$PINS_MANIFEST`, else `<lib dir>/../profiles/versions.tsv`.
- `pins_want <tool> [fallback]` → prints a version or `latest`. Order: `GFF_PINS_TRACK=latest` → `latest`.
  Then the manifest row: track `latest` prints `latest`, unless `GFF_PINS_TRACK=pin` and the row has a concrete version. Otherwise
  the version (`@file` resolved relative to the repo root, leading `v` stripped). If there's no row or no manifest, print the fallback.
  Always exits 0. (Per-tool env overrides stay in each script as `${X_VERSION:-$(pins_want x …)}`.)
- `pins_latest_gh <owner/repo>` → latest release tag without `v` (curl the GitHub API, honours `GITHUB_TOKEN`). Non-zero on failure.
- Installer contract: `HAVE` (installed, no `v`) vs `WANT`. Equal and `X_FORCE!=1` → "already at" and exit 0.
  Otherwise download WANT. `WANT=latest` → resolve via `pins_latest_gh` first.

### 3.3 Go `sdk/fleet/internal/pins`
```go
type Track string // TrackPin "pin", TrackFloor "floor", TrackLatest "latest"
type Entry struct { Tool string; Track Track; Version, VersionFile, Upstream, Installer, Probe string; Line int }
type Manifest struct { Path, Root string; Entries []Entry /* + unexported raw lines */ }
func Load(repoRoot string) (*Manifest, error)          // <root>/opt/profiles/versions.tsv
func Parse(r io.Reader, root, path string) (*Manifest, error)
func (m *Manifest) Get(tool string) (Entry, bool)
func (m *Manifest) Set(tool, version string) error      // pin/floor only; @file → rewrites that file
func (m *Manifest) Save() error                         // atomic; preserves comments/spacing of other lines
func ExtractVersion(s string) string                    // first \d+\.\d+(\.\d+)?; "" if none
func Compare(a, b string) int                           // numeric dotted compare, missing parts = 0
func Gap(pin, latest string) string                     // "current"|"patch"|"minor"|"major"|"ahead"|"unknown"
type Release struct { Version string; Published time.Time }
type Resolver interface { Latest(ctx context.Context, upstream string) (Release, error) }
func NewHTTPResolver(c *http.Client, token string) Resolver // gh:, go:, text:; base URLs overridable for tests
type Exec func(ctx context.Context, shellCmd string) (string, error)
type InstalledRow struct { Tool, Track, Want, Installed, State string } // State: ok|drift|missing|unknown
type Summary struct { OK, Drift, Missing, Unknown int }
func CheckInstalled(ctx context.Context, m *Manifest, ex Exec, mode string) ([]InstalledRow, Summary) // mode = pins.track
```
`fleet pins status --installed --json` stdout (consumed by `fleet status`):
`{"rows":[InstalledRow…],"summary":{"ok":N,"drift":N,"missing":N,"unknown":N}}` (lower-case keys).

### 3.4 CLI
```
fleet pins status [--installed] [--json] [--repo DIR]
fleet pins bump (TOOL... | --all) [--to VERSION] [--dry-run] [--repo DIR]   # --to needs exactly one TOOL
fleet pins apply [TOOL...] [--dry-run] [--repo DIR]
```
`--repo` defaults to the same dotfiles-clone resolution fleet already uses (`--repo` / `~/git/dotfiles`).

### 3.5 Status probe
`probeCmdFor` appends `; echo '__fleet_pins__'; ~/opt/bin/fleet pins status --installed --json --repo <repoPath> 2>/dev/null || true`.
`Row.Pins *pins.Summary` (nil = old host or malformed). Table column `PINS`: `ok` | `N drift` | `-` (missing is deliberately NOT shown: a tool can be absent because its gff
flag is off; the JSON field `pins` carries the full summary). The TUI table is a follow-up: one more column pushes UPDATE
off a 100-column terminal.

## 4. TDD build order
Every task: RED test → fail → GREEN → pass → gates. Evidence goes to `plans/pins/evidence/<task>.txt`.

- **T1 (L1)**: manifest + `pins.sh` + `pins_test.sh` + gff flag `pins.track`. Gate: `bash opt/lib/pins_test.sh`; `make lint-shell lint-portability`.
- **T2 (L1)**: converge sops/yq/gitleaks/k8s + tests with stubbed curl/version. Manifest defaults for uv/glow/rust/keyd/fonts/vault/install.sh go. Gate: each `*_test.sh`, lint.
- **T3 (L2)**: `internal/pins` manifest/version (Parse, Set, Save, Extract, Compare, Gap). Gate: `go test ./internal/pins` ≥85% cov.
- **T4 (L2)**: HTTP resolver with httptest. Gate: same.
- **T5 (L2)**: `cmd/pins.go` status + bump. Gate: `go test ./cmd -run Pins`.
- **T6 (L2)**: CheckInstalled + apply. Gate: same + `go vet`.
- **T7 (L3)**: status probe section + PINS column + JSON. Gate: `go test ./...` module ≥60% cov; `make lint-go`.
- **T8 (L3)**: `pins.yaml`, install.sh chmod, `pins-report.yml`, docs. Gate: actionlint, `fleet update --file opt/etc/fleet/pins.yaml --dry-run <host>`.
- **T9 (L3)**: `.go-version` → 1.26.3 via `fleet pins bump go --to 1.26.3`. Gate: live `fleet pins status`, `fleet pins status --installed`, `fleet status` output captured.

## 5. Verification mapping
F1→T3 `TestParse*`/T1 · F2→`pins_test.sh` · F3→`install_*_test.sh` · F5→`TestResolver*`,`TestPinsStatus*` ·
F6→`TestCheckInstalled*` · F7→`TestManifestSet*`,`TestPinsBump*` · F8→`TestPinsApply*` · F9→`TestProbePins*`,`TestRenderTablePins` ·
F10/F11→T8 gates · F12→features.yaml lint (`gff` CI).

## 6. Integration & rollout
After merge: `install.sh` on each host (interactive) or `fleet update --file opt/etc/fleet/pins.yaml <hosts>`.
Existing hosts converge sops/yq/gitleaks/k8s on their next install.

### 6.1 Build leaves / DAG
| Leaf | Owns (paths) | Consumes | done-when | Blocking? |
| :-- | :-- | :-- | :-- | :-- |
| L1 shell | `opt/profiles/versions.tsv`, `opt/lib/pins*`, installer scripts, `install.sh` (Go block only), `features.yaml` | §3.1/3.2 | T1–T2 gates | no |
| L2 go | `sdk/fleet/internal/pins/**`, `sdk/fleet/cmd/pins*.go` | §3.1/3.3/3.4 | T3–T6 gates | yes, for L3 |
| L3 integrate | `sdk/fleet/cmd/status*.go`, `opt/etc/fleet/pins.yaml`, `install.sh` (chmod line), workflow, docs, `.go-version` | L2 | T7–T9 gates | no |

Single gss worker. L1 and L2 run concurrently as subagents on disjoint paths. L3 runs after L2.

## 7. Validation & evidence
Coverage: `internal/pins` ≥85%, fleet module ≥60%. Lint: `make lint-shell lint-portability lint-go`.
Live evidence (T9) captured under `plans/pins/evidence/`.
