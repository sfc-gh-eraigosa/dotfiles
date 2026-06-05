#!/bin/bash
# safety_guard.sh — AI Agent PreToolUse / BeforeTool hook
#
# Enforces regex-based safety rules that the prefix-only permission DSL in
# assistant settings cannot express. Mirrors the rule set from
# ai/gemini/policies/safety.toml.
#
# Contract:
#   - stdin: JSON {tool_name, tool_input}
#   - exit 0: allow (Gemini: outputs JSON; Claude: no output)
#   - exit 2: block (Gemini: use stderr; Claude: use stderr)
#   - other non-zero: non-blocking error
#
# Dependencies: jq, bash 3.2+

set -u

# Read the hook payload
PAYLOAD="$(cat)"

# We only inspect shell command tools; everything else passes
TOOL_NAME="$(printf '%s' "$PAYLOAD" | jq -r '.tool_name // empty')"
if [ "$TOOL_NAME" != "Bash" ] && [ "$TOOL_NAME" != "run_shell_command" ]; then
    exit 0
fi

CMD="$(printf '%s' "$PAYLOAD" | jq -r '.tool_input.command // empty')"
if [ -z "$CMD" ]; then
    [ "$TOOL_NAME" = "run_shell_command" ] && echo '{"decision": "allow"}'
    exit 0
fi

# Strip heredoc bodies before matching. Heredocs are how multi-line text
# (commit messages, doc strings, README content) gets passed to commands —
# we don't want literal patterns inside those bodies to trip the rules.
# The heredoc START line is preserved (e.g. `cat <<'EOF'` is still visible),
# but the body and terminator are dropped.
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [ -r "$SCRIPT_DIR/strip_heredocs.awk" ] && command -v awk > /dev/null; then
    CMD_SCRUBBED="$(printf '%s\n' "$CMD" | awk -f "$SCRIPT_DIR/strip_heredocs.awk")"
else
    CMD_SCRUBBED="$CMD"
fi

deny() {
    # stderr is what Claude sees; stdout is what the user sees.
    # Show the ORIGINAL command (not the scrubbed one) so the user sees what
    # they actually typed.
    printf 'BLOCKED by safety_guard: %s\n' "$1" >&2
    printf 'Command: %s\n' "$CMD" >&2
    exit 2
}

# Pattern note: bash regex `.*` matches newlines and command separators (; | &),
# which causes false positives across multi-line scripts (e.g. an unrelated `*`
# many lines after a safe `rm -f one_file`). Use SAFE_CHARS to scope each rule
# to one shell-command segment.
SAFE_CHARS='[^[:cntrl:];|&]'

# --- 1. Wildcard deletion: rm -rf *, rm -rf ./*, rm -f *, etc. ---
# Note: any rm form with -f and a wildcard is treated as dangerous, not just -rf.
# `rm -f *` deletes every file in cwd, which is nearly as catastrophic in most
# repos. If you legitimately need this, name the files explicitly.
if [[ "$CMD_SCRUBBED" =~ rm[[:space:]]+${SAFE_CHARS}*-[rRf]+${SAFE_CHARS}*\* ]]; then
    deny "Wildcard deletion with rm -[rRf] is prohibited (matches both rm -rf * and rm -f *). Enumerate the files instead."
fi

# --- 2. Recursive deletion of current/parent dir: rm -rf . / rm -rf .. ---
if [[ "$CMD_SCRUBBED" =~ rm[[:space:]]+${SAFE_CHARS}*-[rRf]+[[:space:]]+\.{1,2}([[:space:]]|/|$) ]]; then
    deny "Recursive deletion of current/parent directory is prohibited."
fi

# --- 3. Deletion of root or critical system dirs ---
if [[ "$CMD_SCRUBBED" =~ rm[[:space:]]+${SAFE_CHARS}*-[rRf]+[[:space:]]+/(etc|usr|bin|sbin|var|boot|root|lib|home)([[:space:]]|/|$) ]] \
   || [[ "$CMD_SCRUBBED" =~ rm[[:space:]]+${SAFE_CHARS}*-[rRf]+[[:space:]]+/[[:space:]]*$ ]]; then
    deny "Deletion of root or protected system directories is prohibited."
fi

# --- 4. Disk management / raw writes ---
# Hard-block tools that have no legitimate read-only invocation we care about.
for tool in fdisk sfdisk mkswap; do
    if [[ "$CMD_SCRUBBED" =~ (^|[[:space:];|\&])$tool([[:space:]]|$) ]]; then
        deny "Disk management command '$tool' is restricted."
    fi
