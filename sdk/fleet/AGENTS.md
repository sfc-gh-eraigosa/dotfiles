# fleet — dotfiles install-status checker

> **Up:** [`sdk/`](../AGENTS.md) · objective [`docs/mbo/`](../../docs/mbo/index.md) slug `fleet`
> (design · spec · plan · execution trio).

Answers *"which of my hosts are out of sync with the latest dotfiles install?"* —
on demand, never as a daemon — and manages fleet membership and access keys.

## Why it exists

`install.sh` used to leave **no record that it ran**, so a host's git clone could be
current while the installer last ran weeks ago. "Pulled" and "installed" are different
facts. `opt/scripts/system/install-stamp.sh` now records the second one; this tool reads it.

## Commands

| Command | Does |
| :-- | :-- |
| `fleet status [host...]` | table of host · commit · **branch** · last run · status; `--json`; exits non-zero if any host is stale |
| `fleet discover [--scan]` | list every concrete ssh-config host as `in-fleet` / `available`; `--scan` sweeps the subnet to refresh a moved `HostName` and offer unknown responders; `--json`; `--add-all` bulk-adopts (one pass, one backup; `--dry-run` / `--yes`) |
| `fleet tui` | streaming dashboard: vim nav (`gg`/`G`/`ctrl+d`), `/` regex search, `space`/`v`/`a` selection, concurrent background updates (`--jobs`), `w` wake, `s` ssh, `F` forget answers, `?` help |
| `fleet update <host>...` | fetch → checkout `--ref` (default `main`) → `pull --ff-only` → `install.sh` over `ssh -t`, one host at a time |
| `fleet add <alias>` | **adopt** an existing ssh-config entry (marks in place, no `--hostname`); with `--hostname H` **creates** a new `#fleet` block. `--dry-run` |
| `fleet remove <alias> [--purge]` | unmark (keeps SSH access); `--purge` deletes the block |
| `fleet keys list\|sync\|prune` | audit / authorize / remove authorized keys |
| `fleet config pull\|push\|diff` | one-way ssh-config transfer: import FROM one host, publish TO hosts, or compare without changing anything |
| `fleet wake [host...]` | rouse hosts asleep at layer 2: ladder `retry → local-prime → peer-relay`, printed rung by rung; `--json`; exits non-zero if any target stayed down |

## Layout

| Path | Responsibility |
| :-- | :-- |
| `cmd/` | cobra commands, rendering, SSH fan-out |
| `cmd/tui_model.go` | TUI state machine: modes, alias-keyed cursor/selection, update engine |
| `cmd/tui_view.go` | pure `View()` + the one lipgloss `theme` |
| `cmd/tui_keys.go` | keymap + mode routing (`keyHelp` is the single source of truth) |
| `cmd/tui_cmds.go` | tea.Cmd producers: poll, precheck, background update, handoffs |
| `internal/sshconf` | parse **and edit** `~/.ssh/config` (the only inventory) |
| `internal/stamp` | parse the install stamp |
| `internal/drift` | classify drift + format age (`now` injected — never `time.Now()`) |
| `internal/sshfail` | read ssh's stderr to tell a refused *connection* from a refused *credential* |
| `internal/cfgplan` | plan a ONE-WAY ssh-config transfer (pure): `Build` + `Apply` |
| `internal/lanscan` | sweep a subnet for a listening port (injected dialer — no nmap, no socket in tests) |
| `internal/keys` | authorized_keys diff (reports removals, never applies them) |
| `internal/reach` | the wake ladder: rung order, peer ranking, provenance (pure; every impure edge injected via `Deps`) |
| `cmd/answers_store.go` | the non-secret prompt preferences on disk (`0600`); the on-disk type has no credential field |
| `internal/runner` | the **only** seam that touches a remote host (`Exec` real, `Fake` for tests) |

Everything but `runner` is pure text-in/struct-out, so the decision surface is unit-tested
without opening a socket.

## Invariants (each pinned by a test — don't regress these)

- **No private key ever leaves the workstation.** Sync authorizes public keys only.
- **`prune` is diff-first**: prints each removal, applies nothing without confirmation, and
  deletes only that exact line — never rewrites `authorized_keys` from local state.
- **`add` adopts before it creates.** An alias already in `~/.ssh/config` is marked
  in place (`sshconf.Mark`), preserving every directive — `--hostname` is required
  *only* when writing a genuinely new block. Adoption is idempotent and reversible
  by `remove`.
