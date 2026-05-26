# AI Plugin Manifest — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Claude Code plugin set reproducible from a tracked manifest (`ai/plugins.yaml`) that `install.sh` consumes, with a schema and code path ready for Gemini extensions.

**Architecture:** A YAML manifest is the source of truth. A new `sync-plugins.sh` (ensure-only/additive) parses it with mikefarah `yq` and runs `claude plugin marketplace add` + `claude plugin install`/`enable`. `install.sh` provisions `yq` (like it does `sops`) and then calls `sync-plugins.sh`. A `sync-plugins` alias allows ad-hoc re-syncs.

**Tech Stack:** Bash, mikefarah `yq` (Go YAML processor), Claude Code plugin CLI, Gemini CLI extensions.

**Reference spec:** `docs/plans/ai-plugin-manifest.md`

**Working location:** All edits happen in the gss worktree on branch
`feature/ai-plugins/edward-raigosa/plan` (PR #24). Never edit on `main`. After
the tasks, a final `gss feature checkpoint` updates the PR (token required;
note the **manual `git push -u origin <branch>` first** gotcha — checkpoint's
`gh pr create` needs the branch on `origin` already).

---

## File Structure

| File | Responsibility |
|------|----------------|
| `ai/plugins.yaml` | **New.** Source-of-truth manifest: marketplaces + plugins, per-platform blocks |
| `opt/scripts/system/install_yq.sh` | **New.** Provision mikefarah `yq` (Linux/WSL binary fetch; no-op on macOS) |
| `opt/scripts/system/sync-plugins.sh` | **New.** Ensure-only engine: parse manifest, install + enable plugins |
| `opt/scripts/system/sync-plugins_test.sh` | **New.** Assert driver for `sync-plugins.sh --dry-run` |
| `opt/profiles/packages.tsv` | **Modify.** Add `yq` (brew only; apt `-`) |
| `install.sh` | **Modify.** Call `install_yq.sh` (near sops) + `sync-plugins.sh` (after claude install) |
| `ai/claude/aliases.sh` | **Modify.** Add `sync-plugins` alias |
| `ai/claude/settings.json.template` | **Modify.** Allowlist the new alias so it doesn't prompt |
| `opt/scripts/system/GEMINI.md` + `CLAUDE.md` | **Modify.** Document the two new scripts |

---

## Task 1: `install_yq.sh` — provision mikefarah yq

**Files:**
- Create: `opt/scripts/system/install_yq.sh`

**Why:** macOS gets mikefarah `yq` from `packages.tsv` (brew). On Debian/Ubuntu/WSL the apt `yq` package is the unrelated **kislyuk Python** wrapper with incompatible syntax, so we fetch the official mikefarah static binary into `~/opt/bin`. This mirrors `opt/scripts/system/install_sops.sh` exactly, plus an `armv7l→arm` arch case for 32-bit Raspberry Pi.

- [ ] **Step 1: Write the script**

```bash
#!/usr/bin/env bash
# ==============================================================================
# yq Binary Setup — install mikefarah `yq` (YAML processor) into ~/opt/bin
# ==============================================================================
# Why this exists:
#   * macOS installs yq from opt/profiles/packages.tsv (brew = mikefarah yq).
#   * Debian/Ubuntu/WSL apt ships `yq` as the unrelated *kislyuk Python* wrapper
#     (different, jq-style syntax), so the packages.tsv apt path can't provide
#     the tool sync-plugins.sh expects.
#   * mikefarah yq is a single static Go binary, so fetching the official
#     release is the most portable option (mirrors install_sops.sh).
#
# Safe to re-run. Override the version with YQ_VERSION=x.y.z.
set -e

YQ_VERSION="${YQ_VERSION:-4.44.6}"
INSTALL_DIR="${HOME}/opt/bin"

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

# Already present, runnable, AND the mikefarah build? Nothing to do. Probing
# `yq --version | grep mikefarah` also means a stale kislyuk yq or a dangling
# symlink is treated as "not installed" and gets replaced below.
if command -v yq >/dev/null 2>&1 && yq --version 2>&1 | grep -qi mikefarah; then
    echo -e "${GREEN}yq already installed: $(yq --version 2>&1 | head -1)${NC}"
    exit 0
fi

# --- OS / arch detection (matches the mikefarah/yq release asset naming) ---
case "$(uname -s)" in
    Linux)  YQ_OS="linux"  ;;
    Darwin) YQ_OS="darwin" ;;
    *) echo -e "${RED}install_yq: unsupported OS $(uname -s)${NC}"; exit 1 ;;
esac
case "$(uname -m)" in
    x86_64)         YQ_ARCH="amd64" ;;
    arm64|aarch64)  YQ_ARCH="arm64" ;;
    armv7l|armv6l)  YQ_ARCH="arm"   ;;
    *) echo -e "${RED}install_yq: unsupported arch $(uname -m)${NC}"; exit 1 ;;
esac

mkdir -p "${INSTALL_DIR}"

# Clear any stale entry (including a dangling symlink) before writing the binary.
if [ -e "${INSTALL_DIR}/yq" ] || [ -L "${INSTALL_DIR}/yq" ]; then
    rm -f "${INSTALL_DIR}/yq"
fi

URL="https://github.com/mikefarah/yq/releases/download/v${YQ_VERSION}/yq_${YQ_OS}_${YQ_ARCH}"
echo -e "${BLUE}Installing yq ${YQ_VERSION} (${YQ_OS}/${YQ_ARCH}) to ${INSTALL_DIR}/yq...${NC}"
curl -fsSL "$URL" -o "${INSTALL_DIR}/yq"
chmod +x "${INSTALL_DIR}/yq"

if command -v yq >/dev/null 2>&1 && yq --version >/dev/null 2>&1; then
    echo -e "${GREEN}yq installed: $(yq --version 2>&1 | head -1)${NC}"
else
    echo -e "${RED}install_yq: yq not resolvable after install — is ${INSTALL_DIR} on PATH?${NC}"
    echo -e "${RED}            (binary written to ${INSTALL_DIR}/yq)${NC}"
fi
```

- [ ] **Step 2: Make it executable**

Run: `chmod +x opt/scripts/system/install_yq.sh`

- [ ] **Step 3: Run it (this also installs yq locally, needed for later tasks)**

Run: `bash opt/scripts/system/install_yq.sh`
Expected: prints `Installing yq 4.44.6 (linux/amd64) ...` then `yq installed: ... version v4.44.6` (or your platform/arch).

- [ ] **Step 4: Verify idempotency (the real test)**

Run: `bash opt/scripts/system/install_yq.sh`
Expected: prints `yq already installed: ...` and exits 0 without re-downloading.

Also verify the binary works:
Run: `yq --version`
Expected: output contains `mikefarah` and `version v4.`

- [ ] **Step 5: Commit**

```bash
git add opt/scripts/system/install_yq.sh
git commit -m "feat(system): add install_yq.sh to provision mikefarah yq"
```

---

## Task 2: Add `yq` to the package manifest

**Files:**
- Modify: `opt/profiles/packages.tsv`

**Why:** On macOS `pkg-install-brew` reads `packages.tsv`; the brew `yq` formula IS the mikefarah build. The apt column is `-` because the Debian/Ubuntu `yq` package is the wrong (kislyuk) tool — Linux/WSL gets yq from `install_yq.sh` instead.

- [ ] **Step 1: Add the row after the `jq`/`jc` lines**

Find this block in `opt/profiles/packages.tsv`:

```
jq                   jq
jc                   jc
```

Change it to:

```
jq                   jq
jc                   jc
# yq: brew ships the mikefarah build; the apt `yq` is the unrelated kislyuk
# Python wrapper (incompatible syntax), so Linux/WSL gets yq from
# opt/scripts/system/install_yq.sh instead — hence the `-` in the APT column.
yq                   -
```

- [ ] **Step 2: Verify the manifest still parses (no tabs/format breakage)**

Run: `grep -nP '^\S+\s+\S+' opt/profiles/packages.tsv | grep yq`
Expected: shows the `yq                   -` line.

- [ ] **Step 3: Commit**

```bash
git add opt/profiles/packages.tsv
git commit -m "feat(packages): add yq (brew mikefarah build) to common core"
```

---

## Task 3: Create the manifest `ai/plugins.yaml`

**Files:**
- Create: `ai/plugins.yaml`

**Why:** The single source of truth. Seeded with all 12 currently-installed Claude plugins (verified live from `claude plugin list`, `installed_plugins.json`, and `enabledPlugins`). Tracked via the `!ai/**` allowlist rule.

- [ ] **Step 1: Write the manifest**

```yaml
# AI-assistant plugins/extensions — source of truth.
# See docs/plans/ai-plugin-manifest.md for the design.
#
# Ensure-only (additive): opt/scripts/system/sync-plugins.sh installs + enables
# what's listed here; it never removes anything. Parsed with mikefarah `yq`.
#
#   enabled: true   -> install AND enable for each platform block present
#   enabled: false  -> parked: documented but not installed/enabled
#   per-plugin `claude:` / `gemini:` blocks are optional; a missing block means
#   that tool is skipped for that row.

marketplaces:
  claude-plugins-official:
    claude: anthropics/claude-plugins-official   # arg to `claude plugin marketplace add`

plugins:
  - name: superpowers
    enabled: true
    claude: { plugin: superpowers@claude-plugins-official }
  - name: github
    enabled: true
    claude: { plugin: github@claude-plugins-official }
  - name: code-review
    enabled: true
    claude: { plugin: code-review@claude-plugins-official }
  - name: skill-creator
    enabled: true
    claude: { plugin: skill-creator@claude-plugins-official }
  - name: claude-md-management
    enabled: true
    claude: { plugin: claude-md-management@claude-plugins-official }
  - name: pr-review-toolkit
    enabled: true
    claude: { plugin: pr-review-toolkit@claude-plugins-official }
  - name: gopls-lsp
    enabled: true
    claude: { plugin: gopls-lsp@claude-plugins-official }
  - name: remember
    enabled: true
    claude: { plugin: remember@claude-plugins-official }
  - name: deploy-on-aws
    enabled: true
    claude: { plugin: deploy-on-aws@claude-plugins-official }
  - name: aws-serverless
    enabled: true
    claude: { plugin: aws-serverless@claude-plugins-official }
  - name: aws-core
    enabled: true
    claude: { plugin: aws-core@claude-plugins-official }
  - name: mcp-apps
    enabled: true
    claude: { plugin: mcp-apps@claude-plugins-official }

  # Future Gemini equivalent (illustrative — none exist today):
  # - name: some-tool
  #   enabled: true
  #   gemini: { source: https://github.com/owner/repo }
```

- [ ] **Step 2: Verify yq parses it and finds all 12 plugins**

Run: `yq '.plugins[] | select(.enabled == true) | .claude.plugin' ai/plugins.yaml | grep -c '@claude-plugins-official'`
Expected: `12`

Run: `yq '.marketplaces[].claude' ai/plugins.yaml`
Expected: `anthropics/claude-plugins-official`

- [ ] **Step 3: Confirm git sees it (allowlist sanity)**

Run: `git status --short ai/plugins.yaml`
Expected: `?? ai/plugins.yaml` (not silently ignored).

- [ ] **Step 4: Commit**

```bash
git add ai/plugins.yaml
git commit -m "feat(ai): add plugins.yaml manifest seeded with 12 Claude plugins"
```

---

## Task 4: `sync-plugins.sh` + test (TDD)

**Files:**
- Create: `opt/scripts/system/sync-plugins.sh`
- Test: `opt/scripts/system/sync-plugins_test.sh`

**Why:** The ensure-only engine. `--dry-run` previews actions (and runs even when the `claude`/`gemini` CLIs are absent) so it's testable anywhere `yq` and the manifest exist.

- [ ] **Step 1: Write the failing test**

Create `opt/scripts/system/sync-plugins_test.sh`:

```bash
#!/usr/bin/env bash
# Test driver for sync-plugins.sh --dry-run. Mirrors safety_guard_test.sh style.
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SYNC="${SCRIPT_DIR}/sync-plugins.sh"

PASS=0
FAIL=0

assert_contains() {
    local haystack="$1" needle="$2" desc="$3"
    if printf '%s' "$haystack" | grep -qF -- "$needle"; then
        echo "PASS: $desc"; PASS=$((PASS+1))
    else
        echo "FAIL: $desc (missing: $needle)"; FAIL=$((FAIL+1))
    fi
}

assert_eq() {
    local got="$1" want="$2" desc="$3"
    if [ "$got" = "$want" ]; then
        echo "PASS: $desc"; PASS=$((PASS+1))
    else
        echo "FAIL: $desc (got '$got' want '$want')"; FAIL=$((FAIL+1))
    fi
}

OUT="$(bash "$SYNC" --dry-run 2>&1)"

assert_contains "$OUT" "DRY-RUN: claude plugin marketplace add anthropics/claude-plugins-official" "adds the official marketplace"
assert_contains "$OUT" "DRY-RUN: claude plugin install superpowers@claude-plugins-official" "installs superpowers"
assert_contains "$OUT" "DRY-RUN: claude plugin install mcp-apps@claude-plugins-official" "installs mcp-apps"

INSTALL_COUNT="$(printf '%s' "$OUT" | grep -c 'DRY-RUN: claude plugin install ')"
assert_eq "$INSTALL_COUNT" "12" "plans install for all 12 plugins"

ENABLE_COUNT="$(printf '%s' "$OUT" | grep -c 'DRY-RUN: claude plugin enable ')"
assert_eq "$ENABLE_COUNT" "12" "plans enable for all 12 plugins"

echo "----"
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ]
```

Run: `chmod +x opt/scripts/system/sync-plugins_test.sh`

- [ ] **Step 2: Run the test to verify it fails**

Run: `bash opt/scripts/system/sync-plugins_test.sh`
Expected: FAIL — `sync-plugins.sh` doesn't exist yet (errors / non-zero exit).

- [ ] **Step 3: Write `sync-plugins.sh`**

```bash
#!/usr/bin/env bash
# sync-plugins.sh — ensure the AI-assistant plugins declared in ai/plugins.yaml
# are installed and enabled. Ensure-only (additive): never removes anything.
# Mirrors sync-skills.sh. Safe to re-run.
#
# Usage:
#   sync-plugins.sh            install + enable per the manifest
#   sync-plugins.sh --dry-run  print planned actions, change nothing (and
#                              previews even when the claude/gemini CLIs are absent)
set -u

# Resolve the real repo root even when invoked via the ~/opt symlink.
SCRIPT_PATH="$(readlink -f "$0")"
BASE_DIR="$(cd "$(dirname "$SCRIPT_PATH")/../../.." && pwd)"
MANIFEST="${BASE_DIR}/ai/plugins.yaml"

DRY_RUN=0
case "${1:-}" in
    --dry-run) DRY_RUN=1 ;;
    --help|-h)
        echo "Usage: sync-plugins [--dry-run]"
        echo "Install + enable the AI plugins listed in ai/plugins.yaml (ensure-only)."
        exit 0 ;;
    "") ;;
    *) echo "sync-plugins: unknown argument '$1'" >&2; exit 2 ;;
esac

run() {
    if [ "$DRY_RUN" = "1" ]; then
        echo "DRY-RUN: $*"
    else
        echo "+ $*"
        "$@" || echo "sync-plugins: WARNING — '$*' failed; continuing." >&2
    fi
}

if ! command -v yq >/dev/null 2>&1; then
    echo "sync-plugins: 'yq' not found. Install it via opt/scripts/system/install_yq.sh" >&2
    exit 1
fi
if [ ! -f "$MANIFEST" ]; then
    echo "sync-plugins: manifest not found: $MANIFEST" >&2
    exit 1
fi

sync_claude() {
    if [ "$DRY_RUN" = "0" ] && ! command -v claude >/dev/null 2>&1; then
        echo "sync-plugins: 'claude' CLI not on PATH; skipping Claude plugins."
        return 0
    fi
    # Marketplaces (idempotent add).
    while IFS= read -r src; do
        [ -z "$src" ] || [ "$src" = "null" ] && continue
        run claude plugin marketplace add "$src"
    done < <(yq '.marketplaces[] | select(.claude != null) | .claude' "$MANIFEST")
    # Install + enable each enabled plugin that has a claude.plugin.
    while IFS= read -r plugin; do
        [ -z "$plugin" ] || [ "$plugin" = "null" ] && continue
        run claude plugin install "$plugin"
        run claude plugin enable "$plugin"
    done < <(yq '.plugins[] | select(.enabled == true) | select(.claude.plugin != null) | .claude.plugin' "$MANIFEST")
}

sync_gemini() {
    if [ "$DRY_RUN" = "0" ] && ! command -v gemini >/dev/null 2>&1; then
        echo "sync-plugins: 'gemini' CLI not on PATH; skipping Gemini extensions."
        return 0
    fi
    local any=0
    while IFS= read -r source; do
        [ -z "$source" ] || [ "$source" = "null" ] && continue
        any=1
        run gemini extensions install "$source"
    done < <(yq '.plugins[] | select(.enabled == true) | select(.gemini.source != null) | .gemini.source' "$MANIFEST")
    [ "$any" = "0" ] && echo "sync-plugins: no Gemini extension sources in manifest (nothing to do)."
}

echo "Syncing AI plugins from ${MANIFEST}$([ "$DRY_RUN" = "1" ] && echo ' (dry-run)')..."
sync_claude
sync_gemini
echo "sync-plugins: done."
```

Run: `chmod +x opt/scripts/system/sync-plugins.sh`

- [ ] **Step 4: Run the test to verify it passes**

Run: `bash opt/scripts/system/sync-plugins_test.sh`
Expected: all PASS, ends with `PASS=5 FAIL=0` and exit 0.

- [ ] **Step 5: Eyeball the dry-run output**

Run: `bash opt/scripts/system/sync-plugins.sh --dry-run`
Expected: one `marketplace add` line, then 12 `install` + 12 `enable` lines, then the "no Gemini extension sources" note, then `done.`

- [ ] **Step 6: Commit**

```bash
git add opt/scripts/system/sync-plugins.sh opt/scripts/system/sync-plugins_test.sh
git commit -m "feat(system): add sync-plugins.sh ensure-only manifest sync + test"
```

---

## Task 5: Wire `install.sh`

**Files:**
- Modify: `install.sh` (two insertions)

**Why:** Provision `yq` alongside `sops`, and run the plugin sync after the Claude CLI is installed.

- [ ] **Step 1: Add the yq install next to the sops block**

In `install.sh`, find the sops block (around lines 193–199):

```bash
if [ -f "${BASE_DIR}/opt/scripts/system/install_sops.sh" ]; then
    echo "Installing sops..."
    "${BASE_DIR}/opt/scripts/system/install_sops.sh" || echo "WARNING: sops install reported problems; continuing."
fi
```

Immediately AFTER that block, add:

```bash
# Install yq (YAML processor). Same rationale as sops: macOS gets the mikefarah
# build from packages.tsv (brew); Linux/WSL fetches the official binary because
# the apt `yq` is the incompatible kislyuk variant. Needed by sync-plugins.sh.
if [ -f "${BASE_DIR}/opt/scripts/system/install_yq.sh" ]; then
    echo "Installing yq..."
    "${BASE_DIR}/opt/scripts/system/install_yq.sh" || echo "WARNING: yq install reported problems; continuing."
fi
```

- [ ] **Step 2: Add the plugin sync after the Claude settings block**

In `install.sh`, find the end of the Claude settings symlink block (around line 354, the `ln -sf "${CLAUDE_SETTINGS}" "${HOME}/.claude/settings.json"` and its closing `fi`). Immediately AFTER that closing `fi`, add:

```bash
# Sync AI plugins from the manifest (ai/plugins.yaml). Ensure-only: installs +
# enables the listed plugins; never removes anything. Runs after the Claude CLI
# (claude_install.sh) and yq are installed.
if [ -f "${BASE_DIR}/opt/scripts/system/sync-plugins.sh" ]; then
    echo "Syncing AI plugins..."
    "${BASE_DIR}/opt/scripts/system/sync-plugins.sh" || echo "WARNING: plugin sync reported problems; continuing."
fi
```

- [ ] **Step 3: Syntax-check install.sh**

Run: `bash -n install.sh`
Expected: no output, exit 0.

Run: `shellcheck install.sh || true`
Expected: no NEW errors introduced by the two added blocks (pre-existing warnings are out of scope).

- [ ] **Step 4: Commit**

```bash
git add install.sh
git commit -m "feat(install): provision yq and sync AI plugins from manifest"
```

---

## Task 6: `sync-plugins` alias + settings allowlist

**Files:**
- Modify: `ai/claude/aliases.sh`
- Modify: `ai/claude/settings.json.template`

**Why:** A standalone re-sync command (mirrors `sync-skills`), and pre-allowing it avoids permission prompts.

- [ ] **Step 1: Add the alias**

In `ai/claude/aliases.sh`, find:

```sh
alias sync-skills="bash $HOME/opt/scripts/system/sync-skills.sh"
```

Add directly below it:

```sh
alias sync-plugins="bash $HOME/opt/scripts/system/sync-plugins.sh"
```

- [ ] **Step 2: Allowlist it in the settings template**

In `ai/claude/settings.json.template`, find the allow entry:

```json
      "Bash(sync-skills:*)",
```

Add directly below it:

```json
      "Bash(sync-plugins:*)",
      "Bash(bash $HOME/opt/scripts/system/sync-plugins.sh:*)",
```

- [ ] **Step 3: Validate the template is still valid JSON**

Run: `jq empty ai/claude/settings.json.template && echo OK`
Expected: `OK`

- [ ] **Step 4: Commit**

```bash
git add ai/claude/aliases.sh ai/claude/settings.json.template
git commit -m "feat(claude): add sync-plugins alias and allowlist it"
```

---

## Task 7: Document the new scripts

**Files:**
- Modify: `opt/scripts/system/GEMINI.md`
- Modify: `opt/scripts/system/CLAUDE.md` (only if it is NOT a symlink to GEMINI.md)

**Why:** The `opt/scripts/system/` registry should list the two new scripts so agent-based discovery finds them.

- [ ] **Step 1: Check whether CLAUDE.md is a symlink**

Run: `ls -l opt/scripts/system/CLAUDE.md`
Expected: tells you if it's `-> GEMINI.md` (a symlink — then only edit GEMINI.md) or a real file (edit both).

- [ ] **Step 2: Add registry entries to GEMINI.md**

Open `opt/scripts/system/GEMINI.md`, find where the other `install_*.sh` / `sync-*.sh` scripts are listed, and add two entries matching the existing format, for example:

```markdown
- **`install_yq.sh`**: Installs mikefarah `yq` (YAML processor) into `~/opt/bin` on Linux/WSL; no-op on macOS (brew provides it via `packages.tsv`). Mirrors `install_sops.sh`.
- **`sync-plugins.sh`**: Ensure-only sync of AI-assistant plugins from `ai/plugins.yaml` (installs + enables Claude plugins; ready for Gemini extensions). `--dry-run` previews. Companion test: `sync-plugins_test.sh`.
```

(Match the surrounding bullet/heading style exactly; if entries are alphabetized, place them accordingly.)

- [ ] **Step 3: If CLAUDE.md is a real file, mirror the same two entries there.**

- [ ] **Step 4: Commit**

```bash
git add opt/scripts/system/GEMINI.md
# add opt/scripts/system/CLAUDE.md too only if it was edited (not a symlink)
git commit -m "docs(system): register install_yq.sh and sync-plugins.sh"
```

---

## Task 8: End-to-end verification + checkpoint PR #24

**Files:** none (verification + PR update)

- [ ] **Step 1: Full dry-run from the alias path**

Run: `bash opt/scripts/system/sync-plugins.sh --dry-run`
Expected: marketplace add + 12 installs + 12 enables + Gemini no-op note.

- [ ] **Step 2: Real sync is idempotent (plugins already installed on this host)**

Run: `bash opt/scripts/system/sync-plugins.sh`
Expected: each `claude plugin install`/`enable` reports already-installed/enabled; exit without error. (This mutates nothing new because all 12 are present.)

- [ ] **Step 3: Re-run the test driver**

Run: `bash opt/scripts/system/sync-plugins_test.sh`
Expected: `PASS=5 FAIL=0`.

- [ ] **Step 4: Confirm the worktree tree is clean and review the diff**

Run: `git -C . status --short` and `git log --oneline main..HEAD`
Expected: clean tree; commits for tasks 1–7.

- [ ] **Step 5: Checkpoint the PR** (per gss rules — two separate calls, token first; push the branch manually first if checkpoint's `gh pr create` complains the head ref is blank)

```bash
# (separate call) generate token from worktree HEAD:
mkdir -p ~/.config/gss && git rev-parse HEAD > ~/.config/gss/approval.token
# (separate call) update PR #24:
gss feature checkpoint
```

Expected: `Checkpoint ... PR .../pull/24 (draft)`.

---

## Self-Review notes (author)

- **Spec coverage:** manifest (Task 3), `yq` provisioning (Tasks 1–2), `sync-plugins.sh` + dry-run + tests (Task 4), install.sh wiring (Task 5), alias (Task 6), settings.json authority + allowlist (Task 6), docs (Task 7), Gemini path implemented-but-no-op (Task 4). All spec sections map to a task.
- **yq dialect risk:** the queries use mikefarah v4 syntax (`select(... != null)`, `.marketplaces[].claude`). Task 3 Step 2 validates the exact queries against the real manifest before `sync-plugins.sh` depends on them.
- **CLI arg forms:** `claude plugin install`/`enable` are called with the full `name@marketplace` form (matches `enabledPlugins` keys and `installed_plugins.json`).
- **Out of scope (YAGNI):** pruning unlisted plugins, version pinning, real Gemini sources.
