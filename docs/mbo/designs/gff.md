# gff — git fast features: a generic, git-persisted feature-flag engine — design

- **Slug:** gff
- **Date:** 2026-07-23
- **Status:** Approved
- **Relates to:** spec `../specs/gff.md` / issue (pending) / PR (pending)
- **Author(s):** Edward Raigosa + Claude Code (superpowers:brainstorming)

## 1. Problem / context

`install.sh` (581 lines) unconditionally runs ~25 sequential component steps (profile
symlinks, package installs, tool fetchers, runtime managers, AI-CLI setup, the four
`sdk/` builds, Windows Desktop deployment). There is no way to turn a component off on
one machine without editing the script. The Windows side is worse: `install_windows.sh`
fans out to PowerShell phases (WSL platform, Terminal themes, winget apps, the elevated
batch installing the Wispr Flow MSI, the PowerToys Copilot-key remap, AHK autostart)
with a single y/n prompt gating all of them.

We want per-component feature flags with repo-dictated defaults and personal overrides —
and the flag engine itself should be **generic**: usable by any repo, not hardcoded to
dotfiles. Multiple repos should be able to define flags for their own areas and deploy
them to a machine where one `gff` reads the merged set.

Verified facts grounding this design:

- `sdk/` is the home for Go CLIs; `sdk/gss` is the structural template (cobra `cmd/`,
  `internal/` with a mockable runner, `build.sh`, `internal/version` via ldflags,
  `go install github.com/sfc-gh-eraigosa/dotfiles/sdk/<tool>`).
- `~/opt` is a symlink to the repo's `opt/` (created by `install.sh`), so a repo-shipped
  defaults snapshot under `opt/conf/` is visible at `~/opt/conf/` on installed machines.
- No protobuf toolchain exists in the repo yet; gff introduces it.
- `install.sh` installs the Go toolchain (goenv + pinned `.go-version`) *before* the
  `sdk/` builds — so gff can be built mid-install and gate every step after it.

## 2. Goals & non-goals

**Goals**

- A generic feature-flag engine (`sdk/gff`) with zero repo-specific keys compiled in.
- Hierarchical canonical keys: `area → component → feature` (dotted paths).
- Two value types: **bool** (strictly `true`=on / `false`=off; negative-named flags are
  rejected by lint) and **choice** (`selected int` over an indexed option set
  `map<int,string>` where the string is the human description).
- Repo-dictated defaults persisted in git (tracked, diffable, PR-reviewed) + personal
  and system overrides at well-known paths.
- Multi-repo: any repo can define flags for its own claimed area(s); `gff install`
  registers + snapshots them machine-wide; one CLI reads the merged set anywhere.
- Cobra CLI + bubbletea TUI; Go client SDK (runtime API + optional typed codegen).
- Instrument `install.sh` (and the Windows PowerShell phases) with per-step flags,
  all defaulting **on**.

**Non-goals**

- No flag server / network fetch — resolution is purely local files + git.
- No percentage rollouts, A/B experiments, or per-user targeting.
- No cross-language SDKs beyond Go + the shell interface (CLI exit codes / env export).
- Not a general config-management system: values are bool/choice only, deliberately.

## 3. Options considered

**Schema source of truth**

1. **Proto files canonical (chosen):** `.proto` defines the generic *shape* (FeatureSet,
   Feature, bool/choice values, source registry); flag *data* is never compiled in.
   Strong typing, one contract, protojson gives YAML/JSON encodings for free.
2. YAML registry canonical, proto generated from it — easier hand-editing but proto
   becomes derived output rather than the contract.
3. Go code canonical — no protoc dependency but weakest cross-language story.

Rejected initially-considered variant: hardcoding the dotfiles flag keys/defaults into
the proto — defeats generalization; the engine must stay data-free.

**Git persistence of defaults**

1. **Tracked file + git-config discovery (chosen):** `.gff/features.yaml` tracked in the
   repo; discovery walks up from CWD to the repo root (like `.git` discovery); a
   `[gff] source` git-config key can redirect to another path/repo.
2. Pure git-config storage (`git config -f .gffconfig`) — dotted keys map naturally but
   descriptions/choice sets don't fit flat config values.
3. Dedicated ref (`refs/gff/*`) — most git-native but bypasses PR review and needs
   refspec plumbing on every host.

**Machine-wide deployment**

1. **Registry + snapshot on install (chosen):** `gff install` registers the repo as a
   source (url, owned areas, commit) in `~/.config/gff/sources.yaml` and snapshots its
   defaults into the defaults layer. Areas are claimed exclusively.
2. `go:embed` provider binaries per repo — go-install native but N binaries + exec per
   read.
