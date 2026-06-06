# prping — Implementation Plan

> ⚠️ **SUPERSEDED (2026-06-06) — pending regeneration.** This plan targets the original
> **bash-script** design (`src/prping/{pr-status,notify-diff,prping}.sh` + shell `*_test.sh` +
> `make shell-test` wiring). The spec was revised to a **single Go CLI** under `sdk/prping`
> (cobra + `internal/` packages + mockable `gh` runner + Go table tests + coverage gate) — see
> the rewritten `docs/mbo/specs/2026-06-05-prping-design.md` §3/§7/§9 and §12.7. The
> Go form also dissolves this plan's "Blocker 1" (shell-test not scanning `src/`): Go tests run
> via the existing `scripts/test.sh` discovery. **A new Go-CLI implementation plan will replace
> this file.** The §8→§9 evaluation/traceability content below remains valid (the event rules
> are unchanged; they now live in `internal/diff`).

A Claude-native PR/push notifier. This plan operationalizes the approved design
`docs/mbo/specs/2026-06-05-prping-design.md` into a concrete, TDD-ordered,
buildable sequence with exact artifact paths. Every must-fix from the adversarial plan
review is resolved inline (see the **Resolution** notes), or explicitly deferred with a
reason.

All paths are absolute-from-repo-root ``$HOME/git/dotfiles/``.

---

## 1. Status

- **Date:** 2026-06-05
- **Relates to:** the approved spec `docs/mbo/specs/2026-06-05-prping-design.md`
  and **PR #127** (the open PR tracking prping). This plan is the implementation contract
  that PR #127's review gates against.
- **Verdict:** **plan-with-fixes — approved to build.** The spec is sound and buildable.
  All six review-raised gaps, five missing artifacts, and five ordering issues are
  resolved below against verified ground truth (Makefile lines 118 / 146–152,
  `ai/_test_helpers.sh`, `opt/scripts/system/sync-skills.sh` lines 30–42 / 97–130,
  `ai/hooks/privacy_guard.sh` lines 131–151, `.gitignore` lines 40–41 / 97–99).
- **Scope of the build:** one new non-Go skill dir `src/prping/` (3 scripts + SKILL.md +
  3 test drivers + testdata), plus two `Makefile` discovery edits and one `src/GEMINI.md`
  doc link. Pure addition; rollback = delete `src/prping/` + `~/.config/prping/` and revert
  the Makefile / GEMINI.md edits.

---

## 2. Evaluation summary (verdict + how must-fix items are addressed)

The design's load-bearing idea is correct and worth building: because `PushNotification` is
agent-loop-only (no shell→phone path), the only Claude-native way to ping a phone on a
git/gh state change is a **persistent agent watcher** that observes effects and relays.
Pushing **all** notification decisions into a pure `notify-diff.sh` and making the agent a
thin print→`PushNotification` relay collapses the non-deterministic surface to one line,
which is what makes the §8/§9 confidence story credible. That core is approved as-is.

The adversarial review found **no design defect** — every issue was a plan-precision or
CI-wiring gap. Resolutions, each verified against source:

