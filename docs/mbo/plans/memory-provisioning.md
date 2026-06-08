# Account-scoped, dotfiles-managed memory — implementation plan

- **Slug:** memory-provisioning
- **Date:** 2026-06-08
- **Status:** In-progress
- **Relates to:** spec `../specs/memory-provisioning.md` · issue #134 · PR #135

## 1. Summary & verdict

Build the **provisioning half**: a standalone `provision-claude-memory.sh` that copies the
`scope: account` canonical memories into this machine's computed live slug dir (seed-and-preserve,
union index), wired into `install_claude_skills.sh`. The **capture→PR skill** is a stacked
follow-up (§6.1).

**Design verdict folded in:** account-scoping is synthesized at install (no native account memory);
slug computed from `pwd -P` (closes the adversary's `pwd` gap); seed-and-preserve never clobbers
host-local; MEMORY.md regenerated from the union (closes the index-clobber gap); canonical files are
identity-scrubbed (the privacy hole the adversary caught is already fixed in the captured bootstrap).

**Reconciliation note (design §3 tension).** The design asked for both "warn-and-**skip** on slug
no-match" and "`mkdir -p` to pre-seed a fresh machine" — these conflict (a fresh machine is always a
no-match). Resolution: compute the slug with `pwd -P` (which is what actually fixes the symlink risk,
since it resolves the path the way the session cwd does), **always provision** into it (so fresh
machines pre-seed), and downgrade the no-match case to a **warning**, not a skip. Documented here and
in the script.

## 2. File inventory

| Path | Purpose | Implements |
| :-- | :-- | :-- |
| `opt/scripts/system/provision-claude-memory.sh` *(new)* | the provisioner: slug → copy account files → union index | F1–F6 |
| `opt/scripts/system/provision-claude-memory_test.sh` *(new)* | shell test driver (throwaway `$HOME`, `ai/_test_helpers.sh`) | F1–F6 |
| `opt/scripts/system/install_claude_skills.sh` | add one invocation block beside the hooks-copy block | F7 |
| `opt/scripts/system/install_claude_skills_test.sh` | extend: assert account memory provisioned end-to-end | F7 |
| `ai/claude/memory/*.md` *(present)* | canonical scrubbed `scope:account` store + index | store |
| `opt/scripts/system/GEMINI.md` | one line documenting the new script (per-dir docs rule) | docs |

No `install.sh` body change (it already calls `install_claude_skills.sh`). No `scripts/test.sh`
change (`make shell-test` discovers `*_test.sh`).

## 3. Interface contracts

`provision-claude-memory.sh` — pure function of env, no args:
```sh
# Inputs:  BASE_DIR (repo root; default: derived via pwd -P from $0), HOME, CLAUDE_HOME=$HOME/.claude
# Effect:  writes ~/.claude/projects/<slug>/memory/{<account>.md, MEMORY.md}
# Slug:    REPO_ABS="$(cd "$BASE_DIR" && pwd -P)"; SLUG="$(printf '%s' "$REPO_ABS" | sed 's#/#-#g')"
# Account: a file is account iff its front-matter matches /^[[:space:]]*scope:[[:space:]]*account/
# Copy:    cp each repo account file (except MEMORY.md) into the live dir, UNLESS a same-named
#          live file exists that is NOT account (host-local) -> warn + skip (F4).
# Index:   live MEMORY.md = (repo ai/claude/memory/MEMORY.md) followed by a generated line
#          "- [<name>](<base>) — <description>  <!-- host-local -->" for every live *.md that has
#          no repo counterpart (host-local), skipping MEMORY.md. Idempotent.
# Exit:    0 always on a reachable HOME; warnings to stderr; never deletes host files.
```
Front-matter readers (`name:`, `description:`, `scope:`) use `awk`/`grep` on the leading `---` block.

## 4. TDD build order

**Phase 1 — slug (F1).** *Test first:* `provision-claude-memory_test.sh` asserts the script, given a
known `BASE_DIR`, targets `…/projects/<expected-slug>/memory` (probe via a dry-run/echo mode or by
observing where files land). *Then:* implement slug computation. *Done-when:* slug test green.

