#!/usr/bin/env bash
# privacy_rules.sh — the ONE rule set behind every privacy guard.
#
# Sourced (never executed) by:
#   ai/hooks/privacy_guard.sh        the agent PreToolUse hook (Claude / agy)
#   ai/githooks/pre-commit           the git index, whatever wrote it
#   ai/githooks/commit-msg           the commit message, -m or -F
#   ai/githooks/pre-push             the outgoing commits (this is what gss hits)
#
# One library so the agent-side and git-side layers cannot drift: a token the
# git hook refuses is exactly a token the agent hook refuses.
#
# API
#   privacy_scan <text> [context-dir]
#       Judges text. Prints "category<TAB>snippet" for the FIRST finding and
#       returns 2; returns 0 when clean. context-dir is where identity is
#       gathered from (the repo being written to), default $PWD.
#   privacy_identity_tokens [context-dir]
#       Prints the literal identity tokens in force, one per line. For
#       diagnostics ("why was this blocked?").
#
# Configuration (all optional), in $PRIVACY_GUARD_CONFIG_DIR, default
# ${XDG_CONFIG_HOME:-~/.config}/privacy_guard — LOCAL to the machine, never
# tracked:
#   identity   extra literal tokens to refuse, one per line (# comments ok):
#              an old handle, a project codename, a relative's name.
#   allow      tokens to NEVER refuse, one per line. The escape for a login
#              name that is also an ordinary word.
#
# Always exempt: a git trailer (Signed-off-by:, Co-authored-by:, any *-by:)
# carrying the CONFIGURED git user.email of the repo being judged — that address
# is already on every commit's author line. Trailers naming anyone else are not.
#
# Portability: bash 3.2+ (no associative arrays, no mapfile, no ${v,,}), and
# only POSIX grep -E. Every helper is prefixed privacy_ so sourcing it into a
# hook cannot shadow anything.

# --- identity -------------------------------------------------------------------

privacy_config_dir() {
    printf '%s' "${PRIVACY_GUARD_CONFIG_DIR:-${XDG_CONFIG_HOME:-$HOME/.config}/privacy_guard}"
}

# privacy_file_tokens <file>: the non-comment, non-empty lines of a config file.
privacy_file_tokens() {
    [ -r "$1" ] || return 0
    grep -Ev '^[[:space:]]*(#|$)' "$1" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//'
}

# privacy_identity_tokens [context-dir]: every literal that names THIS person
# or THIS machine. Sources, in order: login name, home basename, short
# hostname, the git user.email of the context repo (whole address and the
# local part), and the identity file. Tokens shorter than 3 chars are dropped
# (they cannot be matched safely) and the allow file removes the rest.
privacy_identity_tokens() {
    local ctx="${1:-$PWD}" cfg tok email
    cfg="$(privacy_config_dir)"
    {
        printf '%s\n' "${USER:-$(id -un 2>/dev/null || true)}"
        [ -n "${HOME:-}" ] && basename "$HOME"
        printf '%s\n' "${HOSTNAME:-$(hostname 2>/dev/null || true)}" | cut -d. -f1
        email="$(git -C "$ctx" config user.email 2>/dev/null || true)"
        if [ -n "$email" ]; then
            printf '%s\n' "$email"
            printf '%s\n' "${email%%@*}"
        fi
        privacy_file_tokens "$cfg/identity"
    } | awk 'length($0) >= 3 && $0 != "localhost" && !seen[$0]++' \
      | privacy_drop_allowed "$cfg/allow"
}

# privacy_drop_allowed <allow-file>: filter stdin, removing allowlisted tokens
# (case-insensitive).
privacy_drop_allowed() {
    local allow="$1"
    if [ -r "$allow" ]; then
        grep -Fixv -f <(privacy_file_tokens "$allow") || true
    else
        cat
    fi
}

# privacy_allowed <token>: is this exact token allowlisted? Used by the rules
# that match by SHAPE (email) rather than by token.
privacy_allowed() {
    local allow
    allow="$(privacy_config_dir)/allow"
    [ -r "$allow" ] && privacy_file_tokens "$allow" | grep -Fixq -- "$1"
}

# --- matching helpers ----------------------------------------------------------

# privacy_first <text> <ERE>: first case-insensitive match, for a readable
# deny message. `--` guards patterns that begin with '-' (a PEM header).
privacy_first() { printf '%s' "$1" | grep -Eio -- "$2" | head -n1; }

