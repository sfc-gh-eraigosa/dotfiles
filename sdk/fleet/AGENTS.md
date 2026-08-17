# fleet — dotfiles install-status checker

> **Up:** [`sdk/`](../AGENTS.md) · objective [`docs/mbo/`](../../docs/mbo/index.md) slug `fleet`
> (design · spec · plan · execution trio).

Answers *"which of my hosts are out of sync with the latest dotfiles install?"* —
on demand, never as a daemon — and manages fleet membership and access keys.

## Why it exists

`install.sh` used to leave **no record that it ran**, so a host's git clone could be
current while the installer last ran weeks ago. "Pulled" and "installed" are different
facts. `opt/scripts/system/install-stamp.sh` now records the second one; this tool reads it.

## Commands

| Command | Does |
| :-- | :-- |
| `fleet status [host...]` | table of host · commit · last run · status; `--json`; exits non-zero if any host is stale |
| `fleet discover` | list every concrete ssh-config host as `in-fleet` / `available`; `--json` |
| `fleet tui` | same rows interactively; `u` updates the selected host |
| `fleet update <host>...` | fetch → checkout `--ref` (default `main`) → `pull --ff-only` → `install.sh` over `ssh -t`, one host at a time |
| `fleet add <alias>` | **adopt** an existing ssh-config entry (marks in place, no `--hostname`); with `--hostname H` **creates** a new `#fleet` block. `--dry-run` |
| `fleet remove <alias> [--purge]` | unmark (keeps SSH access); `--purge` deletes the block |
| `fleet keys list\|sync\|prune` | audit / authorize / remove authorized keys |

## Layout

| Path | Responsibility |
| :-- | :-- |
| `cmd/` | cobra commands, rendering, SSH fan-out |
| `internal/sshconf` | parse **and edit** `~/.ssh/config` (the only inventory) |
| `internal/stamp` | parse the install stamp |
| `internal/drift` | classify drift + format age (`now` injected — never `time.Now()`) |
| `internal/keys` | authorized_keys diff (reports removals, never applies them) |
| `internal/runner` | the **only** seam that touches a remote host (`Exec` real, `Fake` for tests) |

Everything but `runner` is pure text-in/struct-out, so the decision surface is unit-tested
without opening a socket.

## Invariants (each pinned by a test — don't regress these)

- **No private key ever leaves the workstation.** Sync authorizes public keys only.
- **`prune` is diff-first**: prints each removal, applies nothing without confirmation, and
  deletes only that exact line — never rewrites `authorized_keys` from local state.
- **`add` adopts before it creates.** An alias already in `~/.ssh/config` is marked
  in place (`sshconf.Mark`), preserving every directive — `--hostname` is required
  *only* when writing a genuinely new block. Adoption is idempotent and reversible
  by `remove`.
- **`remove` unmarks; only `--purge` deletes.** Leaving the fleet never costs SSH access.
- **Every ssh-config write takes a timestamped backup first** and keeps `0600`.
- **A dirty clone is skipped** unless `--force`, which *preserves* work in a rescue worktree
  (`git add -A` onto a branch — **not** `stash@{0}`, which silently drops untracked files).
- **Failures are named per host** and reflected in the exit code; never swallowed.

## Gotchas

- The stamp is **not retroactive**: a host reports `unknown` until it runs an
  `install.sh` that *contains* the stamp step. Pre-merge, `fleet update <host>`
  (default target `main`) pulls a `main` whose `install.sh` has no stamp step yet,
  so status stays `unknown` — expected, not a bug. Point `update --ref` at the
  feature branch to prove the stamp before it merges.
- A stamp that exists but won't parse reports `unknown (corrupt stamp)` — deliberately
  distinct from never-installed.
- `build.sh` injects `cmd.Version`/`Commit`/`Dirty`/`BuildDate` by exact symbol path. Keep
  them exported, or the ldflags silently no-op and every binary reports `dev`.
- The coverage floor lives in `scripts/test.sh` `coverage_min()`; a module missing from that
  map is silently exempt.

## Build & test

```bash
bash sdk/fleet/build.sh          # -> ~/opt/bin/fleet
cd sdk/fleet && go test ./... -cover
./scripts/test.sh                # repo-wide; enforces the fleet coverage floor
```
