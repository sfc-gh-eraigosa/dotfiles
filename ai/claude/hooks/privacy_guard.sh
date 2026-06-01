#!/bin/bash
# privacy_guard.sh — Claude Code PreToolUse hook
#
# Stops local IDENTITY (home paths, the login username, the hostname) and
# SECRETS from being written into content that will be committed or published —
# tracked repo files (specs, plans, READMEs, code), pull-request / issue bodies,
# and commit messages. The fix is always to use a variable or placeholder:
#
#     $HOME  /  ~  /  ${USER}  /  <user>  /  <host>  /  <REDACTED>
#
# Why: design docs and PR bodies are shared artifacts. An absolute path like
# C:\Users\edwar or /mnt/c/Users/edwar (or /home/<you>) leaks the account name;
# a pasted token leaks a credential. The prefix-only permission DSL and the
# Bash-only safety_guard cannot catch a Write to a markdown file, which is
# exactly how such a leak slips through.
#
# Scope (deliberately narrow to avoid false positives):
#   * Write / Edit / MultiEdit / NotebookEdit — scanned ONLY when the target
#     file is tracked-able (i.e. `git check-ignore` says it is NOT ignored and
#     it lives inside a git repo). Local-only files (~/.claude, ~/.remember,
#     memory notes, gitignored scratch) are skipped — they never get published.
#   * Bash — scanned ONLY for publishing verbs: `gh pr|issue create|edit|
#     comment`, `git commit`, `git tag -m`. Other Bash passes through.
#
# Contract (Claude Code PreToolUse):
#   stdin : JSON {tool_name, tool_input}
#   exit 0: allow
#   exit 2: block (stderr is fed back to Claude as context)
#   other : non-blocking error
#
# Dependencies: jq, grep -E, bash 3.2+, git (optional; absence = fail-open scan)

set -u

PAYLOAD="$(cat)"
TOOL_NAME="$(printf '%s' "$PAYLOAD" | jq -r '.tool_name // empty')"

jqf() { printf '%s' "$PAYLOAD" | jq -r "$1" 2>/dev/null; }

# --- Decide what text to scan, and whether to scan at all -----------------------
TEXT=""
TARGET=""
case "$TOOL_NAME" in
    Write)
        TEXT="$(jqf '.tool_input.content // empty')"
        TARGET="$(jqf '.tool_input.file_path // empty')" ;;
    Edit)
        TEXT="$(jqf '.tool_input.new_string // empty')"
        TARGET="$(jqf '.tool_input.file_path // empty')" ;;
    MultiEdit)
        TEXT="$(jqf '[.tool_input.edits[]?.new_string] | join("\n")')"
        TARGET="$(jqf '.tool_input.file_path // empty')" ;;
    NotebookEdit)
        TEXT="$(jqf '.tool_input.new_source // empty')"
        TARGET="$(jqf '.tool_input.notebook_path // empty')" ;;
    Bash)
        CMD="$(jqf '.tool_input.command // empty')"
        # Only publishing verbs carry content to a shared place. Everything else
        # (builds, greps, file ops) is out of scope for a privacy guard.
        if printf '%s' "$CMD" | grep -Eq '(^|[^[:alnum:]_])(gh[[:space:]]+(pr|issue)[[:space:]]+(create|edit|comment)|git[[:space:]]+commit|git[[:space:]]+tag[[:space:]].*-m)'; then
            TEXT="$CMD"
        else
            exit 0
        fi ;;
    *)
        exit 0 ;;
esac

[ -z "$TEXT" ] && exit 0

# --- File-target gate: skip local-only (gitignored / non-repo) targets ----------
# For file edits, only guard content that could actually be committed. A file is
# "tracked-able" when git check-ignore exits 1 (NOT ignored) inside a repo.
#   exit 0  -> ignored          -> local, skip
#   exit 1  -> not ignored      -> tracked-able, SCAN
#   exit 128-> not a git repo   -> outside any repo, skip
if [ -n "$TARGET" ] && command -v git > /dev/null 2>&1; then
    tdir="$(dirname "$TARGET")"
    git -C "$tdir" rev-parse --is-inside-work-tree > /dev/null 2>&1 || exit 0
    if git -C "$tdir" check-ignore -q "$TARGET"; then
        exit 0   # ignored -> local only
    fi
fi

# --- Identity values for THIS host (matched literally) --------------------------
ME_USER="${USER:-$(id -un 2>/dev/null || true)}"
ME_HOME="${HOME:-}"
ME_HOMEBASE=""
[ -n "$ME_HOME" ] && ME_HOMEBASE="$(basename "$ME_HOME")"
ME_HOST="${HOSTNAME:-$(hostname 2>/dev/null || true)}"
ME_HOST="$(printf '%s' "$ME_HOST" | cut -d. -f1)"

