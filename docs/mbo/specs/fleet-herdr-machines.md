# fleet-herdr-machines — spec

- **Slug:** fleet-herdr-machines
- **Date:** 2026-10-06
- **Status:** Approved (design decisions recorded in design §4.5; build deferred)
- **Relates to:** issue [#371](https://github.com/sfc-gh-eraigosa/dotfiles/issues/371) /
  PR [#372](https://github.com/sfc-gh-eraigosa/dotfiles/pull/372) / design
  [`../designs/fleet-herdr-machines.md`](../designs/fleet-herdr-machines.md)

## 1. Goal

From `fleet tui`, a CLI verb, or a herdr key, the operator can make any fleet host show up in
the herdr they already have open, as a herdr **saved machine**. They then switch to it with
`prefix+w` and drive its agents with one herdr and one prefix. fleet adds machines and never
removes them, and it never answers a prompt herdr raises.

## 2. Use cases

**UC1. Show one host from the dashboard.**
- **Actor / trigger:** operator in `fleet tui`, presses `R` on a host row.
- **Flow:** fleet checks `herdr machine list --json`. If the host isn't saved, fleet hands the
  terminal to `herdr machine add <alias> --label <alias>`, then returns and sets the status line.
- **Acceptance:** the host appears in the herdr sidebar. The status line names the host and says
  `prefix+w` (inside herdr) or `herdr --remote <alias>` (outside herdr).

**UC2. Show several hosts at once.**
- **Actor / trigger:** operator selects hosts (`space`/`v`/`a`) and presses `R`.
- **Flow:** the same as UC1 for each selected host, one at a time, in selection order.
- **Acceptance:** each host is saved, or reported as skipped with a reason. One host failing
  does not stop the rest.

**UC3. Sync from a script or the plugin.**
- **Actor / trigger:** `fleet herdr sync [host...]`, or the plugin's `sync-machines` action.
- **Flow:** for each in-fleet, non-local host (or the named ones), report `already`, `saved`,
  `needs-approval` or `failed`.
- **Acceptance:** exit 0 only when every host is `already` or `saved`. With no TTY, a host that
  needs approval is not attempted; the output names the exact command to run instead.

**UC4. Open fleet inside herdr.**
- **Actor / trigger:** operator presses `prefix+shift+h` in herdr.
- **Flow:** a herdr popup runs `fleet tui`. Quitting fleet closes the popup.
- **Acceptance:** all fleet tui keys work in the popup, including `R`. A host without fleet
  shows `fleet is not installed` in the popup and exits.

**UC5. Install.**
- **Actor / trigger:** `install.sh` or `fleet update` on any host where `install.tools.fleet` is
  on.
- **Flow:** the `fleet` plugin is copied to `~/.config/herdr/plugins/local/fleet/`, linked with
  `herdr plugin link`, and its key is rendered into the managed `config.toml`.
- **Acceptance:** a re-run with an unchanged plugin does nothing.
  `gff set install.herdr-plugin.fleet false` installs nothing and binds no key.

## 3. Architecture

| Unit | Responsibility | Consumed by | Depends on |
| :-- | :-- | :-- | :-- |
| `sdk/fleet/internal/herdrmach` | `Machine`; pure `Parse([]byte)`; `Saved(ms, alias)`; `AddArgv(alias)`; `List(ctx, Runner)` (local exec through a one-method interface) | `cmd` (TUI + CLI); later the `fleet-connect` herdr provider | `encoding/json` only. The runner interface is local to the package so tests use a fake. |
| `sdk/fleet/cmd/herdr.go` | `fleet herdr sync` (cobra), result classification, `--dry-run`/`--json` | operator, plugin | `herdrmach`, `sshconf`, `isLocalHost` |
| `sdk/fleet/cmd/tui_*` | the `R` key, its target walk, the `ExecProcess` handoff, status lines | operator | `herdrmach`, `updateTargets()`, `inFlight()` |
| `sdk/fleet/pkg/provider` | `ReservedKeys['R'] = true` | providers | — |
| `ai/herdr/plugins/fleet/` | `herdr-plugin.toml` + `scripts/open-fleet.sh`, `scripts/sync-machines.sh` (POSIX sh) | herdr | `fleet` on PATH |
| `ai/herdr/plugins/fleet.toml` | `prefix+shift+h` → `fleet.open-fleet` | `install_herdr.sh config` | — |
| `opt/scripts/system/install_herdr.sh` | repo-owned plugin row: copy, link, idempotence | `install.sh` | `ai/herdr/plugins.tsv`, gff |

**Data flow (UC1):** `R` → `updateTargets()` → filter (local, in-flight, saved via
`herdrmach.List`) → for each target `tea.ExecProcess(herdr machine add …)` → `execDoneMsg` →
next target or final status line. `List` runs once per key press, not once per host.

**Interfaces (frozen for the build):**

```go
package herdrmach

type Machine struct {
    ID, Label, Target, Session string
    Enabled, Selected          bool
}
type Runner interface { Output(ctx context.Context, argv []string) ([]byte, error) }

func Parse(b []byte) ([]Machine, error)              // unknown fields ignored; non-array → error
func List(ctx context.Context, r Runner) ([]Machine, error)
func Saved(ms []Machine, alias string) (Machine, bool) // match on Target
func AddArgv(alias string) []string                   // herdr machine add <alias> --label <alias>
```

## 4. Behavior / features

- **F1. Read saved machines.** `herdrmach.Parse`/`List` over `herdr machine list --json` (shape
  verified on herdr 0.9.3).
- **F2. Recognise a saved host.** `Saved` matches the ssh alias against `Target`, so a machine
  renamed in herdr still counts.
- **F3. Build the add command.** `AddArgv` returns separate argv elements and never goes through
  a shell.
- **F4. `R` in fleet tui.** Selection-or-cursor targets. Skips the local host, hosts being
  updated, and saved hosts, each with a reason. Serial `ExecProcess` handoffs. Status lines
  depend on whether `HERDR_ENV` is set. No local herdr gives a message and no handoff.
- **F5. `fleet herdr sync`.** Defaults to every in-fleet, non-local host. Per-host result
  classes, TTY-aware (no handoff without a TTY), `--dry-run`, `--json`, and a non-zero exit when
  any host is `needs-approval` or `failed`.
- **F6. `R` is reserved.** Present in `provider.ReservedKeys` and in `keyHelp`.
- **F7. The herdr plugin.** Two actions, one key, a clear message when fleet is missing.
- **F8. Installer support.** Copy, link and bind, idempotent, gff-gated, on by default.

## 5. Evaluation criteria (per feature)

Format: **trigger · fires · must-not-fire · edge · pass**.

- **F1a** captured 0.9.3 `machine list --json` · returns one `Machine` with every field set ·
  — · an empty array returns an empty slice and no error · `TestParseCapturedMachineList`.
- **F1b** JSON with an extra unknown field · it parses · — · a non-array top level (an object or
  an error string) returns an error, never an empty list that would look like "nothing saved" ·
  `TestParseRejectsNonArrayAndToleratesNewFields`.
- **F1c** `List` with a fake runner returning an error · the error is wrapped with the argv ·
  must not return an empty list · — · `TestListSurfacesRunnerErrors`.
- **F2a** a machine with `Target=<a>`, `Label=renamed` · `Saved(ms, "<a>")` is true · `Saved`
  must not match on label alone (`Label=<a>`, `Target=<b>` → false) · — ·
  `TestSavedMatchesTargetNotLabel`.
- **F3a** an alias containing `; rm -rf ~` · `AddArgv` returns it as one element ·
  must not produce a `sh -c` string · — · `TestAddArgvIsInertToMetacharacters`.
- **F4a** `R` on a plain cursor host, not saved · exactly one `ExecProcess` with `AddArgv(alias)`
  · must not fire in `modeSearch`/`modeAnswers`/`modeConfirm` (there `R` is text) · — ·
  `TestRHandsOffMachineAddForTheCursorHost`.
- **F4b** a selection of local + in-flight + saved + two unsaved hosts · two handoffs, in
  selection order · no handoff for the other three, each named in the status line with its
  reason · selection empty → the cursor host · `TestRWalksTheSelectionAndSkipsWithReasons`.
- **F4c** handoff completes with `HERDR_ENV=1` · the status line contains `prefix+w` · without
  `HERDR_ENV` it contains `herdr --remote <alias>` · a non-nil handoff error shows herdr's error
  and still advances to the next host · `TestRStatusLineDependsOnHerdrEnv`.
- **F4d** no local herdr (`LookPath` fails) · status line `herdr is not installed here` ·
  no handoff, no `List` call · — · `TestRWithoutLocalHerdrIsAMessage`.
- **F4e** `List` fails · the status line says machines can't be read · must not hand off (we
  can't tell what's saved, and a duplicate add would prompt) · — ·
  `TestRRefusesWhenMachineListFails`.
- **F5a** `fleet herdr sync --dry-run` over 3 in-fleet hosts (1 local, 1 saved) · prints one
  `herdr machine add` line for the unsaved host · runs nothing · — · `TestSyncDryRun`.
- **F5b** no TTY, one unsaved host · result `needs-approval`, the output names
  `herdr machine add <alias>`, exit code ≠ 0 · must not execute the add · — ·
  `TestSyncWithoutTTYReportsNeedsApproval`.
- **F5c** `--json` · an array of `{host, result, detail}` with results from
  `already|saved|needs-approval|failed` · — · explicit host args limit the set; an unknown alias
  returns `failed` with "not in ssh config" · `TestSyncJSONShapeAndExplicitHosts`.
- **F6a** the fleet keymap · `R` is in `provider.ReservedKeys` and `keyHelp` · — · — ·
  `TestEveryFleetKeyIsReservedAgainstProviders` (existing; extended by the new binding).
- **F7a** `scripts/open-fleet.sh` with `fleet` absent from PATH · prints
  `fleet is not installed`, exits 0 · must not leave a blank popup · — · shell test
  `herdr_plugin_fleet_test.sh`.
- **F7b** `herdr-plugin.toml` · declares exactly the `open-fleet` and `sync-machines` actions,
  with no `[[build]]` and no network calls in any script · — · `shellcheck` and
  `make lint-portability` are clean · same test.
- **F8a** a fresh `$HOME` (temp) · the plugin copy exists, `herdr plugin link` is called once
  (stub) · — · a re-run with identical files makes no copy and no link call ·
  `install_herdr_test.sh` cases.
- **F8b** `install.herdr-plugin.fleet=false` · no copy, no link, no key in `config.toml` · — ·
  `install.tools.fleet=false` also disables it · `install_herdr_test.sh`.

## 6. Verification harness

- **Go:** `go test ./...` in `sdk/fleet`. The coverage bar for `sdk/` (≥60% per package) applies
  to `herdrmach`, aiming high because it is pure. fleet's TUI has no seam for recording
  handoffs today. Its tests assert named argv builders instead (`configVerbArgs`,
  `authorizeArgs`). So the build adds one injectable field on the model, e.g.
  `herdrHandoff func(argv []string) tea.Cmd`, defaulting to `tea.ExecProcess`. Tests drive
  `Update()` with key messages and record the argv each handoff received.
- **Shell:** `herdr_plugin_fleet_test.sh` (new, beside the plugin) and the extended
  `install_herdr_test.sh`, both run in `make` / the shell-test CI job; `shellcheck`,
  `make lint-portability`.
- **Fixtures:** `herdrmach/testdata/machine-list.json` captured from herdr 0.9.3, with the herdr
  version in a provenance header (the same practice `fleet-connect` uses).
- **Human-evidenced (operator):** `fleet herdr sync` live on a host with no saved machine
  (`saved`), then on nano (`already`). A screenshot of the host in the herdr sidebar.
  `prefix+shift+h` → `R` → the host appears. Captured in `plans/fleet-herdr-machines/evidence/`.

## 7. Prerequisites / dependencies

- [#370](https://github.com/sfc-gh-eraigosa/dotfiles/pull/370) (merged): `~/.local/bin/herdr`
  links to the managed binary, so `machine add` finds it. Each target host needs a `fleet update`
  after #370 before `R`/`sync` work without an install prompt.
- herdr ≥ 0.9.x with `machine add/list`. Local herdr on the operator's machine.
- No dependency on `fleet-connect` #290–#296. That work later *consumes* `herdrmach`.

## 8. Out of scope (and why)

- **Switching the herdr view to the machine.** herdr has no API for it; `prefix+w` was accepted
  (design §4.5).
- **Removing machines** for hosts that leave the fleet. It's additive-only in v1, so a mistake
  cannot delete anything.
- **Restarting or updating remote herdr servers.** That ends running agents; it stays herdr's
  prompt and the operator's choice.
- **A herdr column or row state in fleet tui.** That belongs to `fleet-connect`'s herdr
  provider.

## 9. Rollback

- Revert the build PR: `R`, `fleet herdr sync` and `herdrmach` go away. Nothing else depends on
  them until `fleet-connect` adopts `herdrmach`.
- `gff set install.herdr-plugin.fleet false` and `herdr plugin unlink fleet` remove the plugin
  from a host.
- Saved machines remain in herdr. `herdr machine remove <id>` deletes one; remote servers keep
  running.

> Produced from the design review with the operator (2026-10-06). The matching plan goes in
> `../plans/fleet-herdr-machines.md` when the build is scheduled. Registered in `../index.md`.
