#!/bin/bash
# Test driver for opt/scripts/system/claude_install.sh
#
# Focuses on cleanup_conflicting_installs() — the guarded removal of the
# official native-installer copy of claude so only the canonical (npm/brew)
# binary remains. The script is sourceable (main() runs only when executed
# directly), so we load the functions and exercise the decision logic against a
# sandboxed $HOME. Canonical resolution is overridden via $CLAUDE_CANONICAL_BIN.
#
# Run: bash opt/scripts/system/claude_install_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${SELF_DIR}/../../../ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/claude_install.sh"

# === Source-level checks ===
assert_grep "script is sourceable (BASH_SOURCE guard)" \
    'BASH_SOURCE\[0\]' "$SCRIPT"
assert_grep "defines cleanup_conflicting_installs" \
    '^cleanup_conflicting_installs\(\)' "$SCRIPT"
assert_grep "cleanup never touches ~/.claude (documented)" \
    'never touches \~?/?\.claude' "$SCRIPT"

# Sourcing must NOT trigger an install (main guarded behind BASH_SOURCE==$0).
assert_in_subshell "sourcing loads functions without running install" \
    ". '$SCRIPT' && type cleanup_conflicting_installs >/dev/null 2>&1 && type resolve_canonical_claude >/dev/null 2>&1"

# === cleanup_conflicting_installs behavior (sandboxed HOME) ===
# Helper: a sandbox with a native-installer layout. NATIVE link -> ~/.local/share/claude/...
mk_native='H=$(mktemp -d); export HOME="$H";
  mkdir -p "$H/.local/bin" "$H/.local/share/claude/versions" "$H/.local/state/claude" "$H/realbin";
  : > "$H/.local/share/claude/versions/9.9.9";
  ln -s "$H/.local/share/claude/versions/9.9.9" "$H/.local/bin/claude";
  : > "$H/realbin/claude"; chmod +x "$H/realbin/claude";'

# 1) Native install present + canonical confirmed => native link/data/state removed.
assert_in_subshell "removes native install when canonical is confirmed" \
    "$mk_native export CLAUDE_CANONICAL_BIN=\"\$H/realbin/claude\"; . '$SCRIPT'; cleanup_conflicting_installs >/dev/null 2>&1; [ ! -e \"\$H/.local/bin/claude\" ] && [ ! -d \"\$H/.local/share/claude\" ] && [ ! -d \"\$H/.local/state/claude\" ]"

# 2) Native install present but canonical NOT confirmed (path not executable) => kept.
assert_in_subshell "keeps native install when no canonical is confirmed" \
    "$mk_native export CLAUDE_CANONICAL_BIN=\"\$H/nope/claude\"; . '$SCRIPT'; cleanup_conflicting_installs >/dev/null 2>&1; [ -L \"\$H/.local/bin/claude\" ] && [ -d \"\$H/.local/share/claude\" ]"

# 3) Canonical resolves UNDER ~/.local => skip (don't delete the thing we'd keep).
assert_in_subshell "skips when canonical also resolves under ~/.local" \
    "$mk_native mkdir -p \"\$H/.local/altbin\"; : > \"\$H/.local/altbin/claude\"; chmod +x \"\$H/.local/altbin/claude\"; export CLAUDE_CANONICAL_BIN=\"\$H/.local/altbin/claude\"; . '$SCRIPT'; cleanup_conflicting_installs >/dev/null 2>&1; [ -L \"\$H/.local/bin/claude\" ]"

# 4) Convenience symlink to the canonical binary (NOT into ~/.local/share/claude) => kept.
assert_in_subshell "keeps a convenience symlink that points at the canonical binary" \
    "H=\$(mktemp -d); export HOME=\"\$H\"; mkdir -p \"\$H/.local/bin\" \"\$H/realbin\"; : > \"\$H/realbin/claude\"; chmod +x \"\$H/realbin/claude\"; ln -s \"\$H/realbin/claude\" \"\$H/.local/bin/claude\"; export CLAUDE_CANONICAL_BIN=\"\$H/realbin/claude\"; . '$SCRIPT'; cleanup_conflicting_installs >/dev/null 2>&1; [ -L \"\$H/.local/bin/claude\" ]"

