# AI config provisioning into `$HOME` — convention + hook-wiring fix

- **Status:** Proposed (findings + plan for review) — drafted 2026-06-02
- **Relates to:** issue #111 (*safety_guard hook silently not installed*)
- **Authors:** design pass with the architecture team (systems + security architects)
- **Decision owner:** repo owner — settings-model fork resolved in favor of **D2** ([§7](#7-decision-settings-model-owner-approved))

---

## 1. TL;DR — reframing the root cause

Issue #111 frames the bug as two compounding problems: (1) `settings.json` is
*seed-once, never reconciled*, and (2) hooks are referenced by a hardcoded
repo-internal absolute path. Both are real, but the issue's framing misses the
deeper principle and even its *correct* template still violates it.

The **tracked** template `ai/claude/settings.json.template` (lines 70 & 79)
already points the PreToolUse hooks at:

```
$HOME/git/dotfiles/ai/hooks/safety_guard.sh
$HOME/git/dotfiles/ai/hooks/privacy_guard.sh
```

So even a host seeded *today, correctly, from the current template* references
the repo's internal layout and checkout location. The path moving from
`ai/claude/hooks/` → `ai/hooks/` was the *trigger*; the **root cause is that the
global Claude config reaches into the repo at all**.

Everything else Claude installs already does the right thing:

| Artifact | How `settings.json` references it | How it lands in `$HOME` |
|---|---|---|
| statusLine | `~/.claude/statusline-command.sh` ✅ | **symlink** repo shim → `~/.claude/` |
| commands | `~/.claude/commands/*.md` ✅ | **symlink** each into `~/.claude/commands/` |
| skills | `~/.claude/skills/**` ✅ | **symlink** via `sync-skills.sh` |
| **hooks** | `$HOME/git/dotfiles/ai/hooks/*.sh` ❌ | *not installed* — referenced in place |

`install_claude_skills.sh` even comments the exception out loud:
*"We don't symlink hooks — settings.json points at them in-place."* **That
exception is the defect.** Hooks are the one artifact that didn't follow the
established pattern, and that's why a repo reorg silently broke them.

## 2. The design directive (owner, authoritative)

> There are exactly **two** sanctioned ways to populate `$HOME` and configure
> the AI tools we support:
> 1. a **symlink** placed in `$HOME` that references `git/dotfiles`, or
> 2. a **file copied** directly into a well-known path under `$HOME` (e.g. `~/.claude/`).
>
> Settings/config files must reference **well-known `$HOME` paths**
> (`~/.claude/hooks/…`) and **must not** reference repo-internal paths like
> `$HOME/git/dotfiles/ai/hooks/…`.

This is the same portability rule already in `CLAUDE.md` ("the repo can be
cloned anywhere and the config must still work") — applied to the AI-tool
global config, which currently breaks it.

## 3. Findings — every place global config reaches into the repo

| # | File | Line | Reference | Verdict |
|---|---|---|---|---|
| F1 | `ai/claude/settings.json.template` | 70 | `$HOME/git/dotfiles/ai/hooks/safety_guard.sh` | ❌ repo-internal path in global config |
| F2 | `ai/claude/settings.json.template` | 79 | `$HOME/git/dotfiles/ai/hooks/privacy_guard.sh` | ❌ repo-internal path in global config |
| F3 | `ai/claude/settings.json.template` | 3 | `env.DOTFILES_DIR=$HOME/git/dotfiles` | ⚠️ dead (referenced nowhere) |
| F4 | `opt/scripts/system/install_claude_skills.sh` | ~62–66 | "We don't symlink hooks" | ❌ hooks are the lone un-installed artifact |
| F5 | `opt/scripts/system/install_claude_skills.sh` | ~33–37 | seed-once `if [ ! -f settings.json ]` | ❌ template changes never reach provisioned hosts |
| F6 | `ai/claude/scripts/sanity_check.sh` | 64 | validates `$BASE_DIR/ai/hooks/safety_guard.sh` (repo path) | ❌ never validates the *configured* path in `~/.claude/settings.json`; passes green while unwired; `privacy_guard` not checked at all |
| F7 | `ai/gemini/scripts/sanity_check.sh` | 57 | `$HOME/git/dotfiles/ai/claude/scripts/sanity_check.sh` | ❌ hardcoded repo path — breaks on any non-`~/git/dotfiles` clone |
| F8 | `ai/gemini/settings.json` | 25 | `$GEMINI_PROJECT_DIR/ai/hooks/privacy_guard.sh` | ⚠️ repo-relative via Gemini runtime var — *different mechanism* (see §6); only `privacy_guard` wired, no `safety_guard` |

**Two silent failures stack:** F5 means the dead path never gets corrected on
existing hosts; F6 means our own health check can't see it. A host looks
configured and passes sanity_check while both security hooks are absent.

## 4. The convention, formalized

For every AI tool we support, a config/settings file may reference an artifact
**only** by a well-known `$HOME` path. The artifact arrives in `$HOME` by
exactly one of:

1. **Symlink** — `install_*_skills.sh` links `<repo>/…` → `~/.<tool>/…`. Use for
   anything that should track the repo live (hooks, statusline, commands,
   skills). The symlink — not the config file — carries the repo location, so
   config is checkout-path-independent.
2. **Copy** — a file written once into a well-known path and thereafter owned by
   the host (e.g. `~/.claude/settings.local.json` for `apiKeyHelper` / base-URL
   / `enabledPlugins`). Use only for genuinely host-local state that must *not*
   track the repo.

Config files reference `~/.<tool>/…`. They never reference
`$HOME/git/dotfiles/…`, `$DOTFILES_DIR/…`, or any repo-relative path.

## 5. Recommended design (architecture team)

### D1 — Hooks become symlinks into `~/.claude/hooks/`, mirroring statusLine
`install_claude_skills.sh` symlinks each `ai/hooks/*.sh` → `~/.claude/hooks/<name>.sh`
(backing up any pre-existing plain file to `.bak`, the same idiom the statusLine
block already uses), and the tracked config references the well-known path:

```json
"command": "$HOME/.claude/hooks/safety_guard.sh"
"command": "$HOME/.claude/hooks/privacy_guard.sh"
```

Path-independent; works in worktrees, CI, and alternate clone locations. Drop
the dead `env.DOTFILES_DIR` (F3).

### D2 — Invert the settings model (owner-approved; see §7)
Make `ai/claude/settings.json` a **tracked** file (it is already the symlink
*target* — a gitignored "host copy" that every host symlinks to is a
contradiction). Move genuinely host-local fields into
`~/.claude/settings.local.json`, which Claude Code already merges over
`settings.json` and which the installer never overwrites (convention option 2).

This **dissolves the reconcile problem at its root** (F5): immutable security
wiring lives in the tracked, symlinked `settings.json` and updates on every
`git pull` with zero reconcile logic — there is nothing to seed, so nothing
drifts.

### D3 — Validation reads the **live, configured** path and *exercises* the hook
- Install-time (`install_claude_skills.sh`) and `sanity_check.sh`: parse
  `~/.claude/settings.json` with `jq`, extract every `hooks[].hooks[].command`
  and `statusLine.command`, expand `$HOME`/`~`, and **fail loud** if any
  resolved path is not an executable regular file. Also assert each
  `~/.claude/hooks/` symlink resolves *into the dotfiles checkout* (reject
  out-of-tree targets).
- **Behavioral check (security team, strong rec):** sanity_check should drive
  the hook **through the configured command string**, not the repo path — feed
  `safety_guard.sh` a known-bad payload and assert exit 2 (block) and a
  known-good payload and assert exit 0 (allow); same for `privacy_guard.sh`.
  A test that hardcodes `ai/hooks/safety_guard.sh` (today's F6) passes green
  while the *reference* from settings.json is broken — the test must traverse
  the reference, because the reference is what breaks.

### D4 — Gemini stays on `$GEMINI_PROJECT_DIR` (documented asymmetry)
Do **not** force `~/.gemini/hooks/` symlinks for false symmetry.
`$GEMINI_PROJECT_DIR` is a harness-provided *runtime* variable resolved
per-invocation against the active project — the Gemini-native equivalent of
well-known indirection, not a hardcoded `~/git/dotfiles`. Keep it, extend the
Gemini sanity_check (D3-style), **fix F7** (the hardcoded cross-call path), and
record the asymmetry in `ai/GEMINI.md` so it's a decision, not an accident.

### D5 — Optional hardening: a fail-closed dispatcher (security team)
The strongest defense against *future* drift: point settings at one stable
dispatcher (`~/.claude/hooks/pretooluse`) that resolves and runs the real hooks
and **exits 2 (block) + loud stderr if any expected hook is unresolvable** —
inverting #111's silent-fail-*open* into fail-*closed*-and-loud. Proposed as a
follow-up enhancement, not a blocker for the §8 fix.

## 6. Security characterization

The two hooks are security controls: `safety_guard.sh` (destructive-action
prevention) and `privacy_guard.sh` (confidentiality — blocks home paths /
usernames / secrets leaking into tracked files, commit messages, PR bodies).

**Severity: High.** They fail **open and silent** across the entire
already-provisioned fleet. *Open*: the gate is absent, so the actions it exists
to block proceed; the host *presents as configured*, so operators commit/push
more freely believing the net is there. *Silent*: Claude Code surfaces no error
for a missing hook command, so "ran and allowed" is indistinguishable from
"never ran." The window between regression and discovery is unbounded. Held
below Critical only because harm is conditional on a second triggering event
(someone attempting a dangerous/leaky action); held firmly above Medium because
the controls are *entirely absent*, fleet-wide, and one class (privacy)
produces irreversible, outbound disclosure.

**Supply chain:** symlinking repo hooks into `~/.claude/hooks/` is
integrity-**neutral** vs today — settings already executes hooks straight from
the writable repo checkout, so the trust relationship is unchanged. The one
additive win is D3's "symlink resolves into the expected checkout" assertion.
Don't let supply-chain framing inflate the PR's scope.

**Retrospective scan (separate work item):** re-arming the gate does nothing
about what leaked while it was off. A follow-up should scan tracked content +
recent git history on affected hosts using `privacy_guard`'s own matchers; any
confirmed *pushed* secret becomes a **rotation** decision (repo stance is no
history rewrite — see `privacy-guard` memory), routed to the owner. *Do not
declare victory once the hooks run again — re-arming and auditing-what-escaped
are two different tasks.*

## 7. Decision: settings model (owner-approved)

**Decision (2026-06-02): D2** — track `ai/claude/settings.json`; host-local
prefs move to `~/.claude/settings.local.json`. The owner approved D2 over the
gitignored host copy + `jq`-merge reconcile alternative (issue #111 option *a*),
because D2 eliminates the reconcile/drift problem at its root and best fits the
two-ways directive. The comparison that informed the call:

| | **D2 — tracked + local-override (recommended)** | **A1 — gitignored + jq-merge reconcile** |
|---|---|---|
| Reconcile | none needed; `git pull` updates wiring | merge logic on every install, must be tested/maintained |
| Drift risk | eliminated | masked, not removed |
| Host-local prefs | `~/.claude/settings.local.json` (untracked) | inline in the gitignored copy |
| `.gitignore` | invert: **track** `ai/claude/settings.json`, ignore `settings.local.json` | unchanged |
| Risk to verify first | no real secrets currently in any host `settings.json` | n/a |

**Rejected outright:** using `$DOTFILES_DIR`/`$BASE_DIR` in the hook command (a
naive reading of #111) — it still references a repo-internal path (violates the
directive) and silently no-ops if the var isn't in Claude Code's exec
environment: the same silent-fail class as #111.

## 8. Implementation plan (phased)

**Phase 0 — land validation first (goes red on un-migrated hosts; that's the signal).**
- D3 in `ai/claude/scripts/sanity_check.sh`: parse live `~/.claude/settings.json`,
  assert every hook/statusLine command resolves to an executable; exercise both
  hooks through the configured command (known-bad → exit 2, known-good → exit 0).
- Extend `ai/hooks/privacy_guard_test.sh` parity with `safety_guard_test.sh`
  (repo already mandates red-then-green: one new `assert_exit 0` legit case + one
  `assert_exit 2` malicious case per hook change).

**Phase 1 — fix the wiring.**
- `ai/claude/settings.json.template` → `settings.json` (per §7 decision): hooks
  reference `$HOME/.claude/hooks/{safety_guard,privacy_guard}.sh`; remove dead
  `DOTFILES_DIR`.
- `install_claude_skills.sh`: add a hooks-symlink block modeled on the statusLine
  block (back up plain file → `.bak`, `ln -sf`, `chmod +x`); delete the "we don't
  symlink hooks" comment; implement the §7 settings model.

**Phase 2 — Gemini parity.**
- Fix F7 (hardcoded cross-call path → resolve from script location / `$GEMINI_PROJECT_DIR`).
- Add D3-style validation to `ai/gemini/scripts/sanity_check.sh`.

**Phase 3 — stale-host migration.**
- On next install, detect a `~/.claude/settings.json` whose hook command does
  **not** resolve and re-point it (back up stale copy → `.bak`). Idempotent;
  guard on "command does not resolve" so a host that *legitimately* customized
  wiring surfaces a loud warning for manual review instead of being clobbered.
- Order: create `~/.claude/hooks/` symlinks **first**, then repoint settings,
  then run the behavioral check — never leave settings pointing at a
  not-yet-existing path mid-migration.

**Phase 4 — optional hardening (follow-up).** D5 fail-closed dispatcher;
retrospective privacy leak scan (§6).

## 9. Documentation update plan (`CLAUDE.md` → `GEMINI.md` + others)

Per the repo's "GEMINI.md + CLAUDE.md in every documented directory" rule (edit
the `GEMINI.md` source; the `CLAUDE.md` symlink follows):

1. **`ai/GEMINI.md`** — add a **"How AI config reaches `$HOME`"** section
   stating the two-mechanisms convention (§4), the table from §1 (statusLine /
   commands / skills / hooks all symlinked), the `settings.json` +
   `settings.local.json` contract, and the documented Claude-symlink vs
   Gemini-`$GEMINI_PROJECT_DIR` asymmetry (D4).
2. **Root `CLAUDE.md`/`GEMINI.md`** — under *Shell & Dotfiles Conventions*, add a
   one-liner: "AI-tool global config (`~/.claude`, `~/.gemini`) follows the same
   no-repo-path rule — see `ai/GEMINI.md`." Cross-link, don't duplicate.
3. **`opt/scripts/system/GEMINI.md`** — document that `install_claude_skills.sh`
   now installs hooks (symlink) and validates configured hook paths; note
   `sanity_check.sh` exercises hooks via the configured command.
4. **`.gitignore`** — if D2 is chosen, add commented rules: track
   `ai/claude/settings.json`, ignore `ai/claude/settings.local.json` (explain
   why inline, per the allowlist convention). Retire `settings.json.template` or
   repurpose it as a documented seed for `settings.local.json` only.

## 10. Acceptance criteria

- No file that lands in (or is referenced by) `$HOME` config references a
  repo-internal path. `grep -rE '\$HOME/git/dotfiles|\$DOTFILES_DIR|git/dotfiles/ai'`
  over `ai/**/settings*.json*` and `ai/**/sanity_check.sh` returns nothing.
- A freshly cloned repo at a **non-default** path provisions working hooks.
- `sanity_check.sh` **fails** on a host with a dead hook path and **passes**
  after re-install; it exercises both hooks via the configured command.
- Both PreToolUse hooks fire on an already-provisioned host after migration.