done
# mkfs has variants: mkfs.ext4, mkfs.xfs, etc. Match optional dotted suffix.
if [[ "$CMD_SCRUBBED" =~ (^|[[:space:];|\&])mkfs(\.[a-zA-Z0-9]+)?([[:space:]]|$) ]]; then
    deny "Disk management command 'mkfs' is restricted."
fi
# dd: read-only and file-to-file forms are legitimate (backups, image inspection,
# benchmarks against /dev/null). Block only when the output target is a real
# block device. This includes Linux (sd*, nvme*, hd*, vd*, mapper/) and macOS
# (disk*) device names.
if [[ "$CMD_SCRUBBED" =~ (^|[[:space:];|\&])dd([[:space:]]|$) ]]; then
    if [[ "$CMD_SCRUBBED" =~ dd[[:space:]]+${SAFE_CHARS}*of=/dev/(sd[a-z]|nvme[0-9]+n[0-9]*|hd[a-z]|vd[a-z]|mapper/|disk[0-9]+) ]]; then
        deny "dd targeting a block device (of=/dev/<disk>) is prohibited."
    fi
fi
# parted: --list / -l only inspect partitions and are safe. Block when targeting
# a specific device or using a mutating subcommand.
if [[ "$CMD_SCRUBBED" =~ (^|[[:space:];|\&])parted([[:space:]]|$) ]]; then
    if [[ "$CMD_SCRUBBED" =~ parted[[:space:]]+(-l|--list)([[:space:]]|$) ]]; then
        :  # read-only listing — allow
    elif [[ "$CMD_SCRUBBED" =~ parted[[:space:]]+${SAFE_CHARS}*/dev/ ]]; then
        deny "parted targeting a device is restricted."
    fi
fi

# --- 5. Recursive chmod/chown on root ---
# Match: chmod -R <anything> / (with / as the last meaningful arg)
if [[ "$CMD_SCRUBBED" =~ (chmod|chown)[[:space:]]+(-[a-zA-Z]*R[a-zA-Z]*|--recursive)([[:space:]]+[^/[:space:]][^[:space:]]*)*[[:space:]]+/([[:space:]]|$) ]]; then
    deny "Recursive permission/ownership change on the root filesystem is prohibited."
fi

# --- 6. Fork bomb ---
if [[ "$CMD_SCRUBBED" =~ :\(\)\{[[:space:]]*:\|:\&[[:space:]]*\}\;: ]]; then
    deny "Fork bomb pattern detected."
fi

# --- 7. Piping web content directly to a shell ---
if [[ "$CMD_SCRUBBED" =~ (curl|wget|fetch)[[:space:]]+[^|]*\|[[:space:]]*(sudo[[:space:]]+)?(sh|bash|zsh|ksh|dash) ]]; then
    deny "Piping unverified web content directly into a shell is prohibited. Download to a file, inspect, then run."
fi

# --- 8. Redirect to block devices ---
if [[ "$CMD_SCRUBBED" =~ \>[[:space:]]*/dev/(sd[a-z]|nvme[0-9]+n[0-9]+|mapper/|hd[a-z]|vd[a-z]) ]]; then
    deny "Direct redirection to block devices is prohibited."
fi

# Shared gss context for the rules below: resolve the effective working dir
# (target of a leading `cd <path> &&`, else the hook's CWD) and whether it is
# inside a registered feature worker worktree (under the gss worktree root).
GSS_WT_ROOT="${GSS_WORKTREE_ROOT:-$HOME/.config/gss/worktrees}"
GSS_EFFECTIVE_DIR="$PWD"
if [[ "$CMD_SCRUBBED" =~ (^|[[:space:];|&])cd[[:space:]]+([^[:space:];|&]+)[[:space:]]*(&&|;) ]]; then
    GSS_EFFECTIVE_DIR="${BASH_REMATCH[2]/#\~/$HOME}"
