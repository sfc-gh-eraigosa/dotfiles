# agy startup parity with Claude Code — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans (inline, this
> objective is a single-branch build) to implement this plan task-by-task. Steps use checkbox
> (`- [ ]`) syntax for tracking; the live cursor is [`agy-parity/TODO.md`](./agy-parity/TODO.md).

- **Slug:** agy-parity
- **Date:** 2026-09-03
- **Status:** Approved
- **Relates to:** spec `../specs/agy-parity.md` · design `../designs/agy-parity.md` · issue #268 · PR #269

**Goal:** make `agy` start the way Claude Code starts on every host the dotfiles bootstrap.

**Architecture:** mirror the Claude provisioning shape file-for-file inside `ai/antigravity/`
and `install_antigravity_skills.sh`: a sentinel-driven wrapper, a first-run settings template,
a forced subset merged by the existing `apply-forced-settings.sh`, a jq-merged `hooks.json`,
and a generated local agy plugin carrying the repo slash commands and account memories.

**Tech Stack:** bash 3.2-compatible shell, `jq`, the repo's `ai/_test_helpers.sh` mini
framework, `make shell-test` / `lint-shell` / `lint-portability`.

**Spec:** `docs/mbo/specs/agy-parity.md`

## Global Constraints

- Shell portability standard: `docs/mbo/specs/shell-portability.md`; `make lint-portability`
  is enforcing (no `mapfile`, no `read -A`, GNU-only flags, bash-4-isms).
- No underscore-prefixed shell functions in sourced alias files (Claude's shell snapshot strips
  them; the agy driver guards the same).
- Copy-forward provisioning only: no new symlinks into the checkout; settings reference
  well-known `$HOME` paths.
- Stage by explicit file name; `.gitignore` is allowlist-based (`!ai/**`, `!opt/**`,
  `!docs/**` already opt these paths in; verify with `git status --short -- <path>`).
- Never run `install.sh` from this worktree. `install_antigravity_skills.sh` alone is
  copy-forward and may be run for live evidence.
- Every gate command's output is `tee`'d into `docs/mbo/plans/agy-parity/evidence/<unit>/`.

---

## 1. Summary & verdict

Builds design §4 units 1–7 as seven tasks on the single branch `worktree/agy_defaults`
(PR #269), TDD throughout. Verifications from design §7 were partly resolved before planning:
agy's binary contains no `{{args}}`/`$ARGUMENTS` substitution token, so the renderer rewrites
`$ARGUMENTS` to prose; `agy agents` discovery is out of scope; the `command(...)` prefix-match
probe is Task 7's live evidence item (deny rules are additive to the hook, so the build does
not wait on it).

## 2. File inventory

| Path | Purpose | Implements |
| :-- | :-- | :-- |
| `ai/antigravity/aliases.sh` | rewrite: `AGY_CONFIG_DIR`, `AGY_YOLO_FILE`, `agy_launch_flags`, `agy`, `agy-config`; `agy-yolo` removed | spec F1 |
| `ai/antigravity/aliases_test.sh` | extend with the ported Claude cases | F1 |
| `ai/antigravity/settings.json.template` | new first-run seed | F2 |
| `ai/antigravity/settings.forced.json` | add `permissions.allow/deny/ask` | F3 |
| `ai/antigravity/hooks.json.template` | unchanged content; now merged, not copied | F4 |
| `opt/scripts/system/install_antigravity_skills.sh` | seed from template, merge hooks.json, render + enable plugin | F2 F3 F4 F6 F7 |
| `opt/scripts/system/install_antigravity_skills_test.sh` | extend: seed, forced, hooks-preserve, plugin, config.json | F2 F3 F4 F7 |
| `ai/hooks/antigravity_adapter.sh` | sensitive-root `ask` for file tools | F5 |
| `ai/hooks/antigravity_adapter_test.sh` | new driver | F5 |
| `opt/scripts/system/render-agy-plugin.sh` | new renderer | F6 |
| `opt/scripts/system/render-agy-plugin_test.sh` | new driver | F6 |
| `install.sh` (herdr comment block, lines ~278-283) | comment correction only | F4 |
| `opt/scripts/system/install_herdr.sh` (header comment) | comment correction only | F4 |
| `ai/antigravity/AGENTS.md`, `ai/AGENTS.md`, `docs/machine-local-overrides.md` | docs | F8 |
| `ai/antigravity/scripts/sanity_check.sh` | assert seeded keys + plugin dir | F8 |
| `docs/mbo/plans/agy-parity/evidence/**` | captured gate output | all |
| `docs/mbo/index.md` | state transitions | — |

## 3. Interface contracts

**Shell (aliases.sh)**

