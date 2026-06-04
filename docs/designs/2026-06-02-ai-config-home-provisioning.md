# AI config provisioning into `$HOME` — convention + hook-wiring fix

- **Status:** Implemented in PR #113 (revised 2026-06-04 per review; drafted 2026-06-02) — see [§11](#11-implementation-status)
- **Relates to:** issue #111 (*safety_guard hook silently not installed*)
- **Authors:** design pass with the architecture team (systems + security architects)
- **Decision owner:** repo owner. Key calls: **copy, not symlink** (no new symlinks); settings via a **forced-field merge** ([§7](#7-decision-settings-model-revised-in-pr-113-review)); Gemini drops `$GEMINI_PROJECT_DIR` ([D4](#d4--gemini-drop-gemini_project_dir-copy-into-geminihooks-revised)).

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

Everything else Claude installs already references a **well-known `$HOME`
path** — only hooks reach into the repo:

| Artifact | How `settings.json` references it | How it lands in `$HOME` (today) |
|---|---|---|
| statusLine | `~/.claude/statusline-command.sh` ✅ | symlink repo shim → `~/.claude/` *(legacy mechanism)* |
| commands | `~/.claude/commands/*.md` ✅ | symlink into `~/.claude/commands/` *(legacy)* |
| skills | `~/.claude/skills/**` ✅ | symlink via `sync-skills.sh` *(legacy)* |
| **hooks** | `$HOME/git/dotfiles/ai/hooks/*.sh` ❌ | *not installed* — referenced in place |

`install_claude_skills.sh` even comments the exception out loud:
*"We don't symlink hooks — settings.json points at them in-place."* **That
exception is the defect.** The right column is how things land *today*; the
mechanism is shifting from symlink to **copy** (owner directive, §2/§4). The
property that matters — the left column — is unchanged: config must reference a
well-known `$HOME` path, never the repo checkout. Hooks are the one artifact
that fails that, and a repo reorg silently broke them.

## 2. The design directive (owner, authoritative)

> Populate `$HOME` and configure the AI tools we support by **copying** files
> into well-known paths under `$HOME` (e.g. `~/.claude/`, `~/.gemini/`).
> **Symlinks are legacy** — still present and allowed, but **no new ones**;
> they are to be retired over time.
>
> Settings/config files must reference **well-known `$HOME` paths**
> (`~/.claude/hooks/…`) and **must not** reference repo-internal paths like
> `$HOME/git/dotfiles/ai/hooks/…`.

*(Refined during PR #113 review: the original draft listed symlink and copy as
co-equal mechanisms; the owner clarified that copy is the forward path and no
new symlinks should be introduced. The directive is being recorded in the root
`CLAUDE.md`/`GEMINI.md` — see §9.)*

This is the same portability rule already in `CLAUDE.md` ("the repo can be
cloned anywhere and the config must still work") — applied to the AI-tool
global config, which currently breaks it.

## 3. Findings — corrected classification

**Live reproduction (primary host, 2026-06-03):** `~/.claude/settings.json` →
(symlink) `~/git/dotfiles/ai/claude/settings.json` wires **only** `safety_guard`
at the **dead** path `$HOME/git/dotfiles/ai/claude/hooks/safety_guard.sh` (the
pre-reorg `ai/claude/hooks/`, not `ai/hooks/`) and **no `privacy_guard` at
all** — so both PreToolUse security controls are absent on this host *right now*,
yet `make` (`sanity_check.sh`) passes green. That is #111, reproduced.

The findings sort into three buckets. **Only bucket A is a convention
violation** — an earlier draft over-flagged the rest (corrected after owner
review).

### A. Convention violations — config references a raw repo-checkout path
| # | File:line | Reference | Why it violates |
|---|---|---|---|
| F1/F2 | `ai/claude/settings.json(.template)` hooks (live l.70; template l.70/79) | `$HOME/git/dotfiles/ai/(claude/)hooks/*.sh` | A raw checkout path. No `~/` symlink covers `ai/` (only `~/opt → repo/opt` exists; there is no `~/ai`), so the hooks are reached by **neither** sanctioned mechanism. Breaks on any non-`~/git/dotfiles` clone (worktrees, CI). This is the #111 bug. |
| F7 | `ai/gemini/scripts/sanity_check.sh:57` | `$HOME/git/dotfiles/ai/claude/scripts/sanity_check.sh` | Same defect — hardcoded checkout path; breaks if cloned elsewhere. |

### B. Robustness / validation gaps — why the breakage is *silent* (not path violations)
| # | File:line | Issue |
|---|---|---|
| F6 | `ai/claude/scripts/sanity_check.sh:51,64` | `BASE_DIR` is the **repo root** (`dirname/../../..`, i.e. `~/git/dotfiles` — **not** `$HOME`). The check stats `$BASE_DIR/ai/hooks/safety_guard.sh` (the repo copy) and runs its self-test; it **never reads `~/.claude/settings.json`** to confirm the *configured* command resolves. Proven: this host's live settings is broken yet sanity-check is green. `privacy_guard` is not checked at all. → fix = validate (and *exercise*) the live configured path (D3). |
| F8 | `ai/gemini/settings.json:25`, installed **globally** at `~/.gemini/settings.json` (install.sh:334) | `$GEMINI_PROJECT_DIR` resolves to the **active project root at runtime** ([Gemini docs](https://geminicli.com/docs/hooks/): "absolute path to the project root … regardless of the agent's working directory"). A *global* hook anchored to it resolves to `<whatever-project-Gemini-is-in>/ai/hooks/privacy_guard.sh`, so the privacy guard fires **only when Gemini runs inside the dotfiles repo**; in any other project the path doesn't exist and it silently no-ops — same fail-open class. Only `privacy_guard` is wired (no `safety_guard`). → see revised D4. |

### C. Deliberate design to preserve — NOT a defect
| # | File:line | Note |
|---|---|---|
| F5 | seed-once + gitignored `settings.json` (`install_claude_skills.sh` ~l.33-37) | **Working as intended.** It protects real host-local customization: the live file carries `enabledPlugins` (13 plugins), `extraKnownMarketplaces` (2), and `agentPushNotifEnabled` — none of which are in the template. That is the desired "configurable but not committed to the repo" property. The *only* downside is the side effect that template **wiring** changes never reach the host. **Any fix must keep the customization ability** — which is exactly what D2's forced-field merge does (force the wiring, preserve undeclared host fields). |
| F3 | `ai/claude/settings.json.template:3` `env.DOTFILES_DIR` | Dead (referenced nowhere) — minor cleanup, not load-bearing. |

**On the earlier F4 (installer "We don't symlink hooks"):** *not* an independent
violation. `~/opt → ~/git/dotfiles/opt` is confirmed, so the installer itself
runs from a well-known path; the comment is a design choice whose only problem
is its *consequence* (F1/F2). Folded into F1/F2.

**Why the breakage is silent:** F1/F2 put a dead path in the config; F5's
seed-once (correctly protecting customizations) means that dead path is never
re-reconciled; and F6 means our own health check stats the repo copy instead of
the live reference — so a host passes sanity-check while both hooks are absent.
The fix targets F1/F2 (path) and F6 (validation) **without** disabling F5
(customization).

## 4. The convention, formalized

For every AI tool we support, a config/settings file references an artifact
**only** by a well-known `$HOME` path. The artifact arrives in `$HOME` by a
**copy** into that well-known path, refreshed by the install / `sync-*` flow:

1. **Copy (forward mechanism)** — `install_*_skills.sh` copies `<repo>/…` →
   `~/.<tool>/…`, re-copied on every install/`sync-*` so it tracks the repo.
   Use for the executables and shims that must stay current: hooks, statusline,
   commands, skills.
2. **Symlink (legacy — do not add new ones)** — existing symlinks (statusline,
   commands, skills, `~/opt`, profiles) may remain but are slated for migration
   to copy. **No new symlinks.**

Host-local settings *values* (not files) are handled by the **forced-field
merge** in D2: declared-immutable fields are applied from the repo on install
while undeclared host values are preserved.

Config files reference `~/.<tool>/…`. They never reference
`$HOME/git/dotfiles/…`, `$DOTFILES_DIR/…`, or any repo-relative path.

**Tradeoff acknowledged:** a copy is a point-in-time snapshot, so a repo change
to a hook does not reach the host until the next install/`sync-*` run (unlike a
symlink's live tracking). That is intended — it decouples `$HOME` from the
checkout location — and D3's validation plus the `sync-*` refresh close the
staleness gap.

## 5. Recommended design (architecture team)

### D1 — Hooks are **copied** into `~/.claude/hooks/` (no symlink)
`install_claude_skills.sh` copies each `ai/hooks/*.sh` → `~/.claude/hooks/<name>.sh`
(`chmod +x`, backing up any pre-existing *differing* file), re-copying on every
install/`sync-*` run so the host stays current. The config references the
well-known path:

```json
"command": "$HOME/.claude/hooks/safety_guard.sh"
"command": "$HOME/.claude/hooks/privacy_guard.sh"
```

Path-independent; works in worktrees, CI, and alternate clone locations. **Copy,
not symlink**, per the owner's no-new-symlinks directive (§2). Drop the dead
`env.DOTFILES_DIR` (F3).

### D2 — Settings: a **forced-field merge**, host owns the file (revised per review)
*(Supersedes the earlier "track `settings.json` + symlink" plan — the owner
noted a tracked symlinked file risks mismatches for non-forced host fields and
contradicts the no-new-symlinks directive.)*

`~/.claude/settings.json` stays a **real, host-owned file** (never a symlink).
The repo declares a **forced subset** — the immutable fields we must keep
current on every host: `hooks` (PreToolUse wiring), `statusLine`, and the
security `permissions.deny` / `permissions.ask` lists. On every install/`sync-*`
run the installer **deep-merges that forced subset over the host's
`settings.json`** (e.g. via `jq`), overwriting *only* the declared keys and
leaving every undeclared host field (`enabledPlugins`, `extraKnownMarketplaces`,
`theme`, `apiKeyHelper`, …) untouched.

This is the reconcile mechanism that fixes #111's drift **without** clobbering
customizations (F5): a wiring change in the repo reaches every host on the next
install because forced fields are re-applied, while non-forced host values
persist. `settings.local.json` remains available for purely host-local
overrides, but the forced-field merge is what guarantees the security wiring.

Concretely: keep `settings.json.template` as the first-run **seed** (full
defaults) and add `ai/claude/settings.forced.json` (the immutable subset) that
is **always** merged in. No symlink; no fully-tracked host file.

### D3 — Validation reads the **live, configured** path and *exercises* the hook
- Install-time (`install_claude_skills.sh`) and `sanity_check.sh`: parse
  `~/.claude/settings.json` with `jq`, extract every `hooks[].hooks[].command`
  and `statusLine.command`, expand `$HOME`/`~`, and **fail loud** if any
  resolved path is not an executable regular file. Since hooks are now copies
  (D1), also diff the installed `~/.claude/hooks/*.sh` against the repo source
  and warn on drift (a stale copy from a missed install/`sync-*`).
- **Behavioral check (security team, strong rec):** sanity_check should drive
  the hook **through the configured command string**, not the repo path — feed
  `safety_guard.sh` a known-bad payload and assert exit 2 (block) and a
  known-good payload and assert exit 0 (allow); same for `privacy_guard.sh`.
  A test that hardcodes `ai/hooks/safety_guard.sh` (today's F6) passes green
  while the *reference* from settings.json is broken — the test must traverse
  the reference, because the reference is what breaks.

### D4 — Gemini: drop `$GEMINI_PROJECT_DIR`, copy into `~/.gemini/hooks/` (revised)
*(Supersedes an earlier draft that called `$GEMINI_PROJECT_DIR` a benign
"documented asymmetry" — F8, then PR review, show it is wrong for a global
settings file.)*

`ai/gemini/settings.json` is installed **globally** at `~/.gemini/settings.json`
(install.sh:334), but `$GEMINI_PROJECT_DIR` resolves to the **active project
root at runtime** ([Gemini docs](https://geminicli.com/docs/hooks/)). So the
hook only fires when Gemini runs *inside the dotfiles repo*; everywhere else it
silently no-ops (F8).

**Decision (2026-06-04): D4a + drop `$GEMINI_PROJECT_DIR`.** The Gemini privacy
guard should protect *all* Gemini sessions (matching Claude's global guard).
`$GEMINI_PROJECT_DIR` is the wrong variable for a **global** settings file — it
anchors to whatever project is active — so drop it. Instead **copy**
`ai/hooks/*.sh` → `~/.gemini/hooks/` and reference the fixed well-known path
`$HOME/.gemini/hooks/privacy_guard.sh` in `~/.gemini/settings.json`. Mirrors the
Claude D1 copy; makes the guard project-independent.

> Alternative rejected (D4b): moving the hook to a project-level
> `<repo>/.gemini/settings.json` (where `$GEMINI_PROJECT_DIR` *would* be correct)
> would scope the guard to dotfiles-repo edits only — not the intent.

Also wire `safety_guard` for Gemini (currently only `privacy_guard` is present),
**fix F7** (the hardcoded cross-call path → resolve from the script's own
location), apply the same forced-field merge to `~/.gemini/settings.json`,
extend the Gemini `sanity_check` (D3-style, exercising the configured command),
and record the model in `ai/GEMINI.md`.

### D5 — Optional hardening: a fail-closed dispatcher (security team)
The strongest defense against *future* drift: point settings at one stable
dispatcher (`~/.claude/hooks/pretooluse`, itself a **copy**) that resolves and
runs the real hooks and **exits 2 (block) + loud stderr if any expected hook is
unresolvable** — inverting #111's silent-fail-*open* into fail-*closed*-and-loud.
Proposed as a follow-up enhancement, not a blocker for the §8 fix.

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

**Gemini's global guard fails open too (F8):** because `~/.gemini/settings.json`
anchors the hook to `$GEMINI_PROJECT_DIR`, the privacy guard only runs when
Gemini operates inside the dotfiles repo — every other project runs with no
guard, silently. Same severity logic; D4 resolves it.

**Supply chain:** copying repo hooks into `~/.claude/hooks/` is
integrity-**neutral-to-positive** vs today — settings currently executes hooks
straight from the writable repo checkout; an installed copy is an independent
snapshot in `$HOME`, owned by the user. D3's "executable regular file +
drift-vs-source" check closes the loop. Don't let supply-chain framing inflate
the PR's scope.

**Retrospective scan (separate work item):** re-arming the gate does nothing
about what leaked while it was off. A follow-up should scan tracked content +
recent git history on affected hosts using `privacy_guard`'s own matchers; any
confirmed *pushed* secret becomes a **rotation** decision (repo stance is no
history rewrite — see `privacy-guard` memory), routed to the owner. *Do not
declare victory once the hooks run again — re-arming and auditing-what-escaped
are two different tasks.*

## 7. Decision: settings model (revised in PR #113 review)

**Decision: forced-field merge; the host owns `settings.json` (no symlink, no
fully-tracked host file).** The repo declares an immutable subset
(`ai/claude/settings.forced.json`: `hooks`, `statusLine`, security
`deny`/`ask`) that the installer deep-merges over the host's real
`settings.json` on every run; `settings.json.template` seeds first-run defaults;
undeclared host fields are preserved.

How this evolved across review:
- *Original draft (D2):* track `settings.json` and symlink it. **Rejected** by
  the owner — a tracked symlinked file risks mismatches for non-forced host
  fields, and adds a symlink against the no-new-symlinks directive (§2).
- *Issue #111 option a:* gitignored host copy + jq-merge. Closest to the final
  shape; the refinement is making the *forced subset explicit* (a declared
  `settings.forced.json`) rather than merging the whole template.

| | **Forced-field merge (chosen)** | tracked + symlink (rejected) |
|---|---|---|
| Mechanism | deep-merge declared subset over host file | symlink host file → repo |
| Non-forced host fields | preserved | risk of mismatch / clobber |
| New symlinks | none ✅ | adds one ❌ |
| Drift on wiring change | reconciled on next install/`sync-*` | n/a |

Verified safe on the primary host: the live `settings.json` carries
`enabledPlugins`/`extraKnownMarketplaces`/`agentPushNotifEnabled` (all
non-forced → preserved by the merge) and **no secrets** (no `apiKeyHelper`/tokens).

**Also rejected:** `$DOTFILES_DIR`/`$BASE_DIR` in the hook command — still a
repo-internal path (violates the directive) and silently no-ops if the var isn't
in Claude Code's exec environment: the same silent-fail class as #111.

## 8. Implementation plan (phased)

**Phase 0 — land validation first (goes red on un-migrated hosts; that's the signal).**
- D3 in `ai/claude/scripts/sanity_check.sh`: parse live `~/.claude/settings.json`,
  assert every hook/statusLine command resolves to an executable; exercise both
  hooks through the configured command (known-bad → exit 2, known-good → exit 0).
- Extend `ai/hooks/privacy_guard_test.sh` parity with `safety_guard_test.sh`
  (repo already mandates red-then-green: one new `assert_exit 0` legit case + one
  `assert_exit 2` malicious case per hook change).

**Phase 1 — fix the wiring.**
- `ai/claude/settings.json.template` (seed) + new `ai/claude/settings.forced.json`
  (immutable subset = `hooks`, `statusLine`, security `deny`/`ask`): hooks
  reference `$HOME/.claude/hooks/{safety_guard,privacy_guard}.sh`; remove dead
  `DOTFILES_DIR`.
- `install_claude_skills.sh`: **copy** `ai/hooks/*.sh` → `~/.claude/hooks/`
  (`chmod +x`); delete the "we don't symlink hooks" comment; add the
  forced-field `jq` merge over the host `settings.json` (seed from template on
  first run). No symlink.

**Phase 2 — Gemini parity.**
- Per D4a: **copy** `ai/hooks/*.sh` → `~/.gemini/hooks/`; reference
  `$HOME/.gemini/hooks/...` in `~/.gemini/settings.json`; **drop
  `$GEMINI_PROJECT_DIR`**.
- Wire `safety_guard` for Gemini too (today only `privacy_guard` is present).
- Fix F7 (hardcoded cross-call path → resolve from the script's own location).
- Apply the same forced-field merge to `~/.gemini/settings.json`; add D3-style
  validation (exercise the configured command) to `ai/gemini/scripts/sanity_check.sh`.

**Phase 3 — stale-host migration.**
- On next install the forced-field merge re-applies the correct `hooks` block,
  overwriting the dead path automatically. Order: (re)write the hook **copies**
  to `~/.claude/hooks/` **first**, then merge settings — never leave settings
  pointing at a not-yet-written path mid-migration.
- The merge only touches forced keys, so a host's customized *non-forced* field
  survives; if a host intentionally altered a *forced* field, surface a loud
  warning (don't silently clobber) for manual review.
- Then run the behavioral check (D3) to confirm both hooks fire.

**Phase 4 — optional hardening (follow-up).** D5 fail-closed dispatcher;
retrospective privacy leak scan (§6).

## 9. Documentation update plan (`CLAUDE.md` → `GEMINI.md` + others)

Per the repo's "GEMINI.md + CLAUDE.md in every documented directory" rule (edit
the `GEMINI.md` source; the `CLAUDE.md` symlink follows):

1. **Root `CLAUDE.md`/`GEMINI.md` (done in this PR)** — record the directive:
   *AI-tool config is provisioned into well-known `$HOME` paths by **copy**;
   **no new symlinks** (existing ones are legacy, to be retired); config never
   references repo-internal paths.* (Owner request, PR #113 review.)
2. **`ai/GEMINI.md`** — add a **"How AI config reaches `$HOME`"** section: the
   copy-forward / symlink-legacy convention (§4), the forced-field settings
   merge (D2), and the Gemini hook model (copy into `~/.gemini/hooks/`, no
   `$GEMINI_PROJECT_DIR`).
3. **`opt/scripts/system/GEMINI.md`** — document that `install_claude_skills.sh`
   now **copies** hooks into `~/.claude/hooks/`, merges the forced settings
   subset, and validates + exercises the configured hook command.
4. **`.gitignore`** — keep `ai/claude/settings.json` ignored (host-owned); track
   the new `ai/claude/settings.forced.json`. Document both with inline comments
   per the allowlist convention.

## 10. Acceptance criteria

- No file that lands in (or is referenced by) `$HOME` config references a
  repo-internal path. `grep -rE '\$HOME/git/dotfiles|\$DOTFILES_DIR|git/dotfiles/ai'`
  over `ai/**/settings*.json*` and `ai/**/sanity_check.sh` returns nothing.
- The installers create **no new symlinks**; hooks are present as executable
  **copies** under `~/.claude/hooks/` and `~/.gemini/hooks/`.
- A freshly cloned repo at a **non-default** path provisions working hooks.
- `sanity_check.sh` **fails** on a host with a dead hook path and **passes**
  after re-install; it exercises both hooks via the configured command.
- The forced-field merge updates a host's `hooks` wiring while preserving an
  undeclared custom field (e.g. `enabledPlugins`).
- Both PreToolUse hooks fire on an already-provisioned host after migration.

## 11. Implementation status

Landed in PR #113 (TDD; `make shell-test` green — 17/17 drivers):

| Area | Change |
|---|---|
| Forced merge | `opt/scripts/system/apply-forced-settings.sh` (+ `_test.sh`, 15 cases) — deep-merge declared subset, strip `_`-doc keys, fail loud without clobbering |
| Claude config | `ai/claude/settings.forced.json` (new immutable subset); `settings.json.template` → `$HOME/.claude/hooks/...`, dead `DOTFILES_DIR` removed |
| Gemini config | `ai/gemini/settings.forced.json` (new); `settings.json` drops `$GEMINI_PROJECT_DIR` → `$HOME/.gemini/hooks/...`, wires `safety_guard` too |
| Claude installer | `install_claude_skills.sh` — **copies** hooks (+`strip_heredocs.awk`) into `~/.claude/hooks/`; seeds host file; forced-merge; legacy-symlink migration; one-time `.bak` (+ `_test.sh`, 13 cases) |
| Gemini installer | `install_gemini_skills.sh` — copies hooks into `~/.gemini/hooks/`; seed + forced-merge; migration + `.bak` (+ `_test.sh`, 9 cases) |
| install.sh | removed the duplicate settings-symlink blocks (skill installers now own provisioning) |
| Validation (D3) | `ai/claude/scripts/validate_hooks.sh` (+ `_test.sh`, 4 cases) — event-agnostic; validates the **live configured** command resolves and **exercises** safety_guard (known-bad→block); wired into both sanity checks; `privacy_guard_test.sh` now also run; Gemini F7 hardcoded path fixed |
| Directive | recorded in root `CLAUDE.md`/`GEMINI.md` (no new symlinks; copy into well-known `$HOME`) |

Deferred to follow-ups (out of scope here): D5 fail-closed dispatcher; the
retrospective privacy leak scan (§6); migrating the *existing* statusline /
commands / skills symlinks to copies (legacy, allowed to remain).
