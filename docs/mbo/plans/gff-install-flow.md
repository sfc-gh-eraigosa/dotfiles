# install.sh flag-flow refactor — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development
> (recommended) or superpowers:executing-plans to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

- **Slug:** gff-install-flow
- **Date:** 2026-07-26
- **Status:** Approved
- **Spec (source of truth):** [`../specs/gff-install-flow.md`](../specs/gff-install-flow.md) · PR #193 · worker `gff-install-flow/edward-raigosa/impl`
- **Execution trio:** [`gff-install-flow/IMPLEMENTATION.md`](./gff-install-flow/IMPLEMENTATION.md) (procedure + kickoff) · [`TRACKING.md`](./gff-install-flow/TRACKING.md) (evidence ledger) · [`TODO.md`](./gff-install-flow/TODO.md) (cursor — first unchecked box is the next action)

**Goal:** `gff set install.windows.<x> false && ./install.sh` works with zero manual
export steps on any machine state, including the UAC-elevated gates; prompts stay
front-loaded; the skip state moves into a gff override.

**Architecture:** New POSIX helper `opt/lib/winsetup.sh` (choice persistence +
skip-state + migration, fully unit-tested); `install_windows.sh` splits into
`--ask` (prompt only, early) / `--deferred` (deploy + customization, last) around
an extracted `run` path; `install.sh` gains a top-of-script early export and moves
Windows execution after the gff bootstrap; the PowerShell pair crosses the UAC
boundary by argument (`-GffEnv`), not environment.

**Tech stack:** POSIX sh (dash-safe) + bash, Windows PowerShell 5.1. No new deps.