```bash
AGY_CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/antigravity"
AGY_YOLO_FILE="$AGY_CONFIG_DIR/yolo.enabled"        # present => YOLO ON
agy_launch_flags <tty|other> [args...]              # sets global array AGY_LAUNCH_FLAGS
agy [args...]                                       # tmux anchor + command agy "${AGY_LAUNCH_FLAGS[@]}" "$@"
agy-config [status]                                 # prints "  yolo    ON|OFF"
agy-config yolo on|off                              # exit 0; unknown arg exit 2
agy-config doctor                                   # exit 0 (one binary) / 1 (duplicates) / prints "command agy -> <path>"
```

**Renderer**

```bash
render-agy-plugin.sh <repo-root> <plugin-dir>
# writes <plugin-dir>/plugin.json            {"name":"dotfiles","version":"1","description":...}
#        <plugin-dir>/commands/<name>.toml    description = "<frontmatter description>"\nprompt = """<body>"""
#        <plugin-dir>/rules/AGENTS.md         "# dotfiles account memories" + one "## <title>" section per scope:account memory
# body transforms: strip YAML frontmatter; `!`cmd`` -> "Run `cmd` first and use its output."; "$ARGUMENTS" -> "the arguments the user passed to this command"; `"""` -> `'''`
# exit 0; exit 1 with message on missing repo dirs. Removes stale <plugin-dir>/commands/*.toml first.
```

**Installer additions (install_antigravity_skills.sh)**

```bash
# settings seed:  [ ! -f "$AGY_SETTINGS_DEST" ] && cp "$AGY_SETTINGS_TEMPLATE" "$AGY_SETTINGS_DEST"
# hooks merge:    jq -s '.[0] * .[1]' existing rendered  (rendered wins for "guards"; others preserved)
# plugin:         render-agy-plugin.sh "$BASE_DIR" "$AGY_CONFIG_ROOT/plugins/dotfiles"
# config.json:    jq '.plugins.dotfiles = {enabled:true}' (created as {} when absent)
```

**Adapter (antigravity_adapter.sh)** — after guard evaluation and before `verdict allow`:
if `tool_name` ∈ {write_file, replace} and `file_path` (with `~` expanded) is equal to or
under one of `$HOME/.ssh $HOME/.aws $HOME/.gnupg $HOME/.config/gss $HOME/.kube $HOME/.docker`,
emit `{"decision":"ask","reason":"antigravity_adapter: <path> is under the sensitive path <root>"}`.

## 4. TDD build order

### Task 1: aliases.sh in the Claude shape (unit 1)

**Files:** Modify `ai/antigravity/aliases.sh`; Modify `ai/antigravity/aliases_test.sh`.
**Interfaces:** Produces the Shell contract above. Consumes nothing.

- [ ] **Step 1: Write the failing tests** — append to `ai/antigravity/aliases_test.sh`
  before `_test_report`:

```bash
# === agy-parity (F1): sentinel launch config, ported from ai/claude/aliases_test.sh ===
assert_grep_negative "agy-yolo removed (one canonical alias)" '^agy-yolo\(\)' "$ALIASES"
assert_in_subshell "agy-config function defined after sourcing" ". '$ALIASES' && type agy-config >/dev/null 2>&1"
assert_in_subshell "agy_launch_flags function defined after sourcing" ". '$ALIASES' && type agy_launch_flags >/dev/null 2>&1"
assert_in_subshell "AGY_YOLO_FILE set" ". '$ALIASES' && [ -n \"\$AGY_YOLO_FILE\" ]"
assert_in_subshell "yolo defaults OFF (no sentinel)" "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES' && [ ! -f \"\$AGY_YOLO_FILE\" ]"
assert_in_subshell "agy-config yolo on creates sentinel" "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES' && agy-config yolo on >/dev/null && [ -f \"\$AGY_YOLO_FILE\" ]"
assert_in_subshell "agy-config yolo off removes sentinel" "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES' && agy-config yolo on >/dev/null && agy-config yolo off >/dev/null && [ ! -f \"\$AGY_YOLO_FILE\" ]"
assert_in_subshell "agy-config status reports yolo OFF by default" "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES' && agy-config | grep -q 'yolo    OFF'"
assert_in_subshell "agy-config rejects unknown setting (exit 2)" "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES'; agy-config bogus >/dev/null 2>&1; [ \$? -eq 2 ]"
assert_in_subshell "doctor prints the resolved binary line" "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES'; agy-config doctor 2>&1 | grep -q 'command agy ->'"
assert_in_subshell "doctor: single binary => no warning, exit 0" "H=\$(mktemp -d); mkdir -p \"\$H/b1\"; : > \"\$H/b1/agy\"; chmod +x \"\$H/b1/agy\"; export PATH=\"\$H/b1:/usr/bin:/bin\"; . '$ALIASES'; out=\$(agy-config doctor 2>&1); rc=\$?; [ \$rc -eq 0 ] && printf '%s' \"\$out\" | grep -q \"\$H/b1/agy\" && ! printf '%s' \"\$out\" | grep -q WARNING"
assert_in_subshell "doctor: multiple binaries => WARNING + nonzero" "H=\$(mktemp -d); mkdir -p \"\$H/b1\" \"\$H/b2\"; : > \"\$H/b1/agy\"; : > \"\$H/b2/agy\"; chmod +x \"\$H/b1/agy\" \"\$H/b2/agy\"; export PATH=\"\$H/b1:\$H/b2:/usr/bin:/bin\"; . '$ALIASES'; agy-config doctor >/dev/null 2>&1; [ \$? -ne 0 ]"
assert_in_subshell "doctor: dedups repeated PATH entries" "H=\$(mktemp -d); mkdir -p \"\$H/b1\"; : > \"\$H/b1/agy\"; chmod +x \"\$H/b1/agy\"; export PATH=\"\$H/b1:\$H/b1:/usr/bin:/bin\"; . '$ALIASES'; agy-config doctor >/dev/null 2>&1; [ \$? -eq 0 ]"
assert_in_subshell "doctor: resolves the binary under zsh" "command -v zsh >/dev/null 2>&1 || exit 0; H=\$(mktemp -d); mkdir -p \"\$H/b1\"; : > \"\$H/b1/agy\"; chmod +x \"\$H/b1/agy\"; export H ALIASES='$ALIASES'; zsh -c 'export PATH=\"\$H/b1:/usr/bin:/bin\"; . \"\$ALIASES\"; agy-config doctor 2>&1 | grep -q \"\$H/b1/agy\"'"
assert_in_subshell "default: no flags injected" "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES'; agy_launch_flags tty 'fix bug'; [ \${#AGY_LAUNCH_FLAGS[@]} -eq 0 ]"
assert_in_subshell "yolo on: injects --dangerously-skip-permissions" "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES'; agy-config yolo on >/dev/null; agy_launch_flags other 'x'; printf '%s\n' \"\${AGY_LAUNCH_FLAGS[@]}\" | grep -q -- '--dangerously-skip-permissions'"
assert_in_subshell "yolo on: prompt not captured into flags" "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES'; agy-config yolo on >/dev/null; agy_launch_flags tty 'fix bug'; ! printf '%s\n' \"\${AGY_LAUNCH_FLAGS[@]}\" | grep -qx 'fix bug'"
assert_in_subshell "yolo on + print mode: flag still injected (print mode is not interactive-only)" "export XDG_CONFIG_HOME=\$(mktemp -d); . '$ALIASES'; agy-config yolo on >/dev/null; agy_launch_flags other -p 'x'; printf '%s\n' \"\${AGY_LAUNCH_FLAGS[@]}\" | grep -q -- '--dangerously-skip-permissions'"
assert_in_subshell "agy body calls agy_launch_flags" ". '$ALIASES' && declare -f agy | grep -q 'agy_launch_flags'"
```

  Also replace the existing `agy-yolo` assertions (`agy-yolo() defined`, `agy-yolo function
  defined after sourcing`, `agy-yolo body calls ...`) with the removal guard above.
- [ ] **Step 2: Run to verify failure** — `bash ai/antigravity/aliases_test.sh | tail -3` →
  expect `FAIL=` > 0 (agy-config undefined).
- [ ] **Step 3: Rewrite `ai/antigravity/aliases.sh`** with the Shell contract: header comment
  (commands, sentinel semantics, no remote equivalent), `AGY_CONFIG_DIR`/`AGY_YOLO_FILE`,
  `agy_launch_flags` (yolo only; print mode not special-cased since the flag applies to all
  modes), `agy()` (tmux anchor unchanged), `agy-config` (status/yolo/doctor, doctor's PATH scan
  copied from Claude's `tr ':' '\n'` loop with `agy` substituted), keep the three `sync-*`
  aliases. Delete `agy-yolo`.
- [ ] **Step 4: Run to verify pass** — `bash ai/antigravity/aliases_test.sh | tee
  docs/mbo/plans/agy-parity/evidence/u1-aliases/aliases_test.txt | tail -3` → `FAIL=0`.
- [ ] **Step 5: Gates** — `bash -n ai/antigravity/aliases.sh`; `shellcheck
  ai/antigravity/aliases.sh`; `make lint-portability`.
- [ ] **Step 6: Commit** — `feat(agy): agy-config launch config in the claude wrapper shape (#268)`.

**Done when:** aliases test `FAIL=0`, lint gates clean, evidence file committed.

### Task 2: settings template seeded on first run (unit 2)

**Files:** Create `ai/antigravity/settings.json.template`; Modify
`opt/scripts/system/install_antigravity_skills.sh` (settings block); Modify
`opt/scripts/system/install_antigravity_skills_test.sh`.
**Interfaces:** Produces `AGY_SETTINGS_TEMPLATE="$BASE_DIR/ai/antigravity/settings.json.template"`.

- [ ] **Step 1: Failing tests** — in the installer test after the fresh-host `run_install "$A"`:

```bash
S="$A/.gemini/antigravity-cli/settings.json"
assert_eq "$(jq -r '.toolPermission' "$S")" "request-review" "template: toolPermission seeded"
assert_eq "$(jq -r '.editorMode' "$S")" "vim" "template: editorMode vim"
assert_eq "$(jq -r '.allowNonWorkspaceAccess' "$S")" "true" "template: allowNonWorkspaceAccess true"
assert_eq "$(jq -r '.notifications' "$S")" "true" "template: notifications on"
assert_eq "$(jq -r '.enableTerminalSandbox' "$S")" "false" "template: sandbox off"
assert_in_subshell "template: allow list carries command(gss status)" "jq -e '.permissions.allow | index(\"command(gss status)\")' '$S' >/dev/null"
assert_in_subshell "template: no _comment key survives the merge" "! jq -e '._comment' '$S' >/dev/null 2>&1"
```

  and in the pre-existing host block (B): `assert_in_subshell "existing host: template NOT
  applied (no toolPermission)" "! jq -e '.toolPermission' '$B/.gemini/antigravity-cli/settings.json' >/dev/null 2>&1"`.
- [ ] **Step 2: Run** — `bash opt/scripts/system/install_antigravity_skills_test.sh | tail -3` → FAIL > 0.
- [ ] **Step 3: Implement** — write the template:

```json
{
  "_comment": "New-host baseline for the Antigravity CLI (agy >= 1.1). Host-owned after first install; ai/antigravity/settings.forced.json is re-applied on every run. Mirrors ai/claude/settings.json.template key-for-key where agy has a key: toolPermission~defaultMode, editorMode, colorScheme~theme, notifications, enableTerminalSandbox~sandbox.enabled. allowNonWorkspaceAccess is true because gss/herdr worktrees and ~/opt/scripts live outside any workspace root. NEVER add trustedWorkspaces here (host-local).",
  "toolPermission": "request-review",
  "editorMode": "vim",
  "colorScheme": "terminal",
  "notifications": true,
  "enableTerminalSandbox": false,
  "allowNonWorkspaceAccess": true,
  "permissions": {
    "allow": [
      "command(gss status)", "command(gss scan)", "command(gss version)",
      "command(gss push)", "command(gss pr)", "command(gss sync)",
      "command(tmux-mgr session)", "command(tmux-mgr window)", "command(tmux-mgr capture)",
      "command(tmux-mgr agent list)", "command(tmux-mgr desktop)", "command(tmux-mgr save)",
      "command(tmux-mgr restore)", "command(tmux-mgr version)", "command(wol version)",
      "command(sync-skills)", "command(sync-plugins)",
      "command(git status)", "command(git diff)", "command(git log)", "command(git branch)", "command(git rev-parse)",
      "command(make help)", "command(make unit-test)", "command(make bin)", "command(make integration-test)"
    ]
  }
}
```

  In the installer replace `echo '{}' > "$AGY_SETTINGS_DEST"` with a template copy (fall back
  to `{}` if the template is missing) and add `AGY_SETTINGS_TEMPLATE`. Since `_comment` must not
  reach the live file, strip it at seed time: `jq 'del(._comment)' template > dest`.
- [ ] **Step 4: Run** → `FAIL=0`; tee to `evidence/u2-settings-template/installer_test.txt`.
- [ ] **Step 5: Commit** — `feat(agy): seed agy settings.json from a tracked template on first run (#268)`.

**Done when:** installer test `FAIL=0`; `jq -e .` on the template.

### Task 3: forced deny/ask/allow policy (unit 3)

**Files:** Modify `ai/antigravity/settings.forced.json`; Modify the installer test.

- [ ] **Step 1: Failing tests** — fresh host A: deny contains `command(rm -rf /)`, ask contains
  `command(git push --force)` and `command(sudo)`. Host B: pre-seed
  `{"colorScheme":"light","permissions":{"allow":["command(mytool)"],"deny":["command(host-only)"]}}`;
  after install `allow` contains both `command(mytool)` and `command(gss push)`; `deny` does NOT
  contain `command(host-only)` and DOES contain `command(mkfs)`.
- [ ] **Step 2: Run** → FAIL > 0.
- [ ] **Step 3: Implement** — extend the forced file:

```json
{
  "_comment": "FORCED (immutable) agy settings subset, deep-merged over ~/.gemini/antigravity-cli/settings.json on every install (apply-forced-settings.sh): statusLine + permissions.deny/ask REPLACED from the repo, permissions.allow UNIONED, every other host key preserved. Targets use agy's action(target) form; command(...) is prefix-matched. Same policy as ai/claude/settings.forced.json.",
  "statusLine": { "type": "command", "command": "bash ~/.gemini/config/statusline-command.sh", "enabled": true },
  "permissions": {
    "allow": ["command(gss push)", "command(gss pr)", "command(gss sync)"],
    "deny": ["command(rm -rf /)", "command(rm -rf ~)", "command(rm -rf $HOME)", "command(dd if=)", "command(mkfs)", "command(fdisk)", "command(parted)", "command(sfdisk)", "command(mkswap)", "command(init 0)", "command(init 6)"],
    "ask": ["command(git push --force)", "command(git push -f)", "command(reboot)", "command(shutdown)", "command(poweroff)", "command(halt)", "command(sudo)"]
  }
}
```

- [ ] **Step 4: Run** → `FAIL=0`; tee to `evidence/u3-forced-policy/installer_test.txt`.
- [ ] **Step 5: Commit** — `feat(agy): enforce the repo deny/ask permission policy in agy settings (#268)`.

**Done when:** installer test `FAIL=0`.

### Task 4: hooks.json merge instead of overwrite (unit 4a)

**Files:** Modify installer (hooks.json block); Modify installer test; comment edits in
`install.sh` and `opt/scripts/system/install_herdr.sh`.

- [ ] **Step 1: Failing tests** — new host C: pre-seed
  `~/.gemini/config/hooks.json` with `{"herdr":{"PreToolUse":[{"matcher":"*","hooks":[{"command":"herdr-hook"}]}]},"guards":{"PreToolUse":[{"matcher":"stale","hooks":[{"command":"stale"}]}]}}`;
  after install: `jq -r 'keys|join(",")'` == `guards,herdr`; the `guards` matcher is the repo
  one (`run_command|...`), the herdr command is untouched. Host D: pre-seed `not json` → after
  install `hooks.json` valid with `guards`, and `hooks.json.invalid` exists.
- [ ] **Step 2: Run** → FAIL > 0.
- [ ] **Step 3: Implement** — render to a temp file, then:

```bash
if [ -f "$AGY_CONFIG_ROOT/hooks.json" ] && ! jq -e . "$AGY_CONFIG_ROOT/hooks.json" >/dev/null 2>&1; then
    mv "$AGY_CONFIG_ROOT/hooks.json" "$AGY_CONFIG_ROOT/hooks.json.invalid"
fi
if [ -f "$AGY_CONFIG_ROOT/hooks.json" ]; then
    jq -s '.[0] * .[1]' "$AGY_CONFIG_ROOT/hooks.json" "$rendered" > "$tmp" && cat "$tmp" > "$AGY_CONFIG_ROOT/hooks.json"
else
    cat "$rendered" > "$AGY_CONFIG_ROOT/hooks.json"
fi
```

  (`*` replaces the whole `guards` object because its value is an object of arrays; arrays are
  replaced by the right operand, which is what we want for the matcher list.) Without `jq`,
  fall back to overwrite with a warning. Fix the two comment blocks to say the ordering is no
  longer load-bearing.
- [ ] **Step 4: Run** → `FAIL=0`; tee to `evidence/u4-hooks-merge/installer_test.txt`.
- [ ] **Step 5: Commit** — `fix(agy): merge hooks.json so foreign named hooks (herdr) survive an install (#268)`.

**Done when:** installer test `FAIL=0`; `bash ai/claude/scripts/validate_hooks.sh <C>/.gemini/config/hooks.json` exits 0.

### Task 5: sensitive-root ask in the adapter (unit 4b)

**Files:** Modify `ai/hooks/antigravity_adapter.sh`; Create `ai/hooks/antigravity_adapter_test.sh`.

- [ ] **Step 1: Failing tests** — new driver:

```bash
#!/usr/bin/env bash
set -u
SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
. "$SELF_DIR/../_test_helpers.sh"
ADAPTER="$SELF_DIR/antigravity_adapter.sh"
command -v jq >/dev/null 2>&1 || { echo "SKIP: jq not installed"; exit 0; }
decide() { printf '%s' "$1" | bash "$ADAPTER" safety_guard.sh privacy_guard.sh | jq -r .decision; }
assert_eq "$(decide '{"toolCall":{"name":"run_command","args":{"CommandLine":"echo hi"}}}')" "allow" "run_command echo -> allow"
assert_eq "$(decide '{"toolCall":{"name":"run_command","args":{"CommandLine":"rm -rf /"}}}')" "deny" "run_command rm -rf / -> deny"
assert_eq "$(decide "{\"toolCall\":{\"name\":\"write_to_file\",\"args\":{\"TargetFile\":\"$HOME/.ssh/id_test\",\"CodeContent\":\"x\"}}}")" "ask" "write under ~/.ssh -> ask"
assert_eq "$(decide '{"toolCall":{"name":"write_to_file","args":{"TargetFile":"~/.aws/credentials","CodeContent":"x"}}}')" "ask" "tilde target under ~/.aws -> ask"
assert_eq "$(decide "{\"toolCall\":{\"name\":\"write_to_file\",\"args\":{\"TargetFile\":\"$HOME/proj/main.go\",\"CodeContent\":\"x\"}}}")" "allow" "write in workspace -> allow"
assert_eq "$(decide "{\"toolCall\":{\"name\":\"replace_file_content\",\"args\":{\"TargetFile\":\"$HOME/.gnupg/gpg.conf\",\"ReplacementChunks\":[]}}}")" "ask" "replace under ~/.gnupg -> ask"
assert_eq "$(printf '{}' | bash "$ADAPTER" missing_guard.sh | jq -r .decision)" "ask" "missing guard -> ask"
_test_report
```

- [ ] **Step 2: Run** → the two `ask` path cases FAIL (currently `allow`).
- [ ] **Step 3: Implement** — after the guard loop and the `ASK_REASON` check, add:

```bash
FILE_PATH="$(printf '%s' "$TRANSLATED" | jq -r 'select(.tool_name == "write_file" or .tool_name == "replace") | .tool_input.file_path // empty')"
if [ -n "$FILE_PATH" ]; then
    case "$FILE_PATH" in "~"|"~/"*) FILE_PATH="${HOME}${FILE_PATH#\~}" ;; esac
    FILE_PATH="${FILE_PATH%/}"
    for s in "$HOME/.ssh" "$HOME/.aws" "$HOME/.gnupg" "$HOME/.config/gss" "$HOME/.kube" "$HOME/.docker"; do
        if [ "$FILE_PATH" = "$s" ] || case "$FILE_PATH" in "$s"/*) true ;; *) false ;; esac; then
            verdict ask "antigravity_adapter: $FILE_PATH is under the sensitive path $s (credentials/security config) — confirm before writing"
        fi
    done
fi
```

- [ ] **Step 4: Run** → `FAIL=0`; tee to `evidence/u4-hooks-merge/adapter_test.txt`.
- [ ] **Step 5: Commit** — `feat(agy): ask before file tools touch credential paths (dir_added_guard parity) (#268)`.

**Done when:** adapter test `FAIL=0`; `shellcheck ai/hooks/antigravity_adapter.sh` clean.

### Task 6: local `dotfiles` plugin — commands + memory rules (units 5, 6)

**Files:** Create `opt/scripts/system/render-agy-plugin.sh`; Create
`opt/scripts/system/render-agy-plugin_test.sh`; Modify installer + installer test.

- [ ] **Step 1: Failing tests** — renderer driver:

```bash
#!/usr/bin/env bash
set -u
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
. "$REPO_ROOT/ai/_test_helpers.sh"
R="$SCRIPT_DIR/render-agy-plugin.sh"
OUT="$(mktemp -d)"; trap 'rm -rf "$OUT"' EXIT
assert_exit_code 0 "renderer runs against the repo" bash "$R" "$REPO_ROOT" "$OUT/dotfiles"
assert_file_exists "$OUT/dotfiles/plugin.json" "plugin.json written"
assert_eq "$(jq -r .name "$OUT/dotfiles/plugin.json")" "dotfiles" "plugin name"
n_md=$(ls "$REPO_ROOT"/ai/claude/commands/*.md | wc -l); n_toml=$(ls "$OUT"/dotfiles/commands/*.toml | wc -l)
assert_eq "$n_toml" "$n_md" "one TOML per command"
assert_grep "sync.toml has description" '^description = "' "$OUT/dotfiles/commands/sync.toml"
assert_grep "sync.toml has prompt" '^prompt = """' "$OUT/dotfiles/commands/sync.toml"
assert_grep_negative "no literal \$ARGUMENTS" '\$ARGUMENTS' "$OUT/dotfiles/commands/sync.toml"
assert_grep_negative "no Claude !-injection lines" '^!`' "$OUT/dotfiles/commands/sync.toml"
assert_grep "injection rewritten as an instruction" 'Run `git status --short --branch` first' "$OUT/dotfiles/commands/sync.toml"
assert_file_exists "$OUT/dotfiles/rules/AGENTS.md" "rules/AGENTS.md written"
assert_grep "rules carry an account memory" '^## gss land flow' "$OUT/dotfiles/rules/AGENTS.md"
assert_grep_negative "rules have no frontmatter fences" '^---$' "$OUT/dotfiles/rules/AGENTS.md"
if command -v python3 >/dev/null 2>&1 && python3 -c 'import tomllib' 2>/dev/null; then
  assert_in_subshell "every TOML parses" "for f in '$OUT'/dotfiles/commands/*.toml; do python3 -c 'import sys,tomllib; d=tomllib.load(open(sys.argv[1],\"rb\")); assert d[\"description\"] and d[\"prompt\"]' \"\$f\" || exit 1; done"