- **`remove` unmarks; only `--purge` deletes.** Leaving the fleet never costs SSH access.
- **Every ssh-config write takes a timestamped backup first** and keeps `0600`.
- **A dirty clone is skipped** unless `--force`, which *preserves* work in a rescue worktree
  (`git add -A` onto a branch — **not** `stash@{0}`, which silently drops untracked files).
- **Failures are named per host** and reflected in the exit code; never swallowed.
- **`unreachable` means the network, never the keys.** A probe that CONNECTED and was
  then refused — unknown or changed host key, no accepted credential — classifies as
  `auth-failed`, not `unreachable`. Every probe runs `BatchMode=yes`, so an unknown host
  key fails *instantly* and used to look exactly like a dead machine; the cost was a real
  investigation aimed at the network for a one-line `known_hosts` gap. The evidence is
  ssh's stderr, which `(*exec.Cmd).Output()` already captures. A failure with no stderr
  to read stays `unreachable` — `internal/sshfail` never invents a diagnosis. Pinned by
  `TestAuthFailureReportsAuthFailedNotUnreachable` and
  `TestFailureWithNoEvidenceStaysUnreachable`.
- **`discover` is a local read; `--scan` is the ONE exception.** Without the flag it never
  opens a socket. The sweep needs no nmap — the shell `ssh-find` it replaces shells out to
  it, and on a machine without nmap that script exits before scanning anything, which is a
  discovery tool that silently cannot discover. `internal/lanscan` takes an injected
  dialer, so the sweep is unit-tested without a socket.
- **A scan matches on the host's OWN reported name, never on the address.** That is what
  turns a DHCP move into a one-line `HostName` refresh instead of a duplicate `Host` block
  under a second alias, and the refresh goes through `sshconf.Update` so a `ProxyCommand`
  survives it. Pinned by `TestScanPlanRefreshesAMovedHostRatherThanAddingIt`.
- **A responder that will not authenticate is never written.** We do not know what it is or
  which user it wants, and guessing would put a broken block in the file every command
  depends on. It is reported and left alone. Pinned by
  `TestApplyScanNeverWritesAnUnidentifiedResponder`.
- **Addresses render in numeric order.** Lexical order puts `.128` before `.16` and `.201`
  before `.61`, which makes a scan of the same network read as noise. Pinned by
  `TestClassifyScanOrdersAddressesNumerically` — a bug found by running a live sweep, not
  by review.
- **One authentication per host, then connection reuse.** Every ssh invocation carries
  `ControlMaster=auto` / `ControlPersist=10m`, so the first connection authenticates —
  interactively, on a real terminal, with fleet never seeing the secret — and every later
  command rides that socket and skips authentication entirely, BatchMode included. This is
  deliberately NOT a stored credential: `sudo -S` reads stdin by design, but `ssh` opens
  `/dev/tty` precisely so a password cannot be piped, so the sudo pattern cannot be copied
  here. Multiplexing is better than copying it would have been — nothing is stored, it
  serves key and password auth alike, and it removes a full handshake per command
  (measured: 3 connections in 0.98s cold vs 0.016s warm).
  `ControlPath` uses `%C`, a fixed-length hash: a literal `%r@%h:%p` grows with the user
  and host name and can exceed the ~104-byte unix socket limit, at which point
  multiplexing fails SILENTLY and prompting returns. The TUI's interactive `s` session
  passes the SAME options via `runner.MuxArgs`, or it would open its own socket and the
  next probe would prompt again. **Caveat:** a live master pins the connection settings it
  was opened with, so an `IdentityFile`/`User` change does not take effect until it expires
  or is closed (`ssh -O exit -o ControlPath=~/.ssh/fleet-mux-* <host>`). `FLEET_NO_MUX=1`
  disables the whole mechanism. Pinned by `TestEveryRemotePathCarriesTheMuxOptions`,
  `TestControlPathIsShortEnoughToBeAUnixSocket`, `TestMultiplexingCanBeDisabled`.
- **A missing `~/.ssh/config` is an EMPTY fleet, not a failure.** Every command reads the
  inventory through `readConfig`; four of them once used `os.ReadFile` directly and treated
  "missing" as fatal, which made `fleet` refuse to start on precisely the fresh machine that
  needed setting up — and once bare `fleet` opened the dashboard, that was the first thing a
  new user hit. Pinned by `TestMissingConfigIsAnEmptyFleetNotAnError`.
