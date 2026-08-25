#!/usr/bin/env bash
# Test driver for opt/scripts/system/wsl_dns_lan.sh
#
# The script only ever writes /etc/wsl.conf and /etc/resolv.conf, and only
# after a probe succeeds — so it is NOT safe to execute for real here. Every
# case below drives it through --dry-run (which writes nothing) against a
# stub `dig` and a stub `powershell.exe` placed first on PATH, so the probe,
# the selection logic, and the rendered output are all exercised without
# touching the host or the network.
#
# Run: bash opt/scripts/system/wsl_dns_lan_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/wsl_dns_lan.sh"

# === 1. Syntax check ===
assert_exit_code 0 "wsl_dns_lan.sh parses with bash -n" bash -n "$SCRIPT"

# --- stub harness ----------------------------------------------------------
# Builds a throwaway PATH prefix containing:
#   dig            — answers only for (server,host) pairs listed in STUB_ZONE
#   powershell.exe — prints the CRLF-terminated server list in STUB_WIN_DNS
# STUB_ZONE lines are "server host ip". Anything else gets an empty answer,
# which is how real dig reports NXDOMAIN/no-A-record.
_make_stubs() {
    local dir="$1"
    mkdir -p "$dir"

    cat > "${dir}/dig" <<'STUB'
#!/usr/bin/env bash
# stub dig: parse "@server" and the name, look them up in $STUB_ZONE
set -u
srv=""; name=""
for a in "$@"; do
    case "$a" in
        @*) srv="${a#@}" ;;
        +*|A) : ;;
        *) name="$a" ;;
    esac
done
[ -n "${STUB_ZONE:-}" ] || exit 0
while read -r z_srv z_host z_ip; do
    [ -n "$z_srv" ] || continue
    if [ "$z_srv" = "$srv" ] && [ "$z_host" = "$name" ]; then
        echo "$z_ip"
        exit 0
    fi
done <<< "$STUB_ZONE"
exit 0
STUB

    cat > "${dir}/powershell.exe" <<'STUB'
#!/usr/bin/env bash
# stub powershell.exe: emit the candidate list with Windows CRLF line endings
set -u
printf '%s\r\n' ${STUB_WIN_DNS:-}
STUB

    cat > "${dir}/getent" <<'STUB'
#!/usr/bin/env bash
# stub getent: resolves names listed in $STUB_RESOLVABLE; anything else misses
# after $STUB_MISS_DELAY seconds (used to simulate a slow resolver timeout).
set -u
[ "${1:-}" = "hosts" ] || exit 2
for n in ${STUB_RESOLVABLE:-}; do
    if [ "$n" = "${2:-}" ]; then echo "10.1.2.3 $2"; exit 0; fi
done
sleep "${STUB_MISS_DELAY:-0}"
exit 2
STUB

    chmod +x "${dir}/dig" "${dir}/powershell.exe" "${dir}/getent"
}

STUB_DIR="$(mktemp -d)"
_make_stubs "$STUB_DIR"

# Minimal ssh config with three #fleet hosts, one non-fleet host, and one
# wildcard pattern that must NOT be probed.
SSH_CFG="$(mktemp)"
cat > "$SSH_CFG" <<'EOF'
Host lab-pi  #fleet
    Hostname lab-pi

Host lab-nas  #fleet
    Hostname lab-nas

Host notfleet
    Hostname notfleet

Host *  #fleet
    ServerAliveInterval 30
EOF

# Run the script under the stubs. Args after the first three are passed through.
_run() {
    local zone="$1" win_dns="$2" ssh_cfg="$3"; shift 3
    PATH="${STUB_DIR}:${PATH}" \
    STUB_ZONE="$zone" \
    STUB_WIN_DNS="$win_dns" \
    WSL_DNS_SSH_CONFIG="$ssh_cfg" \
    WSL_DNS_FALLBACKS="1.1.1.1" \
        bash "$SCRIPT" --dry-run "$@" 2>&1
}

# The script no-ops off WSL. Every case below needs the WSL gate to pass, so
# skip the behavioural cases entirely when not on WSL rather than reporting
# false failures.
if grep -qi microsoft /proc/version 2>/dev/null; then

    # === 2. Picks the server that resolves the most fleet hosts ===
    ZONE_GOOD="10.10.0.1 lab-pi 10.10.0.21