fi
touch "$OUT/dotfiles/commands/stale.toml"; bash "$R" "$REPO_ROOT" "$OUT/dotfiles" >/dev/null
assert_in_subshell "stale TOML removed on re-render" "[ ! -e '$OUT/dotfiles/commands/stale.toml' ]"
_test_report
```

  Installer test (fresh host A): `plugins/dotfiles/plugin.json` exists; `config.json`
  `.plugins.dotfiles.enabled == true`. Host B pre-seed `config.json`
  `{"plugins":{"superpowers":{"enabled":true}},"userSettings":{"x":1}}` → both entries survive.
- [ ] **Step 2: Run** → FAIL (renderer missing).
- [ ] **Step 3: Implement the renderer** (bash 3.2, awk for the body transform):

```bash
#!/usr/bin/env bash
# render-agy-plugin.sh <repo-root> <plugin-dir> — see docs/mbo/plans/agy-parity.md §3
set -u
REPO="${1:-}"; DEST="${2:-}"
[ -n "$REPO" ] && [ -n "$DEST" ] || { echo "usage: render-agy-plugin.sh <repo-root> <plugin-dir>" >&2; exit 1; }
CMDS="$REPO/ai/claude/commands"; MEM="$REPO/ai/claude/memory"
[ -d "$CMDS" ] || { echo "render-agy-plugin: missing $CMDS" >&2; exit 1; }
mkdir -p "$DEST/commands" "$DEST/rules"
rm -f "$DEST"/commands/*.toml
printf '{\n  "name": "dotfiles",\n  "version": "1",\n  "description": "dotfiles repo slash commands + account memories (rendered by install_antigravity_skills.sh; do not edit)"\n}\n' > "$DEST/plugin.json"
fm_desc() { awk 'NR==1 && $0!="---"{exit} /^---$/{c++; next} c==1 && /^description:/{sub(/^description:[[:space:]]*/,""); print; exit}' "$1"; }
body() { awk 'NR==1 && $0!="---"{p=1} /^---$/ && !p {c++; if(c==2){p=1}; next} p' "$1" \
  | sed -e 's/^!`\(.*\)`[[:space:]]*$/Run `\1` first and use its output./' \
        -e 's/\$ARGUMENTS/the arguments the user passed to this command/g' \
        -e "s/\"\"\"/'''/g"; }