- **First run offers, it never assumes.** An empty fleet triggers an offer to create the
  config and then to scan; nothing is written without an explicit yes, and no self entry is
  added (a `HostName` equal to the machine's own name resolves to loopback, which the
  transfer verbs correctly refuse as a peer — a confusing thing to create for someone
  automatically). An existing key is ALWAYS looked for before generating is offered:
  a second key where a good one exists is how a machine ends up with credentials nobody
  can account for. Pinned by `TestPickIdentityPrefersAnExistingKeyOverGenerating`.
- **Interactivity is decided by asking the descriptor, not the file mode.** `/dev/null` is
  itself a character device, so the usual `Mode()&os.ModeCharDevice` idiom classified a
  script run with `</dev/null` as interactive — it printed a question nobody could see and
  read the EOF as "no". `isTerminal` uses `term.IsTerminal` (already in the module graph via
  bubbletea, so no new dependency). Pinned by `TestDevNullIsNotATerminal`.
- **Bare `fleet` opens the dashboard.** Help is `fleet help` (and `--help`). `Args` is
  constrained so a mistyped subcommand still errors rather than falling through to the TUI
  and hiding the typo behind a working-looking UI.
- **The TUI list orders by host name, never by severity.** Severity ordering is unstable
  while rows stream in: a class changes, its severity changes, and the row jumps under the
  operator's eyes. `fleet status` keeps worst-first — a one-shot report has no re-sorting
  problem and leading with the broken hosts is the point there. Pinned by
  `TestTUIOrderDoesNotMoveWhenAClassChanges`.
- **Config transfer is one-way, always.** `config pull` and `config push` each move
  configuration in exactly one direction, named at the call site. There is deliberately no
  `sync` verb: a combined operation would resolve conflicts by policy instead of by an
  operator reading a diff, and would make one mistake's blast radius the union of both
  directions. `config diff` shows both directions and writes nothing — that is how you
  choose a verb, not a third direction.
- **A transfer cannot carry an exec directive.** `sshconf.Host` models only inert fields,
  so `ProxyCommand` / `LocalCommand` / `Match exec` have nowhere to land. This is
  STRUCTURAL, not a filter — there is no allowlist to forget to update. Because that also
  makes the exclusion invisible, `cfgplan` scans the raw text and NAMES what it withheld,
  along with any `Include` it could not follow. Pinned by
  `TestBuildNeverCarriesAnExecDirective` and `TestBuildNamesWhatItWithheld`.
- **`Add` purges; `Update` preserves.** `sshconf.Add` re-renders a block from the struct
  and would silently take an operator's `ProxyCommand` with it, so it is used ONLY for
  aliases that do not exist yet. Every update goes through `sshconf.Update`, which
  rewrites the four modelled directives in place. Pinned by
  `TestUpdateRewritesOnlyModelledDirectives`.
- **A transfer MERGES into the destination, never replaces it.** Applying a plan to an
  empty string would delete every host and directive the destination had that we do not
  model. Push applies onto the target's own text. Pinned by
  `TestPushMergesIntoTheTargetConfigRatherThanReplacingIt` — a bug caught by that test
  before it ever ran against a machine.
- **A push validates before it commits.** The staged config is parsed by ssh ON THE TARGET
  before it can replace the live one, the original is backed up first, and the target is
  re-probed after. Fleet cannot repair a host it can no longer reach — the transport it
  would need is the thing that broke — so these guards exist to make that outcome unlikely
  and human-recoverable when it happens. Pinned by `TestRemoteInstallValidatesBeforeMoving`.
- **A push never silently retargets its own route.** A plan that changes `HostName`/`Port`/
  `User`/`IdentityFile` for the alias being written to is refused without
  `--allow-self-retarget`. Pinned by `TestSelfRetargetIsDetected`.
- **Omission is never deletion.** An empty field on either side leaves the destination
  value alone; no transfer blanks a directive the other side simply did not set.
- **Key readiness is a `stat`, never a read.** An imported `IdentityFile` is only a path,
  so a missing key is NAMED rather than left to fail at connect time — but no private key
  is ever read, transmitted, or written. `keys sync --host` authorizes only named hosts,
  and hosts that refuse us are reported as needing MANUAL bootstrap, because appending to
  a remote `authorized_keys` requires the access we are trying to establish.
