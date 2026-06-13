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

# 6) resolve_canonical_claude honors the override.
assert_in_subshell "resolve_canonical_claude honors CLAUDE_CANONICAL_BIN" \
    "export CLAUDE_CANONICAL_BIN=/opt/x/claude; . '$SCRIPT'; [ \"\$(resolve_canonical_claude)\" = /opt/x/claude ]"

_test_report
