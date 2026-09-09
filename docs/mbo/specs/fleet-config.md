# fleet config — spec

- **Slug:** fleet-config
- **Date:** 2026-08-31
- **Status:** Draft
- **Relates to:** issue TBD / PR TBD / design [`../designs/fleet-config.md`](../designs/fleet-config.md)

## 1. Goal

An operator can bring one machine's `~/.ssh/config` up to date from another
fleet host (**pull**), or publish local entries out to chosen targets (**push**),
and see in the same output whether the keys those entries name are actually
present and authorized — without any private key ever moving between machines.

**Each command moves configuration in exactly one direction.** There is no
combined operation: the direction is always named at the call site, so an
operator is never guessing which way a change flowed.

## 2. Use cases

**UC1 — adopt a fleet's knowledge onto a fresh workstation.**
*Actor:* operator on a new machine. *Trigger:* `fleet config pull <source>`.
*Flow:* read source config → plan → review diff → confirm → backup → write →
key-readiness report. *Acceptance:* every `#fleet` alias on the source exists
locally with identical modelled fields; local unmodelled lines untouched;
withheld directives named.

**UC2 — propagate a rediscovered IP.**
*Actor:* operator whose peer ran `ssh-find`. *Trigger:* `fleet config pull <peer>`.
*Flow:* as UC1; the plan shows a single `HostName` field delta.
*Acceptance:* `HostName` updated; `User`, `Port`, `IdentityFile`, comments, and
any `ProxyCommand` in the local block unchanged.

**UC3 — publish a correction to the fleet.**
*Actor:* operator who just fixed an entry. *Trigger:* `fleet config push <target>...`.
*Flow:* per-target plan → confirm → remote backup → temp write → validate →
move → post-write probe. *Acceptance:* target config contains the new values;
target still reachable; backup path printed.

**UC4 — see drift without changing anything.**
*Trigger:* `fleet config diff <host>`. *Acceptance:* exit 0, nothing written
anywhere, differences rendered in both directions.

**UC5 — discover that an imported host is unusable.**
*Trigger:* UC1 where the source names an `IdentityFile` absent locally.
*Acceptance:* the alias is still imported, and the miss is named; the operator
is told the host will fail to authenticate until a key exists.

## 3. Architecture

```
                    ┌─────────────────────────────────────────┐
  remote text ────► │ internal/cfgplan   (PURE)               │
  local text  ────► │   Plan(local, remote, Opts) → Plan      │ ──► Plan
                    │   Apply(local, Plan)        → text      │ ──► new text
                    └─────────────────────────────────────────┘
                                    ▲
  internal/sshconf (PURE) ──────────┘   Parse · Add · Update(new) · Mark · Purge
  internal/runner  (ONLY seam that touches a host)
  internal/sshfail (classifies a failed probe)
```

Boundaries: `cfgplan` has no filesystem, network, or clock. `cmd/config_*.go`
owns all I/O. Every merge decision is therefore unit-testable with no socket —
the module's existing rule.

### Types

```go
type ChangeKind string // "add" | "update" | "unchanged" | "skipped"

type FieldDelta struct{ Name, From, To string }

type Change struct {
    Alias  string
    Kind   ChangeKind
    Host   sshconf.Host // resolved result, for add and update
    Fields []FieldDelta // what changed, for rendering
    Reason string       // why skipped
}

type Opts struct {
    Marker string // fleet marker, default "#fleet"
    All    bool   // every concrete Host, not just marked ones
    Source string // recorded in the provenance comment
}

type Plan struct {
    Source      string
    Changes     []Change
    Includes    int      // Include directives seen in the source text
    NotImported []string // distinct unmodelled directive names found
}
```

## 4. Behavior / features

| # | Feature |
| :-- | :-- |
| **F1** | `fleet config pull <source>` — import `#fleet` blocks from one host. Flags: `--all`, `--dry-run`, `--yes`, `--marker`. |
| **F2** | Field-level merge: add missing aliases, update changed modelled fields, preserve every unmodelled local line and comment. |
| **F3** | Structural exec-safety: unmodelled directives cannot be represented and so cannot be written. |
| **F4** | Withheld-directive report: distinct unmodelled directive names found on the source are named. |
| **F5** | `Include` directives are counted and reported; never silently skipped. |
| **F6** | Provenance: imported blocks carry `#fleet` and `# imported from <source>`, with **no timestamp**, so a repeat pull is a genuine no-op. |
| **F7** | Local write safety: timestamped `0600` backup, then exactly one write. |
| **F8** | Key readiness: each imported `IdentityFile` is stat'd locally; misses named. |
| **F9** | `fleet keys sync --host <alias>...` — new flag restricting authorization to named hosts, so a pull can authorize only what it just added. |
| **F10** | Post-pull key offer: for newly added hosts that are **reachable**, offer `keys sync`. Hosts that refuse us are reported as needing manual bootstrap — `keys sync` appends to `authorized_keys` and therefore *cannot* bootstrap a host you cannot already log into. |
| **F11** | `fleet config push <target>...` — publish local `#fleet` entries outward, per-target plan and confirmation. |
| **F12** | Push validation: temp file → `ssh -G -F <temp>` syntax check → move. A config that fails to parse is never installed. |
| **F13** | Self-retarget guard: refuse to modify the alias currently used to reach that target unless `--allow-self-retarget`. |
| **F14** | Remote write safety: timestamped `0600` remote backup, one write per host, path printed. |
| **F15** | Post-push probe: re-probe each target; a target that stopped answering is reported loudly with its backup path. |
| **F16** | `fleet config diff <host>` — render both directions, write nothing, exit 0. |
| **F17** | TUI `p` (pull) and `P` (push) on the cursor host, declared in `keyHelp` so they cannot ship undiscoverable. |
| **F18** | Loopback guard: refuse a source/target whose `ssh -G` hostname parses as loopback (`127.0.0.0/8`, `::1`). |
| **F19** | Failed probes are classified via `internal/sshfail`, so a trust failure is never reported as a network one. |

