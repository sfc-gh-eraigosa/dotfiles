#!/usr/bin/env bash
# wsl_dns_lan.sh — make LAN / VPN hostnames resolvable from inside WSL2.
#
# Problem: WSL2 points /etc/resolv.conf at the Windows NAT DNS proxy
# (typically 10.255.255.254). That proxy answers from whatever resolver
# Windows considers primary — normally the Wi-Fi/Ethernet interface's public
# DNS. Bare LAN names served by a home router (or by a router reachable over a
# VPN/WireGuard tunnel) are NOT in that resolver, so `ssh <fleet-host>` stalls
# for the full resolver timeout and then dies with
#   ssh: Could not resolve hostname <host>: Temporary failure in name resolution
# while `ssh <ip>` connects instantly.
#
# Naive "just use the default gateway" detection gets this WRONG: the default
# route points at the local network's gateway, which is exactly the resolver
# that does NOT know the fleet. The DNS server that does know them may live on
# a secondary interface (a WireGuard tunnel, a docking-station NIC, a VPN).
#
# Fix: enumerate the DNS servers Windows has on EVERY interface, actually PROBE
# each one for the hostnames we care about, and pin the winner as the first
# nameserver in a hand-managed /etc/resolv.conf (with the previous resolvers
# kept as fallbacks so public DNS still works when the tunnel is down).
#
# IMPORTANT -- why the winner must be a full recursive resolver:
# /etc/resolv.conf is an ORDERED LIST, not a routing table. glibc sends EVERY
# query to nameserver #1 regardless of the name; there is no per-suffix or
# per-range dispatch. The later `nameserver` lines are tried only when #1
# TIMES OUT -- an NXDOMAIN from #1 is a FINAL answer and glibc does not fall
# through on it. So pinning a resolver that serves only local names would
# answer NXDOMAIN for github.com and break public DNS outright, with no
# fallback. Step 4b below therefore proves the winner also resolves a public
# name before anything is written.
#
# Probe hostnames default to the `#fleet`-marked Host entries in ~/.ssh/config,
# so the set follows the ssh config instead of being hardcoded here.
#
# Idempotent and non-fatal: safe to re-run from install.sh; exits 0 with a
# warning whenever it cannot act (not WSL, no probe hosts, no dig, no winner,
# no root).
#
# Environment overrides:
#   WSL_DNS_PROBE_HOSTS   space/comma list of hostnames to probe (skips ssh config)
#   WSL_DNS_SERVERS       space/comma list of candidate DNS servers to try FIRST
#   WSL_DNS_FALLBACKS     space/comma list of fallback resolvers
#                         (default: the resolvers already in /etc/resolv.conf,
#                          then 1.1.1.1)
#   WSL_DNS_SSH_CONFIG    path to the ssh config to scan (default ~/.ssh/config)
#   WSL_DNS_HOSTS_FILE    hosts file consulted before DNS (default /etc/hosts)
#   WSL_DNS_DIG           dig binary to probe with (default: dig from PATH)
#   WSL_DNS_PUBLIC_PROBE  public name used to prove the winner recurses
#                         (default: github.com)
#   WSL_DNS_ALLOW_NONRECURSIVE=1
#                         pin the winner even if it cannot resolve public
#                         names (see the recursion check below) -- this WILL
#                         break public DNS; for split-horizon setups only
#   WSL_DNS_BACKUP_DIR    snapshot location (default /etc/wsl_dns_lan.backup)
#   WSL_DNS_MAX_FAIL_SECONDS
#                         --verify: longest acceptable failed lookup, in
#                         seconds. DERIVED from the live resolv.conf by
#                         default (see expected_fail_budget); set it to pin an
#                         explicit ceiling instead.
#   WSL_DNS_WSL_CONF / WSL_DNS_RESOLV_CONF / WSL_DNS_NO_SUDO / WSL_DNS_GETENT
#                         testing hooks; retarget the files, skip sudo, stub
#                         the name-resolution call
#
# Flags:
#   --dry-run   probe and report the winner; write nothing
#   --revert    restore the pre-change /etc/resolv.conf and /etc/wsl.conf and
#               exit; undoes everything this script did
#   --verify    live self-check: resolve the public sentinel and every fleet
#               host through the REAL resolver path and print a pass/fail
#               table. Run it once with the VPN/tunnel up and once with it
#               down -- the public name must resolve in BOTH states, the fleet
#               names only when the tunnel is up, and a fleet miss must fail
#               FAST (that is the original 10s stall regressing). Exits 1 on a
#               failed expectation.
#   --quiet     suppress the informational (non-warning) output
#
# Safety: before its FIRST write this script snapshots both files (including
# whether resolv.conf was a symlink and where it pointed) into
# /etc/wsl_dns_lan.backup. `--revert` restores that snapshot exactly, so a bad
# pin can always be undone. If the snapshot is missing, --revert still repairs
# the machine by restoring WSL's stock layout: resolv.conf -> /mnt/wsl/resolv.conf
# and generateResolvConf dropped from wsl.conf.

