# gff-install-flow — execution cursor

- **Slug:** gff-install-flow
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Ledger:** [`TRACKING.md`](./TRACKING.md)
- **Plan (source of truth):** [`../gff-install-flow.md`](../gff-install-flow.md) — every Task/step below points there; its code blocks are normative

> **How to use:** the **first unchecked box is the next action**. Tick a box only after
> you ran the command and read the output. After finishing a `###` task: update
> `TRACKING.md`, commit with the plan's exact message, checkpoint.
>
> **Legend:** `SETUP` prep · `RED` failing test · `RUN-RED` run, expect FAIL ·
> `GREEN` implement · `RUN-GREEN` run, expect PASS · `VERIFY` extra gate ·
> `ALLOWLIST` `.gitignore` check · `DOCS` · `COMMIT` · `LEDGER` · `CHECKPOINT`.

## Preflight (once)

- [x] `git log --oneline origin/main | grep -m1 191` → the `/w` fix (cd87074) on main
- [x] `gss feature list | grep -A2 gff-install-flow` → worker row present (build worker; impl merged with #193)
- [x] Read plan fully (code blocks normative) + spec §5 + the plan's behavior invariants

---

### Task 1 — `opt/lib/winsetup.sh` + test driver  (plan Task 1)

- [x] RED: write `opt/lib/winsetup_test.sh` verbatim from the plan (9 cases: choice
  round-trip, take-absent→none, sentinel-honored, sentinel-migrates, env-override-skip,
  clean-not-skipped, record_skip-gff, record_skip-sentinel-fallback, no-tty→`__notty__`)
- [x] RUN-RED: `bash opt/lib/winsetup_test.sh` → **FAIL observed**: 17 failed, `winsetup.sh: No such file`
- [x] GREEN: implement `opt/lib/winsetup.sh` (POSIX/dash-safe; deviation: tty OPEN-probe replaces `[ -r /dev/tty ]` — see TRACKING T1 note)
- [x] RUN-GREEN: `bash opt/lib/winsetup_test.sh` → 18 passed, **0 failed**
- [x] RUN-GREEN: `sh opt/lib/winsetup_test.sh` → 18 passed, **0 failed** (dash)
- [x] VERIFY: `make lint-shell && make lint-portability` → rc 0 both
- [x] ALLOWLIST: `git status --short` showed both `??` via `!opt/**`; driver mode 0755
- [x] COMMIT: `feat(install): winsetup.sh — choice persistence + gff-owned skip state (TDD)` (5bdcd45)
- [x] LEDGER (T1 row + F3 automated cell) + CHECKPOINT

**Done when:** driver green under both shells, gates clean.

---

### Task 2 — `install_windows.sh` `--ask` / `--deferred` split  (plan Task 2)

- [ ] GREEN: restructure per the plan layout — mode arg, `#!/usr/bin/env bash` shebang
  fix, winsetup sourcing, `print_prompt_text` (with the new `[s]` gff-override text),
  `notty_guidance`, `deploy_windows_files`, `run_windows_customization` (WSLENV builder
  moves inside), `--ask`/`--deferred`/`--full` dispatch; delete the SENTINEL block
- [ ] VERIFY: `bash -n opt/bin/install_windows.sh` → clean
- [ ] VERIFY: non-WSL smoke — `bash opt/bin/install_windows.sh "$PWD" --ask; echo rc=$?` → silent, `rc=0`; same for `--deferred`
- [ ] VERIFY: structural greps — `winsetup.sh` sourced before first `winsetup_` call;
  WSLENV builder appears once, inside `run_windows_customization`; `grep -n SENTINEL` → no hits
- [ ] VERIFY: `make lint-shell && make lint-portability` → rc 0 both
- [ ] COMMIT: `feat(install): split install_windows.sh into --ask/--deferred around the prompt`
- [ ] LEDGER (T2 row + F2 automated cell) + CHECKPOINT

**Done when:** all greps hit as specified; gates clean; smoke rc=0.

---

### Task 3 — `install.sh` early export + Windows-last  (plan Task 3)

- [ ] GREEN: insert the early-export block after the `opt/lib/gff.sh` source (~line 23)
- [ ] GREEN: early Windows gate call gains `--ask` (~line 67)
- [ ] GREEN: insert the `--deferred` gate block before the `WIN_SETUP_MARKER` banner (~line 619)
- [ ] VERIFY: order grep — gff.sh source → early export → `--ask` → bootstrap → `--deferred` → banner
- [ ] VERIFY: `bash -n install.sh` → clean; `make lint-shell && make lint-portability` → rc 0
- [ ] COMMIT: `feat(install): early gff export + prompt-early/Windows-last execution`
- [ ] LEDGER (T3 row + F1 automated cell) + CHECKPOINT

**Done when:** order grep exact; gates clean.

---

### Task 4 — PowerShell `-GffEnv` + log + loud failure  (plan Task 4)

- [ ] GREEN: `setup-elevated.ps1` — `param([string]$GffEnv = '')` first statement; log →
  `$env:USERPROFILE\setup-elevated.log` (+ header comment); self-elevate ArgumentList
  forwards `-GffEnv`; seeding loop after the gff.ps1 fallback (validated pairs → `$env:`)
- [ ] GREEN: `setup-apps.ps1` — `$gffPairs` collection; `Start-Process … -PassThru` with
  `-GffEnv`; exit-code warning + rerun hints; catch text aligned ("cancelled, timed out, or failed")
- [ ] VERIFY: pwsh AST parse of both files → 0 errors — or record "deferred to Task 6
  human run" in TRACKING (do NOT tick as passed without running it)
- [ ] COMMIT: `feat(install): -GffEnv argument hand-off across UAC + user-readable elevated log`
- [ ] LEDGER (T4 row + F6 automated cell; F4 parse cell per what actually ran) + CHECKPOINT

**Done when:** both files edited per plan; parse status honestly recorded.

---

### Task 5 — docs + ledgers  (plan Task 5)

- [ ] DOCS: root `AGENTS.md` install.sh bullet — prompt-early/Windows-last flow, UAC at
  END on `[y]`, `[s]` = gff override (undo: `gff unset install.windows.desktop-deploy`)
- [ ] DOCS: `opt/Desktop/Apps/scripts/AGENTS.md` — update any old-flow/log-path text
- [ ] LEDGER: `docs/mbo/index.md` row → state `building`; gff `TRACKING.md` §10 row 5
  resolution += "Fix built in gff-install-flow (PR #193): -GffEnv argument hand-off"
- [ ] COMMIT: `docs(gff-install-flow): flow docs + ledger updates (plan, index, §10 row-5 closure)`
- [ ] LEDGER + CHECKPOINT

**Done when:** no doc still describes deploy-before-prompt or the admin-only log.

---

### Task 6 — human validation matrix (owner, real WSL — never fake)  (plan Task 6)

- [ ] RUN 1 (owner): `[y]` + `gff set install.windows.wispr-flow false` → elevated log
  (user-profile path) shows `SKIP (gff: install.windows.wispr-flow=false)`; then unset
- [ ] RUN 2 (owner): `[n]` → deploy still happens at the end; no PS customization
- [ ] RUN 3 (owner): `[s]` → `gff list install.windows.desktop-deploy` = `false ·
  user-override`; `~/.config/dotfiles/.skip_windows_setup` absent
- [ ] RUN 4 (owner): flag-off → no prompt, single `SKIP (gff: install.windows.desktop-deploy=false)`; then unset
- [ ] CAPTURE: transcript slices → `../gff/evidence/F09-gating/gff-install-flow-matrix.txt`
  (usernames/hostnames redacted)
- [ ] COMMIT evidence; LEDGER (F1–F6 human cells, §3 stop-condition boxes)
- [ ] CHECKPOINT, re-apply any custom PR body AFTER this final checkpoint
- [ ] PROMOTE: `gh pr ready 193` — **owner-confirmed only**; merge — owner-gated

**Done when — objective gate:** TRACKING §3 fully green.
