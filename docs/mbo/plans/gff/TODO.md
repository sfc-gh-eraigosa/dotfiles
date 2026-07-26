# gff — TDD execution cursor

- **Slug:** gff
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Ledger:** [`TRACKING.md`](./TRACKING.md)
- **Plan (source of truth):** [`../gff.md`](../gff.md) — every `§` reference below points there

> **How to use:** the **first unchecked box is your next action**. Steps are ordered and
> self-sufficient. Tick a box only after you ran the command and read the output.
> After finishing a `###` task: update `TRACKING.md`, commit with the exact message, and
> `gss feature checkpoint`.
>
> **Legend:** `SETUP` prep · `RED` write a failing test · `RUN-RED` run it, expect FAIL ·
> `GREEN` implement · `RUN-GREEN` run it, expect PASS · `VERIFY` extra gate ·
> `ALLOWLIST` `.gitignore` check · `DOCS` · `COMMIT` · `LEDGER` update TRACKING.md ·
> `CHECKPOINT` `gss feature checkpoint`.
>
> **Paths:** `<WT>` = the leaf's worktree path captured from `gss feature worker add --json`.
> Go commands run from `<WT>/sdk/gff/`; `make` targets from `<WT>/`. Never work in
> `${HOME}/git/dotfiles` except where a step says so explicitly (P2-T5 only).

---

## Preflight (once, before any leaf)

- [x] Confirm design PR #181 is merged: `git -C "${HOME}/git/dotfiles" fetch origin && git -C "${HOME}/git/dotfiles" show origin/main:docs/mbo/plans/gff.md | head -5`
- [x] Confirm toolchain: `go version` (matches `.go-version` = 1.26.1), `git --version`, `gh auth status`, `gss feature list`
- [x] Confirm `command -v protoc` (needed only for P1-T2 / regen checks); install if absent
- [x] Read plan §3 in full — the frozen contracts (proto, file formats, Go interfaces, CLI, shell)
- [x] Read plan §6.1 (leaf DAG) and §7 (validation plan, IH-*/IA-*, §7.4 matrix, §7.5 done-when)
- [x] Read `IMPLEMENTATION.md` §3 (the per-task loop) and §5 (hard rules)

---

# Leaf 1 — `p1-engine` (BLOCKING — nothing else starts until this merges)

## Leaf setup

- [x] `gss feature worker add --feature gff --purpose p1-engine --engine claude --json --description "P1 engine: proto schema, resolver, registry, core CLI, SDK, CI, e2e harness (#180)"`
- [x] Record `worker_ref` / `branch` / `worktree_path` **verbatim** in `TRACKING.md` §0
- [x] `cd <WT>` and confirm you are on the worker branch (`git status -sb`)

---

### P1-T1 — module scaffold + version  (plan §4 P1-T1)

**Files:** `sdk/gff/{go.mod,main.go,VERSION,build.sh,README.md,AGENTS.md,LICENSE}`,
`sdk/gff/cmd/{root.go,version.go,root_test.go}`, `sdk/gff/internal/version/version.go`,
symlink `sdk/gff/CLAUDE.md → AGENTS.md`.