| # | Review item | Resolution (verified) |
| :-- | :-- | :-- |
| G1 | **`make shell-test` has 3 `find` invocations; adding `sdk` silently activates 3 un-run sdk drivers.** Verified Makefile 146–152: `find ai opt/scripts opt/bin opt/profiles -maxdepth 6` + `find . -maxdepth 1` + `find scripts -maxdepth 1`. | **Drop `sdk` from this plan entirely (out of scope).** Add **only `src`** to the first `find` (its `-maxdepth 6` reaches `src/prping/*_test.sh` at depth 3). The 3 sdk drivers (`sdk/gsl/scripts/check-deps_test.sh`, `check-font-glyphs_test.sh`, `sdk/gss/scripts/check-deps_test.sh`) use their **own inline `assert_exit`** helper and require a Go toolchain (and `check-font-glyphs` a Nerd Font) — verified they pass on this host today, but wiring them into the shell harness is a separate concern that prping must not own. Listed as a touch-point risk in §5, not actioned. |
| G2 | **`lint-shell` scope gap; prping scripts must be shellcheck-clean.** Verified Makefile 118: `find opt/scripts ai -name '*.sh'`. | Add **`src`** to that glob. New explicit done-when gate (Phase 7): **all `src/prping/*.sh` pass `shellcheck -S warning`**. Pre-flight confirms the only pre-existing `src/*.sh` (`src/ssh-key-sync/ssh-key-sync.sh`) stays clean so adding `src` doesn't red the build on an unrelated file. |
| G3 | **Test-helper interface unverified.** | **Verified `ai/_test_helpers.sh` directly.** The public API is `assert_eq <got> <want> <label>`, `assert_grep <label> <pat> <file>`, `assert_grep_negative`, `assert_in_subshell <label> <code…>`, `assert_file_exists <path> <label>`, `assert_exit_code <expected> <label> <cmd…>`, and `_test_report` (prints `PASS=N FAIL=M`, exits 1 on any FAIL). **There is no `assert_exit` and no `assert_contains` in this helper** — the draft's `assert_exit` was wrong (that name lives in the unrelated sdk drivers). prping drivers MUST call `assert_exit_code` for exit-status checks and end with `_test_report` (its non-zero exit is what `make shell-test` keys on via `bash "$f"`). Reading the helper is a **Phase −0 pre-flight**, before any driver is written. |
| G4 | **Privacy/fixture Rule attribution.** Verified `privacy_guard.sh` 141–148: `word_present` needs `len ≥ 3` and matches case-insensitively on a word boundary. | Per-token attribution corrected (see §6): the committer login `<login>` trips **Rule C** (word-bounded). The §3.2 sample literal `sfc-gh-eraigosa/dotfiles` does **not** contain a bare `<login>` token, so it does **not** trip Rule C — replacing it with synthetic `acme/widgets` is **consistency/hygiene** (and defense-in-depth via the fixture-lint), not a privacy_guard block. `/home/<login>` paths trip **Rule B**. The fixture-lint greps the login as a substring (stricter than Rule C) deliberately. |
| G5 | **State-dir filename sanitization untested.** Spec §3 keys on `~/.config/prping/<owner>-<repo>.json`; `repo` can contain `/`. | Add a **filename-derivation unit case** (Phase 1, in `pr-status_test.sh` / orchestrator): `owner/repo` → filename via `/`→`-` (and any non-`[A-Za-z0-9._-]`→`-`). Document the chosen rule as a tested contract so `a/b-c` and `a-b/c` cannot collide (e.g. encode the slash distinctly, or accept the documented limitation with a test asserting the exact derived name). |
| G6 | **§9.5 / §9.4 are human-evidenced, not CI-checkable, but Phase 7 bundled them with `make shell-test`.** | Phase 7 now **separates** the two gate classes explicitly (§9 / §10): **automated gates** = `make lint-shell` + `make shell-test` green with the 3 `src/prping/*_test.sh` in the RUN list; **human-evidenced gates** = trigger-eval ≥90% number recorded + one manual phone-acceptance sign-off. A green CI run is never mistaken for full DoD. |

Missing artifacts (all resolved): **(a)** no `build.sh` — prping is non-Go, has no binary;
made explicit in §5 (sync-skills `build_component` at lines 89–93 only fires on `build.sh`,
which is absent by design). **(b)** sync-skills has **no `--dry-run`** (verified args loop
lines 30–42 = `--build`/`--help` only) — the verification command is the real run +
symlink inspection (§5, Phase 5). **(c)** the 3 pre-existing un-run sdk drivers are listed
as a risk/touch-point (§5). **(d/e)** the load-bearing Makefile comment blocks (lint-shell
103–117, shell-test 128–141) are **required edits** to mention `src/` (Phase 0).

Ordering issues (all resolved): helper-read precedes first driver (Phase −0); the sdk-driver
risk is removed by dropping sdk (Phase 0); Phase 1 **freezes the snapshot schema** as an
explicit input gate to Phase 2 (so §8.6 goldens are authored once); `scenario_test.sh` is
**split with a labeled handoff** — lock/resume assertions land in Phase 3, lifecycle
transcript goldens in Phase 4; SKILL.md (Phase 5) is gated on Phase 3's delivery-semantics
decision being locked.

---

## 3. File inventory

### Inside `src/prping/` (the skill)

