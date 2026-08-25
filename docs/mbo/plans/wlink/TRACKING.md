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
| P3 — `probe` native DNS | **done** | *(this commit)* | `go test -cover ./internal/probe/...` → ok, **87.1%**; all four outcomes distinguished against an in-process DNS server; dig-parity comparison + `grep '"dig"'` → none; `evidence/p3-probe/` | **no DNS library added** — stdlib preserves the distinctions |
| P4 — `probe` scoring + recursion guard | **done** | *(this commit)* | `go test -cover ./internal/probe/...` → ok, **94.7%**, 18 cases; EC-1/EC-2/EC-14 each asserted; `evidence/p4/` | ties resolve to first-enumerated (stable across runs) |
| P5 — `resolvconf` render + derived budget | **done** | *(this commit)* | `go test -cover ./internal/resolvconf/...` → ok, **95.2%**; all five INI shapes byte-exact; budget 7s managed / 11s unmanaged; rendered artifacts captured; `evidence/p5/` | Set→Remove round trip byte-for-byte |
| P6 — snapshot + drift | **done** | *(this commit)* | `go test -cover ./internal/resolvconf/...` → ok, **80.9%**, 28 cases; snapshot-failure ⇒ no write asserted; round trip byte-for-byte incl. symlink target; `evidence/p6-snapshot/` | all against a temp root — no privileged write in tests |
| P7 — `fleetsrc` (+ `/etc/hosts` exclusion) | **done** | *(this commit)* | `go test -cover ./internal/fleetsrc/...` → ok, **90.1%**, 10 cases; read-only gate (no WriteFile/Create/OpenFile) → none; `evidence/p7/` | consumes `fleet discover --json`; ssh scan is fallback only |
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
| EC-1 | F1/F2 candidate selection | [x] `probe/TestScore_PrefersTheResolverThatAnswersForTheFleet`| — | the default-gateway trap |
| EC-2 | F3 recursion guard | [x] `probe/TestGuard_RefusesAResolverThatCannotRecurse`, `TestGuard_OverrideAllowsButStillExplains`| — | NXDOMAIN from ns#1 is final |
| EC-3 | F4/F5 render | [x] `resolvconf/TestSetGenerateResolvConf_AllFiveShapes`, `TestRenderResolvConf_WinnerFirstThenFallbacks`| — | golden byte-exact |
| EC-4 | F6 snapshot/restore | [x] `resolvconf/TestApply_SnapshotsBeforeWritingAndRecordsTheSymlinkTarget`, `TestApply_RefusesToWriteWhenTheSnapshotCannotBeTaken`, `TestRestore_RoundTripsByteForByteAndClearsTheSnapshot`, `TestApply_DoesNotOverwriteAGoodSnapshotOnRerun`| [ ] | the safety property |
| EC-5 | F7 `/etc/hosts` exclusion | [x] `fleetsrc/TestResolve_ExcludesNamesServedByHostsFile`, `TestHostsFileNames`| — | scores `1/1`, not `1/2` |
| EC-6 | F8/F15 readiness | [ ] `probe/TestAllSilent_IsNotReady`, `cmd/TestWaitReady_Timeout` | [ ] during a real handshake | the race hit on the first run |
| EC-7 | F9 verify matrix | [ ] `cmd/TestVerify_Matrix` | [ ] tunnel up **and** down | public resolves in both states |
| EC-8 | F11 native DNS | [x] `probe/TestLookupA_DistinguishesTheFourOutcomes` (resolved · nxdomain · nodata · servfail · silent) | — | drops the `dig` dependency |
| EC-9 | F12/F13 status/json | [ ] `cmd/TestStatusJSON_Schema` | — | exit codes are contract |
| EC-10 | F14 doctor | [ ] `sshcfg/TestKeepaliveDetection`, `TestFix_Idempotent` | [ ] on a real ssh config | |
| EC-11 | F17 drift | [x] `resolvconf/TestDetectDrift`, `TestDetectDrift_UnmanagedIsNotDrift`, `TestDetectDrift_DeletedManagedFile`| — | |
| EC-12 | non-WSL no-op | [ ] `cmd/TestNonWSL_NoOpExitZero` | — | must never write off-WSL |
| EC-13 | wildcard Host skipped | [x] `fleetsrc/TestResolve_NeverProbesWildcardHostPatterns`| — | |
| EC-14 | candidate filtering | [x] `probe/TestFilterCandidates`| — | + de-dup, first-seen order |
| EC-15 | zero probe hosts | [x] `fleetsrc/TestResolve_NoFleetHostsIsNotAnError`| — | not an error |
| EC-16 | unpin without a snapshot | [x] `resolvconf/TestRestore_WithoutASnapshotRepairsTheStockLayout`| — | restores WSL's stock symlink |
| EC-17 | symlink → real file | [x] `resolvconf/TestApply_ReplacesTheSharedSymlinkInsteadOfWritingThroughIt`| — | `/mnt/wsl/resolv.conf` is distro-shared |
| EC-18 | snapshot removed after unpin | [x] `resolvconf/TestRestore_RoundTripsByteForByteAndClearsTheSnapshot`| — | next pin snapshots fresh state |
| EC-19 | unknown args exit 2 | [ ] `cmd/TestUnknownFlag_ExitTwo` | — | distinct from a safe decline |
| EC-21 | IP-hostname / alias-vs-Hostname | [x] `fleetsrc/TestResolve_SkipsHostnamesThatAreAlreadyAddresses`, `TestResolve_ProbesTheHostnameNotTheAlias` | — | **added in P7** — spec gap found while wiring the fleet contract |
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
| 2026-08-25 | P7 | Host source wired to `fleet discover --json`, killing the duplicate `#fleet` parser; the ssh scan is a **read-only** fallback for machines without fleet, proven by a grep gate (no `WriteFile`/`Create`/`OpenFile` in the package) — fleet stays the only writer of those blocks. **Spec gap found and closed as EC-21:** a fleet entry whose `Hostname` is already an IP must be excluded, not probed — DNS has nothing to resolve for it, so probing would penalise every candidate for a name no resolver was ever asked about, the same false-cap EC-5 prevents. The same rule pinned that the **`Hostname`** is probed, never the alias, since `Hostname` is what ssh actually resolves. |
| 2026-08-25 | P6 | The safety phase. Every case runs against a temp root, so replacing a symlink and rewriting system files is fully covered with **no privileged write in the suite**. EC-17 is asserted the strong way: the test keeps the distro-shared target file and checks it was **not modified**, which is what proves the pin cannot leak into other WSL distros. Two states the prototype never exercised are now covered because they are the *common* ones on a stock install: **no `/etc/wsl.conf` at all** (pin creates it, so unpin must delete it rather than leave a file the user never had) and an absent `resolv.conf`. Re-running pin is asserted not to re-snapshot — a second snapshot would capture wlink's own managed files and unpin would faithfully restore the pin it exists to remove. |
| 2026-08-25 | P5 | Render + INI surgery. Three behaviours added beyond the prototype, each with a reason: the winner is **de-duplicated** out of its own fallback list (a repeat wastes a second full timeout on the same dead server); output is capped at **glibc MAXNS=3** (extras are silently ignored, so emitting more is false redundancy); and `Set`→`Remove` is asserted to round-trip **byte-for-byte**, which is the property the whole undo path rests on. Rendering is kept separate from writing so every shape that could clobber a user's `wsl.conf` is covered as a pure string transform, with no privileged write in the tests. |
| 2026-08-25 | P4 | Scoring + guard. EC-1 asserted with the gateway listed **first** in enumeration order, so a naive implementation that trusts ordering fails the test. Tie-breaking pinned to first-enumerated and looped 20× — map iteration order would otherwise change the pinned resolver run to run on an unchanged machine. `Score` deliberately scores **every** candidate even after a winner emerges, because `status` needs the whole picture: "the ISP resolver answered but knew none of your hosts" is the line that explains why the default route is not the answer. |
| 2026-08-25 | P3 | Native DNS landed; `dig` dependency gone. **No DNS library needed** — checked first, and `net.Resolver` already preserves every distinction wlink needs (timeout→`IsTimeout`, NXDOMAIN→`IsNotFound`, SERVFAIL→neither), so the module still has 14 total deps and works where `dnsutils` never installs. `PreferGo` is load-bearing: without it cgo's resolver reads `/etc/resolv.conf` and answers from the very configuration wlink is trying to evaluate. A test caught cancellation being classified `Unhelpful` (i.e. "the server answered") when nothing came back — a status-line timeout would have looked like a reachable resolver; now checked before the DNSError switch. |
| 2026-08-25 | P2 | Schema frozen at 100% coverage. **Spec gap found and closed:** the JSON needed a computed `link` verdict so gsl reads one field instead of re-deriving the degraded rules — added to spec §4.1 and pinned as new rule **EC-20**, which also fixes two traps the rules did not state: off-WSL is `ok` (a no-op, not a failure, or install.sh looks broken on machines the feature never applied to) and an **empty** fleet is `ok` (nothing to resolve is not a shortfall). Also pinned: empty collections marshal as `[]`, absences as `null`. |
| 2026-08-25 | P1 | Seam frozen. Two design calls worth recording: queries ask PowerShell for **JSON** (`ConvertTo-Json`) rather than the default table rendering, because table output is column-truncated and locale-dependent; and `decodeRows` handles the **bare-object-vs-array** shape, since `ConvertTo-Json` emits an object when exactly one row matches — a parser assuming an array silently returns nothing on a single-interface machine. A test also caught the tunnel-alias regex being too narrow (hex-only, so `wg-lab`/`wg-home` failed without adapter data); broadened, code fixed rather than the test. |
| 2026-08-25 | P0 | Module skeleton landed. **Deviation from plan §2:** version vars stamped into `cmd` (`cmd.Version`/`Commit`/`BuildDate`/`Dirty`), not `internal/version` — mirroring `sdk/fleet`'s actual `build.sh`, which is the live convention. Plan inventory amended. |
| 2026-08-24 | planning | Design approved. Issue #245 opened. Design/spec/plan + this trio laid down. Build **not started**. Scope corrected: `wlink` is built **inside** PR #242 and the shell prototype is deleted there too — none of it ever lands on `main` (verified: `git ls-tree origin/main` matches none of it), so there is no cutover, archival, or flag migration. Baseline recorded from live testing: fleet lookups **20–21s unpinned → 4s pinned**; `--verify` PASS with the tunnel up (3/3 fleet hosts, 0s each) and a real `ssh lab-pi` login. |
