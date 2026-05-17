#!/bin/bash
set -e
SYNC=true; LIST=false; DELETE=false; PRUNE=false; KEY_NAMES=()
while [[ $# -gt 0 ]]; do
    case $1 in
        --no-sync) SYNC=false; shift ;;
        --list) LIST=true; shift ;;
        --delete) DELETE=true; shift ;;
        --prune) PRUNE=true; shift ;;
        *) KEY_NAMES+=("$1"); shift ;;
    esac
done
if [ "$PRUNE" = true ]; then
    HOSTS=$(grep "^Host " "$HOME/.ssh/config" | awk "{print \$2}" | grep -v "*" || true)
    MANAGED=""
    for k in "$HOME/.ssh"/*.pub; do
        if [[ -f "$k" && ! "$k" == *authorized_keys* ]]; then MANAGED+="$(head -n 1 "$k" | awk "{\$1=\$1;print}")"$"\n"; fi
    done
    echo "$MANAGED" > "$HOME/.ssh/authorized_keys"; chmod 600 "$HOME/.ssh/authorized_keys"
    for h in $HOSTS; do
        echo "$MANAGED" | ssh -n -o BatchMode=yes -o ConnectTimeout=5 "$h" "cat > ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys" 2>/dev/null || echo "Failed on $h"
    done
    exit 0
fi
if [ "$LIST" = true ]; then
    HOSTS=$(grep "^Host " "$HOME/.ssh/config" | awk "{print \$2}" | grep -v "*" || true)
    for k in "$HOME/.ssh"/*.pub; do
        if [[ -f "$k" && ! "$k" == *authorized_keys* ]]; then
            PK=$(head -n 1 "$k" | awk "{\$1=\$1;print}")
            SYNCED=""
            grep -qF "$PK" "$HOME/.ssh/authorized_keys" && SYNCED="local"
            for h in $HOSTS; do
                ssh -n -o BatchMode=yes -o ConnectTimeout=1 "$h" "grep -qF \"$PK\" ~/.ssh/authorized_keys" 2>/dev/null && SYNCED+=", $h"
            done
            echo "$(basename "$k" .pub): $SYNCED"
        fi
    done
    exit 0
fi
for K in "${KEY_NAMES[@]}"; do
    P="$HOME/.ssh/$K"
    [ -f "$P" ] || ssh-keygen -t ed25519 -f "$P" -N "" -C "$(whoami)@$(hostname)-$K"
    PK=$(head -n 1 "$P.pub" | awk "{\$1=\$1;print}")
    grep -qF "$PK" "$HOME/.ssh/authorized_keys" || echo "$PK" >> "$HOME/.ssh/authorized_keys"
    if [ "$SYNC" = true ]; then
        HOSTS=$(grep "^Host " "$HOME/.ssh/config" | awk "{print \$2}" | grep -v "*" || true)
        for h in $HOSTS; do
            scp "$P" "$P.pub" "$h:~/.ssh/" 2>/dev/null && ssh -n "$h" "grep -qF \"$PK\" ~/.ssh/authorized_keys || echo \"$PK\" >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys" 2>/dev/null || true
        done
    fi
done