set -u

DRY_RUN=0
QUIET=0
REVERT=0
VERIFY=0
for _arg in "$@"; do
    case "$_arg" in
        --dry-run) DRY_RUN=1 ;;
        --revert)  REVERT=1 ;;
        --verify)  VERIFY=1 ;;
        --quiet)   QUIET=1 ;;
        -h|--help)
            # print the leading comment block, minus the '#' prefix
            awk 'NR>1 && /^#/ { sub(/^# ?/, ""); print; next } NR>1 { exit }' "$0"
            exit 0
            ;;
        *)
            echo "wsl-dns: unknown argument '$_arg' (use --dry-run, --revert, --verify, --quiet)" >&2
            exit 2
            ;;
    esac
done

SSH_CONFIG="${WSL_DNS_SSH_CONFIG:-${HOME}/.ssh/config}"
HOSTS_FILE="${WSL_DNS_HOSTS_FILE:-/etc/hosts}"
DIG_BIN="${WSL_DNS_DIG:-dig}"
BACKUP_DIR="${WSL_DNS_BACKUP_DIR:-/etc/wsl_dns_lan.backup}"
GETENT_BIN="${WSL_DNS_GETENT:-getent}"

# WSL's stock resolv.conf is a symlink here; the revert fallback recreates it.
WSL_STOCK_RESOLV=/mnt/wsl/resolv.conf
# Testing hooks: these let the test driver point the script at throwaway files
# instead of the host's /etc. They default to the real paths.
WSL_CONF="${WSL_DNS_WSL_CONF:-/etc/wsl.conf}"
RESOLV_CONF="${WSL_DNS_RESOLV_CONF:-/etc/resolv.conf}"
# The Windows NAT proxy and loopback/link-local addresses are never useful
# candidates: the proxy is the thing that is already failing us.
FLEET_MARKER='#fleet'

say()  { [ "$QUIET" -eq 1 ] || echo "wsl-dns: $*"; }
warn() { echo "wsl-dns: WARNING — $*" >&2; }

# --- gate: WSL only --------------------------------------------------------
if ! grep -qi microsoft /proc/version 2>/dev/null; then
    exit 0
fi

# --- normalize a comma/space separated list into one item per line ---------
to_lines() {
    printf '%s\n' "$1" | tr ',' ' ' | tr -s '[:space:]' '\n' | sed '/^$/d'
}

# --- privilege escalation: root, interactive sudo (tty), or sudo -n --------
# WSL_DNS_NO_SUDO=1 runs commands directly (testing hook: the driver targets
# throwaway files it already owns, so sudo would only get in the way).
run_root() {
    if [ "${WSL_DNS_NO_SUDO:-0}" = "1" ]; then
        "$@"
    elif [ "$(id -u)" = "0" ]; then
        "$@"
    elif [ -t 0 ]; then
        sudo "$@"
    else
        sudo -n "$@"
    fi
}

# --- snapshot / restore ----------------------------------------------------
# Taken ONCE, before the first write. Re-running the script must never
# overwrite a good snapshot with our own managed files, or --revert would
# "restore" the very state it is meant to undo.
backup_state() {
    if [ -d "${BACKUP_DIR}" ]; then
        return 0
    fi
    run_root mkdir -p "${BACKUP_DIR}" || return 1

    if [ -L "${RESOLV_CONF}" ]; then
        readlink "${RESOLV_CONF}" | run_root tee "${BACKUP_DIR}/resolv.conf.symlink" > /dev/null
    elif [ -f "${RESOLV_CONF}" ]; then
        run_root cp "${RESOLV_CONF}" "${BACKUP_DIR}/resolv.conf.file"
    else
        run_root touch "${BACKUP_DIR}/resolv.conf.absent"
    fi

    if [ -f "${WSL_CONF}" ]; then
        run_root cp "${WSL_CONF}" "${BACKUP_DIR}/wsl.conf.file"
    else
        run_root touch "${BACKUP_DIR}/wsl.conf.absent"
    fi
    say "snapshotted the previous DNS config to ${BACKUP_DIR}."
}

