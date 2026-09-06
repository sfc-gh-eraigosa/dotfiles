# fleet — dotfiles install-status checker

`fleet` answers one question on demand — *"which of my hosts are out of sync with
the latest dotfiles install?"* — and manages the fleet's membership and SSH access
keys. It is never a daemon: you run it, it fans out over SSH, it prints, it exits.

It exists because `install.sh` used to leave **no record that it ran**. A host's
git clone could be current while the installer last ran weeks ago — "pulled" and
"installed" are different facts. `opt/scripts/system/install-stamp.sh` now records
the second one (`~/.local/state/dotfiles/install-stamp`), and `fleet` reads it.

## Install

`build.sh` compiles the binary and installs it to `~/opt/bin/fleet`:

```sh
cd sdk/fleet
bash build.sh
```

`build.sh` stamps version, commit, build date, and dirty flag via `-ldflags`
(injected by exact symbol path into `cmd.Version`/`Commit`/`Dirty`/`BuildDate`).
The repo's `install.sh` builds it for you via the same per-tool block as `gss`,
`gsl`, and `tmux-mgr`; `make bin` in this directory does the same.

## Inventory — where the host list comes from

`fleet` has no host file of its own. Its inventory **is** your `~/.ssh/config`: a
`Host` block is in the fleet when it carries a `#fleet` marker comment.

```sshconfig
Host web-01          # fleet
    HostName 10.0.0.21
    User deploy
```

Pattern hosts (`Host *.internal`) are skipped — only concrete aliases are polled.
Override the config path with `--config` and the marker with `--marker`.

Most of your hosts are probably in `~/.ssh/config` already. You don't re-declare
them — `fleet discover` shows which are adoptable and `fleet add <alias>` marks
one in place (see below), no hostname re-typing required.

## Subcommands

### `fleet status [host...]`

Fans out over SSH, reads each host's install stamp, classifies it against your
local dotfiles clone, and prints a worst-first table. Exits non-zero if **any**
host is stale, so it drops into scripts and CI.

```sh
fleet status                       # every #fleet host
fleet status web-01 db-01          # just these two
fleet status --json | jq .         # machine-readable
```

The `BRANCH` column is the branch **checked out on the host right now**. When it
differs from the one the host last installed from, both are shown —
`feature/gff≠main` reads *"checked out feature/gff, last installed from main"*,
which is usually the explanation for an `ahead/divergent` row. `detached` is a
detached HEAD and `-` means no clone. It costs nothing: the live branch rides in
the same SSH round-trip fleet already spends reading the install stamp.

Classes: `up-to-date`, `behind`, `divergent`, `unknown`, `unreachable`. An
unreachable host is reported in the table, never silently dropped. A host that has
never run the stamped installer reads `unknown` (the stamp is **not** retroactive);
a stamp that exists but won't parse reads `unknown (corrupt stamp)` — deliberately
distinct from never-installed.

A host that only *looked* unreachable and was roused by the wake ladder reads its
real class with provenance attached: `up-to-date (woke via nano-01)`.

### `fleet wake [host...]`

Some hosts are asleep, not dead. A Wi-Fi NIC with power saving enabled sleeps
through the **broadcast** ARP requests that a cold neighbour cache has to send, so
your workstation never resolves its MAC and gives up at layer 2 — before a single
SSH packet is sent. The host is up; you simply cannot address it.

`fleet wake` escalates a bounded ladder against each target and prints it rung by
rung, because the ladder *is* the diagnostic:

```sh
fleet wake pi-01                   # one host
fleet wake                         # every #fleet host
fleet wake pi-01 --json | jq .     # full ladder, machine-readable
```

```text
pi-01
  retry        x   still unreachable after 1 retries
  local-prime  x   skipped: workstation is not on the target's subnet
  peer-relay   OK via nano-01
  => reachable (woke via nano-01)
```

| Rung | What it does | When it applies |
| :-- | :-- | :-- |
| `retry` | re-probe after a short backoff | always; catches a neighbour entry that just expired |
| `local-prime` | ping the target so the local stack ARPs for it | only when this machine shares the target's subnet |
| `peer-relay` | ask a reachable peer for `$SSH_CONNECTION` to learn this workstation's LAN address, reach the target **through** that peer, and tell it to send traffic back | whenever another fleet host answers |

`peer-relay` is the rung that resolves the common case, and the only one that works
when your workstation has no layer-2 presence on the fleet's subnet at all — under
WSL2's NAT the ARP table that matters belongs to Windows, so `local-prime` correctly
skips itself there.

Two properties worth knowing:

