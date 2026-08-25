# wlink — live state ledger

- **Slug:** `wlink`
- **Started:** *(not started — gated on PR #242 merging; see `IMPLEMENTATION.md` §1)*
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
| cli (single worker — plan §6.1 recommends no fan-out) | | | | | not created |

## 1. Task ledger

Phases from plan §4. `P1` and `P2` are blocking — they freeze the `winhost.Runner` seam and the
`linkstate.State` schema that everything else consumes.

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| P0 — module skeleton + `libs/log` wiring | todo | | | `SetDefaultTool("wlink")`; no bespoke logger |
| P1 — `winhost` + `Runner` **(BLOCKING)** | todo | | | fixtures captured from a real WSL host |
| P2 — `linkstate.State` **(BLOCKING)** | todo | | | freezes the `--json` schema |
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
| — | 54 shell cases ported | [ ] plan §5 table complete, each row citing a passing test | — | the behavioral oracle |

## 3. Validation done-when — the stop condition

- [ ] EC-1…EC-12 each have a passing named Go test (plan §5 table complete)
- [ ] All 54 shell cases have a cited Go counterpart
- [ ] `go test ./...` green; module coverage **≥60%**
- [ ] `sdk/AGENTS.md` "Adding a module" checklist — all 9 items, including the `sdk/README.md` section with a **real captured** demo
- [ ] `install.sdk.wlink` registered `boolDefault: false`, gated `gff_opt_in`; `install.sh` builds and pins **only** when it is true
- [ ] `install.system.wsl-dns` **removed** from features.yaml and install.sh in the same commit that archives the script (plan §3.2)
- [ ] Both flag states verified on a real `install.sh` run; a leftover `install.system.wsl-dns=true` override switches nothing on
- [ ] `wsl_dns_lan.sh` + `_test.sh` archived **in the same PR** as the wiring
- [ ] `docs/wsl-dns.md` folded into `sdk/wlink/README.md`, existing links kept alive
- [ ] Live acceptance checklist (plan §6) captured under `evidence/e2e/`
- [ ] Miss timing ≤ baseline: **20–21s unpinned → 4s pinned**
- [ ] `docs/mbo/index.md` state advanced to `merged`

## 4. Blockers & escalations

Failing command + its **real** output. **A behavior divergence from the 54 shell cases is a
contract defect and goes here** — it gets escalated, never silently patched.

| Date | Task | Blocker | Command + observed output | Resolution |
| :-- | :-- | :-- | :-- | :-- |
| 2026-08-24 | P0 | Gated: PR #242 not yet merged (ships the predecessor) | `gh pr view 242 --json state` → `OPEN` (draft) | Merge #242, then start |

## 5. Session log (append-only)

| Date | Session | What happened |
| :-- | :-- | :-- |
| 2026-08-24 | planning | Design approved. Issue #245 opened. Design/spec/plan + this trio laid down. Build **not started** — gated on #242 merging. Baseline recorded from live testing: fleet lookups **20–21s unpinned → 4s pinned**; `--verify` PASS with the tunnel up (3/3 fleet hosts, 0s each) and a real `ssh wenlockpi` login. |
