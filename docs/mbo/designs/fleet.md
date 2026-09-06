# fleet — on-demand dotfiles install-status checker — design

- **Slug:** fleet
- **Date:** 2026-08-09
- **Status:** Draft
- **Relates to:** issue [#222](https://github.com/sfc-gh-eraigosa/dotfiles/issues/222) · PR [#223](https://github.com/sfc-gh-eraigosa/dotfiles/pull/223)
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
3. **Managing fleet membership and its access keys is manual and partly broken.** Nothing in
   the repo *adds* or *removes* a `Host` block — `opt/scripts/network/ssh-find` only rewrites
   the `HostName` of an alias that already exists. Key distribution does exist, as
   `src/ssh-key-sync/ssh-key-sync.sh` (66 lines, skill-wrapped), but auditing it against
   this objective found four defects (see §3 "Absorbing ssh-key-sync").

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
- **Manage fleet membership** — add a target, remove one, without hand-editing
  `~/.ssh/config`.
- **Manage fleet access keys** — generate, distribute, and audit SSH keys across fleet
  hosts, absorbing (and fixing) today's `ssh-key-sync.sh`.

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

### Absorbing `ssh-key-sync` (decided: absorb, not wrap)

`src/ssh-key-sync/ssh-key-sync.sh` already generates/distributes/audits keys across every
host in `~/.ssh/config`. Options were absorb into `fleet keys`, delegate to the script, or
leave it alone. **Decision: absorb** — one host model (`#fleet`-marked, not "every host in
the config"), one tool, and the logic becomes testable. Auditing the script line-by-line
found four defects that make porting-as-is the wrong move:

| # | Defect (line) | Consequence | Disposition in `fleet` |
| :-- | :-- | :-- | :-- |
| 1 | `scp "$P" "$P.pub"` (63) copies the **private** key to every host | any single compromised host yields an identity valid everywhere | **Fixed.** Public-key-only distribution by default. Private-key propagation is not reimplemented; if a shared identity is ever genuinely needed it must be an explicit, separately-designed opt-in. |
| 2 | `--prune` **overwrites** local and remote `authorized_keys` with only the workstation's `*.pub` (24, 26) | silently deletes any key not on the workstation — CI keys, other machines, colleagues; real lockout risk | **Fixed.** `fleet keys prune` is diff-first: prints what would be removed and requires explicit confirmation (`--yes` non-interactively); never blanket-overwrites. |
| 3 | `--delete` is a **no-op** (8) — consumes the flag, then falls through to the *generate* loop | `ssh-key-sync --delete foo` creates and syncs `foo` instead of deleting it; the skill documents deletion that does not exist | **Fixed.** Deletion is a real, tested verb with the same diff-first confirmation. |
| 4 | Failures swallowed by `\|\| true` / `2>/dev/null` (43, 63) | no report of which hosts failed; partial syncs look successful | **Fixed.** Per-host success/failure is reported and reflected in the exit code. |

Migration: `fleet keys` supersedes the script and its skill. Retire
`src/ssh-key-sync/` once parity is proven by the evidence in §7 — not before.

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

**Unit 5 — membership (`fleet add` / `fleet remove`).** Turns `~/.ssh/config` from a
read-only input into something `fleet` also *writes*. This is the one place the design's
original "we only read the config" boundary moves, so it is deliberately narrow:

- `fleet add <alias> --hostname <h> [--user u] [--port p] [--identity f]` appends a `Host`
  block carrying the `#fleet` marker. Idempotent: re-adding an existing alias updates only
  the fields given and never duplicates the block.
- `fleet remove <alias>` **unmarks** — drops `#fleet`, leaving the `Host` block intact.
  Removing a machine from the fleet is not the same as losing the ability to SSH to it, and
  the safe default must be the non-destructive one.
- `fleet remove <alias> --purge` deletes the whole block, for a machine that is truly gone.
- Every write takes a timestamped backup (`~/.ssh/config.bak-<ts>`) first, preserves
  comments/ordering/formatting elsewhere in the file, and never touches an unmarked block.
- `--dry-run` prints the resulting diff without writing.

**Unit 6 — keys (`fleet keys list|sync|prune|delete`).** The absorbed, corrected
`ssh-key-sync`, scoped to `#fleet` hosts:

- `list` — matrix of managed key → which hosts authorize it (parity with `--list`).
- `sync [name...]` — generate an ed25519 key if absent, then authorize its **public** key on
  each fleet host; per-host success/failure reported.
- `prune` / `delete` — diff-first reconciliation: show exactly which authorized entries would
  disappear from which hosts and require confirmation before touching anything.

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
- **`fleet` now writes `~/.ssh/config` (Unit 5).** A malformed write could cost SSH access to
  every machine — the highest-consequence failure in the design. Mitigated by a timestamped
  backup before every write, `--dry-run`, idempotent block editing that never touches
  unmarked hosts, and unmark-not-delete as the default for `remove`.
- **Key operations touch `authorized_keys` on real hosts (Unit 6).** The absorbed script's
  blanket-overwrite prune is the specific behavior that could lock you out; the replacement
  is diff-first and confirmation-gated. Distribution is public-key-only, so a compromised
  host no longer yields a reusable private identity.
- **Migration risk.** Retiring `ssh-key-sync.sh` before `fleet keys` reaches parity would
  leave a gap; §7 requires parity evidence first, and the scoping change (all-config-hosts →
  `#fleet`-marked) must be called out at retirement since it is a deliberate behavior change.

## 6. Rollback

- The `install.sh` stamp is ~5 lines appended at the end; reverting the commit removes it.
  A stale stamp file left on a host is inert — nothing reads it but this tool.
- `sdk/fleet` is a standalone binary in `~/opt/bin`; deleting it and its `gff_on
  install.sdk.fleet` block removes the feature with no residue.
- No schema, service, or shared state is introduced, so there is nothing to migrate back.
- **Config writes** are recoverable from the `~/.ssh/config.bak-<ts>` taken before each one.
- **Key changes** are the only partially-irreversible action: a removed `authorized_keys`
  entry must be re-synced (`fleet keys sync`) rather than "undone". This is why prune/delete
  are diff-first and confirmation-gated rather than rollback-dependent.
- **`ssh-key-sync.sh` stays in place until parity is proven**, so rollback during the
  migration window is simply "keep using the script".

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
- **Membership round-trip (Unit 5):** `add` a throwaway alias → it appears in `status` →
  `remove` (unmark) → it leaves fleet scope while the `Host` block survives → `--purge`
  removes the block. Captured with the `~/.ssh/config` diff at each step and the backup file
  listed, proving nothing else in the config moved.
- **Config-write safety:** a `--dry-run` capture showing the diff without a write, and a
  before/after showing unrelated (unmarked) blocks byte-identical.
- **Key parity + the four fixes (Unit 6):** `fleet keys list` output matching
  `ssh-key-sync.sh --list` on the same hosts (parity); a `sync` transcript showing **only the
  public key** landing on a host (defect 1); a `prune` capture showing the diff and the
  confirmation gate, with a deliberately foreign `authorized_keys` entry **surviving** a
  declined prune (defect 2); a `delete` capture proving it deletes rather than generating
  (defect 3); and a run against an unreachable host showing per-host failure reported with a
  non-zero exit (defect 4).

> Produced via `superpowers:brainstorming`. Register the objective in `../index.md`.
> The matching spec goes in `../specs/fleet.md`.