3. Live refs to clone paths only — breaks when a clone moves; nothing on checkout-less
   machines.

**SDK access style**

1. **Generic runtime API + optional codegen (chosen):** `gff.Bool("a.b.c")` always
   works; `gff gen` optionally emits typed accessors for compile-time key safety.
2. Codegen-only — max type safety but shell still needs string keys, so two grammars
   exist regardless.
3. Runtime-only — simplest but typos surface only at runtime.

## 4. Decision

`sdk/gff` — a Go module mirroring `sdk/gss` structure:

- **`proto/gff/v1/features.proto`** — generic messages only: `FeatureSet{area,
  features[]}`, `Feature{path, type, description, default_value}`, `BoolValue`,
  `ChoiceValue{selected, options map<int32,string>}`, `Config{overrides}`,
  `Source{name, url, areas[], commit}`, `SourceRegistry`. Generated Go is committed so
  contributors don't need protoc; `buf` (or pinned protoc in `build.sh`) regenerates.
- **`internal/schema`** — load/validate `.gff/features.yaml|json` (protojson-compatible
  encodings of `FeatureSet`); lint rules (no negative bool names, unique paths, valid
  choice indices).
- **`internal/resolve`** — the layer chain (lowest → highest):
  1. source-default snapshots: `/opt/conf/gff/<source>.yaml` (system), then
     `~/opt/conf/gff/<source>.yaml` (user/repo-shipped);
  2. live repo defaults: `<repo>/.gff/features.yaml` when CWD is inside a registered
     repo (git-style upward discovery; `[gff] source` git-config redirect honored);
  3. overrides: `/var/opt/conf/gff/config.yaml` (system), then
     `~/.config/gff/config.yaml` (user — the **only** file gff ever writes).
  Sparse per-key merge; effective value = highest layer that sets the key. The resolver
  reports *which* layer won (surfaced in `list`/TUI).
- **`internal/registry`** — `~/.config/gff/sources.yaml` management; exclusive area
  claims; snapshot refresh on `gff install`.
- **`cmd/`** — cobra verbs: `get`, `enabled` (exit-code gate), `set`/`unset` (user
  override only), `list [--json]`, `install`, `export --shell`
  (`GFF_<AREA>_<COMPONENT>_<FEATURE>=…`), `gen`, `lint`, `tui`, `version`.
- **`internal/tui`** — bubbletea tree: area → component → feature; shows description,
  default, effective value + winning layer; toggling writes the user override.
- **`pkg/gff`** — the public Go SDK (runtime API); `gff gen` emits typed accessors.
- **Dotfiles instrumentation** — `.gff/features.yaml` at the repo root enumerating
  ~36 flags (all default on) across `install.shell.*`, `install.pkg.*`,
  `install.tools.*`, `install.runtime.*`, `install.ai.*`, `install.sdk.*`,
  `install.fonts.*`, `install.network.*`, and `install.windows.*` (incl.
  `install.windows.wispr-flow`). `install.sh` builds gff right after the Go toolchain,
  then gates each later step; steps before Go exists stay unconditional. Bash passes
  the flag set to PowerShell via `GFF_*` environment so `setup-apps.ps1` /
  `setup-elevated.ps1` skip disabled phases. Every gate **fails open** (missing
  binary/key ⇒ default on) so bootstrap never breaks.

Build phasing: **P1** engine (proto + resolver + core CLI) → **P2** dotfiles
enumeration + install.sh/PowerShell gating → **P3** TUI → **P4** `gff gen`.

## 5. Risks & blast radius

- **install.sh regression** is the big one: a wrong gate skips a component silently on
  every machine. Mitigated by fail-open semantics, flag-per-existing-block (no step
  reordering), and a bats-style test proving disabled ⇒ skipped / enabled ⇒ runs.
- **Proto toolchain drift**: committed generated code + pinned generator in `build.sh`;
  CI check that regeneration is clean.
- **Area-claim collisions** across repos: registry rejects a second claim; error
  message names the existing owner.
- **Shell portability**: all shell edits obey `docs/mbo/specs/shell-portability.md`;
  `make lint-shell` + `lint-portability` gate them.
- **Windows/PowerShell coupling**: env-var handoff only; PS phases treat an unset
  `GFF_*` var as on (same fail-open rule).

## 6. Rollback

- gff is additive: removing the `.gff/` dir, the `sdk/gff` build block, and the
  `gff_on`-style gates restores today's unconditional install.sh behavior.
- Fail-open means a broken/missing gff binary already degrades to current behavior.
- Per-phase PRs (P1–P4) revert independently; P2 (instrumentation) is the only one
  touching existing behavior.

> Produced via `superpowers:brainstorming`. Registered in `../index.md`; spec at
> `../specs/gff.md`.
