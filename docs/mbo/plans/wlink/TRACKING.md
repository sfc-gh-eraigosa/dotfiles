# wlink — live state ledger

- **Slug:** `wlink`
- **Started:** 2026-08-25 — built inside PR #242, in the existing `wsl-dns-lan/edward-raigosa/dns` worker
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Cursor:** [`TODO.md`](./TODO.md)
- **Plan (source of truth):** [`../wlink.md`](../wlink.md) · spec [`../../specs/wlink.md`](../../specs/wlink.md)
- **Anchors:** issue [#245](https://github.com/sfc-gh-eraigosa/dotfiles/issues/245)

> **Update after EVERY task.** Status: `todo · in-progress · blocked · done`.
> **Evidence** = the exact command run plus its real result. A row is `done` only with a commit
> SHA **and** evidence. **Never write a result you did not observe.**

## 0. Worker registry

Fill verbatim from `gss feature worker add --feature wlink --purpose cli … --json`.

| Leaf/worker | Worker ref | Branch | Worktree path | PR | State |
| :-- | :-- | :-- | :-- | :-- | :-- |
| cli (single worker — plan §6.1 recommends no fan-out) | `wsl-dns-lan/edward-raigosa/dns` | `feature/wsl-dns-lan/edward-raigosa/dns` | `~/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/wsl-dns-lan/edward-raigosa/dns` | [#242](https://github.com/sfc-gh-eraigosa/dotfiles/pull/242) (draft) | exists |

## 1. Task ledger

Phases from plan §4. `P1` and `P2` are blocking — they freeze the `winhost.Runner` seam and the
`linkstate.State` schema that everything else consumes.

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| P0 — module skeleton + `libs/log` wiring | **done** | *(this commit)* | `go test ./...` → ok; `wlink --version` → `wlink 0.0.0-untagged (18d8436) built … linux/amd64`; hand-rolled-logger grep → none; `evidence/p0/` | `SetDefaultTool("wlink")` once via sync.Once |
| P1 — `winhost` + `Runner` **(BLOCKING)** | **done** | *(this commit)* | `go test -cover ./internal/winhost/...` → ok, **64.3%**; live run against this machine's real Windows parsed all 6 interfaces; interop-outside-seam grep → none; `evidence/p1-winhost/` | seam frozen: `Runner`, `Interface` |
| P2 — `linkstate.State` **(BLOCKING)** | **done** | *(this commit)* | `go test -cover ./internal/linkstate/...` → ok, **100.0%**; JSON shape asserted against spec §4.1; `evidence/p2/` | schema frozen + documented in README |
| P3 — `probe` native DNS | todo | | | must reproduce recorded `dig` outcomes |
| P4 — `probe` scoring + recursion guard | todo | | | EC-1 default-gateway trap; EC-2 guard |
| P5 — `resolvconf` render + derived budget | todo | | | five INI shapes, golden byte-exact |
| P6 — snapshot + drift | todo | | | round-trip byte-for-byte; no write without snapshot |
| P7 — `fleetsrc` (+ `/etc/hosts` exclusion) | todo | | | `fleet discover --json`, ssh fallback |
| P8 — `cmd/pin` + `cmd/unpin` | todo | | | all 54 ported shell cases green |
| P9 — `cmd/status` + `--json` | todo | | | schema validates; status-line budget |
| P10 — `cmd/verify` | todo | | | both matrix states |
| P11 — `cmd/wait` + readiness | todo | | | `not-ready` ≠ `down` |
| P12 — `sshcfg` + `cmd/doctor` | todo | | | keepalive detection; `--fix` idempotent |
| P13 — integration & rollout | todo | | | sdk checklist ×9; script archived + old flag retired same commit; **one flag, `install.sdk.wlink`, default false, fail-closed** |
| P14 — live acceptance | todo | | | real-machine captures |
| P15 — retire the prototype | todo | | | **gated:** EC-1…EC-19 each cite a passing Go test and spec §5.1 is current |

## 2. Feature → proof matrix (spec §5)

A rule is proven only when its named Go test passes **and**, where marked, a live capture exists.

| Rule | Feature | Automated proof | Human/live proof | Notes |
| :-- | :-- | :-- | :-- | :-- |
| EC-1 | F1/F2 candidate selection | [ ] `probe/TestScore_PrefersFleetResolverOverDefaultGateway` | — | the default-gateway trap |
| EC-2 | F3 recursion guard | [ ] `probe/TestGuard_RefusesNonRecursive` (+`_Override`) | — | NXDOMAIN from ns#1 is final |
| EC-3 | F4/F5 render | [ ] `resolvconf/TestWslConf_AllFiveShapes` | — | golden byte-exact |
| EC-4 | F6 snapshot/restore | [ ] `resolvconf/TestSnapshotRoundTrip_ByteForByte`, `TestNoWriteWithoutSnapshot` | [ ] | the safety property |
| EC-5 | F7 `/etc/hosts` exclusion | [ ] `fleetsrc/TestExcludesHostsFileNames` | — | scores `1/1`, not `1/2` |
| EC-6 | F8/F15 readiness | [ ] `probe/TestAllSilent_IsNotReady`, `cmd/TestWaitReady_Timeout` | [ ] during a real handshake | the race hit on the first run |
| EC-7 | F9 verify matrix | [ ] `cmd/TestVerify_Matrix` | [ ] tunnel up **and** down | public resolves in both states |
| EC-8 | F11 native DNS | [ ] `probe/TestNativeResolver_MatchesDigOutcomes` | — | drops the `dig` dependency |
| EC-9 | F12/F13 status/json | [ ] `cmd/TestStatusJSON_Schema` | — | exit codes are contract |
| EC-10 | F14 doctor | [ ] `sshcfg/TestKeepaliveDetection`, `TestFix_Idempotent` | [ ] on a real ssh config | |
| EC-11 | F17 drift | [ ] `resolvconf/TestDriftDetection` | — | |
| EC-12 | non-WSL no-op | [ ] `cmd/TestNonWSL_NoOpExitZero` | — | must never write off-WSL |
| EC-13 | wildcard Host skipped | [ ] `fleetsrc/TestSkipsWildcardHostPatterns` | — | |
| EC-14 | candidate filtering | [ ] `probe/TestFiltersLoopbackAndLinkLocal` | — | + de-dup, first-seen order |
| EC-15 | zero probe hosts | [ ] `cmd/TestNoFleetHosts_CleanNoOp` | — | not an error |
| EC-16 | unpin without a snapshot | [ ] `resolvconf/TestRepairStockLayout` | — | restores WSL's stock symlink |
| EC-17 | symlink → real file | [ ] `resolvconf/TestReplacesSharedSymlink` | — | `/mnt/wsl/resolv.conf` is distro-shared |
| EC-18 | snapshot removed after unpin | [ ] `resolvconf/TestUnpinClearsSnapshot` | — | next pin snapshots fresh state |
| EC-19 | unknown args exit 2 | [ ] `cmd/TestUnknownFlag_ExitTwo` | — | distinct from a safe decline |
| EC-20 | link health / exit code | [x] `linkstate/TestState_Health`, `TestState_NonWSLIsNotDegraded`, `TestState_EmptyFleetIsNotDegraded` | — | **added in P2** — spec gap found while freezing the schema |
| — | spec §5.1 kept current | [ ] every build-time discovery recorded as an EC rule | — | the prototype is deleted; the spec is the record |

## 3. Validation done-when — the stop condition

- [ ] EC-1…EC-19 each have a passing named Go test (plan §5 table complete)
- [ ] Spec §5.1 current — every build-time discovery recorded there as an EC rule
- [ ] `go test ./...` green; module coverage **≥60%**
- [ ] `sdk/AGENTS.md` "Adding a module" checklist — all 9 items, including the `sdk/README.md` section with a **real captured** demo
- [ ] `install.sdk.wlink` registered `boolDefault: false`, gated `gff_opt_in`; `install.sh` builds and pins **only** when it is true
- [ ] Both flag states verified on a real `install.sh` run (plan §6)
- [ ] **P15**: `grep -rn 'wsl_dns_lan\|install.system.wsl-dns' . | grep -v docs/mbo/` → empty; suite still green
- [ ] `docs/wsl-dns.md` content folded into `sdk/wlink/README.md`; `docs/AGENTS.md` repointed
- [ ] Live acceptance checklist (plan §6) captured under `evidence/e2e/`
- [ ] Miss timing ≤ baseline: **20–21s unpinned → 4s pinned**
- [ ] `docs/mbo/index.md` state advanced to `merged`

## 4. Blockers & escalations

Failing command + its **real** output. **A behavior the spec does not cover is a spec gap, not a
free choice** — record it here, add the EC rule to spec §5.1, then implement. Once the prototype
is deleted the spec is the only record.

| Date | Task | Blocker | Command + observed output | Resolution |
| :-- | :-- | :-- | :-- | :-- |
| | | | | |

## 5. Session log (append-only)

| Date | Session | What happened |
| :-- | :-- | :-- |
| 2026-08-25 | P2 | Schema frozen at 100% coverage. **Spec gap found and closed:** the JSON needed a computed `link` verdict so gsl reads one field instead of re-deriving the degraded rules — added to spec §4.1 and pinned as new rule **EC-20**, which also fixes two traps the rules did not state: off-WSL is `ok` (a no-op, not a failure, or install.sh looks broken on machines the feature never applied to) and an **empty** fleet is `ok` (nothing to resolve is not a shortfall). Also pinned: empty collections marshal as `[]`, absences as `null`. |
| 2026-08-25 | P1 | Seam frozen. Two design calls worth recording: queries ask PowerShell for **JSON** (`ConvertTo-Json`) rather than the default table rendering, because table output is column-truncated and locale-dependent; and `decodeRows` handles the **bare-object-vs-array** shape, since `ConvertTo-Json` emits an object when exactly one row matches — a parser assuming an array silently returns nothing on a single-interface machine. A test also caught the tunnel-alias regex being too narrow (hex-only, so `wg-lab`/`wg-home` failed without adapter data); broadened, code fixed rather than the test. |
| 2026-08-25 | P0 | Module skeleton landed. **Deviation from plan §2:** version vars stamped into `cmd` (`cmd.Version`/`Commit`/`BuildDate`/`Dirty`), not `internal/version` — mirroring `sdk/fleet`'s actual `build.sh`, which is the live convention. Plan inventory amended. |
| 2026-08-24 | planning | Design approved. Issue #245 opened. Design/spec/plan + this trio laid down. Build **not started**. Scope corrected: `wlink` is built **inside** PR #242 and the shell prototype is deleted there too — none of it ever lands on `main` (verified: `git ls-tree origin/main` matches none of it), so there is no cutover, archival, or flag migration. Baseline recorded from live testing: fleet lookups **20–21s unpinned → 4s pinned**; `--verify` PASS with the tunnel up (3/3 fleet hosts, 0s each) and a real `ssh lab-pi` login. |