- **Wake never modifies a target.** It sends ICMP, reads `$SSH_CONNECTION`, and probes.
- **Only a direct re-probe counts as success.** A working relay proves the *peer* can
  reach the target, not that you can — so a real network partition can never be
  reported as a wake.

Wake also runs **automatically** whenever a probe would report `unreachable`, inside
`status`, `tui`, and `update` alike. It runs within the existing concurrent fan-out,
so ten sleeping hosts cost about one budget of wall clock rather than ten. Use
`--no-wake` when you want the fast, literal answer (CI, cron, scripts).

> **The permanent fix lives on the host, not here.** If a machine needs waking on
> every run, disable Wi-Fi power save on it — check with
> `iw dev wlan0 get power_save`, and make it stick with a NetworkManager drop-in at
> `/etc/NetworkManager/conf.d/wifi-powersave-off.conf` containing
> `[connection]` and `wifi.powersave = 2`.
>
> fleet deliberately will not do this for you: reconfiguring a machine's networking
> as a side effect of *looking* at it is a worse surprise than a slow probe.

### `fleet tui`

```
╭────────────────────────────────────────────────────────────────────────────────╮
│ 🛰️  fleet 0.1.0 (40b4953)                                                      │
│ ❓ ?: help   🔍 /: search   ● space: selection   🚀 u: update                  │
│ 📜 l: log pane   🖥️ s: ssh   🔄 r: refresh   🚪 q: quit                        │
╰────────────────────────────────────────────────────────────────────────────────╯
```

The interactive dashboard. Framed panels provide unified visual structure and clear
discoverability for all actions. Opens **instantly** and streams rows in as each host
answers — a slow or unreachable host never blocks the view.

```sh
fleet tui
fleet tui --jobs 8                       # more concurrent updates
fleet tui --update-ref feature/x         # update targets that ref instead of main
```

| Key | Does |
|-----|------|
| `j` `k` `↓` `↑` | move cursor |
| `gg` / `G` | first / last host |
| `ctrl+d` / `ctrl+u` | half page down / up |
| `ctrl+f` / `ctrl+b` | page down / up |
| `/` | regex search — smartcase, live match count, inline error on a bad pattern |
| `n` / `N` | next / previous match (wraps) |
| `space` | toggle selection on the cursor host |
| `v` | visual range select (extend with motions) |
| `esc` | clear search / selection |
| `a` | select all — respects an active `/` filter |
| `u` | update the selection (or the cursor host) |
| `w` | wake the selection (or the cursor host) — rows tick `waking ⠋` |
| `F` | forget the remembered answers (including the saved preferences) |
| `e` | on the confirm strip: edit the remembered answers |
| `s` | ssh to the cursor host |
| `l` | show / hide the streaming log pane |
| `tab` | move the vim keys between the host list and the log pane |
| `J` / `K` | scroll the log pane |
| `gg` / `G` | (log focused) first line / resume following the tail |
| `/` `n` `N` | (log focused) search the log, next / previous match |
| `r` | refresh |
| `?` | help overlay |
| `q` | quit (guarded while updates run) |

The status dot left of each hostname is **navy** when selected, and flips
**green** or **red** to report an update's outcome — so a finished wave reads at
a glance. Each area (host list, log pane, help, answer form) is its own framed
panel.

Rows are colored by status (green up-to-date · yellow behind · magenta
divergent · dim unknown · red unreachable) and degrade cleanly on terminals
without color.

**Updates run concurrently in the background.** `u` prechecks each target with
`sudo -n true`, then updates up to `--jobs` (default 4) hosts at once *without
taking the terminal* — each row shows `queued → updating → ok/FAIL`, and the
TUI stays fully interactive: you can navigate, search, and even `r` refresh
while a wave runs (hosts being updated are excluded from the re-poll).

Background runs use `ssh -o BatchMode=yes`, so a host that unexpectedly asks
for a password **fails fast with the reason on its row** instead of hanging on
input it cannot receive. Hosts whose precheck says they *need* a password are
routed to a serial interactive handoff that runs after the background wave —
there the terminal is genuinely released so the sudo prompt reaches you.

**The log pane is on by default.** While idle it collapses to a single hint
line, so it costs the host list nothing; once output starts flowing it claims
its share. `l` splits the view: the host list keeps the top ~20% and a
framed pane below streams each in-flight host's output live, tagged with the
alias — each host in its own colour, so a concurrent wave stays readable. It follows the tail by default;
`J`/`K` scroll (which pauses following so the tail can't yank the view away
mid-read). `l` again restores the list to the full height. The per-host
progress column and `FAIL:` text are unchanged — the pane adds detail, it does
not replace the summary.