# Drop the generateResolvConf key (and a [network] section left empty by its
# removal) so WSL resumes generating resolv.conf itself. Used only by the
# no-snapshot fallback path.
strip_generate_key() {
    [ -f "${WSL_CONF}" ] || return 0
    awk '
        # buffer a [network] header until we know the section has content
        /^[[:space:]]*\[network\][[:space:]]*$/ { held = $0; in_net = 1; next }
        /^[[:space:]]*\[/ {
            if (held != "") { print held; held = "" }
            in_net = 0; print; next
        }
        in_net && /^[[:space:]]*generateResolvConf[[:space:]]*=/ { next }
        {
            if (in_net && held != "" && $0 !~ /^[[:space:]]*$/) { print held; held = "" }
            print
        }
    ' "${WSL_CONF}"
}

do_revert() {
    if [ ! -d "${BACKUP_DIR}" ]; then
        warn "no snapshot at ${BACKUP_DIR}; restoring WSL's stock layout instead."
        run_root rm -f "${RESOLV_CONF}"
        run_root ln -sfn "${WSL_STOCK_RESOLV}" "${RESOLV_CONF}" || {
            warn "could not recreate ${RESOLV_CONF} -> ${WSL_STOCK_RESOLV}."
            return 1
        }
        if [ -f "${WSL_CONF}" ]; then
            _stripped="$(strip_generate_key)"
            printf '%s\n' "${_stripped}" | run_root tee "${WSL_CONF}" > /dev/null
        fi
        say "restored ${RESOLV_CONF} -> ${WSL_STOCK_RESOLV}; WSL will regenerate it."
        return 0
    fi

    # resolv.conf
    run_root rm -f "${RESOLV_CONF}"
    if [ -f "${BACKUP_DIR}/resolv.conf.symlink" ]; then
        _target="$(cat "${BACKUP_DIR}/resolv.conf.symlink")"
        run_root ln -sfn "${_target}" "${RESOLV_CONF}"
        say "restored ${RESOLV_CONF} -> ${_target}"
    elif [ -f "${BACKUP_DIR}/resolv.conf.file" ]; then
        run_root cp "${BACKUP_DIR}/resolv.conf.file" "${RESOLV_CONF}"
        say "restored the original ${RESOLV_CONF} file."
    else
        say "there was no ${RESOLV_CONF} before; left it absent."
    fi

    # wsl.conf
    if [ -f "${BACKUP_DIR}/wsl.conf.file" ]; then
        run_root cp "${BACKUP_DIR}/wsl.conf.file" "${WSL_CONF}"
        say "restored the original ${WSL_CONF}."
    elif [ -f "${BACKUP_DIR}/wsl.conf.absent" ]; then
        run_root rm -f "${WSL_CONF}"
        say "there was no ${WSL_CONF} before; removed it."
    fi

    run_root rm -rf "${BACKUP_DIR}"
    say "revert complete. Run 'wsl.exe --shutdown' from Windows for a fully clean slate."
}

# --- live self-check --------------------------------------------------------
# Deliberately goes through getent (the real nsswitch -> glibc resolver path),
# NOT dig -- dig would bypass resolv.conf ordering and prove nothing about
# what ssh will actually experience.
verify_name() {
    _vn="$1"; _t0=$SECONDS
    if "${GETENT_BIN}" hosts "${_vn}" > /dev/null 2>&1; then
        _vr=OK
    else
        _vr=MISS
    fi
    printf '%s %s %s\n' "${_vn}" "${_vr}" "$(( SECONDS - _t0 ))"
}

# Longest a FAILED lookup should plausibly take, derived from the resolver
# config actually in force rather than guessed. A miss must walk every
# nameserver, and getent resolves both A and AAAA, so a dead server's timeout
# is paid once per family:
#
#     budget = nameservers x timeout x 2 families + 1s slack
#
# With the pinned config (3 nameservers, timeout:1) that is 7s, comfortably
# above the ~4s a real off-tunnel miss costs, and far below the 20s+ stall
# this script exists to remove -- so the check still catches the regression it
# was written for. An unmanaged resolv.conf has no `options` line, where the
# glibc default timeout is 5s.
expected_fail_budget() {
    _ns=$(awk '$1 == "nameserver"' "${RESOLV_CONF}" 2>/dev/null | wc -l | tr -d ' ')
    [ "${_ns}" -gt 0 ] 2>/dev/null || _ns=1
    _to=$(awk '/^[[:space:]]*options/ {
                   for (i = 1; i <= NF; i++)
                       if ($i ~ /^timeout:/) { split($i, a, ":"); print a[2]; exit }
               }' "${RESOLV_CONF}" 2>/dev/null)
    [ -n "${_to}" ] || _to=5          # glibc default when unset
    echo $(( _ns * _to * 2 + 1 ))
}

do_verify() {
    MAX_FAIL_SECONDS="${WSL_DNS_MAX_FAIL_SECONDS:-$(expected_fail_budget)}"
    _pinned="$(awk '$1 == "nameserver" { print $2; exit }' "${RESOLV_CONF}" 2>/dev/null)"
    _public="${WSL_DNS_PUBLIC_PROBE:-github.com}"
    _hosts="$(probe_hosts)"

    if [ -z "${_hosts}" ]; then
        warn "no fleet hosts to verify (checked ${SSH_CONFIG})."
        return 1
    fi

    # Tunnel state: can the pinned resolver be reached at all? dig is optional
    # here -- when it is missing we infer state from the fleet results instead.
    # Ask the pinned resolver for a FLEET name, not the public one: the stock
    # WSL proxy happily answers github.com and would masquerade as a working
    # fleet resolver.
    _first_fleet="$(printf '%s\n' "${_hosts}" | head -n 1)"
    _tunnel=unknown
    if [ -n "${_pinned}" ] && command -v "${DIG_BIN}" > /dev/null 2>&1; then
        if resolves "${_pinned}" "${_first_fleet}" > /dev/null; then
            _tunnel=up
        else
            _tunnel=down
        fi
    fi

    echo "wsl-dns: verify — pinned resolver: ${_pinned:-<none>} (serves fleet names: ${_tunnel})"
    echo "wsl-dns: verify — a failed lookup must complete within ${MAX_FAIL_SECONDS}s (derived from ${RESOLV_CONF})"
    echo "wsl-dns: verify — NAME RESULT SECONDS"

    _fail=0

    # 1. the public sentinel must resolve in BOTH tunnel states.
    read -r _vname _vres _vsec <<< "$(verify_name "${_public}")"
    echo "  ${_vname} ${_vres} ${_vsec}s"
    if [ "${_vres}" != "OK" ]; then
        warn "${_public} did NOT resolve — public DNS is broken. This is a FAIL in either tunnel state."
        _fail=1
    fi

    # 2. fleet hosts: expected to resolve only when the tunnel is up.
    _fleet_ok=0
    _fleet_total=0
    for _h in ${_hosts}; do
        _fleet_total=$(( _fleet_total + 1 ))
        read -r _vname _vres _vsec <<< "$(verify_name "${_h}")"
        echo "  ${_vname} ${_vres} ${_vsec}s"
        if [ "${_vres}" = "OK" ]; then
            _fleet_ok=$(( _fleet_ok + 1 ))
        elif [ "${_vsec}" -gt "${MAX_FAIL_SECONDS}" ]; then
            warn "${_vname} took ${_vsec}s to fail (limit ${MAX_FAIL_SECONDS}s) — the resolver timeout tuning regressed."
            _fail=1
        fi
    done

    case "${_tunnel}" in
        up)
            if [ "${_fleet_ok}" -eq 0 ]; then
                warn "the pinned resolver is reachable but resolved 0/${_fleet_total} fleet hosts — re-run without --verify to re-pin."
                _fail=1
            else
                say "verify — tunnel UP: public OK, ${_fleet_ok}/${_fleet_total} fleet hosts resolve."
            fi
            ;;
        down)
            if [ "${_fail}" -eq 0 ]; then
                say "verify — fleet resolver unreachable: public OK, ${_fleet_ok}/${_fleet_total} fleet names resolve, and every miss failed within ${MAX_FAIL_SECONDS}s."
            else
                say "verify — fleet resolver unreachable: public OK, but a miss exceeded ${MAX_FAIL_SECONDS}s (see the warnings above)."
                say "verify — that slow miss IS the bug this script fixes; run it without --verify to pin a resolver."
            fi
            ;;
        *)
            say "verify — tunnel state unknown (no dig): public OK, ${_fleet_ok}/${_fleet_total} fleet hosts resolve."
            ;;
    esac

    if [ "${_fail}" -eq 0 ]; then
        echo "wsl-dns: verify — PASS"
        return 0
    fi
    echo "wsl-dns: verify — FAIL"
    return 1
}

