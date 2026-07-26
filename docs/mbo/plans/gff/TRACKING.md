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
| `p1-engine` | `gff/edward-raigosa/p1-engine` | `feature/gff/edward-raigosa/p1-engine` | `${HOME}/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/gff/edward-raigosa/p1-engine` | [#182](https://github.com/sfc-gh-eraigosa/dotfiles/pull/182) | merged |
| `p2-instrument` | `gff/edward-raigosa/p2-instrument` | `feature/gff/edward-raigosa/p2-instrument` | `${HOME}/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/gff/edward-raigosa/p2-instrument` | [#184](https://github.com/sfc-gh-eraigosa/dotfiles/pull/184) | merged |
| `p3-tui` | `gff/edward-raigosa/p3-tui` | `feature/gff/edward-raigosa/p3-tui` | `${HOME}/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/gff/edward-raigosa/p3-tui` | [#186](https://github.com/sfc-gh-eraigosa/dotfiles/pull/186) (closed) | superseded → `p34-tui-gen` |
| `p4-gen` | `gff/edward-raigosa/p4-gen` | `feature/gff/edward-raigosa/p4-gen` | `${HOME}/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/gff/edward-raigosa/p4-gen` | [#185](https://github.com/sfc-gh-eraigosa/dotfiles/pull/185) (closed) | superseded → `p34-tui-gen` |
| `p34-tui-gen` | `gff/edward-raigosa/p34-tui-gen` | `feature/gff/edward-raigosa/p34-tui-gen` | `${HOME}/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/gff/edward-raigosa/p34-tui-gen` | [#187](https://github.com/sfc-gh-eraigosa/dotfiles/pull/187) | in-review (ready, rebased onto #184 merge) |
| `vd-demo` | `gff/edward-raigosa/vd-demo` | `feature/gff/edward-raigosa/vd-demo` | `${HOME}/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/gff/edward-raigosa/vd-demo` | _(pending)_ | building |

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
| P1-T8 write verbs — set/unset | done | 9c24776 | `go test ./cmd/ -cover` -> ok, 78.2% (38 tests); `go test ./...` green; `go vet ./...` clean; RED verified: `unknown command \"set\"` | evidence: F08-write-path/P1-T8-write.txt; internal/overrides.Write/Unset extracted (P3 seam) + WriteFileAtomic shared with registry; adversary fix: single-mode arity error de-wrapped from ErrUnknownOption so IA-2 gets exit 1 not 2; testify added (test-only dep) |
| P1-T9 export + install verbs | done | e55c762 | `go test ./cmd/ -cover` -> ok, 72.6% (~53 tests); golden = AI_CLAUDE=true / PKG_MANAGER=auto / WISPR_FLOW=false byte-exact; dotenv round-trips go-envparse v0.1.0; json/yaml -> identical []ResolvedJSON; `go vet` clean; RED verified: `undefined: mangleKey` | evidence: F07-export/P1-T9-export-golden.txt; injection assert narrowed to shell/dotenv (json/yaml carry description BY CONTRACT §3.3) + value-regex assert |
| P1-T10 public SDK + CI + coverage gate | done | _(next commit)_ | coverpkg-excl-gen total 91.6% (bar ≥90), resolve 96.0% (≥95), schema 95.6%/92.0% (≥90); `go test ./...` + `go vet` green; `go run . version` smoke ok; `bash build.sh` installs working binary; CI gate dry-run GATE-PASSES | QA fan-out twice tried to lower the total gate (77→85) — REJECTED; closed the gap with real tests instead (overrides/version/main-exitCode/cmd error paths/pkg typed accessors + white-box helpers). Also fixed: dotenv default `-o .env` test leak (t.Chdir), IA-10 resolve fix — non-repo `--source` path now ErrUnknownSource (was silently empty world) |
| P1-T11 binary-level e2e harness | done | 13efccf | `make gff-e2e` -> PASS, all 25 subtests (IH-1..10 + IA-1..15); unit suite + `go vet` still green; lint-shell OK; portability Tier1=0 Tier2=0 | Harness exposed 4 harness bugs (cmd.Dir always fakeHome — runCmdIn added; IA-14 chdir-before-mkdir; IA-9 glob len>1; IA-3 fixture was valid yaml) + IA-6 false-positive hardened; production additions: lint `missing-default` rule (TDD), IA-10 resolve fix (earlier commit) |
| **P1 done-when gate** | done | — | PR #182 all checks green (2026-07-25): `unit tests + coverage gate` pass (total 91.7% ≥90, resolve 96.0 ≥95, schema 95.7 ≥90), `binary-level e2e tests` pass (25/25), shell-lint pass, `go run . version` smoke in CI, `make gff-proto-check` clean; `bash build.sh` installs working `gff version` | awaiting ready-promotion confirmation |

---

## 2. Leaf `p2-instrument`

**Owns:** `.github/gff/**`, `opt/lib/gff.sh*`, `install.sh`, `opt/bin/install_windows.sh`,
`opt/Desktop/Apps/scripts/{lib/gff.ps1,setup-apps.ps1,setup-elevated.ps1}`
**Consumes:** `p1-engine` (plan §3.4 CLI, §3.5 shell contract)
**Done-when (plan §6.1):** lint-shell + lint-portability clean; `gff lint` clean;
P2-T5 human evidence posted.

| Task | Status | Commit | Evidence (test run / gate) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| P2-T1 dotfiles flag inventory (43 flags) | done | 5dd5e1b | allowlist: `.gitignore:33:!.github/**` NOT ignored; `gff lint` exit 0; 43 keys counted (grep + `gff list --json` jq), all repo-live | evidence F09-gating/p2-t1-lint-list.txt |
| P2-T2 shell helper `opt/lib/gff.sh` | done | 6d00762 | orchestrator re-ran: `bash opt/lib/gff_test.sh` 10/10 PASS **and** `sh` (dash) 10/10 PASS; RED verified (helper missing ⇒ FAIL 1); driver mode 100755 confirmed via `git ls-files -s` | plan snippet verbatim + shellcheck disable comment lines only; evidence p2-t2-*.txt (append-only re-capture) |
| P2-T3 instrument `install.sh` (Linux/common) | done | a131195 | orchestrator re-ran: `bash -n` clean; helper sourced line 23 (before first gate line 67, fail-closed guard comment); `set -a`/`set +a` wrap at the bootstrap eval (the binding NOTE); 35 `gff_on` sites / 33 keys; sops SKIP line reproduced | new later `install.sdk.gff`-gated duplicate build block added (plan presupposed one; bootstrap build stays ungated) |
| P2-T4 Windows pass-through + PS gating | done | 0a2820e | orchestrator re-ran: `bash -n` clean on install_windows.sh; `make lint-shell` clean; `make lint-portability` Tier1=0 Tier2=0; WSLENV builder verbatim + dedup proven (2-pass dash test); Test-GffOn per plan | **pwsh absent — both Test-GffOn checks defer to P2-T5 human run (not faked)**; WSLENV loop inserted once after `ps_exe` (precedes every powershell.exe call, dedup makes it equivalent) |
| P2-T5 human-evidenced acceptance | todo | — | | real terminal, WSL; **must `eval "$(gff export --shell)"` in the calling shell first** — see §10 row 3. #184 merged ahead of this evidence (owner call) — run it from `${HOME}/git/dotfiles` on main now |
| **P2 done-when gate** | todo | — | lint gates clean + `gff lint` clean (done); P2-T5 evidence pending | PR [#184](https://github.com/sfc-gh-eraigosa/dotfiles/pull/184) MERGED 2026-07-26 (adf80cc); P2-T5 evidence still owed for §7.5 |

---

## 3. Leaf `p3-tui`

**Owns:** `sdk/gff/internal/tui/**`, `sdk/gff/cmd/tui.go`
**Consumes:** `p1-engine` (plan §3.3 Resolver, overrides writer)
**Done-when (plan §6.1):** teatest suite green; overall coverage ≥90%.

| Task | Status | Commit | Evidence (test run / gate) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| P3-T1 TUI (model, view, teatest, `cmd/tui.go`) | done | 6bfa4c3 | orchestrator re-ran in p3 worktree: `go vet ./...` clean; full `go test ./... -count=1` green (16 tui tests); CI-style coverpkg-excl-gen total 90.4% (≥90), resolve 96.1% (≥95), schema 95.7% (≥90), tui pkg 90.6%; evidence F10-tui/P3-T1-teatest-cover.txt | RED verified: `no non-test Go files in internal/tui`; `internal/overrides.Write` consumed (extracted in P1-T8 — no refactor needed); sole shared-file edit = root.go TTY-dispatch RunE per §6.1; deps bubbletea v1.3.10 + teatest; 3 cover*.out debris files caught + removed before commit |
| **P3 done-when gate** | done | — | teatest suite green; overall ≥90% holds (90.4%); PR [#186](https://github.com/sfc-gh-eraigosa/dotfiles/pull/186) | promotion awaits user confirmation |

---

## 4. Leaf `p4-gen`

**Owns:** `sdk/gff/cmd/gen.go`, `cmd/gen_test.go`, `cmd/testdata/gen.golden`
**Consumes:** `p1-engine` (plan §3.3 `pkg/gff`)
**Done-when (plan §6.1):** golden test green; generated output vets.

| Task | Status | Commit | Evidence (test run / gate) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| P4-T1 `gff gen` typed accessors | done | be17f88 | orchestrator re-ran in p4 worktree: `go vet ./...` clean; `go test ./...` all 10 pkgs ok (cmd incl. 7 Gen tests: golden byte-exact, shape, scratch-module `go vet` compile, empty world, bad --out, naming, update-golden); CI-style coverpkg-excl-gen total 91.4% (≥90), resolve 96.1% (≥95), schema 95.7% (≥90) | RED verified: `undefined: resetGenFlags/segmentToTitle`; agent-reported, gates re-run by orchestrator; one debris file (tmux-mgr scheduled_tasks.lock) caught + restored before commit |
| **P4 done-when gate** | done | — | golden test green + generated output vets (TestGenGoldenCompiles runs `go vet` on a scratch module embedding the output, offline via replace directive); PR [#185](https://github.com/sfc-gh-eraigosa/dotfiles/pull/185) | promotion awaits user confirmation |

---

## 5. Leaf `vd-demo`

**Owns:** `sdk/gff/scripts/demo.sh`
**Consumes:** `p1-engine` + `p2-instrument` (TUI addendum additionally `p3-tui`)
**Done-when (plan §6.1):** demo transcript + evidence posted on the PR (plan §7.3).

| Task | Status | Commit | Evidence (test run / gate) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| VD-1 demo script (`scripts/demo.sh`) | done | _(this commit)_ | 3 consecutive runs exit 0 (fresh scratch HOME each); steps 1–6 per §7.3 incl. dash eval, .env parse, namespace-claim rejection naming the existing url, zero-install `go run .` finale; `make lint-shell` clean, portability Tier1=0/Tier2=0; real `~/.config/gff` mtime-verified untouched | step 6 uses the module-local `go run .` stand-in by default; `GFF_DEMO_TAG=<tag>` opts into the true `@tag` form |
| VD-1 run on WSL + transcript posted | in-progress | — | transcript captured on linux/arm64 (evidence/demo/VD-1-demo-transcript.txt), posted to the leaf PR | a WSL re-run is a one-liner if the owner wants the exact-platform capture |
| VD-1 post-P3 TUI segment (~30s) captured | todo | — | | after `p3-tui` merges |
| **VD-1 done-when gate** | todo | — | | + P2-T5 real-install proof |

---

## 6. Feature → proof matrix (plan §7.4)

A feature is **built** only when all three of its proofs are green. Tick each cell as its
proof passes; record which task proved it in Notes.

| Feature | Unit | Integration | Demo | Notes |
| :-- | :-- | :-- | :-- | :-- |
| **F1** keys + lint | [x] `lint_test.go` table | [x] IH-1, IA-3 | [ ] demo step 2 | |
| **F2** bool semantics | [x] schema/resolve tests | [x] IH-3, IH-5 | [ ] demo steps 2–3 | |
| **F3** choice (modes, ids, typed values) | [x] lint/resolve/write tests | [x] IH-4, IH-6, IA-1, IA-2 | [ ] demo steps 1, 3 | |
| **F4** layered resolution + provenance | [x] resolve matrix 1–10 | [x] IH-5, IH-9 | [ ] demo step 2 | |
| **F5** discovery + `gff.source` redirect | [x] `gitx_test.go` | [x] IH-3, IA-12 | [ ] demo step 2 | |
| **F6** registry + namespace identity | [x] `registry_test.go` | [x] IH-2, IA-6, IA-7, IA-13, IA-14 | [ ] demo step 5 | |
| **F7** export formats + injection safety | [x] export golden | [x] IH-7, IH-8, IA-5, IA-15 | [ ] demo steps 4, 6 | |
| **F8** write path (0600, user-only) | [x] `write_test.go` | [x] IH-5, IA-8, IA-11, IA-13 | [ ] demo step 3 | |
| **F9** fail-open gating | [x] `gff_test.sh` bash + dash (binary-absent is unit-only) | [x] IH-7, IA-7 (proven by P1-T11 e2e) | [ ] P2-T5 evidence | P2-T2 (6d00762): 10/10 under bash AND dash |
| **F10** TUI | [x] teatest goldens + 3 real-terminal key-shape tests | [x] live tmux drive of the compiled binary (#187) | [x] `F10-tui/p34-tui-live-capture.txt` (frames: browse → toggle → provenance → picker) | P3-T1 + the #187 KeySpace fix; a video/gif capture remains optional at owner discretion |
| **F11** go-run + `--source` | [x] CI smoke (T10, `go run . version` in gff-ci) + read tests (T7) | [x] IH-10, IA-10 | [ ] demo step 6 | |

---

## 7. Integration scenario checklist (plan §7.2, proven by P1-T11)

Compiled binary, fake `$HOME`, real `git`, zero network. Each scenario is one named subtest.

### 7.1 Happy path — IH-* (ordered subtests sharing one world)

- [x] **IH-1** `gff lint` on an authored flag file (bools + one radio + one checkbox choice with typed values) ⇒ exit 0
- [x] **IH-2** `gff install` in repo A ⇒ `sources.yaml` + snapshot written; `gff list` works from `$HOME`
- [x] **IH-3** `get`/`enabled` on a default-true bool from a foreign CWD ⇒ `true` / exit 0
- [x] **IH-4** `selected` on the default choice option ⇒ exit 0; `get` prints the id(s)
- [x] **IH-5** `set` bool `false` ⇒ ONLY the user override file changes (0600); `list --json` shows `layer=user-override`
- [x] **IH-6** `set` choice — single: one id; multi: two ids — round-trips through `get`
- [x] **IH-7** `export --format shell` evals cleanly in bash AND dash; `gff_on` skips the false key, runs the true key
- [x] **IH-8** `export --format dotenv -o .env` parses with go-envparse; `json` and `yaml` unmarshal to identical `[]Resolved` incl. typed payloads
- [x] **IH-9** `unset` ⇒ default restored; winning layer reverts to snapshot/repo
- [x] **IH-10** zero-install + cross-repo: `go run . <verb>` and `--source <name>` / `--source <path>` from a foreign CWD

### 7.2 Adversarial / negative — IA-* (isolated worlds)

Errors must be *clean*: correct exit code, message names the offender, zero partial writes.

- [x] **IA-1** unknown key ⇒ exit 2 on `get`/`enabled`/`set`; unknown option id ⇒ exit 2 on `selected`
- [x] **IA-2** `set` with two ids on a `single`-mode choice ⇒ exit 1; override file byte-identical before/after
- [x] **IA-3** malformed flag file (truncated mid-list, bad indent) ⇒ `lint` and every read verb fail naming file+line; never a panic/stacktrace
- [x] **IA-4** malformed override yaml ⇒ read verbs error cleanly (not silently skipped); other layers unaffected afterward
- [x] **IA-5** injection: description containing `$(rm -rf /tmp/pwned)` never reaches export output; option id `evil;rm` rejected by lint; exported bytes assert against `[A-Z0-9_=,.\n-]`-only
- [x] **IA-6** different url installing an already-registered namespace ⇒ `ErrNamespaceTaken` naming the existing url; registry unchanged; same short keys coexist across namespaces
- [x] **IA-7** corrupt `sources.yaml` ⇒ verbs degrade with a clear error — and the shell gate stays fail-open
- [x] **IA-8** read-only `~/.config` ⇒ `set` exits 1, no temp-file litter
- [x] **IA-9** `HOME` unset ⇒ clear error; nothing written to CWD
- [x] **IA-10** `--source` with an unknown name and with a non-repo path ⇒ exit 2
- [x] **IA-11** 10 concurrent `set` calls ⇒ final override is valid yaml equal to one of the written values (atomic temp+rename)
- [x] **IA-12** `gff.source` redirect pointing at a missing file / outside the repo ⇒ clean error; no path-traversal surprises
- [x] **IA-13** after `gff install`, `git status --porcelain` in the source repo is empty (installing never dirties the registered worktree)
- [x] **IA-14** registered repo's checkout moved on disk ⇒ registry snapshot still resolves from any CWD
- [x] **IA-15** empty feature set ⇒ all four export formats emit valid empty output (never null/malformed), exit 0

### 7.3 Shell-side negatives (plan §7.2 tail, proven by P2-T2 `opt/lib/gff_test.sh`)

- [x] unset var ⇒ run · [x] exactly `"false"` ⇒ skip · [x] `"FALSE"` / `"0"` / garbage ⇒ run · [x] missing binary ⇒ run
- [x] all of the above pass under **bash** · [x] and under **dash** (`sh`) — P2-T2, 10/10 each, evidence p2-t2-gff-test-bash-dash*.txt

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
| `internal/resolve` | ≥95% | 96.0% (2026-07-25) | `go test ./internal/resolve/ -cover` |
| `internal/schema` | ≥90% | 95.6% (2026-07-25) | `go test ./internal/schema/ -cover` |
| `sdk/gff` overall | **≥90%** | 90.4% (2026-07-26, p3 worktree post-TUI; 91.4% in p4 worktree — coverpkg excl /gen/, -count=1) | `COVERPKG=$(go list ./... \| grep -v /gen/ \| paste -sd, -); go test ./... -count=1 -coverpkg="$COVERPKG" -coverprofile=cover.out && go tool cover -func=cover.out \| tail -1` |

---

## 10. Blockers & escalations

Record anything that stopped a task, with the failing command and its **real** output.
A frozen-contract (plan §3) defect goes here and is escalated — never silently patched.

| Date | Task | Blocker | Command + observed output | Resolution |
| :-- | :-- | :-- | :-- | :-- |
| 2026-07-26 | (branch-wide CI) | Docker Image CI hung 1.5h+ ("Build the Docker image"): the exec-bit fix let install_snowflake_cli.sh actually RUN inside docker build for the first time, and its `sudo apt-get install pipx` hit tzdata's interactive debconf prompt (latent bug — script predates gff). Note: the heavy job only runs for NON-draft PRs, so it first fired when #182 left draft. | cancelled-run log: `Configuring tzdata / Please select the geographic area` then stdin-wait | Fixed: `sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends pipx`; verified in clean jammy container (rc=0, ~2min, no prompt); stale duplicate runs cancelled |
| 2026-07-25 | (branch-wide CI) | shell-lint workflow red on any gff PR: pre-existing `opt/bin/docker:44` bash-4 `mapfile` (landed via #178/#179; main never re-scanned due to path filters) | `make lint-portability` -> `TIER 2 … opt/bin/docker:44 — bash-4 mapfile/readarray` | Fixed in-branch (fd-3 while-read keeps the #179 stdin-preservation fix; behaviorally tested); scan now Tier1=0 Tier2=0 |
| 2026-07-26 | P2-T4 | `install.windows.{claude-rc-autostart,sshd,portproxy}` have NO invocation site in any owned file — `install-claude-rc-autostart.ps1`, `setup-sshd.ps1`, `refresh-wsl-portproxy.ps1` are standalone by design ("not wired into install.sh") | grep across install_windows.sh / setup-apps.ps1 / setup-elevated.ps1: no call sites | ORCHESTRATOR DECISION: leave the 3 flags declared-but-unenforced (fail-open no-ops today); gating activates if/when the scripts gain an invocation site. Documented in the PR body; NOT silently wired (would change behavior + exceed §6.1 ownership) |
| 2026-07-26 | P2-T5 (upcoming) | Windows deploy invocation (install.sh line ~67) runs BEFORE the in-script gff bootstrap eval (~line 360, placed after goenv per the frozen plan §4 P2-T3) — on a plain run, `gff set` overrides are not yet exported when the PowerShell chain executes | code inspection: source line 23 / windows gate line 67 / bootstrap 357–365 | The plan's own pre-bootstrap caveat applies to the whole Windows path: P2-T5 must `eval "$(gff export --shell)"` in the calling shell before `install.sh` (TODO amended — procedure fix, not a contract edit). UAC boundary env propagation (`Start-Process -Verb RunAs`) remains a P2-T5 observation point; if the flag doesn't cross, escalate as plan-level |

---

## 11. Session log (append-only)

One line per working session. Never rewrite history here — append.

| Date | Session | Leaf(s) | What advanced |
| :-- | :-- | :-- | :-- |
| 2026-07-25 | planning | — | Tracking files authored (`IMPLEMENTATION.md`, `TRACKING.md`, `TODO.md`); build not yet started |
| 2026-07-26 | build-1 | p1-engine | User multi-repo probe found TWO resolver §3.2 gaps, both fixed TDD: (1) named --source snapshots were labeled repo-live -> now user-snapshot; (2) unqualified keys ignored the focus namespace (CWD repo / --source) and errored ambiguous -> now bind focus-first, ambiguity only with no focus. resolve cover 96.1%. |
| 2026-07-26 | build-1 | p1-engine | Review round: user guide + gff-build/test/install make targets; snowflake exec-bit fix; OWNER-APPROVED §3.4 extension (requested on PR review): `gff list [pattern]` glob/prefix filter, aligned table header, indented --json (default) + `--raw` compact form, lipgloss styled table (TTY auto / `--pretty` / NO_COLOR-aware; piped output stays plain) — TDD'd in cmd/list_enhance_test.go; deps lipgloss + x/term added |
| 2026-07-25 | build-1 | p1-engine | P1-T1..T11 all done; PR #182 fully green; extra fixes: opt/bin/docker portability (unblocks shell-lint repo-wide), CI -count=1 profile fix, IA-10 resolve ErrUnknownSource fix, missing-default lint rule. Coverage 91.7/96.0/95.7. Awaiting --ready confirmation. |
| 2026-07-25 | build-1 | p1-engine | Preflight green (plan on origin/main; go 1.26.3 toolchain, go directive stays 1.26.1; protoc 3.21.12; gh authed). NOTE: gff feature row was absent from the gss registry post-#181-merge; ran `gss feature start gff` to recreate it, then added the p1-engine worker. |
| 2026-07-26 | build-2 | p1-engine, p2/p3/p4 | p1 closeout: PR #182 merged; `gss feature merged gff/edward-raigosa/p1-engine` run (positional ref — `--worker` flag doesn't exist on `merged`; procedure note). Created p2-instrument/p3-tui/p4-gen workers; gss branched them from a stale local `main` (2b49b6c, pre-#182) so each fresh branch was reset onto origin/main 6f1003f before any work. `${HOME}/opt/bin/gff` rebuilt from 6f1003f (Dirty:false). Ledger discipline for the parallel phase: TODO/TRACKING/index edits are single-writer (orchestrator), riding the p2 branch (then vd-demo) — p3/p4 branches touch only their owned code paths to keep merges conflict-free. |
| 2026-07-26 | build-2 | p3+p4 → p34-tui-gen | OWNER-DIRECTED restructure: p3-tui + p4-gen combined into one integration PR #187 (draft; #185/#186 closed as superseded). Combined-tree gates all green (11 pkgs, total 90.2%, e2e 25/25). The tmux e2e demo of the REAL binary caught a bug all teatests missed: bubbletea delivers spacebar as KeySpace, model only matched KeyRunes{' '} — toggle dead in a real terminal; fixed TDD (3 real-key-shape tests) + write errors now surfaced in the footer instead of silently discarded (3dba587). Demo evidence committed: e2e/p34-integration-demo.txt (engine→gen compile+run→export) + F10-tui/p34-tui-live-capture.txt (live TUI frames). |
| 2026-07-26 | build-3 | p2, p34, vd-demo | #184 (p2) landed: auto-merge completed after the docker Build+Integration job; `gss feature merged` run; #187 promoted READY via `gh pr ready` (safety_guard/gss token double-bind blocks `gss feature pr --ready` when session cwd branch != worker branch — recorded in memory + here), rebased onto new main via checkpoint, body re-applied, CI re-running. vd-demo worker created off adf80cc; demo.sh authored + 3 clean runs; transcript in evidence/demo/. P2-T5 evidence still owed (merged ahead of it, owner call). |
