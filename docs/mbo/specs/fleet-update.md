# fleet-update — config-driven, multi-repo update DAG for the fleet CLI — spec

- **Slug:** fleet-update
- **Date:** 2026-09-02
- **Status:** Draft
- **Relates to:** issue [#265](https://github.com/sfc-gh-eraigosa/dotfiles/issues/265) / PR [#270](https://github.com/sfc-gh-eraigosa/dotfiles/pull/270) / design [`../designs/fleet-update.md`](../designs/fleet-update.md)

## 1. Goal

`fleet update` reads a per-user YAML plan (`~/.config/fleet/fleet.yaml`, `update:` section) that
names a GitHub home folder, any number of repos under it, and a small dependency graph of steps
(`sync` a repo to one or more branches, `run` a command in it, `gh-auth` bootstrap GitHub
credentials). The controller walks the graph on each host over SSH, skips dependents of a failed
step, retries transient failures with backoff under per-attempt timeouts, preserves and restores
local state on the host according to a per-repo policy, and reports one line per step. With no
file present, behaviour is byte-for-byte today's `fleet update`.

## 2. Use cases

**UC-1 — Update the fleet exactly as today (no plan file).**
Actor: operator. Trigger: `fleet update <host>...`. Flow: no `fleet.yaml` exists → built-in
plan (`~/git/dotfiles`, `main`, `./install.sh` interactive) → one SSH precheck, one sync, one
interactive install per host. Acceptance: the remote sync script is identical to today's minus
`&& ./install.sh`; `--ref feature/x` and `--force` behave exactly as before; exit code and
`N host(s) not updated` message unchanged.

**UC-2 — Several repos, one of them script-free.**
Actor: operator with `dotfiles`, `work/scripts`, `tools/bin` clones on every host. Trigger:
`fleet update init`, edit repos and steps, `fleet update <host>`. Flow: `dotfiles.sync →
dotfiles.install`, `scripts.sync → scripts.make`, `tools.sync` (no run step). Acceptance: each
repo is fetched exactly once; `tools` is moved to its remote default branch with no command run;
the report shows every step with status, exit, duration and notes.

**UC-3 — Success criteria and continuation.**
Trigger: `scripts.make` exits 2. Flow: `scripts.make` is `failed`; its dependents are
`dependency-failed "blocked by scripts.make"`; the `dotfiles.*` chain still runs to completion.
With `on_failure: continue` on `scripts.make`, its dependents run but the host is still reported
as not updated. Acceptance: exactly that cascade, visible in the report and the run log.

**UC-4 — GitHub credentials on a fresh host.**
Trigger: a `gh-auth` step precedes a private repo's sync. Flow: batch `gh auth status` →
passes: done, zero prompts; fails: interactive `gh auth login --web` + `setup-git` over `ssh -t`,
then re-check. Acceptance: already-authenticated hosts make no interactive call; no token,
`GH_TOKEN`, `GITHUB_TOKEN`, or `--with-token` ever appears in any remote string; `gh` missing
(exit 127) is reported as "gh not installed", not as an auth failure.

**UC-5 — Host on another branch / with local changes.**
Trigger: host has `dotfiles` checked out on `feature/x` with a tracked edit and an untracked
file. Flow under `local: skip`: SKIP, dependents blocked. Under `local: carry`: `stash push -u`
(SHA captured), switch to target, ff, steps run, then `checkout feature/x` + `stash apply
<sha>` + drop. Under any policy with a *clean* off-branch checkout: switch, steps, switch back.
Acceptance: after a carry run the host is on `feature/x` with both changes present and
`git stash list` empty; on an apply conflict the stash is kept and its SHA named; with
`restore: false` the host stays on the target.

**UC-6 — Flaky link and hung install.**
Trigger: SSH exits 255 during `scripts.sync`; `dotfiles.install` hangs. Flow: `scripts.sync`
retries with `5s, 10s` backoff (attempts 3), then succeeds; `dotfiles.install` (batch, timeout
30m) is killed at the deadline, reported `timed out after 30m`, retried only if `timeout` is in
`retry.on`. Acceptance: the report reads `attempt 3/3` / `timeout`; the run log shows each
attempt header; dependents see only the final outcome.

**UC-7 — Switch the plan source via gff.**
Trigger: `gff set fleet.update.config repo`. Flow: `fleet update` reads
`<--repo>/opt/etc/fleet/fleet.yaml`. `gff set fleet.update.enabled false` → built-in plan
regardless of files. Acceptance: the first output line names the plan source; with gff missing
or erroring the tool behaves as enabled + `home`.

## 3. Architecture

```
~/.config/fleet/fleet.yaml ──► updplan.Parse ──► Plan (Order, Dependents, WithRef)
                                                     │
featflag.Resolve(gff|Static) ──► which file / enabled │
                                                     ▼
                cmd/update.go ──► updexec.Executor.RunHost(host, plan)
                                     │   per step: attempts × (deadline ctx) → StepIO
                                     │   Console (CLI: RunStreamCtx / RunInteractive)
                                     │   Background (TUI: RunStreamCtx; Interactive → ErrNoTerminal)
                                     ▼
                              runner.Runner (the only remote seam; +RunStreamCtx)
```

- `internal/updplan` — pure. Types, YAML decode with `KnownFields(true)`, defaults merge
  (`update.defaults` → step, field by field), validation with aggregated errors, allowlisted
  charsets, stable Kahn order, transitive dependents, `WithRef`.
- `internal/updexec` — pure script builders (every builder re-validates its inputs) plus the
  executor. Impure edges injected: `StepIO`, `Output`, `Now`, `Sleep`, `Rand`.
- `internal/runner` — gains `RunStreamCtx(ctx, …)`; `Exec` uses `exec.CommandContext`, `Fake`
  honours `ctx.Done()`; `RunStream` delegates with `context.Background()`.
- `internal/featflag` — `Source` interface, fail-open `Resolve`, `Static` test double, and the
  one file that imports `sdk/gff/pkg/gff` (`WithSource(namespace)` first, unscoped on
  `ErrUnknownSource`).
- `cmd` — `loadPlan` (flag → gff → default), `runUpdate`, `fleet update init`, per-host capture
  via `applog.NewCapture`, report/JSON/dry-run. Later the TUI lanes.

## 4. Behavior / features

### F1 Plan file schema

```yaml
version: 1
update:
  root: ~/git
  defaults:
    timeout: 30m
    retry: { attempts: 1, on: [transport], backoff: { initial: 5s, factor: 2, max: 2m, jitter: true } }
  repos:
    <name>:                    # ^[a-z0-9][a-z0-9._-]*$
      path: <rel|abs|~/…>      # default = name; relative → <root>/<path>; charset [A-Za-z0-9._/-], optional leading ~/, no "..", no leading "-"
      url: <https://|ssh://|git@…>   # optional; enables clone when path is missing
      branches: [default]      # "default" (remote HEAD) only first; rest ValidRef, unique; multi-entry lists are BRANCHES (a tag works only as the sole entry — not syntactically checkable, so documented rather than validated)
      local: skip|rescue|carry # default skip
      restore: true|false      # default true
  steps:
    - id: <id>                 # ^[a-z0-9][a-z0-9._-]*$, unique
      kind: sync|run|gh-auth
      repo: <name>             # required for sync; optional for run; forbidden for gh-auth
      run: <shell>             # run only; required; no NUL/newline; otherwise verbatim
      interactive: true|false  # run only; sync/gh-auth reject it
      hostname: github.com     # gh-auth only
      needs: [<id>…]           # existing ids, not self, acyclic
      expect: { exit: [0] }    # each 0..255; default [0]
      on_failure: stop|continue
      timeout: <duration>      # 0 = none; interactive steps default to 0
      retry: { attempts: <n≥1>, on: [transport|timeout|any|<exit code>…], backoff: {…} }   # merges over defaults; forbidden on interactive steps
```

Built-in default (`Default()` == `Parse(DefaultYAML)`): root `~/git`; repo `dotfiles`
(`path: dotfiles`, `branches: [main]`, `local: skip`, `restore: true`); steps `dotfiles.sync` →
`dotfiles.install` (`./install.sh`, interactive, needs `dotfiles.sync`).

### F2 Plan resolution
`--file` (must exist) → `featflag.Settings.Enabled == false` ⇒ built-in → configured location
(`home` = `$XDG_CONFIG_HOME/fleet/fleet.yaml` or `~/.config/fleet/fleet.yaml`; `repo` =
`<--repo>/opt/etc/fleet/fleet.yaml`) → file missing ⇒ built-in with `Source = "built-in default
(no <path>)"`. A present file must be owned by the current uid and not group/world-writable.

### F3 Sync step
Precheck (read-only) → `state=<missing|clean|dirty|in-progress> branch=<name|detached>`.
`missing` + `url` → clone (the clone is the one network call); `missing` without `url` →
failed. `in-progress` → skipped under every policy. `dirty` → `skip` ⇒ skipped; `rescue` ⇒
rescue script (today's, parameterised) then sync; `carry` ⇒ sync with the carry prologue.
Sync script = prologue (record `orig`, optional stash) + body (single-branch = today's string;
multi-branch = one `git fetch origin b1 b2 …`, checkout b1, `merge --ff-only origin/b1`,
extras ff'd only when local and ancestor, else `skipped(diverged)`; `default` = local symref,
then `ls-remote --symref` fallback + `set-head`) + epilogue (echo `switched a -> b` if moved).

### F4 Restore
Synthesized `<repo>.restore` after the last step in `Order()` that references the repo (or
immediately when the sync failed after switching/stashing). Runs unless nothing was
switched/carried or `restore: false` / `--no-restore`. `checkout <orig>`; if a stash was
carried, `stash apply <sha>` then `stash drop <sha>` only after a clean apply; on any failure
the stash is kept and the report names `stash=<sha> branch=<orig>`. `orig` and `sha` are the
only remote-originated values ever interpolated and are validated (`ValidRef` / 40 hex) first.
Fixed policy: `attempts: 3, on: [transport], timeout: 5m`.

### F5 Run step
`cd <path> && <run>` (or `<run>` with no repo). Batch or interactive (`ssh -t`). The CLI lane
exports `WINSETUP_ANSWER` / `GEMINI_TEARDOWN_ANSWER` from the local environment into run steps
only; the TUI lane prefixes the sudo `-S` preamble on run steps only. Never the secret in argv/env.

### F6 gh-auth step
`command -v gh || exit 127; gh auth status -h <host>` (batch, retryable on transport) → on
failure `gh auth login -h <host> --web --git-protocol https && gh auth setup-git -h <host>`
(interactive, never retried, `ErrNoTerminal` in the TUI lane ⇒ failed "needs a terminal") →
one re-check.

### F7 Executor semantics
Stable topological order; a step whose need is not `ok` and whose need's `on_failure` is
`stop` (or is itself `dependency-failed`) is `dependency-failed "blocked by <id>"`; siblings
continue. `ok` iff exit ∈ `expect.exit`. Each attempt under `context.WithTimeout(step.Timeout)`
(0 = none); retry when the failure class matches `retry.on` and attempts remain; wait
`min(max, initial × factor^(n-1))` ± 50% jitter via injected `Sleep`/`Rand`; interactive steps
never auto-retry. `Result{Status, Exit, Duration, Reason, Notes, Attempts, TimedOut}` per step;
`HostReport.Failed()` if any step is not `ok`.

### F8 CLI
`fleet update <host>... [--local skip|rescue|carry] [--force] [--no-restore] [--reset] [--timeout D] [--no-retry] [--ref B|repo=B]... [--file PATH] [--dry-run]` (`--json` from root);
`fleet update init [--file PATH] [--overwrite] [--print]`. `--force` ≡ `--local rescue`;
`--ref` default empty (= the plan's branches); `--reset` incompatible with `carry`.
Report: first line names the plan source; per host `=== host ===` then one line per step
`ok|FAIL|skip|dep-fail <id> [exit N] [attempt a/n] [timeout] <dur> <notes|reason>` then
`log: <capture path>`; `--dry-run` prints every effective script + timeout/retry and sends nothing.

### F9 gff flags
`fleet.update.enabled` (bool, default true) and `fleet.update.config` (single choice: `home`
selected, `repo`) in `.github/gff/features.yaml`, area `fleet`. Fail-open on every error.

### F10 Headless run log
Every CLI run tees each host's output to `$XDG_STATE_HOME/fleet/logs/` through
`applog.NewCapture` with header `fleet update — host=H plan=<Source> mode=<fast-forward|FORCE
RESET> started=<RFC3339>` and per-attempt `=== step <id> attempt a/n ===` markers. A capture
that cannot be opened never costs the update.

### F11 TUI (leaf E)
`u` runs the same executor in the `Background` lane; a host that hits `ErrNoTerminal`
(interactive run step or gh login) is routed to the interactive queue, which now self-execs
`fleet update <alias> …` (answers via env, never the secret). `--update-ref` maps to `WithRef`;
`--file` is accepted.

## 5. Evaluation criteria (per feature)

| Feature | Trigger predicate | Fires | Must-not-fire | Edge | Pass = test |
| :-- | :-- | :-- | :-- | :-- | :-- |
| F1 defaults | no file / disabled | built-in plan | any other repo/step | `DefaultYAML` round-trips | `TestDefaultPlanIsTodaysUpdate`, `TestDefaultYAMLRoundTripsToDefault` |
| F1 validation | any invalid field | aggregated error naming step/field | silent acceptance (`KnownFields`) | two faults → both named | `TestParseRejects`, `TestParseAggregatesEveryError` |
| F1 retry/timeout merge | step overrides one field | other fields inherited | interactive gets a default timeout | explicit timeout on interactive kept | `TestStepInheritsDefaultsFieldByField`, `TestInteractiveStepsDefaultToNoTimeout` |
| F2 | flag / gff / missing file | correct source, named in output | reading a world-writable file | gff unavailable ⇒ enabled+home | `TestLoadPlan*`, `TestResolveDefaultsWhenSourceErrors` |
| F3 one fetch | every sync form | fetch+clone == 1, no pull | `ls-remote` before the `\|\|` | default-branch fallback ≤ 1 | `TestUpdateMakesExactlyOneNetworkCall` (moved), `TestEverySyncFormMakesAtMostOneUnconditionalNetworkCall` |
| F3 byte-compat | single branch, skip/rescue | today's exact string | any extra token | — | `TestSyncScriptSingleBranchMatchesTodaysForm` |
| F3 multi-branch | ≥2 branches | extras ff only when ancestor | moving a diverged branch or `$b1` | missing local branch → created | `TestMultiBranchFetchesAllInOneCall`, `TestExtrasOnlyForceMoveAnAncestor` |
| F3 local policy | dirty/off-branch | skip / rescue / carry as table | dropping work | in-progress merge ⇒ skip under all | `TestUpdateSkipsDirtyCloneByDefault`, `TestForceRescuesDirtyWorkBeforePulling`, `TestCarryStashesWithUntrackedAndCapturesTheSHA` |
| F4 restore | switched or carried | restore after last repo step, even after failure | restore when on target / `restore:false` | conflict keeps stash | `TestCleanOffBranchIsRestoredUnderEveryPolicy`, `TestRestoreRunsEvenWhenAnIntermediateStepFailed`, `TestRestoreConflictKeepsTheStash`, `TestRestoreRejectsUnvalidatedOrigOrSHA` |
| F5 | run step | `cd P && RUN` verbatim | preamble/env on sync or gh steps | no repo ⇒ no `cd` | `TestRunScriptIsVerbatimAfterCd`, `TestPreambleAndStdinApplyToRunStepsOnly` |
| F6 | gh-auth | check → login only on miss → re-check | any token string; login retry | 127 ⇒ not installed; no tty ⇒ failed cleanly | `TestGhAuthSkipsLoginWhenStatusPasses`, `TestGhAuthNeverCarriesAToken`, `TestGhAuthWithoutATerminalFailsCleanly` |
| F7 cascade | failed/skipped need | dependents dep-failed, siblings run | blocking after `continue` | dep-failed also blocks | `TestFailedStepSkipsTransitiveDependents`, `TestOnFailureContinueLetsDependentsRunButStillFailsTheHost` |
| F7 retry | matching failure class | whole-script rerun, logged backoff | expected exit retried; interactive retried | exhaustion ⇒ on_failure | `TestTransportFailureIsRetriedWithBackoff`, `TestExpectedExitIsNeverRetried`, `TestInteractiveStepsAreNeverRetried`, `TestBackoffScheduleIsExponentialAndCapped` |
| F7 timeout | deadline passes | attempt cancelled, `TimedOut` | interactive deadline without explicit timeout | retried only if listed | `TestTimeoutCancelsTheAttempt`, `TestInteractiveHasNoDeadlineUnlessSet`, `TestRunStreamCtxKillsTheChildOnDeadline` |
| F8 | CLI flags | report lines, exit code, dry-run sends nothing | `--reset` with carry | `--force` ≡ rescue | `TestReportNamesEveryStepAndTheLog`, `TestDryRunSendsNothing`, `TestForceIsAnAliasForLocalRescue` |
| F9 | gff present/absent | selected location; disabled ⇒ built-in | failing closed | unknown key ⇒ fail-open | `TestResolveHonoursDisabled`, `TestResolveUnknownKeyIsFailOpen`, `gff lint` |
| F10 | any CLI run | capture with header + attempt markers | secret in capture | unusable dir ⇒ no capture, update proceeds | `TestHeadlessUpdateIsCaptured`, `TestAttemptHeaderIsWrittenToTheCapture` |
| F11 | TUI `u` | Background lane; no-terminal ⇒ interactive queue | secret in handoff env | — | `TestNeedsTerminalRoutesToInteractiveQueue`, `TestHandoffEnvNeverCarriesTheSecret` |

## 6. Verification harness

- **Unit** (`cd sdk/fleet && gofmt -l . && go vet ./... && go test -race -cover ./...`): new
  packages `updplan`, `updexec`, `featflag` ≥ 90 % each; `cmd` not below its current figure;
  repo floor `fleet=60` in `scripts/test.sh` unchanged. No test opens a socket, reads `$HOME`
  (`t.Setenv("HOME", t.TempDir())`), or touches `~/.config/gff`. Executor tests use a scripted
  `fakeIO` (substring-keyed, per-step sequences, `ctx.Done()` aware) plus injected clock/sleep.
- **gff**: `cd sdk/gff && go run . lint ../../.github/gff/features.yaml` clean.
- **Human-evidenced gates** (captured under `plans/fleet-update/evidence/e2e/`): G1 no-file
  `--dry-run` equals today's commands; G2 two-repo + gh-auth live run with a failing `make`
  showing the cascade; G3 gh-auth on an authenticated host makes zero interactive calls; G4
  `gff set fleet.update.enabled false` ⇒ built-in source; G5 TUI with one background + one
  interactive host; G6 clean feature-branch round-trip; G7 carry round-trip (tracked + untracked
  restored, stash list empty) — this is what licenses amending the "never stash" invariant; G8
  carry conflict keeps the stash; G9 forced transport failure retried with logged backoff.

## 7. Prerequisites / dependencies

`gopkg.in/yaml.v3`; `github.com/sfc-gh-eraigosa/dotfiles/sdk/gff` via `replace ../gff` (mirrors
`../libs`); `sdk/libs/log` for captures; Go 1.26; git ≥ 2.23 on hosts (`stash push -u`,
`symbolic-ref --short`, `merge-base --is-ancestor`); a POSIX login shell on every host (already
assumed by the status probe).

## 8. Out of scope (and why)

- Workstation-local execution and host-side `fleet apply` — different trust and bootstrap story.
- Parallel steps within a host — the step graph is small; serial keeps the sudo/tty story simple.
- Cross-host ordering, an `--all` selector, token forwarding, credential storage (see design §2).
- A free-form gff path — not expressible in gff's closed choice sets; `--file` covers it.

## 9. Rollback

Reverting leaf D restores today's `update.go`; `gff set fleet.update.enabled false` or deleting
`fleet.yaml` pins the built-in plan at runtime. Leaves A/B/C are additive packages.

> Produced via `superpowers:brainstorming` (mbo-plan). The matching plan goes in
> `../plans/fleet-update.md`. Register / update `../index.md`.