10.10.0.1 lab-nas 10.10.0.22
10.10.0.1 github.com 198.51.100.10"
    OUT="$(_run "$ZONE_GOOD" "198.51.100.53 10.10.0.1" "$SSH_CFG")"
    OUT_FILE="$(mktemp)"; printf '%s\n' "$OUT" > "$OUT_FILE"

    assert_grep "selects the LAN resolver that answers for the fleet" \
        'selected 10\.10\.0\.1 \(2/2 fleet hosts\)' "$OUT_FILE"
    # 198.51.100.53 has no stub entries at all, so it answers nothing -- the
    # script must report that as unreachable, not as "resolved 0/N".
    assert_grep "an entirely silent candidate is reported as unreachable" \
        'candidate 198\.51\.100\.53: NO RESPONSE' "$OUT_FILE"
    assert_grep "pins the winner as the FIRST nameserver" \
        '^nameserver 10\.10\.0\.1$' "$OUT_FILE"
    assert_grep "keeps a fallback resolver after the winner" \
        '^nameserver 1\.1\.1\.1$' "$OUT_FILE"
    assert_grep "sets a short resolver timeout so off-network fails over fast" \
        '^options timeout:1 attempts:1$' "$OUT_FILE"
    assert_grep "renders generateResolvConf = false" \
        '^generateResolvConf = false$' "$OUT_FILE"
    assert_grep_negative "never probes the wildcard Host pattern" \
        'candidate .*: resolved .*\*' "$OUT_FILE"

    # === 3. Default-gateway trap: the WRONG server must not win ===
    # The local gateway answers for an unrelated name only; the tunnel resolver
    # answers for the fleet. Selection must follow the fleet, not the gateway.
    ZONE_TRAP="192.0.2.1 notfleet 192.0.2.9
10.10.0.1 lab-pi 10.10.0.21
10.10.0.1 lab-nas 10.10.0.22
10.10.0.1 github.com 198.51.100.10"
    OUT2="$(_run "$ZONE_TRAP" "192.0.2.1 10.10.0.1" "$SSH_CFG")"
    OUT2_FILE="$(mktemp)"; printf '%s\n' "$OUT2" > "$OUT2_FILE"
    assert_grep "does not pick the default-gateway resolver" \
        'selected 10\.10\.0\.1' "$OUT2_FILE"

    # === 4. No resolver answers -> warn, write nothing, still exit 0 ===
    OUT3="$(_run "" "198.51.100.53 192.0.2.1" "$SSH_CFG")"
    OUT3_FILE="$(mktemp)"; printf '%s\n' "$OUT3" > "$OUT3_FILE"
    assert_grep "warns when no candidate resolves any fleet host" \
        'no candidate resolver answered' "$OUT3_FILE"
    assert_grep_negative "writes no resolv.conf when nothing resolves" \
        '^nameserver ' "$OUT3_FILE"
    assert_exit_code 0 "exits 0 when no resolver answers (non-fatal)" \
        env PATH="${STUB_DIR}:${PATH}" STUB_ZONE="" STUB_WIN_DNS="198.51.100.53" \
            WSL_DNS_SSH_CONFIG="$SSH_CFG" bash "$SCRIPT" --dry-run

    # === 4b. ALL candidates silent => "tunnel not ready" diagnosis ===
    # Nothing answers, not even for the public probe. That is the signature of
    # a VPN adapter attached before its handshake completed.
    assert_grep "diagnoses an attached-but-not-ready tunnel when nothing answers" \
        'TUNNEL LIKELY NOT READY' "$OUT3_FILE"
    assert_grep "reports how many resolvers stayed silent" \
        'NOT ONE of the 2 configured resolvers responded' "$OUT3_FILE"

    # === 4c. Some answer, none knows the fleet => a DIFFERENT diagnosis ===
    # Must not be misreported as "not ready" -- the network is plainly working.
    ZONE_PUBLIC_ONLY="198.51.100.53 github.com 198.51.100.10"
    OUT3B="$(_run "$ZONE_PUBLIC_ONLY" "198.51.100.53 192.0.2.1" "$SSH_CFG")"
    OUT3B_FILE="$(mktemp)"; printf '%s\n' "$OUT3B" > "$OUT3B_FILE"
    assert_grep "a reachable-but-ignorant resolver is labelled reachable" \
        'candidate 198\.51\.100\.53: reachable, resolved 0/' "$OUT3B_FILE"
    assert_grep_negative "does NOT blame the tunnel when resolvers are answering" \
        'TUNNEL LIKELY NOT READY' "$OUT3B_FILE"

    # === 4d. Names served by /etc/hosts are excluded from the DNS probe ===
    # The local hostname is in /etc/hosts, so nsswitch "files" answers it
    # before DNS. Probing a resolver for it would cap the score forever and
    # make --verify count a /etc/hosts hit as resolver evidence.
    FAKE_HOSTS="$(mktemp)"
    printf '127.0.0.1\tlocalhost\n127.0.1.1\tselfhost.localdomain\tselfhost\n' > "$FAKE_HOSTS"
    OUT3C="$(PATH="${STUB_DIR}:${PATH}" \
        STUB_ZONE="10.10.0.1 lab-pi 10.10.0.21