for f in "$CMDS"/*.md; do
    [ -e "$f" ] || continue
    name="$(basename "$f" .md)"
    desc="$(fm_desc "$f" | sed 's/"/\\"/g')"
    { printf 'description = "%s"\n' "$desc"; printf 'prompt = """\n'; body "$f"; printf '"""\n'; } > "$DEST/commands/$name.toml"
done
{
    echo "# dotfiles account memories"; echo
    echo "Always-on notes from the dotfiles repo (rendered from ai/claude/memory; scope: account)."
    for f in "$MEM"/*.md; do
        [ -e "$f" ] || continue
        case "$(basename "$f")" in MEMORY.md) continue ;; esac
        grep -qE '^[[:space:]]*scope:[[:space:]]*account' "$f" || continue
        title="$(sed -n 's/^title:[[:space:]]*//p' "$f" | head -1)"; [ -n "$title" ] || title="$(basename "$f" .md)"
        echo; echo "## $title"; echo; body "$f"
    done
} > "$DEST/rules/AGENTS.md"
```

  Installer: call it after the aliases block; then
  `jq '.plugins = (.plugins // {}) | .plugins.dotfiles = {enabled: true}'` over `config.json`
  (seed `{}` if absent), temp-file then replace.
- [ ] **Step 4: Run both drivers** → `FAIL=0`; tee to `evidence/u5-u6-plugin/`.
- [ ] **Step 5: Commit** — `feat(agy): render repo slash commands + account memories as a local agy plugin (#268)`.

**Done when:** renderer + installer tests `FAIL=0`; lint gates clean.

### Task 7: docs, sanity check, live evidence (unit 7)

**Files:** `ai/antigravity/AGENTS.md`, `ai/AGENTS.md`, `docs/machine-local-overrides.md`,
`ai/antigravity/scripts/sanity_check.sh`, `docs/mbo/index.md`, evidence files.

- [ ] **Step 1: Sanity** — add to `sanity_check.sh` after step 8: assert `jq -e
  '.toolPermission' settings` when the file exists, and `[ -f ~/.gemini/config/plugins/dotfiles/plugin.json ]`.
- [ ] **Step 2: Docs** — `ai/antigravity/AGENTS.md`: layout table rows for the template, forced
  file, renderer, `agy-config`; version line → 1.1.25; `docs/machine-local-overrides.md`: an
  `agy-config yolo` note next to the Claude one; `ai/AGENTS.md`: plugin sentence.
- [ ] **Step 3: Gates** — `make shell-test`, `make lint-shell`, `make lint-portability`; tee to
  `evidence/u7-docs-gates/`.
- [ ] **Step 4: Live evidence** — from this checkout run
  `bash opt/scripts/system/install_antigravity_skills.sh`, then capture `agy-config status`,
  `jq 'del(.trustedWorkspaces)' ~/.gemini/antigravity-cli/settings.json`,
  `jq keys ~/.gemini/config/hooks.json`, `ls ~/.gemini/config/plugins/dotfiles/commands`, and a
  bounded `timeout 90 agy -p 'run: rm -rf /tmp/agy-parity-probe' --output-format json` probe
  into `evidence/live/`.
- [ ] **Step 5: index.md** → `in-review`; **Commit** — `docs(agy): agy-parity docs, sanity check, and evidence (#268)`.

**Done when:** all three make gates clean, live transcript committed, PR body refreshed.

## 5. Verification mapping

| Spec rule | Test case |
| :-- | :-- |
| F1 sentinel default OFF / on / off / status / unknown | aliases_test: "yolo defaults OFF", "yolo on creates sentinel", "yolo off removes", "status reports yolo OFF", "rejects unknown setting" |
| F1 flags injected / prompt preserved / print mode | "default: no flags injected", "yolo on: injects", "prompt not captured", "yolo on + print mode" |
| F1 doctor | four doctor cases incl. zsh |
| F1 no `agy-yolo`, no `_` helpers | source-level greps |
| F2 seed / no re-seed | installer test A template rows, B "template NOT applied" |
| F3 deny/ask replaced, allow unioned | installer test A + B permission rows |
| F4 preserve foreign hook, replace stale guards, invalid file | installer test C, D |
| F5 ask / allow / deny precedence / tilde | adapter test rows |
| F6 counts, fields, transforms, stale removal, TOML parse | renderer test |
| F7 enable + preserve | installer test A + B config.json rows |
| F8 sanity + docs | lint gates + live transcript |

## 6. Integration & rollout

Single branch, single PR (#269). Tests are discovered automatically by `make shell-test`
(paths under `ai/` and `opt/scripts/`). No `install.sh` change beyond comments. Existing hosts:
next `install.sh` run applies the forced subset, merges hooks, renders the plugin; `agy-yolo`
users see `agy-config` in the status output. `docs/mbo/index.md` moves
`planning → building → in-review` as tasks land.

## 7. Validation & evidence (show the work)

Evidence tree `docs/mbo/plans/agy-parity/evidence/{u1-aliases,u2-settings-template,
u3-forced-policy,u4-hooks-merge,u5-u6-plugin,u7-docs-gates,live}/`, each file with a dated
header, appended never rewritten, committed with its task. Coverage bar: every spec §5 row has
a named test (§5 above); the three make gates are the release bar. Adversarial scenarios covered:
invalid pre-existing `hooks.json`, host deny entries that must be replaced, tilde-prefixed
credential paths, a prompt token that looks like a flag, stale plugin TOMLs.

### 6.1 Build leaves / DAG

Not broken out: tasks 1–6 touch overlapping installer/test files and are cheap serially.
Order: T1 → T2 → T3 → T4 → T5 → T6 → T7 (T2–T4 share the installer test file, so they are
strictly sequential; T1 and T5 are independent of the rest).

> Produced via `superpowers:writing-plans` from the approved design. Execute with
> `superpowers:executing-plans`, TDD throughout; the trio in `./agy-parity/` is the live state.