# 5) Idempotent: no native link present => no-op, exit 0.
assert_in_subshell "no-op when there is no native install" \
    "H=\$(mktemp -d); export HOME=\"\$H\"; mkdir -p \"\$H/.local/bin\"; export CLAUDE_CANONICAL_BIN=\"\$H/realbin/claude\"; . '$SCRIPT'; cleanup_conflicting_installs >/dev/null 2>&1"

# 5b) Link into an OLDER nvm node's npm global => re-pointed at the canonical binary.
mk_nvm='H=$(mktemp -d); export HOME="$H";
  mkdir -p "$H/.local/bin" "$H/.nvm/versions/node/v1.0.0/lib/node_modules/@anthropic-ai/claude-code/bin" "$H/.nvm/versions/node/v2.0.0/bin";
  old="$H/.nvm/versions/node/v1.0.0/lib/node_modules/@anthropic-ai/claude-code/bin/claude.exe"; : > "$old"; chmod +x "$old";
  new="$H/.nvm/versions/node/v2.0.0/bin/claude"; : > "$new"; chmod +x "$new";'
assert_in_subshell "re-points a link pinned to an older nvm node at the canonical binary" \
    "$mk_nvm ln -s \"\$old\" \"\$H/.local/bin/claude\"; export CLAUDE_CANONICAL_BIN=\"\$new\"; . '$SCRIPT'; cleanup_conflicting_installs >/dev/null 2>&1; [ \"\$(readlink \"\$H/.local/bin/claude\")\" = \"\$new\" ]"

# 5c) Link already inside the current nvm node's prefix => untouched.
assert_in_subshell "keeps a link already inside the current nvm prefix" \
    "$mk_nvm cur=\"\$H/.nvm/versions/node/v2.0.0/lib/node_modules/x/claude.exe\"; mkdir -p \"\${cur%/*}\"; : > \"\$cur\"; ln -s \"\$cur\" \"\$H/.local/bin/claude\"; export CLAUDE_CANONICAL_BIN=\"\$new\"; . '$SCRIPT'; cleanup_conflicting_installs >/dev/null 2>&1; [ \"\$(readlink \"\$H/.local/bin/claude\")\" = \"\$cur\" ]"

# 5d) Stale nvm link but no canonical confirmed => untouched.
assert_in_subshell "keeps a stale nvm link when no canonical is confirmed" \
    "$mk_nvm ln -s \"\$old\" \"\$H/.local/bin/claude\"; export CLAUDE_CANONICAL_BIN=\"\$H/nope/claude\"; . '$SCRIPT'; cleanup_conflicting_installs >/dev/null 2>&1; [ \"\$(readlink \"\$H/.local/bin/claude\")\" = \"\$old\" ]"

# 5e) Downgrade guard: link into a NEWER node than the canonical => untouched.
assert_in_subshell "never re-points a link to an older node than its target" \
    "$mk_nvm ln -s \"\$new\" \"\$H/.local/bin/claude\"; export CLAUDE_CANONICAL_BIN=\"\$old\"; . '$SCRIPT'; cleanup_conflicting_installs >/dev/null 2>&1; [ \"\$(readlink \"\$H/.local/bin/claude\")\" = \"\$new\" ]"

# 5f) A custom NVM_DIR (not ~/.nvm) is recognized too.
assert_in_subshell "re-points a stale link under a custom NVM_DIR" \
    "H=\$(mktemp -d); export HOME=\"\$H\"; N=\"\$H/.config/nvm\"; mkdir -p \"\$H/.local/bin\" \"\$N/versions/node/v1.0.0/bin\" \"\$N/versions/node/v2.0.0/bin\"; o=\"\$N/versions/node/v1.0.0/bin/claude\"; n=\"\$N/versions/node/v2.0.0/bin/claude\"; : > \"\$o\"; : > \"\$n\"; chmod +x \"\$o\" \"\$n\"; ln -s \"\$o\" \"\$H/.local/bin/claude\"; export CLAUDE_CANONICAL_BIN=\"\$n\"; . '$SCRIPT'; cleanup_conflicting_installs >/dev/null 2>&1; [ \"\$(readlink \"\$H/.local/bin/claude\")\" = \"\$n\" ]"