| Path | Purpose | Contents (what it must contain) | Implements |
| :-- | :-- | :-- | :-- |
| `src/prping/SKILL.md` | Skill prompt + thin relay wrapper + manual-acceptance checklist. **Not** the orchestrator. | YAML front-matter (`name: prping`, trigger `description` tuned for §9.5). Prereq checks (§6: Claude Code ≥ 2.1.110, Remote Control attached, `gh` auth, `jq`). Orchestration pseudocode that calls `prping.sh` and relays each printed line via `PushNotification(status:"proactive")`. Pacing via the **`loop`** skill (self-paced) + optional `Monitor(persistent:true)` re-armed each iteration. The §9.4 manual phone-acceptance checklist. Fixture-hygiene note. References **only real primitives** — no `ScheduleWakeup` (does not exist). | §3, §3.1, §5, §6, §9.4 |
| `src/prping/prping.sh` | **Executable orchestrator** (the missing-script the spec implies in §3.1 / §9.3). snapshot → diff → persist, with atomicity, locking, and `--print`. | `set -euo pipefail`. Derives state filename from `<owner/repo>` (tested, §G5). Acquires per-repo lockfile; reads prev state (seed-silent if absent); calls `pr-status.sh` then `notify-diff.sh`; **persists new snapshot THEN prints lines** (at-most-once); `--print` prints + persists with **no** `PushNotification`. Atomic write: temp file → `mv`/rename; `umask 077` → 0700 dir / 0600 file. | §3.1, §3.3, §5, §8.7 |
| `src/prping/pr-status.sh` | Pure I/O → JSON snapshot. **No decisions.** | `set -euo pipefail`. `gh pr list --json number,title,headRefName,headRefOid,isDraft,mergeStateStatus,mergeable,statusCheckRollup` + `git ls-remote --heads origin`. `jq -r` with `// empty` / `// []` defaults; sorted `failingChecks`; PRs sorted by number; **schema-closed** output (only §3.2 fields; never serializes gh auth/error bodies to stdout). | §3.2, §8.6 |
| `src/prping/notify-diff.sh` | **Pure decision function** — the heart. `(prev, now)` → event lines. | `set -euo pipefail`, `jq` only. **Total**: absent/empty/malformed/`UNKNOWN` prev ⇒ seed-silent, 0 lines. Per-tick precedence resolver (§8.8: check-failed > needs-update). First-sight consolidation (§8 global rule). Control-char/newline/ANSI strip + `<200`-char truncation of titles/branches. Deterministic PR-number ordering. | §4, §8.1–8.8 |
| `src/prping/pr-status_test.sh` | Layer 1 unit tests. | Sources `ai/_test_helpers.sh`. Shims `gh`/`git` on `PATH` via a temp dir. Asserts exact snapshot JSON across: 0-PR, one-per-status, multi-PR, draft, conflicting, missing/empty fields. **Schema-closed** assertion (no extra keys). **Filename-derivation** case (§G5). Ends with `_test_report`. | §9.1 |
| `src/prping/notify-diff_test.sh` | Layer 2 golden decision tests. | One+ named case per §8 rule (Fires + Must-NOT-fire), the §8.7 invariants, §8.8 precedence, totality goldens (UNKNOWN flap, empty-refill), sanitization goldens. Coverage assertion: every §8 rule id has ≥1 case. Uses `assert_eq` against golden lines. Ends with `_test_report`. | §8, §9.2 |
| `src/prping/scenario_test.sh` | Layer 3 lifecycle E2E. | Drives `prping.sh --print` over `testdata/scenario/tick-NN.json`. **Phase-3 half:** restart-resume (no replay) + single-writer lock (second instance no-ops). **Phase-4 half:** concatenated transcript == golden. Fixture-lint: fails if any `testdata/**` file contains the committer login or `$HOME` basename. Ends with `_test_report`. | §9.3, §8.7 |
| `src/prping/GEMINI.md` | Per-dir docs (repo convention). | What prping is; the 3 scripts + the orchestrator boundary; the `~/.config/prping/` state dir; how the harness runs under `make shell-test`; the fixture-hygiene rule; link back to the spec. | §7 |
| `src/prping/CLAUDE.md` | Symlink → `GEMINI.md` (relative). | `ln -s GEMINI.md CLAUDE.md`. | §7 |
| `src/prping/testdata/snapshots/*.json` | Snapshot fixtures for Layers 1–2. | Synthetic, identity-free (`acme/widgets`, `feature/x`, fake titles/SHAs). prev/now pairs per §8 rule. | §9.1, §9.2 |
| `src/prping/testdata/scenario/tick-00..NN.json` | Ordered lifecycle ticks for Layer 3. | open(draft)→ready→push→pending→CLEAN→behind→update-push→CLEAN→merged. Synthetic. | §9.3 |
| `src/prping/testdata/golden/*.txt` | Golden transcripts for Layers 2–3. | Exact expected event lines. Synthetic identifiers only. | §9.2, §9.3 |
| `src/prping/testdata/trigger-eval.json` | Skill-trigger eval corpus (manual-but-evidenced gate). | Positive phrasings + near-miss confusers (`review this PR`, `merge this`, routing to `gss-pr`/`code-review`) → expected routing. | §9.5 |