# --- 1. probe hostnames ----------------------------------------------------
# Prefer the explicit override; otherwise take every `Host <name>  #fleet`
# entry from the ssh config. Host lines may carry several names; take them all
# and drop wildcard patterns, which cannot be resolved.
# Names already served by /etc/hosts. nsswitch is "files dns", so these are
# answered BEFORE any resolver is consulted -- probing DNS for them is
# meaningless. The local hostname is the usual case: WSL writes it into
# /etc/hosts, yet a LAN/VPN resolver returns NXDOMAIN for it forever, which
# would cap the score below 100% and make --verify report a fleet "hit" that
# actually came from /etc/hosts rather than from the resolver under test.
hosts_file_names() {
    [ -r "${HOSTS_FILE}" ] || return 0
    awk '{ sub(/#.*/, ""); for (i = 2; i <= NF; i++) print $i }' "${HOSTS_FILE}"
}

probe_hosts_raw() {
    if [ -n "${WSL_DNS_PROBE_HOSTS:-}" ]; then
        to_lines "${WSL_DNS_PROBE_HOSTS}"
        return
    fi
    [ -r "${SSH_CONFIG}" ] || return 0
    awk -v marker="${FLEET_MARKER}" '
        # only Host lines carrying the fleet marker
        $1 == "Host" && index($0, marker) > 0 {
            for (i = 2; i <= NF; i++) {
                if ($i ~ /^#/) break            # start of the trailing comment
                if ($i ~ /[*?!]/) continue      # patterns are not resolvable
                print $i
            }
        }
    ' "${SSH_CONFIG}"
}

probe_hosts() {
    _local_names="$(hosts_file_names)"
    probe_hosts_raw | while read -r _ph; do
        [ -n "${_ph}" ] || continue
        if printf '%s\n' "${_local_names}" | grep -q -x -F "${_ph}"; then
            continue
        fi
        printf '%s\n' "${_ph}"
    done
}

# Reports what probe_hosts dropped, so a capped score is never a mystery.
skipped_hosts() {
    _local_names="$(hosts_file_names)"
    probe_hosts_raw | while read -r _ph; do
        [ -n "${_ph}" ] || continue
        if printf '%s\n' "${_local_names}" | grep -q -x -F "${_ph}"; then
            printf '%s\n' "${_ph}"
        fi
    done
}

# --- 2. candidate DNS servers ---------------------------------------------
# Windows knows the per-interface resolvers; WSL does not. Ask Windows via
# interop. `command -v powershell.exe` can fail even on a healthy WSL when the
# interop PATH entries are missing, so fall back to the well-known absolute
# path before giving up.
find_powershell() {
    if command -v powershell.exe > /dev/null 2>&1; then
        command -v powershell.exe
        return 0
    fi
    _ps='/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe'
    if [ -x "${_ps}" ]; then
        printf '%s\n' "${_ps}"
        return 0
    fi
    return 1
}

windows_dns_servers() {
    _pwsh="$(find_powershell)" || return 0
    # ForEach-Object flattens the ServerAddresses arrays into one address per
    # line; Windows emits CRLF, so strip the CR.
    "${_pwsh}" -NoProfile -NonInteractive -Command \
        'Get-DnsClientServerAddress -AddressFamily IPv4 | ForEach-Object { $_.ServerAddresses } | Sort-Object -Unique' \
        2>/dev/null | tr -d '\r'
}

# Keep only plausible, useful IPv4 resolvers.
filter_candidates() {
    grep -E '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$' \
        | grep -v -E '^(127\.|169\.254\.|0\.0\.0\.0$)' \
        | awk '!seen[$0]++'
}

# --- 3. probe a server for a hostname -------------------------------------
# dig is the only dependency; install.sh installs dnsutils (packages.tsv)
# before this script runs. Short timeout + single try so an unreachable
# candidate costs ~2s rather than the default ~15s.
resolves() {
    _srv="$1"; _host="$2"
    _out="$("${DIG_BIN}" +short +time=2 +tries=1 "@${_srv}" "${_host}" A 2>/dev/null \
            | grep -E '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$' | head -n 1)"
    [ -n "${_out}" ] || return 1
    printf '%s\n' "${_out}"
}

# --verify and --revert are dispatched here, after the helpers they call are
# defined but before the dig requirement: neither mode needs a working probe.
if [ "${VERIFY}" -eq 1 ]; then
    do_verify
    exit $?
fi

if [ "${REVERT}" -eq 1 ]; then
    do_revert
    exit $?
fi

if ! command -v "${DIG_BIN}" > /dev/null 2>&1; then
    warn "dig not found (install the 'dnsutils' package); skipping DNS setup."
    exit 0
fi

HOSTS="$(probe_hosts)"
_SKIPPED="$(skipped_hosts)"
if [ -n "${_SKIPPED}" ]; then
    say "not probing $(printf '%s ' ${_SKIPPED})— already served by ${HOSTS_FILE} (files precedes dns)."
fi
if [ -z "${HOSTS}" ]; then
    say "no ${FLEET_MARKER} hosts in ${SSH_CONFIG} and WSL_DNS_PROBE_HOSTS unset; nothing to do."
    exit 0
fi

CANDIDATES="$(
    { [ -n "${WSL_DNS_SERVERS:-}" ] && to_lines "${WSL_DNS_SERVERS}"
      windows_dns_servers
    } | filter_candidates
)"

