#!/usr/bin/env bash
# Test driver for ai/hooks/antigravity_adapter.sh — the agy hook dialect bridge.
#
# Drives the adapter end to end with real agy-shaped payloads on stdin and
# asserts the JSON verdict it prints: guard verdicts (allow / deny) pass
# through, a misconfigured guard degrades to "ask", and — agy-parity F5 — a
# file tool aimed under a credential/security-config root is answered "ask"
# (the agy analogue of Claude's DirectoryAdded dir_added_guard.sh; agy has no
# such event, so the check rides on PreToolUse).
#
# Run: bash ai/hooks/antigravity_adapter_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=../_test_helpers.sh
. "$SELF_DIR/../_test_helpers.sh"

ADAPTER="$SELF_DIR/antigravity_adapter.sh"

if ! command -v jq >/dev/null 2>&1; then
    echo "SKIP: jq not installed — antigravity_adapter tests skipped"
    exit 0
fi

# decide <payload-json> → the adapter's decision for that tool call.
decide() { printf '%s' "$1" | bash "$ADAPTER" safety_guard.sh privacy_guard.sh | jq -r .decision; }
# reason <payload-json> → the adapter's reason text ("" when none).
reason() { printf '%s' "$1" | bash "$ADAPTER" safety_guard.sh privacy_guard.sh | jq -r '.reason // ""'; }

# === Guard verdict pass-through ===
assert_eq "$(decide '{"toolCall":{"name":"run_command","args":{"CommandLine":"echo hi"}}}')" \
    "allow" "run_command echo -> allow"
assert_eq "$(decide '{"toolCall":{"name":"run_command","args":{"CommandLine":"rm -rf /"}}}')" \
    "deny" "run_command rm -rf / -> deny (safety_guard wins)"
assert_eq "$(printf '{"toolCall":{"name":"run_command","args":{"CommandLine":"ls"}}}' | bash "$ADAPTER" missing_guard.sh | jq -r .decision)" \
    "ask" "missing guard -> ask (never silent allow)"

# === F5: sensitive-root ask for file tools ===
assert_eq "$(decide "{\"toolCall\":{\"name\":\"write_to_file\",\"args\":{\"TargetFile\":\"$HOME/.ssh/id_test\",\"CodeContent\":\"x\"}}}")" \
    "ask" "write_to_file under ~/.ssh -> ask"
assert_eq "$(decide '{"toolCall":{"name":"write_to_file","args":{"TargetFile":"~/.aws/credentials","CodeContent":"x"}}}')" \
    "ask" "tilde-prefixed target under ~/.aws -> ask"
assert_eq "$(decide "{\"toolCall\":{\"name\":\"replace_file_content\",\"args\":{\"TargetFile\":\"$HOME/.gnupg/gpg.conf\",\"ReplacementChunks\":[]}}}")" \
    "ask" "replace_file_content under ~/.gnupg -> ask"
assert_eq "$(decide "{\"toolCall\":{\"name\":\"edit_file\",\"args\":{\"TargetFile\":\"$HOME/.config/gss/approval.token\",\"Instruction\":\"x\"}}}")" \
    "ask" "edit_file under ~/.config/gss -> ask"
assert_in_subshell "ask reason names the sensitive root" \
    "printf '%s' \"\$(printf '{\"toolCall\":{\"name\":\"write_to_file\",\"args\":{\"TargetFile\":\"$HOME/.ssh/x\",\"CodeContent\":\"x\"}}}' | bash '$ADAPTER' safety_guard.sh privacy_guard.sh | jq -r .reason)\" | grep -qF '$HOME/.ssh'"
assert_eq "$(decide "{\"toolCall\":{\"name\":\"write_to_file\",\"args\":{\"TargetFile\":\"$HOME/proj/main.go\",\"CodeContent\":\"package main\"}}}")" \
    "allow" "write_to_file in a workspace path -> allow"
assert_eq "$(decide "{\"toolCall\":{\"name\":\"write_to_file\",\"args\":{\"TargetFile\":\"$HOME/.sshd_notes/x\",\"CodeContent\":\"x\"}}}")" \
    "allow" "sibling path sharing a prefix (~/.sshd_notes) is NOT under ~/.ssh -> allow"
assert_eq "$(decide "{\"toolCall\":{\"name\":\"run_command\",\"args\":{\"CommandLine\":\"cat $HOME/.ssh/config\"}}}")" \
    "allow" "run_command is unaffected by the file-path check"

_test_report
