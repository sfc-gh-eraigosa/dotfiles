# fleet-connect — implementation playbook

- **Slug:** fleet-connect
- **Date:** 2026-09-02
- **Status:** Ready to execute
- **Plan (source of truth):** [`../fleet-connect.md`](../fleet-connect.md) · spec
  [`../../specs/fleet-connect.md`](../../specs/fleet-connect.md) · design
  [`../../designs/fleet-connect.md`](../../designs/fleet-connect.md)
- **Objective anchors:** issue [#266](https://github.com/sfc-gh-eraigosa/dotfiles/issues/266) · design PR [#267](https://github.com/sfc-gh-eraigosa/dotfiles/pull/267) · `docs/mbo/index.md` row `fleet-connect`

> This file is the **procedure**. It does not restate the plan — it tells a fresh agent session
> how to execute the plan, task by task, resumably. The plan wins any conflict.

## 0. The three files

| File | Role | Written by |
| :-- | :-- | :-- |
| `IMPLEMENTATION.md` (this) | Procedure: preconditions, worker map, per-task loop, hard rules, kickoff prompt | Read-only during the run (except §7 corrections) |
| [`TRACKING.md`](./TRACKING.md) | Live **state ledger**: per-task status/commit/evidence, proof matrix, blockers, append-only session log | Updated after **every** task |
| [`TODO.md`](./TODO.md) | The **cursor**: the plan's 29 tasks expanded into ordered micro-steps | Checkboxes ticked as you go |

**Resumption rule:** the first unchecked box in `TODO.md` is the next action; `TRACKING.md` says
what has been proven. Re-run the last verification command before continuing — the ledger is a
claim, the command is the proof.

## 1. Preconditions

| Precondition | Verify with |
| :-- | :-- |
| Go toolchain matches the module | `cd sdk/fleet && go version` (needs ≥ 1.26.3 per `go.mod`) |
| The module is green before you start | `cd sdk/fleet && go test -race ./... && gofmt -l . && go vet ./...` |
| Repo-wide gates pass | `./scripts/test.sh` (fleet coverage floor 60) |
| Shell gates pass (the remote scripts are POSIX sh) | `make lint-shell && make lint-portability` |
| A live herdr host is reachable for the §4 live gates | `fleet status <spark>` then `ssh <spark> '~/.local/bin/herdr status'` |
| Local herdr exists (needed for attach) | `command -v herdr || ls ~/.local/bin/herdr` |
| Working tree is the objective's worktree, not the main checkout | `git rev-parse --show-toplevel && git branch --show-current` |
| `.gitignore` opts in every new path (the file starts with `*`) | `git status --short -- <new path>`; if absent, `git check-ignore -v <path>` and add a narrow `!`-rule **before** staging |

**Never run `install.sh` from this worktree** (root `CLAUDE.md`): it writes absolute symlinks into
`$HOME`. Build to a scratch path instead — see §5.

## 2. Worker map

Default: **one worker, tasks 1 → 29 in plan order.** The plan's §6.1 DAG is the fan-out shape if
the operator asks for parallel execution; capture `gss feature worker add --json` output verbatim
into `TRACKING.md` §0 if that happens.

| Leaf | Tasks | Owns | Blocking? |
| :-- | :-- | :-- | :-- |
| A contract | T1–T4 | `pkg/provider/{provider,providertest}`, `internal/runner/{handoff,bridge}*.go`, `runner.go` | **yes (base)** |
| B protocol | T5–T8 | `pkg/provider/{wire,serve,client}*.go` | **yes** |
| C registry+config+verbs | T9–T12 | `internal/providers/**`, `cmd/providers.go`, `go.mod` | no |
| D herdr | T13–T18 | `internal/provider/herdr/**` | no |
| E TUI nav | T19–T22 | `cmd/tui_nav*.go` + edits to `cmd/tui_{model,keys,view}.go`, `tui.go`, `tui_demo_test.go` | no |
| F CLI | T23–T24 | `cmd/ls.go`, `cmd/connect.go` | no |
| H bridges | T25–T28 | `internal/bridge/**`, `internal/provider/ports/**`, `cmd/tui_bridge*.go`, `cmd/bridge*.go`, the tunnel branch of `connect.go`, `keyHelp` rows for `t`/`T` (after E and F) | no |
| G integrate | T29 | `cmd/provider_registry.go`, `sdk/fleet/AGENTS.md`, `README.md`s | no |

## 3. The execution loop (every task)

1. **Locate.** First unchecked `TODO.md` box → its plan task. Read the plan task fully, plus the
   spec rules it implements (the plan §5 mapping names them).
2. **RED.** Write the failing test first. Run it. **Verify it fails**, and record the failure line
   in `TRACKING.md` — a test that passes before the implementation exists is testing nothing.
3. **GREEN.** Implement the minimum that passes. Run it.
4. **Gates.** `go test -race ./...`, `gofmt -l .` empty, `go vet ./...` clean, plus the task's own
   gate from the plan (round-trip counts, coverage, `bash -n`/portability for shell scripts,
   `.gitignore` allowlist for new paths).
5. **Evidence.** `tee` the gate command's output into
   `docs/mbo/plans/fleet-connect/evidence/taskNN/`, with a dated header; sanitise hostnames.
6. **Ledgers.** Tick `TODO.md`; update the `TRACKING.md` task row with status, commit SHA and the
   observed evidence.
7. **Commit.** Stage **by explicit path** (never `git add -A`), with the plan's task in the
   subject. Confirm via the interactive prompt before any `git add`/`commit`/push, per the repo's
   gss rules.

## 4. Done-when gates

**Per leaf** (plan §6.1): A — T1–T4 green, `pkg/provider` stdlib-only, ≥ 90% coverage, the eleven
`runner.Exec{}` sites and `cmd/wake.go`'s type assertion untouched. B — T5–T8 green including the
credential leak sweep and the `-32001` refusal, ≥ 90%. C — T9–T12 green, missing config = the
built-in set, ≥ 90%. D — T13–T18 green, fixtures carry provenance, round-trip counts 1/2/1, the
dual-path rendering identical, ≥ 90%, no `cmd` import. E — T19–T22 green, new frames inside the
width guard, **no existing test or golden frame changed**. F — T23–T24 green, golden JSON
committed. H — T25–T28 green, no real port bound by any test, ≥ 90%, one `ssh -N` per host
proven by process count, `Close()` on every exit path. G — `./scripts/test.sh` green and every
live gate captured.

**The objective is done when** every box in `TODO.md` is ticked, every row in `TRACKING.md` §1
carries a SHA and observed evidence, `TRACKING.md` §2 shows a proof for all 26 features, and the
eleven-step manual checklist in plan §6 is signed off with captures.

## 5. Hard rules

- **`internal/runner` is the only package that opens a connection to a host.** A provider reaches
  a machine solely through `Host.Exec` / `host/exec`. Anything else is a contract defect, not a
  shortcut.
- **`host/exec` carries a `callId`, never an alias.** Do not "simplify" it by passing the host —
  that reintroduces the fleet-enumeration escape the plan §1 correction removed.
- **The contract is frozen at the end of T4 and the protocol at the end of T8.** After that, a
  needed change goes through `TRACKING.md` §4 as a blocker and an explicit plan amendment — never
  a quiet edit while implementing a later task.
- **Local handoffs are argv with no shell; every value interpolated into a remote command is
  `runner.Quote`d.** No exceptions, including for values that "cannot" contain metacharacters.
- **No credential, hostname, port, user or key path may cross the plugin wire.** The leak sweep in
  T8 asserts it over the marshalled bytes; keep it passing.
- **No new TUI in-flight state.** Drill-down reuses `canStartConfigAction()`.
- **Do not touch `Row`, the eleven `runner.Exec{}` sites, `cmd/wake.go`'s assertion, `install.sh`,
  or `scripts/test.sh`'s floor.**
- **No action payload names a host or an address.** `Handoff`, `Stream` and `Tunnel` carry no
  such field; `runner` takes the alias as a parameter and only fleet supplies it. A tunnel
  targets the dispatched host's `127.0.0.1` and binds the workstation's `127.0.0.1` — never
  another address on either side.
- **One `ssh -N` per bridged host, and a bridge never outlives fleet.** `internal/bridge` owns
  every bridge context; `q`, Ctrl-C and `Close()` tear every set down before exit. No `ssh -f`,
  no daemon, no pidfile, no automatic restart of a failed set.
- **Bridge tests bind no real port.** `listen`/`dial` are injected; a test that opens a socket
  is a defect.
- **Every remote script is POSIX `sh`** per `docs/mbo/specs/shell-portability.md` — no `[[`, no
  arrays, no `local`. `make lint-portability` is enforcing.
- **Demos and fixtures must be real output.** Fixtures carry the herdr version and capture date.
  An invented transcript fails only for the reader who trusts it.
- **Evidence before assertions.** A `TRACKING.md` row is `done` only with a commit SHA *and*
  observed output. Never write a result you did not see.
- **Build to a scratch path** (`go build -o /tmp/.../fleet ./`) while iterating. Install to
  `~/opt/bin/fleet` via `sdk/fleet/build.sh` **only** when the operator asks to verify the CLI —
  that replaces the binary they are using.

## 6. Command cheat-sheet

```bash
cd sdk/fleet
go test ./pkg/provider/ -run TestName -v          # one test, verbose
go test -race ./...                               # the per-task gate
go test ./... -cover                              # coverage
go test ./cmd/ -run TestDemoFrames -v             # golden frames (ASCII, CI-safe)
FLEET_DEMO=1 go test ./cmd/ -run TestDemoFrames   # frames in colour, for a human
go list -deps ./pkg/provider | grep -v '^\(internal/\)\?[a-z/]*$'   # prove stdlib-only
gofmt -l . && go vet ./...
cd ../.. && ./scripts/test.sh                     # repo-wide, enforces the fleet floor
make lint-shell && make lint-portability          # the remote sh scripts
go build -o /tmp/fleet-dev ./sdk/fleet && /tmp/fleet-dev providers list   # scratch build
```

## 7. Resuming, recovery, and corrections

Fix a wrong command in this file freely and note it in `TRACKING.md` §5. Never edit the plan or
spec as part of the build: a contract defect goes into `TRACKING.md` §4 with the failing command
and its real output, and is escalated for an explicit amendment.

If a leaf's worktree or PR drifts from the registry, repair with `gss feature audit --feature
fleet-connect --json`, then `--repair`, then `gss feature list --feature fleet-connect --tree`.
Observable state wins over the registry.

## 8. Kickoff prompt (always CURRENT — history lives in git)

> **Maintenance rule:** exactly ONE prompt here — the one that starts the NEXT session. Replace
> it at session end; past prompts are in `git log -- <this file>`.

> Continue the `fleet-connect` objective in this worktree. Read
> `docs/mbo/plans/fleet-connect/TODO.md` and start at the **first unchecked box**; the task detail
> is in `docs/mbo/plans/fleet-connect.md`, the requirements in
> `docs/mbo/specs/fleet-connect.md`, and the rationale in `docs/mbo/designs/fleet-connect.md`.
> Work strictly TDD: write the failing test first, **run it and paste the observed failure**,
> implement the minimum, run `go test -race ./...` plus the task's own gate, `tee` the gate output
> into `docs/mbo/plans/fleet-connect/evidence/taskNN/`, then update
> `docs/mbo/plans/fleet-connect/TRACKING.md` with the commit SHA and that observed output before
> ticking the box. Honour the hard rules in `IMPLEMENTATION.md` §5 — above all: only
> `internal/runner` opens a connection to a host, `host/exec` carries a `callId` and never an
> alias, no action payload names a host, the contract freezes after T4 and the protocol after
> T8, local handoffs are argv with no shell, and a bridge never outlives fleet. Stage by explicit path and **confirm with the interactive prompt before any `git add`,
> `commit`, or push**. Stop and ask the operator for: the live gates on real hardware (plan §7),
> any change to a frozen contract, and the first push. If you are blocked, record it in
> `TRACKING.md` §4 with the failing command and its real output rather than working around it.
> Done when every `TODO.md` box is ticked, `TRACKING.md` §2 shows a proof for all 26 features, and
> `./scripts/test.sh` is green.