- **The TUI delegates config transfer to the CLI verb.** `p` / `P` suspend the TUI and run
  `fleet config pull|push`, so the diff is visible and every guard applies identically from
  either entry point. A second confirm flow inside the TUI would be a second place for
  those guards to drift.
- **An answering host is never woken, and never relays.** The ladder rouses machines
  asleep at layer 2; a host that refused us is awake, so waking it spends a full budget
  (~12s) per host per run to fix nothing. It is equally never ranked as a live relay
  peer — we cannot run a command through a hop that refuses us. Pinned by
  `TestWakeLadderNeverFiresForAnAuthFailure` and
  `TestAuthFailedHostIsNeverOfferedAsALiveRelayPeer`.
- **TUI in-flight ownership**: a host is in exactly one of `pending` / `updating` /
  `waking` / resolved. Refresh skips hosts an async path owns; every completion
  re-polls its host. Two async paths must never own one row.
- **TUI updates are background-first**: `tea.ExecProcess` suspends the WHOLE TUI, so
  it is reserved for the sudo-precheck fallback and the `s` ssh action. The default
  lane runs over the runner seam with `BatchMode=yes` so a surprise prompt fails
  fast and visibly instead of hanging.
- **The sudo credential is memory-only and stdin-only.** It is never persisted,
  logged, rendered (the form masks it), placed in argv, or exported as an env
  var — `/proc/<pid>/{cmdline,environ}` are world-readable. `runner.RunStdin` is
  the only channel. Pinned by `TestSudoSecretNeverAppearsInTheRemoteCommand`.
- **Prime and install share one ssh session, and the prime is verified.** sudo's
  timestamp is tty/session-scoped, so a separate priming connection may not
  carry; `sudo -n true` gates the install so it can't run with every privileged
  step silently skipping (exit 91 = bad password, 92 = did not persist).
- **Bulk adopt is one pass, one write.** `discover --add-all` accumulates every
  `Mark` into a single config then writes once — N separate writes would mean N
  backups and N windows in which a partial write costs SSH access. Nothing
  available ⇒ no write at all.
- **Answers are sticky for the session; the confirm strip is the gate.** This
  deliberately *reverses* the earlier "form starts empty every wave" rule. That rule
  forced a retype for every wave of a fleet-wide update, and retyping is exactly how two
  waves end up applying *different* answers — the opposite of what the form is for. `esc`
  now backs out without forgetting, `u` skips the form when answers are remembered, and
  the protection against stale answers comes from the confirm strip **displaying** them
  (masked) rather than from throwing them away. `F` forgets on purpose; process exit
  forgets unconditionally. Don't "restore" the old behaviour thinking it was an oversight.
- **The credential is session-scoped and never serialised.** Sticky answers widened its
  lifetime from one wave to one process — a bounded, deliberate trade. `~/.config/fleet/answers.json`
  (`0600`) holds `windows` and `gemini` only: the on-disk type has no field for a
  credential, so the mistake is unrepresentable rather than merely avoided. Pinned by
  `TestSavedAnswersNeverContainTheCredential` (asserts on the marshalled bytes) and
  `TestLoadIgnoresACredentialPlantedInTheFile`.
- **The persistence path is INJECTED (`tuiModel.ansPath`), never resolved inside the model.**
  A model that called `answersPath()` itself made every test write to the developer's real
  `~/.config/fleet`. Empty path = no persistence, which is what tests get.
- **Branch costs no extra round-trip.** The live checked-out branch rides in the *same*
  remote command as the stamp read, split on `probeDelim`. A second dial per host would
  double the poll for one column. Pinned by `TestBranchCostsNoExtraRoundTrip`.
- **Row width is derived, not guessed.** `rowPrefixWidth` sums the same numbers as
  `rowView`'s format string and `failWidth` budgets from it; the failure cause is dropped
  rather than clamped to a floor when nothing fits. Adding the BRANCH column against a
  hardcoded prefix overflowed the row, and a minimum-width floor pushed it past the edge
  anyway — both caught by the demo width guard.
- **TUI cursor/selection are alias-keyed**, never index-keyed — rows re-sort as they
  stream in.
- **Wake never mutates a target.** The ladder sends ICMP, reads `$SSH_CONNECTION` on a
  peer, and probes — nothing else. It runs automatically inside `status`, a *read* path,
  so anything that wrote to a host would be a side effect of merely looking at it. Pinned
  by `TestWakeNeverSendsAnythingThatWritesToATarget` (argv allowlist + banned-substring
  sweep).
