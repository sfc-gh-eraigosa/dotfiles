# fleet-update — config-driven, multi-repo update DAG for the fleet CLI — design

- **Slug:** fleet-update
- **Date:** 2026-09-02
- **Status:** Proposed
- **Relates to:** issue [#265](https://github.com/sfc-gh-eraigosa/dotfiles/issues/265) / PR [#270](https://github.com/sfc-gh-eraigosa/dotfiles/pull/270) · parent objective [`fleet`](./fleet.md) (#222) · siblings [`fleet-config`](./fleet-config.md), [`fleet-tui`](./fleet-tui.md)
- **Author(s):** repo owner (via Claude)

## 1. Problem / context

`fleet update <host>...` is one hardcoded remote script (`sdk/fleet/cmd/update.go`):

```
cd ~/git/dotfiles && git fetch origin <ref> && git checkout <ref> && git merge --ff-only FETCH_HEAD && ./install.sh
```

One repo, one branch, one command, one path. Verified gaps:

1. **More than one repo.** The fleet carries several clones per host. Some only need "move to
   the remote's default branch" with no script at all; others need a different command
   (`make install`, another `.sh`, a binary). Today every one of them is out of reach of `fleet`.
2. **Dependencies between updates.** `gh` must be authenticated before a private repo can be
   fetched; `install.sh` must finish before a tool build that depends on it. There is no way to
   express "B after A, only if A succeeded".
3. **No GitHub credential bootstrap** anywhere in the repo (grep of `install.sh`, `opt/`, `sdk/`:
   only read-only `gh auth status` checks; `gss` deliberately never triggers a login).
4. **Local state on the host is handled bluntly.** A dirty clone is skipped or, with `--force`,
   committed aside into a rescue worktree and the host is left on the target branch. A clean
   clone sitting on a feature branch is silently switched to `main` and never switched back.
5. **No retry, no timeout.** A transient SSH drop fails the host; a hung install hangs `fleet`.

Constraints inherited from the existing tool (each pinned by a test in `sdk/fleet/AGENTS.md`):
exactly one `git fetch` per update, never `git pull`, never `git stash` as a rescue vehicle
(`branch <n> stash@{0}` loses untracked files), the sudo secret travels only over stdin,
`internal/runner` is the only impure seam, `time.Now()` never runs in pure paths. Naming
constraint: `fleet config` / `--config` / `internal/cfgplan` already mean **ssh-config
transfer** (slug `fleet-config`), so the new file cannot be called "config".

## 2. Goals & non-goals

**Goals**
- A declarative, per-user **update plan** (`~/.config/fleet/fleet.yaml`, `update:` section)
  naming a GitHub home folder, N repos (path under that root, optional clone URL, one or more
  branches, `default` = remote HEAD), and a small **DAG of steps** (`sync`, `run`, `gh-auth`)
  with success criteria and a failure policy.
- **Byte-for-byte compatibility** when no file exists: the built-in plan is today's update.
  `--ref` and `--force` keep working.
- **Controller-driven execution**: the workstation walks the DAG and issues one SSH command per
  step per host over the existing runner; hosts need nothing installed.
- **gh credential bootstrap** as a step kind that checks first and only prompts (web login over
  `ssh -t`) when the check fails. Never forwards a token.
- **Local-state policy per repo** (`skip` / `rescue` / `carry`) and **branch restore** so a host
  that was on another branch is put back after its steps, under every policy.
- **Retries with backoff and per-attempt timeouts**, settable as defaults and per step.
- **gff gating**: a fail-open kill switch and a location selector for the plan file.

**Non-goals (v1)**
- Running the plan on the workstation itself (`--local`); a host-side `fleet apply`.
- Parallel steps within one host (hosts already fan out; steps stay serial per host).
- Cross-host dependencies ("host B after host A").
- A `--all` host selector (explicit hosts as today; `selectHosts` exists for a later follow-up).
- Token forwarding or any credential storage.

## 3. Options considered

**Execution model**
1. **Controller drives each step over SSH (chosen).** One multiplexed SSH command per step; the
   controller owns ordering, skip propagation, retries, timeouts, and the run log. Hosts stay
   dumb, so the first update of a brand-new host works. Cost: N round trips per host (cheap with
   `ControlMaster`; measured 0.016s warm per command in the `fleet` design).
2. Render the DAG to one POSIX script per host and run it in a single `ssh -t`. Fewer round
   trips, but per-step status must be parsed out of a stream, interactive steps cannot be
   isolated, and a timeout can only kill the whole script.
3. Push the YAML and run `fleet apply` on the host. Cleanest offline story, but requires a
   current `fleet` binary on every host before the first update (chicken-and-egg).

**gh credential bootstrap**
1. Forward the workstation's token over stdin (`gh auth login --with-token`). Fully automatic,
   but copies one credential onto every host. Rejected by the user.
2. **Check first, interactive web login only on a miss (chosen).** Each host gets its own
   credential; a human confirms a device code once per host; already-authenticated hosts cost
   one batch command.
3. Defer entirely. Rejected: the check-first form is small and the user asked for it.

**Where the plan file's path lives**
1. A gff string flag. Not expressible: gff choice flags are closed option sets (`gff set` rejects
   an id that is not declared — `resolve.ErrUnknownOption`).
2. **A gff choice between two locations + `--file` for anything else (chosen).** `home`
   (`~/.config/fleet/fleet.yaml`, default) or `repo` (`<dotfiles clone>/opt/etc/fleet/fleet.yaml`,
   a plan under version control).
3. No gff at all. Rejected: the user wants the same switchboard that gates `install.sh`.

**Timeouts**
1. Remote `timeout(1)`. Missing on macOS by default.
2. **Local `exec.CommandContext` around the SSH child (chosen).** Portable; the remote receives
   SIGHUP when the session drops, the same as a dropped connection today.

## 4. Decision

Four units, each independently testable, in build order:

| Unit | Purpose | Depends on |
| :-- | :-- | :-- |
| `internal/updplan` (pure) | YAML schema, defaults, validation (aggregated errors, allowlisted charsets), topological order, `--ref` shim, defaults merge for retry/timeout | — |
| `internal/updexec` (pure builders + executor) | exact remote scripts (`precheck`, `sync` = prologue+body+epilogue, `clone`, `rescue`, `reset`, `restore`, `run`, `gh-auth`), the per-host executor (attempts, backoff, deadline, skip cascade, synthesized restore), two lanes (`Console` for the CLI, `Background` for the TUI) | `updplan`, `runner` (+ new `RunStreamCtx`) |
| `internal/featflag` | `Source` interface + fail-open `Resolve`; the only file importing the gff SDK | gff SDK |
| `cmd` | `fleet update` rewired onto the executor, `fleet update init`, headless run log, report/JSON/dry-run; later the TUI lanes | all of the above |

Shape of the plan file (full schema in the spec):

```yaml
update:
  root: ~/git
  defaults: { timeout: 30m, retry: { attempts: 1, on: [transport], backoff: { initial: 5s, factor: 2, max: 2m, jitter: true } } }
  repos:
    dotfiles: { path: dotfiles, url: https://github.com/<owner>/dotfiles.git, branches: [default], local: skip, restore: true }
  steps:
    - { id: gh-auth, kind: gh-auth }
    - { id: dotfiles.sync, kind: sync, repo: dotfiles, needs: [gh-auth], retry: { attempts: 3, on: [transport, timeout] } }
    - { id: dotfiles.install, kind: run, repo: dotfiles, run: ./install.sh, interactive: true, needs: [dotfiles.sync] }
```

Key behaviours:
- **Steps run serially per host in a stable topological order**; a failed or skipped step marks
  its transitive dependents `dependency-failed` unless it declares `on_failure: continue`;
  independent branches keep going. The host counts as "not updated" if any step is not `ok`.
- **Local state**: the precheck reports `state ∈ {missing, clean, dirty, in-progress}` and the
  current branch, read-only. `local:` picks `skip` (default, today's behaviour), `rescue`
  (today's `--force`), or `carry` (`git stash push -u`, SHA captured, `stash apply <sha>` on
  restore, never `pop`, never an index). `restore:` (default true, every policy) puts a switched
  host back on its original branch or detached SHA after the last step that uses the repo,
  even if a step failed; a restore conflict keeps the stash and names it.
- **Retries** re-run the whole idempotent step script; interactive steps never auto-retry; the
  synthesized restore step is the most retried (transport only). **Timeouts** are per attempt,
  enforced by the controller, none by default for interactive steps.
- **`run:` is verbatim by contract** (executable configuration). The plan file must be owned by
  the user and not group/world-writable; `--dry-run` prints every script as sent; every other
  interpolated field is allowlisted; the only remote-originated values ever interpolated
  (original branch, stash SHA for restore) are validated first.
- **gff is fail-open here**: any resolver error means "enabled, default location".

## 5. Risks & blast radius

- `run:` is arbitrary shell on every host — made explicit rather than pretending to sanitise
  (file ownership/mode check, dry-run, allowlists elsewhere). A shared `repo` plan is code
  review territory.
- The one-network-call invariant moves packages; it is re-asserted over a full recorded run so
  the executor cannot regress it by wrapping the script. `default` costs one extra `ls-remote`
  only until `origin/HEAD` is set once.
- Carry/restore can strand a host mid-state if the controller dies between switch and restore;
  the stash SHA and original branch are in the report and run log, the stash is never dropped
  before a clean apply, and a rerun simply syncs.
- A timeout kills the local `ssh`, not a `sudo` child on the host; defaults are conservative
  and the report marks `timeout` so the operator knows the host may still be busy.
- The gff SDK pulls protobuf + yaml into the binary; isolated behind one interface, size
  measured, replaceable by a `gff get` shell-out.
- TUI regression: the TUI lands last on top of the same executor; until then it keeps a thin
  `updateScript` wrapper.

## 6. Rollback

Every unit is additive until leaf D. Reverting D restores today's `update.go` verbatim (the
built-in plan *is* that script). `gff set fleet.update.enabled false` pins the built-in plan at
runtime without a rebuild. Removing `~/.config/fleet/fleet.yaml` has the same effect for one
machine.

## 7. Evidence expectations

- **Unit captures** per leaf under `plans/fleet-update/evidence/<leaf>/` (test run output with
  coverage; the moved invariant tests named in the commit).
- **Wire transcripts**: `fleet update <host> --dry-run` with no file (today's three commands)
  and with a two-repo + `gh-auth` plan.
- **Live runs on a real host**: a plain run; a failing `make install` showing `dep-fail` on its
  dependent while the dotfiles chain still completes; `gh-auth` on an already-authenticated
  host making zero interactive calls; a clean feature-branch round-trip (host back on its
  branch); a `carry` round-trip with a tracked edit and an untracked file (both present after,
  `git stash list` empty); a `carry` conflict (stash kept, SHA in the report); a forced
  transport failure retried with the logged backoff.
- **TUI transcript** with one background and one interactive host.
- **Binary size** before/after the gff import.

> Produced via `superpowers:brainstorming` (mbo-plan). Register the objective in `../index.md`.
> The matching spec goes in `../specs/fleet-update.md`.