if [ -z "${CANDIDATES}" ]; then
    warn "no candidate DNS servers found (Windows interop unavailable?); skipping."
    exit 0
fi

# --- 4. pick the server that resolves the most probe hosts -----------------
BEST_SRV=''
BEST_HITS=0
TOTAL=$(printf '%s\n' "${HOSTS}" | wc -l | tr -d ' ')

# A candidate that answers NOTHING at all is a different failure from one that
# answers but doesn't know the fleet. The first usually means a VPN/tunnel
# interface is attached while its handshake is still completing (Windows adds
# the adapter and its DNS server before WireGuard finishes), or the tunnel
# dropped. Distinguishing them turns a confusing "0/N everywhere" into an
# actionable "wait and re-run".
SILENT_SRVS=""
SILENT_COUNT=0
CAND_COUNT=0
for srv in ${CANDIDATES}; do
    CAND_COUNT=$(( CAND_COUNT + 1 ))
    hits=0
    for h in ${HOSTS}; do
        if resolves "${srv}" "${h}" > /dev/null; then
            hits=$((hits + 1))
        fi
    done
    if [ "${hits}" -eq 0 ]; then
        if resolves "${srv}" "${WSL_DNS_PUBLIC_PROBE:-github.com}" > /dev/null; then
            say "candidate ${srv}: reachable, resolved 0/${TOTAL} fleet host(s)."
        else
            # Neutral wording on purpose: a silent resolver is often silent
            # BECAUSE a full-tunnel VPN is up and its network is no longer
            # routable. Only the all-silent case below implies "not ready".
            say "candidate ${srv}: NO RESPONSE — not reachable from this network."
            SILENT_SRVS="${SILENT_SRVS}${srv} "
            SILENT_COUNT=$(( SILENT_COUNT + 1 ))
        fi
    else
        say "candidate ${srv}: resolved ${hits}/${TOTAL} fleet host(s)."
    fi
    if [ "${hits}" -gt "${BEST_HITS}" ]; then
        BEST_HITS="${hits}"
        BEST_SRV="${srv}"
    fi
