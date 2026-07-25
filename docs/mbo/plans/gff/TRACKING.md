# gff — live state ledger

- **Slug:** gff
- **Started:** 2026-07-25
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Cursor:** [`TODO.md`](./TODO.md)
- **Plan (source of truth):** [`../gff.md`](../gff.md) · spec [`../../specs/gff.md`](../../specs/gff.md)
- **Objective anchors:** issue #180 · design PR #181 · `docs/mbo/index.md` row `gff`

> **Update this file after EVERY task** (IMPLEMENTATION.md §3 step 6). Status values:
> `todo` · `in-progress` · `blocked` · `done`. **Evidence** = the exact command run plus
> its real result (e.g. `go test ./internal/resolve/ -cover -> ok, 96.4%`). A row is
> `done` only with a commit SHA **and** evidence. Never write a result you did not observe.

---

## 0. Leaf / worker registry

Fill in from the `gss feature worker add --json` output — **verbatim**, never reconstructed.

| Leaf | Worker ref | Branch | Worktree path | PR | State |
| :-- | :-- | :-- | :-- | :-- | :-- |
| `p1-engine` | `gff/edward-raigosa/p1-engine` | `feature/gff/edward-raigosa/p1-engine` | `${HOME}/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/gff/edward-raigosa/p1-engine` | [#182](https://github.com/sfc-gh-eraigosa/dotfiles/pull/182) | building |
| `p2-instrument` | _(pending)_ | _(pending)_ | _(pending)_ | _(pending)_ | todo |
| `p3-tui` | _(pending)_ | _(pending)_ | _(pending)_ | _(pending)_ | todo |
| `p4-gen` | _(pending)_ | _(pending)_ | _(pending)_ | _(pending)_ | todo |
| `vd-demo` | _(pending)_ | _(pending)_ | _(pending)_ | _(pending)_ | todo |

Leaf state vocabulary (mirrors `docs/mbo/index.md`): `todo → building → in-review → merged`.

---

## 1. Leaf `p1-engine` — the blocking base

**Owns:** `sdk/gff/**` (excl. `internal/tui`, `cmd/gen.go`), `.github/workflows/gff-ci.yml`
**Done-when (plan §6.1):** gff-ci green — vet + tests, ≥90% cover, proto regen clean,
e2e harness green; `build.sh` installs a working binary.

| Task | Status | Commit | Evidence (test run / gate) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| P1-T1 module scaffold + version | done | b3ed141 | `go test ./... && go vet ./...` -> ok (cmd 0.002s); `bash build.sh` -> installed; `${HOME}/opt/bin/gff version` prints `gff v0.1.0` block | RED verified: `undefined: NewRootCmd` |
| P1-T2 proto schema + committed codegen | done | 678a69e | `make gff-proto` -> gen/gff/v1/features.pb.go (package gffv1); `go build ./... && go vet ./...` ok; `make gff-proto-check` -> clean; `make lint-shell` rc=0. `make lint-portability` fails on a PRE-EXISTING `opt/bin/docker:44 mapfile` Tier-2 finding already on origin/main — zero findings for gff files (build.sh, genproto.sh) | genproto.sh needed `mkdir -p gen` (protoc doesn't create --go_out root); noted as procedure fix |
| P1-T3 schema load + lint | done | 83cc7d7 | `go test ./internal/schema/ -cover` -> ok, 90.5% (bar ≥90); `go vet ./...` clean; RED verified: `no non-test Go files in internal/schema` | evidence: F01/F02/F03 P1-T3-schema-lint-cover.txt; namespace-vs-origin WARN deferred to CLI verb (frozen Lint(f) has no origin param) |
| P1-T4 paths + git discovery | done | fa24cd1 | `go test ./internal/paths/ ./internal/gitx/` -> ok (80.0% / 72.4%); `go vet ./...` clean; RED verified: `no non-test Go files` in both pkgs | evidence: F05-discovery/P1-T4-gitx-paths.txt; no per-pkg bar here — watch overall ≥90% at P1-T10 |
| P1-T5 resolver (the core) | done | a20c15b | `go test ./internal/resolve/ -cover` -> ok, 95.9% (bar ≥95); `go vet ./...` clean; RED verified: `no non-test Go files in internal/resolve`; 31 subtests incl. matrix 1–10 | evidence: F04-layers/P1-T5-resolve-matrix-cover.txt; Resolved carries an unexported namespace field for JSON() (frozen exported surface unchanged); unqualified multi-namespace key -> error wrapping ErrUnknownKey naming candidates |
| P1-T6 registry + install | done | 20b3e7f | `go test ./internal/registry/ -cover` -> ok, 78.4%; `go vet ./...` clean; RED verified: `no non-test Go files in internal/registry`; snapshot byte-identical + ErrNamespaceTaken names existing url | evidence: F06-registry/P1-T6-registry-cover.txt |
| P1-T7 read verbs — get/enabled/selected/list/lint | done | e1051b2 | `go test ./cmd/ -cover` -> ok, 74.6% (24 tests); full `go test ./...` green; `go vet ./...` clean; RED verified: `undefined: errOff` | evidence: F11-gorun/P1-T7-read-verbs.txt; exit mapping only in main.go (silent exit-1 via cmd.IsExit1Silent); F4/F11 unit cells partially proven here, cmd paths finish via e2e |
| P1-T8 write verbs — set/unset | done | _(next commit)_ | `go test ./cmd/ -cover` -> ok, 78.2% (38 tests); `go test ./...` green; `go vet ./...` clean; RED verified: `unknown command \"set\"` | evidence: F08-write-path/P1-T8-write.txt; internal/overrides.Write/Unset extracted (P3 seam) + WriteFileAtomic shared with registry; adversary fix: single-mode arity error de-wrapped from ErrUnknownOption so IA-2 gets exit 1 not 2; testify added (test-only dep) |
| P1-T9 export + install verbs | todo | | | |
| P1-T10 public SDK + CI + coverage gate | todo | | | ≥90% overall |
| P1-T11 binary-level e2e harness | todo | | | IH-* + IA-* |
| **P1 done-when gate** | todo | — | | P1-T10 green **plus** e2e green |

---

## 2. Leaf `p2-instrument`

**Owns:** `.github/gff/**`, `opt/lib/gff.sh*`, `install.sh`, `opt/bin/install_windows.sh`,
`opt/Desktop/Apps/scripts/{lib/gff.ps1,setup-apps.ps1,setup-elevated.ps1}`
**Consumes:** `p1-engine` (plan §3.4 CLI, §3.5 shell contract)
**Done-when (plan §6.1):** lint-shell + lint-portability clean; `gff lint` clean;
P2-T5 human evidence posted.

| Task | Status | Commit | Evidence (test run / gate) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| P2-T1 dotfiles flag inventory (43 flags) | todo | | | allowlist check first |
| P2-T2 shell helper `opt/lib/gff.sh` | todo | | | bash **and** dash |
| P2-T3 instrument `install.sh` (Linux/common) | todo | | | no reordering |
| P2-T4 Windows pass-through + PS gating | todo | | | pwsh check or defer to P2-T5 |
| P2-T5 human-evidenced acceptance | todo | — | | real terminal, WSL |
| **P2 done-when gate** | todo | — | | evidence posted on PR |

---

## 3. Leaf `p3-tui`

**Owns:** `sdk/gff/internal/tui/**`, `sdk/gff/cmd/tui.go`
**Consumes:** `p1-engine` (plan §3.3 Resolver, overrides writer)
**Done-when (plan §6.1):** teatest suite green; overall coverage ≥90%.

| Task | Status | Commit | Evidence (test run / gate) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| P3-T1 TUI (model, view, teatest, `cmd/tui.go`) | todo | | | extract `internal/overrides.Write` |
| **P3 done-when gate** | todo | — | | `go test ./... -cover` ≥90% |

---

## 4. Leaf `p4-gen`

**Owns:** `sdk/gff/cmd/gen.go`, `cmd/gen_test.go`, `cmd/testdata/gen.golden`
**Consumes:** `p1-engine` (plan §3.3 `pkg/gff`)
**Done-when (plan §6.1):** golden test green; generated output vets.

| Task | Status | Commit | Evidence (test run / gate) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| P4-T1 `gff gen` typed accessors | todo | | | golden compiles + vets |
| **P4 done-when gate** | todo | — | | |

---

## 5. Leaf `vd-demo`

**Owns:** `sdk/gff/scripts/demo.sh`
**Consumes:** `p1-engine` + `p2-instrument` (TUI addendum additionally `p3-tui`)
**Done-when (plan §6.1):** demo transcript + evidence posted on the PR (plan §7.3).

| Task | Status | Commit | Evidence (test run / gate) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| VD-1 demo script (`scripts/demo.sh`) | todo | | | scratch `$HOME`, re-runnable |
| VD-1 run on WSL + transcript posted | todo | — | | |
| VD-1 post-P3 TUI segment (~30s) captured | todo | — | | after `p3-tui` merges |
| **VD-1 done-when gate** | todo | — | | + P2-T5 real-install proof |

---

## 6. Feature → proof matrix (plan §7.4)

A feature is **built** only when all three of its proofs are green. Tick each cell as its
proof passes; record which task proved it in Notes.

| Feature | Unit | Integration | Demo | Notes |
| :-- | :-- | :-- | :-- | :-- |
| **F1** keys + lint | [x] `lint_test.go` table | [ ] IH-1, IA-3 | [ ] demo step 2 | |
| **F2** bool semantics | [x] schema/resolve tests | [ ] IH-3, IH-5 | [ ] demo steps 2–3 | |
| **F3** choice (modes, ids, typed values) | [x] lint/resolve/write tests | [ ] IH-4, IH-6, IA-1, IA-2 | [ ] demo steps 1, 3 | |
| **F4** layered resolution + provenance | [x] resolve matrix 1–10 | [ ] IH-5, IH-9 | [ ] demo step 2 | |
| **F5** discovery + `gff.source` redirect | [x] `gitx_test.go` | [ ] IH-3, IA-12 | [ ] demo step 2 | |
| **F6** registry + namespace identity | [x] `registry_test.go` | [ ] IH-2, IA-6, IA-7, IA-13, IA-14 | [ ] demo step 5 | |
| **F7** export formats + injection safety | [ ] export golden | [ ] IH-7, IH-8, IA-5, IA-15 | [ ] demo steps 4, 6 | |
| **F8** write path (0600, user-only) | [x] `write_test.go` | [ ] IH-5, IA-8, IA-11, IA-13 | [ ] demo step 3 | |
| **F9** fail-open gating | [ ] `gff_test.sh` bash + dash (binary-absent is unit-only) | [ ] IH-7, IA-7 | [ ] P2-T5 evidence | |
| **F10** TUI | [ ] teatest goldens | [ ] (visual — teatest is the harness) | [ ] post-P3 capture | |
| **F11** go-run + `--source` | [~] CI smoke pending (T10) — read tests done (T7) | [ ] IH-10, IA-10 | [ ] demo step 6 | |

---

## 7. Integration scenario checklist (plan §7.2, proven by P1-T11)

Compiled binary, fake `$HOME`, real `git`, zero network. Each scenario is one named subtest.

### 7.1 Happy path — IH-* (ordered subtests sharing one world)

- [ ] **IH-1** `gff lint` on an authored flag file (bools + one radio + one checkbox choice with typed values) ⇒ exit 0
- [ ] **IH-2** `gff install` in repo A ⇒ `sources.yaml` + snapshot written; `gff list` works from `$HOME`
- [ ] **IH-3** `get`/`enabled` on a default-true bool from a foreign CWD ⇒ `true` / exit 0
- [ ] **IH-4** `selected` on the default choice option ⇒ exit 0; `get` prints the id(s)
- [ ] **IH-5** `set` bool `false` ⇒ ONLY the user override file changes (0600); `list --json` shows `layer=user-override`
- [ ] **IH-6** `set` choice — single: one id; multi: two ids — round-trips through `get`
- [ ] **IH-7** `export --format shell` evals cleanly in bash AND dash; `gff_on` skips the false key, runs the true key
- [ ] **IH-8** `export --format dotenv -o .env` parses with go-envparse; `json` and `yaml` unmarshal to identical `[]Resolved` incl. typed payloads
- [ ] **IH-9** `unset` ⇒ default restored; winning layer reverts to snapshot/repo
- [ ] **IH-10** zero-install + cross-repo: `go run . <verb>` and `--source <name>` / `--source <path>` from a foreign CWD

### 7.2 Adversarial / negative — IA-* (isolated worlds)

Errors must be *clean*: correct exit code, message names the offender, zero partial writes.

- [ ] **IA-1** unknown key ⇒ exit 2 on `get`/`enabled`/`set`; unknown option id ⇒ exit 2 on `selected`
- [ ] **IA-2** `set` with two ids on a `single`-mode choice ⇒ exit 1; override file byte-identical before/after
- [ ] **IA-3** malformed flag file (truncated mid-list, bad indent) ⇒ `lint` and every read verb fail naming file+line; never a panic/stacktrace
- [ ] **IA-4** malformed override yaml ⇒ read verbs error cleanly (not silently skipped); other layers unaffected afterward
- [ ] **IA-5** injection: description containing `$(rm -rf /tmp/pwned)` never reaches export output; option id `evil;rm` rejected by lint; exported bytes assert against `[A-Z0-9_=,.\n-]`-only
- [ ] **IA-6** different url installing an already-registered namespace ⇒ `ErrNamespaceTaken` naming the existing url; registry unchanged; same short keys coexist across namespaces
- [ ] **IA-7** corrupt `sources.yaml` ⇒ verbs degrade with a clear error — and the shell gate stays fail-open
- [ ] **IA-8** read-only `~/.config` ⇒ `set` exits 1, no temp-file litter
- [ ] **IA-9** `HOME` unset ⇒ clear error; nothing written to CWD
- [ ] **IA-10** `--source` with an unknown name and with a non-repo path ⇒ exit 2
- [ ] **IA-11** 10 concurrent `set` calls ⇒ final override is valid yaml equal to one of the written values (atomic temp+rename)
- [ ] **IA-12** `gff.source` redirect pointing at a missing file / outside the repo ⇒ clean error; no path-traversal surprises
- [ ] **IA-13** after `gff install`, `git status --porcelain` in the source repo is empty (installing never dirties the registered worktree)
- [ ] **IA-14** registered repo's checkout moved on disk ⇒ registry snapshot still resolves from any CWD
- [ ] **IA-15** empty feature set ⇒ all four export formats emit valid empty output (never null/malformed), exit 0

### 7.3 Shell-side negatives (plan §7.2 tail, proven by P2-T2 `opt/lib/gff_test.sh`)

- [ ] unset var ⇒ run · [ ] exactly `"false"` ⇒ skip · [ ] `"FALSE"` / `"0"` / garbage ⇒ run · [ ] missing binary ⇒ run
- [ ] all of the above pass under **bash** · [ ] and under **dash** (`sh`)

---

## 8. Validation done-when (plan §7.5) — the stop condition

- [ ] `gff-ci.yml` fully green: vet, unit tests with **≥90%** coverage, `e2e` job (all IH-* and IA-* subtests), proto-regen clean, `go run .` smoke
- [ ] VD-1 demo transcript posted on the PR
- [ ] P2-T5 real-install evidence (`SKIP (gff: install.windows.wispr-flow=false)`) posted on the PR
- [ ] Every §6 feature→proof row above checked, **and** reproduced in the leaf PR descriptions
- [ ] `docs/mbo/index.md` state updated per leaf
- [ ] Issue #180 closed (only once all four leaves have landed)

---

## 9. Coverage snapshot (plan §7.1)

Update on each measurement; keep the latest observed number and the command that produced it.

| Scope | Bar | Latest observed | Command |
| :-- | :-- | :-- | :-- |
| `internal/resolve` | ≥95% | 95.9% (2026-07-25) | `go test ./internal/resolve/ -cover` |
| `internal/schema` | ≥90% | 90.5% (2026-07-25) | `go test ./internal/schema/ -cover` |
| `sdk/gff` overall | **≥90%** | _(not measured)_ | `go test ./... -coverprofile=cover.out && go tool cover -func=cover.out \| tail -1` |

---

## 10. Blockers & escalations

Record anything that stopped a task, with the failing command and its **real** output.
A frozen-contract (plan §3) defect goes here and is escalated — never silently patched.

| Date | Task | Blocker | Command + observed output | Resolution |
| :-- | :-- | :-- | :-- | :-- |
| _(none yet)_ | | | | |

---

## 11. Session log (append-only)

One line per working session. Never rewrite history here — append.

| Date | Session | Leaf(s) | What advanced |
| :-- | :-- | :-- | :-- |
| 2026-07-25 | planning | — | Tracking files authored (`IMPLEMENTATION.md`, `TRACKING.md`, `TODO.md`); build not yet started |
| 2026-07-25 | build-1 | p1-engine | Preflight green (plan on origin/main; go 1.26.3 toolchain, go directive stays 1.26.1; protoc 3.21.12; gh authed). NOTE: gff feature row was absent from the gss registry post-#181-merge; ran `gss feature start gff` to recreate it, then added the p1-engine worker. |