- [x] SETUP: `mkdir -p <WT>/sdk/gff/{cmd,internal/version}`
- [x] SETUP: copy `sdk/gsl/build.sh` → `sdk/gff/build.sh`; replace every `gsl` with `gff` (stamps `internal/version` via ldflags only — no build vars in `cmd`/`main`)
- [x] SETUP: write `sdk/gff/VERSION` = `0.1.0`; copy `sdk/gsl/LICENSE` → `sdk/gff/LICENSE`
- [x] SETUP: `go.mod` — module `github.com/sfc-gh-eraigosa/dotfiles/sdk/gff`, `go 1.26.1`; `go get github.com/spf13/cobra`
- [x] SETUP: `internal/version/version.go` — vars `Version, Commit, BuildDate, Dirty = "dev","none","unknown","false"` + `func String() string` (mirror gsl's format)
- [x] RED: write `cmd/root_test.go` `TestVersionCommand` exactly as the plan's snippet (buffers on `NewRootCmd()`, args `{"version"}`, asserts output contains `gff`)
- [x] RUN-RED: `go test ./cmd/` → expect **FAIL** (`NewRootCmd` undefined)
- [x] GREEN: `cmd/root.go` — `NewRootCmd() *cobra.Command` (Use `gff`, Short "git fast features — layered feature flags persisted in git") + `Execute()`; later verbs self-register via `init()` + `rootCmd.AddCommand` (copy the `sdk/gsl/cmd` pattern)
- [x] GREEN: `cmd/version.go` printing `version.String()`; `main.go` calling `cmd.Execute()`
- [x] RUN-GREEN: `go test ./... && go vet ./...` → expect **PASS**
- [x] VERIFY: `bash build.sh` installs `${HOME}/opt/bin/gff`; `${HOME}/opt/bin/gff version` prints the version block
- [x] DOCS: write `sdk/gff/README.md` and `sdk/gff/AGENTS.md`; `ln -s AGENTS.md sdk/gff/CLAUDE.md`
- [x] DOCS: add the `sdk/gff` line to the root `CLAUDE.md` Repository Structure (plan §6)
- [x] ALLOWLIST: `git status --short -- sdk/gff/` and `git check-ignore -v sdk/gff/go.mod` — expect `!sdk/**` coverage, no new rules
- [x] COMMIT: `feat(gff): scaffold sdk/gff module with cobra root + version`
- [x] LEDGER + CHECKPOINT

**Done when:** `go test ./... && go vet ./...` pass and `bash build.sh` yields a working `gff version`.

---

### P1-T2 — proto schema + committed codegen (raw protoc, NO buf)  (plan §4 P1-T2)

**Files:** `sdk/gff/proto/gff/v1/features.proto`, `sdk/gff/scripts/genproto.sh`,
committed `sdk/gff/gen/gff/v1/*.pb.go`; modify root `Makefile` + `.gitignore`.

- [x] SETUP: `go get google.golang.org/protobuf@latest`
- [x] SETUP: `go get -tool google.golang.org/protobuf/cmd/protoc-gen-go@latest` (pins the plugin version in `go.mod`)
- [x] SETUP: confirm `command -v protoc` (regeneration only — contributors build from committed output)
- [x] GREEN: write `proto/gff/v1/features.proto` **verbatim from plan §3.1** — frozen contract, no additions
- [x] GREEN: write `scripts/genproto.sh` exactly as the plan's snippet (raw protoc, `GOBIN="${PWD}/.bin" go install`, `--proto_path=proto --go_out=gen --go_opt=paths=source_relative`); `chmod +x`
- [x] GREEN: root `Makefile` — `gff-proto` → `bash sdk/gff/scripts/genproto.sh`; `gff-proto-check` → `gff-proto` then `git diff --exit-code -- sdk/gff/gen/`
- [x] ALLOWLIST: add an **ignore** rule for `sdk/gff/.bin/` in `.gitignore` with a comment (otherwise `!sdk/**` tracks the plugin binary); verify `git check-ignore -v sdk/gff/.bin/protoc-gen-go`
- [x] RUN-GREEN: `make gff-proto` → `sdk/gff/gen/gff/v1/features.pb.go` exists, package `gffv1`
- [x] VERIFY: `go build ./...` PASS; `go vet ./...` PASS
- [x] VERIFY: `make gff-proto-check` → clean (regeneration idempotent)
- [x] ALLOWLIST: `git status --short -- sdk/gff/gen/` shows the generated file as trackable
- [x] COMMIT: `feat(gff): proto v1 schema + raw-protoc codegen behind make gff-proto (committed output)`
- [x] LEDGER + CHECKPOINT

**Done when:** `make gff-proto-check` is clean and `go build ./...` passes against committed `gen/`.

---

### P1-T3 — schema load + lint  (plan §4 P1-T3)

**Files:** `internal/schema/{schema.go,lint.go,schema_test.go,lint_test.go}`.

> **NOTE (resolved 2026-07-25):** the `int→ChoiceValue` leftover this note used to
> flag was fixed in the plan after the PR #181 team review — §3.2 and the §4 P1-T3
> prose now agree: **bool → BoolValue, string/[]string → ChoiceSelection, anything
> else = parse error naming key and type.** No inconsistency to log.

- [x] RED: `schema_test.go` — `TestLoadFeatureFileYAML` exactly as the plan's snippet (asserts bool default, `CHOICE_MODE_SINGLE`, option id `apt` + `stringValue`, default `selected`)
- [x] RED: `schema_test.go` — `TestLoadOverrides` (bool override, single-choice `apt` ⇒ 1 selected id, multi `[fzf, starship]` ⇒ 2 selected ids)
- [x] RED: `schema_test.go` — `TestLoadOverridesMissingFile` (absent sparse layer ⇒ empty map, nil error)
- [x] RED: `lint_test.go` table case — duplicate `path` within a file ⇒ finding
- [x] RED: `lint_test.go` table case — negative name per prefix: `no-*`, `not-*`, `disable-*`, `disabled-*`, `skip-*`, `off-*` on the last segment ⇒ finding each
- [x] RED: `lint_test.go` table case — depth 2 (`install.claude`) and depth 4 ⇒ finding (exactly 3 dotted segments)
- [x] RED: `lint_test.go` table case — uppercase segment and underscore segment ⇒ finding (segment regex `^[a-z0-9]+(-[a-z0-9]+)*$`)
- [x] RED: `lint_test.go` table case — choice with empty `options` ⇒ finding
- [x] RED: `lint_test.go` table case — duplicate option ids, and non-kebab option id ⇒ finding
- [x] RED: `lint_test.go` table case — mixed value types within one feature ⇒ finding (homogeneous per feature)
- [x] RED: `lint_test.go` table case — `single` mode with zero and with two default-selected options ⇒ finding
- [x] RED: `lint_test.go` table case — `mode` unspecified (`CHOICE_MODE_UNSPECIFIED`) ⇒ finding
- [x] RED: `lint_test.go` table case — `path` not starting with its set's `area.` ⇒ finding
- [x] RED: `lint_test.go` table case — clean file ⇒ zero findings
- [x] RUN-RED: `go test ./internal/schema/` → expect **FAIL** (undefined `LoadFeatureFile`/`LoadOverrides`/`Lint`)
- [x] GREEN: `schema.go` `LoadFeatureFile` — `.json` ⇒ `protojson.Unmarshal`; `.yaml|.yml` ⇒ `yaml.Unmarshal` into `any`, recursively normalize map keys to strings, `json.Marshal`, `protojson.Unmarshal` with `DiscardUnknown: false`
- [x] GREEN: `schema.go` `LoadOverrides` — `os.IsNotExist` ⇒ `(map{}, nil)`; yaml scalar map; bool ⇒ `BoolValue`; string or string-list ⇒ `ChoiceSelection`; anything else ⇒ error naming the key
- [x] GREEN: `lint.go` `Lint(f) []Finding{Path, Rule, Msg}` — seen-set for duplicates, per-segment regex, 3-segment depth, negative-prefix check on the last segment, area-prefix check, and the choice rules (non-empty options, unique kebab ids, homogeneous value type, `single` arity, mode set)
- [x] RUN-GREEN: `go test ./internal/schema/` → expect **PASS**
- [x] VERIFY: `go test ./internal/schema/ -cover` → **≥90%**; record the number in `TRACKING.md` §9
- [x] COMMIT: `feat(gff): schema loader (yaml/json protojson) + lint rules`
- [x] LEDGER: tick F1 / F2 / F3 **unit** cells; CHECKPOINT

**Done when:** schema package tests pass at ≥90% coverage.

---

### P1-T4 — paths + git discovery  (plan §4 P1-T4)

**Files:** `internal/paths/{paths.go,paths_test.go}`, `internal/gitx/{gitx.go,gitx_test.go}`.

- [x] RED: `paths_test.go` — `Default()` fields end with the exact §3.3 suffixes: `/opt/conf/gff`, `${HOME}/opt/conf/gff`, `/var/opt/conf/gff/config.yaml`, `${HOME}/.config/gff/config.yaml`, `${HOME}/.config/gff/sources.yaml` (resolve home via `os.UserHomeDir`)
- [x] RED: `gitx_test.go` — `RepoRoot`: temp tree `a/b/c` with `.git` **dir** at `a` ⇒ found from `c`
- [x] RED: `gitx_test.go` — `RepoRoot`: `.git` **file** (`gitdir: …`, the worktree case) at `a` ⇒ found
- [x] RED: `gitx_test.go` — `RepoRoot`: no `.git` anywhere ⇒ `("", false)`
- [x] RED: `gitx_test.go` — add the `fakeRunner{out, err}` helper from the plan snippet
- [x] RED: `gitx_test.go` — `SourcePath`: runner returns `custom/flags.yaml` ⇒ joined to repoRoot (redirect wins over probing)
- [x] RED: `gitx_test.go` — `SourcePath` probe order (runner errors): only `.gff/features.yaml` ⇒ that path; only `.github/gff/features.yaml` ⇒ that path; both present ⇒ `.gff/features.yaml` wins; neither ⇒ `.gff/features.yaml` (missing live layer is simply absent)
- [x] RUN-RED: `go test ./internal/paths/ ./internal/gitx/` → expect **FAIL**
- [x] GREEN: `paths.go` — `Paths` struct + `Default() (Paths, error)` exactly per §3.3, `WorkDir` = CWD, all fields overridable so tests can point at temp dirs
- [x] GREEN: `gitx.go` — `RepoRoot`: `os.Stat(filepath.Join(dir, ".git"))` accepting dir **or** file, walking up to the filesystem root
- [x] GREEN: `gitx.go` — `SourcePath`: `r.Output(repoRoot, "config", "--get", "gff.source")`, trim; relative ⇒ join to repoRoot; on error ⇒ probe order above
- [x] GREEN: `gitx.go` — real `ExecRunner` implementing `Runner`, execing `git -C <dir> <args...>` via `os/exec`
- [x] RUN-GREEN: `go test ./internal/paths/ ./internal/gitx/` → expect **PASS**
- [x] COMMIT: `feat(gff): well-known paths + git-style repo discovery with gff.source redirect`
- [x] LEDGER: tick F5 **unit** cell; CHECKPOINT

**Done when:** both packages' tests pass, including the `.git`-file worktree case.

---

### P1-T5 — resolver (the core)  (plan §4 P1-T5)

**Files:** `internal/resolve/{resolve.go,resolve_test.go}`.

- [x] RED: build the fixture — `type world struct{ sysSnap, userSnap, repo, sysOvr, usrOvr string }` and `newResolver(t, w)` writing each non-empty layer into `t.TempDir()`, WorkDir = repo (plan snippet)
- [x] RED: case 1 — key only in system snapshot ⇒ default value, `Layer == LayerSystemSnapshot`
- [x] RED: case 2 — same key also in user snapshot ⇒ user-snapshot definition wins, `LayerUserSnapshot`
- [x] RED: case 3 — live repo file redefines the default ⇒ `LayerRepoLive`
- [x] RED: case 4 — system override flips the value ⇒ `LayerSystemOverride`
- [x] RED: case 5 — user override flips it back ⇒ `LayerUserOverride`
- [x] RED: case 6 — override for an UNKNOWN key ⇒ ignored by `All()`; `Resolve` ⇒ `ErrUnknownKey`
- [x] RED: case 7 — choice: default selection wins with no override; override to valid id(s) ⇒ that selection; unknown id, or two ids on a `single`-mode flag ⇒ error naming the key and the offending ids
- [x] RED: case 8 — WorkDir outside any git repo ⇒ snapshots + overrides still resolve
- [x] RED: case 9 — `All()` sorted by `Feature.Path`; sparse overrides never invent keys
- [x] RED: case 10 — `Source: <local path>` resolves that repo's live file though WorkDir is elsewhere; `Source: <registered name>` resolves from that source's snapshot from any CWD; unknown name/path ⇒ `ErrUnknownSource`
- [x] RUN-RED: `go test ./internal/resolve/` → expect **FAIL**
- [x] GREEN: load definition layers in order — every `*.yaml|*.json` in `SystemSnapshotDir`, then `UserSnapshotDir`, then the live file via `gitx.RepoRoot(P.WorkDir)` + `SourcePath`; a later def layer replaces a path's `(Feature, defLayer)`
- [x] GREEN: apply the two override maps in order; validate choice ids + mode arity against the **winning** definition
- [x] GREEN: effective value = default unless overridden; `Layer` = the winning layer; expose `All()` (sorted) and `Resolve(key)` per §3.3
- [x] RUN-GREEN: `go test ./internal/resolve/` → expect **PASS**
- [x] VERIFY: `go test ./internal/resolve/ -cover` → **≥95%**; record in `TRACKING.md` §9
- [x] COMMIT: `feat(gff): 5-layer resolver with provenance + choice validation`
- [x] LEDGER: tick F4 **unit** cell (matrix 1–10); CHECKPOINT

**Done when:** all 10 matrix cases pass at ≥95% coverage.

---

### P1-T6 — registry + install  (plan §4 P1-T6)

**Files:** `internal/registry/{registry.go,registry_test.go}`.

- [x] RED: fresh `Install` writes `sources.yaml` with `{namespace, url, commit}` **and** a snapshot at `<UserSnapshotDir>/<namespace>.yaml` byte-identical to the source file
- [x] RED: re-installing the same name refreshes commit + snapshot with **no duplicate** registry entry
- [x] RED: a DIFFERENT url installing an already-registered namespace ⇒ `ErrNamespaceTaken`, error text contains the **existing url**; `Snapshot(namespace)` returns path/`ok=false` (the `resolve.SourceLookup` impl)
- [x] RED: `Sources()` against a missing registry file ⇒ empty slice, nil error
- [x] RUN-RED: `go test ./internal/registry/` → expect **FAIL**
- [x] GREEN: implement `Registry.Install` / `Sources` — yaml encoding of `SourceRegistry` via the same protojson-normalize trick as `schema`; `os.MkdirAll`; atomic temp+rename
- [x] RUN-GREEN: `go test ./internal/registry/` → expect **PASS**
- [x] COMMIT: `feat(gff): source registry keyed by reverse-DNS namespace + snapshots`
- [x] LEDGER: tick F6 **unit** cell; CHECKPOINT

**Done when:** registry tests pass, including the area-claim rejection naming the owner.

---

### P1-T7 — read verbs: get / enabled / selected / list / lint  (plan §4 P1-T7)

**Files:** `cmd/{get.go,enabled.go,selected.go,list.go,lint.go}` + `cmd/read_test.go`;
modify `cmd/root.go` (test seam + `--source` persistent flag).

- [x] SETUP: add the test seam in `cmd/root.go`: `var newResolver = defaultResolver` (swapped in tests — the pattern gss uses for its runner)
- [x] SETUP: register the global persistent flag `--source <name|path>` on the root command (plan §3.4)
- [x] RED: `get k` prints `true\n`; a choice key prints `apt\n` (comma-joined ids when multi)
- [x] RED: `selected k apt` ⇒ exit 0 selected / 1 not selected / 2 unknown option id
- [x] RED: `enabled k` ⇒ 0 on / 1 off; unknown key ⇒ the `ErrUnknownKey` sentinel (exit mapping lives only in `main.go`, so tests assert the sentinel)
- [x] RED: `enabled` on a **choice** key ⇒ exit 2 + stderr message (per §3.4)
- [x] RED: `list` renders a table containing an `install.ai.claude  bool  true  default(user-snapshot)`-style row (PATH TYPE VALUE LAYER DESCRIPTION)
- [x] RED: `list --json` output unmarshals into `[]Resolved`
- [x] RED: `lint` on a bad file exits non-zero and lists the findings; on the discovered repo file by default
- [x] RED: `get --source <path>` resolves a second temp repo from an unrelated CWD
- [x] RED: `get --source <registered-name>` resolves via that source's snapshot
- [x] RED: unknown `--source` name and non-repo path ⇒ `ErrUnknownSource` sentinel
- [x] RUN-RED: `go test ./cmd/` → expect **FAIL**
- [x] GREEN: implement the five verb files (~30 lines each), each self-registering via `init()`; all build `resolve.Resolver{P: paths.Default(), R: gitx.ExecRunner{}}` through the `newResolver` hook
- [x] GREEN: exit-code mapping **only** in `main.go` — `errors.Is(err, resolve.ErrUnknownKey)` or `resolve.ErrUnknownSource` ⇒ exit 2; other non-nil ⇒ exit 1; set `SilenceUsage`
- [x] RUN-GREEN: `go test ./cmd/` → expect **PASS**
- [x] COMMIT: `feat(gff): get/enabled/list/lint verbs`
- [x] LEDGER: tick F4 / F11 **unit** cells; CHECKPOINT

**Done when:** `go test ./cmd/` passes including the exit-code sentinels and `--source` cases.

---

### P1-T8 — write verbs: set / unset  (plan §4 P1-T8)

**Files:** `cmd/{set.go,unset.go}` + `cmd/write_test.go`.

- [x] RED: `set k false` creates `<tmp cfg>/config.yaml` with mode **0600**, containing only that key
- [x] RED: `set` a choice with an unknown option id ⇒ error and the override file is **byte-identical** before/after
- [x] RED: `set` two ids on a `single`-mode choice ⇒ error, file untouched
- [x] RED: `set` an unknown key ⇒ `ErrUnknownKey`
- [x] RED: `unset k` removes that key and keeps the others
- [x] RED: round-trip — `set` then `get` agree
- [x] RED: assert **no** test writes outside `t.TempDir()` (no repo/system writes)
- [x] RUN-RED: `go test ./cmd/` → expect **FAIL**
- [x] GREEN: implement `set.go` / `unset.go` — read-modify-write the `LoadOverrides` map + yaml marshal, atomic temp+rename, `os.Chmod(0600)`, create parent dirs; **validate via the Resolver before writing**
- [x] RUN-GREEN: `go test ./cmd/` → expect **PASS**
- [x] COMMIT: `feat(gff): set/unset writing the user override only`
- [x] LEDGER: tick F3 / F8 **unit** cells; CHECKPOINT

**Done when:** write tests pass and nothing outside the user override is ever written.

---

### P1-T9 — export + install verbs  (plan §4 P1-T9)

**Files:** `cmd/{export.go,install.go}` + `cmd/export_test.go`, golden `cmd/testdata/export.golden`.

- [x] RED: env-mangling table — `install.windows.wispr-flow` → `GFF_INSTALL_WINDOWS_WISPR_FLOW` (uppercase, `.` and `-` → `_`)
- [x] RED: golden test — world with 3 flags (one overridden false, one choice) produces exactly the sorted lines `GFF_INSTALL_AI_CLAUDE=true` / `GFF_INSTALL_PKG_MANAGER=apt` / `GFF_INSTALL_WINDOWS_WISPR_FLOW=false`
- [x] RED: injection assert — a description containing `$(rm -rf)` never appears in export output; values are bool literals or lint-constrained kebab ids only
- [x] RED: `--format dotenv -o <tmp>/.env` writes the same lines as shell and round-trips through `hashicorp/go-envparse` (test-only dep)
- [x] RED: `--format json` unmarshals into `[]Resolved` and carries choice option ids + typed values
- [x] RED: `--format yaml` round-trips to the **same** `[]Resolved` as the json form (equality assert)
- [x] RED: `--shell` alias behaves identically to `--format shell`
- [x] RED: `install` inside a temp repo registers + snapshots (assert via `Sources()`); outside a repo ⇒ a clear error
- [x] RUN-RED: `go test ./cmd/` → expect **FAIL**
- [x] GREEN: implement `export.go` — `--format shell|dotenv|json|yaml`, `-o <file>` (dotenv defaults to `.env`), `--shell` alias; stdout carries only export lines
- [x] GREEN: implement `install.go` — name = repo dir basename; url = `git config --get remote.origin.url` via the gitx Runner (tolerate absence); commit = `rev-parse --short HEAD`; delegates to `internal/registry`
- [x] RUN-GREEN: `go test ./cmd/` → expect **PASS**
- [x] COMMIT: `feat(gff): shell export + repo install verbs`
- [x] LEDGER: tick F6 / F7 **unit** cells; CHECKPOINT

**Done when:** the export golden matches byte-for-byte and all four formats round-trip.

---

### P1-T10 — public SDK + CI + coverage gate  (plan §4 P1-T10)

**Files:** `pkg/gff/{gff.go,gff_test.go}`, `.github/workflows/gff-ci.yml`.

- [x] RED: `pkg/gff/gff_test.go` — `Bool` / `Selected` / `IsSelected` / `StringValues` agree with a `Resolver` over the same temp world; SDK takes a `WithPaths(p)` functional option (default `paths.Default()`)
- [x] RUN-RED: `go test ./pkg/gff/` → expect **FAIL**
- [x] GREEN: implement `pkg/gff/gff.go` — thin wrapper exposing `Bool`, `Selected`, `IsSelected`, `IntValues`, `FloatValues`, `StringValues`, `BoolValues` per §3.3; a wrong-type accessor errors naming the actual type
- [x] RUN-GREEN: `go test ./pkg/gff/` → expect **PASS**
- [x] GREEN: write `.github/workflows/gff-ci.yml` — PR path filter `sdk/gff/**`; setup-go from `.go-version`
- [x] GREEN: CI step — `go run . version` (zero-install entrypoint smoke, proves the module stays `go run`-able)
- [x] GREEN: CI step — `go vet ./... && go test ./... -coverprofile=cover.out`; **fail if** `go tool cover -func=cover.out | tail -1` < **90%**
- [x] GREEN: CI step — `sudo apt-get install -y protobuf-compiler` then `make gff-proto-check` (regeneration clean)
- [x] VERIFY: locally `go test ./... -coverprofile=cover.out && go tool cover -func=cover.out | tail -1` → **≥90%**; record in `TRACKING.md` §9
- [x] VERIFY: `bash build.sh` installs a working `${HOME}/opt/bin/gff version`
- [x] ALLOWLIST: `git check-ignore -v .github/workflows/gff-ci.yml` — expect `!.github/**` coverage
- [x] COMMIT: `feat(gff): public SDK + CI (vet, tests, coverage gate, proto-regen check)`
- [x] CHECKPOINT, then confirm the **gff-ci run is green** on the draft PR
- [x] LEDGER: tick F11 **unit** (CI smoke) cell

**Done when:** CI is green end-to-end and total coverage is ≥90%.

---

### P1-T11 — binary-level e2e harness (happy path + adversarial)  (plan §4 P1-T11, §7.2)

**Files:** `sdk/gff/e2e/e2e_test.go` (build tag `e2e`), `sdk/gff/scripts/e2e.sh`;
modify `.github/workflows/gff-ci.yml` (add the `e2e` job) and root `Makefile` (`gff-e2e`).

> **TDD note:** these subtests exercise the compiled binary, so some may pass on first
> run. Before ticking any box, confirm the assertion is real — flip the expectation to a
> deliberately wrong value, see it fail, then restore it. A green box must mean *proven*.

- [x] SETUP: `scripts/e2e.sh` — builds `gff` into a temp dir, then runs `go test -tags e2e ./e2e/`; shell-portability clean (`#!/usr/bin/env bash`, `set -euo pipefail`)
- [x] SETUP: root `Makefile` target `gff-e2e` → `bash sdk/gff/scripts/e2e.sh`
- [x] SETUP: `e2e/e2e_test.go` scaffold with build tag `e2e` — drives the binary via `os/exec` against a fake `$HOME` and temp git repos (real `git`, zero network)
- [x] RED: **IH-1** `gff lint` on an authored flag file (bools + one radio + one checkbox choice with typed values) ⇒ exit 0
- [x] RED: **IH-2** `gff install` in repo A ⇒ `sources.yaml` + snapshot written; `gff list` works from `$HOME`
- [x] RED: **IH-3** `get`/`enabled` on a default-true bool from a foreign CWD ⇒ `true` / exit 0
- [x] RED: **IH-4** `selected` on the default choice option ⇒ exit 0; `get` prints the id(s)
- [x] RED: **IH-5** `set` bool `false` ⇒ ONLY the user override file changes (0600); `list --json` shows `layer=user-override`
- [x] RED: **IH-6** `set` choice — single: one id; multi: two ids — round-trips through `get`
- [x] RED: **IH-7** `export --format shell` evals cleanly in bash AND dash; `gff_on` then skips the false key and runs the true key
- [x] RED: **IH-8** `export --format dotenv -o .env` parses with go-envparse; `json` and `yaml` unmarshal to identical `[]Resolved` incl. typed payloads
- [x] RED: **IH-9** `unset` ⇒ default restored; winning layer reverts to snapshot/repo
- [x] RED: **IH-10** zero-install + cross-repo: `go run . <verb>` and `--source <name>` / `--source <path>` from a foreign CWD
- [x] RED: **IA-1** unknown key ⇒ exit 2 on `get`/`enabled`/`set`; unknown option id ⇒ exit 2 on `selected`
- [x] RED: **IA-2** `set` two ids on a `single`-mode choice ⇒ exit 1; override file byte-identical before/after
- [x] RED: **IA-3** malformed flag file (truncated mid-list, bad indent) ⇒ `lint` and every read verb fail naming file+line; never a panic/stacktrace
- [x] RED: **IA-4** malformed override yaml ⇒ read verbs error cleanly (never silently skipped); other layers unaffected afterward
- [x] RED: **IA-5** injection: description containing `$(rm -rf /tmp/pwned)` never reaches export output; option id `evil;rm` rejected by lint; exported bytes assert against a `[A-Z0-9_=,.\n-]`-only set
- [x] RED: **IA-6** different url installing an already-registered namespace ⇒ `ErrNamespaceTaken` naming the existing url; registry unchanged; same short keys resolve in both namespaces when qualified
- [x] RED: **IA-7** corrupt `sources.yaml` ⇒ verbs degrade with a clear error — and the shell gate stays fail-open (a broken gff still runs every step)
- [x] RED: **IA-8** read-only `~/.config` ⇒ `set` exits 1, no temp-file litter
- [x] RED: **IA-9** `HOME` unset ⇒ clear error; nothing written to CWD
- [x] RED: **IA-10** `--source` with an unknown name and with a non-repo path ⇒ exit 2
- [x] RED: **IA-11** 10 concurrent `set` calls, distinct keys ⇒ final file is valid YAML and exactly ONE writer's complete snapshot (never merged/interleaved; lost-update accepted — no locking)
- [x] RED: **IA-12** `gff.source` redirect pointing at a missing file / outside the repo ⇒ clean error; no path-traversal surprises
- [x] RED: **IA-13** after `gff install`, `git status --porcelain` in the source repo is empty
- [x] RED: **IA-14** registered repo moved on disk ⇒ snapshot still resolves from any CWD
- [x] RED: **IA-15** empty feature set ⇒ all four export formats emit valid empty output, exit 0
- [x] RUN-RED: `make gff-e2e` → expect **FAIL** on the not-yet-satisfied scenarios
- [x] GREEN: fix the binary-side gaps the harness exposes (clean exit codes, messages naming the offender, zero partial writes) — without touching plan §3 contracts
- [x] RUN-GREEN: `make gff-e2e` → expect **PASS**, all 25 subtests
- [x] GREEN: add the `e2e` job to `.github/workflows/gff-ci.yml`, running **after** the unit job
- [x] COMMIT: `test(gff): binary-level e2e harness — happy path + adversarial suite`
- [x] LEDGER: tick every IH-*/IA-* box in `TRACKING.md` §7 and the **integration** column of §6; CHECKPOINT

**Done when — P1 done-when gate:** P1-T10 all green **plus** the e2e harness green.

---

## Leaf `p1-engine` closeout

- [x] VERIFY the §4 gate: gff-ci green (vet + tests, ≥90% cover, proto regen clean, e2e job green) and `bash build.sh` installs a working binary
- [x] Token call 1: `mkdir -p ~/.config/gss && git rev-parse HEAD > ~/.config/gss/approval.token`
- [x] Token call 2 (separate Bash call): `gss feature pr --ready --worker <p1 worker_ref>` (promotion + merge happened via the user's review of PR #182; state MERGED confirmed via `gh pr view`)
- [x] After merge: `gss feature merged <p1 worker_ref>` (token-gated; note: the ref is **positional** — `merged` has no `--worker` flag)
- [x] LEDGER: `p1-engine` state → `merged`; session-log line; update `docs/mbo/index.md`

---

# Leaf 2a — `p2-instrument`  (starts after `p1-engine` merges)

## Leaf setup

- [x] `gss feature worker add --feature gff --purpose p2-instrument --engine claude --json --description "P2: dotfiles flag inventory + gff_on shell gate + install.sh/PowerShell gating (#180)"`
- [x] Record `worker_ref` / `branch` / `worktree_path` verbatim in `TRACKING.md` §0
- [x] Confirm `${HOME}/opt/bin/gff` is the P1 binary (`gff version`) — P2 verification uses it (rebuilt from main 6f1003f, Dirty:false)

---

### P2-T1 — dotfiles flag inventory (`.github/gff/features.yaml`)  (plan §4 P2-T1)

- [x] ALLOWLIST **FIRST**: `git check-ignore -v .github/gff/features.yaml` must show it **NOT** ignored (`!.github/**` already opts it in — expect no new `.gitignore` rules; if a deeper deny rule surprises you, add a narrow `!` rule with a comment)
- [x] GREEN: create `.github/gff/features.yaml` — ONE `sets:` entry, `area: install`, every feature `boolDefault: true`, description = one plain sentence naming the `install.sh` block it gates
- [x] GREEN: `install.system.*` (4) — `wsl-interop`, `jetson`, `gitrepos`, `nano-profile`
- [x] GREEN: `install.shell.*` (2) — `profiles`, `default-zsh`
- [x] GREEN: `install.pkg.*` (2) — `common-core`, `brewfile`
- [x] GREEN: `install.tools.*` (6) — `sops`, `yq`, `k8s`, `snowflake`, `docker`, `git-aliases`
- [x] GREEN: `install.runtime.*` (5) — `goenv`, `pyenv`, `rbenv`, `nvm`, `fnm`
- [x] GREEN: `install.ai.*` (6) — `skills`, `antigravity`, `claude`, `google-cli`, `plugins`, `teams`
- [x] GREEN: `install.sdk.*` (5) — `gss`, `tmux-mgr`, `wol`, `gsl`, `gff`
- [x] GREEN: `install.fonts.nerd-font` (1) and `install.network.sshd` (1)
- [x] GREEN: `install.windows.*` (11) — `desktop-deploy`, `wsl-platform`, `nerd-font`, `terminal-themes`, `apps`, `wispr-flow`, `copilot-key`, `ahk-autostart`, `claude-rc-autostart`, `sshd`, `portproxy`
- [x] VERIFY: the file declares exactly **43** flags (4+2+2+6+5+6+5+1+1+11)
- [x] VERIFY: `git status --short -- .github/gff/` shows the file as trackable
- [x] VERIFY: `${HOME}/opt/bin/gff lint .github/gff/features.yaml` → exit 0, no findings
- [x] VERIFY: from the worktree root, `gff list` shows all 43 keys with `repo-live` layer. NOTE (P1 lesson): other registered namespaces (e.g. test repos in `~/.config/gff/sources.yaml`) also appear in `list` — count OUR rows precisely with `gff list --json | jq '[.[] | select(.namespace=="com.github.sfc-gh-eraigosa.dotfiles")] | length'` = 43; the new `gff list 'install.*'` filter and `--pretty`/`--raw` flags are available. Every feature MUST carry `boolDefault: true` — P1 added a `missing-default` lint rule that fails default-less features
- [x] COMMIT: `feat(gff): enumerate dotfiles install components as flags (all on)`
- [x] LEDGER + CHECKPOINT

**Done when:** `gff lint` is clean and `gff list` shows 43 repo-live flags.

---

### P2-T2 — shell helper `opt/lib/gff.sh`  (plan §4 P2-T2)

- [x] RED: write `opt/lib/gff_test.sh` **first**, mirroring `ai/hooks/safety_guard_test.sh`'s assert style. NOTE (P1 lesson — the snowflake bug): commit executable scripts WITH the exec bit — after staging, verify `git ls-files -s opt/lib/gff_test.sh` shows `100755` (`gff.sh` itself is SOURCED, stays 100644 with the `# shellcheck shell=bash` header)
- [x] RED: case — var unset ⇒ `gff_on` returns 0 (fail-open)
- [x] RED: case — `=true` ⇒ 0; `=false` ⇒ 1
- [x] RED: case — `=FALSE` ⇒ 0, `=0` ⇒ 0, garbage ⇒ 0 (only exact lowercase `false` disables)
- [x] RED: case — key mangling: `install.windows.wispr-flow` reads `GFF_INSTALL_WINDOWS_WISPR_FLOW`
- [x] RED: case — `gff_skip_msg <key>` echoes `SKIP (gff: <key>=false)`
- [x] RUN-RED: `bash opt/lib/gff_test.sh` → expect **FAIL** (`opt/lib/gff.sh` missing)
- [x] GREEN: implement `opt/lib/gff.sh` exactly as the plan's snippet — POSIX only (dash-safe, no `[[`, no arrays), `# shellcheck shell=bash` header, `gff_on()` + `gff_skip_msg()`
- [x] RUN-GREEN: `bash opt/lib/gff_test.sh` → expect **PASS** (all cases)
- [x] RUN-GREEN: `sh opt/lib/gff_test.sh` (dash) → expect **PASS** (all cases)
- [x] VERIFY: `make lint-shell && make lint-portability` → clean
- [x] COMMIT: `feat(gff): fail-open gff_on shell gate helper + test driver`
- [x] LEDGER: tick F9 **unit** cell and `TRACKING.md` §7.3 shell negatives; CHECKPOINT

**Done when:** the driver passes under both bash and dash and both lint gates are clean.

---

### P2-T3 — instrument `install.sh` (Linux/common)  (plan §4 P2-T3)

- [x] SETUP: locate the goenv/Go block in `install.sh` (~line 309) and the pyenv block that follows it
- [x] GREEN: insert the gff bootstrap block **verbatim from the plan** immediately AFTER the goenv/Go block and BEFORE pyenv (conditional `build.sh`, `eval "$(… export --shell)"`, then `. "${BASE_DIR}/opt/lib/gff.sh"`) — fail-open: a failed build warns and runs everything. **NOTE (P1 lesson, verified in sandbox): wrap the eval in `set -a` / `set +a`** — export lines are plain `VAR=v` (shell-local); without allexport the `GFF_*` vars never reach child scripts, and P2-T4's WSLENV builder greps `env` which only sees EXPORTED vars
- [x] GREEN: add ONE comment line at the top documenting that flags for pre-bootstrap steps take effect via `gff export` in the calling shell or on the next run
- [x] GREEN: wrap `install.system.*` blocks in-place with `if gff_on <key>; then … else gff_skip_msg <key>; fi` — NO reordering, NO logic changes inside
- [x] GREEN: wrap `install.shell.*` blocks (profiles, default-zsh)
- [x] GREEN: wrap `install.pkg.*` blocks (common-core, brewfile)
- [x] GREEN: wrap `install.tools.*` blocks (sops — the exemplar pattern in the plan — yq, k8s, snowflake, docker, git-aliases)
- [x] GREEN: wrap `install.runtime.*` blocks (goenv, pyenv, rbenv, nvm, fnm)
- [x] GREEN: wrap `install.ai.*` blocks (skills, antigravity, claude, google-cli, plugins, teams)
- [x] GREEN: wrap `install.sdk.*` blocks — **note:** `install.sdk.gff` gates only the LATER duplicate build guard; the bootstrap build itself is never gated
- [x] GREEN: wrap `install.fonts.nerd-font`, `install.network.sshd`, and the `install.windows.desktop-deploy` invocation site
- [x] VERIFY: `bash -n install.sh` → clean
- [x] VERIFY: `make lint-shell && make lint-portability` → clean
- [x] VERIFY manual: `GFF_INSTALL_TOOLS_SOPS=false bash -c '. opt/lib/gff.sh; gff_on install.tools.sops || gff_skip_msg install.tools.sops'` prints the SKIP line
- [x] COMMIT: `feat(install): gate every install.sh component behind gff flags (fail-open)`
- [x] LEDGER + CHECKPOINT

**Done when:** every component block is gated, `bash -n` is clean, and both lint gates pass.

---

### P2-T4 — Windows pass-through + PowerShell gating  (plan §4 P2-T4)

- [x] GREEN: `opt/bin/install_windows.sh` — top-level `gff_on install.windows.desktop-deploy || { gff_skip_msg install.windows.desktop-deploy; exit 0; }`
- [x] GREEN: `opt/bin/install_windows.sh` — before each `powershell.exe` invocation, insert the WSLENV builder loop verbatim from the plan (appends each `GFF_INSTALL_WINDOWS_*` name with the `/u` flag, de-duplicated). NOTE: this loop reads `env`, so it REQUIRES P2-T3's `set -a` bootstrap wrapping — verify with `env | grep -c GFF_INSTALL_WINDOWS_` inside a child shell
- [x] GREEN: create `opt/Desktop/Apps/scripts/lib/gff.ps1` with `Test-GffOn([string]$Key)` exactly as the plan's snippet (unset ⇒ on; only the literal string `false` disables)
- [x] GREEN: gate `setup-apps.ps1` phases — WSL platform (`install.windows.wsl-platform`), Nerd Font, Terminal themes, winget apps; each disabled phase prints `SKIP (gff: <key>=false)`
- [x] GREEN: gate `setup-elevated.ps1` items — Wispr Flow MSI (`install.windows.wispr-flow`), PowerToys Copilot remap (`install.windows.copilot-key`), AHK autostart task (`install.windows.ahk-autostart`)
- [x] GREEN: gate the standalone scripts' invocation sites — `install.windows.claude-rc-autostart`, `install.windows.sshd`, `install.windows.portproxy`
- [x] VERIFY: `make lint-shell && make lint-portability` → clean (the bash file)
- [x] VERIFY: `pwsh -NoProfile -Command ". opt/Desktop/Apps/scripts/lib/gff.ps1; Test-GffOn 'install.windows.wispr-flow'"` → `True`; with `$env:GFF_INSTALL_WINDOWS_WISPR_FLOW='false'` → `False`
- [x] If `pwsh` is unavailable in WSL: record in `TRACKING.md` that this check **defers to the P2-T5 human run** (do not tick it as passed)
- [x] COMMIT: `feat(install): gff gating for Windows setup phases via WSLENV pass-through`
- [x] LEDGER + CHECKPOINT

**Done when:** the WSLENV handoff is in place, every PS phase is gated, and lint is clean.

---

### P2-T5 — human-evidenced acceptance (spec §6)  (plan §4 P2-T5)

> **Human-in-the-loop.** Requires a real interactive WSL terminal. Never run `install.sh`
> from a worker worktree — it creates absolute symlinks in `${HOME}`.

- [x] SETUP: switch to `${HOME}/git/dotfiles`, checkout the branch carrying P2, and run from there. NOTE: rebuild first (`make gff-install`) so `${HOME}/opt/bin/gff` is the merged binary; leftover test registrations in `~/.config/gff/sources.yaml` (e.g. `com.example.demo`) are harmless — the focus-namespace rule binds unqualified keys to the dotfiles repo when run from inside it. (Ran on `fix/gff-wslenv-w-flag` @ 0435b9d — the WSLENV `/w` fix, PR #191, was required for the flag to cross)
- [x] RUN: `gff set` the flag false; confirmed via `gff get` + `env`. KEY SWITCHED wispr-flow → **install.windows.apps**: the elevated batch cannot receive env across UAC (TRACKING §10), so the evidence uses a non-elevated gate on the identical chain
- [x] RUN: **`set -a; eval "$(gff export --shell)"; set +a`** in the calling shell — bare eval is NOT enough, export lines are plain `VAR=v` (TRACKING §10)
- [x] RUN: `install.sh` in a real interactive terminal, prompt answered `y` (owner, 2026-07-26)
- [x] CAPTURE: `SKIP (gff: install.windows.apps=false)` replaced the whole `[5/6] Desktop apps` phase — evidence/F09-gating/P2-T5-real-install.txt
- [x] RUN: `gff unset install.windows.apps` (+ the earlier wispr-flow override); defaults restored
- [x] POST: evidence committed to the tree + posted on issue #180 (closeout)
- [x] LEDGER: F9 **demo** cell + P2 done-when gate ticked (TRACKING §2/§6/§8)

**Done when — P2 done-when gate:** lint gates clean, `gff lint` clean, and this evidence posted.

---

## Leaf `p2-instrument` closeout

- [x] Token call 1: `mkdir -p ~/.config/gss && git rev-parse HEAD > ~/.config/gss/approval.token`
- [x] Token call 2: promotion done by owner on GitHub (gss `pr --ready` blocked by the safety_guard/gss token double-bind — TRACKING §11)
- [x] After merge: `gss feature merged <p2 worker_ref>` (positional ref)
- [x] LEDGER: `p2-instrument` → `merged`; session-log line; update `docs/mbo/index.md`

---

# Leaf 2b — `p3-tui`  (parallel with p2 / p4, after `p1-engine` merges)

## Leaf setup

- [x] `gss feature worker add --feature gff --purpose p3-tui --engine claude --json --description "P3: bubbletea TUI — browse, provenance, toggle (#180)"`
- [x] Record `worker_ref` / `branch` / `worktree_path` verbatim in `TRACKING.md` §0

---

### P3-T1 — TUI  (plan §4 P3-T1)

**Files:** `internal/tui/{model.go,view.go,tui_test.go}`, `cmd/tui.go`; modify `cmd/root.go`.

- [x] SETUP: `go get github.com/charmbracelet/bubbletea github.com/charmbracelet/lipgloss github.com/charmbracelet/x/exp/teatest@latest`
- [x] ~~REFACTOR: extract `internal/overrides.Write`~~ ALREADY DONE in P1-T8 (`internal/overrides` with `Write`/`Unset`/`WriteFileAtomic`; `set.go`/`unset.go` are thin callers) — just consume it. Also: `lipgloss` + `x/term` are already deps (P1 styled `list`); only bubbletea/teatest need `go get`
- [x] VERIFY: `go test ./cmd/` still **PASS** after the refactor (no behavior change)
- [x] RED: teatest — the initial frame lists areas **collapsed**
- [x] RED: teatest — navigating (`enter` on area → components → features) shows a feature row containing description + `default`/`override` + the winning layer
- [x] RED: teatest — `space` on a bool writes ONLY the user override file (temp-paths world) and the row flips
- [x] RED: teatest — `space` on a choice opens the option picker: **radio** list for `single` mode, **checkbox** list for `multi`, built from `ChoiceDefault.Options`, showing id + description + typed value
- [x] RED: teatest — `q` after no toggles writes **nothing** (override file mtime unchanged)
- [x] RUN-RED: `go test ./internal/tui/` → expect **FAIL**
- [x] GREEN: implement `model.go` — `Model{items []resolve.Resolved, cursor, expanded map[string]bool, w io.Writer}`, reusing `cmd`'s resolver hook; all writes go through `internal/overrides.Write` (same path as `gff set`)
- [x] GREEN: implement `view.go` rendering the tree + provenance column
- [x] GREEN: `cmd/tui.go` registering the `tui` verb; modify `cmd/root.go` so bare `gff` with no args **and a TTY** runs the TUI, else prints help
- [x] RUN-GREEN: `go test ./internal/tui/` → expect **PASS**
- [x] VERIFY: `go test ./... -cover` → overall still **≥90%**; record in `TRACKING.md` §9
- [x] COMMIT: `feat(gff): bubbletea TUI — browse, provenance, toggle`
- [x] LEDGER: tick F10 **unit** + **integration** cells; CHECKPOINT

**Done when — P3 done-when gate:** teatest suite green and overall coverage ≥90%.

---

## Leaf `p3-tui` closeout

- [x] SUPERSEDED: p3 landed inside the combined `p34-tui-gen` PR #187 (owner-directed restructure; #186 closed) — merged 2f39b66
- [x] LEDGER: state recorded in TRACKING §0/§3; `docs/mbo/index.md` updated

---

# Leaf 2c — `p4-gen`  (parallel with p2 / p3, after `p1-engine` merges)

## Leaf setup

- [x] `gss feature worker add --feature gff --purpose p4-gen --engine claude --json --description "P4: gff gen typed-accessor codegen (#180)"`
- [x] Record `worker_ref` / `branch` / `worktree_path` verbatim in `TRACKING.md` §0

---

### P4-T1 — `gff gen` typed accessors  (plan §4 P4-T1)

**Files:** `cmd/gen.go`, `cmd/gen_test.go`, `cmd/testdata/gen.golden`.

- [x] RED: golden test — against the P1-T9 world, `gff gen --pkg gffgen --out <tmp>` writes `<tmp>/gffgen.go` matching `cmd/testdata/gen.golden`
- [x] RED: assert the emitted shape — per flag a var chain `var Install = struct{ Ai struct{ Claude BoolFlag } … }` with `func (f BoolFlag) Bool() (bool, error)` delegating to `pkg/gff` by **literal key string**
- [x] RED: assert naming — segments Title-cased, dashes camel-cased (`wispr-flow` → `WisprFlow`)
- [x] RED: assert the golden **compiles** — the test runs `go vet` on a scratch module embedding the output
- [x] RUN-RED: `go test ./cmd/` → expect **FAIL**
- [x] GREEN: implement `cmd/gen.go` using `text/template` + `go/format.Source`; self-register via `init()` (no shared-file edits)
- [x] RUN-GREEN: `go test ./cmd/` → expect **PASS**
- [x] VERIFY: `go test ./... -cover` → overall still **≥90%**
- [x] COMMIT: `feat(gff): gen — typed accessor codegen`
- [x] LEDGER + CHECKPOINT

**Done when — P4 done-when gate:** golden test green and the generated output vets.

---

## Leaf `p4-gen` closeout

- [x] SUPERSEDED: p4 landed inside the combined `p34-tui-gen` PR #187 (owner-directed restructure; #185 closed) — merged 2f39b66
- [x] LEDGER: state recorded in TRACKING §0/§4; `docs/mbo/index.md` updated

---

# Leaf 3 — `vd-demo`  (after `p1-engine` + `p2-instrument` merge)

## Leaf setup

- [x] `gss feature worker add --feature gff --purpose vd-demo --engine claude --json --description "VD-1: narrated end-to-end demo script + recorded evidence (#180)"`
- [x] Record `worker_ref` / `branch` / `worktree_path` verbatim in `TRACKING.md` §0

---

### VD-1 — scripted end-to-end demo  (plan §4 VD-1, script per §7.3)

**Files:** `sdk/gff/scripts/demo.sh` (shell-portability-lint clean).

- [x] SETUP: create `sdk/gff/scripts/demo.sh` — narrated, re-runnable, running against a scratch `$HOME` (`GFF_DEMO_HOME` temp dir) so it never touches real config; each step echoes what it is about to prove
- [x] GREEN: step 1 — scaffold a demo repo; author a flag file with 1 bool + 1 radio choice + 1 checkbox choice (typed values shown)
- [x] GREEN: step 2 — `lint` → `install` → `list`, calling out the winning-layer/provenance column
- [x] GREEN: step 3 — gate a toy script with `gff_on`; `set` the bool off; rerun shows the SKIP line; flip it back
- [x] GREEN: step 4 — `export` all four formats; eval the shell form in **dash**; parse the `.env`
- [x] GREEN: step 5 — a second repo claims the same area ⇒ show the rejection message (the guardrail moment)
- [x] GREEN: step 6 — finale from an empty directory with no gff on PATH: `eval "$(go run <module>@<tag> export --format shell --source demo)"`
- [x] VERIFY: `make lint-shell && make lint-portability` → clean
- [x] VERIFY: `bash sdk/gff/scripts/demo.sh` runs clean **twice in a row** (re-runnable) and never touches `${HOME}/.config/gff`
- [x] RUN: demo executed (linux/arm64; 3 consecutive clean runs) and full transcript captured — a WSL re-run remains optional
- [x] POST: transcript posted in the #188 PR body + committed under evidence/demo/
- [x] COMMIT: `docs(gff): end-to-end demo script + recorded evidence` (46bf3be)
- [x] LEDGER: demo cells ticked (F9-demo remains with P2-T5); CHECKPOINT (#188)

**Done when — VD-1 done-when gate:** transcript posted on the PR; P2-T5's human-evidenced
wispr-flow SKIP run stands alongside it as the real-install proof.

---

### VD-1 addendum — post-P3 TUI segment  (do after `p3-tui` merges)

- [x] RUN: satisfied by the live tmux frame captures merged with #187 (browse → toggle → provenance → picker); a video/gif stays at owner discretion
- [x] POST: linked from the #187 PR body (F10-tui evidence files)
- [x] LEDGER: F10 **demo** cell ticked; CHECKPOINT

---

## Leaf `vd-demo` closeout

- [ ] Token call 1: `mkdir -p ~/.config/gss && git rev-parse HEAD > ~/.config/gss/approval.token`
- [ ] Token call 2 (separate Bash call): `gss feature pr --ready --worker <vd worker_ref>`
- [ ] After merge: `gss feature merged --worker <vd worker_ref>`
- [ ] LEDGER: `vd-demo` → `merged`; session-log line

---

# Objective closeout (plan §7.5 — the stop condition)

- [x] VERIFY `gff-ci.yml` fully green on `main`: green on the #182/#184/#187 merges; post-tag zero-install additionally proven (F11-gorun/post-tag-zero-install.txt)
- [x] VERIFY the VD-1 demo transcript is posted on the PR (#188 body + evidence/demo/)
- [x] VERIFY the P2-T5 real-install evidence is posted (evidence/F09-gating/P2-T5-real-install.txt + #180 comment; key = install.windows.apps per TRACKING §10)
- [x] VERIFY every §7.4 feature→proof row is checked in `TRACKING.md` §6 **and** reproduced in the leaf PR descriptions (F9-demo was the last cell — closed 2026-07-26)
- [x] Update `docs/mbo/index.md` — `gff` state → `done`, all leaf PRs linked
- [x] Close issue #180 — closed 2026-07-26 with the evidence comment (all four leaves + #191 landed)
- [x] Final session-log entry in `TRACKING.md` §11
