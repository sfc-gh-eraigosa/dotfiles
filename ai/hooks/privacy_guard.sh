#!/usr/bin/env bash
# privacy_guard.sh — AI Agent PreToolUse / BeforeTool hook
#
# Stops local IDENTITY (home paths, the login username, the hostname, email
# addresses) and SECRETS from being written into content that will be
# committed or published — tracked repo files, pull-request / issue / release
# bodies, and commit messages. The fix is always a variable or placeholder:
#
#     $HOME  /  ~  /  ${USER}  /  <user>  /  <host>  /  <email>  /  <REDACTED>
#
# Why: design docs and PR bodies are shared artifacts. An absolute path like
# C:\Users\<user> or /mnt/c/Users/<user> (or /home/<you>) leaks the account
# name; a pasted token leaks a credential. The prefix-only permission DSL and
# the Bash-only safety_guard cannot catch a Write to a markdown file, which is
# exactly how such a leak slips through.
#
# The RULES live in privacy_rules.sh, shared with the git hooks in
# ai/githooks/ — this hook is the fast, in-editor layer; the git hooks are the
# layer that judges what is actually recorded and published, whatever wrote it.
#
# What is judged (each is one way a leak reached a shared place before):
#   * Write / Edit / MultiEdit / NotebookEdit / write_file / replace — when the
#     target is tracked-able (inside a repo and not gitignored). Local-only
#     files (~/.claude, memory notes, gitignored scratch) are skipped.
#   * Bash / run_shell_command that WRITES A FILE by redirection, heredoc, tee,
#     sed -i or an interpreter one-liner — judged like a Write when any target
#     is tracked-able. The Write tool was never the only way to fill a file.
#   * Bash / run_shell_command that PUBLISHES — judged on what it publishes,
#     not only on the command text:
#       git commit            the staged additions (+ a -F message file)
#       git push, gss push|pr|sync, gss feature checkpoint
#                             the commits the remote does not have yet
#       gh pr|issue create|edit|comment, gh release create
#                             the inline body/notes AND any --body-file / -F /
#                             --notes-file
#       gh gist create        the files being published
#       git tag -m            the annotation
#     The repo-side judgements need the tool's cwd; the Claude payload carries
#     it. Without one, only the command text and named files are judged.
#
# Posture:
#   * FAILS CLOSED. No jq, or an unparsable payload, means the input cannot be
#     inspected — that is a refusal, not a pass.
#   * Every deny is appended to $PRIVACY_GUARD_LOG
#     (default ${XDG_STATE_HOME:-~/.local/state}/privacy_guard/blocks.log).
#   * $PRIVACY_GUARD_CONFIG_DIR/{identity,allow} extend or exempt tokens;
#     see privacy_rules.sh.
#
# Contract:
#   stdin : JSON {tool_name, tool_input, cwd?}
#   exit 0: allow (run_shell_command dialect: outputs JSON; Claude: no output)
#   exit 2: block (stderr carries the reason for both dialects)
#   other : non-blocking error
#
# Dependencies: jq (required), grep -E, bash 3.2+, git (for the repo gates)

set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=privacy_rules.sh
. "$HERE/privacy_rules.sh" || { echo "BLOCKED by privacy_guard: privacy_rules.sh missing beside $0; refusing to guess" >&2; exit 2; }

PAYLOAD="$(cat)"

deny() {
    # stderr -> Agent (actionable): category + the offending snippet + the fix.
    local category="$1" snippet="$2" log
    printf 'BLOCKED by privacy_guard: %s. Offending text: %s. Do not write local identity or secrets into committed/published content. Use a variable or placeholder instead: $HOME / ~ / ${USER} / <user> / <host> / <email> / <REDACTED>.\n' "$category" "$snippet" >&2
    log="${PRIVACY_GUARD_LOG:-${XDG_STATE_HOME:-$HOME/.local/state}/privacy_guard/blocks.log}"
    { mkdir -p "$(dirname "$log")" && printf '%s\t%s\t%s\t%s\n' "$(date '+%Y-%m-%dT%H:%M:%S')" "${TOOL_NAME:-?}" "$category" "$snippet" >> "$log"; } 2>/dev/null || true
    # Legacy-Gemini dialect expects JSON on stdout for a deny with exit 0; we
    # exit 2 for both dialects, and stderr carries the reason.
    exit 2
}

