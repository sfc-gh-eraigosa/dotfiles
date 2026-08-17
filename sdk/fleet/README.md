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

Classes: `up-to-date`, `behind`, `divergent`, `unknown`, `unreachable`. An
unreachable host is reported in the table, never silently dropped. A host that has
never run the stamped installer reads `unknown` (the stamp is **not** retroactive);
a stamp that exists but won't parse reads `unknown (corrupt stamp)` — deliberately
distinct from never-installed.

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
| `u` | update the selection (or the cursor host) |
| `s` | ssh to the cursor host |
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

**Unattended answers.** `u` opens a short form before anything runs, asked once
per wave (nothing is pre-filled, so a wave never inherits stale answers):

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