**Hard rules (inherited):** `make lint-shell && make lint-portability` before every
shell commit; stage by explicit name; `bash -n` both scripts; never run
`install.sh` from this worktree (validation runs happen in `${HOME}/git/dotfiles`
on the branch, on the owner's WSL machine); evidence transcripts go to
`docs/mbo/plans/gff/evidence/F09-gating/`.

**Behavior invariants (verify, don't regress):**
1. Fresh system, no gff → identical to today (fail-open everywhere).
2. Desktop file deploy + winenv cache run on every non-skipped WSL run — they are
   gated by `install.windows.desktop-deploy` only, NOT by the y/n answer (today
   they run even on `[n]`; keep that, just later in the run).
3. `WIN_SETUP_MARKER` contract unchanged: cleared at invocation start, written on
   a successful `[y]` customization, read by install.sh's tail banner.
4. `__notty__` (CI / no terminal): print the existing guidance, never hang, never
   silently pretend the user answered.

---

### Task 1: `opt/lib/winsetup.sh` + test driver (TDD)

**Files:**
- Create: `opt/lib/winsetup.sh`
- Create: `opt/lib/winsetup_test.sh` (mode 0755)

- [ ] **Step 1: write the failing test driver first** — `opt/lib/winsetup_test.sh`:

```sh
#!/usr/bin/env bash
# winsetup_test.sh — drives opt/lib/winsetup.sh under bash AND dash (sh).
# Mirrors the opt/lib/gff_test.sh assert style. Every case runs in a scratch
# HOME-like tmpdir; a stub gff records its argv so delegation is observable.
set -u
here="$(cd -- "$(dirname "$0")" && pwd -P)"
pass=0; fail=0
ok()  { pass=$((pass+1)); echo "PASS: $1"; }
bad() { fail=$((fail+1)); echo "FAIL: $1"; }

fixture() {  # $1=with_gff(yes|no) -> sets WS_TMP, exports the override vars
  WS_TMP="$(mktemp -d)"
  export WINSETUP_CHOICE_FILE="${WS_TMP}/cache/win-setup-choice"
  export WINSETUP_SENTINEL="${WS_TMP}/config/.skip_windows_setup"
  if [ "$1" = "yes" ]; then
    mkdir -p "${WS_TMP}/bin"
    cat > "${WS_TMP}/bin/gff" <<'STUB'
#!/bin/sh
echo "$@" >> "${GFF_STUB_LOG}"
exit 0
STUB
    chmod +x "${WS_TMP}/bin/gff"
    export GFF_STUB_LOG="${WS_TMP}/gff-calls.log"
    export WINSETUP_GFF="${WS_TMP}/bin/gff"
  else
    export WINSETUP_GFF="${WS_TMP}/no-such-gff"
  fi
}

run_case() {  # $1=shell $2=desc $3=snippet ; snippet sources both libs first
  _sh="$1"; _desc="$2"; _snip="$3"
  if "$_sh" -c ". '${here}/gff.sh'; . '${here}/winsetup.sh'; ${_snip}"; then
    ok "[$_sh] $_desc"
  else
    bad "[$_sh] $_desc"
  fi
}

for sh_bin in bash sh; do
  # 1. choice round-trip: save then take echoes it back and consumes the file
  fixture no
  run_case "$sh_bin" "choice round-trip y" '
    winsetup_save_choice y &&
    [ "$(winsetup_take_choice)" = "y" ] &&
    [ ! -f "$WINSETUP_CHOICE_FILE" ]'
  rm -rf "$WS_TMP"

  # 2. take with no file -> "none"
  fixture no
  run_case "$sh_bin" "take_choice absent -> none" '
    [ "$(winsetup_take_choice)" = "none" ]'
  rm -rf "$WS_TMP"

  # 3. skip_state: sentinel present, no gff -> skipped (0), sentinel kept
  fixture no
  run_case "$sh_bin" "sentinel honored without gff" '
    mkdir -p "$(dirname "$WINSETUP_SENTINEL")" && : > "$WINSETUP_SENTINEL" &&
    winsetup_skip_state && [ -f "$WINSETUP_SENTINEL" ]'
  rm -rf "$WS_TMP"

  # 4. skip_state: sentinel + working gff -> migrated (gff set called, file gone), still 0
  fixture yes
  run_case "$sh_bin" "sentinel migrates to gff override" '
    mkdir -p "$(dirname "$WINSETUP_SENTINEL")" && : > "$WINSETUP_SENTINEL" &&
    winsetup_skip_state >/dev/null &&
    [ ! -f "$WINSETUP_SENTINEL" ] &&
    grep -q "set install.windows.desktop-deploy false" "$GFF_STUB_LOG"'
  rm -rf "$WS_TMP"

  # 5. skip_state: no sentinel, env override false -> skipped (via gff_on)
  fixture no
  run_case "$sh_bin" "env override false -> skipped" '
    GFF_INSTALL_WINDOWS_DESKTOP_DEPLOY=false winsetup_skip_state'
  rm -rf "$WS_TMP"

  # 6. skip_state: nothing set -> NOT skipped (rc 1)
  fixture no
  run_case "$sh_bin" "clean state -> not skipped" '
    ! winsetup_skip_state'
  rm -rf "$WS_TMP"

  # 7. record_skip with gff -> gff set called, no sentinel written
  fixture yes
  run_case "$sh_bin" "record_skip delegates to gff" '
    winsetup_record_skip >/dev/null &&
    grep -q "set install.windows.desktop-deploy false" "$GFF_STUB_LOG" &&
    [ ! -f "$WINSETUP_SENTINEL" ]'
  rm -rf "$WS_TMP"

  # 8. record_skip without gff -> sentinel fallback written
  fixture no
  run_case "$sh_bin" "record_skip sentinel fallback" '
    winsetup_record_skip >/dev/null && [ -f "$WINSETUP_SENTINEL" ]'
  rm -rf "$WS_TMP"

  # 9. ask with no controlling tty -> __notty__ (setsid detaches; skip if absent)
  if command -v setsid >/dev/null 2>&1; then
    fixture no
    if [ "$(setsid "$sh_bin" -c ". '${here}/gff.sh'; . '${here}/winsetup.sh'; winsetup_ask" </dev/null)" = "__notty__" ]; then
      ok "[$sh_bin] ask without tty -> __notty__"
    else
      bad "[$sh_bin] ask without tty -> __notty__"
    fi
    rm -rf "$WS_TMP"
  else
    echo "SKIP: setsid unavailable — no-tty case deferred to the human matrix"
  fi
done

echo "----------------------------------------"
echo "winsetup_test: ${pass} passed, ${fail} failed"
[ "$fail" -eq 0 ]
```

- [ ] **Step 2: run it, verify it FAILS** — `bash opt/lib/winsetup_test.sh` →
  expect FAIL/error on every case (`winsetup.sh: No such file`). Record the RED line.
- [ ] **Step 3: implement `opt/lib/winsetup.sh`** (POSIX/dash-safe — no `[[`, no
  arrays; `local` is not POSIX, use `_ws_`-prefixed vars):

```sh
# shellcheck shell=sh
# winsetup.sh — Windows-setup choice + skip-state helpers (POSIX/dash-safe).
# Sourced by opt/bin/install_windows.sh AFTER opt/lib/gff.sh (needs gff_on).
# Test seams (override via env): WINSETUP_CHOICE_FILE, WINSETUP_SENTINEL,
# WINSETUP_GFF. All functions are silent on success unless they change state.

winsetup_choice_file() { printf '%s\n' "${WINSETUP_CHOICE_FILE:-${HOME}/.cache/dotfiles/win-setup-choice}"; }
winsetup_sentinel()    { printf '%s\n' "${WINSETUP_SENTINEL:-${HOME}/.config/dotfiles/.skip_windows_setup}"; }
winsetup_gff()         { printf '%s\n' "${WINSETUP_GFF:-gff}"; }

# 0 = Windows setup is permanently skipped. Single source of truth is the gff
# override install.windows.desktop-deploy=false; a legacy sentinel file is
# migrated to it when a working gff exists, and honored as-is when not.
winsetup_skip_state() {
  _ws_sent="$(winsetup_sentinel)"
  if [ -f "$_ws_sent" ]; then
    if command -v "$(winsetup_gff)" >/dev/null 2>&1 \
       && "$(winsetup_gff)" set install.windows.desktop-deploy false >/dev/null 2>&1; then
      rm -f "$_ws_sent"
      echo "Migrated legacy skip sentinel to gff override: install.windows.desktop-deploy=false"
    fi
    return 0
  fi
  if ! gff_on install.windows.desktop-deploy; then
    return 0
  fi
  return 1
}

# Record the "[s] never ask again" choice: gff override first, sentinel fallback.
winsetup_record_skip() {
  if command -v "$(winsetup_gff)" >/dev/null 2>&1 \
     && "$(winsetup_gff)" set install.windows.desktop-deploy false; then
    echo "Recorded permanent skip as a gff override (undo: gff unset install.windows.desktop-deploy)."
  else
    _ws_sent="$(winsetup_sentinel)"
    mkdir -p "$(dirname "$_ws_sent")"
    echo "User chose to skip Windows setup on $(date)" > "$_ws_sent"
    echo "gff unavailable; wrote legacy sentinel ${_ws_sent}"
  fi
}

# Prompt on the controlling terminal; echoes y|n|s|__notty__. The caller prints
# the option text — this only reads. Anything unrecognized maps to n (skip once).
winsetup_ask() {
  if [ -r /dev/tty ]; then
    printf 'Choice [y/n/s]: ' > /dev/tty
    read -r _ws_choice < /dev/tty || _ws_choice=""
    case "$_ws_choice" in
      y|Y) echo y ;;
      s|S) echo s ;;
      *)   echo n ;;
    esac
  else
    echo __notty__
  fi
}

winsetup_save_choice() {  # $1 = y|n|s
  _ws_cf="$(winsetup_choice_file)"
  mkdir -p "$(dirname "$_ws_cf")"
  printf '%s\n' "$1" > "$_ws_cf"
}

winsetup_take_choice() {  # echoes the recorded choice or "none"; consumes the file
  _ws_cf="$(winsetup_choice_file)"
  if [ -f "$_ws_cf" ]; then
    cat "$_ws_cf"
    rm -f "$_ws_cf"
  else
    echo none
  fi
}
```

- [ ] **Step 4: run to green** — `bash opt/lib/winsetup_test.sh` then
  `sh opt/lib/winsetup_test.sh` → expect `0 failed` under both.
- [ ] **Step 5: gates** — `make lint-shell && make lint-portability` → rc 0 both.
- [ ] **Step 6: allowlist** — `git status --short -- opt/lib/winsetup.sh
  opt/lib/winsetup_test.sh` shows both (covered by `!opt/**`); `chmod +x
  opt/lib/winsetup_test.sh`.
- [ ] **Step 7: commit** —
  `git add opt/lib/winsetup.sh opt/lib/winsetup_test.sh`
  `git commit -m "feat(install): winsetup.sh — choice persistence + gff-owned skip state (TDD)"`

---

### Task 2: split `opt/bin/install_windows.sh` into `--ask` / `--deferred`

**Files:**
- Modify: `opt/bin/install_windows.sh` (whole-file restructure; every existing
  block is MOVED, not rewritten — diff should show relocation + the mode plumbing)

- [ ] **Step 1: restructure.** Target layout (element numbers = today's blocks):

```sh
#!/usr/bin/env bash          # NOTE: also fix the current #!/bin/bash shebang
# (header comment: document BASE_DIR + the mode contract)
# Usage: install_windows.sh <BASE_DIR> [--ask|--deferred]
#   --ask       WSL-detect, check skip state, prompt y/n/s, persist the choice.
#   --deferred  consume the choice: deploy Desktop files (always, per-run), then
#               run the PowerShell customization only for a recorded "y";
#               record a permanent skip for "s". Runs at the END of install.sh,
#               after the gff bootstrap has exported GFF_* (so WSLENV sees them).
#   (no mode)   legacy standalone flow: ask, then execute immediately.
set -euo pipefail
BASE_DIR="${1:-}"; MODE="${2:---full}"
[ -z "$BASE_DIR" ] && { echo "ERROR: BASE_DIR required" >&2; exit 1; }

. "<gff lib sourcing block — unchanged, incl. the fail-open stubs>"
. "$(cd -- "$(dirname "$0")" && pwd -P)/../lib/winsetup.sh"
gff_on install.windows.desktop-deploy || { gff_skip_msg install.windows.desktop-deploy; exit 0; }

WIN_SETUP_MARKER=…; rm -f "$WIN_SETUP_MARKER"   # unchanged, every invocation
grep -qi microsoft /proc/version || exit 0       # WSL guard, every mode

print_prompt_text() {  # the existing echo block, with ONE text change:
  #   [s] Skip and never ask again (recorded as a gff override).
  …
}

notty_guidance() { … }   # the existing __notty__ case body, verbatim

deploy_windows_files() {
  # today's blocks in order: binfmt interop ensure; ps_exe resolution (sets the
  # global ps_exe, exits 0 with NOTE if absent); Desktop path resolve; winenv
  # cache; opt/Desktop deploy. Verbatim moves.
}

run_windows_customization() {
  # today's [y] case body: the WSLENV /w builder (MOVED here from top-level so
  # it runs at deferred time, after the bootstrap export); setup-apps.ps1
  # invocation + log cat; wispr doc pointer; WIN_SETUP_MARKER write. Verbatim.
}

case "$MODE" in
  --ask)
    winsetup_skip_state && exit 0
    print_prompt_text
    choice="$(winsetup_ask)"
    case "$choice" in
      __notty__) notty_guidance ;;
      y) winsetup_save_choice y
         echo "Windows customization will run at the END of this install (UAC prompt then)." ;;
      n) winsetup_save_choice n ;;
      s) winsetup_save_choice s ;;
    esac
    ;;
  --deferred)
    deploy_windows_files              # per-run, independent of the y/n answer
    case "$(winsetup_take_choice)" in
      y) run_windows_customization ;;
      s) winsetup_record_skip ;;
      n|none) : ;;                    # none = ask never ran/answered: skip once
    esac
    ;;
  --full|*)
    winsetup_skip_state && exit 0
    deploy_windows_files
    print_prompt_text
    case "$(winsetup_ask)" in
      __notty__) notty_guidance ;;
      y) run_windows_customization ;;
      s) winsetup_record_skip ;;
      n) : ;;
    esac
    ;;
esac
```

  Deletions: the old `SENTINEL_DIR`/`SENTINEL_FILE` definitions and the `[s]`
  sentinel-write body (winsetup owns both paths now); the top-level WSLENV
  builder placement (moved into `run_windows_customization`).
- [ ] **Step 2: syntax + smoke** — `bash -n opt/bin/install_windows.sh` clean;
  on the non-WSL dev box: `bash opt/bin/install_windows.sh "$PWD" --ask; echo rc=$?`
  → prints nothing, `rc=0` (WSL guard). Same for `--deferred`.
- [ ] **Step 3: structural asserts** (greps, each must hit exactly where stated):
  `grep -n 'winsetup.sh' opt/bin/install_windows.sh` → one source line before the
  first `winsetup_` call; `grep -c 'GFF_INSTALL_WINDOWS_' opt/bin/install_windows.sh`
  → the WSLENV builder appears once, inside `run_windows_customization`;
  `grep -n 'SENTINEL' opt/bin/install_windows.sh` → no hits.
- [ ] **Step 4: gates** — `make lint-shell && make lint-portability` → rc 0.
- [ ] **Step 5: commit** —
  `git add opt/bin/install_windows.sh`
  `git commit -m "feat(install): split install_windows.sh into --ask/--deferred around the prompt"`

---

### Task 3: `install.sh` — early export, early ask, Windows-last

**Files:**
- Modify: `install.sh` (three surgical edits; anchor by pattern, not line number)

- [ ] **Step 1: early export.** Immediately AFTER the existing
  `. "${BASE_DIR}/opt/lib/gff.sh"` line (~23), insert:

```bash
# Early flag export (fail-open): pre-bootstrap gates read env only. If a gff
# binary already exists (any run after the first), materialize the overrides
# now so install.system/shell/pkg/tools/windows gates honor them. The
# mid-script bootstrap below remains the authoritative refresh.
if command -v gff >/dev/null 2>&1; then
  set -a
  eval "$(cd "${BASE_DIR}" && gff export --shell 2>/dev/null || true)"
  set +a
fi
```

- [ ] **Step 2: early call becomes ask-only.** In the `install.windows.desktop-deploy`
  gate (~line 67), change
  `bash "${BASE_DIR}/opt/bin/install_windows.sh" "${BASE_DIR}"` →
  `bash "${BASE_DIR}/opt/bin/install_windows.sh" "${BASE_DIR}" --ask`.
- [ ] **Step 3: deferred execution at the tail.** Immediately BEFORE the
  `WIN_SETUP_MARKER` banner block (~line 619, `if [ -f "$WIN_SETUP_MARKER" ]`),
  insert:

```bash
# Deferred Windows execution (the y/n/s answer was captured up top): runs AFTER
# the gff bootstrap so install.windows.* overrides are exported — including on
# a fresh system. See docs/mbo/specs/gff-install-flow.md.
if gff_on install.windows.desktop-deploy; then
  if [ -f "${BASE_DIR}/opt/bin/install_windows.sh" ]; then
    bash "${BASE_DIR}/opt/bin/install_windows.sh" "${BASE_DIR}" --deferred
  fi
else gff_skip_msg install.windows.desktop-deploy; fi
```

- [ ] **Step 4: verify order** — `grep -n 'install_windows.sh\|opt/lib/gff.sh\|command -v gff' install.sh`
  must show: gff.sh source → early export → `--ask` → (bootstrap, far later) →
  `--deferred` → banner. `bash -n install.sh` clean.
- [ ] **Step 5: gates** — `make lint-shell && make lint-portability` → rc 0.
- [ ] **Step 6: commit** —
  `git add install.sh`
  `git commit -m "feat(install): early gff export + prompt-early/Windows-last execution"`

---

### Task 4: PowerShell — `-GffEnv` UAC hand-off, log relocation, loud failure

**Files:**
- Modify: `opt/Desktop/Apps/scripts/setup-apps.ps1` (elevated block, ~line 485)
- Modify: `opt/Desktop/Apps/scripts/setup-elevated.ps1` (param, log path,
  seeding, self-elevate passthrough)

- [ ] **Step 1: `setup-elevated.ps1` — param + seeding + log.**
  - Very first statement of the file (before `$ErrorActionPreference`; `param()`
    must precede all other statements — move the comment header above it is fine):

```powershell
param([string]$GffEnv = '')
```

  - Log path (line ~15): `$log = 'C:\Windows\Temp\setup-elevated.log'` →
    `$log = Join-Path $env:USERPROFILE 'setup-elevated.log'`; update the header
    comment's log mention (line ~7) to `%USERPROFILE%\setup-elevated.log`.
  - Self-elevate passthrough (line ~28-30): the `Start-Process … -ArgumentList`
    array gains `,'-GffEnv',"`"$GffEnv`""` after the `-File` pair.
  - Seeding: immediately AFTER the `lib\gff.ps1` source/fallback block (~line 42):

```powershell
# GFF_* does not survive the UAC boundary as environment (Start-Process -Verb
# RunAs starts a fresh env — dotfiles gff TRACKING §10). The parent passes the
# flag states as -GffEnv "NAME=value;…"; seed them into $env: so Test-GffOn
# works unchanged. Strictly validated; malformed pairs are logged and ignored.
foreach ($pair in ($GffEnv -split ';' | Where-Object { $_ })) {
    if ($pair -match '^(GFF_INSTALL_WINDOWS_[A-Z_]+)=(true|false|[a-z0-9,-]+)$') {
        Set-Item -Path ('env:' + $Matches[1]) -Value $Matches[2]
    } else { Log "  WARNING: ignored malformed -GffEnv pair: $pair" }
}
```

- [ ] **Step 2: `setup-apps.ps1` — collect + pass + fail loud.** In the elevated
  block, before the `Start-Process`:

```powershell
# Collect the WSLENV-delivered gff flags for the elevated child — env does not
# cross the UAC boundary, so they ride as an argument (see setup-elevated.ps1).
$gffPairs = (Get-ChildItem env: |
    Where-Object { $_.Name -match '^GFF_INSTALL_WINDOWS_[A-Z_]+$' } |
    ForEach-Object { "$($_.Name)=$($_.Value)" }) -join ';'
```

  Replace the `Start-Process … -Wait` call + success line with:

```powershell
$p = Start-Process powershell -Verb RunAs -WorkingDirectory $env:SystemRoot -ArgumentList @(
    '-NoProfile','-ExecutionPolicy','Bypass','-File',"`"$elev`"",'-GffEnv',"`"$gffPairs`"") -Wait -PassThru
Write-Host "Elevated setup finished. Details: $env:USERPROFILE\setup-elevated.log" -ForegroundColor Green
if ($p.ExitCode -ne 0) {
    Write-Warning "Elevated setup exited with code $($p.ExitCode) — it may have been cancelled or timed out at the UAC prompt."
    Write-Warning "Re-run it from a native PowerShell (approve UAC):"
    Write-Warning "  powershell -ExecutionPolicy Bypass -File `"$elev`" -GffEnv `"$gffPairs`""
}
```

  Keep the existing `catch` (UAC declined throws) but align its text: "Elevated
  setup was cancelled, timed out, or failed" + the same two rerun-hint lines.
- [ ] **Step 3: parse check.** If `pwsh` is available:
  `pwsh -NoProfile -Command "[void][System.Management.Automation.Language.Parser]::ParseFile('opt/Desktop/Apps/scripts/setup-elevated.ps1',[ref]$null,[ref]$e); $e.Count"`
  → `0`, same for setup-apps.ps1. If pwsh is absent (expected in this sandbox),
  record "parse check deferred to the Task 6 human run" — do NOT tick as passed.
- [ ] **Step 4: commit** —
  `git add opt/Desktop/Apps/scripts/setup-apps.ps1 opt/Desktop/Apps/scripts/setup-elevated.ps1`
  `git commit -m "feat(install): -GffEnv argument hand-off across UAC + user-readable elevated log"`

---

### Task 5: docs + ledgers

**Files:**
- Modify: `CLAUDE.md` (root — via its `AGENTS.md` symlink target)
- Modify: `opt/Desktop/Apps/scripts/AGENTS.md` (if it references the old flow/log)
- Modify: `docs/mbo/index.md`, `docs/mbo/plans/gff/TRACKING.md`

- [ ] **Step 1: root docs.** The root `AGENTS.md` bullet
  "**`install.sh` is interactive — never run it non-interactively…**" states the
  file deploy happens BEFORE the prompt and PowerShell runs mid-script — update
  it: prompt (and sudo) happen in the first minute, the Windows deploy +
  customization run at the END (after the gff bootstrap), a `[y]` answer means a
  UAC prompt at the END of the run, `[s]` records
  `install.windows.desktop-deploy=false` as a gff override (undo with
  `gff unset`). Grep `opt/Desktop/Apps/scripts/AGENTS.md` for
  `C:\Windows\Temp\setup-elevated.log` / prompt-flow text and update likewise.
- [ ] **Step 2: ledgers.** `docs/mbo/index.md` row `gff-install-flow`: plan link
  → `./plans/gff-install-flow.md`, PR column → #193, state → `building`.
  `docs/mbo/plans/gff/TRACKING.md` §10 UAC row (row 5), Resolution column:
  append "Fix built in `gff-install-flow` (PR #193): -GffEnv argument hand-off".
- [ ] **Step 3: commit** —
  `git add CLAUDE.md AGENTS.md opt/Desktop/Apps/scripts/AGENTS.md docs/mbo/index.md docs/mbo/plans/gff/TRACKING.md`
  (drop paths with no changes)
  `git commit -m "docs(gff-install-flow): flow docs + ledger updates (plan, index, §10 row-5 closure)"`

---

### Task 6: human validation matrix (owner, real WSL — never fake)

**Files:**
- Create: `docs/mbo/plans/gff/evidence/F09-gating/gff-install-flow-matrix.txt`

- [ ] **Step 1: hand the owner the four-run sheet** (from `${HOME}/git/dotfiles`
  on the PR branch, real interactive terminal):

```sh
git fetch origin && git checkout feature/gff-install-flow/edward-raigosa/impl
# Run 1 — [y] + elevated flag off (THE original P2-T5 line, now via -GffEnv):
gff set install.windows.wispr-flow false
./install.sh          # answer y; UAC fires at the END of the run
cat /mnt/c/Users/<winuser>/setup-elevated.log   # expect: SKIP (gff: install.windows.wispr-flow=false)
gff unset install.windows.wispr-flow
# Run 2 — [n]: answer n; verify the deploy still happens at the end, no PS setup.
./install.sh
# Run 3 — [s]: answer s; verify the override (no sentinel file):
./install.sh
gff list install.windows.desktop-deploy         # expect: false · user-override
ls ~/.config/dotfiles/.skip_windows_setup       # expect: No such file
# Run 4 — flag-off: verify no prompt at all, one SKIP line, then re-enable:
./install.sh                                    # expect SKIP (gff: install.windows.desktop-deploy=false), no prompt
gff unset install.windows.desktop-deploy
```

  Note: the early export means runs 2–4 need NO manual eval — that's part of
  what's being proven. No `set -a` anywhere in the sheet.
- [ ] **Step 2: capture** the transcript slices (prompt block, tail execution,
  elevated-log SKIP, `gff list` output) into
  `docs/mbo/plans/gff/evidence/F09-gating/gff-install-flow-matrix.txt`
  (redact usernames/hostnames per the privacy rule).
- [ ] **Step 3: commit** the evidence; update spec `Status:` stays Approved; add
  the evidence link to the PR body (after the FINAL checkpoint — gss regenerates
  the body on every checkpoint, §7.1 correction 3).
- [ ] **Step 4: promote** — checkpoint, then `gh pr ready 193` (§7.1 correction 4)
  **only after owner confirmation**; merge is owner-gated.

---

## Traceability (spec § → task)

| Spec feature | Task(s) | Proven by |
| :-- | :-- | :-- |
| F1 early export | 3 | Task 3 step 4 order-grep + matrix runs 2–4 (no manual eval) |
| F2 prompt-early / Windows-last | 2, 3 | winsetup tests 1–2, 9 + matrix run ordering |
| F3 gff-owned skip state | 1, 2 | winsetup tests 3–8 + matrix run 3 |
| F4 UAC argument hand-off | 4 | matrix run 1 elevated-log SKIP |
| F5 readable elevated log | 4 | matrix run 1 `cat` via /mnt/c, no elevation |
| F6 loud UAC failure | 4 | code (PassThru + catch); optional live decline probe |
```
