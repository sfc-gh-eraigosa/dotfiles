# wlink — WSL link (tunnel + resolver) management — design

- **Slug:** `wlink`
- **Date:** 2026-08-24
- **Status:** Approved
- **Relates to:** PR [#242](https://github.com/sfc-gh-eraigosa/dotfiles/pull/242) — ships
  `opt/scripts/system/wsl_dns_lan.sh`, the shell predecessor `wlink` succeeds
- **Author(s):** edward-raigosa

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

## 3. Alternatives considered

Both were rejected. Recorded so the same ground is not re-litigated.

### Keep extending the shell script — rejected

`wsl_dns_lan.sh` is already ~600 lines of bash doing INI rewriting, DNS parsing, and Windows
interop. Every capability in §1 makes it worse: structured output (`--json`) in bash is a
stringly-typed trap, the `dig` dependency cannot be removed from a shell probe, and the
duplicate `#fleet` parser stays and drifts from `fleet`. The script is a good *solution to one
problem* and a bad *foundation* — which is why it ships as-is in #242 and is superseded rather
than grown.

### Fold it into `sdk/fleet` as `fleet link …` — rejected

Tempting, because `fleet` already owns `#fleet` parsing, so the duplication would vanish by
construction. Rejected on **blast radius and platform fit**: `fleet` is cross-platform — it runs
on macOS and Raspberry Pi — while this work is irreducibly WSL/Windows-specific
(`powershell.exe` interop, `/etc/wsl.conf`, `/mnt/wsl/resolv.conf`). Folding it in would put
platform-gated dead code into a cross-platform tool and let a bug in resolver pinning take out
`fleet status`. It also blurs a boundary worth keeping sharp: `fleet` answers *who my hosts
are*; `wlink` answers *can I reach them from here*.

The duplication is solved instead by **consuming** `fleet`'s contract — see §4.

## 4. Decision

Build **`sdk/wlink`**, binary `wlink`: a WSL-only Go CLI mirroring `sdk/gss` conventions
(cobra `cmd/`, `internal/` behind a mockable runner, `internal/version` ldflags), which

- takes fleet hostnames from `fleet discover --json` — falling back to a read-only ssh-config
  parse when `fleet` is absent — so the `#fleet` marker has exactly one owner;
- resolves DNS **natively in Go** against a chosen server, dropping the `dig`/`dnsutils`
  dependency;
- reaches Windows through a single interop layer behind an interface, making every probe
  testable against recorded fixtures instead of a live machine;
- emits `--json` from every read command, so `gsl` can surface link state in the status line
  and CI can gate on it;
- **logs through the shared `sdk/libs/log`**, like every other sdk tool — see below.

It carries over every safety property #242 established — opt-in, fail-closed,
snapshot-before-write, no write without an undo path, and refusing to pin a non-recursive
resolver — as hard requirements, each one a test in the spec.

### The name

**`wlink` = WSL link** — the link between this WSL box and the private network its fleet
lives on. "Link" is the thing the tool reasons about: the tunnel carrying it, the resolver
that makes its names resolvable, and whether it is currently usable. The `w` scopes it to
WSL, which is what the tool is — Windows interop, `/etc/wsl.conf`, the WSL NAT proxy.

It reads as a pair with `fleet`:

```
fleet status     # who my hosts are, and are they in sync
wlink status     # can I reach them by name from here
```

> **What this looks like in practice:** the spec's
> [§4.1 worked example](../specs/wlink.md#41-worked-example--what-you-actually-end-up-with)
> walks one fictitious lab through the whole thing — the ssh config in, the `status` / `pin` /
> `doctor` output, and the resulting `resolv.conf` and `wsl.conf`. Read it before the boundaries
> below; it makes the shape concrete.

### Logging

`wlink` uses the repo's shared logger, `sdk/libs/log` — the same one `fleet`, `gsl`, and
`tmux-mgr` use. **No hand-rolled logger, file writer, or rotation**, per the `sdk/AGENTS.md`
contract:

```go
import applog "github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/log"

applog.SetDefaultTool("wlink")    // once, at startup
applog.Default().WithField("resolver", srv).Info("pinned")
```

- **Diagnostics** — what `wlink` did and why it decided it — go to `New`/`Default`: logrus
  JSON with lumberjack rotation, controlled by `$WLINK_LOG_FILE` / `$WLINK_LOG_LEVEL`.
- **Captured output** — bytes produced by a process `wlink` shelled out to (`powershell.exe`
  interop, `fleet discover`) — goes to `NewCapture`, kept plain-text per run so a capture is
  readable as-is.

This matters more here than in a typical tool for two reasons. First, the decisions worth
auditing are invisible by design: which candidate resolvers were seen, what each answered, why
one won, and why a write was declined. A structured diagnostic log turns "it did nothing" into
an answerable question — the failure mode that cost the most time while testing the shell
predecessor. Second, the interop and privileged-write paths are exactly where post-hoc evidence
is most valuable, and `libs/log` guarantees the property that makes logging safe there:
**construction never fails** — a logger that cannot open its file discards, a nil `*Capture` is
safe to call, so logging can never introduce a failure mode into a tool that rewrites
`/etc/resolv.conf`.

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
| *(logging)* | no bespoke package — every unit logs via `sdk/libs/log` | `libs/log` |

The **frozen interface** that lets this be built in parallel is `winhost.Runner` plus the
`linkstate.State` struct — everything else consumes them. See the plan's §3 and §6.1.

## 5. Risks & blast radius

| Risk | Severity | Mitigation |
| :-- | :-- | :-- |
| **Breaking DNS on the host** — the whole point is rewriting `/etc/resolv.conf` | High | Every safety property from #242 is carried over as a hard requirement, not a nicety: snapshot before first write, refuse to write if the snapshot fails, recursion guard, `unpin` restores byte-for-byte. The spec makes each one a test. |
| **Losing what the prototype proved** when it is deleted | High | Its 54 cases are distilled into spec EC-1…EC-19, and **spec §5.1 records how the non-obvious rules were discovered** — the ones a from-scratch Go implementation would get wrong. P15 is gated on every rule citing a passing test. Standing obligation during the build: anything learned that the prototype had handled goes into §5.1 as a rule, not just into code. |
| **Native DNS behaves differently from `dig`** | Medium | The Go resolver must reproduce the recorded `dig` behavior for the cases #242 already characterized (NXDOMAIN vs no-response vs NOERROR-no-data). Those become fixture tests, seeded from real captures. |
| **Windows interop is untestable in CI** | Medium | All interop behind `winhost.Runner`; CI runs against recorded fixtures. A live-machine acceptance checklist covers what fixtures cannot. |
| **Scope creep into VPN management** | Medium | §2 non-goals are explicit: observe, never manage. |
| **The module stalls half-built**, leaving the shell script rotting beside an unfinished Go tool | Medium | The §8 cutover is a single PR: the binary is wired and the script archived together, so `main` never carries two live implementations. Until that PR lands, the shipped shell script remains the only thing `install.sh` can run. The execution trio makes a stalled run resumable rather than abandoned. |

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
| **Fixture-backed unit runs** | Candidate scoring, the recursion guard, `/etc/hosts` exclusion, and all five `wsl.conf` INI shapes — one test per spec rule EC-1…EC-19. |
| **Round-trip evidence** | `pin` → re-run → `unpin` restoring `resolv.conf` (symlink target included) and `wsl.conf` **byte-for-byte**. Already proven in shell; must not regress. |
| **The live tunnel matrix** | `wlink verify` with the tunnel **up** and **down**, captured from a real machine. The invariant: the public sentinel resolves in *both* states, fleet names only when up, and a miss completes inside the derived budget. |
| **The readiness race** | A capture taken *during* a handshake showing `status` reporting `not-ready` rather than a confusing all-zero probe. |
| **Timing evidence** | Before/after miss timings. #242's baseline is recorded: **20–21s unpinned → 4s pinned** on this machine. `wlink` must match or beat that. |
| **`doctor` findings** | A real run flagging the missing `ServerAliveInterval` (verified absent: `serveraliveinterval 0`, `connecttimeout none`). |

Demo worth planning now: a single asciinema-style capture of `wlink status` before connect,
during the handshake, and after — because the readiness race is the least obvious behavior and
the hardest to explain in prose.

## 8. Sequencing

Ordered so nothing lands half-migrated:

1. **Design issue** — the durable tracker and parent for any build sub-issues, per MBO policy.
2. **Execution trio** (`plans/wlink/{IMPLEMENTATION,TRACKING,TODO}.md`) — policy requires it for
   any plan that will be built; it is what makes the run resumable and evidence-backed.
3. **Build**, TDD, in the order the plan's §4 sets out — the frozen `winhost.Runner` seam and
   the `linkstate.State` schema first, since everything else consumes them.
4. **Build and retire inside PR #242.** The shell prototype was never meant to ship — it exists
   to have proven the behavior on real hardware and to have produced this design. None of it has
   landed on `main`, so there is no cutover, no archival, and no flag migration: the prototype,
   its test, `docs/wsl-dns.md`, and its `install.system.wsl-dns` entry are **deleted in the same
   PR that introduced them**, once `wlink` supersedes them (plan §4 P15). `main` only ever sees
   the Go tool.
5. **Delete last, not first.** P15 is gated on every spec rule EC-1…EC-19 citing a passing Go
   test, and on spec §5.1 being current — see the §5 risk row. The prototype stays runnable
   until then, as a reference to compare against rather than as a gate.

`wlink` is gated by **one** flag, `install.sdk.wlink` — fail-closed, default **off** (plan
§3.1). It decides whether `install.sh` builds and installs the binary *and* whether the pin
runs: enabling it already says "this machine uses the tunnel and wants its fleet resolvable",
which is the same consent. A machine that has not asked for it never even builds the binary —
unlike the other `install.sdk.*` flags, which default true because every machine wants those
tools.