done

if [ -z "${BEST_SRV}" ]; then
    warn "no candidate resolver answered for any of: $(printf '%s ' ${HOSTS})"
    if [ "${SILENT_COUNT}" -eq "${CAND_COUNT}" ] && [ "${CAND_COUNT}" -gt 0 ]; then
        # NOTHING answered, not even for a public name. On a machine with a
        # working internet connection that almost always means a tunnel is
        # attached but NOT READY: Windows adds the VPN adapter and its DNS
        # server the moment you click connect, seconds before the handshake
        # completes -- and while it is half-up the old network is already
        # unroutable. Retrying after the handshake is the fix.
        warn "NOT ONE of the ${CAND_COUNT} configured resolvers responded, even for a public name."
        warn "TUNNEL LIKELY NOT READY: a VPN adapter and its DNS server appear the moment you"
        warn "click connect, seconds before the handshake finishes, and the old network is"
        warn "already unroutable by then. Wait for it to finish connecting, then re-run."
    elif [ -n "${SILENT_SRVS}" ]; then
        warn "some resolvers answered but none knows these names; unreachable ones: ${SILENT_SRVS}"
        warn "Is the tunnel that serves these names the one that is up?"
    else
        warn "every resolver responded, but none knows these names — is the right VPN/tunnel up?"
    fi
    warn "re-run with: ~/opt/scripts/system/wsl_dns_lan.sh"
    exit 0