10.10.0.1 github.com 198.51.100.10" \
        STUB_WIN_DNS="10.10.0.1" \
        WSL_DNS_PROBE_HOSTS="lab-pi selfhost" \
        WSL_DNS_HOSTS_FILE="$FAKE_HOSTS" \
        WSL_DNS_FALLBACKS="1.1.1.1" \
        bash "$SCRIPT" --dry-run 2>&1)"
    OUT3C_FILE="$(mktemp)"; printf '%s\n' "$OUT3C" > "$OUT3C_FILE"
    assert_grep "announces the /etc/hosts name it is not probing" \
        'not probing selfhost' "$OUT3C_FILE"
    assert_grep "scores only the DNS-resolvable hosts (1/1, not 1/2)" \
        'selected 10\.10\.0\.1 \(1/1 fleet hosts\)' "$OUT3C_FILE"

    # === 5. Loopback / link-local candidates are filtered out ===
    OUT4="$(_run "$ZONE_GOOD" "127.0.0.53 169.254.1.1 10.10.0.1" "$SSH_CFG")"
    OUT4_FILE="$(mktemp)"; printf '%s\n' "$OUT4" > "$OUT4_FILE"
    assert_grep_negative "skips loopback candidates" 'candidate 127\.' "$OUT4_FILE"
    assert_grep_negative "skips link-local candidates" 'candidate 169\.254\.' "$OUT4_FILE"

    # === 6. WSL_DNS_PROBE_HOSTS overrides the ssh config ===
    ZONE_OVERRIDE="10.10.0.1 myhost 10.10.0.50
10.10.0.1 github.com 198.51.100.10"
    OUT5="$(PATH="${STUB_DIR}:${PATH}" STUB_ZONE="$ZONE_OVERRIDE" \
            STUB_WIN_DNS="10.10.0.1" WSL_DNS_PROBE_HOSTS="myhost" \
            WSL_DNS_SSH_CONFIG=/nonexistent WSL_DNS_FALLBACKS="1.1.1.1" \
            bash "$SCRIPT" --dry-run 2>&1)"
    OUT5_FILE="$(mktemp)"; printf '%s\n' "$OUT5" > "$OUT5_FILE"
    assert_grep "WSL_DNS_PROBE_HOSTS drives the probe set" \
        'selected 10\.10\.0\.1 \(1/1 fleet hosts\)' "$OUT5_FILE"

    # === 7. No probe hosts at all -> clean no-op ===
    OUT6="$(PATH="${STUB_DIR}:${PATH}" STUB_ZONE="" STUB_WIN_DNS="10.10.0.1" \
            WSL_DNS_SSH_CONFIG=/nonexistent bash "$SCRIPT" --dry-run 2>&1)"
    OUT6_FILE="$(mktemp)"; printf '%s\n' "$OUT6" > "$OUT6_FILE"
    assert_grep "no-ops cleanly with no fleet hosts to probe" \
        'nothing to do' "$OUT6_FILE"

    # === 8. Missing dig is non-fatal ===
    # Point WSL_DNS_DIG at a binary that does not exist rather than emptying
    # PATH — the script needs grep/awk/sed itself, so an empty PATH would test
    # nothing but the WSL gate.
    OUT7="$(PATH="${STUB_DIR}:${PATH}" WSL_DNS_DIG=/nonexistent/dig \
            WSL_DNS_SSH_CONFIG="$SSH_CFG" bash "$SCRIPT" --dry-run 2>&1)"
    OUT7_FILE="$(mktemp)"; printf '%s\n' "$OUT7" > "$OUT7_FILE"
    assert_grep "warns (not fails) when dig is absent" 'dig not found' "$OUT7_FILE"
    assert_exit_code 0 "exits 0 when dig is absent (non-fatal)" \
        env WSL_DNS_DIG=/nonexistent/dig WSL_DNS_SSH_CONFIG="$SSH_CFG" \
            bash "$SCRIPT" --dry-run

    # === 8b. --help prints the header block and exits 0 ===
    HELP_FILE="$(mktemp)"; bash "$SCRIPT" --help > "$HELP_FILE" 2>&1
    assert_grep "--help documents the WSL_DNS_DIG override" \
        'WSL_DNS_DIG' "$HELP_FILE"
    assert_grep_negative "--help strips the comment markers" '^#' "$HELP_FILE"

    # === 9. Unknown flags are rejected ===
    assert_exit_code 2 "rejects unknown arguments" bash "$SCRIPT" --bogus

    # === 10. Recursion guard: a local-only resolver must NOT be pinned ===
    # resolv.conf is an ordered list, not a routing table -- nameserver #1
    # answers every query and its NXDOMAIN is final, so a resolver that cannot
    # answer for public names would take out public DNS with no fallback.
    ZONE_LOCAL_ONLY="10.10.0.1 lab-pi 10.10.0.21
