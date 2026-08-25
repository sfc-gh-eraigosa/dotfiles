# wlink — WSL link (tunnel + resolver) management — design

- **Slug:** `wlink`
- **Date:** 2026-08-24
- **Status:** Draft — **proposal pending a build/no-build decision**
- **Relates to:** PR [#242](https://github.com/sfc-gh-eraigosa/dotfiles/pull/242) (the shell
  predecessor this would succeed) · design issue: *not opened yet, deliberately — see §8*
- **Author(s):** edward-raigosa

> **Read this first.** This document exists so the rewrite can be *declined* on the evidence.
> The shell script in #242 works, is tested (54 cases), and was validated end-to-end on a
> real WireGuard-connected machine. Nothing here is urgent. §3 includes "do nothing" as a
> genuine option, and §8 records what would make this **not** worth doing.

## 1. Problem / context

`opt/scripts/system/wsl_dns_lan.sh` (PR #242) solves one real problem: from WSL,
`ssh <fleet-host>` stalls ~20s and dies with *Temporary failure in name resolution* while
`ssh <ip>` connects instantly, because WSL2 points `/etc/resolv.conf` at the Windows NAT DNS
proxy, which answers from whatever resolver Windows treats as primary — normally the ISP's,
which has never heard of the fleet.

It solves that well. But building and testing it surfaced a cluster of *adjacent* problems it
is not shaped to solve, all verified on a live machine during the #242 work:

| Observed | Evidence |
| :-- | :-- |
| **Tunnel-readiness race** — probing during the WireGuard handshake finds nothing; Windows publishes the adapter and its DNS server seconds before the handshake completes, while the old network is already unroutable | Hit on the very first real run: every candidate returned `0/4` |
| **Intermittent git hangs over the tunnel** — SSH stalls with TCP `ESTABLISHED`, 0 bytes queued both ways, forever | `ssh -G github.com` → `serveraliveinterval 0`, `connecttimeout none`. One `ls-remote` hung 45s; the next three ran in 2–3s |
| **No way to ask "what is the state of my link?"** — tunnel up? handshake fresh? which resolver won? is the pin still in force? | Every answer today requires running `--verify` or reading `/etc/resolv.conf` by hand |
| **`dig` is a hard dependency** for a DNS probe | `dnsutils` had to be added to `packages.tsv`; on a nix-managed host it never installs and the feature silently no-ops |
| **`#fleet` parsing is duplicated** | `sdk/fleet` already owns those ssh-config blocks (`fleet add`/`discover`/`remove`) and exposes `fleet discover --json`; the shell script re-parses `~/.ssh/config` independently |

Three of those five are things a shell script *can* do but does not do well: structured state,
native DNS, and reuse of another module's contract.

## 2. Goals & non-goals

**Goals**

1. Answer *"is my link to the fleet healthy, and if not, why?"* in one command, with
   machine-readable output.
2. Keep every safety property #242 established: opt-in, fail-closed, snapshot-before-write,
   never write without an undo path, refuse to pin a non-recursive resolver.
3. Remove the `dig`/`dnsutils` dependency by resolving natively.
4. Consume `fleet`'s `#fleet` contract instead of re-deriving it.
5. Make the tunnel-readiness race a *first-class* state, not an error message — including the
   ability to wait for readiness.
6. Detect (and optionally fix) the ssh keepalive gap that turns a transient tunnel blip into an
   infinite git hang.

**Non-goals**

- Managing the VPN itself (connect/disconnect/keys). `wlink` **observes** the tunnel; WireGuard
  and its Windows client own its lifecycle.
- Changing WireGuard routing (split-tunnel `AllowedIPs`). Explicitly declined during #242
  testing — the keepalive fix addresses the git hangs without restructuring the network.
- Replacing `fleet`. `fleet` owns *which hosts exist and their access*; `wlink` owns *can I
  reach them by name from this WSL box*.
- Cross-platform parity. This is WSL-shaped by nature (Windows interop, `/etc/wsl.conf`,
  the WSL NAT proxy). It must **degrade cleanly** elsewhere, not pretend to work.
- A daemon. Like `fleet`, on-demand only.

## 3. Options considered

### Option A — do nothing; keep the shell script (the null option)

Ship #242, stop. The core problem is solved and tested.

- **For:** zero cost. 54 tests already pass. No new module, no coverage floor, no tag stream, no
  second thing to maintain. The adjacent problems are annoyances, not outages.
- **Against:** every added capability (status, doctor, wait) makes the script worse — it is
  already ~600 lines of bash doing INI rewriting, DNS parsing, and Windows interop. Structured
  output (`--json`) in bash is a stringly-typed trap. The `dig` dependency stays. The `#fleet`
  duplication stays and will drift from `fleet`.
- **Verdict:** legitimate. Choose this if the adjacent problems stay rare in daily use.

### Option B — fold the behavior into `sdk/fleet` as `fleet link …`

- **For:** no new module; `fleet` already owns `#fleet` parsing, so the duplication disappears
  by construction; one binary for "my hosts".
- **Against:** `fleet` is cross-platform (it manages hosts, keys, wake, install status). This
  work is irreducibly WSL/Windows-specific — `powershell.exe` interop, `/etc/wsl.conf`,
  `/mnt/wsl/resolv.conf`. Pushing that into `fleet` puts platform-gated dead code into a tool
  that runs on macOS and Raspberry Pi, and widens its blast radius: a bug in resolver pinning
  could take out `fleet status`. It also muddies a clean boundary — `fleet` answers *who*,
  this answers *can I reach them*.
- **Verdict:** rejected on blast radius and platform mismatch, not on convenience.

### Option C — a new `sdk/wlink` module that consumes `fleet`'s contract  ← **recommended**

A small Go CLI mirroring `sdk/gss` conventions (cobra `cmd/`, `internal/` with a mockable
runner, `internal/version` ldflags), which:

- gets fleet hostnames from `fleet discover --json` (falling back to an ssh-config parse when
  `fleet` is absent), so the `#fleet` marker has exactly one owner;
- resolves DNS **natively** in Go against a chosen server, dropping `dig`/`dnsutils`;
- queries Windows via one interop layer behind an interface, so every probe is unit-testable
  against recorded fixtures rather than a live machine;
- emits `--json` on every read command so `gsl` can surface link state in the status line and
  CI can gate on it.

- **For:** clean boundary; the platform-specific code is quarantined in a WSL-only module;
  removes both the `dig` dependency and the `#fleet` duplication; makes the untestable parts
  (Windows interop, privileged writes) testable behind interfaces; gives `gsl` something to
  consume.
- **Against:** a real module's worth of overhead — `go.mod`, `build.sh`, tag-driven versioning,
  a coverage floor in `scripts/test.sh`, `AGENTS.md` + `CLAUDE.md` symlink, `install.sh` build
  wiring. And a migration: `install.sh`, the gff flag, and `docs/wsl-dns.md` all move.
- **Verdict:** recommended **if** the adjacent problems in §1 are worth solving. If they are
  not, Option A is the honest answer.

## 4. Decision

**Proposed: Option C**, `sdk/wlink`, binary `wlink` — *pending the go/no-go this document exists
to inform.*

### Why this name

`link` was the first choice and had to be dropped: `/usr/bin/link` is the POSIX hard-link
utility (coreutils/uutils). `install.sh` prepends `~/opt/bin` to `PATH` (position 16 vs
`/usr/bin` at 40), so a binary named `link` would **shadow a POSIX utility on every machine
that runs install.sh**. `wlink` keeps the semantics, scopes it to WSL, and collides with
nothing. It still reads as a pair with `fleet`:

```
fleet status     # who my hosts are, and are they in sync
wlink status     # can I reach them by name from here
```

### Boundaries

| Unit | Owns | Depends on |
| :-- | :-- | :-- |
| `cmd/` | cobra verbs, flag parsing, exit codes, human vs `--json` rendering | every `internal/` package |
| `internal/winhost` | Windows interop: per-interface DNS servers, adapters, routes, tunnel identity + handshake age. One `Runner` interface over `powershell.exe` | — (mockable seam) |
| `internal/probe` | native DNS queries against a specific server; candidate scoring; the recursion guard | `winhost` |
| `internal/linkstate` | the composed answer: tunnel state, chosen resolver, pin status, drift | `winhost`, `probe`, `fleetsrc` |
| `internal/resolvconf` | parse/render `/etc/resolv.conf` + `/etc/wsl.conf` INI; snapshot; restore; atomic privileged writes | — |
| `internal/fleetsrc` | fleet hostnames via `fleet discover --json`, ssh-config fallback, `/etc/hosts` exclusion | — |
| `internal/sshcfg` | ssh config inspection for the keepalive/`doctor` checks | — |

The **frozen interface** that lets this be built in parallel is `winhost.Runner` plus the
`linkstate.State` struct — everything else consumes them. See the plan's §3 and §6.1.

## 5. Risks & blast radius

| Risk | Severity | Mitigation |
| :-- | :-- | :-- |
| **Breaking DNS on the host** — the whole point is rewriting `/etc/resolv.conf` | High | Every safety property from #242 is carried over as a hard requirement, not a nicety: snapshot before first write, refuse to write if the snapshot fails, recursion guard, `unpin` restores byte-for-byte. The spec makes each one a test. |
| **Two implementations during migration** — shell + Go both able to pin | Medium | The shell script is retired to `archive/` in the same PR that wires the binary; the gff flag points at exactly one implementation at a time. Never both. |
| **Native DNS behaves differently from `dig`** | Medium | The Go resolver must reproduce the recorded `dig` behavior for the cases #242 already characterized (NXDOMAIN vs no-response vs NOERROR-no-data). Those become fixture tests, seeded from real captures. |
| **Windows interop is untestable in CI** | Medium | All interop behind `winhost.Runner`; CI runs against recorded fixtures. A live-machine acceptance checklist covers what fixtures cannot. |
| **Scope creep into VPN management** | Medium | §2 non-goals are explicit: observe, never manage. |
| **The module is never finished** and the shell script rots alongside a half-built Go tool | Medium | This is the real risk of saying yes prematurely — hence Option A staying on the table and the trio deliberately deferred (§8). |

**Blast radius if `wlink` is wrong:** one WSL machine's DNS, recoverable with `wlink unpin` or
by deleting `/etc/resolv.conf` and letting WSL regenerate it. It is opt-in and fail-closed, so
a machine that never enables it cannot be affected.

## 6. Rollback

- **Before merge:** decline the objective; #242's shell script is already the shipped answer.
- **After merge, per machine:** `wlink unpin` restores the snapshot exactly; failing that,
  remove `/etc/resolv.conf`, drop `generateResolvConf` from `/etc/wsl.conf`, `wsl.exe --shutdown`.
- **After merge, repo-wide:** the gff flag (`install.system.wsl-dns`, default **false**) is the
  kill switch — nothing runs unless explicitly enabled. Restoring the shell script means
  reverting one commit and un-archiving one file.

## 7. Evidence expectations

The plan must capture, in `docs/mbo/plans/wlink/evidence/`:

| Proof class | What it must show |
| :-- | :-- |
| **Fixture-backed unit runs** | Candidate scoring, the recursion guard, `/etc/hosts` exclusion, and all five `wsl.conf` INI shapes — ported from the 54 shell cases, which are the executable specification. |
| **Round-trip evidence** | `pin` → re-run → `unpin` restoring `resolv.conf` (symlink target included) and `wsl.conf` **byte-for-byte**. Already proven in shell; must not regress. |
| **The live tunnel matrix** | `wlink verify` with the tunnel **up** and **down**, captured from a real machine. The invariant: the public sentinel resolves in *both* states, fleet names only when up, and a miss completes inside the derived budget. |
| **The readiness race** | A capture taken *during* a handshake showing `status` reporting `not-ready` rather than a confusing all-zero probe. |
| **Timing evidence** | Before/after miss timings. #242's baseline is recorded: **20–21s unpinned → 4s pinned** on this machine. `wlink` must match or beat that. |
| **`doctor` findings** | A real run flagging the missing `ServerAliveInterval` (verified absent: `serveraliveinterval 0`, `connecttimeout none`). |

Demo worth planning now: a single asciinema-style capture of `wlink status` before connect,
during the handshake, and after — because the readiness race is the least obvious behavior and
the hardest to explain in prose.

## 8. Deliberately not done yet

- **No design issue opened.** MBO policy anchors every objective to a design issue; that is
  skipped here on purpose so a declined proposal leaves no GitHub debris. If this is approved,
  the issue is the first step.
- **No execution trio** (`plans/wlink/{IMPLEMENTATION,TRACKING,TODO}.md`). Policy requires it for
  any plan that will be built — so it is the second step after approval, not part of a proposal.
- **Migration of `docs/wsl-dns.md`** is planned but not written; it becomes `wlink`'s module docs.

**What would make this NOT worth doing:** if, after living with the shell script for a while,
the readiness race and the git hangs turn out to be rare enough that the `--verify` warning and
a one-line ssh config fix cover them. Then Option A wins and this document is marked
`Superseded`. That verdict needs usage, not argument — which is the strongest reason to ship
#242 first and decide later.