Pressing `u` again while a host is still updating leaves that run alone and
says so — it never starts a second install on top of a live one. Hosts that
have *finished* can be updated again freely; re-running clears the previous
`ok`/`FAIL` from the row.

**Unattended answers.** `u` opens a short form before anything runs — asked once
per **session**, not once per wave:

| Field | Feeds |
|-------|-------|
| sudo password | primes `sudo -S -v` on the host so install.sh's privileged steps actually run. Masked on screen; held in memory only; sent over ssh **stdin** — never argv or env, both of which are world-readable via `/proc`. Leave empty to skip privileged steps. |
| windows setup `[y/n/s]` | `WINSETUP_ANSWER` — install.sh's Windows desktop prompt (`s` = never ask again, recorded as a gff override) |
| force reset `[y/n]` | hard-resets each host onto the fetched commit instead of fast-forwarding — for a host whose branch has diverged. **Destructive**, so the host's entire current state (local commits *and* uncommitted files) is committed to a `fleet-reset/<ts>` branch first. The confirm gate calls it out in red. |
| gemini leftovers `[y/k/n]` | `GEMINI_TEARDOWN_ANSWER` — `yes` clean up, `keep` never ask again, `skip` this run only |

The credential is primed and used in the **same ssh session** as install.sh
(sudo's timestamp is tty/session-scoped, so priming in a separate connection is
not guaranteed to carry), and the prime is **verified** with `sudo -n true`
before the install starts — otherwise a long run would proceed with every
privileged step silently skipping. A rejected password and a credential that
did not persist are reported as distinct, named failures on the row.

**Answers are sticky.** They survive `esc`, selection changes, and every later
wave, so a fleet-wide update applies *the same* answers everywhere without you
retyping them — retyping is exactly how two waves end up diverging. On later
waves `u` skips the form and goes straight to the confirm strip, which shows
what is about to be applied:

```text
update 7 host(s) → main: web-01, web-02, db-01, …
  answers: sudo •••••• · windows s · gemini keep   y: go  e: edit answers  n/esc: cancel
```

That display is the point: the old design forgot the answers between waves, so
it never had to show them. This one remembers, so it shows them every time.

- `e` on the confirm strip reopens the form, pre-filled.
- `F` forgets everything, credential included, and deletes the saved preferences.
- The **credential is never written to disk** and dies with the process. Only
  `windows` and `gemini` persist, to `~/.config/fleet/answers.json` (`0600`) —
  the on-disk type has no field for a credential, so it cannot leak there even
  by accident.

**Targeting a subset.** `a` selects everything currently visible, and it respects
an active search — so `/feature` then `a` then `u` updates exactly the hosts on
a feature branch, which is what the `BRANCH` column is for.

Selection and cursor are keyed by host **alias**, not row position, so the
worst-first re-sort that happens as rows stream in never moves your cursor or
corrupts a selection.

### `fleet update <host>...`

Brings hosts current from a **plan** — a small YAML file, `fleet.yaml`, naming the
repos on every host and a dependency graph of steps to run in them. Hosts run one
at a time (an interactive step cannot share a terminal); within a host the steps
run in dependency order, a failed step blocks only its dependents, transient
failures retry with backoff under per-attempt timeouts, and one line per step is
reported at the end.

**With no plan file, nothing changes**: the built-in plan is exactly the old
`fleet update` — `fetch` → checkout `main` → `merge --ff-only` → `./install.sh`
over `ssh -t` — byte for byte on the wire.

```sh
fleet update web-01                          # built-in plan, or your fleet.yaml if present
fleet update web-01 --ref v1.2.0             # point the (sole / dotfiles) repo at a tag or branch
fleet update web-01 --ref scripts=release    # per-repo form when the plan has several
fleet update web-01 db-01 --dry-run          # print every script that WOULD run; send nothing
fleet update web-01 --file ./fleet.yaml      # an explicit plan (skips gff resolution)
fleet update init                            # write the starter plan to ~/.config/fleet/fleet.yaml
fleet update init --print                    # ...or just show it
```

| Flag | Does |
|------|------|
| `--local skip\|rescue\|carry` | override every repo's local-changes policy (table below) |
| `--force` | alias for `--local rescue` — the pre-plan meaning, unchanged |
| `--no-restore` | leave each host on the target branch; never re-apply a carried stash |
| `--reset` | hard-reset onto the fetched commit instead of fast-forwarding — for a diverged host. **Destructive**, so the clone's whole state is committed to `fleet-reset/<ts>` first. Incompatible with `carry` |
| `--timeout D` | override every **batch** step's per-attempt timeout (interactive steps keep the plan's) |
| `--no-retry` | run every step at most once |
| `--ref B` / `--ref repo=B` | target a ref; repeatable. Bare `B` picks the repo named `dotfiles`, or the plan's only repo |
| `--file PATH` | read this plan; must exist |
| `--dry-run` | print the plan source, every effective script and each step's timeout/retry; touch no runner |
| `--json` (global) | the report as one JSON document, nothing else |