> **No `src/prping/smoke_test.sh`.** The draft's placeholder is dropped: the first **real**
> driver (`pr-status_test.sh`, Phase 1) proves discovery just as well, so there is no
> throwaway file to land-then-delete. Phase 0's discovery proof uses a temporary local
> probe that is never committed (see Phase 0 done-when).

### Touch-points OUTSIDE `src/prping/`

| Path | Change | Why |
| :-- | :-- | :-- |
| `Makefile` `shell-test` target (line 148, first `find`) | Add `src` to the find roots: `find ai opt/scripts opt/bin opt/profiles src -maxdepth 6 …`. **Do not add `sdk`** (§G1). | Without it the §9 harness never runs in CI. `-maxdepth 6` already reaches `src/prping/*_test.sh` (depth 3). |
| `Makefile` `shell-test` comment block (lines 128–141) | Update "Discovery scans ai/, opt/scripts/, opt/bin/, opt/profiles/, and the repo root" → add `src/`. | Load-bearing doc comment; a reviewer reading it must not be misled (§G-missing-d). |
| `Makefile` `lint-shell` target (line 118) | Add `src`: `find opt/scripts ai src -name '*.sh' …`. | `src` shell scripts are currently never shellchecked (§G2). |
| `Makefile` `lint-shell` comment block / echo (lines 103–117, 103) | Update the scope enumeration ("opt/scripts, ai, opt/profiles, install.sh") to include `src`. | Same doc-truth reason (§G-missing-e). |
| `src/GEMINI.md` (after line 10, under `## Projects`) | Add a `prping/` bullet linking `./prping/SKILL.md` + `./prping/GEMINI.md`. | Per-dir docs convention; discoverability. |
| `~/.config/prping/` | Runtime state dir, created 0700 by `prping.sh` at first run. **Never committed.** | §3.3, §11. |

**`.gitignore`: no change needed.** `!src/` + `!src/**` (lines 40–41) already opt in the
entire `src/prping/` tree including `testdata/`. The `~/.config/prping/` state dir lives in
`$HOME`, **outside the repo tree** — the repo's `.gitignore` `.config/` allowlist (lines
97–99, only `.config/terminator/`) is about the *repo's* `.config/`, not `$HOME`, so no rule
is required or possible. **Confirmed.**

**`sync-skills.sh`: no code change for v1.** A flat `src/prping/SKILL.md` is auto-linked as
`prping` by the Priority-2 branch (lines 127–128). No case-map arm is added (a friendly
slash name would require the Priority-1 `skill/` subdir layout — deferred). **No `build.sh`**
— `build_component` (lines 89–93) only fires when `build.sh` exists; prping is non-Go and
intentionally has none.

---

## 4. Interface contracts

### `pr-status.sh`
```
Usage:  pr-status.sh <owner/repo>
Stdin:  none
Stdout: exactly one JSON object (the snapshot; schema below). Schema-closed.
Stderr: human-readable diagnostics only — never gh auth output or error bodies.
Exit:   0 on success; non-zero on gh/git/jq failure (set -euo pipefail; pipefail
        scopes every gh|jq pipeline).
```

### `notify-diff.sh`
```
Usage:  notify-diff.sh <prev.json> <now.json>
Stdin:  none
Stdout: 0+ event lines, one per line, each <200 chars, no markdown, sanitized
        (no embedded newline/ANSI/control char survives), ordered by PR number.
        TOTAL: never crashes on UNKNOWN / missing / malformed / empty input.
Exit:   0 whenever inputs are readable. Absent/empty/malformed prev ⇒ seed-silent
        (0 lines, exit 0). notify-diff(now, now) ⇒ 0 lines.
```

### `prping.sh` (orchestrator)
```
Usage:  prping.sh [--print] <owner/repo>
Behavior (normal):  read prev state → pr-status.sh → notify-diff.sh →
                    PERSIST new snapshot (atomic) → print event lines (caller relays).
Behavior (--print): identical pipeline, prints lines, persists, emits NO PushNotification.
State file:         ~/.config/prping/<derived-name>.json   (0600, in a 0700 dir, umask 077)
Lock file:          ~/.config/prping/<derived-name>.lock   (single writer; 2nd instance no-ops)
Filename rule:      <owner/repo> → derived-name by mapping '/' and any
                    non-[A-Za-z0-9._-] char to '-' (tested; §G5).
Ordering:           persist BEFORE print/relay → at-most-once (a crash drops a
                    push, never duplicates). [delivery-semantics decision, Phase 3]
Exit:               0 on success; non-zero on lock-held or pipeline failure.
```