fi

say "selected ${BEST_SRV} (${BEST_HITS}/${TOTAL} fleet hosts)."

# --- 4b. the winner must also recurse for public names ---------------------
# See the header note: nameserver #1 answers everything, and its NXDOMAIN is
# final. A local-only resolver in slot #1 would take out public DNS entirely.
PUBLIC_PROBE="${WSL_DNS_PUBLIC_PROBE:-github.com}"
if resolves "${BEST_SRV}" "${PUBLIC_PROBE}" > /dev/null; then
    say "${BEST_SRV} also resolves ${PUBLIC_PROBE}; safe to pin first."
elif [ "${WSL_DNS_ALLOW_NONRECURSIVE:-0}" = "1" ]; then
    warn "${BEST_SRV} did NOT resolve ${PUBLIC_PROBE}, but WSL_DNS_ALLOW_NONRECURSIVE=1 -- pinning anyway."
    warn "public DNS will break: an NXDOMAIN from the first nameserver is final, so the fallbacks never run."
else
    warn "${BEST_SRV} resolves fleet hosts but NOT ${PUBLIC_PROBE}."
    warn "Pinning it first would break ALL public DNS: every query goes to nameserver #1,"
    warn "and its NXDOMAIN is a final answer -- the fallback nameservers are only tried on timeout."
    warn "Refusing to change DNS. Override with WSL_DNS_ALLOW_NONRECURSIVE=1 if you know better,"
    warn "or point WSL_DNS_PUBLIC_PROBE at a name this resolver should be able to answer."
    exit 0
fi

# --- 5. fallback resolvers -------------------------------------------------
# Keep whatever is answering today so ordinary internet DNS still works when
# the LAN/VPN resolver is unreachable, and always end with a public resolver.
if [ -n "${WSL_DNS_FALLBACKS:-}" ]; then
    FALLBACKS="$(to_lines "${WSL_DNS_FALLBACKS}" | filter_candidates)"
else
    FALLBACKS="$(awk '$1 == "nameserver" { print $2 }' "${RESOLV_CONF}" 2>/dev/null | filter_candidates)"
    FALLBACKS="$(printf '%s\n1.1.1.1\n' "${FALLBACKS}" | filter_candidates)"
fi
# Never list the winner twice.
FALLBACKS="$(printf '%s\n' "${FALLBACKS}" | grep -v -x -F "${BEST_SRV}")"

# --- 6. render the new resolv.conf ----------------------------------------
# `timeout:1 attempts:1` is what keeps this safe off-network: when the tunnel
# is down the pinned resolver fails over to the fallbacks in ~1s instead of the
# default 5s x 2 tries, so a laptop on a foreign network stays usable.
render_resolv() {
    echo "# Managed by dotfiles opt/scripts/system/wsl_dns_lan.sh — do not edit."
    echo "# Regenerate: ~/opt/scripts/system/wsl_dns_lan.sh"
    echo "# WSL's generated resolv.conf is disabled via ${WSL_CONF} (generateResolvConf)."
    echo "options timeout:1 attempts:1"
    echo "nameserver ${BEST_SRV}"
    for fb in ${FALLBACKS}; do
        echo "nameserver ${fb}"
    done
}