# privacy_hit <text> <ERE>: does text match (case-insensitive)?
privacy_hit() { printf '%s' "$1" | grep -Eiq -- "$2"; }

# privacy_escape <token>: make a literal safe inside an ERE.
privacy_escape() { printf '%s' "$1" | sed 's/[][\.|$(){}?+*^]/\\&/g'; }

# privacy_found <category> <snippet>: emit a finding. Callers `return 2`.
privacy_found() { printf '%s\t%s\n' "$1" "$2"; }

# --- the rules -----------------------------------------------------------------

# privacy_scan <text> [context-dir]
privacy_scan() {
    local text="$1" ctx="${2:-$PWD}"
    [ -n "$text" ] || return 0

    # Trailers ("Signed-off-by: Name <addr>", "Co-authored-by: …") are where git
    # identity is SUPPOSED to appear, and the author field already publishes the
    # configured address on every commit. A *-by: trailer carrying the
    # configured user.email is therefore no leak: it is cut out before judging,
    # wherever it sits (a message line, or inside -m "…" on a command line).
    # Only the trailer itself goes — the name part may not carry a path, URL or
    # address — so anything else on the line is still judged, and a trailer
    # naming anyone ELSE is judged like any other text.
    local gitmail
    gitmail="$(git -C "$ctx" config user.email 2>/dev/null || true)"
    if [ -n "$gitmail" ]; then
        text="$(printf '%s\n' "$text" | sed -E "s|[A-Za-z][A-Za-z-]*-[Bb][Yy]:[[:space:]]*[^<>/:@]*<$(privacy_escape "$gitmail")>||g")"
        [ -n "$(printf '%s' "$text" | tr -d '[:space:]')" ] || return 0
    fi

    # Rule A — Windows / WSL user-home paths: C:\Users\<name>, /mnt/c/Users/<name>.
    # Placeholders (Users\<user>, Users\%USERNAME%, Users\${USER}) do not match:
    # the trailing class excludes '<', '%' and '$'.
    local win='(/mnt/[a-z]/|[a-z]:[\\/])users[\\/]+[a-z0-9._-]+'
    if privacy_hit "$text" "$win"; then
        privacy_found "Windows/WSL user-home path leaks the account name (use \$HOME / a variable)" "$(privacy_first "$text" "$win")"
        return 2
    fi

    # Rule B — this host's real Unix home path. The REAL basename only, so
    # /home/linuxbrew, /Users/Shared and CI's /home/runner do not trip it.
    local homebase=""
    [ -n "${HOME:-}" ] && homebase="$(basename "$HOME")"
    if [ -n "$homebase" ] && ! privacy_allowed "$homebase"; then
        local unix
        unix="/(home|users)/$(privacy_escape "$homebase")([/\"' ]|$)"
        if privacy_hit "$text" "$unix"; then
            privacy_found "Absolute home path leaks the account name (use \$HOME or ~)" "$(privacy_first "$text" "/(home|users)/$(privacy_escape "$homebase")")"
            return 2
        fi
    fi

    # Rule C — identity tokens, two shapes each:
    #   whole word   "deployed by alice"          (the original rule)
    #   prefix       "alicegigabyte", "aliceboxpi" (a fleet hostname that
    #                extends the login name — the shape the original missed)
    # The prefix shape needs 5+ chars or every short login name matches
    # ordinary words. The VALUE is matched, never "$USER", so ${USER} stays legal.
    local tok esc label user host
    user="${USER:-$(id -un 2>/dev/null || true)}"
    host="$(printf '%s' "${HOSTNAME:-$(hostname 2>/dev/null || true)}" | cut -d. -f1)"
    while IFS= read -r tok; do
        [ -n "$tok" ] || continue
        esc="$(privacy_escape "$tok")"
        case "$tok" in
            "$user") label="Login username appears verbatim (use \${USER} or <user>)" ;;
            "$host") label="Hostname/computer name appears verbatim (use a <host> placeholder)" ;;
            *@*)     label="Your email address appears verbatim (use <email> or a role address)" ;;
            *)       label="Identity token \"$tok\" appears verbatim (use a placeholder)" ;;
        esac
        if privacy_hit "$text" "(^|[^A-Za-z0-9_\$\{@.-])${esc}([^A-Za-z0-9_@.-]|$)"; then
            privacy_found "$label" "$tok"
            return 2
        fi
        if [ "${#tok}" -ge 5 ] && privacy_hit "$text" "(^|[^A-Za-z0-9_\$\{@.-])${esc}[a-z0-9]+([^A-Za-z0-9_@.-]|$)"; then
            privacy_found "${label%% appears*} appears as a prefix of \"$(privacy_first "$text" "${esc}[a-z0-9]+")\" (use a placeholder)" "$tok"
            return 2
        fi
    done <<EOF
