# Account-scoped, dotfiles-managed memory — design

- **Slug:** memory-provisioning
- **Date:** 2026-06-08
- **Status:** Proposed
- **Relates to:** issue #134
- **Author(s):** architecture team (research · sysarch · principal · secarch · adversary) via Workflow

## 1. Problem / context

We want Claude Code memory that is **account-scoped** (every session in this repo gets it), **dotfiles-managed** (the repo holds canonical memories; install provisions them; editing a memory in the repo updates every instance on next install), with a **capture→PR pattern** to save new memories back into the repo. This extends the merged `ai-config-home-provisioning` convention (PR #113): *copy into well-known `$HOME` paths; copy is the forward mechanism; no new symlinks; `settings.json` uses a forced-field merge that preserves host-local values.*

Two premises had to be checked against disk state before designing, because the objective asserted them as done. Both were re-verified this session:

- **Claude Code has NO account-scoped memory.** The store is exclusively *project-scoped* at `~/.claude/projects/<slug>/memory/`, where `<slug>` is the repo's absolute path with `/`→`-` (verified: `printf '%s' /home/<user>/git/dotfiles | sed 's#/#-#g'` → `-home-<user>-git-dotfiles`, which matches the live directory byte-for-byte). `~/.claude/memory/` does not exist. The slug embeds the username and absolute path, so the same repo on another machine/user keys to a *different* folder, and memory does not sync across machines today. Research confidence on this mechanism is **high** and independently confirmed by the adversary against the live filesystem.
- **The "bootstrap already committed" claim is FALSE.** Verified this session: `ls ai/claude/memory/` → *No such file or directory*; `git ls-files | grep -i memory` → empty. The directory does not exist in the repo and zero memory files are tracked. **Step zero of any implementation is to create the canonical store with per-file-triaged, identity-scrubbed content** — nothing has been scrubbed or committed yet.

Consequences that shape the design:

- `~/.claude` is not a git repo and nothing syncs it. `install.sh` provisions `settings.json` (forced-field merge), `hooks/` (copy), and `skills/` (sync-skills), but **never touches `~/.claude/projects/`**. "Account-scoped" must therefore be **synthesized at install time** by copying a canonical seed into the per-machine, computed slug folder — it is an install-time fan-out, not a single shared store.
- `MEMORY.md` (first 200 lines / 25 KB) is loaded at session start; individual topic `.md` files are recalled on demand. So `MEMORY.md` is the index that actually reaches context.
- These memories push to a **shared org repo**. A `privacy_guard` PreToolUse hook blocks committing a literal home path or login username — but, verified by reading the hook, it does **not** protect this flow as written (see §5). Identity-scrubbing must be an explicit capture-time transform, not a reliance on the guard.

## 2. Goals & non-goals

**Goals**
- A git-tracked canonical memory store in the repo that seeds every machine on install.
- Per-machine provisioning that **computes** the live slug (never hardcodes it) and **merges rather than clobbers** — locally-grown memories survive, mirroring the settings forced-field precedent at file granularity.
- A capture→PR pattern that scrubs identity before anything reaches the shared org repo, routed through the existing `gss` writer.
- An explicit **portable vs host-local** split so machine-specific state never becomes account-global.

**Non-goals**
- Real-time cross-machine sync (no daemon, no symlink-into-slug, no writing into a tracked worktree at session time).
- A bespoke git path — `gss` remains the single writer.
- **Gemini parity is out of scope for this design** (see §4, "Gemini parity verdict"). Claude's per-project-slug model and Gemini's single-file `~/.gemini/GEMINI.md` model differ structurally, and the live Gemini config path (`~/.agents` vs `~/.gemini`) is unverified.

## 3. Options considered

Lead recommendation: **copy a canonical seed into the per-machine computed slug, seed-and-preserve.** Two rejected alternatives below.

**Option A — Copy-into-computed-slug + seed-and-preserve (RECOMMENDED).**
Canonical memories live at `ai/claude/memory/*.md` (git-tracked, scrubbed). On install, compute the live slug from the resolved repo checkout, `mkdir -p` the destination, and `cp` only the files the repo ships. Host-local `.md` files with no repo counterpart are never written and never deleted — they survive untouched (the directory analogue of "preserve undeclared host fields"). The live `MEMORY.md` is **regenerated** from the union of provisioned-canonical entries and surviving host-local files, never blind-copied.
*Trade-offs:* duplicates the seed onto each machine (acceptable — it is a seed, not a sync target); index correctness becomes the capture step's responsibility. *Why it wins:* it is exactly the established hooks-copy pattern; "copy is the forward mechanism"; the live store stays host-owned and writable; the repo stays the reviewed source of truth.

**Option B — Symlink the repo into every slug at session init (REJECTED).**
*Trade-offs:* would make memories identical across machines automatically, but (a) violates the "no new symlinks" directive, (b) lets the harness write session-discovered, possibly secret-bearing memories straight into a tracked worktree with no scrub gate, and (c) couples a global path to the checkout location, breaking worktrees/CI. Rejected by all three lenses and the adversary.

**Option C — Provision the seed into every existing project slug (REJECTED).**
*Trade-offs:* would broadcast memory to all repos, but pollutes unrelated projects' memory with dotfiles-specific lore. This repo's memory is dotfiles-specific, not universal. Rejected.

**Account-scope mechanism — trade-off and confidence/fallback.**
The mechanism *is* the copy-into-computed-slug fan-out (Option A). The slug is **computed**, not hardcoded: `SLUG = "$(printf '%s' "$REPO_ABS" | sed 's#/#-#g')"`, with `REPO_ABS` resolved from the real checkout. This is the one spot needing a **verification step rather than an asserted fact**: `install_claude_skills.sh` currently derives `BASE_DIR` via plain `pwd`, *not* `pwd -P`. A symlinked or network-mounted checkout could compute a slug that differs from the one Claude actually keys on, silently provisioning into a phantom directory. The mechanism's *existence* is high-confidence; the *slug equality across all platforms* (macOS `/Users`, WSL `/mnt/c`, symlinked homes) is **not** universally verified — it is confirmed only on local Linux `/home`. **Recommended approach + verified fallback:**
- **Recommended:** compute the slug from `realpath`/`pwd -P` of the main checkout.
- **Verification/fallback (mandatory):** before writing, **discover** the live slug by listing `~/.claude/projects/` and matching the computed slug against the existing entries. On a match, provision. On **no** match, *warn and skip* rather than create an orphan folder. This turns a possibly-wrong construction into a checked one and is the install step's first acceptance criterion.

## 4. Decision

**Canonical repo path.** `ai/claude/memory/*.md` — one scrubbed `.md` per memory plus a generated account `MEMORY.md` index. **Verified git-trackable with no new `.gitignore` rule:** `git check-ignore -v ai/claude/memory/foo.md` reports `.gitignore:84:!ai/** …`, so the existing `!ai/**` allowlist already opts the path in (do not add a redundant rule; do not `git add -f`). The directory **does not exist yet and must be created first** with per-file-triaged, scrubbed content. Naming it `ai/claude/memory/` (not `ai/memory/`) keeps a sibling `ai/gemini/memory/` open later without entangling the two.

**Account-scope mechanism.** Synthesized at install time by copying the canonical seed into `~/.claude/projects/<computed-slug>/memory/`. The slug is computed per-machine and cross-checked against the live `~/.claude/projects/` listing (§3); no hardcoded slug, no `$USER`/`$HOME` literal baked in.

**Install provisioning — computes per-machine paths, merges not clobbers.** Add one block to `opt/scripts/system/install_claude_skills.sh`, beside the existing hooks-copy block (the canonical Claude-config owner, already invoked by `install.sh` and re-run by sync/install). Do **not** modify `install.sh`'s top-level body. The block:
1. Derive `MEM_SLUG` from the resolved main checkout (`pwd -P`/`realpath`), **cross-check against `~/.claude/projects/`, warn-and-skip on no match** (the verification step from §3).
2. `mkdir -p "$MEM_DEST"` (pre-seeds a fresh machine where Claude has never opened the repo).
3. For each `ai/claude/memory/*.md` **except `MEMORY.md`**, and **only** files tagged `scope: account` (see split below): `cp` into `$MEM_DEST` (repo-canonical files win — "edit in repo → updates every instance"). **Never delete** files already present that the repo doesn't ship (host-local memories survive — the file-granular forced-merge precedent: repo-declared files replaced, undeclared host files preserved). On a basename collision between a repo-canonical file and a pre-existing host-local file, **warn and skip rather than overwrite**.
4. **Regenerate** `$MEM_DEST/MEMORY.md` from the **union** of account entries ∪ surviving host-local files — never blind-copy the repo index (this closes the verified index-clobber gap, where copying the repo's `MEMORY.md` would drop host-only entries from session-start context while their topic files sit unreachable on disk).
Keep `install.sh` "dumb" (copy + regenerate-from-union); **no rehydration** — memories stay in `$HOME`/`<user>` placeholder form in the live store, and Claude resolves them contextually. This avoids round-trip drift.

**The capture→PR pattern.** A thin skill (designed via `docs/mbo`, authored via `skill-creator`), honoring the two-call `gss` recipe and mandatory `AskUserQuestion` confirmation:
1. Read the candidate memory from the computed live slug dir.
2. **Refuse** any `scope: host-local` file.
3. **Scrub identity-agnostically** (see guardrails) — this is a transform, not a `cp`.
4. **Hard-fail** if any identity or secret pattern survives the scrub.
5. **Write the scrubbed body via the Write tool** to the tracked `ai/claude/memory/<name>.md` path — *not* a shell `cp` (verified: `privacy_guard`'s content scan fires only on a Write/Edit to a tracked file; a `cp` + `git commit` is never body-scanned).
6. Regenerate the committed account `MEMORY.md`.
7. Hand to the existing **`gss`** skill for the commit + draft PR (single writer; two-call token recipe; stage by explicit name, never `git add -A`). Human review on the PR is the final gate before a memory becomes org-canonical.
On merge, the next `install.sh` on any machine copies the new canonical memory into that machine's slug — closing the edit-in-repo → update-every-instance loop.

**Identity-scrub guardrails.** Canonical memories push to a shared org repo, so scrubbing is load-bearing and **must not** lean on `privacy_guard` as the safety net. Verified gaps in the guard for *this* flow: it **skips non-repo targets** (so writes to `~/.claude/projects/` are never scanned) and **only matches this host's** literal `USER`/`HOME`/`HOST` (so a foreign username or teammate's `/home/<name>` path sails through), and `git commit` scans only the message, not staged file bodies. Therefore the capture step runs an **identity-AGNOSTIC** scrub: replace any `/home/<x>`, `/Users/<x>`, `/mnt/<d>/Users/<x>`, and bare-token home paths with `~`/`${HOME}`/`<user>` placeholders regardless of owner, plus reuse `privacy_guard`'s secret regexes (PEM, `AKIA`/`ASIA`, `ghp_`, `github_pat_`, `xox[baprs]-`, `AIza`, and the password/token assignment heuristic). The guard remains a fail-safe on the tracked Write only; the scrub is the real gate.

**Host-local vs portable split.** Declared explicitly via `scope:` front-matter, **default `host-local` (fail closed)** — an untagged memory is treated as un-shareable until a human marks it `account`. `scope: account` → scrubbed, repo-canonical, identical across machines, eligible for install-time provisioning. `scope: host-local` → lives only in the originating slug, never staged, never clobbered by install, never promoted. Per-file triage is mandatory at bootstrap: at least the workflow/transient memories (e.g. the gss-autonomous-backlog note referencing a specific PR range and a resumable `STATE.md`, and the tmux-mgr-rebuild note) are **host/workflow-specific and must be `host-local`** — the 7 live files must **not** be bulk-copied into the repo.

**Gemini parity verdict — OUT of scope.** Claude's per-project-slug directory model and Gemini's global single-file `~/.gemini/GEMINI.md` model are structurally different, and whether this repo's Gemini config lives at `~/.agents` or `~/.gemini` is unverified this session. Forcing symmetry now would be speculative. Provision Gemini separately as its own objective once the live path is verified; the `ai/claude/memory/` naming leaves room for a sibling `ai/gemini/memory/`.

## 5. Risks & blast radius

- **Identity/PII or secret leak into the shared org repo (high).** `privacy_guard` does not scan the live store and only catches this host's identity; `git commit` doesn't body-scan. *Mitigation:* identity-agnostic capture-time scrub + secret-regex pass + hard-fail-on-residual, performed via a tracked-path Write; refuse `scope: host-local`. Blast radius without this: a foreign username/path/token published org-wide.
- **Host-specific memory becoming account-global (high→medium).** Inferred splits sweep machine-specific state into the portable set. *Mitigation:* explicit `scope:` front-matter, default `host-local`; per-file bootstrap triage; install copies only `scope: account`.
- **`MEMORY.md` index drift / clobber (medium).** Blind-copying the repo index drops host-only entries from session context. *Mitigation:* install **regenerates** the live index from the union; capture owns the committed account index. Do not smart-merge in `install.sh`.
- **Slug mismatch on symlinked/network/cross-OS homes (medium).** Plain `pwd` may differ from Claude's key. *Mitigation:* `realpath`/`pwd -P` + cross-check against the live `~/.claude/projects/` listing, warn-and-skip on no match.
- **Worktree slugs differ (low, expected).** Verified: gss worktrees key to their own slugs. Install runs only from the main checkout (worktree-safety rule), so provisioning lands only in the main repo's slug; a session inside a worktree gets a separate empty store. **Document as expected behavior; do not fan out.**
- **Fresh-machine pre-seed (low).** `mkdir -p` creates the path before first session. *Acceptance criterion:* confirm Claude loads a pre-existing `MEMORY.md` it did not create.
- **Capture skill tripping the chained-token safety guard (low).** A one-shot scrub+stage+commit+push trips the block. *Mitigation:* thread through `gss` honoring the two-call recipe.

Blast radius of the install change itself is tight: one new copy block in one already-invoked script; pure overwrite-of-shipped-files plus index regeneration; no `install.sh` body change; idempotent.

## 6. Rollback

- **Per-machine, immediate:** the install block only writes `scope: account` files the repo ships and never deletes host-only files. To undo on a host, delete the provisioned canonical files from `~/.claude/projects/<slug>/memory/` (host-local memories are untouched) and regenerate `MEMORY.md`; the next install re-seeds.
- **Repo-level:** revert the `install_claude_skills.sh` block and remove `ai/claude/memory/`. Because nothing symlinks `~/.claude` and the live store is host-owned, removing the canonical store has no destructive effect on any machine beyond stopping future seeding.
- **Capture path:** entirely additive (a new skill routed through `gss`); disabling it removes the write-back loop without affecting provisioning or existing memories.
- No data migration, no schema, no service — rollback is file deletion plus index regeneration.

> Produced via an architecture-team `Workflow` (research · sysarch · principal · secarch · adversary). Register in `../index.md`. Spec → `../specs/memory-provisioning.md`.