## 5. Evaluation criteria (per feature)

Every rule below is a test.

| Feature | Fires | Must NOT fire | Edge | Pass |
| :-- | :-- | :-- | :-- | :-- |
| F2 | alias exists both sides, modelled field differs | unmodelled local line changes | alias present with identical fields → `unchanged` | local `ProxyCommand` survives a `HostName` update |
| F3 | source block contains `ProxyCommand` | that directive reaching the local file | `Match exec` in source | hostile fixture yields zero exec directives locally |
| F4 | unmodelled directive present on source | naming a directive that was imported | source has none → empty list | report lists distinct names, sorted |
| F5 | source text contains `Include` | counting `Include` inside a comment | zero includes → no message | count matches fixture |
| F6 | second identical pull | any diff on re-run | source alias renamed | re-run plan is empty |
| F7 | any local write | writing before backup exists | backup dir unwritable → abort, no write | backup present, mode `0600` |
| F8 | imported `IdentityFile` absent locally | flagging a key that exists | no `IdentityFile` in block → silent | miss named with its path |
| F10 | new host reachable | offering `keys sync` to an `auth-failed` host | zero new hosts → no offer | unreachable/refusing hosts listed as manual-bootstrap |
| F12 | remote temp config malformed | installing it anyway | validation command unavailable → abort | malformed fixture is rejected, original intact |
| F13 | plan changes the connecting alias | blocking an unrelated alias | flag supplied → allowed with warning | refused without the flag |
| F15 | target unreachable after write | flagging a target that still answers | target was already unreachable pre-write → note it | backup path printed on failure |
| F18 | `ssh -G` hostname is `127.0.0.1` | blocking a normal LAN address | name does not resolve → normal error path | refused before any connection |
| F1 | `config pull <source>` invoked | pulling when `--dry-run` is set | source alias absent from local config | plan rendered; write gated on confirmation |
| F9 | `keys sync --host a --host b` | authorizing a host not named | `--host` naming a non-fleet alias → error | only named hosts touched |
| F11 | `config push <target>` invoked | pushing to a target not named | zero local `#fleet` blocks → nothing to push, exit 0 | per-target plan shown before any write |
| F14 | any remote write | writing before the remote backup exists | remote `~/.ssh` unwritable → abort, no write | backup present on target, mode `0600`, path printed |
| F16 | `config diff <host>` | any write, local or remote | hosts identical → empty diff | exit 0 with nothing written |
| F17 | `p` / `P` pressed in the TUI | acting on a host owned by another async path | no cursor host → no-op | both keys appear in `keyHelp` header/overlay |
| F19 | probe fails with ssh stderr | classifying a plain error as auth | no stderr available → `unreachable` | trust failure reported as `auth-failed`, not network |

## 6. Verification harness

- **Unit (pure):** table tests for `cfgplan.Plan`/`Apply` and `sshconf.Update`.
- **Command:** `runner.Fake` drives pull/push/diff with no socket; a hostile
  source fixture is a permanent regression test for F3.
- **Race:** `go test -race ./...`, as the module already requires.
- **Human-evidenced gates** (cannot be faked in unit tests): a real two-machine
  pull; a push whose validation *rejects* a malformed config; a refused
  self-retarget. Captured under `plans/fleet-config/evidence/`.
- **Coverage:** new packages ≥ 90%, consistent with `internal/drift` (100%) and
  `internal/sshfail` (93.3%).

## 7. Prerequisites / dependencies

- `internal/sshconf` gains `Update` (field-level rewrite).
- `fleet keys sync` gains `--host` (today it filters by *key* name only).
- No new third-party dependency.

## 8. Out of scope (and why)

| Item | Why |
| :-- | :-- |
| Moving private keys | Breaks a pinned invariant; no design makes it acceptable. |
| Multi-host union / auto conflict resolution | One named source keeps the diff auditable. Additive later. |
| A bidirectional `sync` verb | Direction must be explicit at the call site. A combined operation resolves conflicts by policy instead of by an operator reading a diff, and doubles the blast radius of one mistake. |
| Following `Include` on the remote | Needs approach B or C, both rejected in the design. Reported instead. |
| Scheduled/daemonised sync | `fleet` is on-demand by charter. |
| Removing remote entries on push | Deletion multiplies blast radius; push only adds and updates. |

## 9. Rollback

Local: restore the printed timestamped backup. Remote: same, but if a push cost
SSH access, restoration is out-of-band (console/physical) — fleet cannot repair
a host it can no longer reach. F12/F13 exist to make that outcome very unlikely;
F14/F15 make it recoverable by a human.

> Produced via `superpowers:brainstorming`. The matching plan goes in `../plans/fleet-config.md`.
