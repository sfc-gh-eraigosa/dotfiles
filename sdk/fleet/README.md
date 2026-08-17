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

The same rows, interactively (Bubble Tea). Arrow keys move within bounds; `u`
updates the selected host (releases the terminal so an `ssh -t` sudo prompt reaches
you); `q` / `Ctrl+C` quits. `--ref` selects the git ref to update toward.

```sh
fleet tui
```

### `fleet update <host>...`

Brings a host current, one at a time: `fetch` → `pull --ff-only` → run `install.sh`
over `ssh -t` so you can answer credential and sudo prompts live.

```sh
fleet update web-01
```

A **dirty** remote clone is skipped by default rather than clobbered. `--force`
proceeds, but first preserves the host's uncommitted work in a rescue worktree
(`git add -A` onto a `fleet-rescue/<ts>` branch — not `git stash`, which silently
drops untracked files). Nothing is ever discarded.

### `fleet add` / `fleet remove`

Edit the ssh-config inventory. Every write takes a timestamped backup first and
keeps `0600`.

```sh
fleet add web-02 --hostname 10.0.0.22 --user deploy    # add a #fleet Host block
fleet add web-02 --hostname 10.0.0.22 --dry-run        # print the diff, write nothing
fleet remove web-02                                    # unmark — keeps SSH access
fleet remove web-02 --purge                            # delete the Host block entirely
```

`remove` only **unmarks** by default: leaving the fleet never costs you SSH access.
`--purge` is the destructive form.

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
