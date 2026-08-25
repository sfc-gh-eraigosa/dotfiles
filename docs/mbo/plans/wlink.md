# wlink — WSL link (tunnel + resolver) management — implementation plan

- **Slug:** `wlink`
- **Date:** 2026-08-24
- **Status:** Approved
- **Relates to:** spec [`../specs/wlink.md`](../specs/wlink.md) · design
  [`../designs/wlink.md`](../designs/wlink.md) · issue
  [#245](https://github.com/sfc-gh-eraigosa/dotfiles/issues/245) · execution trio
  [`wlink/`](./wlink/) · PR [#242](https://github.com/sfc-gh-eraigosa/dotfiles/pull/242)
  (shell predecessor)

## 1. Summary & verdict

Builds `sdk/wlink`, a WSL-only Go CLI that succeeds `opt/scripts/system/wsl_dns_lan.sh`:
probes every per-interface DNS server Windows knows, pins the one that resolves `#fleet` names
(reversibly, with a recursion guard), and adds structured `status`/`doctor`/`wait` on top.

**Approved and ready to execute.** The design's §8 sequencing prerequisites are in place: the
design issue is [#245](https://github.com/sfc-gh-eraigosa/dotfiles/issues/245), and the
execution trio lives in [`wlink/`](./wlink/) —
[`IMPLEMENTATION.md`](./wlink/IMPLEMENTATION.md) (procedure + kickoff prompt),
[`TRACKING.md`](./wlink/TRACKING.md) (evidence ledger), [`TODO.md`](./wlink/TODO.md) (the
resumable cursor).

**Built in PR #242 itself, replacing the shell prototype before that PR merges.**
`wsl_dns_lan.sh` was never meant to ship: it exists to have proven the behavior on real
hardware and to have formed this design. Nothing about it has landed on `main` — verified: `git
ls-tree origin/main` matches none of it. So there is no cutover, no archival, no flag
retirement, and no migration note. The prototype and its `install.system.wsl-dns` flag are
simply **deleted in the same PR that introduced them**, once the Go tool supersedes them
(§4 P15). `main` only ever sees `wlink`.

**The 54 cases in `opt/scripts/system/wsl_dns_lan_test.sh` are this plan's executable
specification.** Every one gets a Go counterpart (§5). That is what makes this a port with a
known-good oracle rather than a rewrite from prose.

## 2. File inventory

| Path | Purpose | Implements |
| :-- | :-- | :-- |
| `sdk/wlink/go.mod` | Module `github.com/sfc-gh-eraigosa/dotfiles/sdk/wlink` | §7 prereqs |
| `sdk/wlink/main.go` | Thin entrypoint → `cmd.Execute()` | — |
| `sdk/wlink/build.sh` | Sources `sdk/version.sh`, stamps version via `-ldflags -X` | sdk checklist #2 |
| `sdk/wlink/cmd/root.go` | cobra root, `--json`, WSL gate, exit-code mapping | F13, EC-12 |
| `sdk/wlink/cmd/status.go` | `wlink status` | F12, EC-9 |
| `sdk/wlink/cmd/pin.go` | `wlink pin` (+ `--dry-run`, `--allow-nonrecursive`) | F1–F6, F10 |
| `sdk/wlink/cmd/unpin.go` | `wlink unpin` | F6, EC-4 |
| `sdk/wlink/cmd/verify.go` | `wlink verify` | F9, EC-7 |
| `sdk/wlink/cmd/doctor.go` | `wlink doctor` (+ `--fix`) | F14, EC-10 |
| `sdk/wlink/cmd/wait.go` | `wlink wait --ready` | F15, EC-6 |
| `sdk/wlink/internal/winhost/runner.go` | `Runner` interface — **the frozen seam** | design §4 |
| `sdk/wlink/internal/winhost/powershell.go` | Real runner; `PATH` lookup + absolute fallback | §7 prereqs |
| `sdk/wlink/internal/winhost/query.go` | DNS servers per interface, adapters, routes, handshake age | F1, F12 |
| `sdk/wlink/internal/winhost/testdata/` | Recorded PowerShell output fixtures | §6 harness |
| `sdk/wlink/internal/probe/dns.go` | Native Go DNS against a specific server | F11, EC-8 |
| `sdk/wlink/internal/probe/score.go` | Candidate scoring; silent vs reachable-but-ignorant | F2, F8, EC-1 |
| `sdk/wlink/internal/probe/guard.go` | Recursion guard | F3, EC-2 |
| `sdk/wlink/internal/linkstate/state.go` | Composed `State` — **the frozen data contract** | F12, F13 |
| `sdk/wlink/internal/resolvconf/resolv.go` | Parse/render `resolv.conf`; derived fail budget | F4, F9 |
| `sdk/wlink/internal/resolvconf/wslconf.go` | `wsl.conf` INI rewrite, all five shapes | F5, EC-3 |
| `sdk/wlink/internal/resolvconf/snapshot.go` | Snapshot / restore / drift detection | F6, F17 |
| `sdk/wlink/internal/fleetsrc/fleet.go` | `fleet discover --json`; ssh-config fallback | F16 |
| `sdk/wlink/internal/fleetsrc/hostsfile.go` | `/etc/hosts` exclusion | F7, EC-5 |
| `sdk/wlink/internal/sshcfg/keepalive.go` | ssh keepalive inspection + `--fix` | F14, EC-10 |
| `sdk/wlink/internal/version/version.go` | ldflags target | sdk checklist #2 |
| *(no logging package)* | every unit logs via `sdk/libs/log`; `SetDefaultTool("wlink")` in `cmd/root.go` | sdk checklist #3 |
| `sdk/wlink/AGENTS.md` + `CLAUDE.md` → symlink | Module docs | sdk checklist #4 |
| `sdk/wlink/README.md` | Deep docs — absorbs `docs/wsl-dns.md` | sdk checklist #5 |

**Touch-points outside the new directory** — the step most often missed:

| Path | Change |
| :-- | :-- |
| `install.sh` | One `gff_opt_in install.sdk.wlink` block that builds/installs the binary and runs the pin (§3.1). The prototype's block is **deleted**, not migrated |
| `.github/gff/features.yaml` | `install.sdk.wlink` (**`boolDefault: false`**, deliberately unlike the other `install.sdk.*` flags) **replaces** the prototype's `install.system.wsl-dns` entry — neither ever reaches `main` |
| `sdk/AGENTS.md` | Row in the **Modules** table (checklist #7) |
| `sdk/README.md` | Row in "Pick your tool" **and** a full section in the house shape: pitch → problem → what it does → reach for it when → `console` demo → gotchas → footer (checklist #7–#8) |
| `scripts/test.sh` | Coverage floor entry `wlink) echo 60 ;;` |
| `docs/wsl-dns.md` | **Deleted** — its content becomes `sdk/wlink/README.md`. It never lands on `main`, so no redirect stub is needed; the `docs/AGENTS.md` row is repointed at the module |
| `docs/mbo/index.md` | State transitions |
| `opt/scripts/system/wsl_dns_lan.sh` + `_test.sh` | **Deleted** in P15, after every one of their 54 cases has a passing Go counterpart. Not archived: it never shipped, and git history holds it |
| `opt/scripts/system/AGENTS.md` | Drop the script's entry, point at the module |
| `opt/profiles/packages.tsv` | **Unchanged** — `dnsutils` stays on its own merit as a core diagnostic |

## 3. Interface contracts

**The frozen seam** (everything else consumes these; freeze before parallel work):

```go
// internal/winhost — the ONLY place that shells out to Windows.
type Runner interface {
    Run(ctx context.Context, script string) ([]byte, error)
}

type Interface struct {
    Alias      string   // "wg65d1fe92", "Wi-Fi"
    Addresses  []string
    DNSServers []string
    IsTunnel   bool     // heuristic: WireGuard-shaped alias / adapter description
}

// internal/linkstate — the composed answer; the --json schema.
type State struct {
    WSL        bool          `json:"wsl"`
    Tunnel     TunnelState   `json:"tunnel"`      // up | not-ready | down | unknown
    Pinned     *PinState     `json:"pinned"`      // nil when unpinned
    Candidates []Candidate   `json:"candidates"`
    Fleet      FleetSummary  `json:"fleet"`       // resolved/total, excluded-by-hosts
    Drift      *DriftReport  `json:"drift"`       // nil when clean
}
```

**CLI contract** (stdout is the API):

| Command | Exit 0 | Exit 1 | Exit 2 |
| :-- | :-- | :-- | :-- |
| `status` | healthy | degraded (tunnel down/not-ready, drift, unpinned-but-needed) | usage error |
| `pin` | pinned, or safely declined (no winner / guard tripped / non-WSL) | write failed | usage error |
| `unpin` | restored | restore failed | usage error |
| `verify` | matrix expectations met | an expectation failed | usage error |
| `doctor` | no findings | findings present | usage error |
| `wait --ready` | became ready | timed out | usage error |

A **safe decline is exit 0** — carried over from the shell script, and load-bearing: `install.sh`
must never fail because a tunnel happens to be down.

Orchestration, `pin`:

```
gate WSL → collect fleet names (fleetsrc, minus /etc/hosts)
         → enumerate candidates (winhost)
         → score each (probe): resolved N/M | reachable-but-ignorant | silent
         → no winner? → all silent ⇒ "not-ready" : "wrong tunnel" → EXIT 0, no write
         → recursion guard (probe.guard) → tripped ⇒ EXIT 0, no write
         → snapshot (resolvconf) → FAILS ⇒ EXIT 0, no write
         → write wsl.conf, replace resolv.conf symlink with real file
         → verify via the host resolver → report
```

### 3.1 Feature-flag contract

**One flag: `install.sdk.wlink`** — `boolDefault: false`, gated with `gff_opt_in`.

```sh
gff set install.sdk.wlink true      # build + install the tool, and let install.sh pin
```

| Flag | Default | Gate | Controls |
| :-- | :-- | :-- | :-- |
| `install.sdk.wlink` | **false** | `gff_opt_in` | whether `install.sh` builds and installs the binary **and** runs the pin |

`wlink` is only useful on a machine that reaches its fleet over a VPN/tunnel, so a machine that
has not asked for it never even builds the binary.

**Why not a second flag for the pin.** Setting `install.sdk.wlink=true` already says *"this
machine uses the tunnel and wants its fleet resolvable"* — that **is** the consent to pin. A
separate run-gate would add a step without gating anything meaningfully different. Anyone who
wants the tool without an install-time pin simply runs `wlink pin` when they choose; the
install-time pin is safe to repeat because it is idempotent (a no-op when already pinned) and
declines safely — exit 0, no write — whenever the tunnel is down at install time.

**This deliberately departs from the other `install.sdk.*` flags**, which are `boolDefault: true`
and gated with the fail-open `gff_on` — right for tools every machine wants. `wlink` is not
that: an unset flag, a missing `gff` binary, or a machine where the export never happened must
all mean **do not build**, never "build by default". Hence `gff_opt_in`, matching the
`install.windows.*` opt-in precedent rather than the sdk one. The `features.yaml` description
must say so, so the mismatch is not later "corrected".

**No migration.** `install.system.wsl-dns` gates the prototype inside PR #242 and never reaches
`main`, so `install.sdk.wlink` **replaces** it in that same PR rather than superseding a shipped
flag. No retirement, no orphaned user overrides, no migration note — the flag inventory on
`main` only ever contains the one that matters.

## 4. TDD build order

Each phase: tests first · how to verify · **done-when** · **evidence** (`tee` into
`plans/wlink/evidence/<phase>/`, committed with the task).

| # | Phase | Tests first | Done-when |
| :-- | :-- | :-- | :-- |
| **P0** | Module skeleton + `libs/log` wiring | `main_test.go` asserts `--version` stamps; a test asserts `SetDefaultTool("wlink")` runs before any command body | `go build` + `build.sh` stamps a version; `git status --short -- sdk/wlink` shows tracked; no bespoke logger anywhere in the module |
| **P1** | `winhost` + `Runner` **(BLOCKING — freezes the seam)** | Fixture-driven parse of recorded PowerShell output → `[]Interface` | Parses real captures from this machine (Wi-Fi + WireGuard + Bluetooth); tunnel detection correct; ≥60% |
| **P2** | `linkstate.State` **(BLOCKING — freezes the schema)** | Schema round-trip; `--json` golden | Struct stable; documented in README |
| **P3** | `probe` (native DNS) | EC-8 against a local in-process DNS server: NXDOMAIN / no-response / SERVFAIL / NOERROR-no-data | Reproduces recorded `dig` outcomes; **no `dig` in the module** |
| **P4** | `probe` scoring + guard | EC-1, EC-2 | Default-gateway trap test passes; guard blocks a non-recursive winner |
| **P5** | `resolvconf` | EC-3 (five INI shapes), derived budget | Golden files byte-exact; budget = `ns × timeout × 2 + 1` |
| **P6** | `snapshot` + drift | EC-4, EC-11 | Round-trip byte-for-byte; snapshot-failure ⇒ no write; re-run preserves the original snapshot |
| **P7** | `fleetsrc` | EC-5; `fleet discover --json` stub + ssh-config fallback | Correct hosts with and without `fleet`; `/etc/hosts` names excluded and announced |
| **P8** | `cmd/pin` + `unpin` | Wire P1–P7; dry-run writes nothing | All ported shell cases green (§5) |
| **P9** | `cmd/status` + `--json` | EC-9 | Schema validates; exit codes correct; within the status-line budget |
| **P10** | `cmd/verify` | EC-7 | Both matrix states pass on fixtures |
| **P11** | `cmd/wait` + readiness | EC-6 | `not-ready` distinguished from `down`; timeout ⇒ exit 1 |
| **P12** | `sshcfg` + `cmd/doctor` | EC-10 | Flags a missing `ServerAliveInterval`; `--fix` idempotent |
| **P13** | Integration & rollout | §6 checklist | `install.sdk.wlink` registered fail-closed and `install.system.wsl-dns` removed in the same commit; binary installed only when the flag is true; shell script archived; both sdk tables + README section done |
| **P14** | Live acceptance | §7 | Real-machine captures committed |
| **P15** | **Retire the prototype** | — (deletion; the gate is that P0–P14 are green) | `wsl_dns_lan.sh`, `wsl_dns_lan_test.sh`, `docs/wsl-dns.md`, and the `install.system.wsl-dns` entry are **deleted**; `go test ./...` and the full suite stay green afterwards |

## 5. Verification mapping

Every spec rule → its named test. **And** every one of the 54 shell cases → a Go counterpart;
the port is not done until this table is complete and each row cites a passing test name.

| Spec rule | Go test | Shell ancestor |
| :-- | :-- | :-- |
| EC-1 | `probe/TestScore_PrefersFleetResolverOverDefaultGateway` | "does not pick the default-gateway resolver" |
| EC-2 | `probe/TestGuard_RefusesNonRecursive` (+`_Override`) | "refuses a resolver that cannot answer for public names" |
| EC-3 | `resolvconf/TestWslConf_AllFiveShapes` | the five INI cases |
| EC-4 | `resolvconf/TestSnapshotRoundTrip_ByteForByte`, `TestNoWriteWithoutSnapshot` | "round trip restored … byte-for-byte" |
| EC-5 | `fleetsrc/TestExcludesHostsFileNames` | "scores only the DNS-resolvable hosts (1/1, not 1/2)" |
| EC-6 | `probe/TestAllSilent_IsNotReady`, `cmd/TestWaitReady_Timeout` | "diagnoses an attached-but-not-ready tunnel" |
| EC-7 | `cmd/TestVerify_Matrix` (up/down/no-public/slow-miss) | the four `--verify` cases |
| EC-8 | `probe/TestNativeResolver_MatchesDigOutcomes` | *(new — F11)* |
| EC-9 | `cmd/TestStatusJSON_Schema` | *(new — F12/F13)* |
| EC-10 | `sshcfg/TestKeepaliveDetection`, `TestFix_Idempotent` | *(new — F14)* |
| EC-11 | `resolvconf/TestDriftDetection` | *(new — F17)* |
| EC-12 | `cmd/TestNonWSL_NoOpExitZero` | "not running under WSL" skip |

## 6. Integration & rollout

Follows `sdk/AGENTS.md` **"Adding a module"** literally — it exists because `fleet`, `gff`, and
`libs` each shipped unlisted:

1. `build.sh` sourcing `version.sh` · 2. log via `libs/log` · 3. `AGENTS.md` + `CLAUDE.md`
symlink · 4. `README.md` · 5. `install.sh` build wiring · 6. rows in **both** the `sdk/AGENTS.md`
Modules table **and** `sdk/README.md`'s "Pick your tool" · 7. a `sdk/README.md` section in the
house shape (**demo must be real captured output — an invented transcript fails exactly the
reader who trusts it**) · 8. `git status --short -- sdk/wlink` to confirm tracking against the
`*`-default `.gitignore`.

Plus: coverage floor in `scripts/test.sh`; `docs/wsl-dns.md`'s content moves into
`sdk/wlink/README.md` and `docs/AGENTS.md` is repointed. The prototype's removal is **P15**, not
part of P13 — see below.

**Flag wiring (§3.1)** — `install.sdk.wlink` (`boolDefault: false`, `gff_opt_in`) replaces the
prototype's entry. Verify both states on a real `install.sh` run before calling P13 done:

| `install.sdk.wlink` | Expected |
| :-- | :-- |
| **false** (the default on every machine) | one SKIP line; binary not built; nothing pinned |
| **true** | binary built into `~/opt/bin/`; pin runs — and declines safely (exit 0, no write) if the tunnel happens to be down |

**Retiring the prototype (P15)** — the deletion is gated, not incidental. The 54 shell cases are
the behavioral oracle for this port; deleting them before their Go counterparts pass would
destroy the only evidence the port is faithful. So P15 runs **last**, and only when the §5
traceability table is complete with every row citing a passing test. Deleted, not archived:
`archive/` is for retired-but-kept artifacts that once shipped, and this never did — git history
and this PR's early commits hold it.

**Manual acceptance checklist** (cannot run in CI):

- [ ] Tunnel down → `status` = `down`; `pin` declines, writes nothing, exit 0
- [ ] During handshake → `status` = `not-ready`; `wait --ready` returns 0 when it completes
- [ ] Tunnel up → `pin` selects the tunnel resolver, guard passes, snapshot written
- [ ] `verify` PASS with tunnel up; PASS with tunnel down (miss within budget)
- [ ] `ssh <fleet-host>` connects immediately; public DNS still resolves
- [ ] `doctor` flags the missing `ServerAliveInterval`
- [ ] `unpin` restores both files byte-for-byte
- [ ] Miss timing ≤ the shell baseline (**20–21s unpinned → 4s pinned**, recorded in #242)
- [ ] Both flag states from the §6 table behave as specified on a real `install.sh` run
- [ ] P15 done: prototype, its test, `docs/wsl-dns.md`, and the old flag entry are gone; `grep -rn "wsl_dns_lan\|install.system.wsl-dns" .` returns nothing outside `docs/mbo/`

### 6.1 Build leaves / DAG

Authoritative graph, if the build is broken out (mbo-plan CAP-B). Recommendation:
**do not fan out** — this is one cohesive module and the leaves share `internal/`; a
single-PR sequential build is cheaper than the integration overhead. Recorded for
completeness should that judgment change.

| Leaf | Owns (paths) | Consumes (← edge) | done-when gate | Blocking? |
| :-- | :-- | :-- | :-- | :-- |
| `seam` | `internal/winhost/**`, `internal/linkstate/**` | — | P1+P2 green, ≥60% | **yes (base)** |
| `resolv` | `internal/resolvconf/**` | — | P5+P6 green, golden byte-exact | no |
| `probe` | `internal/probe/**` | `seam` (§3 `Runner`, `Interface`) | P3+P4 green, no `dig` | no |
| `hosts` | `internal/fleetsrc/**`, `internal/sshcfg/**` | — | P7+P12 green | no |
| `cli` | `cmd/**`, `main.go`, `build.sh` | `seam`, `resolv`, `probe`, `hosts` | P8–P12, all 54 ported cases green | no |
| `rollout` | `install.sh`, both sdk tables, `sdk/README.md`, `scripts/test.sh`, and the P15 deletions | `cli` | §6 checklist + manual acceptance | no |

`resolv` and `hosts` have no in-edges and could start immediately; `probe` needs `seam`'s
interfaces frozen; `cli` needs all four; `rollout` is last by construction.

## 7. Validation & evidence (show the work)

**Coverage:** `sdk/` floor of **≥60%** (`scripts/test.sh`), warn-only today under
`COVERAGE_ENFORCE=0` — treat it as hard for this module regardless, since the privileged-write
paths are exactly where untested code hurts.

**Evidence protocol** — `docs/mbo/plans/wlink/evidence/`, one folder per phase plus `e2e/` and
`demo/`; every done-when command's output `tee`'d with a dated header, append-only, committed
with the task that produced it. **A feature without captured evidence is not done.**

Required captures, per design §7:

| Folder | Contents |
| :-- | :-- |
| `p1-winhost/` | Real PowerShell captures from a WSL host with Wi-Fi + WireGuard + Bluetooth |
| `p3-probe/` | Native-resolver vs recorded-`dig` outcome comparison |
| `p6-snapshot/` | `pin` → re-run → `unpin` diff proving byte-for-byte restoration |
| `e2e/tunnel-up/`, `e2e/tunnel-down/` | `verify` output in both states, from a real machine |
| `e2e/handshake/` | `status` captured *during* a handshake showing `not-ready` |
| `e2e/timing/` | Miss timings vs the 20–21s → 4s baseline |
| `demo/` | One capture of `status` before connect / mid-handshake / after — the readiness race is the least obvious behavior and the hardest to convey in prose |

**Adversarial scenarios** the e2e set must include: snapshot directory unwritable; `resolv.conf`
hand-edited after `pin`; `fleet` binary absent; `powershell.exe` not on `PATH`; a candidate that
answers SERVFAIL rather than staying silent; two candidates tying on score.
