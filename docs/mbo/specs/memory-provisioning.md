# Account-scoped, dotfiles-managed memory — spec

- **Slug:** memory-provisioning
- **Date:** 2026-06-08
- **Status:** Approved
- **Relates to:** issue #134 · PR #135 · design `../designs/memory-provisioning.md`

## 1. Goal

On `install.sh`, the repo's canonical Claude memories (`ai/claude/memory/*.md`, `scope: account`)
are **provisioned into this machine's live project-memory store** — `~/.claude/projects/<computed-slug>/memory/` —
so the same memories are present in every session on every machine, **without** clobbering
memories grown locally on a host. The live `MEMORY.md` index is regenerated to include both the
account entries and any host-local ones. A later capture→PR flow writes new memories back to the
repo (tracked separately; see §8).

## 2. Use cases

**UC-1 — Fresh machine (pre-seed).** *Actor:* `install.sh` on a host where Claude has never opened
this repo. *Flow:* the provisioner computes the slug from the real checkout, `mkdir -p`s the live
memory dir, copies the `scope: account` files, and writes `MEMORY.md`. *Acceptance:* the account
memories + index exist at `~/.claude/projects/<slug>/memory/` before the first session.

**UC-2 — Established machine (seed-and-preserve).** *Actor:* `install.sh` where the live store
already holds a host-local memory (`scope: host-local`, no repo counterpart). *Flow:* account files
are (re)written from the repo; the host-local file is **never deleted**; `MEMORY.md` is regenerated
to list **both**. *Acceptance:* account files match the repo, the host-local file and its index line
survive.

**UC-3 — Edit-in-repo propagates.** *Actor:* an account memory edited in the repo, then `install.sh`.
*Flow:* the file is re-copied (repo wins for files it ships). *Acceptance:* the live account file
equals the repo version after install.

**UC-4 — Collision guard.** *Actor:* a repo account file whose basename matches a pre-existing
**host-local** live file. *Flow:* warn and **skip** (do not overwrite the host's file). *Acceptance:*
the host-local file is preserved; a warning is emitted.

**UC-5 — Slug-mismatch signal.** *Actor:* a host where the computed slug dir does not exist but a
sibling project dir plausibly maps to this repo (symlinked/relocated checkout). *Flow:* provision
into the computed (`pwd -P`) slug and emit a **warning** so the user can verify. *Acceptance:* a
warning names the computed slug; install does not silently no-op.

## 3. Architecture

- **Canonical store:** `ai/claude/memory/*.md` (git-tracked, scrubbed, `scope:` front-matter) +
  `MEMORY.md` account index. Already present (the captured bootstrap).
- **Provisioner:** a new standalone helper `opt/scripts/system/provision-claude-memory.sh`
  (mirrors the `apply-forced-settings.sh` precedent — complex provisioning lives in its own
  testable script, not inline in `install_claude_skills.sh`). Pure function of `BASE_DIR` + `HOME`.
- **Wiring:** one invocation block in `install_claude_skills.sh`, beside the hooks-copy block.
- **Index regeneration:** live `MEMORY.md` = repo account index ⧺ generated lines for surviving
  host-local files (those in the live dir with no repo counterpart).

## 4. Behavior / features

- **F1** Compute the live slug from the **resolved** checkout (`pwd -P`/`realpath`): `slug = REPO_ABS` with `/`→`-`.
- **F2** Copy only `scope: account` files (skip `MEMORY.md`, skip non-account) into the live dir; `mkdir -p` first (pre-seed).
- **F3** Seed-and-preserve: never delete a live file the repo doesn't ship.
- **F4** Collision guard: a repo account basename matching a pre-existing host-local live file → warn + skip.
- **F5** Regenerate live `MEMORY.md` from the union (account index ⧺ host-local lines), never blind-copy.
- **F6** Slug-mismatch warning (UC-5); idempotent re-runs.
- **F7** Wire the provisioner into `install_claude_skills.sh` (runs on every `install.sh`).

## 5. Evaluation criteria (per feature)

| Feature | Fires (must) | Must-not | Pass predicate (→ test) |
| :-- | :-- | :-- | :-- |
| F1 slug | `slug == "$(printf %s "$REPO_ABS" \| sed 's#/#-#g')"`, `REPO_ABS` from `pwd -P` | hardcoded slug; `$USER`/`$HOME` literal | unit: stub a checkout path → expected slug |
| F2 copy | account files land in `~/.claude/projects/<slug>/memory/` | `MEMORY.md` or non-account files copied | fresh-HOME run → account files present, index present |
| F3 preserve | a pre-existing host-local file survives a run | host-local file deleted/emptied | seed host-local → run → still present |
| F4 collision | host-local file with an account basename is **not** overwritten + warning | host file overwritten | seed host-local `X.md` (account also ships `X.md`) → run → host content intact, warns |
| F5 index | regenerated `MEMORY.md` lists account **and** host-local entries | host-local entry dropped from index | seed host-local → run → grep both in `MEMORY.md` |
| F6 idempotent | second run = first run (no dupes in index) | duplicated index lines | run twice → `MEMORY.md` identical |
| F7 wiring | `install_claude_skills.sh` invokes the provisioner | — | `install_claude_skills_test.sh`: after install, account memory present in throwaway `$HOME` |

## 6. Verification harness

- **Shell tests** via `make shell-test` (discovers `*_test.sh`, uses `ai/_test_helpers.sh`):
  a new `opt/scripts/system/provision-claude-memory_test.sh` (F1–F6, throwaway `$HOME`), and an
  extension to `install_claude_skills_test.sh` (F7 — end-to-end through the installer).
- **Human-evidenced:** run `install.sh` in a sandbox `$HOME` and confirm
  `~/.claude/projects/<slug>/memory/` holds the account set + a union `MEMORY.md`
  (`superpowers:verification-before-completion`).
- **Privacy gate:** the canonical files are already identity-scrubbed; CI/`privacy_guard` remain a
  fail-safe on tracked Writes (not the primary gate — see design §5).

## 7. Prerequisites / dependencies

None new. POSIX shell + coreutils (`sed`, `awk`, `cp`, `mkdir`). No `jq` (memory files are markdown,
not JSON). Reuses `ai/_test_helpers.sh`.

## 8. Out of scope (and why)

- **The capture→PR skill** — the write-back half. Valuable but a distinct `skill-creator`
  deliverable with its own evals; built as a **stacked follow-up** (plan §6.1) so this PR stays a
  reviewable provisioning unit. The manual capture pattern (demonstrated in PR #135) works meanwhile.
- **Gemini parity** — design §4: out of scope until `~/.agents` vs `~/.gemini` is verified.
- **Rehydration** (expanding `$HOME`/`<user>` in the live store) — design §4: store-and-serve placeholders.

## 9. Rollback

Revert the `install_claude_skills.sh` block + delete `provision-claude-memory.sh`; the live store is
host-owned and nothing is symlinked, so removal just stops future seeding. Per-host: delete the
provisioned account files and regenerate `MEMORY.md` (host-local memories untouched).

> Produced from the design. Plan → `../plans/memory-provisioning.md`. Update `../index.md`.