10.10.0.1 lab-nas 10.10.0.22"
    OUT8="$(_run "$ZONE_LOCAL_ONLY" "10.10.0.1" "$SSH_CFG")"
    OUT8_FILE="$(mktemp)"; printf '%s\n' "$OUT8" > "$OUT8_FILE"
    assert_grep "refuses a resolver that cannot answer for public names" \
        'resolves fleet hosts but NOT github\.com' "$OUT8_FILE"
    assert_grep_negative "writes no resolv.conf when the winner cannot recurse" \
        '^nameserver ' "$OUT8_FILE"
    assert_exit_code 0 "recursion refusal is non-fatal" \
        env PATH="${STUB_DIR}:${PATH}" STUB_ZONE="$ZONE_LOCAL_ONLY" \
            STUB_WIN_DNS="10.10.0.1" WSL_DNS_SSH_CONFIG="$SSH_CFG" \
            bash "$SCRIPT" --dry-run

    # === 10b. The override forces the pin, with a loud warning ===
    OUT9="$(PATH="${STUB_DIR}:${PATH}" STUB_ZONE="$ZONE_LOCAL_ONLY" \
            STUB_WIN_DNS="10.10.0.1" WSL_DNS_SSH_CONFIG="$SSH_CFG" \
            WSL_DNS_FALLBACKS="1.1.1.1" WSL_DNS_ALLOW_NONRECURSIVE=1 \
            bash "$SCRIPT" --dry-run 2>&1)"
    OUT9_FILE="$(mktemp)"; printf '%s\n' "$OUT9" > "$OUT9_FILE"
    assert_grep "WSL_DNS_ALLOW_NONRECURSIVE=1 pins anyway" \
        'pinning anyway' "$OUT9_FILE"
    assert_grep "override still renders the resolv.conf" \
        '^nameserver 10\.10\.0\.1$' "$OUT9_FILE"

    # === 10c. A custom public probe is honoured ===
    OUT10="$(PATH="${STUB_DIR}:${PATH}" STUB_ZONE="$ZONE_LOCAL_ONLY