# Fail closed: without jq there is no way to know what is being written.
if ! command -v jq > /dev/null 2>&1; then
    deny "jq is not installed, so the tool input cannot be inspected (install jq)" "<payload not inspected>"
fi
if [ -n "$PAYLOAD" ] && ! printf '%s' "$PAYLOAD" | jq -e . > /dev/null 2>&1; then
    deny "the hook payload is not valid JSON, so it cannot be inspected" "<payload not inspected>"
fi

TOOL_NAME="$(printf '%s' "$PAYLOAD" | jq -r '.tool_name // empty')"
jqf() { printf '%s' "$PAYLOAD" | jq -r "$1" 2>/dev/null; }
CWD="$(jqf '.cwd // empty')"

# The run_shell_command dialect (agy) wants explicit JSON on allow.
allow() { [[ "$TOOL_NAME" =~ ^[a-z_]+$ ]] && echo '{"decision": "allow"}'; exit 0; }

# --- Repo gates ------------------------------------------------------------------

# trackable <path>: could this file be committed? Inside a work tree AND not
# gitignored. Everything else is local-only and never published.
#   check-ignore exit 0 -> ignored, 1 -> not ignored, 128 -> not a repo.
trackable() {
    local p="$1" d
    case "$p" in "~"*) p="$HOME${p#"~"}" ;; esac
    case "$p" in /dev/*) return 1 ;; esac
    command -v git > /dev/null 2>&1 || return 0   # no git: cannot tell, judge it
    d="$(dirname "$p")"
    git -C "$d" rev-parse --is-inside-work-tree > /dev/null 2>&1 || return 1
    ! git -C "$d" check-ignore -q "$p"
}

# staged_additions <repo>: the lines a `git commit` would record.
staged_additions() { git -C "$1" diff --cached -U0 --no-color --diff-filter=ACMR 2>/dev/null | grep -E '^\+' | grep -Ev '^\+\+\+ ' | cut -c2-; }

# outgoing <repo>: the commits a push would publish — those the upstream lacks,
# or (no upstream yet) those no remote has at all.
outgoing() {
    local repo="$1" range
    git -C "$repo" rev-parse --is-inside-work-tree > /dev/null 2>&1 || return 0
    if git -C "$repo" rev-parse --abbrev-ref '@{upstream}' > /dev/null 2>&1; then
        range='@{upstream}..HEAD'
    else
        range='HEAD --not --remotes'
    fi
    # shellcheck disable=SC2086  # range is deliberately word-split
    git -C "$repo" log -p -U0 --no-color --format='%n%B' $range 2>/dev/null | grep -E '^\+' | grep -Ev '^\+\+\+ ' | cut -c2-
    # shellcheck disable=SC2086
    git -C "$repo" log --no-color --format='%B' $range 2>/dev/null
}

# file_content <path>: the content of a named file, if readable (relative to
# cwd when one is known).
file_content() {
    local p="$1"
    case "$p" in "~"*) p="$HOME${p#"~"}" ;; esac
    case "$p" in /*) ;; *) [ -n "$CWD" ] && p="$CWD/$p" ;; esac
    [ -r "$p" ] && cat "$p"
}

# flag_files <cmd> <flag-ERE>: the file arguments of flags like --body-file F,
# --body-file=F, -F F.
flag_files() { printf '%s' "$1" | grep -Eo -- "$2[= ]+[^[:space:]\"']+" | sed -E "s/^$2[= ]+//"; }

# write_targets <cmd>: paths the command writes to by redirection/heredoc,
# tee, sed -i, or an interpreter one-liner.
write_targets() {
    local cmd="$1"
    printf '%s' "$cmd" | grep -Eo '>>?[[:space:]]*[^[:space:];&|)]+' | sed -E 's/^>>?[[:space:]]*//'
    printf '%s' "$cmd" | grep -Eo 'tee([[:space:]]+-a)?[[:space:]]+[^[:space:];&|)]+' | sed -E 's/^tee([[:space:]]+-a)?[[:space:]]+//'
    if printf '%s' "$cmd" | grep -Eq '(^|[^[:alnum:]_])(sed[[:space:]]+-[a-zA-Z]*i|python[0-9.]*[[:space:]]+-c|perl[[:space:]]+-e|ruby[[:space:]]+-e|node[[:space:]]+-e)'; then
        printf '%s' "$cmd" | grep -Eo "(~|/|\./|\\\$[A-Za-z_][A-Za-z0-9_]*/)[^[:space:]\"'\`;&|)]+"
    fi
}

