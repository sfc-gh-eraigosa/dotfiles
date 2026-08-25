# wlink — implementation playbook

- **Slug:** `wlink`
- **Date:** 2026-08-24
- **Status:** Ready to execute
- **Plan (source of truth):** [`../wlink.md`](../wlink.md) · spec [`../../specs/wlink.md`](../../specs/wlink.md) · design [`../../designs/wlink.md`](../../designs/wlink.md)
- **Objective anchors:** issue [#245](https://github.com/sfc-gh-eraigosa/dotfiles/issues/245) · design docs ride with PR [#242](https://github.com/sfc-gh-eraigosa/dotfiles/pull/242) · `docs/mbo/index.md` row `wlink`

> This file is the **procedure**. It does not restate the plan — it tells a fresh agent session
> how to execute the plan, task by task, resumably. **The plan wins any conflict.**

## 0. The three files

| File | Role |
| :-- | :-- |
| `IMPLEMENTATION.md` (this) | Procedure: preconditions, worker map, per-task loop, hard rules, kickoff prompt. Read-only during the run except §7. |
| [`TRACKING.md`](./TRACKING.md) | Live state ledger: per-task status/commit/evidence, proof matrix, blockers, session log. Updated after **every** task. |
| [`TODO.md`](./TODO.md) | The cursor: plan phases expanded into ordered micro-steps. Ticked as you go. |

**Resumption rule:** the first unchecked box in `TODO.md` is the next action; `TRACKING.md` says
what has been proven. **Re-run the last verification command before continuing** — the ledger is
a claim, the command is the proof.

## 1. Preconditions

| # | Precondition | Verify |
| :-- | :-- | :-- |
| 1 | Working in PR #242's worker worktree — the build happens **in** that PR, not after it | `git rev-parse --abbrev-ref HEAD` → `feature/wsl-dns-lan/edward-raigosa/dns` |
| 2 | Go toolchain present | `go version` |
| 3 | On a WSL host for the live phases (P1 fixtures, P14) | `grep -qi microsoft /proc/version && echo WSL` |
| 4 | `fleet` binary available (P7 consumes its contract) | `fleet discover --json \| head -3` |
| 5 | Worker worktree exists and is current | `gss feature list --feature wlink --json` |
| 6 | The prototype still runs — useful for side-by-side comparison until P15 deletes it | `bash opt/scripts/system/wsl_dns_lan_test.sh \| tail -2` → `PASS=54 FAIL=0` |

**The prototype stays runnable until P15** — not as a gate, but because comparing against a
working reference is the fastest way to settle "is this Go behavior right?". Its behavioral
inventory already lives in spec §5.1 as EC-1…EC-19, which is what P15 is actually gated on.

## 2. Worker map

**Use the existing worker — do not create a new feature.** The build happens in PR #242, whose
worker already exists:

| Field | Value |
| :-- | :-- |
| Feature | `wsl-dns-lan` |
| Worker ref | `wsl-dns-lan/edward-raigosa/dns` |
| Branch | `feature/wsl-dns-lan/edward-raigosa/dns` |
| PR | [#242](https://github.com/sfc-gh-eraigosa/dotfiles/pull/242) (draft) |
| Base | `main` |

Confirm with `gss feature list --feature wsl-dns-lan --json`; push with
`gss feature checkpoint --worker wsl-dns-lan/edward-raigosa/dns`.

The plan's §6.1 records a leaf DAG but **recommends against fanning out**: the leaves share
`internal/`, so a sequential build in this one worker is cheaper than the integration overhead.

## 3. The execution loop (every task)

1. **Locate** — first unchecked `TODO.md` box → its plan phase. Read the plan phase fully.
2. **RED** — write the failing test first. Run it. **Verify it fails**, and record the failure
   line in `TRACKING.md`. A test that passes before the implementation is not a test.
3. **GREEN** — implement the minimum to pass. Run it.
4. **Gates** — the phase's extra checks: `go vet`, coverage, and for any new path outside
   `sdk/wlink/`, `git status --short -- <path>` (the `.gitignore` allowlist).
5. **Evidence** — `tee` the gate command's output into `evidence/<phase>/`, dated header,
   append-only. **A phase without captured evidence is not done.**
6. **Ledgers** — tick `TODO.md`; update the `TRACKING.md` row (status, commit SHA, evidence).
7. **Commit** — stage by explicit name (never `git add -A`), then `gss feature checkpoint`.
   Per repo rules, **confirm via `AskUserQuestion` before any `git add`/`commit`/`gss` push.**

## 4. Done-when gates

Per phase, from plan §4. The **overall stop condition** (also tickable in `TRACKING.md` §3):

- [ ] All 12 spec evaluation rules (EC-1…EC-12) have a passing named Go test — plan §5 table complete
- [ ] Spec §5.1 current — anything learned during the build is recorded there as an EC rule
- [ ] `go test ./...` green; module coverage **≥60%** (the `sdk/` floor)
- [ ] `sdk/AGENTS.md` "Adding a module" checklist complete, all 9 items
- [ ] `install.sdk.wlink` registered (`boolDefault: false`, `gff_opt_in`); `install.sh` builds and pins only when true
- [ ] Both flag states verified on a real `install.sh` run (plan §6)
- [ ] **P15**: prototype, its test, `docs/wsl-dns.md`, and the `install.system.wsl-dns` entry deleted; suite still green
- [ ] `wsl_dns_lan.sh` + its test archived **in the same PR**
- [ ] Live acceptance checklist (plan §6) captured under `evidence/e2e/`
- [ ] Miss timing ≤ the recorded baseline: **20–21s unpinned → 4s pinned**

## 5. Hard rules

1. **The spec carries the knowledge, so keep it current.** The prototype is deleted in P15; once
   it is gone, spec §5.1 is the only record of what it proved. Anything you learn during the
   build that the prototype had handled goes **into spec §5.1 as an EC rule**, not just into
   code. A behavior that never reaches the spec is a behavior that is lost. P15 is gated on
   EC-1…EC-19 each citing a passing Go test.
2. **No write without an undo path.** If the snapshot cannot be written, `pin` writes nothing
   and exits 0. This is not negotiable — it is the property that makes the tool safe.
3. **A safe decline is exit 0.** No winner, guard tripped, non-WSL, tunnel down → exit 0.
   `install.sh` must never fail because a tunnel happens to be down.
4. **A behavior the spec does not cover is a spec gap.** Diverging from the prototype may well
   be right — but decide it deliberately: log it in `TRACKING.md` §4 and add or amend the EC rule
   in spec §5.1. Never a silent change; the spec outlives the script.
5. **No hand-rolled logging.** `sdk/libs/log` only, `applog.SetDefaultTool("wlink")` once at
   startup (`sdk/AGENTS.md` contract).
6. **Interop stays behind `winhost.Runner`.** No `powershell.exe` call anywhere else, or the
   module stops being testable in CI.
7. **Demos must be real captured output.** An invented transcript fails exactly the reader who
   trusts it (`sdk/AGENTS.md`).
8. **Build gating is fail-closed.** `install.sdk.wlink` uses `gff_opt_in`, not the `gff_on` the
   other `install.sdk.*` flags use. An unset flag, absent `gff`, or a missing export must mean
   *do not build*. This tool is only wanted on machines that reach their fleet over a tunnel.
9. **Never run `install.sh` from the worker worktree** — it symlinks `$HOME` configs. Use the
   main checkout.

## 6. Evidence protocol

`docs/mbo/plans/wlink/evidence/` — one folder per phase, plus `e2e/` and `demo/`. Every
done-when command's output `tee`'d with a dated header, append-only, committed with the task
that produced it. Required captures are enumerated in plan §7; the non-obvious ones:

- `e2e/handshake/` — `status` captured **during** a real handshake, showing `not-ready`
- `e2e/timing/` — miss timings against the 20–21s → 4s baseline
- `p3-probe/` — native-resolver vs recorded-`dig` outcomes (NXDOMAIN / no-response / SERVFAIL /
  NOERROR-no-data)

## 7. Corrections log

Append when this playbook is wrong. Do not silently deviate.

| Date | What was wrong | Correction |
| :-- | :-- | :-- |

## 8. Kickoff prompt (next session)

> Continue the `wlink` build. Read `docs/mbo/plans/wlink/IMPLEMENTATION.md` (procedure),
> then `TRACKING.md` (what is proven), then `TODO.md` (the cursor). The first unchecked box in
> `TODO.md` is your next action.
>
> Before doing anything else: verify preconditions §1, and **re-run the last verification
> command recorded in `TRACKING.md`** — the ledger is a claim, the command is the proof.
>
> The plan `docs/mbo/plans/wlink.md` is the source of truth; this playbook only says how to
> execute it. TDD throughout: failing test first, verify it fails, then implement. Capture
> evidence for every done-when gate into `evidence/<phase>/`. Confirm via `AskUserQuestion`
> before any `git add`/`commit`/`gss` push.
>
> Hard rules are §5 — especially: no write without an undo path, a safe decline is exit 0, and
> spec §5.1 (EC-1…EC-19) is the behavioral record, distilled from the prototype. A behavior it
> does not cover is a spec gap: log it in `TRACKING.md` §4 and add the rule — never a silent
> change. The prototype in `opt/scripts/system/` still runs until P15; compare against it when a
> Go result surprises you.
