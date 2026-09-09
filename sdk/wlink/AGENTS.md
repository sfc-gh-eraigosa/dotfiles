# wlink — WSL link (tunnel + resolver) management

> **Up:** [`sdk/`](../AGENTS.md) · user-facing tour [`sdk/README.md`](../README.md#-wlink--why-does-ssh-hang-but-the-ip-work) ·
> objective [`docs/mbo/`](../../docs/mbo/index.md) slug `wlink` (design · spec · plan · trio)

**wlink = WSL link** — the link between this WSL box and the private network its
fleet lives on: the tunnel carrying it, the resolver that makes its names
resolvable, and whether it is currently usable. Pairs with
[`fleet`](../fleet/AGENTS.md): *fleet* answers who my hosts are, *wlink* answers
whether I can reach them from here.

## Commands

| Command | Does | Exit |
| :-- | :-- | :-- |
| `wlink status [--json]` | tunnel state, pinned resolver, fleet reachability, drift | 0 healthy · 1 degraded |
| `wlink pin [--dry-run] [--allow-nonrecursive]` | probe every resolver Windows knows, pin the one that answers for the fleet | 0 pinned **or safely declined** · 1 write failed |
| `wlink unpin` | restore the snapshot exactly, symlink target included | 0 · 1 restore failed |
| `wlink verify [--max-fail-seconds N]` | the tunnel up/down matrix, through the host resolver | 0 met · 1 failed |
| `wlink wait --ready [--timeout D]` | block until the fleet resolves | 0 ready · 1 timed out |
| `wlink doctor [--fix]` | conditions that predict outages (ssh keepalive, snapshot, drift) | 0 clean · 1 findings |

Unknown flags exit **2**. A *safe decline* is exit **0** — not WSL, Windows
unreachable, no fleet names, no winner, guard tripped, or no undo path.
`install.sh` must never fail because a tunnel happened to be down.

## The five things that are easy to get wrong

Each cost real debugging time; each is pinned by a test.

1. **The default route's resolver is usually the wrong one.** On a VPN'd
   machine the gateway resolves nothing local while a secondary interface
   resolves everything. `wlink` probes *every* interface rather than reasoning
   about routes (spec EC-1).
2. **`resolv.conf` is an ordered list, not a routing table.** Nameserver #1
   answers every query and its NXDOMAIN is **final** — glibc falls through only
   on timeout. Pinning a local-only resolver first kills public DNS with no
   fallback, so the recursion guard refuses it (EC-2).
3. **`/etc/resolv.conf` is a symlink into `/mnt/wsl`, which is shared across
   distros.** Writing *through* it leaks the pin to every distro and gets
   regenerated. `pin` replaces the link with a real file (EC-17).
4. **A name in `/etc/hosts` is answered before any resolver is consulted**
   (`nsswitch: files dns`). Probing it caps the score forever and lets `verify`
   count a `/etc/hosts` hit as evidence the *resolver* works (EC-5).
5. **Attached ≠ ready.** Windows publishes a VPN adapter and its DNS server the
   moment you click connect, seconds before the handshake completes, and the old
   network is already unroutable — so every candidate goes silent and it looks
   like "no tunnel" (EC-6).

## Layout

| Path | Owns |
| :-- | :-- |
| `cmd/` | cobra verbs, exit codes, human vs `--json` rendering, the `Runtime` that wires everything |
| `internal/winhost/` | **the only place that talks to Windows.** All interop behind `Runner`, so the rest is testable in CI |
| `internal/probe/` | native DNS (no `dig`), candidate scoring, the recursion guard |
| `internal/linkstate/` | the `State` schema — a **published contract** (`--json`, gsl, CI) |
| `internal/resolvconf/` | render/parse `resolv.conf` + `wsl.conf` INI, snapshot, restore, drift |
| `internal/fleetsrc/` | fleet names via `fleet discover --json`; ssh-config fallback is **read-only** |
| `internal/sshcfg/` | `ssh -G` inspection for the keepalive check |

## Rules for changing it

- **Interop stays behind `winhost.Runner`.** A `powershell` reference anywhere
  else makes the module untestable in CI.
- **No write without an undo path.** `TakeSnapshot` runs before the first byte;
  if it fails, nothing is written and the command still exits 0.
- **Log via `sdk/libs/log` only** — never hand-roll a logger.
- **`State` is a contract.** Renaming a field or wire value breaks `gsl` and any
  CI gate; the shape is pinned by test against the spec's §4.1 example.
- **Behaviour learned here belongs in the spec.** The shell prototype that
  produced this design was deleted; [`docs/mbo/specs/wlink.md`](../../docs/mbo/specs/wlink.md)
  §5.1 (EC-1…EC-22) is now the only record of what it proved. A behaviour that
  never reaches the spec is a behaviour that is lost.

## Testing

`go test ./...` — everything runs against fixtures and temp directories: no
Windows, no privileged write, no network. `WLINK_LIVE=1` additionally runs the
`winhost` parser against this machine's real PowerShell output.

Testing hooks (also useful for evidence capture): `WLINK_RESOLV_CONF`,
`WLINK_WSL_CONF`, `WLINK_BACKUP_DIR`, `WLINK_SSH_CONFIG`, `WLINK_HOSTS_FILE`,
`WLINK_PROBE_HOSTS`, `WLINK_PUBLIC_PROBE`, `WLINK_FALLBACKS`, `WLINK_GIT_HOST`.