$(privacy_identity_tokens "$ctx")
EOF

    # Rule E — ANY email address is somebody's identity, not just yours.
    # Exempt: placeholder local parts (<user>, user, you, me, name, test,
    # noreply) and a variable in the local-part slot ($USER@…, %USERNAME%@…).
    # Angle brackets are NOT a placeholder: "<bob@corp.example>" is an address.
    local mail='[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}'
    local addr local_part
    addr="$(privacy_first "$text" "$mail")"
    if [ -n "$addr" ]; then
        local_part="$(printf '%s' "${addr%%@*}" | tr '[:upper:]' '[:lower:]')"
        case "$local_part" in
            user|you|me|name|test|example|noreply|no-reply|someone|admin|root|git|bot) ;;
            *)
                if ! privacy_allowed "$addr" && ! privacy_hit "$text" "[\$%{]$(privacy_escape "$addr")"; then
                    privacy_found "An email address appears verbatim (use <email> or a role address)" "$addr"
                    return 2
                fi ;;
        esac
    fi

    # Rule D — secrets. High-confidence shapes first.
    local rx
    for rx in \
        '-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----' \
        '(AKIA|ASIA)[0-9A-Z]{16}' \
        'ghp_[A-Za-z0-9]{30,}' \
        'github_pat_[A-Za-z0-9_]{20,}' \
        'gh[ousr]_[A-Za-z0-9]{30,}' \
        'glpat-[A-Za-z0-9_-]{20,}' \
        'xox[baprs]-[A-Za-z0-9-]{10,}' \
        'xapp-[0-9]-[A-Za-z0-9-]{10,}' \
        'hooks\.slack\.com/services/[A-Za-z0-9/]{20,}' \
        'AIza[0-9A-Za-z_-]{30,}' \
        'sk-ant-[A-Za-z0-9_-]{20,}' \
        'sk-(proj|svcacct|admin)-[A-Za-z0-9_-]{20,}' \
        'sk-[A-Za-z0-9]{40,}' \
        'sk_(live|test)_[A-Za-z0-9]{20,}' \
        'npm_[A-Za-z0-9]{36}' \
        'eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}' \
    ; do
        if printf '%s' "$text" | grep -Eq -- "$rx"; then
            privacy_found "Looks like a hard-coded secret/credential (store it in a secret manager, reference a variable)" "$(printf '%s' "$text" | grep -Eo -- "$rx" | head -n1 | cut -c1-12)…"
            return 2
        fi
    done

    # Credentials inside a URL: scheme://user:password@host. Placeholders and
    # variables in the password slot are fine.
    local urlcred='[a-z][a-z0-9+.-]*://[^/:@[:space:]]+:[^/@[:space:]]{3,}@'
    local cred
    cred="$(privacy_first "$text" "$urlcred")"
    if [ -n "$cred" ] && ! printf '%s' "$cred" | grep -Eiq '(redact|changeme|example|password|placeholder|xxx|\*\*\*|<|\$|%)'; then
        privacy_found "A URL carries an inline password (reference a variable or secret store)" "$cred"
        return 2
    fi

    # Heuristic: password|secret|token|api_key|access_key = <real value>,
    # excluding variables ($X, ${X}), placeholders (<...>, %...%) and masks.
    local assign='(password|passwd|secret|token|api[_-]?key|access[_-]?key|client[_-]?secret)["'"'"']?[[:space:]]*[:=][[:space:]]*["'"'"']?[^[:space:]"'"'"'<$%*]{6,}'
    local cand
    cand="$(privacy_first "$text" "$assign")"
    if [ -n "$cand" ] && ! printf '%s' "$cand" | grep -Eiq '(redact|changeme|example|your[-_]|placeholder|xxxx+|\*\*\*|<.+>|\$\{?[A-Za-z_])'; then
        privacy_found "Possible hard-coded credential assignment (reference a variable or secret store)" "$cand"
        return 2
    fi

    return 0
}
