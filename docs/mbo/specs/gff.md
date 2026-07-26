# gff — git fast features — spec

- **Slug:** gff
- **Date:** 2026-07-23
- **Status:** Approved
- **Relates to:** design `../designs/gff.md` / issue (pending) / PR (pending)

## 1. Goal

A generic feature-flag engine (`sdk/gff`, Go, cobra + bubbletea) whose flag *schema* is
defined by protobuf and whose flag *data* is persisted in git: each repo tracks its own
flag file — probed at `.gff/features.yaml` then `.github/gff/features.yaml` (dotfiles
uses the latter; repo root stays clean) — `gff install` deploys it machine-wide, and users flip
flags via a well-known override file or the TUI. The dotfiles repo becomes the first
consumer: every `install.sh` component (Linux and Windows sides) gets a bool flag,
all defaulting on, so components can be disabled per machine later without editing
scripts.

## 2. Use cases

**UC1 — gate an install step (shell)**
- *Actor:* `install.sh` (and `setup-apps.ps1`/`setup-elevated.ps1` via env).
- *Trigger:* bootstrap run on any host.
- *Flow:* install.sh builds gff after the Go toolchain → `eval "$(gff export --shell)"`
  → each later step checks its `GFF_*` var via the fail-open `gff_on` helper —
  **never `gff enabled` as a gate**: its non-zero exits on corrupt config or unknown
  keys would fail CLOSED, the top install.sh regression risk — → disabled steps
  print `SKIP (gff: <key>=false)` and continue.
- *Acceptance:* disabling `install.windows.wispr-flow` in `~/.config/gff/config.yaml`
  skips the Wispr Flow MSI/AHK workflow on the next run; deleting the override restores
  it; a host with no gff binary runs everything (fail-open).

**UC2 — inspect and flip flags (human)**
- *Actor:* user at a terminal.
- *Trigger:* `gff` / `gff tui` / `gff list`.
- *Flow:* TUI tree area→component→feature; each row shows description, default,
  effective value, and the layer that set it; toggling a bool, or picking choice
  options (radio for `single` mode, checkboxes for `multi`), writes
  `~/.config/gff/config.yaml` only.
- *Acceptance:* a toggle round-trips (visible in `gff get`, survives restart); the
  repo defaults file is never modified by the TUI/CLI.

**UC3 — a repo adopts gff**
- *Actor:* maintainer of any git repo.
- *Trigger:* adds a flag file (either probe path: `.gff/features.yaml` or
  `.github/gff/features.yaml`) declaring its `namespace:` (reverse-DNS of the origin
  URL) + features; runs `gff lint`, then `gff install`.
- *Acceptance:* inside the repo, `gff get` resolves live from the tracked file; after
  install, the same keys resolve from any CWD via the snapshot; a second repo claiming
  an already-registered namespace (different url) is rejected with the owner named
  — while a second repo's identical short keys coexist under its own namespace; a
  script in an unrelated
  repo can gate on the flags with **no gff installed** via
  `eval "$(go run github.com/sfc-gh-eraigosa/dotfiles/sdk/gff@<tag> export --format shell --source <name>)"`.

**UC4 — Go code reads a flag**
- *Actor:* a Go program importing `pkg/gff`.
- *Flow:* `gff.Bool("install.ai.claude")` resolves the full chain; optionally the repo
  runs `gff gen` for typed accessors.
- *Acceptance:* SDK and CLI agree on the effective value for every key in every layer
  combination (shared resolver, table-tested).

## 3. Architecture

Components (each independently testable; see design §4 for full detail):

- `proto/gff/v1/features.proto` — generic schema; committed generated Go; regeneration
  pinned in `build.sh`, CI-checked clean.
- `internal/schema` — parse + validate + lint the repo flag file (`.yaml|.json`,
  protojson-compatible encodings of `FeatureSet`).