deny() {
    # stderr -> Claude (actionable); category + the offending snippet + the fix.
    local category="$1" snippet="$2"
    printf 'BLOCKED by privacy_guard: %s\n' "$category" >&2
    printf 'Offending text: %s\n' "$snippet" >&2
    printf 'Do not write local identity or secrets into committed/published content.\n' >&2
    printf 'Use a variable or placeholder instead: $HOME / ~ / ${USER} / <user> / <host> / <REDACTED>.\n' >&2
    exit 2
}

# first ERE match (case-insensitive), trimmed, for a readable deny message.
# `--` guards patterns that begin with '-' (e.g. a PEM header) from being read
# as grep options.
first_match() { printf '%s' "$TEXT" | grep -Eio -- "$1" | head -n1; }

# --- Rule A: Windows / WSL user-home paths (the canonical leak) ------------------
# C:\Users\edwar  /  C:/Users/edwar  /  /mnt/c/Users/edwar
# Placeholder forms (Users\<user>, Users\%USERNAME%) don't match: the trailing
# class is [A-Za-z0-9._-], which excludes '<' and '%'.
WIN='(/mnt/[a-z]/|[a-z]:[\\/])users[\\/]+[a-z0-9._-]+'
if printf '%s' "$TEXT" | grep -Eiq "$WIN"; then
    deny "Windows/WSL user-home path leaks the account name (use \$HOME / a variable)" "$(first_match "$WIN")"
fi

# --- Rule B: this host's real Unix home path ------------------------------------
# Exact /home/<you> or /Users/<you>. We match the REAL basename only, so generic
# system paths (/home/linuxbrew, /Users/Shared, CI /home/runner) don't trip.
if [ -n "$ME_HOMEBASE" ]; then
    UNIX="/(home|users)/${ME_HOMEBASE}([/\"' ]|$)"
    if printf '%s' "$TEXT" | grep -Eiq "$UNIX"; then
        deny "Absolute home path leaks the account name (use \$HOME or ~)" "$(first_match "/(home|users)/${ME_HOMEBASE}")"
    fi
fi

# --- Rule C: bare login username / hostname as a standalone token ---------------
# Match the literal current value on a word boundary. We match the VALUE
# (e.g. the login name), never the string "$USER", so $USER/${USER} stays legal.
word_present() { # $1 = literal token
    local tok="$1"
    [ -n "$tok" ] && [ "${#tok}" -ge 3 ] || return 1
    printf '%s' "$TEXT" | grep -Eiq "(^|[^A-Za-z0-9_\$\{])${tok}([^A-Za-z0-9_]|$)"
}
if word_present "$ME_USER"; then
    deny "Login username appears verbatim (use \${USER} or <user>)" "$ME_USER"
fi
if [ "$ME_HOST" != "localhost" ] && word_present "$ME_HOST"; then
    deny "Hostname/computer name appears verbatim (use a <host> placeholder)" "$ME_HOST"
fi

# --- Rule D: secrets -----------------------------------------------------------
# High-confidence patterns first; then a heuristic password/token assignment that
# excludes variables ($X, ${X}), placeholders (<...>, %...%), and masks (***, xxxx,
# REDACTED, changeme, your-...).
declare -a SECRET_RX=(
    '-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----'        # PEM private keys
    '(AKIA|ASIA)[0-9A-Z]{16}'                       # AWS access key id
    'ghp_[A-Za-z0-9]{30,}'                          # GitHub PAT (classic)
    'github_pat_[A-Za-z0-9_]{20,}'                  # GitHub PAT (fine-grained)
    'xox[baprs]-[A-Za-z0-9-]{10,}'                  # Slack token
    'AIza[0-9A-Za-z_-]{30,}'                        # Google API key
)
for rx in "${SECRET_RX[@]}"; do
    if printf '%s' "$TEXT" | grep -Eq -- "$rx"; then
        deny "Looks like a hard-coded secret/credential (store it in a secret manager, reference a variable)" "$(printf '%s' "$TEXT" | grep -Eo -- "$rx" | head -n1 | cut -c1-12)…"
    fi
done

# Heuristic: password|secret|token|api_key|access_key = <real value>
ASSIGN='(password|passwd|secret|token|api[_-]?key|access[_-]?key|client[_-]?secret)["'"'"']?[[:space:]]*[:=][[:space:]]*["'"'"']?[^[:space:]"'"'"'<$%*]{6,}'
if printf '%s' "$TEXT" | grep -Eiq "$ASSIGN"; then
    # exclude obvious non-secrets: placeholders / masks / env-var references
    cand="$(printf '%s' "$TEXT" | grep -Eio "$ASSIGN" | head -n1)"
    if ! printf '%s' "$cand" | grep -Eiq '(redact|changeme|example|your[-_]|placeholder|xxxx+|\*\*\*|<.+>|\$\{?[A-Za-z_])'; then
        deny "Possible hard-coded credential assignment (reference a variable or secret store)" "$cand"
    fi
fi

exit 0