### Snapshot JSON (§3.2, schema-closed)
```json
{
  "repo": "acme/widgets",
  "branchHeads": { "feature/x": "<sha>" },
  "prs": [
    { "number": 126, "title": "<sanitized>", "branch": "feature/x", "headSha": "<sha>",
      "isDraft": false, "mergeStateStatus": "CLEAN|BEHIND|BLOCKED|DIRTY|UNKNOWN",
      "mergeable": "MERGEABLE|CONFLICTING|UNKNOWN", "failingChecks": ["..."] }
  ]
}
```
*State file = last-seen `now` snapshot, identical shape. If the §8.6 merged-probe option is
chosen in Phase 1, add `"merged": true|false` to disappeared-PR handling; otherwise omit.*

### SKILL.md orchestration pseudocode (relay only)
```
prereqs: Claude Code ≥ 2.1.110; Remote Control attached (warn + point at
         remote-claude-session if not); gh auth; jq present.
arm pacing: loop (self-paced) as the driver; optional Monitor(persistent:true) on the
            in-flight CI run for an early wake, RE-ARMED each iteration (survives Monitor's
            1h cap). CronCreate is NOT used (7-day expiry, idle-only). ScheduleWakeup
            does not exist — never reference it.
each iteration:
  lines = $(bash src/prping/prping.sh <owner/repo>)   # snapshot→diff→PERSIST→print
  for line in lines: PushNotification(message=line, status="proactive")
  # tests / agentless: prping.sh --print → prints lines, persists, NO PushNotification
terminate: per watcher-lifecycle decision (Phase 5).
```

---

## 5. TDD build order (phased, tests-first, done-when gates)

Each phase is tests-first wherever a test can exist. Done-when gates are concrete commands.

### Phase −0 — Pre-flight (read before writing any driver)
1. **Read `ai/_test_helpers.sh`** and pin the API (done — see §2 G3): drivers use
   `assert_eq` / `assert_exit_code` / `assert_grep[_negative]` / `assert_file_exists` and
   end with `_test_report`. **There is no `assert_exit`** here.
2. Confirm the only pre-existing `src/*.sh` is `src/ssh-key-sync/ssh-key-sync.sh` and it
   **shellchecks clean** under `-S warning` (so adding `src` to `lint-shell` won't red the
   build on an unrelated file). If it warns, fix/baseline in Phase 0.
- **Done-when:** helper API written into the test drivers' header comments; ssh-key-sync
  shellcheck result recorded.

### Phase 0 — Make CI able to run the harness (Blocker 1, FIRST)
1. Patch `Makefile` `shell-test` first `find` to add **`src`** (not `sdk`). Update the
   shell-test comment block (128–141) to mention `src/`.
2. Patch `Makefile` `lint-shell` `find` to add **`src`**, and update its comment/echo
   scope (103–117).
- **Done-when:** drop a throwaway local probe `src/prping/_probe_test.sh` (one
  `assert_eq` + `_test_report`), run `make shell-test`, confirm its `RUN:` list contains
  `src/prping/_probe_test.sh` and it passes; run `make lint-shell` and confirm it passes
  with `src` included; **delete the probe** (never commit it). The first committed driver
  in Phase 1 then becomes the standing discovery proof.

### Phase 1 — pr-status.sh (Layer 1) + schema freeze
1. Write `pr-status_test.sh` first: shim `gh`/`git`; fixtures for 0-PR / one-per-status /
   multi / draft / conflicting / missing-empty; **schema-closed** assertion; the
   **filename-derivation** case (§G5).
2. Implement `pr-status.sh` to pass (`jq -r` with `// empty`/`// []`; sorted
   `failingChecks` and PRs-by-number).
3. **Resolve the merged-vs-closed open decision (§8.6) and FREEZE the schema here** — this
   is an explicit **input gate to Phase 2** so the §8.6 goldens are authored once.
   *Recommended v1:* a single neutral `closed` handling (disappearance ⇒ drop from state,
   no merged/closed distinction) — keeps `notify-diff` a pure two-list diff. If the merged
   ping is valued, add a `gh pr view` merged-probe and a `merged` field to the schema now.
