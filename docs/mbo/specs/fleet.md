# fleet — on-demand dotfiles install-status checker — spec

- **Slug:** fleet
- **Date:** 2026-08-09
- **Status:** Draft
- **Relates to:** issue [#222](https://github.com/sfc-gh-eraigosa/dotfiles/issues/222) · PR [#223](https://github.com/sfc-gh-eraigosa/dotfiles/pull/223) · design [`../designs/fleet.md`](../designs/fleet.md)

## 1. Goal

Run one command and see, for every host in your fleet, whether it is running the latest
dotfiles: is it reachable, which commit did `install.sh` last install, how long ago, and how
many commits behind `origin/main` is that. When a host is stale, update it from the same
tool — interactively, so sudo and other credential prompts still work. The host list comes
from `~/.ssh/config` (never committed); everything runs on demand, never in the background.

## 2. Use cases

**UC-1 — "who needs updates?"**
*Actor:* operator at their workstation. *Trigger:* runs `fleet status` (or `fleet` on a TTY).
*Flow:* the tool resolves `origin/main`, reads each marked host's stamp over SSH in parallel,
classifies, prints a worst-first table.
*Acceptance:* every marked host appears exactly once with availability, commit, age, and
status; unreachable hosts are shown as `unreachable`, not silently dropped; exit code is
non-zero if any host is not `up-to-date`.

**UC-2 — "update the stale one."**
*Actor:* operator. *Trigger:* presses `u` on a selected row in the TUI, or runs
`fleet update <host>`.
*Flow:* tool hands the terminal to a live `ssh -t` session which fetches, checks out `main`,
fast-forwards, and runs `./install.sh`; the operator answers sudo/other prompts; on exit the
row refreshes.
*Acceptance:* the operator can type into `install.sh` prompts; on success the host's stamp
shows the new commit and the row flips to `up-to-date`; on failure the row shows the prior
state and a non-zero result is reported.

**UC-3 — "don't clobber my in-progress work."**
*Actor:* operator updating a host that has local edits in `~/git/dotfiles`.
*Flow:* the tool detects a dirty/divergent clone and refuses.
*Acceptance:* by default the host is **skipped** with the reason shown and nothing on it is
modified; with `--force` the dirty state is preserved in a rescue worktree (never discarded)
before the fast-forward proceeds.

**UC-4 — "use it from a script."**
*Actor:* a cron/CI/other script. *Trigger:* `fleet status --json`.
*Acceptance:* stable machine-readable output on stdout; non-zero exit when any host is not
up to date; no interactive prompts and no TUI when stdout is not a TTY.

## 3. Architecture

Four units; the first three are pure and independently testable, the fourth is the only one
that touches a remote machine destructively.

| Unit | Location | Responsibility | Depends on |
| :-- | :-- | :-- | :-- |
| stamp writer | `install.sh` (end of run) | record `commit`/`installed_at`/`branch`/`hostname` | nothing |
| `sshconf` | `sdk/fleet/internal/sshconf` | parse `~/.ssh/config` → in-scope aliases | file content only |
| `stamp` | `sdk/fleet/internal/stamp` | parse stamp text → struct | text only |
| `drift` | `sdk/fleet/internal/drift` | classify + format age | stamp + baseline + injected `now` |
| runner/TUI | `sdk/fleet/cmd` | SSH fan-out, table/JSON render, TUI, update handoff | the three above |

**Data flow:** `~/.ssh/config` → aliases → (SSH `cat` stamp, concurrent) → parse → classify
against `origin/main` → render (table · JSON · TUI). The update path is a separate,
sequential, interactive branch that shells out to `ssh -t` and re-reads the stamp afterwards.

**Key boundary:** every unit except the runner is a pure function over text with no SSH, no
clock, and no git — so the whole classification surface is unit-testable with fixtures and an
injected `now`.

## 4. Behavior / features

- **F1 — stamp on install.** `install.sh` writes `~/.local/state/dotfiles/install-stamp`
  (`key=value`) as its last successful action, only when `INSTALL_PHASE=all`.
- **F2 — host discovery.** Concrete `Host` entries in `~/.ssh/config` carrying the marker
  comment (default `#fleet`) are in scope; `*`/`?` pattern blocks are skipped; explicit CLI
  args override discovery entirely.
- **F3 — status table.** Worst-first rows of `HOST · AVAIL · LAST RUN · COMMIT · STATUS`.
- **F4 — classification.** `up-to-date` · `behind N` · `ahead/divergent` · `unknown` (no
  stamp) · `unreachable` (SSH failed).
- **F5 — JSON + exit code.** `--json` for machines; exit non-zero if any host ≠ `up-to-date`.
- **F6 — TUI.** One row per host with availability/version/status; refresh and quit keys;
  default view on a TTY.
- **F7 — update action.** `u` in the TUI / `fleet update <host...>`: interactive `ssh -t`
  running fetch → checkout main → `pull --ff-only` → `install.sh`; one host at a time.
- **F8 — dirty-clone safety.** Default skip with reason; `--force` preserves the dirty state
  in a rescue worktree under `~/.local/state/dotfiles/rescue/<ts>` before proceeding.

## 5. Evaluation criteria (per feature)

| Feature | Trigger predicate | Fires | Must NOT fire | Edge | Pass |
| :-- | :-- | :-- | :-- | :-- | :-- |
| F1 | `install.sh` completes, `INSTALL_PHASE=all` | stamp written with 40-char SHA + epoch | on `--phase deps`/`config`; on early exit/abort | `BASE_DIR` not a git repo → no stamp, no error | stamp present & parseable after a real run; absent after a phase build |
| F2 | `~/.ssh/config` parsed | marked concrete hosts returned | `Host *`/`?` patterns; unmarked hosts | marker in a trailing comment vs own line; duplicate `Host` lines | fixture config yields exactly the expected alias set |
| F3 | ≥1 host resolved | table with one row per host, worst first | duplicate or missing rows | zero hosts in scope → clear "no fleet hosts" message | golden-output test on a fixed input set |
| F4 | stamp + baseline known | correct class per case | `behind` when SHAs equal; `up-to-date` on unknown | stamp SHA not in local history → `ahead/divergent`, never a crash | table test over all five classes |
| F5 | `--json` passed | valid JSON on stdout; exit ≠ 0 if any stale | TUI or prompts when not a TTY | all hosts unreachable → still valid JSON, non-zero | parse output with `jq`; assert exit code |
| F6 | TTY + no subcommand | TUI renders host rows | in a pipe/redirect | terminal narrower than the table → no panic | manual capture + a render unit test on the model |
| F7 | `u` / `update` invoked | interactive session; stamp refreshed after | parallel updates; captured/suppressed stdin | `install.sh` fails → row keeps prior state, error surfaced | real transcript on the stale host + row flip |
| F8 | clone dirty or divergent | default: skip + reason | any mutation of a dirty host without `--force` | `--force` on a clean clone → normal path, no rescue worktree | dirty-host capture (skip) + `--force` capture (rescue exists, changes recoverable) |

## 6. Verification harness

- **Unit (automated, primary).** Table tests for `sshconf`, `stamp`, `drift` with fixture
  configs/stamps and an injected `now`; the `sdk/` Go coverage gate (≥60%) applies. These
  cover F2, F4, and the age formatting exhaustively without touching a network.
- **Golden output.** Table and `--json` renderers tested against fixed inputs so format
  changes are deliberate (F3, F5).
- **Shell-level.** A test for the `install.sh` stamp block asserting it writes under
  `INSTALL_PHASE=all` and does not under `deps`/`config` (fits the existing
  `install_test.sh` / shell test-driver pattern).
- **Human-evidenced gates (cannot be unit-tested).** F7 and F8 require real transcripts: an
  actual update of the stale host, and a dirty-clone skip plus a `--force` rescue. Recorded
  under `plans/fleet/evidence/` per `superpowers:verification-before-completion`.

## 7. Prerequisites / dependencies

- Go toolchain (pinned via `.go-version`) and the `sdk/` build convention (`build.sh` →
  `~/opt/bin`), plus a `gff_on install.sdk.fleet` block in `install.sh` and `fleet` added to
  the root `Makefile` tool loop (currently `gss gsl wol tmux-mgr`).
- cobra (as used by the other `sdk/` CLIs) and Bubble Tea for the TUI.
- SSH access to the hosts via `~/.ssh/config` aliases with non-interactive auth (keys).
- Each host keeps its dotfiles clone at `~/git/dotfiles` (true for all current hosts).

## 8. Out of scope (and why)

- **Daemon / polling / scheduling** — the requirement is explicitly on demand; a daemon adds
  lifecycle and failure modes for no stated benefit.
- **Committed host inventory** — explicitly excluded; `~/.ssh/config` is the source.
- **Unattended remediation** — `install.sh` needs credentials and human judgement; automating
  it unattended is the highest-blast-radius thing we could build and nobody asked for it.
- **Managing non-dotfiles state** (packages, services) — different problem, different tool.
- **Retroactive history** — the stamp only knows about runs after it ships; reconstructing
  past installs is not possible and not worth approximating.

## 9. Rollback

Revert the stamp commit (~5 lines at the end of `install.sh`); stray stamp files on hosts are
inert. Delete `sdk/fleet` plus its `gff_on install.sdk.fleet` block and Makefile entry; the
binary in `~/opt/bin` can be removed on next install or by hand. No schema, service, or
shared state is introduced, so there is nothing to migrate.

> Produced via `superpowers:brainstorming`. The matching plan goes in `../plans/fleet.md`.
> Register / update `../index.md`.
