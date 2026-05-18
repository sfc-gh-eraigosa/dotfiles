#!/bin/bash
# safety_guard.sh — Claude Code PreToolUse hook
#
# Enforces regex-based safety rules that the prefix-only permission DSL in
# settings.json cannot express. Mirrors the rule set from
# ai/gemini/policies/safety.toml.
#
# Contract (Claude Code PreToolUse):
#   - stdin: JSON with .tool_name and .tool_input.command
#   - exit 0: allow the call (stdout optional, may surface to user)
#   - exit 2: block the call (stderr is fed back to Claude as context)
#   - other non-zero: non-blocking error
#
# Dependencies: jq, bash 3.2+

set -u

# Read the hook payload
PAYLOAD="$(cat)"

# We only inspect Bash tool calls; everything else passes
TOOL_NAME="$(printf '%s' "$PAYLOAD" | jq -r '.tool_name // empty')"
if [ "$TOOL_NAME" != "Bash" ]; then
    exit 0
fi

CMD="$(printf '%s' "$PAYLOAD" | jq -r '.tool_input.command // empty')"
if [ -z "$CMD" ]; then
    exit 0
fi

deny() {
    # stderr is what Claude sees; stdout is what the user sees
    printf 'BLOCKED by safety_guard: %s\n' "$1" >&2
    printf 'Command: %s\n' "$CMD" >&2
    exit 2
}

# --- 1. Recursive wildcard deletion: rm -rf *, rm -rf ./*, rm -rf  * ---
if [[ "$CMD" =~ rm[[:space:]]+.*-[rRf]+.*\* ]]; then
    deny "Recursive wildcard deletion (rm -rf *) is prohibited by safety policy."
fi

# --- 2. Recursive deletion of current/parent dir: rm -rf . / rm -rf .. ---
if [[ "$CMD" =~ rm[[:space:]]+.*-[rRf]+[[:space:]]+\.{1,2}([[:space:]]|/|$) ]]; then
    deny "Recursive deletion of current/parent directory is prohibited."
fi

# --- 3. Deletion of root or critical system dirs ---
if [[ "$CMD" =~ rm[[:space:]]+.*-[rRf]+[[:space:]]+/(etc|usr|bin|sbin|var|boot|root|lib|home)([[:space:]]|/|$) ]] \
   || [[ "$CMD" =~ rm[[:space:]]+.*-[rRf]+[[:space:]]+/[[:space:]]*$ ]]; then
    deny "Deletion of root or protected system directories is prohibited."
fi

# --- 4. Disk management / raw writes ---
# mkfs has variants: mkfs.ext4, mkfs.xfs, etc. Match optional dotted suffix.
for tool in dd fdisk parted sfdisk mkswap; do
    if [[ "$CMD" =~ (^|[[:space:];|\&])$tool([[:space:]]|$) ]]; then
        # Allow harmless `dd --help` / `dd --version` style invocations
        if [[ "$tool" == "dd" ]] && [[ "$CMD" =~ dd[[:space:]]+--(help|version) ]]; then
            continue
        fi
        deny "Disk management command '$tool' is restricted."
    fi
done
if [[ "$CMD" =~ (^|[[:space:];|\&])mkfs(\.[a-zA-Z0-9]+)?([[:space:]]|$) ]]; then
    deny "Disk management command 'mkfs' is restricted."
fi

# --- 5. Recursive chmod/chown on root ---
# Match: chmod -R <anything> / (with / as the last meaningful arg)
if [[ "$CMD" =~ (chmod|chown)[[:space:]]+(-[a-zA-Z]*R[a-zA-Z]*|--recursive)([[:space:]]+[^/[:space:]][^[:space:]]*)*[[:space:]]+/([[:space:]]|$) ]]; then
    deny "Recursive permission/ownership change on the root filesystem is prohibited."
fi

# --- 6. Fork bomb ---
if [[ "$CMD" =~ :\(\)\{[[:space:]]*:\|:\&[[:space:]]*\}\;: ]]; then
    deny "Fork bomb pattern detected."
fi

# --- 7. Piping web content directly to a shell ---
if [[ "$CMD" =~ (curl|wget|fetch)[[:space:]]+[^|]*\|[[:space:]]*(sudo[[:space:]]+)?(sh|bash|zsh|ksh|dash) ]]; then
    deny "Piping unverified web content directly into a shell is prohibited. Download to a file, inspect, then run."
fi

# --- 8. Redirect to block devices ---
if [[ "$CMD" =~ \>[[:space:]]*/dev/(sd[a-z]|nvme[0-9]+n[0-9]+|mapper/|hd[a-z]|vd[a-z]) ]]; then
    deny "Direct redirection to block devices is prohibited."
fi

# --- 9. gss approval token enforcement ---
# The git-safe-sync skill mandates an approval token generated immediately
# before `gss push`/`pr`/`sync`. The binary itself also checks, but we enforce
# at the hook layer so the user can never be surprised by an autonomous push.
if [[ "$CMD" =~ (^|[[:space:];|&])gss[[:space:]]+(push|pr|sync)([[:space:]]|$) ]]; then
    TOKEN_FILE="${HOME}/.config/gss/approval.token"
    if [ ! -f "$TOKEN_FILE" ]; then
        deny "gss push/pr/sync requires an approval token. Per the git-safe-sync skill, you must first confirm with the user, then generate the token: mkdir -p ~/.config/gss && git rev-parse HEAD > ~/.config/gss/approval.token"
    fi
    # Token must be fresh — current HEAD must match the token contents.
    # This prevents reusing a stale token from a previous session.
    if command -v git &> /dev/null; then
        CURRENT_HEAD="$(git rev-parse HEAD 2>/dev/null || echo)"
        TOKEN_HEAD="$(cat "$TOKEN_FILE" 2>/dev/null || echo)"
        if [ -n "$CURRENT_HEAD" ] && [ "$CURRENT_HEAD" != "$TOKEN_HEAD" ]; then
            deny "gss approval token is stale (does not match HEAD $CURRENT_HEAD). Re-confirm with the user and regenerate."
        fi
    fi
fi

# All checks passed
exit 0
