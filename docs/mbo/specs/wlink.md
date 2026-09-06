# wlink — WSL link (tunnel + resolver) management — spec

- **Slug:** `wlink`
- **Date:** 2026-08-24
- **Status:** Approved
- **Relates to:** design [`../designs/wlink.md`](../designs/wlink.md) · PR
  [#242](https://github.com/sfc-gh-eraigosa/dotfiles/pull/242) (shell predecessor)

## 1. Goal

One command answers *"can this WSL box reach my fleet by name, and if not, why?"* — and the
same tool fixes the common cause by pinning a resolver that actually knows those names,
reversibly. It replaces `opt/scripts/system/wsl_dns_lan.sh` with a Go CLI that adds the three
things the script cannot do well: structured state (`--json`), native DNS (no `dig`
dependency), and reuse of `fleet`'s `#fleet` contract instead of a second ssh-config parser.

## 2. Use cases

### UC-1 — "ssh to my Pi hangs for 20 seconds"
- **Actor:** developer on WSL, fleet reachable only over a WireGuard tunnel.
- **Trigger:** `ssh lab-pi` stalls ~20s, then *Temporary failure in name resolution*.
- **Flow:** `wlink status` → reports the tunnel up but the resolver unpinned → `wlink pin`
  probes every per-interface resolver Windows knows, picks the one resolving fleet names,
  snapshots, writes.
- **Acceptance:** after `pin`, fleet names resolve in <1s; public DNS still works; `wlink unpin`
  restores the previous files byte-for-byte.

### UC-2 — "I just clicked connect and nothing works"
- **Actor:** same, mid-handshake.
- **Trigger:** `wlink pin` (or `status`) run seconds after connecting.
- **Flow:** no candidate answers anything, including a public sentinel → `wlink` reports
  `tunnel: not-ready` rather than a bare "0 candidates", and writes nothing.
- **Acceptance:** `status` distinguishes `not-ready` from `down`; `wlink wait --ready`
  blocks until the handshake completes or times out, exit 0/1 respectively.

### UC-3 — "git randomly hangs forever when the VPN is up"
- **Actor:** same.
- **Trigger:** `git fetch` sits with TCP `ESTABLISHED` and no bytes moving.
- **Flow:** `wlink doctor` reports ssh has no keepalive configured
  (`ServerAliveInterval 0`, `ConnectTimeout none`), so a transient tunnel blip becomes an
  infinite hang; `--fix` offers the config stanza.
- **Acceptance:** `doctor` flags it on a machine that lacks it, stays silent on one that has it,
  and `--fix` is idempotent and never rewrites unrelated ssh config.

### UC-4 — "is my link healthy?" (scripted / status line)
- **Actor:** `gsl` status line, or CI.
- **Trigger:** `wlink status --json`.
- **Flow:** one JSON document: tunnel identity + readiness, chosen resolver, pin state, drift,
  fleet reachability counts.
- **Acceptance:** stable schema, documented exit codes, no interactive prompts, completes fast
  enough for a status line (budgeted in §5, EC-9).

### UC-5 — "prove it works in both tunnel states"
- **Actor:** developer validating a change.
- **Trigger:** `wlink verify`, run once with the tunnel up and once down.
- **Flow:** resolves a public sentinel and every fleet name **through the real resolver path**
  (Go's host resolver / `getent` semantics — never a direct-to-server query, which would bypass
  `resolv.conf` ordering and prove nothing).
- **Acceptance:** public resolves in **both** states; fleet names only when up; a miss completes
  inside a budget derived from the live `resolv.conf`.

## 3. Architecture

```
                      ┌──────────────┐
   fleet discover ───▶│  fleetsrc    │──┐        (#fleet hosts, /etc/hosts exclusion)
   (--json contract)  └──────────────┘  │
                                        ▼
   powershell.exe ───▶┌──────────────┐  ┌──────────────┐      ┌────────┐
   (one Runner seam)  │   winhost    │─▶│  linkstate   │─────▶│  cmd/  │─▶ human | --json
                      └──────────────┘  └──────────────┘      └────────┘
                                        ▲          │
   native Go DNS ────▶┌──────────────┐  │          ▼
                      │    probe     │──┘   ┌──────────────┐
                      └──────────────┘      │  resolvconf  │ (snapshot/write/restore)
                                            └──────────────┘
```

Each box is independently testable: `winhost` behind a `Runner` interface fed by recorded
fixtures, `probe` against a local test DNS server, `resolvconf` against temp files, `fleetsrc`
against a stub `fleet` binary and fixture ssh configs.

**Boundary with `fleet`:** `fleet` owns *which hosts exist* (`#fleet` blocks, keys, wake).
`wlink` consumes `fleet discover --json` and never writes ssh-config host blocks. If `fleet` is
absent, `fleetsrc` falls back to parsing `#fleet` markers read-only.

## 4. Behavior / features

Carried over from the shell script (all validated in #242):

| # | Feature |
| :-- | :-- |
| F1 | Probe **every** per-interface DNS server Windows knows — not the default gateway, which is provably the wrong one on a VPN'd machine |
| F2 | Score candidates by fleet names resolved; highest wins |
| F3 | **Recursion guard** — refuse to pin a resolver that cannot answer a public name, because `resolv.conf` is an ordered list and an NXDOMAIN from nameserver #1 is final |
| F4 | Pin winner first in `resolv.conf` with prior resolvers as fallbacks + `options timeout:1 attempts:1` |
| F5 | `generateResolvConf = false` in `wsl.conf`, preserving unrelated sections |
| F6 | Snapshot before first write; **refuse to write if the snapshot fails**; `unpin` restores byte-for-byte |
| F7 | Exclude names already served by `/etc/hosts` (`files` precedes `dns`) from the DNS probe |
| F8 | Tunnel-readiness diagnosis when nothing responds at all |
| F9 | `verify` — the tunnel up/down matrix with a budget derived from the live `resolv.conf` |
| F10 | Dry-run that writes nothing |

New in `wlink`:

| # | Feature | Why |
| :-- | :-- | :-- |
| F11 | **Native Go DNS** — drops `dig`/`dnsutils` entirely | Removes a dependency that silently no-ops on nix-managed hosts |
| F12 | **`status`** — tunnel identity, handshake freshness, resolver, pin state, drift, fleet counts | Today every answer needs a manual `--verify` or reading files by hand |
| F13 | **`--json` on every read command**, stable schema, documented exit codes | `gsl` status line + CI gating |
| F14 | **`doctor`** — ssh keepalive gap, missing snapshot, `resolv.conf` drift, full-tunnel routing, absent fleet source | UC-3; the checks that predict outages |
| F15 | **`wait --ready`** — block until the tunnel handshake completes | UC-2; the race hit on the first real run |
| F16 | **`fleet discover --json` as the host source** | Kills the duplicate `#fleet` parser |
| F17 | **Drift detection** — notice when `resolv.conf` was edited out from under us | Today a hand-edit is invisible until something breaks |
| F18 | **Shell completion** (cobra, free) | Consistency with the other sdk tools |

Usability decisions worth stating: **no interactive prompts** (scriptable by default);
**`pin` is idempotent** and prompts for sudo at most once; **every mutating command has an
inverse**; **human output is the default, `--json` is opt-in**; **exit codes are part of the
contract**, not incidental.

## 4.1 Worked example — what you actually end up with

> **Illustrative, not captured.** `wlink` does not exist yet, so everything below is *target*
> output describing the intended shape — it is the spec's picture of "done", and the tests in §5
> are what hold the implementation to it. This is the opposite of the `sdk/README.md` rule, where
> a demo **must** be real captured output; that rule applies once the tool ships.
>
> All names and addresses are fictitious. The lab network is `10.10.0.0/24`; the other networks
> use the RFC 5737 documentation ranges so they cannot collide with anything real.

### The scenario

A WSL box on a café network (`192.0.2.0/24`), reaching a lab network (`10.10.0.0/24`) over a
WireGuard tunnel. The lab router at `10.10.0.1` serves both the lab's DHCP names and recursion
for everything else. The café's ISP resolver knows nothing about the lab.

| Windows interface | Address | Its DNS server | Knows lab names? |
| :-- | :-- | :-- | :-- |
| Wi-Fi (**the default route**) | `192.0.2.50` | `198.51.100.53` | ❌ |
| `wg-lab` (WireGuard) | `10.20.0.5` | **`10.10.0.1`** | ✅ |
| Bluetooth PAN | — | `203.0.113.1` | ❌ |
| WSL NAT proxy | — | `10.255.255.254` | ❌ |

Note the trap this exists to avoid: the **default route's** resolver is the one that does *not*
work.

### Input 1 — `~/.ssh/config` (owned by `fleet`, read by `wlink`)

```sshconfig
Host lab-pi  #fleet
    Hostname lab-pi
    User <user>

Host lab-nas  #fleet
    Hostname lab-nas
    User <user>

Host lab-jetson  #fleet
    Hostname lab-jetson
    User <user>
```

`Hostname` stays a **name**, never an IP — that is the point. Resolution is fixed centrally
instead of pinning addresses per host.

### Input 2 — the feature flag (§3.1 of the plan)

```sh
gff set install.sdk.wlink true
```

### `wlink status` — before pinning

```console
$ wlink status
link:      DEGRADED
tunnel:    up          wg-lab (handshake 34s ago)
resolver:  unpinned    current: 10.255.255.254 (WSL NAT proxy)
fleet:     0/3 resolvable      (lab-pi, lab-nas, lab-jetson)
hint:      run `wlink pin` — 10.10.0.1 answers for all 3
```

### `wlink pin`

```console
$ wlink pin
not probing selfhost — already served by /etc/hosts (files precedes dns)
candidate 203.0.113.1:    NO RESPONSE — not reachable from this network
candidate 10.10.0.1:      resolved 3/3 fleet host(s)
candidate 198.51.100.53:  reachable, resolved 0/3 fleet host(s)
selected 10.10.0.1 (3/3 fleet hosts)
10.10.0.1 also resolves github.com; safe to pin first
snapshotted the previous DNS config to /etc/wlink.backup
pinned 10.10.0.1 in /etc/resolv.conf; /etc/wsl.conf keeps WSL from overwriting it
verified: lab-pi -> 10.10.0.21
undo any time: wlink unpin
```

### Output 1 — `/etc/resolv.conf`

```
# Managed by wlink — do not edit.
# Regenerate: wlink pin   ·   Undo: wlink unpin
# WSL's generated resolv.conf is disabled via /etc/wsl.conf (generateResolvConf).
options timeout:1 attempts:1
nameserver 10.10.0.1
nameserver 10.255.255.254
nameserver 198.51.100.53
```

The winner is **first** because nameserver #1 answers every query (§3). The rest are fallbacks
for when the tunnel is down — reached on **timeout**, never on an NXDOMAIN. `timeout:1
attempts:1` caps that failover at about a second.

### Output 2 — `/etc/wsl.conf`

Only the one key is added; unrelated sections are preserved verbatim:

```ini
[boot]
systemd=true

[user]
default=<user>

[network]
generateResolvConf = false
```

### `wlink status --json` — the contract `gsl` consumes

```json
{
  "wsl": true,
  "link": "degraded",
  "tunnel": { "state": "up", "interface": "wg-lab", "handshake_age_seconds": null },
  "pinned": { "resolver": "10.10.0.1", "since": "2026-08-24T21:40:11Z", "managed": true },
  "candidates": [
    { "server": "10.10.0.1",      "reachable": true,  "fleet_resolved": 3, "recursive": true },
    { "server": "198.51.100.53",  "reachable": true,  "fleet_resolved": 0, "recursive": true },
    { "server": "203.0.113.1",    "reachable": false, "fleet_resolved": 0, "recursive": false }
  ],
  "fleet": { "total": 3, "resolved": 3, "excluded_by_hosts_file": ["selfhost"] },
  "drift": null
}
```

`handshake_age_seconds` is **always `null` today** — the WireGuard handshake clock is not read
yet. It is documented so the shape is stable, not because a value is available.

`link` is the one-word verdict (`ok` | `degraded`) and is what drives the exit code, so a
consumer reads one field instead of re-deriving the rules. Empty collections are emitted as
`[]`, never `null`, so `candidates.length` is always meaningful; absences (`pinned`, `drift`)
*are* `null`, because "not pinned" and "pinned to nothing" must not look alike.

### `wlink doctor`

```console
$ wlink doctor
[warn] ssh has no keepalive for github.com (ServerAliveInterval 0, ConnectTimeout none)
       a stalled connection over the tunnel hangs forever instead of failing
       fix: wlink doctor --fix   (adds ServerAliveInterval 20 / ServerAliveCountMax 3)
[ok]   resolv.conf matches what wlink wrote (no drift)
[ok]   snapshot present at /etc/wlink.backup — unpin will restore
[ok]   pinned resolver 10.10.0.1 answers for public names
2 checks passed, 1 finding
```

### Off-network — the same box with the tunnel down

```console
$ wlink status
link:      DEGRADED
tunnel:    down
resolver:  pinned 10.10.0.1 (unreachable — falling through to 10.255.255.254)
fleet:     0/3 resolvable
note:      public DNS unaffected; fleet misses fail in ~1s, not ~20s
```

Nothing is rewritten when the tunnel drops. The pin stays, the fallbacks carry public DNS, and
fleet lookups fail *fast* — which is the whole point of `options timeout:1 attempts:1`.

## 5. Evaluation criteria (per feature)

Format: *trigger predicate · fires · must-not-fire · edge · pass*.

| ID | Feature | Rule |
| :-- | :-- | :-- |
| EC-1 | F1/F2 | Given fixtures where a non-default-route interface resolves all fleet names and the default gateway resolves none → that interface's server is selected. **Must not** select by route metric. Edge: two candidates tie → deterministic (first by enumeration order). |
| EC-2 | F3 | Candidate resolves fleet names but NXDOMAINs a public sentinel → **nothing is written**, exit 0, warning names the reason. Must not fire when the candidate answers the sentinel. Edge: `--allow-nonrecursive` overrides and says so loudly. |
| EC-3 | F4/F5 | Rendered `resolv.conf` has winner first, prior resolvers after, `options timeout:1 attempts:1`. `wsl.conf` gains `generateResolvConf = false` across all five INI shapes (key present/absent, `[network]` present/absent/last, empty file) with other sections untouched. |
| EC-4 | F6 | Snapshot exists before the first byte is written. Simulated snapshot failure → **no write**, exit 0, explicit warning. `unpin` restores `resolv.conf` (symlink target included) and `wsl.conf` byte-for-byte. Re-running `pin` must **not** overwrite a good snapshot. |
| EC-5 | F7 | A probe host present in `/etc/hosts` is excluded and announced; score reflects only DNS-resolvable hosts (`1/1`, not `1/2`). Must not exclude a name absent from `/etc/hosts`. |
| EC-6 | F8/F15 | All candidates silent (including for the sentinel) → `not-ready`, not `down`. Some answer but none knows the fleet → **not** reported as a tunnel problem. `wait --ready` returns 0 when a candidate starts answering, 1 on timeout. |
| EC-7 | F9 | `verify` passes when public resolves and fleet resolves (tunnel up); passes when public resolves and fleet misses **within budget** (tunnel down); fails when public does not resolve in either state; fails when a miss exceeds budget. Budget = `nameservers × timeout × 2 families + 1`. |
| EC-8 | F11 | Native resolver reproduces recorded `dig` outcomes for NXDOMAIN, no-response, and NOERROR-no-data. Edge: server reachable but returns SERVFAIL → treated as "reachable, unhelpful", not "silent". |
| EC-9 | F12/F13 | `status --json` validates against the documented schema; exit code 0 healthy, 1 degraded, 2 usage error. Completes within a fixed budget on a fixture-backed run so it is safe in a status line. |
| EC-10 | F14 | `doctor` flags ssh with `ServerAliveInterval 0` for the git host; silent when set. `--fix` is idempotent and touches only the block it owns. Must not report a finding it cannot explain in one line. |
| EC-11 | F17 | `resolv.conf` edited after `pin` → drift reported by `status`/`doctor` with what changed. Must not report drift for a byte-identical file. |
| EC-12 | all | Non-WSL host → every command exits 0 as a no-op with one clear line. Must never write on a non-WSL host. |
| EC-13 | F2 | Wildcard `Host` patterns (`*`, `?`, `!`) in the ssh config are **never probed** — they are not resolvable names. A `Host *` block carrying the fleet marker contributes nothing. |
| EC-14 | F1 | Candidate filtering: loopback (`127.*`), link-local (`169.254.*`), and `0.0.0.0` are dropped before probing. Duplicates across interfaces are de-duplicated, preserving first-seen order. |
| EC-15 | F2 | Zero probe hosts (no fleet markers, no override) → clean no-op: one explanatory line, no writes, exit 0. Must not be reported as an error. |
| EC-16 | F6 | `unpin` **with no snapshot** still repairs the machine: restore WSL's stock layout (`/etc/resolv.conf` → symlink to `/mnt/wsl/resolv.conf`), drop `generateResolvConf` from `wsl.conf`, remove a `[network]` section left empty by that removal, and leave every other section byte-identical. |
| EC-17 | F4 | `pin` must replace the `resolv.conf` **symlink** with a real file — WSL ships it as a symlink to the distro-shared `/mnt/wsl/resolv.conf`, so writing through it would leak the pin to other distros and be regenerated. `unpin` restores the symlink *and its original target*. |
| EC-18 | F6 | After a successful `unpin`, the snapshot directory is removed, so a subsequent `pin` takes a fresh snapshot of the genuinely-current state rather than restoring a stale one. |
| EC-19 | §3 CLI | Unknown flags/arguments exit **2** (usage error), distinct from a safe decline (0) and a real failure (1). |
| EC-22 | F4 | Fallbacks are **the resolvers already in `resolv.conf`** — wlink does not append a hardcoded public resolver. The prototype appended `1.1.1.1`; that is deliberately dropped, because on WSL the NAT proxy (`10.255.255.254`) is the Windows host and is effectively always present, so the third entry is unreachable-in-practice while silently directing a user's DNS to a third party. A public fallback is available opt-in via `WLINK_FALLBACKS`. Rendered output is still capped at glibc's `MAXNS` (3). |
| EC-21 | F7/F16 | A fleet entry whose `Hostname` is **already an IP address** is excluded from the probe set and reported as excluded, not probed. DNS has nothing to resolve for it and pinning a resolver would not change it, so probing would penalise every candidate for a name no resolver was ever asked about — the same false-cap the `/etc/hosts` exclusion (EC-5) prevents. The **`Hostname`** is probed, never the `Host` alias: `Hostname` is what ssh actually resolves. |
| EC-20 | F12/F13 | `link` is `degraded` **only** when a non-empty fleet is not fully resolvable, or drift is present. Tunnel state and pin status are **reported, not health inputs**: a machine sitting directly on the fleet's LAN resolves everything with no tunnel and no pin, and calling that degraded is the tool crying wolf about a link that plainly works. Off WSL it is `ok` (a no-op, not a failure). An **empty** fleet is `ok` — nothing to resolve is not a shortfall. Empty collections marshal as `[]`, absences (`pinned`, `drift`, `handshake_age_seconds`) as `null`. |

### 5.1 Provenance — the prototype's behavioral inventory

EC-1…EC-19 are not invented. They are the distilled inventory of the **54 cases** the shell
prototype (`opt/scripts/system/wsl_dns_lan_test.sh`) proved on real hardware before it was
deleted in P15. That prototype existed to discover the behavior; this table exists so the
discovery outlives it.

The rules that came *only* from running the prototype against a live tunnel — the ones nobody
would have written from first principles — are worth calling out, because they are exactly what
a from-scratch Go implementation would get wrong:

| Rule | Why it exists | How it was found |
| :-- | :-- | :-- |
| EC-1 | The default route's resolver is often the **wrong** one | A VPN'd machine where the gateway resolved nothing local and a secondary interface resolved everything |
| EC-2 | `resolv.conf` is an ordered list; an NXDOMAIN from #1 is **final** | Realising a local-only resolver in slot #1 would silently kill public DNS with no fallback |
| EC-5 | The local hostname is in `/etc/hosts` and no DNS server will ever answer for it | A permanently-capped score (`3/4`) that looked like a resolver defect and was not |
| EC-6 | A VPN adapter and its DNS server appear **before** the handshake completes | Probing seconds after clicking connect returned `0/N` on every candidate |
| EC-7 | The failure budget must be **derived**, not guessed | A hardcoded 3s limit failed a run that was actually a 5× improvement |
| EC-16 | The `resolv.conf` symlink may already be gone when a repair is needed | Designing the undo path for the case where the snapshot itself is missing |
| EC-17 | WSL's `resolv.conf` is a symlink into a **distro-shared** mount | Writing through it would leak the pin across distros and be regenerated |

**Because the prototype is deleted, this section is load-bearing.** A behavior that is not
captured as an EC rule here is a behavior that will be lost — so anything discovered while
building `wlink` that the prototype had handled must be **added here**, not just fixed in code.

## 6. Verification harness

| Layer | Covers | Gate |
| :-- | :-- | :-- |
| **Unit (fixtures)** | EC-1…EC-8, EC-11, EC-12 — `winhost` fed recorded PowerShell output; `probe` against a local in-process DNS server; `resolvconf` against temp files | `go test ./...`, module coverage **≥60%** (the `sdk/` floor in `scripts/test.sh`) |
| **Golden files** | Rendered `resolv.conf` / `wsl.conf` for all five INI shapes | byte-exact comparison |
| **Round-trip** | EC-4 — `pin` → re-run → `unpin` restores both files byte-for-byte | temp-dir integration test, no privileged writes |
| **Rule coverage** | Every EC-1…EC-19 rule has a named Go test (§5.1 explains their provenance) | traceability table in the plan §5 |
| **Live acceptance** (human-evidenced) | EC-6 readiness during an actual handshake; EC-7 both tunnel states; EC-10 on a real ssh config; timing vs the recorded **20–21s → 4s** baseline | captures committed under `plans/wlink/evidence/` |

Windows interop cannot run in CI; that is precisely why it lives behind `winhost.Runner`, with
fixtures in CI and the live checklist covering the rest.

## 7. Prerequisites / dependencies

- **Shared logging via `sdk/libs/log`** (`applog.SetDefaultTool("wlink")`; diagnostics to
  `Default`, shelled-out process output to `NewCapture`; `$WLINK_LOG_FILE` /
  `$WLINK_LOG_LEVEL`). No hand-rolled logger, writer, or rotation — same contract as `fleet`,
  `gsl`, and `tmux-mgr`.
- Go module conventions from [`sdk/AGENTS.md`](../../../sdk/AGENTS.md) — including its
  **"Adding a module"** checklist (build.sh + `version.sh`, `libs/log`, `AGENTS.md` +
  `CLAUDE.md` symlink, `README.md`, `install.sh` wiring, **both** module tables, a
  `sdk/README.md` section in the house shape, and confirming `git status` tracking).
- `sdk/fleet`'s `fleet discover --json` output contract (consumed, not imported — `fleet` has
  no `pkg/`, only `internal/`).
- `powershell.exe` reachable — via `PATH` or the absolute fallback, since interop `PATH`
  entries can be missing on an otherwise healthy WSL.
- One fail-closed gff flag, **`install.sdk.wlink`** (`boolDefault: false`, `gff_opt_in`), gating
  both the build/install and the install-time pin (plan §3.1). The tool is only needed on
  machines reaching their fleet over a VPN/tunnel, so a machine that does not ask for it never
  builds it. The prototype's `install.system.wsl-dns` never reaches `main` — it is replaced
  inside PR #242 rather than migrated (plan §3.1).
- **No `dig`/`dnsutils`** — removing that dependency is F11. `dnsutils` stays in
  `packages.tsv` on its own merit as a core diagnostic, not as a `wlink` prerequisite.

## 8. Out of scope (and why)

| Excluded | Why |
| :-- | :-- |
| Connecting/disconnecting the VPN | `wlink` observes; the WireGuard client owns lifecycle. Managing it means credential handling and a much larger blast radius. |
| Split-tunnel `AllowedIPs` changes | Explicitly declined during #242 testing — the keepalive fix addresses the git hangs without restructuring the network. |
| Cross-platform parity | Irreducibly WSL-shaped. Must degrade cleanly, not pretend. |
| A daemon / background watcher | On-demand only, like `fleet`. `wait --ready` is a foreground block, not a service. |
| Replacing `fleet` | Different question: `fleet` = who my hosts are; `wlink` = can I reach them. |
| Editing `/etc/hosts` | Pinning a resolver is reversible and central; per-host entries go stale silently — the failure mode this whole objective exists to remove. |

## 9. Rollback

Per machine: `wlink unpin`. Failing that, delete `/etc/resolv.conf`, drop `generateResolvConf`
from `/etc/wsl.conf`, `wsl.exe --shutdown`. Repo-wide: the gff flag defaults false, so an
un-enabled machine is unaffected; reverting the module means one revert plus un-archiving
`wsl_dns_lan.sh`. Pre-merge: decline the objective — #242 is already the shipped answer.
