# WSL LAN/VPN name resolution (`install.system.wsl-dns`)

`ssh <fleet-host>` hangs for 20s from WSL and then fails, while `ssh <ip>` works. This
pins a resolver that actually knows your fleet. **Opt-in and off by default** — it takes
over `/etc/resolv.conf`, so nothing happens until you ask for it.

## Quickstart

```sh
S=~/opt/scripts/system/wsl_dns_lan.sh

$S --dry-run     # 1. preview: which resolver wins, what would be written
$S               # 2. apply  : snapshots first, then pins (asks for sudo once)
$S --verify      # 3. check  : run once with the VPN up, once with it down
$S --revert      # undo, restoring the exact previous files
```

Bring the VPN/tunnel up **before** step 1 and give it a few seconds to finish
connecting — the adapter appears before its handshake completes, and probing in that
window finds nothing.

To have `install.sh` do it on every run:

```sh
gff set install.system.wsl-dns true
```

Everything below is the reasoning, the failure modes, and the safety design. You do not
need it to use the tool.

---

## How it is gated

The `install.sh` block uses **`gff_opt_in`**, not `gff_on` — it runs only when the flag
resolves to exactly `true`. An unset flag, a missing `gff` binary, or a machine where the
flag export never happened all mean **skip**, never "run by default". (`gff_on` is
deliberately fail-open for ordinary steps; an opt-in step that rewrites host DNS must not
inherit that.)

`dig` comes from `dnsutils` in `opt/profiles/packages.tsv`, part of the default core
package set — no flag gates it, and it installs whether or not this feature is enabled.
macOS ships `dig` in `/usr/bin` already, so its BREW column is `-`.

## The problem

From WSL, `ssh <fleet-host>` hangs for ~10s and then dies, while the same host by IP
connects instantly:

```console
$ ssh lab-pi
ssh: Could not resolve hostname lab-pi: Temporary failure in name resolution   # ~10s
$ ssh <user>@10.10.0.21
lab-pi                                                                          # 0.7s
```

WSL2 points `/etc/resolv.conf` at the Windows NAT DNS proxy (typically
`10.255.255.254`). That proxy answers from whatever resolver **Windows** considers
primary — normally the Wi-Fi/Ethernet interface's public or ISP DNS, which has never
heard of your LAN hostnames. The stall is the resolver timeout, not SSH.

> **All addresses and host names in this document are illustrative.** The lab network is
> `10.10.0.0/24`; the other networks use the RFC 5737 documentation ranges (`192.0.2.0/24`,
> `198.51.100.0/24`, `203.0.113.0/24`) so they cannot collide with anything real.

## Architecture: how a name actually gets resolved

This is the part worth internalising, because it dictates the whole design.

**`/etc/resolv.conf` is an ordered list, not a routing table.** glibc's resolver has no
concept of "send `192.168.*` names here and public names there". There is no dispatch by
address range, and none by name either unless you add a `search`/domain suffix scheme —
which does not help for bare single-label names like `lab-pi`.

What actually happens for **every** query, public or private:

```
getent hosts lab-pi
        │
        ├─ nsswitch.conf: "hosts: files dns"
        │      1. /etc/hosts          → no match
        │      2. DNS                 ↓
        │
        └─ /etc/resolv.conf, IN ORDER:
               nameserver 10.10.0.1      ← asked FIRST, for every name
               nameserver 10.255.255.254   ← only on TIMEOUT of the one above
               nameserver 1.1.1.1          ← only on TIMEOUT of the one above
```

Two consequences drive everything else:

1. **The first nameserver answers everything.** `github.com` is sent to `10.10.0.1`
   exactly like `lab-pi` is. The fallback entries are *not* "the ones that handle
   public names".
2. **An NXDOMAIN is a final answer.** glibc falls through to the next `nameserver` only
   on **timeout or network error** — never on a valid negative reply. A resolver in slot
   #1 that serves only local names would answer NXDOMAIN for `github.com`, and the
   fallbacks would never be consulted. Public DNS would be dead.

