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
| `fleet status [host...]` | table of host · commit · last run · status; `--json`; exits non-zero if any host is stale |
| `fleet discover` | list every concrete ssh-config host as `in-fleet` / `available`; `--json`; `--add-all` bulk-adopts (one pass, one backup; `--dry-run` / `--yes`) |
| `fleet tui` | streaming dashboard: vim nav (`gg`/`G`/`ctrl+d`), `/` regex search, `space`/`v` selection, concurrent background updates (`--jobs`), `s` ssh, `?` help |
| `fleet update <host>...` | fetch → checkout `--ref` (default `main`) → `pull --ff-only` → `install.sh` over `ssh -t`, one host at a time |
| `fleet add <alias>` | **adopt** an existing ssh-config entry (marks in place, no `--hostname`); with `--hostname H` **creates** a new `#fleet` block. `--dry-run` |
| `fleet remove <alias> [--purge]` | unmark (keeps SSH access); `--purge` deletes the block |
| `fleet keys list\|sync\|prune` | audit / authorize / remove authorized keys |
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
| `internal/keys` | authorized_keys diff (reports removals, never applies them) |
| `internal/reach` | the wake ladder: rung order, peer ranking, provenance (pure; every impure edge injected via `Deps`) |
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
- **TUI in-flight ownership**: a host is in exactly one of `pending` / `updating` /
  resolved. Refresh skips hosts the update engine owns; every update completion
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
- **The answer form starts empty every wave** — no inherited answers.
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
- **A host that needs waking every run is not healthy — it is power-saving.** The `woke via
  <peer>` note exists to keep that visible instead of smoothing it away. The permanent cure
  is on the host, not in fleet: a Wi-Fi NIC with `power_save on` sleeps through the
  *broadcast* ARP requests a cold neighbour cache must send (`iw dev wlan0 get power_save`
  to check). Disable it persistently with a NetworkManager drop-in —
  `/etc/NetworkManager/conf.d/wifi-powersave-off.conf` containing `[connection]` /
  `wifi.powersave = 2`. Fleet deliberately does **not** apply this for you; see the
  non-mutation invariant.
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