fi
gss_in_worker() {
    case "$GSS_EFFECTIVE_DIR" in
        "$GSS_WT_ROOT" | "$GSS_WT_ROOT"/*) return 0 ;;
        *) return 1 ;;
    esac
}

# --- 9. gss --force-autonomous inside a worker worktree (resolution #22) ---
# Classic `gss push/pr --force-autonomous` is valid on a regular checkout but
# is the WRONG MODE inside a feature worker worktree (the binary errors
# ErrWrongMode). Fires before the token gate so the user sees the real cause.
if [[ "$CMD_SCRUBBED" =~ (^|[[:space:];|&])gss[[:space:]]+(push|pr)([[:space:]]|$) ]] \
   && [[ "$CMD_SCRUBBED" =~ (^|[[:space:]])--force-autonomous([[:space:]]|$) ]] \
   && gss_in_worker; then
    deny "gss push/pr --force-autonomous is invalid inside a feature worker worktree ($GSS_EFFECTIVE_DIR) — it is the wrong mode (the binary errors ErrWrongMode). Use the gss feature commands, or run classic gss from a regular checkout."
fi

# --- 10. gss approval token enforcement ---
# The git-safe-sync skill mandates an approval token generated immediately
# before any remote-mutating gss verb. The binary also checks, but we enforce
# at the hook layer so the user can never be surprised by an autonomous
# publish. Covers classic push/pr/sync AND the publish-class feature verbs:
# `feature pr --ready` (promote draft→ready), `feature merged` (re-target +
# auto-promote children), `feature restack` (force-push + retarget).
if [[ "$CMD_SCRUBBED" =~ (^|[[:space:];|&])gss[[:space:]]+(push|pr|sync)([[:space:]]|$) ]] \
   || [[ "$CMD_SCRUBBED" =~ (^|[[:space:];|&])gss[[:space:]]+feature[[:space:]]+(merged|restack)([[:space:]]|$) ]] \
   || { [[ "$CMD_SCRUBBED" =~ (^|[[:space:];|&])gss[[:space:]]+feature[[:space:]]+pr([[:space:]]|$) ]] \
        && [[ "$CMD_SCRUBBED" =~ (^|[[:space:]])--ready([[:space:]]|$) ]]; }; then
    TOKEN_FILE="${HOME}/.config/gss/approval.token"
    if [ ! -f "$TOKEN_FILE" ]; then
        deny "This gss command publishes/mutates remote state (push/pr/sync, or feature pr --ready / merged / restack) and requires an approval token, issued as a SEPARATE Bash call BEFORE it. Two-call recipe: (1) \`mkdir -p ~/.config/gss && git rev-parse HEAD > ~/.config/gss/approval.token\` (2) the gss command. Chaining both with && in one Bash call is intentionally blocked so the user sees an explicit approve→publish gate."
    fi
    # Token must be fresh — current HEAD must match the token contents. When
    # the command leads with `cd <path>`, HEAD is resolved from that repo
    # (cross-repo publishes), via the shared GSS_EFFECTIVE_DIR above.
    if command -v git &> /dev/null; then
        GIT_CHECK_DIR="."
        [ -d "$GSS_EFFECTIVE_DIR" ] && GIT_CHECK_DIR="$GSS_EFFECTIVE_DIR"
        CURRENT_HEAD="$(git -C "$GIT_CHECK_DIR" rev-parse HEAD 2>/dev/null || echo)"
        TOKEN_HEAD="$(cat "$TOKEN_FILE" 2>/dev/null || echo)"
        if [ -n "$CURRENT_HEAD" ] && [ "$CURRENT_HEAD" != "$TOKEN_HEAD" ]; then
            deny "gss approval token is stale (does not match HEAD $CURRENT_HEAD of ${GIT_CHECK_DIR}). Re-confirm with the user and regenerate."
        fi
    fi
fi

# --- 11. gss feature checkpoint outside a worker worktree ---
# Plain `gss feature checkpoint` resolves the worker from cwd. Without an
# explicit --worker AND outside the worktree root it is a classic-context
# misuse (the binary would error). Require --worker when not in a worker.
if [[ "$CMD_SCRUBBED" =~ (^|[[:space:];|&])gss[[:space:]]+feature[[:space:]]+checkpoint([[:space:]]|$) ]] \
   && ! [[ "$CMD_SCRUBBED" =~ (^|[[:space:]])--worker([[:space:]]|=) ]] \
   && ! gss_in_worker; then
    deny "gss feature checkpoint resolves the worker from cwd, but $GSS_EFFECTIVE_DIR is not a feature worker worktree. Run it from inside the worktree, or pass --worker <feature/user/purpose>."
fi

# All checks passed
[ "$TOOL_NAME" = "run_shell_command" ] && echo '{"decision": "allow"}'
exit 0