- **Done-when:** `bash src/prping/pr-status_test.sh` green; snapshot byte-stable across two
  runs on the same fixtures; schema frozen and recorded in this plan's §4.

### Phase 2 — notify-diff.sh (Layer 2, the heart)
*Input gate: Phase 1 schema is frozen.*
1. Write `notify-diff_test.sh` first — one named case per §8 rule (Fires + Must-NOT-fire),
   the §8.7 invariants, and the totality/precedence/sanitization goldens:
   - Absent/empty/malformed prev ⇒ seed silently, 0 lines.
   - `mergeStateStatus=UNKNOWN` / `mergeable=UNKNOWN` / null `failingChecks` ⇒ never crash,
     emit nothing for the derived event.
   - **Eventual-consistency flap:** CLEAN→UNKNOWN→CLEAN must NOT re-fire "ready";
     empty-then-refill `failingChecks` must NOT manufacture "check-failed" (require
     `prev≠UNKNOWN ∧ now≠UNKNOWN` for §8.3/8.4/8.5).
   - **Precedence (§8.8):** a tick both BEHIND and newly-failing ⇒ exactly the check-failed
     line.
   - **headSha (§8.2):** a change to a non-descendant sha (force-push/rebase) ⇒ one "pushed"
     line.
   - **Sanitization:** title with embedded newline and title with ANSI escape ⇒
     stripped/truncated, one-line invariant, no spoofed second line.
   - Coverage assertion: every §8 rule id maps to ≥1 case.
2. Implement `notify-diff.sh` (pure `jq`, total, precedence resolver, sanitization,
   deterministic order) to pass.
- **Done-when:** `bash src/prping/notify-diff_test.sh` green; `notify-diff(now,now)` ⇒ 0
  lines; the §8 traceability table (§8 below) reproducible from the driver's case names.

### Phase 3 — prping.sh orchestrator + --print (Blocker 2)
1. Write the **lock/resume half** of `scenario_test.sh` first (it drives
   `prping.sh --print`): restart-resume (reload mid-sequence state ⇒ no replay) and
   single-writer lock (second instance no-ops/refuses).
2. Implement `prping.sh`: filename derivation → read prev → `pr-status.sh` →
   `notify-diff.sh` → **persist-new-snapshot-THEN-print**; atomic temp-file + rename;
   `umask 077` (0700 dir / 0600 file); lockfile `~/.config/prping/<name>.lock`; `--print`
   short-circuits the relay.
