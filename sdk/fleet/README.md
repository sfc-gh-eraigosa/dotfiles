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

The interactive dashboard. Opens **instantly** and streams rows in as each host
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
| `J` / `K` | scroll the log pane (re-open to resume following) |
| `r` | refresh |
| `?` | help overlay |
| `q` | quit (guarded while updates run) |

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
alias so a concurrent wave stays readable. It follows the tail by default;
`J`/`K` scroll (which pauses following so the tail can't yank the view away
mid-read). `l` again restores the list to the full height. The per-host
progress column and `FAIL:` text are unchanged — the pane adds detail, it does
not replace the summary.

**Unattended answers.** `u` opens a short form before anything runs — asked once
per **session**, not once per wave:

| Field | Feeds |
|-------|-------|
| sudo password | primes `sudo -S -v` on the host so install.sh's privileged steps actually run. Masked on screen; held in memory only; sent over ssh **stdin** — never argv or env, both of which are world-readable via `/proc`. Leave empty to skip privileged steps. |
| windows setup `[y/n/s]` | `WINSETUP_ANSWER` — install.sh's Windows desktop prompt (`s` = never ask again, recorded as a gff override) |
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

Brings a host current, one at a time: `fetch` → checkout the target ref →
`pull --ff-only` → run `install.sh` over `ssh -t` so you can answer credential
and sudo prompts live.

```sh
fleet update web-01                      # update to main (the default)
fleet update web-01 --ref v1.2.0         # or to a tag / branch
```

The target defaults to `main`. `--ref` points a host at a branch or tag instead
— useful to validate a change **before** it lands on main (e.g. confirming the
install-stamp writes, so `fleet status` starts reporting real data for that
host). The ref is constrained to the git ref charset before it's used in the
remote command.

A **dirty** remote clone is skipped by default rather than clobbered. `--force`
proceeds, but first preserves the host's uncommitted work in a rescue worktree
(`git add -A` onto a `fleet-rescue/<ts>` branch — not `git stash`, which silently
drops untracked files). Nothing is ever discarded.

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
- A dirty clone is skipped unless `--force`, which *preserves* work in a rescue
  worktree.
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