# --- 7. render wsl.conf with generateResolvConf = false --------------------
# Without this WSL overwrites /etc/resolv.conf on every boot. Handle all three
# INI shapes: key already present (rewrite in place), [network] section present
# but key absent (insert under it), no [network] section (append one).
render_wsl_conf() {
    if [ ! -f "${WSL_CONF}" ]; then
        printf '[network]\ngenerateResolvConf = false\n'
        return
    fi
    awk '
        BEGIN { in_net = 0; done = 0; seen_net = 0 }
        # section header
        /^[[:space:]]*\[/ {
            # leaving [network] without having written the key -> write it now
            if (in_net && !done) { print "generateResolvConf = false"; done = 1 }
            in_net = ($0 ~ /^[[:space:]]*\[network\][[:space:]]*$/)
            if (in_net) seen_net = 1
            print
            next
        }
        # the key itself, anywhere in [network]
        in_net && /^[[:space:]]*generateResolvConf[[:space:]]*=/ {
            if (!done) { print "generateResolvConf = false"; done = 1 }
            next
        }
        { print }
        END {
            if (in_net && !done) { print "generateResolvConf = false"; done = 1 }
            if (!seen_net)       { print ""; print "[network]"; print "generateResolvConf = false" }
        }
    ' "${WSL_CONF}"
}

NEW_RESOLV="$(render_resolv)"
NEW_WSL_CONF="$(render_wsl_conf)"

if [ "${DRY_RUN}" -eq 1 ]; then
    say "--dry-run; would write:"
    echo "--- ${RESOLV_CONF} ---"
    printf '%s\n' "${NEW_RESOLV}"
    echo "--- ${WSL_CONF} ---"
    printf '%s\n' "${NEW_WSL_CONF}"
    exit 0
fi

# --- 8. idempotence check --------------------------------------------------
# Compare against what is actually on disk; a no-op run must not touch files
# (and must not prompt for sudo).
current_resolv="$(cat "${RESOLV_CONF}" 2>/dev/null || true)"
current_wsl_conf="$(cat "${WSL_CONF}" 2>/dev/null || true)"
if [ "${current_resolv}" = "${NEW_RESOLV}" ] && [ "${current_wsl_conf}" = "${NEW_WSL_CONF}" ]; then
    say "already configured (${BEST_SRV} pinned); nothing to do."
    exit 0
fi

# --- 9. write ---------------------------------------------------------------
# Snapshot FIRST so `--revert` can always undo what follows.
if ! backup_state; then
    warn "could not write the snapshot to ${BACKUP_DIR}; refusing to change DNS without an undo path."
    exit 0
fi

if ! printf '%s\n' "${NEW_WSL_CONF}" | run_root tee "${WSL_CONF}" > /dev/null 2>&1; then
    warn "need root to write ${WSL_CONF}; skipping (re-run with sudo)."
    exit 0
fi

# WSL ships /etc/resolv.conf as a symlink to /mnt/wsl/resolv.conf, which is
# shared across distros and regenerated by WSL. Replace the link with a real
# file so our content survives and stays local to this distro.
if [ -L "${RESOLV_CONF}" ]; then
    run_root rm -f "${RESOLV_CONF}" || {
        warn "could not remove the ${RESOLV_CONF} symlink; skipping."
        exit 0
    }
fi

if ! printf '%s\n' "${NEW_RESOLV}" | run_root tee "${RESOLV_CONF}" > /dev/null 2>&1; then
    warn "need root to write ${RESOLV_CONF}; skipping."
    exit 0
fi

say "pinned ${BEST_SRV} in ${RESOLV_CONF}; ${WSL_CONF} keeps WSL from overwriting it."
say "undo any time: ~/opt/scripts/system/wsl_dns_lan.sh --revert"

# --- 10. verify -------------------------------------------------------------
# Resolution goes through the C resolver now, so confirm the real path works
# rather than trusting the dig probe from step 4.
first_host="$(printf '%s\n' "${HOSTS}" | head -n 1)"
if getent hosts "${first_host}" > /dev/null 2>&1; then
    say "verified: ${first_host} -> $(getent hosts "${first_host}" | awk '{print $1}' | head -n 1)"
else
    warn "${first_host} still does not resolve via getent; check ${RESOLV_CONF}."
fi