10.10.0.1 example.test 10.9.9.9" \
            STUB_WIN_DNS="10.10.0.1" WSL_DNS_SSH_CONFIG="$SSH_CFG" \
            WSL_DNS_FALLBACKS="1.1.1.1" WSL_DNS_PUBLIC_PROBE=example.test \
            bash "$SCRIPT" --dry-run 2>&1)"
    OUT10_FILE="$(mktemp)"; printf '%s\n' "$OUT10" > "$OUT10_FILE"
    assert_grep "WSL_DNS_PUBLIC_PROBE selects the recursion sentinel" \
        'also resolves example\.test' "$OUT10_FILE"

    # === 10d. --verify, fleet resolver UP: public + fleet both resolve ===
    # The pinned resolver answers for a fleet name, so fleet lookups are
    # EXPECTED to succeed. Uses a throwaway resolv.conf via the injection hook.
    VER_DIR="$(mktemp -d)"
    printf 'nameserver 10.10.0.1\n' > "${VER_DIR}/resolv.conf"
    VER_UP="$(PATH="${STUB_DIR}:${PATH}" \
        STUB_ZONE="$ZONE_GOOD" \
        STUB_RESOLVABLE="github.com lab-pi lab-nas" \
        WSL_DNS_RESOLV_CONF="${VER_DIR}/resolv.conf" \
        WSL_DNS_GETENT=getent \
        WSL_DNS_PROBE_HOSTS="lab-pi lab-nas" \
        bash "$SCRIPT" --verify 2>&1)"
    VER_UP_FILE="$(mktemp)"; printf '%s\n' "$VER_UP" > "$VER_UP_FILE"
    assert_grep "--verify detects the fleet resolver is serving fleet names" \
        'serves fleet names: up' "$VER_UP_FILE"
    assert_grep "--verify passes when public and fleet both resolve" \
        'verify — PASS' "$VER_UP_FILE"
    assert_exit_code 0 "--verify exits 0 on a healthy tunnel-up state" \
        env PATH="${STUB_DIR}:${PATH}" STUB_ZONE="$ZONE_GOOD" \
            STUB_RESOLVABLE="github.com lab-pi lab-nas" \
            WSL_DNS_RESOLV_CONF="${VER_DIR}/resolv.conf" WSL_DNS_GETENT=getent \
            WSL_DNS_PROBE_HOSTS="lab-pi lab-nas" \
            bash "$SCRIPT" --verify

    # === 10e. --verify, fleet resolver DOWN: public resolves, fleet misses FAST ===
    # This is the expected off-VPN state and must still PASS.
    VER_DOWN="$(PATH="${STUB_DIR}:${PATH}" \
        STUB_ZONE="" \
        STUB_RESOLVABLE="github.com" \
        STUB_MISS_DELAY=0 \
        WSL_DNS_RESOLV_CONF="${VER_DIR}/resolv.conf" \
        WSL_DNS_GETENT=getent \
        WSL_DNS_PROBE_HOSTS="lab-pi lab-nas" \
        bash "$SCRIPT" --verify 2>&1)"
    VER_DOWN_FILE="$(mktemp)"; printf '%s\n' "$VER_DOWN" > "$VER_DOWN_FILE"
    assert_grep "--verify detects the fleet resolver is unreachable" \
        'serves fleet names: down' "$VER_DOWN_FILE"
    assert_grep "--verify still requires public DNS to work when the tunnel is down" \
        'github\.com OK' "$VER_DOWN_FILE"
    assert_grep "--verify PASSES on a fast off-VPN miss" \
        'verify — PASS' "$VER_DOWN_FILE"

    # === 10f. --verify FAILS on a slow miss (the original 10s+ stall) ===
    VER_SLOW="$(PATH="${STUB_DIR}:${PATH}" \
        STUB_ZONE="" \
        STUB_RESOLVABLE="github.com" \
        STUB_MISS_DELAY=2 \
        WSL_DNS_MAX_FAIL_SECONDS=1 \
        WSL_DNS_RESOLV_CONF="${VER_DIR}/resolv.conf" \
        WSL_DNS_GETENT=getent \
        WSL_DNS_PROBE_HOSTS="lab-pi" \
        bash "$SCRIPT" --verify 2>&1 || true)"
    VER_SLOW_FILE="$(mktemp)"; printf '%s\n' "$VER_SLOW" > "$VER_SLOW_FILE"
    assert_grep "--verify flags a miss slower than the limit" \
        'resolver timeout tuning regressed' "$VER_SLOW_FILE"
    assert_grep "--verify FAILS on the slow-stall regression" \
        'verify — FAIL' "$VER_SLOW_FILE"

    # === 10g. --verify FAILS when public DNS is broken, in either state ===
    VER_NOPUB="$(PATH="${STUB_DIR}:${PATH}" \
        STUB_ZONE="$ZONE_GOOD" \
        STUB_RESOLVABLE="lab-pi" \
        WSL_DNS_RESOLV_CONF="${VER_DIR}/resolv.conf" \
        WSL_DNS_GETENT=getent \
        WSL_DNS_PROBE_HOSTS="lab-pi" \
        bash "$SCRIPT" --verify 2>&1 || true)"
    VER_NOPUB_FILE="$(mktemp)"; printf '%s\n' "$VER_NOPUB" > "$VER_NOPUB_FILE"
    assert_grep "--verify treats broken public DNS as a hard failure" \
        'public DNS is broken' "$VER_NOPUB_FILE"
    assert_grep "--verify FAILS when the public sentinel does not resolve" \
        'verify — FAIL' "$VER_NOPUB_FILE"

    # === 11. --revert with no snapshot repairs the stock WSL layout ===
    # Targets throwaway files via the injection hooks; the host /etc is never
    # touched and no sudo is involved.
    FAKE_ETC="$(mktemp -d)"
    ln -sfn /mnt/wsl/resolv.conf "${FAKE_ETC}/resolv.conf"
    printf '[boot]\nsystemd=true\n\n[network]\ngenerateResolvConf = false\n' \
        > "${FAKE_ETC}/wsl.conf"
    REVERT_OUT="$(WSL_DNS_NO_SUDO=1 \
        WSL_DNS_WSL_CONF="${FAKE_ETC}/wsl.conf" \
        WSL_DNS_RESOLV_CONF="${FAKE_ETC}/resolv.conf" \
        WSL_DNS_BACKUP_DIR="${FAKE_ETC}/no-such-backup" \
        bash "$SCRIPT" --revert 2>&1)"
    REVERT_FILE="$(mktemp)"; printf '%s\n' "$REVERT_OUT" > "$REVERT_FILE"
    assert_grep "--revert without a snapshot restores the stock symlink" \
        'restored .*resolv\.conf -> /mnt/wsl/resolv\.conf' "$REVERT_FILE"
    assert_exit_code 0 "reverted resolv.conf is a symlink again" \
        test -L "${FAKE_ETC}/resolv.conf"
    assert_grep_negative "--revert drops generateResolvConf from wsl.conf" \
        'generateResolvConf' "${FAKE_ETC}/wsl.conf"
    assert_grep "--revert keeps the unrelated [boot] section intact" \
        '^systemd=true$' "${FAKE_ETC}/wsl.conf"

    # === 12. Full round trip: write -> revert restores the exact originals ===
    RT="$(mktemp -d)"
    ln -sfn /mnt/wsl/resolv.conf "${RT}/resolv.conf"
    printf '[boot]\nsystemd=true\n' > "${RT}/wsl.conf"
    RT_WSL_BEFORE="$(cat "${RT}/wsl.conf")"
    RT_LINK_BEFORE="$(readlink "${RT}/resolv.conf")"

    RT_ENV=(WSL_DNS_NO_SUDO=1
            WSL_DNS_WSL_CONF="${RT}/wsl.conf"
            WSL_DNS_RESOLV_CONF="${RT}/resolv.conf"
            WSL_DNS_BACKUP_DIR="${RT}/backup"
            WSL_DNS_SSH_CONFIG="$SSH_CFG"
            WSL_DNS_FALLBACKS="1.1.1.1"
            STUB_ZONE="$ZONE_GOOD"
            STUB_WIN_DNS="10.10.0.1")

    PATH="${STUB_DIR}:${PATH}" env "${RT_ENV[@]}" bash "$SCRIPT" --quiet > /dev/null 2>&1

    assert_grep "write replaced the symlink with a managed resolv.conf" \
        '^nameserver 10\.10\.0\.1$' "${RT}/resolv.conf"
    assert_grep "write set generateResolvConf = false" \
        '^generateResolvConf = false$' "${RT}/wsl.conf"
    assert_exit_code 0 "write took a snapshot" test -d "${RT}/backup"

    # a second run must be a no-op and must NOT clobber the snapshot
    PATH="${STUB_DIR}:${PATH}" env "${RT_ENV[@]}" bash "$SCRIPT" --quiet > /dev/null 2>&1
    assert_grep "snapshot still holds the ORIGINAL symlink after a re-run" \
        '^/mnt/wsl/resolv\.conf$' "${RT}/backup/resolv.conf.symlink"

    PATH="${STUB_DIR}:${PATH}" env "${RT_ENV[@]}" bash "$SCRIPT" --revert > /dev/null 2>&1

    assert_eq "$RT_LINK_BEFORE" "$(readlink "${RT}/resolv.conf")" \
        "round trip restored the exact resolv.conf symlink target"
    assert_eq "$RT_WSL_BEFORE" "$(cat "${RT}/wsl.conf")" \
        "round trip restored wsl.conf byte-for-byte"
    assert_exit_code 1 "revert removed the snapshot dir" test -d "${RT}/backup"

else
    echo "SKIP: not running under WSL; behavioural cases need the WSL gate."
fi

_test_report