3. **Resolve the delivery-semantics open decision here.** *Recommended v1:* at-most-once
   via persist-then-relay (a crash drops a push, never duplicates). If at-least-once +
   no-dupes is required instead, add an emitted-ids ledger — **decide and document before
   this phase closes** (Phase 5's SKILL.md ordering note depends on it).
- **Done-when:** the lock/resume assertions in `scenario_test.sh` are green; the
  delivery-semantics choice is recorded in §4.

### Phase 4 — scenario lifecycle goldens (Layer 3 full)
1. Author `testdata/scenario/tick-00..NN.json` for the full lifecycle and the golden
   transcript. Add the **lifecycle-transcript half** of `scenario_test.sh` (concatenated
   `--print` transcript == golden).
2. Add the **fixture-lint** assertion: `scenario_test.sh` fails if any `testdata/**` file
   contains the committer login (Rule C, substring grep = stricter than privacy_guard) or
   the `$HOME` basename. All testdata uses synthetic `acme/widgets` identifiers (the §3.2
   sample literal is replaced — hygiene, §G4).
- **Done-when:** `bash src/prping/scenario_test.sh` green (both halves); fixture-lint green.

### Phase 5 — SKILL.md relay + pacing
*Input gate: Phase 3 delivery-semantics decision is locked.*
1. Write `SKILL.md`: trigger `description`; prereq checks (§6); orchestration pseudocode
   calling `prping.sh` and relaying lines to `PushNotification`; pacing on `loop` +
   re-armed `Monitor(persistent:true)`; explicit note that `CronCreate` is unsuitable and
   **`ScheduleWakeup` does not exist** (remove every reference). Encode the persist-then-relay
   ordering note (from Phase 3).
2. **Resolve the watcher-lifecycle open decision** (self-terminate at zero open PRs vs
   standing watcher) and reflect it in the pacing wording.
3. Embed the §9.4 manual-acceptance checklist and the fixture-hygiene note.
- **Done-when:** run the **real** `sync-skills.sh` (there is no `--dry-run`; verified) and
  confirm `~/.claude/skills/prping` and `~/.agents/skills/prping` are symlinks to
  `src/prping/`; SKILL.md references only real primitives.

### Phase 6 — Docs + skill-trigger eval
1. `src/prping/GEMINI.md` + `ln -s GEMINI.md CLAUDE.md`; link from `src/GEMINI.md` after the
   ssh-host-finder bullet (line 10).
2. `testdata/trigger-eval.json`: expanded corpus incl. near-miss confusers; record the exact
   command producing the ≥90% top-1 number and capture it in PR #127 as a
   **manual-but-evidenced** gate (not an automated `*_test.sh`).
- **Done-when:** GEMINI.md present + symlink + parent link; trigger-eval number recorded;
  §9.5 fallback (explicit trigger phrase if it plateaus) noted.

### Phase 7 — Full verification + DoD (gate classes kept separate)
- **Automated (CI-checkable):** `make lint-shell` green **and** all `src/prping/*.sh`
  shellcheck-clean (§G2); `make shell-test` green with **all 3** `src/prping/*_test.sh`
  (`pr-status_test.sh`, `notify-diff_test.sh`, `scenario_test.sh`) appearing in the `RUN:`
  list; every §8 rule → named §9.2 case (table in PR).
- **Human-evidenced (NOT CI-checkable, must not be conflated):** trigger-eval ≥90% recorded
  (§9.5); one manual phone-acceptance run signed off (§9.4).
- **Done-when:** both classes satisfied and recorded in PR #127.

---

## 6. Privacy / fixture rule attribution (verified)

| Identifier in a fixture/commit | privacy_guard rule it trips | Note |
| :-- | :-- | :-- |
| Committer login `<login>` (≥3 chars), as a bare word | **Rule C** (`word_present`, word-bounded, case-insensitive; lines 141–148) | `<login>` inside `<login>smith` does **not** trip (trailing char alphanumeric). The Phase-4 fixture-lint greps it as a **substring** → stricter than Rule C, deliberate defense-in-depth. |
| `$HOME/...` absolute path | **Rule B** (`/(home|users)/<homebase>`; lines 131–136) | Use `$HOME` / `~`. |
| §3.2 sample `sfc-gh-eraigosa/dotfiles` | **None** of A–D | Contains no bare `<login>` token. Replacing with `acme/widgets` is consistency/hygiene, not a block (§G4). Do not calibrate the fixture-lint against this token. |
| Real secrets/tokens | **Rule D** (lines 153–169) | Never in fixtures. |

---

## 7. §8-rule → §9.2-test traceability

| §8 rule | Fires case(s) | Must-NOT-fire case(s) |
| :-- | :-- | :-- |
| §8.1 PR opened | `opened_first_sight` | `opened_already_present`, `opened_draft`, `opened_clean_no_ready` |
| §8.2 Push landed | `push_sha_advance`, `push_force_push_nondescendant` | `push_same_sha` |
| §8.3 Ready to merge | `ready_behind_to_clean` | `ready_clean_to_clean`, `ready_draft`, `ready_unknown_flap` |
| §8.4 Needs update | `behind_green` | `behind_already`, `behind_with_failing` |
| §8.5 Check failed | `check_new_failure`, `check_clear_then_recur` | `check_same_set`, `check_empty_refill_flap` |
| §8.6 Closed / merged | `closed_disappears` (+ `merged_flag` iff merged-probe chosen in Phase 1) | `closed_no_event_after` |
| §8.7 Idempotence | `idempotent_now_now` | — |
| §8.7 No-op tick | `noop_prev_eq_now` | — |
| §8.7 Deterministic order | `order_multi_pr_one_tick` | — |
| §8.7 Restart-safe dedup | `restart_resume_no_replay` (scenario, Phase 3) | — |
| §8.7 Format (<200, one line) | `format_line_len_and_oneline` | — |
| §8.8 Precedence | `precedence_behind_and_failing` | — |
| Totality (review-added) | `total_absent_prev_seed`, `total_unknown_mergestate`, `total_null_checks` | — |
| Sanitization (review-added) | `sanitize_newline_in_title`, `sanitize_ansi_in_title` | — |
| Filename derivation (§G5) | `filename_slash_to_hyphen`, `filename_no_collision` | — |

A coverage assertion in `notify-diff_test.sh` fails if any §8 rule id lacks a case.

---

## 8. Integration + rollout

**sync-skills:** flat `src/prping/SKILL.md` is auto-discovered by the Priority-2 branch
(`sync-skills.sh` lines 127–128) and linked as `prping` into `~/.claude/skills` +
`~/.agents/skills`. No case-map entry for v1 (a friendly name needs the `skill/` subdir
Priority-1 layout — deferred). **No `build.sh`** (non-Go; `build_component` lines 89–93
no-ops). Verification is the **real** `sync-skills.sh` run + symlink inspection — there is
**no `--dry-run`** flag.

**make shell-test discovery:** the Phase-0 patch adds **`src`** (only) to the first `find`
root — the named §9.6 prerequisite — proven by the 3 `src/prping/*_test.sh` appearing in the
`RUN:` output. **`sdk` is deliberately NOT added** (§G1): its 3 pre-existing drivers
(`sdk/gsl/scripts/check-deps_test.sh`, `check-font-glyphs_test.sh`,
`sdk/gss/scripts/check-deps_test.sh`) use a different inline `assert_exit` helper and need a
Go toolchain / Nerd Font; wiring them in is out of scope for prping.

**make lint-shell:** patched to glob `src`; both comment blocks updated. New done-when:
`src/prping/*.sh` shellcheck-clean under `-S warning`.

**Docs:** `src/prping/GEMINI.md` + `CLAUDE.md -> GEMINI.md` (relative symlink), linked from
`src/GEMINI.md` after the ssh-host-finder bullet (line 10). Additive.

**State dir:** `~/.config/prping/` created at runtime 0700; per-repo JSON 0600 via
`umask 077`. Outside the repo tree; never committed; no `.gitignore` rule.

**Manual phone-acceptance checklist (in SKILL.md, signed off in PR #127 — §9.4):**
1. Start `prping` in a Remote-Control-attached session (mobile app signed in, "Push when
   Claude decides" enabled).
2. Push a commit to a synthetic open PR's branch.
3. Confirm the phone receives `✓ pushed to PR #N …` within one pacing interval.
4. While actively typing, trigger a transition — confirm the push is **suppressed**
   (harness behavior; note it is *dropped*, not deferred — consistent with at-most-once).
5. Record pass/fail and the Claude Code version in the PR.

**Rollback (§11):** delete `src/prping/` and `~/.config/prping/`; revert the Makefile and
`src/GEMINI.md` edits. Pure addition; nothing else touched.

---

## 9. Open decisions — RESOLVED 2026-06-05 (authoritative text: spec §12)

> Owner resolved all six: **delivery = at-most-once** (persist-then-relay); **scope =
> selectable + label-driven** (`current` / `all` / `label:<name>`, default label `prping`) with
> the ability to add/remove the watch label on PRs and **self-terminate when the watched set is
> empty**; **merged vs closed distinguished** via `pr-status.sh` reporting each watched PR's
> `state` (so `notify-diff` stays a pure diff); **name = bare `prping`**; fork PRs **out of
> scope v1**; §9.5 fallback = explicit trigger phrase rather than soften the ≥90% gate.
> Implementation impact: `pr-status.sh` gains `--scope` + `--state all`; a small watch-label
> add/remove helper in `prping.sh`; SKILL.md asks scope on start and stops on empty. The
> original options below are retained for rationale.

1. **Merged-vs-closed (§8.6):** recommend v1 single neutral `closed` handling (keeps
   notify-diff a pure two-list diff). **Resolve + freeze schema in Phase 1** (input gate to
   Phase 2).
2. **Delivery semantics:** at-most-once (persist-then-relay) vs at-least-once + emitted-ids
   ledger. Recommend at-most-once. **Resolve in Phase 3** (gates Phase 5's SKILL.md ordering
   note).
3. **Watcher lifecycle (§5):** self-terminate at zero open PRs vs standing watcher. **Resolve
   in Phase 5** (changes pacing wording).
4. **Filename derivation collision (§G5):** pick `/`→`-` with documented limitation + test,
   or a collision-free encoding. **Resolve in Phase 1.**
5. **Friendly slash name:** recommend bare `prping` for v1 (lowest surface, no casemap).
6. **Fork support:** if fork PRs are in scope, title/branch sanitization is an
   external-attacker path (raise §8 wording severity); if single-author/private only, it's
   integrity hygiene. State the assumption in SKILL.md.
7. **§9.5 fallback:** if trigger accuracy plateaus below 90% against confusers, soften the
   gate or adopt an explicit trigger phrase (blocks §9.6 DoD until resolved).