# --- Decide what text to judge, and in which repo context -----------------------
TEXT=""
TARGET=""
CTX="${CWD:-$PWD}"
case "$TOOL_NAME" in
    Write|write_file)
        TEXT="$(jqf '.tool_input.content // empty')"
        TARGET="$(jqf '.tool_input.file_path // empty')" ;;
    Edit|replace)
        TEXT="$(jqf '.tool_input.new_string // empty')"
        TARGET="$(jqf '.tool_input.file_path // empty')" ;;
    MultiEdit)
        TEXT="$(jqf '[.tool_input.edits[]?.new_string] | join("\n")')"
        TARGET="$(jqf '.tool_input.file_path // empty')" ;;
    NotebookEdit)
        TEXT="$(jqf '.tool_input.new_source // empty')"
        TARGET="$(jqf '.tool_input.notebook_path // empty')" ;;
    Bash|run_shell_command)
        CMD="$(jqf '.tool_input.command // empty')"
        [ -n "$CMD" ] || allow

        # 1. A file write by any means, aimed at a tracked-able file.
        while IFS= read -r t; do
            t="${t%\"}"; t="${t#\"}"; t="${t%\'}"; t="${t#\'}"
            [ -n "$t" ] || continue
            case "$t" in /*|"~"*) ;; *) [ -n "$CWD" ] && t="$CWD/$t" ;; esac
            if trackable "$t"; then TEXT="$CMD"; CTX="$(dirname "$t")"; break; fi
        done <<EOF
$(write_targets "$CMD")
EOF

        # 2. A publishing verb: judge the command AND what it publishes.
        pub='(^|[^[:alnum:]_])(gh[[:space:]]+(pr|issue)[[:space:]]+(create|edit|comment)|gh[[:space:]]+(release|gist)[[:space:]]+create|git[[:space:]]+commit|git[[:space:]]+push|gss[[:space:]]+(push|pr|sync|feature[[:space:]]+checkpoint)|git[[:space:]]+tag[[:space:]].*-m)'
        if printf '%s' "$CMD" | grep -Eq "$pub"; then
            TEXT="$CMD"
            if printf '%s' "$CMD" | grep -Eq '(^|[^[:alnum:]_])git[[:space:]]+commit'; then
                [ -n "$CWD" ] && TEXT="$TEXT
$(staged_additions "$CWD")"
                for f in $(flag_files "$CMD" '(-F|--file)'); do TEXT="$TEXT
$(file_content "$f")"; done
            fi
            if printf '%s' "$CMD" | grep -Eq '(^|[^[:alnum:]_])(git[[:space:]]+push|gss[[:space:]]+(push|pr|sync|feature[[:space:]]+checkpoint))'; then
                [ -n "$CWD" ] && TEXT="$TEXT
$(outgoing "$CWD")"
            fi
            for f in $(flag_files "$CMD" '(--body-file|--notes-file|-F)'); do TEXT="$TEXT
$(file_content "$f")"; done
            if printf '%s' "$CMD" | grep -Eq 'gh[[:space:]]+gist[[:space:]]+create'; then
                for f in $(printf '%s' "$CMD" | sed -E 's/.*gh[[:space:]]+gist[[:space:]]+create//' | tr ' ' '\n' | grep -Ev '^(-|$)'); do TEXT="$TEXT
$(file_content "$f")"; done
            fi
        fi
        [ -n "$TEXT" ] || allow ;;
    *)
        allow ;;   # tools that write nothing shared (Read, Grep, ...)
esac

[ -n "$TEXT" ] || allow

# --- File-target gate: skip local-only (gitignored / non-repo) targets ----------
if [ -n "$TARGET" ]; then
    trackable "$TARGET" || allow
    CTX="$(dirname "$TARGET")"
fi

# --- Judge ----------------------------------------------------------------------
if finding="$(privacy_scan "$TEXT" "$CTX")"; then
    allow
fi
deny "${finding%%	*}" "${finding#*	}"