#### `fleet.yaml` — the plan

`fleet update init` writes this starter (it is the built-in plan, and it is what
`opt/etc/fleet/fleet.yaml` in this repo contains):

```yaml
# fleet.yaml — the fleet-update plan.
# With no repos/steps overridden this is byte-for-byte today's `fleet update`:
# fetch+ff-only dotfiles under ~/git, then re-run ./install.sh interactively.
version: 1
update:
  # root: where relative repo paths resolve (repos.<name>.path defaults to <name>).
  root: ~/git
  repos:
    dotfiles:
      path: dotfiles
      branches: [main]
      # local: skip|rescue|carry (default skip); restore: true|false (default true)
  steps:
    - id: dotfiles.sync
      kind: sync
      repo: dotfiles
    - id: dotfiles.install
      kind: run
      repo: dotfiles
      run: ./install.sh
      interactive: true
      needs: [dotfiles.sync]
```

The full schema, with every default:

```yaml
version: 1                              # must be 1
update:
  root: ~/git                           # absolute or ~/ path; relative repo paths resolve under it
  defaults:                             # merged into every step, field by field
    timeout: 30m                        # per ATTEMPT; 0 = none; interactive steps default to 0
    retry:
      attempts: 1                       # >= 1; 1 = no retry
      on: [transport]                   # transport | timeout | any | <exit code> ...
      backoff: { initial: 5s, factor: 2, max: 2m, jitter: true }
  repos:
    <name>:                             # ^[a-z0-9][a-z0-9._-]*$
      path: <rel | /abs | ~/...>        # default = <name>; [A-Za-z0-9._/-], no "..", no leading "-"
      url: <https:// | ssh:// | git@…>  # optional; lets a MISSING clone be cloned (that clone is the one network call)
      branches: [default]               # "default" = the remote HEAD, only allowed first; the rest are branch names
                                        # (a tag works only as the sole entry — multi-entry lists are branches)
      local: skip                       # skip | rescue | carry — what to do with a DIRTY clone (table below)
      restore: true                     # put the host back on the branch it was on (and re-apply a carried stash)
  steps:
    - id: <id>                          # ^[a-z0-9][a-z0-9._-]*$, unique
      kind: sync | run | gh-auth
      repo: <name>                      # sync: required; run: optional (adds `cd <path> &&`); gh-auth: forbidden
      run: <shell>                      # run only, required; VERBATIM — no quoting, no filtering (see trust boundary)
      interactive: false                # run only; true = `ssh -t`, terminal handed over, never retried
      hostname: github.com              # gh-auth only
      needs: [<id>, ...]                # existing ids, not self, acyclic
      expect: { exit: [0] }             # exit codes that count as ok, each 0..255
      on_failure: stop                  # stop = dependents are dep-fail; continue = they run, host still fails
      timeout: <duration>               # overrides defaults.timeout for this step
      retry: { attempts: 3, on: [transport, timeout], backoff: {...} }   # merges over defaults; forbidden on interactive steps
```

Unknown keys are errors (`KnownFields`), and every fault in the file is reported
at once — a plan with two mistakes costs one round-trip, not two.

Step kinds:

- **`sync`** — a read-only precheck (`state=missing|clean|dirty|in-progress branch=…`),
  then exactly **one** network call: `git fetch origin <b>` + `merge --ff-only` for a
  single branch (the old string), one `fetch origin b1 b2 …` for several (extras are
  fast-forwarded only when the local branch is an ancestor of its remote, otherwise
  `skipped(diverged)`), the remote HEAD for `default`, or a `git clone` when the path
  is missing and a `url` is set. A merge or rebase in progress is skipped under every
  policy.
- **`run`** — `cd <path> && <run>` (or just `<run>` with no `repo`), batch or, with
  `interactive: true`, over `ssh -t`. `WINSETUP_ANSWER` / `GEMINI_TEARDOWN_ANSWER`
  from your local environment are exported into run steps only.
- **`gh-auth`** — `gh auth status` in batch; only if that fails, `gh auth login --web`
  + `gh auth setup-git` interactively, then one re-check. An authenticated host makes
  zero interactive calls; `gh` missing reads `gh not installed`, not an auth failure;
  no token, `GH_TOKEN`, `GITHUB_TOKEN` or `--with-token` ever appears in a remote string.

