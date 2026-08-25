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
- **Trigger:** `ssh wenlockpi` stalls ~20s, then *Temporary failure in name resolution*.
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

## 6. Verification harness

| Layer | Covers | Gate |
| :-- | :-- | :-- |
| **Unit (fixtures)** | EC-1…EC-8, EC-11, EC-12 — `winhost` fed recorded PowerShell output; `probe` against a local in-process DNS server; `resolvconf` against temp files | `go test ./...`, module coverage **≥60%** (the `sdk/` floor in `scripts/test.sh`) |
| **Golden files** | Rendered `resolv.conf` / `wsl.conf` for all five INI shapes | byte-exact comparison |
| **Round-trip** | EC-4 — `pin` → re-run → `unpin` restores both files byte-for-byte | temp-dir integration test, no privileged writes |
| **Ported shell cases** | The 54 cases in `wsl_dns_lan_test.sh` are the executable specification and must all have a Go counterpart | traceability table in the plan §5 |
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
  builds it. The shell script's `install.system.wsl-dns` is retired in the same cutover PR
  (plan §3.2).
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
