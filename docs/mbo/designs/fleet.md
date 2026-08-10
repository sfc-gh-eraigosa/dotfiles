# fleet — on-demand dotfiles install-status checker — design

- **Slug:** fleet
- **Date:** 2026-08-09
- **Status:** Draft
- **Relates to:** issue [#222](https://github.com/sfc-gh-eraigosa/dotfiles/issues/222) · PR: (this PR)
- **Author(s):** Edward Raigosa (with Claude)

## 1. Problem / context

There is no way to answer *"which of my hosts are running stale dotfiles?"* without SSHing
to each one and eyeballing it. Two verified gaps cause this:

1. **`install.sh` leaves no record that it ran.** Verified by inspection: the script has
   no timestamp, commit stamp, or state file anywhere in its ~35k of logic. A host's git
   clone can be perfectly current while `install.sh` last ran weeks ago — "pulled" and
   "installed" are different facts, and today we can observe neither.
2. **There is no host inventory.** The set of machines exists only in `~/.ssh/config`, and
   by explicit requirement it must **not** be committed to this repo.

Measured today (2026-08-09) by ad-hoc SSH against the three known hosts, using clone
freshness as a stand-in signal:

| Host (alias) | Clone commit | Status vs `origin/main` |
| :-- | :-- | :-- |
| `<host-pi>` | `0b8726e` | up to date |
| `<host-desktop>` | `0b8726e` | up to date |
| `<host-nano>` | `1bc1928` | **behind 24 commits** (2 weeks) |

That ad-hoc check took several iterations of shell debugging to get right, which is
precisely the argument for a deterministic tool over a remembered one-liner.

## 2. Goals & non-goals

**Goals**

- One on-demand command shows every fleet host: availability, the dotfiles version it
  installed, and how far behind `origin/main` it is.
- Deterministic and testable — drift and age math is unit-tested, not eyeballed.
- Update a stale host from the same tool, interactively, so sudo and other credential
  prompts still work.

**Non-goals (YAGNI)**

- No daemon, polling, scheduling, or background agent. On demand only.
- No inventory system beyond `~/.ssh/config`.
- No unattended/automatic remediation. Updates are explicit, visible, one at a time.

## 3. Options considered

**A. Go CLI under `sdk/` — recommended.** A new `sdk/fleet` cobra CLI beside the existing
`gss`, `gsl`, `gff`, `tmux-mgr`, `wol`. The fiddly parts — `~/.ssh/config` parsing,
behind-count classification, relative-age formatting — become pure functions with unit
tests and injected time. Matches the routing table in `docs/mbo/AGENTS.md` ("A Go CLI under
`sdk/`") and composes naturally with `wol` for waking a sleeping host. Costs a build step
and more upfront work.

**B. POSIX shell script in `opt/scripts/system/`.** Ships fastest, no build. But the
comparison/age/config-parsing logic is exactly what shell is worst at, and it resists
deterministic testing — which is the stated requirement.

**C. Hybrid — shell now, Go later.** Immediate value, migrate if it earns its keep. Risks
becoming permanent, and the migration re-does the work.

**Decision: A.** The explicit ask was for a *deterministic* tool; the repo's convention for
reusable CLIs is Go-in-`sdk/`-with-tests; the drift math deserves real tests. The
`install.sh` stamp addition is identical under all three options.

## 4. Decision

Four units with clear boundaries:

**Unit 1 — the stamp (an `install.sh` change).** On a successful run, write
`~/.local/state/dotfiles/install-stamp` (XDG state dir) as `key=value`:

```
commit=<full 40-char SHA of BASE_DIR HEAD>
installed_at=<unix epoch, UTC>
branch=<branch install ran from>
hostname=<uname -n>
```

Written **last** so an aborted run leaves no false success marker, and **only when
`INSTALL_PHASE=all`** so a Docker `--phase deps|config` build layer never stamps (that
gating already exists in `install.sh` and is load-bearing for the image cache).

**Unit 2 — host discovery (`internal/sshconf`).** Parse `~/.ssh/config`; take concrete
`Host` entries (skip `*`/`?` patterns); a host is in scope when its block carries a marker
comment, default `#fleet`. Connections use the ssh **alias**, inheriting
hostname/user/port/identity from the user's own config. Explicit args override discovery.

**Unit 3 — status (`internal/stamp` + `internal/drift`).** Resolve the baseline
(`git fetch -q origin`, then `origin/main`), read each host's stamp concurrently over SSH,
and classify: `up-to-date` · `behind N` · `ahead/divergent` · `unknown` (no stamp) ·
`unreachable`. Emit a worst-first table, `--json` for machines, and a **non-zero exit** when
any host is not up to date so it works as a scripted check.

**Unit 4 — TUI + update.** `fleet tui` (default on a TTY) renders one row per host:
availability · version · status. Pressing `u` updates the selected host. Because
`install.sh` prompts for sudo, the TUI must **not** capture its I/O: it releases the
terminal via Bubble Tea's `tea.Exec` and hands control to a live `ssh -t` session, then
resumes and refreshes that row. Remote flow:

```
cd ~/git/dotfiles && git fetch origin
  && git checkout main && git pull --ff-only origin main
  && ./install.sh
```

Also exposed headless as `fleet update <host...>`. Updates run **one host at a time** —
interactive sessions cannot share a terminal.

**Dirty-clone policy.** `pull --ff-only` fails on a dirty or divergent clone.
*Default: skip* that host with the reason reported — never mutate a machine carrying local
work. *`--force`: preserve, never discard* — capture the dirty state into a rescue worktree,
then fast-forward and install:

```
ts=$(date -u +%Y%m%dT%H%M%SZ)
git stash push -u -m "fleet-rescue-$ts"
git branch fleet-rescue/$ts stash@{0}
git worktree add ~/.local/state/dotfiles/rescue/$ts fleet-rescue/$ts
```

The rescue worktree is left on the host for inspection; the tool never deletes it.

## 5. Risks & blast radius

- **The stamp is not retroactive.** Every host reports `unknown` until it next runs
  `install.sh`. Until then the tool can only fall back to clone freshness, which answers a
  *different* question. The tool gets more truthful over time; it is not accurate on day one.
- **Update mutates machines** — the only unit with real blast radius. Mitigated by
  skip-on-dirty, `--force` preserving rather than discarding, one-at-a-time sequencing, and
  keeping the session interactive and visible to the operator.
- **The Jetson/ARM host is both the only stale machine and the least-tested target**
  (unlike the other two). Its first update is the real validation of the update path.
- **Reading is safe.** `status` is read-only SSH; a broken checker cannot damage a host.

## 6. Rollback

- The `install.sh` stamp is ~5 lines appended at the end; reverting the commit removes it.
  A stale stamp file left on a host is inert — nothing reads it but this tool.
- `sdk/fleet` is a standalone binary in `~/opt/bin`; deleting it and its `gff_on
  install.sdk.fleet` block removes the feature with no residue.
- No schema, service, or shared state is introduced, so there is nothing to migrate back.

## 7. Evidence expectations

The plan must capture, in `plans/fleet/evidence/`:

- **Unit-test captures** for the three pure packages — `sshconf` (fixture configs incl.
  pattern hosts and unmarked hosts), `stamp` (well-formed, truncated, absent), `drift`
  (up-to-date / behind-N / divergent / unknown, with injected `now` for age strings).
  The `sdk/` Go coverage gate (≥60%) applies.
- **A real-machine `status` capture** against all three hosts showing the mixed states —
  this is the headline proof and it already reproduces today (the Jetson host is behind).
- **A real update transcript** on the stale host: the interactive `ssh -t` session running
  `install.sh` to completion, followed by a `status` capture flipping that row to
  `up-to-date` with a fresh stamp. This is the one gate that cannot be faked by unit tests.
- **A dirty-clone capture** proving default-skip refuses, and a `--force` run showing the
  rescue worktree created and the local changes still recoverable.
- **A TUI screen capture** (asciinema or a still) showing the host list and the update key.

> Produced via `superpowers:brainstorming`. Register the objective in `../index.md`.
> The matching spec goes in `../specs/fleet.md`.
