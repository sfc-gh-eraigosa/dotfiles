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
- Two value types: **bool** (strictly `true`=on / `false`=off; negative-named flags
  are rejected by lint) and **choice** — an option set with `mode: single` (radio:
  exactly one selected) or `mode: multi` (checkboxes: zero or more selected), where
  each option has a stable string id, a human description, a default selected state,
  and a typed payload value (int/float/string/bool; homogeneous within one feature).
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
  features[]}`, `Feature{path, description, oneof default}`,
  `ChoiceDefault{mode: single|multi, options[]}`,
  `ChoiceOption{id, description, selected, oneof value: int64|double|string|bool}`,
  `Value{oneof: bool | ChoiceSelection{selected ids[]}}`,
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
- **`cmd/`** — cobra verbs: `get`, `enabled` + `selected` (exit-code gates), `set`/
  `unset` (user override only), `list [--json]`, `install`,
  `export --format shell|dotenv|json` (`GFF_<AREA>_<COMPONENT>_<FEATURE>=…`; dotenv
  parses with dotenv-family libs; json = full resolved snapshot with typed choice
  payloads), `gen`, `lint`, `tui`, `version`.
- **`internal/tui`** — bubbletea tree: area → component → feature; shows description,
  default, effective value + winning layer; bool rows toggle, choice rows open a
  radio (`single`) or checkbox (`multi`) picker; all writes go to the user override.
- **`pkg/gff`** — the public Go SDK (runtime API); `gff gen` emits typed accessors.
- **Invocation modes** — (1) installed binary (`build.sh` → `~/opt/bin/gff`, or
  `go install …/sdk/gff@<tag>`); (2) **zero-install, first-class**:
  `go run github.com/sfc-gh-eraigosa/dotfiles/sdk/gff@<tag> <verb> …` — the main
  package sits at the module root and CI smokes `go run .`, so any repo's scripts can
  gate on flags with only a Go toolchain (pin a `sdk/gff/vX.Y.Z` tag for
  reproducibility; `@latest` allowed). The global `--source <registered-name|path>`
  flag scopes resolution to another repo's flags from any CWD — purely local, no
  network fetch at resolution time.
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

## 7. Prior art & landscape comparison (added 2026-07-25)

Survey of how feature flags are managed elsewhere, simple → complex, and where gff sits.
Sources: [GrowthBook's 2026 OSS comparison](https://www.growthbook.io/blog/best-open-source-feature-flagging-tools-compared),
[GO Feature Flag's tool survey](https://gofeatureflag.org/blog/best-opensource-feature-flag-tools),
[Flipt](https://github.com/flipt-io/flipt), [flagd](https://github.com/open-feature/flagd),
[GO Feature Flag](https://github.com/thomaspoignant/go-feature-flag),
[Fowler on feature toggles](https://martinfowler.com/articles/feature-toggles.html),
[Home Manager](https://nixos.wiki/wiki/Home_Manager),
[chezmoi cross-platform patterns](https://recca0120.github.io/en/2026/04/13/chezmoi-dotfiles-management/).

### 7.1 The landscape, simple → complex

| Tier | Representatives | Model | Overlap with gff | Why it doesn't fit this problem |
| :-- | :-- | :-- | :-- | :-- |
| Ad-hoc toggles | env vars, `features.conf` sourced by shell, Makefile vars (the classic "toggle configuration" pattern) | Flat `KEY=val`, no schema | The `GFF_*` export surface is exactly this tier | No descriptions, no choice type, no layered precedence, no discovery (`list`/TUI), no lint — every consumer reinvents parsing and defaults |
| Dotfiles-manager native | Nix Home Manager (`programs.X.enable`), chezmoi (Go templates + `.chezmoidata` + `.chezmoiignore`) | Per-machine conditional config baked into the dotfiles framework | Closest in *purpose*: toggling install components per machine, declaratively | All-or-nothing adoption — replaces the symlink/`install.sh` model entirely rather than instrumenting it; toggles aren't runtime-queryable by arbitrary scripts or a second repo |
| Git/file-native flag engines | **Flipt v2** (git-native storage, read-only GitOps mode), **GO Feature Flag** (single flag file, retrievers incl. GitHub/git), **flagd** (OpenFeature daemon, file sources) | Flags-as-code in git, evaluated by a daemon/relay or an in-process app SDK | Same core philosophy as gff: flags tracked in git, PR-reviewed, no vendor DB | Built for *long-running application processes* (HTTP/gRPC evaluation, streaming updates, percentage rollouts). Gating a bootstrap shell script would mean running a daemon during install or embedding their Go SDK — and none has local 5-layer system/user override precedence, exit-code gating, or a multi-repo area registry |
| Spec/standard | **OpenFeature** (CNCF) | Vendor-neutral evaluation API; flagd is its reference daemon | Key naming + bool/typed-value evaluation semantics | A standard, not an engine — still needs a provider backing it |
| Server platforms | Unleash, Flagsmith, GrowthBook, PostHog, LaunchDarkly (commercial) | Central service + DB + dashboard; targeting, approvals, experiments, analytics | Concepts only (flag, default, override) | Requires a running service and network at evaluation time; dotfiles install must work offline on a fresh machine. Their differentiators (targeting, A/B, audit workflows) are explicit non-goals (§2) |

### 7.2 What no surveyed tool provides

The intersection gff occupies is not covered by any single existing tool:

1. **Process-less local evaluation** — one static binary, no daemon, works mid-bootstrap
   and offline. Flipt/flagd/GOFF all assume a serving process or an app-embedded SDK.
2. **Shell as a first-class consumer** — `gff enabled <key>` exit codes and
   `eval "$(gff export --shell)"` for bash *and* PowerShell (WSLENV handoff). Every
   surveyed engine targets application SDKs; shell gating is an afterthought at best.
   The same interface works **zero-install** —
   `eval "$(go run github.com/sfc-gh-eraigosa/dotfiles/sdk/gff@<tag> export --format shell --source <name>)"`
   compiles gff inline from the module proxy, so a repo's scripts can gate on flags
   on machines that never installed it.
3. **Layered local precedence** — system snapshot → user snapshot → live repo → system
   override → user override, XDG-style, with the winning layer reported. Flag engines
   have environments/namespaces, not machine-local override layering.
4. **Multi-repo area registry** — several repos each claiming an area, merged by one
   CLI on the machine (`gff install`, exclusive claims). Nothing comparable exists;
   the closest analogue is package-manager metadata, not flag tooling.
5. **Opinionated minimal typing** — bool (negatives rejected by lint) + indexed choice.
   Deliberately smaller than every surveyed tool.

Where gff deliberately trails the field: no percentage rollouts, targeting,
experimentation, audit dashboards, or streaming updates (§2 non-goals) — and it should
never grow them; that tier is served well by Unleash/GrowthBook et al.

### 7.3 Is it still worth implementing?

**Yes — with one honest caveat.**

- **The niche is real and unserved.** "Gate the steps of an offline bootstrap script,
  per machine, with repo-dictated defaults, across two OSes and multiple repos" is not
  what any surveyed tool does. Adopting the nearest engines (flagd/GOFF/Flipt) would
  import daemon lifecycle or app-SDK assumptions *and* still leave the shell interface,
  override layering, and area registry to be built — i.e., most of gff's actual work.
- **The tools that do solve install-toggling (Home Manager, chezmoi) cost a migration
  of the entire dotfiles model** — abandoning `install.sh` + symlinks for Nix or
  chezmoi templating. That blast radius dwarfs adding one more Go CLI to an `sdk/`
  that already ships four, built with the same conventions.
- **The expensive parts of flag systems are the parts we excluded.** Targeting engines,
  analytics, streaming, dashboards — the man-years in Unleash/LaunchDarkly — are
  non-goals. What remains (file schema, layered merge, cobra CLI, TUI) is a scope this
  repo has delivered repeatedly (gss, gsl, tmux-mgr).
- **Fail-open keeps the downside bounded.** Worst case, gff misbehaves and every gate
  degrades to today's unconditional behavior (§6 rollback).
- **Per-installation optionality is the adoption unlock.** Today the repo is
  all-or-nothing: if 10% of the flow or tools don't fit an instance, the only outs are
  editing the shared repo (a fork in spirit — machine-specific decisions leak into
  everyone's defaults) or not adopting at all. One size won't fit all, and we currently
  have no room to flex. gff's override layers make partial adoption the normal case:
  keep the 90% that works, turn off the 10% that doesn't, stay fully functional — per
  installation via `~/.config/gff/config.yaml`, and per system or account via the
  system/user layer split — **without a single edit to the dotfiles repo**.
- **A safe runway for experimental features.** There is currently no way to ship a
  feature before it's hardened: landing it in `install.sh` imposes it on every machine
  at once. With gff, a new component can land defaulted *off* (or on for the author's
  machines only, via their override) and be flipped on per installation as it matures —
  the TUI and layered config make that per-machine state easy to see (winning-layer
  provenance) and cheap to maintain. Graduation = flipping the tracked default in one
  reviewed PR.
- **Caveat, narrowed:** if scope were ever cut to *only* bool-gating `install.sh` — no
  choice type, no TUI, no multi-repo — a ~50-line sourced `features.conf` could deliver
  the mechanical gating. But note what it could **not** deliver: a repo-tracked conf is
  still an edit-the-shared-repo-per-machine model, so it solves none of the two points
  above — machine-local overrides, per-system/account layering, and provenance are the
  engine's core, not garnish. The revisit trigger is therefore narrower than first
  stated: reconsider only if the *override layering itself* were ever dropped — losing
  the registry (UC3) or TUI (UC2) alone would weaken but not void the case.
- **Future option, not a blocker:** gff's file format could later be exposed via an
  [OpenFeature](https://openfeature.dev/) file-provider so app code in registered repos
  can consume the same flags through the standard API. Noted for a post-P4 objective.

## 8. Cross-language consumption strategy (future work, recorded 2026-07-25)

In-scope today: the Go SDK (`pkg/gff`) plus two universal bridges — the CLI exit-code
gates (`enabled`/`selected`) for shell and any language willing to exec, and
`export --format dotenv|json` for anything that can read a file (dotenv-family libs:
hashicorp/go-envparse, joho/godotenv, python-dotenv, dotenv-java/npm; env/dotenv carry
selected ids only — typed choice payloads require the json form). Neither bridge is
ever deprecated.

Native SDKs for Python/Java/TypeScript (no shelling out) are a **post-P4 objective**
with a decided shape:

1. **Types are free** — `buf generate` emits each language's message types from the
   same `features.proto`; the typed choice options survive codegen.
2. **Semantics stay in one place** — rather than reimplementing the 5-layer resolver
   per language (drift; would demand a cross-language conformance corpus) or shipping
   a cgo `libgff` FFI (cross-compile/packaging toil), non-Go SDKs read the
   **resolved JSON snapshot** produced by `gff export --format json`: ~100 lines per
   language, the Go resolver remains the single semantic authority, offline-safe.
   Staleness (re-export after flag changes) is acceptable for machine-config flags.
3. **API surface via [OpenFeature](https://openfeature.dev/)** — each language gets a
   thin OpenFeature provider over the snapshot, reusing the existing OpenFeature SDKs
   instead of inventing per-language APIs.

Escalate to the FFI approach only if a consumer needs live in-process resolution
without a snapshot; reassess then.

> Produced via `superpowers:brainstorming`. Registered in `../index.md`; spec at
> `../specs/gff.md`. §7 prior-art survey added 2026-07-25 at user request. Choice type
> generalized 2026-07-25: single (radio) / multi (checkbox) modes, stable string
> option ids, per-option typed values (int/float/string/bool, homogeneous per
> feature) — replaces the original `selected int` + `map<int,string>` scheme.
> 2026-07-25 (later): zero-install `go run <module>@<tag>` promoted to a first-class
> invocation mode and global `--source <name|path>` added for cross-repo resolution.