- `internal/resolve` — layer chain (design §4): source snapshots (`/opt/conf/gff/`
  system read-only, `${XDG_DATA_HOME:-~/.local/share}/gff/snapshots/` user — the only
  snapshot dir gff writes; never under `~/opt`, a symlink into the repo worktree) →
  live repo file (git-style discovery, `[gff] source` redirect) →
  overrides (`/var/opt/conf/gff/config.yaml`, `~/.config/gff/config.yaml`); sparse
  per-key merge; winning-layer attribution.
- `internal/registry` — `~/.config/gff/sources.yaml`, keyed by reverse-DNS
  namespace (derived from origin URL); snapshots.
- `cmd/` — cobra: `get`, `enabled`, `set`, `unset`, `list`, `install`, `export`,
  `gen`, `lint`, `tui`, `version`.
- `internal/tui` — bubbletea tree view; writes user override only.
- `pkg/gff` — public runtime SDK; `gff gen` typed accessors (P4).
- Filesystem + git access behind interfaces (mockable, like gss's runner) so resolver
  tests need no real repo.

Data flow: features.yaml (git) ──gff install──▶ snapshot + registry ──resolver──▶
effective values ──▶ {CLI, TUI, SDK, `export --shell` env for bash/PowerShell}.

## 4. Behavior / features

- **F1 canonical keys:** dotted `area.component.feature`, scoped by a required
  reverse-DNS `namespace` declared in the flag file and derived from the origin URL
  (e.g. `com.github.sfc-gh-eraigosa.dotfiles`; fully-qualified form
  `<namespace>:<key>`; uniqueness = namespace + key; areas are grouping only, never
  claimed). Lint enforces key uniqueness within the namespace, lowercase-kebab
  segments, 3-level depth, namespace presence/charset, and warns when the declared
  namespace differs from the origin-derived value (fork case).
- **F2 bool type:** values strictly `true`/`false`; lint rejects negative names
  (`no-*`, `disable-*`, `skip-*`) so `true` always means ON.
- **F3 choice type:** an option set with `mode: single|multi` — `single` renders as a
  radio group (exactly one option selected; lint + `set` enforce arity), `multi` as
  checkboxes (zero or more selected). Each option carries a stable string `id`
  (kebab-case, unique within the feature), a human `description`, a default
  `selected` state, and a typed payload `value` (int, float, string, or bool; all
  options within one feature must share the same value type — lint-enforced).
  Selecting an unknown option id is an error.
- **F4 layered resolution:** design §4 chain; overrides sparse; unknown key ⇒ CLI
  error (exit 2), distinct from `enabled`'s false (exit 1).
- **F5 git-persisted defaults:** tracked flag file; upward discovery from CWD probing
  `.gff/features.yaml` then `.github/gff/features.yaml` (`.gff/` wins if both);
  `git config gff.source` redirect overrides the probe entirely.
- **F6 machine registry:** `gff install` registers/refreshes {namespace, url,
  commit} + snapshot, keyed by the repo's reverse-DNS namespace; a different url
  installing an existing namespace is rejected naming the current owner. No area
  claims — two repos may both ship `install.*` keys under their own namespaces.
- **F7 shell/bridge interface:** `gff enabled <key>` (0=on, 1=off, 2=unknown key,
  unknown option id, or type the verb can't express — any exit ≥2 is a
  usage/definition error and shell callers MUST treat it as fail-open; never use
  `gff enabled` as an install gate — that's `gff_on`'s job);
  `gff selected <key> <option-id>` (0=selected, 1=not, 2=unknown key/id);
  `gff export --format shell|dotenv|json|yaml [-o <file>]` — shell/dotenv emit
  `GFF_<AREA>_<COMPONENT>_<FEATURE>=true|false|<id[,id…]>` (dots/dashes →
  underscores, uppercased; option ids are lint-constrained kebab so values stay
  injection-safe; dotenv output must parse with dotenv-family libs such as
  hashicorp/go-envparse and python-dotenv), json/yaml emit the full resolved snapshot
  including typed choice payloads — two encodings of the same struct (the non-Go
  language bridge).
- **F8 write path:** `set`/`unset`/TUI mutate only `~/.config/gff/config.yaml`
  (created 0600 on first write, parent dirs as needed).
- **F9 install.sh instrumentation:** 43 bool flags per plan P2-T1, all default on;
  gff built immediately after the goenv/Go step; every gate fails open; PS phases
  receive `GFF_*` env and treat unset as on.
- **F10 TUI:** tree nav, description/default/effective/winning-layer per row, bool
  toggle + choice picker (radio/checkbox per mode, options show id + description +
  typed value), quit-without-write safety.
  **Owner-approved extensions (PR #187 review, 2026-07-26):**
  - *Theme-aware palette:* the TUI and the styled `gff list` table resolve a palette
    (dark / light / dark8) per the gsl model — `GFF_THEME` env override → basic-ANSI
    fallback on low-color terminals (the terminal's own theme recolors them) →
    terminal background query (OSC-11 / COLORFGBG). Light palette follows gsl's
    mid-luminance layout.
  - *Category breadcrumb paging:* a fixed header lists the All page plus one page
    per distinct second path segment, alphabetically ("install (All) · ai · pkg …";
    bare "(All)" when multiple areas exist). ←/→ cycle pages; a category page shows
    only its features, flat.
  - *Feature detail view:* Enter on a feature row opens a detail pane — path, type,
    description, namespace, effective value, the full 5-layer table (each layer's
    contribution + the winning layer marked) via the additive `resolve.Explain`
    API, and the option list for choices. Esc/Enter/q returns. (Enter on an area
    row still expands/collapses.)
  - *Viewport-aware rendering:* rows render windowed to the terminal height under
    the fixed breadcrumb; the window follows the cursor; PgUp/PgDn page; overflow
    is indicated ("… N more above/below") — the terminal never hard-wraps the UI.
  - *Help overlay everywhere (`?`/`h`):* every view (list, detail, picker) opens a
    help overlay showing the tool name, version, the current view's key legend, and
    the SOURCES story — registry entries (●) plus discovered-but-unregistered
    origins (○, e.g. the CWD repo's live flag file) so the multi-source picture is
    complete. The launch frame itself stays clean (no always-on about panel); the
    footer advertises `? help`.
  - *Namespace-separated area rows + scoped breadcrumb:* one area row per
    (namespace, area) pair — two sources sharing an area name are visibly separate
    worlds — and the breadcrumb pages rescope to the cursor row's namespace
    (prefixed `<namespace> ▸` when several are present).
  - *Detail-view actions (existing writers only):* in the detail view, Space
    toggles the bool or opens the choice picker (the same `overrides.Write` path
    as `gff set`; the picker returns to the detail), and `u` clears the user
    override via `overrides.Unset` (the `gff unset` path); the layer table
    refreshes in place so cause and effect are visible.
  - *Width-aware `gff list`:* the styled table constrains itself to the terminal
    width (TTY size, else `$COLUMNS`) and wraps within cells — borders stay intact.
- **F11 zero-install invocation + cross-repo source:** every verb also works with no
  gff installed via `go run github.com/sfc-gh-eraigosa/dotfiles/sdk/gff@<tag> …`
  (pinned `sdk/gff/vX.Y.Z` tag recommended, `@latest` allowed; stdout carries only
  command output so `eval` is safe); the global `--source <registered-name|path>`
  flag scopes resolution to another repo's flags from any CWD (local only — never a
  network fetch at resolution time; unknown source ⇒ exit 2).

## 5. Evaluation criteria (per feature)

| Feature | Fires | Must not fire | Edge | Pass |
| :-- | :-- | :-- | :-- | :-- |
| F1/F2 lint | dup path, negative bool name, bad depth → non-zero + named finding | clean file | 2-level or 4-level path | table tests |
| F3 choice | unknown id rejected; 2 ids on `single` rejected | valid selection (1 id single / n ids multi) | empty options, dup ids, mixed value types, `single` with ≠1 default (all lint errors) | table tests |
| F4 resolve | higher layer wins per key | lower layer leaking through | key defined in no layer ⇒ unknown (exit 2) | matrix test over all 5 layers |
| F5 discovery | finds flag file at either probe path from nested CWD; follows `gff.source` redirect | files outside a repo | worktree (`.git` file not dir); both probe paths present (`.gff/` wins) | temp-repo tests |
| F6 registry | different url on an existing namespace rejected, owner named | re-install same repo (refresh); same short keys in two namespaces | moved clone (snapshot still serves) | registry tests |
| F7 export | emits every key, correct mangling | injection via description text (values are bool literals / kebab ids only) | choice flags export selected ids CSV | golden-file test |
| F8 writes | only user override mtime changes | any write to repo/system files | no `~/.config/gff` dir yet | fs-mock test |
| F9 gating | `false` ⇒ step body skipped, SKIP line printed | enabled steps changing behavior | gff binary absent ⇒ all run | `opt/lib/gff_test.sh` driver on the gate function (bash + dash) |
| F10 TUI | toggle writes override; `q` without change writes nothing | writes on navigation | terminal too small | teatest golden frames |
| F11 go-run + `--source` | `--source` path & registered name resolve from foreign CWD; `go run .` entrypoint works | network fetch during resolution | unknown source name/path (exit 2) | CI `go run . version` smoke + `cmd/read_test.go` cases |

## 6. Verification harness

- Go: table-driven unit tests per package; resolver matrix (every layer × bool/choice ×
  set/unset); ≥90% overall coverage, ≥95% `internal/resolve`, ≥90% `internal/schema`
  (gff's own CI bars, enforced per-package; the repo `sdk/` floor is 60%);
  `go vet` + lint in CI; binary-level e2e harness + adversarial suite + scripted
  demo per plan §7.
- Golden files: `export --shell` output, `gen` output, `list --json`.
- Proto: CI job regenerates and fails on diff.
- Shell: `make lint-shell` + `make lint-portability` on install.sh edits; a test
  sourcing the gate helper proving enabled/disabled/missing-binary behavior.
- TUI: bubbletea `teatest` snapshot tests (P3).
- Human-evidenced gate (P2): one full `install.sh` run on WSL with
  `install.windows.wispr-flow=false` showing the SKIP line; recorded in the PR.

## 7. Prerequisites / dependencies

- Go toolchain pinned by `.go-version` (already installed by install.sh before sdk builds).
- New deps: `google.golang.org/protobuf`, `github.com/charmbracelet/bubbletea`
  (+lipgloss/bubbles); regeneration only: system `protoc` + go.mod-pinned
  `protoc-gen-go` via `make gff-proto` (no buf; not needed by contributors since
  generated code is committed).
- `.gitignore`: `sdk/**` coverage exists for gss-era paths — verify `!`-rules cover
  `sdk/gff/**`, and that `!.github/**` covers `.github/gff/` (expected: yes — no new
  rules needed) before staging (repo allowlist gotcha).

## 8. Out of scope (and why)

- Flag servers, remote fetch, percentage rollouts, per-user targeting — gff is a local,
  git-backed system by design.
- Non-Go language SDKs — shell gets the CLI/env interface, and any language can
  consume `export --format dotenv|json|yaml` today; native SDKs (generated proto types +
  OpenFeature providers over the JSON snapshot) are a recorded post-P4 objective
  (design §8), not part of this one.
- Migrating existing config mechanisms (`~/.zshrc.local`, `ai/plugins.yaml`) onto gff —
  candidates later, not part of this objective.
- Auto-disabling any component — this objective only enumerates (all on); disabling
  decisions come after the switches exist.

## 9. Rollback

Per design §6: additive feature; fail-open gates degrade to current behavior; P2 is the
only phase touching existing files and reverts independently.

> Produced via `superpowers:brainstorming`. Plan: `../plans/gff.md` (next).
> Registered in `../index.md`.