# 5g) A RELATIVE link into the current node is not treated as stale.
assert_in_subshell "keeps a relative link into the current nvm node" \
    "$mk_nvm ln -s ../../.nvm/versions/node/v2.0.0/bin/claude \"\$H/.local/bin/claude\"; export CLAUDE_CANONICAL_BIN=\"\$new\"; . '$SCRIPT'; cleanup_conflicting_installs >/dev/null 2>&1; [ \"\$(readlink \"\$H/.local/bin/claude\")\" = ../../.nvm/versions/node/v2.0.0/bin/claude ]"

# === Truncated-install detection + repair ===
# claude_binary_healthy: empty file / symlink to empty / no version => unhealthy.
assert_in_subshell "claude_binary_healthy rejects an empty binary" \
    "H=\$(mktemp -d); : > \"\$H/claude\"; chmod +x \"\$H/claude\"; . '$SCRIPT'; ! claude_binary_healthy \"\$H/claude\""
assert_in_subshell "claude_binary_healthy rejects a symlink to an empty binary" \
    "H=\$(mktemp -d); : > \"\$H/claude.exe\"; chmod +x \"\$H/claude.exe\"; ln -s \"\$H/claude.exe\" \"\$H/claude\"; . '$SCRIPT'; ! claude_binary_healthy \"\$H/claude\""
assert_in_subshell "claude_binary_healthy rejects a binary with no version output" \
    "H=\$(mktemp -d); printf '#!/bin/sh\nexit 0\n' > \"\$H/claude\"; chmod +x \"\$H/claude\"; . '$SCRIPT'; ! claude_binary_healthy \"\$H/claude\""
assert_in_subshell "claude_binary_healthy accepts a binary that prints a version" \
    "H=\$(mktemp -d); printf '#!/bin/sh\necho 2.1.332 Claude Code\n' > \"\$H/claude\"; chmod +x \"\$H/claude\"; . '$SCRIPT'; claude_binary_healthy \"\$H/claude\""

# install_via_npm with a mocked npm: package "already installed" (npm ls ok,
# npm update a no-op) but the binary is 0 bytes => forced reinstall repairs it.
mk_npm='H=$(mktemp -d); export HOME="$H"; P="$H/prefix"; mkdir -p "$P/bin"; export P LOG="$H/npm.log";
  : > "$P/bin/claude"; chmod +x "$P/bin/claude";
  npm_allow_scripts() { :; };'
npm_fixes='npm() { echo "$*" >> "$LOG"; case "$*" in "config get prefix") echo "$P" ;; *--force*) printf "#!/bin/sh\necho 9.9.9 Claude Code\n" > "$P/bin/claude" ;; esac; return 0; };'
npm_noop='npm() { echo "$*" >> "$LOG"; case "$*" in "config get prefix") echo "$P" ;; esac; return 0; };'
assert_in_subshell "install_via_npm force-reinstalls a truncated (0-byte) binary" \
    "$mk_npm . '$SCRIPT'; $npm_fixes install_via_npm >/dev/null 2>&1 && grep -q -- 'install -g --force' \"\$LOG\" && claude_binary_healthy \"\$P/bin/claude\""
assert_in_subshell "install_via_npm fails when the forced reinstall does not repair it" \
    "$mk_npm . '$SCRIPT'; $npm_noop ! install_via_npm >/dev/null 2>&1"
assert_in_subshell "install_via_npm leaves a healthy binary alone (no --force)" \
    "$mk_npm printf '#!/bin/sh\necho 9.9.9\n' > \"\$P/bin/claude\"; . '$SCRIPT'; $npm_fixes install_via_npm >/dev/null 2>&1 && ! grep -q -- '--force' \"\$LOG\""

# 5h) version_lt compares numerically per field (10 > 9); equal is not lower.
assert_in_subshell "version_lt compares dotted versions numerically" \
    ". '$SCRIPT'; version_lt 24.9.0 24.10.0 && ! version_lt 24.10.0 24.9.0 && ! version_lt 24.18.0 24.18.0"

# 6) resolve_canonical_claude honors the override.
assert_in_subshell "resolve_canonical_claude honors CLAUDE_CANONICAL_BIN" \
    "export CLAUDE_CANONICAL_BIN=/opt/x/claude; . '$SCRIPT'; [ \"\$(resolve_canonical_claude)\" = /opt/x/claude ]"

_test_report