- **Only a DIRECT re-probe may report `Woke`.** A successful `ssh -J` proves the *peer*
  can route to the target, which is strictly weaker than "this workstation can". Reporting
  wake on relay success alone would turn a real network partition into a green row. Pinned
  by `TestRelaySuccessAloneNeverReportsWoke`.
- **The cheap rung may not starve the effective one.** `retry` gets at most `Budget/3`;
  the rest is reserved for `peer-relay`. Found by live testing, not review: two retries
  against a dead host ate a 20s budget whole and the relay never ran. Pinned by
  `TestRetryRungCannotStarveThePeerRelay`.
- **`waking` joins the in-flight ownership set.** A host is in exactly one of `pending` /
  `updating` / `waking` / resolved; refresh skips the first two, `u` and `s` cannot claim
  a waking host, and every wake completion releases its claim **unconditionally** — a
  failed ladder that kept ownership would freeze that row for the rest of the session.
- **No `ping -W` anywhere.** It means *seconds* on GNU and *milliseconds* on BSD. Local
  pings are bounded by `exec.CommandContext`; the relay nudge is detached and never waited
  on at all.

## Gotchas

- The stamp is **not retroactive**: a host reports `unknown` until it runs an
  `install.sh` that *contains* the stamp step. Pre-merge, `fleet update <host>`
  (default target `main`) pulls a `main` whose `install.sh` has no stamp step yet,
  so status stays `unknown` — expected, not a bug. Point `update --ref` at the
  feature branch to prove the stamp before it merges.
- A stamp that exists but won't parse reports `unknown (corrupt stamp)` — deliberately
  distinct from never-installed.
- `auth-failed (host key unverified)` almost always means the alias was never accepted
  into `~/.ssh/known_hosts` — common right after `ssh-find` rewrites a `Hostname`, since
  the *new* address is an unknown host to ssh. Fix on the workstation:
  `ssh-keyscan -H <alias> >> ~/.ssh/known_hosts` after checking the fingerprint.
  `auth-failed (host key CHANGED)` is NOT routine — it is the MITM warning, and the row
  is orange rather than red precisely so it is not mistaken for a dead host.
- **A host that needs waking every run is not healthy — it is power-saving.** The `woke via
  <peer>` note exists to keep that visible instead of smoothing it away. The permanent cure
  is on the host, not in fleet: a Wi-Fi NIC with `power_save on` sleeps through the
  *broadcast* ARP requests a cold neighbour cache must send (`iw dev wlan0 get power_save`
  to check). Disable it persistently with a NetworkManager drop-in —
  `/etc/NetworkManager/conf.d/wifi-powersave-off.conf` containing `[connection]` /
  `wifi.powersave = 2`. Fleet deliberately does **not** apply this for you; see the
  non-mutation invariant.
- **`BRANCH` shows the LIVE checkout, not the stamp.** `feature/x≠main` means "checked
  out feature/x, last installed from main" — usually the explanation for an
  `ahead/divergent` row. `detached` is a detached HEAD; `-` means no clone (or a host too
  old to answer the two-part probe). Branch is in the search haystack, so `/feature` → `a`
  → `u` targets every feature-branch host.
- **`BRANCH` shows the LIVE checkout, not the stamp.** `feature/x≠main` means "checked out
  feature/x, last installed from main" — usually the explanation for an `ahead/divergent`
  row. `detached` is a detached HEAD; `-` means no clone, no git, or a host too old to
  answer the two-part probe. Branch is in the search haystack, so `/feature` → `a` → `u`
  targets every feature-branch host in three keystrokes.
- **`local-prime` is a no-op under WSL2 NAT and that is expected.** The workstation's
  `eth0` is a private `172.x` link with no layer-2 presence on the fleet subnet, so the
  rung reports `skipped: workstation is not on the target's subnet`. It earns its place
  when fleet runs *on* a fleet member, which is genuinely on the LAN.
- `build.sh` injects `cmd.Version`/`Commit`/`Dirty`/`BuildDate` by exact symbol path. Keep
  them exported, or the ldflags silently no-op and every binary reports `dev`.
- The coverage floor lives in `scripts/test.sh` `coverage_min()`; a module missing from that
  map is silently exempt.

## Build & test

```bash
bash sdk/fleet/build.sh          # -> ~/opt/bin/fleet
cd sdk/fleet && go test ./... -cover
./scripts/test.sh                # repo-wide; enforces the fleet coverage floor
```