#### Which plan runs — resolution order

1. `--file PATH` — must exist; skips everything below.
2. gff `fleet.update.enabled` — `false` pins the built-in plan regardless of any file.
3. gff `fleet.update.config` — `home` (the default) is `~/.config/fleet/fleet.yaml`
   (`$XDG_CONFIG_HOME/fleet/fleet.yaml`); `repo` is `<--repo>/opt/etc/fleet/fleet.yaml`,
   the **tracked team plan** in the local dotfiles clone. Switch with
   `gff set fleet.update.config repo`.
   Mind the mode check below: git does not record the group-write bit, so a clone made
   under umask `002` (Ubuntu's default) has that file as `664` and `fleet` refuses it —
   `chmod g-w opt/etc/fleet/fleet.yaml` once in your clone.
4. That file missing ⇒ the built-in plan, and the report says so:
   `plan: built-in default (no ~/.config/fleet/fleet.yaml)`.

gff is **fail-open** here: gff missing, a key unknown, a bad selection — all behave as
enabled + `home`, with the reason appended to the `plan:` line. Lookups are scoped to
the `--repo` checkout's live feature file, never the current directory.

A plan file that is present must be **owned by you and not group- or world-writable**,
or `fleet update` refuses it. That is the whole trust boundary: `run:` is executed
verbatim, exactly as written — it is not quoted, filtered or inspected, because a
plan file is executable config in the same sense a Makefile is. What protects you is
who can write the file, and `--dry-run`, which prints the exact bytes that would go
over the wire so you can read them before anything runs.

#### Local changes on the host — `local:` policy

`sync` never clobbers work. What it does with a **dirty** clone is the repo's policy
(overridable fleet-wide with `--local`, and `--force` still means `rescue`):

| State on the host | `skip` (default) | `rescue` (`--force`) | `carry` |
|---|---|---|---|
| clean, on the target branch | sync | sync | sync |
| clean, on another branch (or detached) | switch, sync, steps, **switch back** | same | same |
| dirty (tracked edits and/or untracked files) | **skip** — dependents blocked | `git add -A` onto `fleet-rescue/<ts>`, materialised as a worktree under `~/.local/state/fleet/rescue/<repo>/<ts>`; then sync, steps, switch back | `git stash push -u` (SHA captured), switch, sync, steps, then `checkout <orig>` + `stash apply <sha>` + `stash drop <sha>` |
| merge / rebase in progress | skip | skip | skip |
| missing, `url:` set | clone | clone | clone |
| missing, no `url:` | fail | fail | fail |

Restore rules, under every policy:

- The host is put back on the branch it was on **by default**; `restore: false` on
  the repo or `--no-restore` leaves it on the target (noted on the step, not a failure).
- Restore is **cleanup, not a dependent**: it runs after the last step that uses the
  repo even if that step failed, and immediately if the sync itself failed after
  switching or stashing. It has its own fixed policy (3 attempts, transport only, 5m).
- A carried stash is dropped **only after a clean apply**. On a conflict it is kept and
  the report names it: `restore-failed stash=<sha> branch=<orig>`. Manual recovery on
  the host is then `git checkout <orig> && git stash apply <sha>`.
- Why `carry` may use a stash when the old rescue could not: the old proposal was
  `stash push` + `git branch <n> stash@{0}`, and a branch cut from a stash commit does
  **not** contain the untracked files (they live in `stash^3`). `carry` pushes with `-u`
  and restores with `stash apply <sha>` — the entry itself, by SHA, never `stash@{0}` or
  `pop` — which re-applies the whole entry, untracked files included.

#### Retries and timeouts

| Setting | Default | Meaning |
|---|---|---|
| `timeout` | `30m` batch, `0` (none) interactive | per **attempt**; on expiry the local `ssh` is killed (the remote job is not — it keeps running), the step reads `timed out after 30m` |
| `retry.attempts` | `1` | total attempts; the whole script re-runs |
| `retry.on` | `[transport]` | which failures retry: `transport` (ssh exit 255 / dial failure), `timeout`, `any`, or an exit code such as `75`. An **expected** exit is never retried |
| `retry.backoff` | `5s × 2, max 2m, jitter` | wait before attempt *n* is `min(max, initial × factor^(n-1))` ± 50 % |
| interactive steps | — | never auto-retry (a retried `install.sh` would re-ask everything), never get a deadline unless the plan sets one; `--timeout` does not touch them |
| `<repo>.restore` | fixed | 3 attempts, transport only, 5m — `--no-retry` / `--timeout` do not apply |

`--no-retry` forces one attempt everywhere; `--timeout D` overrides every batch step.
Each attempt writes `=== step <id> attempt a/n (after <wait>) ===` into the run log.

#### The report

The first line names the plan source. Then, per host, `=== host ===` and one line per
step — `ok|FAIL|skip|dep-fail <id> [exit N] [attempt a/n] [timeout] <duration> <notes|reason>`
— then `log: <path>` pointing at the full capture under `~/.local/state/fleet/logs/`
(every CLI run is teed there, header `fleet update — host=H plan=<source> mode=<fast-forward|FORCE RESET> started=<RFC3339>`;
a capture that cannot be opened never costs the update). Exit status is non-zero with
`N host(s) not updated` if any step on any host is not `ok`. `--json` emits
`{ "plan": …, "reports": [ … ] }` with every field of every step, verbatim.

#### Console demo (real output, `--dry-run`; host alias and home redacted)

A two-repo plan with a `gh-auth` gate, `local: carry` on dotfiles, and a `make` step
that tolerates exit 2 and does not block anything:

```yaml
version: 1
update:
  root: ~/git
  defaults:
    timeout: 30m
    retry: { attempts: 3, on: [transport], backoff: { initial: 5s, factor: 2, max: 2m, jitter: true } }
  repos:
    dotfiles:
      branches: [main]
      local: carry
    scripts:
      path: work/scripts
      url: git@github.com:example/scripts.git
      branches: [default]
  steps:
    - id: gh
      kind: gh-auth
    - id: dotfiles.sync
      kind: sync
      repo: dotfiles
      needs: [gh]
    - id: dotfiles.install
      kind: run
      repo: dotfiles
      run: ./install.sh
      interactive: true
      needs: [dotfiles.sync]
    - id: scripts.sync
      kind: sync
      repo: scripts
      needs: [gh]
    - id: scripts.make
      kind: run
      repo: scripts
      run: make install
      needs: [scripts.sync]
      expect: { exit: [0, 2] }
      on_failure: continue
      timeout: 10m
```

```console
$ fleet update <host> --dry-run --file $TMPDIR/fleet.yaml
plan: $TMPDIR/fleet.yaml
=== gh (gh-auth) ===
  check: command -v gh >/dev/null 2>&1 || exit 127; gh auth status -h github.com >/dev/null 2>&1
  login (if check fails, interactive): gh auth login -h github.com --web --git-protocol https && gh auth setup-git -h github.com
  timeout=30m0s retry=3 on=[transport]
=== dotfiles.sync (sync) ===
  precheck: if [ ! -e ~/git/dotfiles/.git ]; then echo "state=missing"; else cd ~/git/dotfiles && g=$(git rev-parse --git-dir) && if [ -e "$g/MERGE_HEAD" ] || [ -d "$g/rebase-merge" ] || [ -d "$g/rebase-apply" ]; then s=in-progress; elif [ -n "$(git status --porcelain)" ]; then s=dirty; else s=clean; fi; b=$(git symbolic-ref -q --short HEAD || echo detached); echo "state=$s branch=$b"; fi
  sync (local=carry): cd ~/git/dotfiles && orig=$(git symbolic-ref -q --short HEAD || git rev-parse HEAD) && echo "fleet: orig=$orig" && ts=$(date -u +%Y%m%dT%H%M%SZ) && { [ -z "$(git status --porcelain)" ] || { git stash push -q -u -m "fleet-carry $ts" && echo "fleet: carried stash=$(git rev-parse stash@{0}) from=$orig"; }; } && git fetch origin main && git checkout main && git merge --ff-only FETCH_HEAD; rc=$?; now=$(git symbolic-ref -q --short HEAD || git rev-parse HEAD); [ "$orig" = "$now" ] || echo "fleet: switched $orig -> $now"; exit $rc
  timeout=30m0s retry=3 on=[transport]
=== dotfiles.install (run) ===
  run (interactive): cd ~/git/dotfiles && ./install.sh
  timeout=0s retry=3 on=[transport]
=== scripts.sync (sync) ===
  precheck: if [ ! -e ~/git/work/scripts/.git ]; then echo "state=missing"; else cd ~/git/work/scripts && g=$(git rev-parse --git-dir) && if [ -e "$g/MERGE_HEAD" ] || [ -d "$g/rebase-merge" ] || [ -d "$g/rebase-apply" ]; then s=in-progress; elif [ -n "$(git status --porcelain)" ]; then s=dirty; else s=clean; fi; b=$(git symbolic-ref -q --short HEAD || echo detached); echo "state=$s branch=$b"; fi
  clone (if missing): mkdir -p "$(dirname ~/git/work/scripts)" && git clone -q 'git@github.com:example/scripts.git' ~/git/work/scripts && cd ~/git/work/scripts && b1=$(git symbolic-ref -q --short HEAD)
  sync (local=skip): cd ~/git/work/scripts && orig=$(git symbolic-ref -q --short HEAD || git rev-parse HEAD) && echo "fleet: orig=$orig" && git fetch origin && { b1=$(git symbolic-ref -q --short refs/remotes/origin/HEAD); b1=${b1#origin/}; [ -n "$b1" ] || { b1=$(git ls-remote --symref origin HEAD | sed -n 's|^ref: refs/heads/\(.*\)[[:space:]]HEAD$|\1|p') && [ -n "$b1" ] && git remote set-head origin "$b1"; }; [ -n "$b1" ] || { echo 'fleet: cannot resolve the default branch' >&2; exit 3; }; } && git checkout -q "$b1" && git merge --ff-only "origin/$b1"; rc=$?; now=$(git symbolic-ref -q --short HEAD || git rev-parse HEAD); [ "$orig" = "$now" ] || echo "fleet: switched $orig -> $now"; exit $rc
  timeout=30m0s retry=3 on=[transport]
=== scripts.make (run) ===
  run: cd ~/git/work/scripts && make install
  timeout=10m0s retry=3 on=[transport]
```

(`dotfiles.install` shows the merged `retry=3`, but interactive steps are forced to one
attempt at run time.) The same command with no plan file names its source and is the
old update exactly:

```console
$ fleet update <host> --dry-run
plan: built-in default (no $HOME/.config/fleet/fleet.yaml)
=== dotfiles.sync (sync) ===
  precheck: if [ ! -e ~/git/dotfiles/.git ]; then echo "state=missing"; else cd ~/git/dotfiles && g=$(git rev-parse --git-dir) && if [ -e "$g/MERGE_HEAD" ] || [ -d "$g/rebase-merge" ] || [ -d "$g/rebase-apply" ]; then s=in-progress; elif [ -n "$(git status --porcelain)" ]; then s=dirty; else s=clean; fi; b=$(git symbolic-ref -q --short HEAD || echo detached); echo "state=$s branch=$b"; fi
  sync (local=skip): cd ~/git/dotfiles && orig=$(git symbolic-ref -q --short HEAD || git rev-parse HEAD) && echo "fleet: orig=$orig" && git fetch origin main && git checkout main && git merge --ff-only FETCH_HEAD; rc=$?; now=$(git symbolic-ref -q --short HEAD || git rev-parse HEAD); [ "$orig" = "$now" ] || echo "fleet: switched $orig -> $now"; exit $rc
  timeout=30m0s retry=1 on=[transport]
=== dotfiles.install (run) ===
  run (interactive): cd ~/git/dotfiles && ./install.sh
  timeout=0s retry=1 on=[transport]
```

#### `fleet update init`

Writes the starter plan to the resolved location (`~/.config/fleet/fleet.yaml`, dir
`0700`, file `0644`) and refuses to overwrite without `--overwrite`; `--file PATH`
writes elsewhere; `--print` writes to stdout instead. It **shadows a host named
`init`** — `fleet update init` is always this verb.

> **Note:** the install-stamp that `status` reads is written by `install.sh`, so
> a host only starts reporting a real commit/age once it has run an `install.sh`
> that *contains* the stamp step. Until then it reads `unknown` — correctly, not
> as an error.

### `fleet discover`

Lists every concrete host in `~/.ssh/config` and whether it's already in the
fleet — the `available` rows are the ones you can adopt. `--json` for scripts.

```sh
fleet discover
# HOST             HOSTNAME             STATUS
# nano             10.0.0.5             available
# web              10.0.0.1             in-fleet
#
# 1 host(s) available — adopt one with `fleet add <alias>`,
# or all of them with `fleet discover --add-all`

fleet discover --add-all              # adopt every available host (confirms first)
fleet discover --add-all --dry-run    # show the resulting config, write nothing
fleet discover --add-all --yes        # non-interactive
```

`--add-all` marks every available host in **one pass**, so the whole batch is a
single write and a single backup rather than one per host — a partial write to
`~/.ssh/config` would cost SSH access to every machine. It names the hosts and
asks before touching anything, does nothing when everything is already in the
fleet (no write, no backup), and is fully reversible: `fleet remove <alias>`
restores the original block byte-for-byte.

### `fleet add` / `fleet remove`

`add` has two modes and picks automatically:

- **Adopt** — if the alias is *already* a Host block in `~/.ssh/config`, `add`
  just marks it `#fleet` in place, preserving every directive. No `--hostname`,
  because ssh already knows how to reach it. This is the common case.
- **Create** — if the alias is *not* in the config, `add` writes a new marked
  block; that genuinely needs `--hostname`.

```sh
fleet add nano                                         # adopt an existing entry
fleet add web-02 --hostname 10.0.0.22 --user deploy    # create a new #fleet block
fleet add nano --dry-run                               # print the diff, write nothing
fleet remove web-02                                    # unmark — keeps SSH access
fleet remove web-02 --purge                            # delete the Host block entirely
```

Adopting is idempotent (`already in the fleet`) and reversible (`remove` unmarks
it, restoring the original block). `remove` only **unmarks** by default: leaving
the fleet never costs you SSH access. `--purge` is the destructive form.

### `fleet keys list` / `sync` / `prune`

Audit and manage `authorized_keys` across the fleet.

```sh
fleet keys list                    # which of your public keys each host authorizes
fleet keys sync                    # authorize your public key where it's missing
fleet keys prune                   # remove foreign keys — diff-first, confirm-per-change
```

- **`sync` sends public-key material only** — no private key ever leaves the
  workstation. The remote append is `grep`-guarded, so re-syncing is a no-op.
- **`prune` is diff-first**: it prints every removal, applies nothing without your
  confirmation, and deletes only that exact line — it never rewrites
  `authorized_keys` from local state.
- Per-host failures are named and rolled into the exit code, never swallowed.

### `fleet version`

```sh
fleet version                      # version, commit, dirty flag, build date
fleet version --json
```

## Global flags

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | `~/.ssh/config` | ssh config path (the host inventory) |
| `--marker` | `#fleet` | comment that marks a `Host` block as in-fleet |
| `--repo` | `~/git/dotfiles` | local dotfiles clone used as the up-to-date baseline |
| `--json` | off | machine-readable output |
| `--no-wake` | off | never try to rouse an unreachable host — fast, literal answer |
| `--wake-timeout` | `12s` | per-host budget for the reachability ladder |

## Safety invariants

Each of these is pinned by a test — they are the reason the tool is trustworthy
against real machines:

- No private key ever leaves the workstation.
- `prune` applies nothing without confirmation and deletes only exact lines.
- `remove` unmarks; only `--purge` deletes. Leaving the fleet keeps SSH access.
- Every ssh-config write takes a backup first and keeps `0600`.
- A dirty clone is skipped unless `--force` / `local: rescue`, which *preserves* work in
  a rescue worktree; `local: carry` stashes with `-u` and re-applies by SHA; a stash is
  dropped only after a clean apply. Nothing is ever discarded.
- Every `sync` step makes exactly one network call; `run:` is verbatim and the plan file
  must be yours and not group/world-writable; `--dry-run` sends nothing.
- A failed step blocks its dependents, never its siblings; a host is put back on its
  original branch by default; interactive steps never auto-retry.
- Failures are named per host and reflected in the exit code.
- Wake never mutates a target: ICMP, `$SSH_CONNECTION`, and the probe, nothing else.
- Only a direct re-probe can report a host woken — a working relay is not enough.

## Design

The whole decision surface is pure text-in / struct-out and unit-tested without
opening a socket. Only one package touches a remote host:

| Path | Responsibility |
|------|----------------|
| `cmd/` | cobra commands, table/JSON rendering, SSH fan-out |
| `internal/sshconf` | parse **and edit** `~/.ssh/config` (the inventory) |
| `internal/stamp` | parse the install stamp |
| `internal/drift` | classify drift + format age (clock injected, never `time.Now()`) |
| `internal/keys` | `authorized_keys` diff (reports removals, never applies them) |
| `internal/runner` | the only seam that runs a remote command (`Exec` real, `Fake` for tests) |
| `internal/updplan` | the `fleet.yaml` schema: parse, defaults merge, validation, step order (pure) |
| `internal/updexec` | the remote script builders + the per-host executor (retries, cascade, restore); clock, sleep and I/O injected |
| `internal/featflag` | fail-open resolution of the `fleet.update.*` gff flags; the only import of the gff SDK |

## Development

```sh
cd sdk/fleet
go build ./...
go test ./... -cover
./scripts/test.sh          # repo-wide; enforces the fleet coverage floor
```

The coverage floor lives in `scripts/test.sh` `coverage_min()`; a module missing
from that map is silently exempt from the gate. See [`AGENTS.md`](./AGENTS.md) for
the full invariant list and gotchas.
```