**Phase 2 — fresh-machine copy (F2, UC-1).** *Test first:* fresh `$HOME`, run → account files +
`MEMORY.md` exist under the computed slug; `MEMORY.md` (the repo index file) is **not** copied as a
topic file, non-account files not copied. *Then:* implement `mkdir -p` + the account-filtered copy.
*Done-when:* fresh-run asserts pass.

**Phase 3 — seed-and-preserve + union index (F3, F5, UC-2).** *Test first:* pre-seed a host-local
`local-note.md` (`scope: host-local`) + a stale live `MEMORY.md`; run → account files present,
`local-note.md` preserved, regenerated `MEMORY.md` greps **both** an account entry and `local-note`.
*Then:* implement union-index regeneration. *Done-when:* preserve + union asserts pass.

**Phase 4 — collision guard + idempotency (F4, F6).** *Test first:* (a) seed a host-local file whose
basename equals a shipped account file → run → host content intact + warning; (b) run twice → final
`MEMORY.md` identical (no dup lines). *Then:* add the collision skip + make regeneration deterministic.
*Done-when:* both pass.

**Phase 5 — wiring (F7).** *Test first:* extend `install_claude_skills_test.sh` — after
`run_install "$A"`, assert an account memory file exists under
`$A/.claude/projects/<slug>/memory/`. *Then:* add the invocation block to `install_claude_skills.sh`
(compute `BASE_DIR` already available; call the provisioner). *Done-when:* installer test green.

**Phase 6 — gate.** `make shell-test` green; `bash opt/scripts/system/provision-claude-memory_test.sh`
green; human-evidenced sandbox `install.sh` run shows the live store seeded; `GEMINI.md` line added.

## 5. Verification mapping

| Spec rule | Test |
| :-- | :-- |
| F1 slug from `pwd -P` | `test: slug computation` |
| F2 account-only copy + pre-seed | `test: fresh HOME provisions account set` |
| F3 preserve host-local | `test: host-local survives` |
| F4 collision warn-skip | `test: account basename collision preserves host file` |
| F5 union index | `test: MEMORY.md lists account + host-local` |
| F6 idempotent | `test: second run identical` |
| F7 installer wiring | `install_claude_skills_test.sh: account memory provisioned` |

## 6. Integration & rollout

- `make shell-test` runs the new driver; no discovery change needed.
- `install.sh` → `install_claude_skills.sh` → `provision-claude-memory.sh` (transitive; no `install.sh` edit).
- Document the new script in `opt/scripts/system/GEMINI.md`.
- Manual acceptance: sandbox `HOME=$(mktemp -d) bash install_claude_skills.sh`; confirm the live
  store + union index; re-run to confirm idempotency.

### 6.1 Build leaves / DAG

This PR builds **one leaf** (provisioning), sequentially — the script + its test + the installer
wiring share files and are one cohesive unit. **Not broken out.**

The **capture→PR skill** is a *separate, stacked* objective leaf (its own `gss` worker on this
feature, based on this branch), because it's a distinct `skill-creator` deliverable (a `SKILL.md` +
identity-agnostic scrub logic + `evals/evals.json`) that *consumes* this leaf's frozen interface
(the `ai/claude/memory/` location + `scope:` front-matter). Sequence: **provisioning (this PR) →
capture skill (next)**.

| Leaf | Owns (paths) | Consumes | done-when | Blocking? |
| :-- | :-- | :-- | :-- | :-- |
| provisioning | `provision-claude-memory.sh(+test)`, install wiring | `ai/claude/memory/` + `scope:` | `make shell-test` green + sandbox install evidence | yes (base) |
| capture-skill | `ai/skills/memory-capture/**` | provisioning's store + scope contract | skill evals + scrub hard-fail test | no (follow-up) |

> Produced from the spec. Execute with TDD (`superpowers:test-driven-development`). Update `../index.md`.