So the pinned resolver **must be a full recursive resolver** — one that both knows your
local names *and* recurses upstream for everything else. A typical home router
(dnsmasq-style: local DHCP leases + forward the rest to the ISP) is exactly that. The
script proves this before writing anything; see [the recursion guard](#the-recursion-guard).

The fallback entries exist for one job only: when the pinned resolver is **unreachable**
(tunnel down, different network), the query times out and moves on. That is why
`options timeout:1 attempts:1` matters — it caps that failover at about a second instead
of the stock 5s × 2 tries.

## Why the obvious fixes are wrong

**"Just use the default gateway as DNS."** The DNS server that knows your fleet is
frequently *not* on the default route. A worked example from a real machine:

| Windows interface | IP | Its DNS server | Resolves fleet names? |
| :-- | :-- | :-- | :-- |
| Wi-Fi (the default route) | 192.0.2.50 | 198.51.100.53 (ISP) | ❌ NOERROR, no records |
| **wg-lab** (WireGuard) | 10.20.0.5 | **10.10.0.1** | ✅ all of them |
| WSL NAT proxy | — | 10.255.255.254 | ❌ NXDOMAIN |

The default gateway there is `192.0.2.1` — precisely the resolver that does **not** work.
Gateway-based autodetection picks the wrong server every time on a VPN'd machine. This
is why the script probes *every* interface's resolver rather than reasoning about routes.

**"Hardcode the IPs in `~/.ssh/config`."** Works until DHCP moves a host, then breaks
silently.

**"Add them to `/etc/hosts`."** Same staleness problem, and WSL regenerates `/etc/hosts`
unless you disable that too.

## What the script does

1. **Gates on WSL** — `grep -qi microsoft /proc/version`; a no-op anywhere else.
2. **Collects probe hostnames** — every `Host … #fleet` entry in `~/.ssh/config`
   (wildcard patterns skipped), so the probe set follows your ssh config, **minus any
   name already in `/etc/hosts`** (see [Names served by `/etc/hosts`](#names-served-by-etchosts)).
3. **Enumerates candidate resolvers** — `Get-DnsClientServerAddress` over Windows
   interop, across **every** interface. Loopback and link-local addresses are dropped.
4. **Probes each candidate** with `dig +time=2 +tries=1` for each fleet host, and picks
   the one resolving the most. Each candidate is classified as *resolved N/M*,
   *reachable but ignorant*, or *no response* — see
   [Tunnel readiness](#tunnel-readiness).
5. **Verifies the winner recurses** — see below.
6. **Pins the winner** as the first `nameserver`, previous resolvers kept as fallbacks.
7. **Stops WSL overwriting it** — `generateResolvConf = false` in `/etc/wsl.conf`.
8. **Verifies** via `getent hosts` that the real resolver path now works.

Resulting `/etc/resolv.conf`:

```
# Managed by dotfiles opt/scripts/system/wsl_dns_lan.sh — do not edit.
# Regenerate: ~/opt/scripts/system/wsl_dns_lan.sh
# WSL's generated resolv.conf is disabled via /etc/wsl.conf (generateResolvConf).
options timeout:1 attempts:1
nameserver 10.10.0.1
nameserver 10.255.255.254
nameserver 1.1.1.1
```

### The recursion guard

Before writing, the script asks the winning resolver for a public sentinel name
(`github.com` by default). If it cannot answer, the script **refuses to change anything**:

```console
wsl-dns: WARNING — 10.10.0.1 resolves fleet hosts but NOT github.com.
wsl-dns: WARNING — Pinning it first would break ALL public DNS: every query goes to
         nameserver #1, and its NXDOMAIN is a final answer — the fallback nameservers
         are only tried on timeout.
wsl-dns: WARNING — Refusing to change DNS. Override with WSL_DNS_ALLOW_NONRECURSIVE=1 …
```

This is the guard against the failure mode described in the architecture section. Point
`WSL_DNS_PUBLIC_PROBE` at a different name if `github.com` is not a fair test on your
network, or set `WSL_DNS_ALLOW_NONRECURSIVE=1` if you genuinely want a local-only
resolver in slot #1 and understand that public DNS will break.

### Names served by `/etc/hosts`

`nsswitch.conf` is `files dns`, so anything in `/etc/hosts` is answered **before any
resolver is consulted**. The local hostname is the usual case — WSL writes it into
`/etc/hosts` itself:

```
127.0.1.1	myhost.localdomain	myhost
```

Such a name is deliberately **excluded from the DNS probe**, and the script says so:

```
wsl-dns: not probing myhost — already served by /etc/hosts (files precedes dns).
```

Two reasons. First, a LAN/VPN resolver returns NXDOMAIN for the local hostname forever,
so probing it would cap the score below 100% for no reason (`3/4` when the resolver is
actually perfect). Second — and worse — `--verify` would count it as a fleet *hit*: it
resolves in 0s even with the tunnel down, because `/etc/hosts` answered, not the resolver
under test. That is a false positive about the very thing being verified.

Such names keep resolving no matter what gets pinned, because `files` runs first:

```console
$ getent ahostsv4 myhost
127.0.1.1       STREAM myhost.localdomain
$ python3 -c "import socket; print(socket.gethostbyname('myhost'))"
127.0.1.1
```

(Note `getent hosts <name>` — without `ahostsv4` — can take an AAAA-first path and fall
through to DNS, which is misleading. `getent ahostsv4` and `gethostbyname` show what
applications like `ssh` actually get.)

### Tunnel readiness

A candidate that answers *nothing* is a different failure from one that answers but does
not know your fleet, so the two are reported differently:

```
candidate 203.0.113.1:  NO RESPONSE — not reachable from this network.
candidate 10.10.0.1:  resolved 3/3 fleet host(s).
candidate 198.51.100.53:  NO RESPONSE — not reachable from this network.
```

The per-candidate wording is neutral on purpose: with a **full-tunnel** VPN up, the
ISP's resolvers become unroutable and go silent — reporting that as "tunnel down" would
be exactly backwards.

The readiness diagnosis therefore fires only when **not one** candidate responds, even
for a public name:

```
WARNING — NOT ONE of the 4 configured resolvers responded, even for a public name.
WARNING — TUNNEL LIKELY NOT READY: a VPN adapter and its DNS server appear the moment
          you click connect, seconds before the handshake finishes, and the old network
          is already unroutable by then. Wait for it to finish connecting, then re-run.
```

This is a real race, not a theoretical one: Windows attaches the VPN adapter and
publishes its DNS server immediately on connect, while WireGuard's handshake takes a
further few seconds — and the pre-existing network is already unroutable in that window.
Running the script in that gap yields `0/N` on every candidate. The fix is simply to wait
for the tunnel to finish and re-run; nothing is written in the meantime.

## Reverting

The script snapshots `/etc/resolv.conf` (including whether it was a symlink and where it
pointed) and `/etc/wsl.conf` into `/etc/wsl_dns_lan.backup` **before its first write**,
and refuses to change anything if that snapshot cannot be written. The snapshot is taken
**once** — re-running never overwrites a good snapshot with the managed files.

```sh
~/opt/scripts/system/wsl_dns_lan.sh --revert
```

restores both files exactly and removes the snapshot. If the snapshot is missing,
`--revert` still repairs the machine by restoring WSL's stock layout:
`/etc/resolv.conf -> /mnt/wsl/resolv.conf`, `generateResolvConf` dropped from `wsl.conf`,
and a `[network]` section left empty by that removal deleted. Other sections are left
untouched. Run `wsl.exe --shutdown` from Windows afterwards for a fully clean slate.

The test suite covers a full write → re-run → revert round trip and asserts both files
come back byte-for-byte identical to the originals.

## Verifying it works: the tunnel-up / tunnel-down matrix

The automated tests stub DNS, so they prove the *logic*, not your actual network. For
that, `--verify` runs the real matrix through `getent` — deliberately **not** `dig`,
because `dig` bypasses `resolv.conf` ordering and would prove nothing about what `ssh`
will experience.

**The manual procedure — run it twice:**

```sh
# 1. with the VPN/tunnel UP
~/opt/scripts/system/wsl_dns_lan.sh --verify

# 2. disconnect the tunnel, then again
~/opt/scripts/system/wsl_dns_lan.sh --verify
```

**What each state must show:**

| | `github.com` | fleet names | Verdict |
| :-- | :-- | :-- | :-- |
| **Tunnel up** | resolves | resolve | PASS |
| **Tunnel down** | resolves | miss, **within `MAX_FAIL_SECONDS`** | PASS |
| Either state | does **not** resolve | — | FAIL — public DNS broken |
| Tunnel down | resolves | miss, but slowly | FAIL — the original stall regressed |

The public name resolving in **both** states is the key invariant: it is what proves the
pinned resolver recurses and that the fallbacks are intact. Fleet names resolving only
with the tunnel up is expected, not a failure — what *would* be a failure is a slow miss,
because that is the 10–20s stall this whole script exists to remove.

Real output from a machine with the tunnel **down** and the fix **not yet applied** —
`--verify` reproducing the original bug as a measurement:

```console
$ wsl_dns_lan.sh --verify
wsl-dns: verify — pinned resolver: 10.255.255.254 (serves fleet names: down)
wsl-dns: verify — NAME RESULT SECONDS
  github.com OK 0s
  lab-pi MISS 20s
wsl-dns: WARNING — lab-pi took 20s to fail (limit 3s) — the resolver timeout tuning regressed.
wsl-dns: verify — fleet resolver unreachable: public OK, but a miss exceeded 3s (see the warnings above).
wsl-dns: verify — that slow miss IS the bug this script fixes; run it without --verify to pin a resolver.
wsl-dns: verify — FAIL
```

Tunnel state is detected by asking the pinned resolver for a **fleet** name, not the
public one — the stock WSL proxy answers `github.com` quite happily and would otherwise
masquerade as a working fleet resolver.

## Safety properties

- **Non-fatal throughout.** Not WSL, no `#fleet` hosts, no `dig`, no candidate resolvers,
  no winner, a non-recursive winner, or no root — each warns and exits 0. `install.sh` is
  never broken by it.
- **Writes only on success.** If no candidate resolves a fleet host, nothing is touched:

  ```console
  $ wsl_dns_lan.sh --dry-run          # with the tunnel down
  wsl-dns: candidate 198.51.100.53: resolved 0/4 fleet host(s).
  wsl-dns: WARNING — no candidate resolver answered for any of: lab-pi …
  wsl-dns: WARNING — if this needs a VPN/tunnel, bring it up and re-run: …
  ```

- **No write without an undo path.** A failed snapshot aborts the run.
- **Idempotent.** A no-op run compares against what is on disk and does not even prompt
  for sudo.

## Options

| Flag | Effect |
| :-- | :-- |
| `--dry-run` | Probe, report the winner, print both files, write nothing. |
| `--verify` | Live self-check across the tunnel-up / tunnel-down matrix. Exits 1 on a failed expectation. |
| `--revert` | Restore the snapshot and exit. |
| `--quiet` | Suppress informational output; warnings still print. |
| `--help` | Print the script's header block. |

| Environment variable | Effect |
| :-- | :-- |
| `WSL_DNS_PROBE_HOSTS` | Hostnames to probe, bypassing the ssh-config scan. |
| `WSL_DNS_SERVERS` | Candidate resolvers to try **first**, before the Windows list. |
| `WSL_DNS_FALLBACKS` | Fallback resolvers (default: current ones, then `1.1.1.1`). |
| `WSL_DNS_SSH_CONFIG` | ssh config to scan (default `~/.ssh/config`). |
| `WSL_DNS_HOSTS_FILE` | Hosts file consulted before DNS (default `/etc/hosts`). |
| `WSL_DNS_DIG` | `dig` binary to probe with (default: `dig` from `PATH`). |
| `WSL_DNS_PUBLIC_PROBE` | Public name proving recursion (default `github.com`). |
| `WSL_DNS_ALLOW_NONRECURSIVE` | `1` pins a local-only resolver anyway. Breaks public DNS. |
| `WSL_DNS_BACKUP_DIR` | Snapshot location (default `/etc/wsl_dns_lan.backup`). |
| `WSL_DNS_MAX_FAIL_SECONDS` | `--verify`: longest acceptable failed lookup (default `3`). |
| `WSL_DNS_WSL_CONF`, `WSL_DNS_RESOLV_CONF`, `WSL_DNS_NO_SUDO`, `WSL_DNS_GETENT` | Testing hooks: retarget the files, skip sudo, stub resolution. |

## Marking hosts as fleet

The probe set comes from the `#fleet` marker already used by the `ssh-host-finder` skill:

```sshconfig
Host lab-pi  #fleet
    Hostname lab-pi
    User <user>
    IdentityFile ~/.ssh/id_ed25519_homelab
```

Note `Hostname` stays the **name**, not an IP — that is the point: resolution is fixed
centrally instead of pinning addresses per host.

## Related

- Requires `dig` — `dnsutils` in [`opt/profiles/packages.tsv`](../opt/profiles/packages.tsv).
  The `install.sh` block runs after the package step for that reason.
- Script + companion test: [`opt/scripts/system/`](../opt/scripts/system/AGENTS.md).
- Flag registry: [`.github/gff/features.yaml`](../.github/gff/features.yaml).
