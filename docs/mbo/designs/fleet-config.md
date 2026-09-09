# fleet config — one-way ssh-config transfer (pull / push)

- **Slug:** fleet-config
- **Date:** 2026-08-31
- **Status:** Proposed
- **Relates to:** issue TBD / PR TBD
- **Author(s):** repo owner (via Claude)

## 1. Problem / context

`fleet` has no path for host configuration in either direction. Verified against
the module, not assumed:

| Surface | Direction | Behaviour |
| :-- | :-- | :-- |
| `fleet discover` | local only | Enumerates `Host` blocks in the local `~/.ssh/config`; `--add-all` marks them `#fleet`. Never opens a socket. |
| `internal/sshconf` | local only | `Parse` / `Add` / `Mark` / `Unmark` / `Purge` — pure functions over config text. |
| `fleet keys sync` | outbound | Authorizes local **public** keys into each fleet host's `authorized_keys`. Takes *key* names, not host names. |
| TUI | — | 21 bindings; none import or export anything. |

Two costs follow. First, a correction is stranded on whichever machine made it:
`ssh-find` rewrites a `Hostname` to a freshly discovered IP, and every other
machine keeps the stale address. Second, a new machine must be configured by
hand even though the fleet collectively already knows every host.

A third gap sits underneath both: an ssh-config entry is only useful if the
matching **key** is present and authorized. Config and access are separate
facts, and today nothing reconciles them.

## 2. Goals & non-goals

**Goals**
- Populate the local `~/.ssh/config` from one named fleet host (**pull**).
- Publish local entries out to configured targets (**push**).
- Keep those two strictly separate: see the non-goal below.
- Report, and where safe repair, the **key readiness** of the hosts a transfer touches.

**Non-goals**
- **A bidirectional verb.** There is no `fleet config sync`. Every invocation
  moves configuration in exactly ONE direction, named at the call site, with its
  own plan and its own confirmation. A combined operation would have to resolve
  conflicts by policy rather than by an operator looking at a diff, and would
  make the blast radius of a mistake the union of both directions. One-way is
  the feature, not a limitation to be removed later.
- Moving private keys between machines — permanently barred, see §5.
- Multi-host union or automatic conflict resolution across N sources. One named
  source per pull.
- Scheduled or daemonised operation. Every verb runs when asked.
- Resolving `Include` directives on the remote (reported, not followed).

## 3. Options considered

**A. Read the remote file; parse locally with the existing pure parser (chosen).**
`cat ~/.ssh/config` over the runner seam, then `sshconf.Parse`. Everything after
the read is already-tested pure code, so the merge is unit-testable with no
socket. Sole weakness: `cat` does not follow `Include`, which is *reportable*.

**B. `ssh -G` for the effective config.** Resolves `Include` and `Match`
correctly, but needs the alias list up front (chicken-and-egg), costs a round
trip per alias, and flattens each host into ~60 mostly-default directives —
destroying block structure, comments, and the `#fleet` marker that decides scope.

**C. A remote-side helper emitting JSON.** Could filter and resolve `Include` at
the source, but requires the source to already run a *current* `fleet` binary.
Fleet exists to detect hosts that are out of date, so depending on them being up
to date is circular.

## 4. Decision

One pure engine, three thin I/O verbs — each strictly one-way.

```
internal/cfgplan   PURE: Plan(localText, remoteText, opts) → Plan;  Apply(text, Plan) → text
internal/sshconf   + Update(cfg, host)  — field-level rewrite preserving unmodelled lines
cmd/config_pull.go   read remote → Plan → confirm → backup → write local
cmd/config_push.go   Plan per target → confirm → validate → backup → write remote
cmd/config_diff.go   Plan → render, never write (read-only, no direction)
```

`Plan` and `Apply` are separate so the CLI and the TUI render, confirm, and
apply the *same computed plan*, and so tests assert on a plan without touching a
file.

**Merge is field-level, never block-level.** Only the five modelled directives
(`HostName`, `User`, `Port`, `IdentityFile`, plus the alias) are rewritten; every
other line, comment, and its ordering survives. A local `ProxyCommand` is not
collateral damage of a `HostName` refresh.

**Exec-safety is structural.** `sshconf.Host` models only inert fields, so a
hostile `ProxyCommand` / `LocalCommand` / `Match exec` from a compromised source
*has nowhere to land*. There is no allowlist to forget to update. Because that
also makes exclusions invisible, `cfgplan` separately scans the raw text and
**names** what it withheld.

**Key readiness is part of the result, not a separate errand.** A pull stats
each imported `IdentityFile` locally and reports misses; for newly added hosts
that are reachable, it can drive `fleet keys sync` so they authorize the key you
already hold.

## 5. Risks & blast radius

| Risk | Severity | Mitigation |
| :-- | :-- | :-- |
| **Push breaks SSH access on N machines** | Highest | Validate before commit: write a temp file remotely, syntax-check it with `ssh -G -F <temp>`, then move into place. Timestamped remote backup, `0600`, one write per host. Per-host plan and confirmation. |
| **Push retargets the alias you are connected through, locking you out** | High | Refuse to modify the alias currently used to reach that host unless `--allow-self-retarget`. This is the specific way a "helpful" push becomes unrecoverable. |
| **Hostile source injects an exec directive** | High | Structural: unmodelled directives cannot be represented, so they cannot be written. |
| **Private key exposure** | Highest | Barred outright — `AGENTS.md`: *"No private key ever leaves the workstation."* No verb reads, transmits, or writes a private key. Key *readiness* is a local `stat` plus public-key authorization only. |
| **Dangling `IdentityFile` after a pull** | Medium | Stat locally; report every miss by name. |
| **Silent loss via `Include`** | Medium | Counted and reported; never silently skipped. |

## 6. Rollback

Every write is preceded by a timestamped `0600` backup — local for pull, remote
for push — and the path is printed. **Honest limitation:** if a push costs SSH
access to a host, fleet cannot roll that host back, because the transport it
would need is the thing that broke. The remote backup path is reported so the
restore can be done out-of-band (console, physical access). This is precisely
why push validates before committing and refuses self-retarget by default.

## 7. Evidence expectations

The plan must capture, not assert:
- A real pull between two machines: before/after config, the rendered plan, and
  the named withheld directives.
- A push whose validation step **rejects** a deliberately malformed config —
  proving the guard fires, not merely that the happy path works.
- A self-retarget attempt refused without the flag.
- A hostile source fixture containing `ProxyCommand` / `LocalCommand`, with the
  resulting local config shown to contain neither.
- Key-readiness output on a host whose `IdentityFile` is genuinely absent.
