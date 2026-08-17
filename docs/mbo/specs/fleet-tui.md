# fleet tui v2 — interactive fleet dashboard — spec

- **Slug:** fleet-tui
- **Date:** 2026-08-16
- **Status:** Draft
- **Relates to:** design `../designs/fleet-tui.md` · parent objective `fleet` (PR #224, merged) · issue [#226](https://github.com/sfc-gh-eraigosa/dotfiles/issues/226)

## 1. Goal

`fleet tui` becomes a real dashboard: it opens instantly and streams host rows
in as they answer; the operator navigates with vim keys across a paged,
status-colored table, finds hosts with `/` regex search, selects several
vim-style, and acts on them — batch update (to `main` or `--update-ref`),
or drop into an SSH shell — without ever leaving the TUI or losing state.

## 2. Use cases

**UC1 — morning sweep.** Operator / runs `fleet tui` / TUI opens instantly,
rows stream in with spinners on the stragglers, worst-first / acceptance: first
frame renders before any SSH completes; an unreachable host resolves red, the
UI never hangs on it.

**UC2 — find and fix the stale ones, without waiting.** Operator / types
`/nano|pi`, sees matches highlighted, `n` to hop, `space` on two hosts, `u` /
TUI lists both targets with their precheck class, confirms, and **both hosts
update concurrently in the background** — rows tick `updating ⠋` while the
operator keeps navigating, searches, even presses `r` (updating hosts
excluded) / acceptance: both rows show fresh stamps without the TUI ever
blocking; a declined confirm changes nothing; a host that needs a sudo
password is routed to the interactive handoff after the background wave, and
its prompt reaches the operator.

**UC3 — big fleet.** 60 hosts, 30-row terminal / `ctrl+d`, `G`, `gg` move the
viewport; position indicator shows `n/60` / acceptance: cursor is always
visible; header never scrolls away.

**UC4 — inspect a box.** Operator on a red row presses `s` / terminal handed
to plain `ssh <host>` / on exit the TUI resumes exactly where it was.

**UC5 — pre-merge validation.** `fleet tui --update-ref feature/x` / `u` runs
the update against that ref (validRef-guarded) / acceptance: identical
semantics to `fleet update --ref feature/x`.

**UC6 — the host that is asleep, not dead.** A Wi-Fi power-saving host has
dropped out of the workstation's ARP cache; a direct probe fails at layer 2
before SSH is attempted / fleet escalates the wake ladder inside the normal
concurrent poll, reaches the host through a peer, and the host's own return
traffic repopulates the cache / acceptance: the row resolves to its real drift
class annotated `woke via <peer>` instead of a permanent red `unreachable`;
the operator did nothing and the fleet's wall-clock poll time grew by at most
one wake budget. **Negative case:** a genuinely powered-off host still resolves
`unreachable` after the ladder is exhausted — wake must never report a host
reachable on the strength of a relay hop alone.

**UC7 — deliberate wake.** Operator knows a box is asleep / `fleet wake <host>`
(or `w` on the TUI row) / acceptance: the ladder prints rung by rung, exits
non-zero if the host stayed down, and mutates nothing on the target.

## 3. Architecture

Per design §4: one package `sdk/fleet/cmd`, files `tui.go` (wiring),
`tui_model.go` (state machine), `tui_view.go` (pure render + theme),
`tui_keys.go` (keymap + `gg` pending state), `tui_cmds.go` (tea.Cmd
producers). Reuses unchanged seams: `collect`/`collectOne` fan-out over
`runner.Runner`, `remoteUpdateScript(ref)` + `validRef`, `sortWorstFirst`,
`drift`/`stamp` packages. **Every unit except `tui_cmds.go` is a pure
function of model data** and is tested with the lipgloss profile pinned to
ASCII.

Model state (single struct): `rows []Row` + `pending map[alias]bool`,
`cursor alias`, `viewport {top, height, width}`, `mode` (normal · search ·
confirm · help), `search {input, re, err, matches}`, `selected map[alias]bool`
+ `visualAnchor *alias`, **update engine:** `updating map[alias]updState`
(queued · precheck · running · ok · fail + captured log tail), `bgQueue`
+ `iaQueue []alias` (background wave / interactive fallback), `jobs int`
(slots free), `status string`.

**In-flight ownership invariant:** a host is in exactly one of `pending`,
`updating`, `waking`, or resolved; refresh excludes `updating` and `waking`
hosts; every update or wake completion clears its claim, records the outcome,
and re-fires `collectOne`.

Per design §3.1/§4.1, wake lives in a new pure package `internal/reach`, not in
`cmd` and not in the `runner` decorator chain. Its public surface:

```go
type Peer struct{ Alias, HostName string }
type Policy struct { Enabled bool; Budget time.Duration; Retries int }
type Attempt struct { Rung, Via string; OK bool; Err string }   // Rung: retry|local-prime|peer-relay
type Result  struct { Woke bool; Via string; Attempts []Attempt }

func Wake(ctx context.Context, target Peer, peers []Peer, p Policy, d Deps) Result
```

`Deps` injects every impure edge — `Probe func(alias string) error`,
`runner.Runner`, `Resolve func(string) ([]net.IP, error)`,
`LocalAddrs func() ([]net.IPNet, error)`, `PingLocal func(ctx, ip string) error`,
`Sleep func(time.Duration)` — so the escalation policy is tested with zero
sockets and zero real elapsed time. `runner.Runner` gains one method,
`RunVia(peer, host string, argv ...string)`, implemented by `Exec` (`ssh -J`)
and `Fake`.

## 4. Behavior / features

| # | Feature |
| :-- | :-- |
| F1 | Instant open + streaming rows: per-host async collect, spinner on pending, worst-first re-sort on arrival, alias-pinned cursor |
| F2 | `r` refresh: re-polls all hosts in place; cursor + selection survive; works during updates (updating hosts excluded) |
| F3 | Styled table: status-colored rows, styled header, cursor row, `n/total` position, terminal-degradation safe |
| F4 | Vim motion: `j/k`/arrows, `gg`/`G`, `ctrl+d/u` half-page, `ctrl+f/b` page; viewport follows cursor; window resize handled |
| F5 | `/` search: incremental regex, smartcase, live match highlight, `enter` commit, `esc` cancel; invalid pattern → inline error, input keeps working |
| F6 | `n`/`N` next/prev match with wraparound |
| F7 | Selection: `space` toggle, `v` visual range via motions, `esc` clear; keyed by alias; selection count in status bar |
| F8 | Concurrent batch update: `u` = selection or cursor row; confirm strip lists targets + precheck class; background wave runs ≤ `--jobs` hosts at once (BatchMode ssh, output captured, row ticks queued→updating→ok/FAIL); TUI stays interactive; auto re-poll each completed host |
| F9 | `--update-ref` flag (default `main`) + `--jobs` (default 4, ≥1), validRef-guarded at command start |
| F13 | Interactive fallback: hosts failing the `sudo -n true` precheck queue for serial `ssh -t` handoff *after* the background wave; declined/failed handoffs advance the queue |
| F10 | `s` SSH shell to cursor host via terminal handoff; TUI resumes after |
| F11 | `?` help overlay listing all keys; any key closes |
| F12 | Empty fleet: guidance to `fleet discover` / `fleet add` (kept from v1) |
| F14 | Wake ladder (`internal/reach`): `retry` → `local-prime` → `peer-relay`, whole sequence under one deadline, stops at the first rung whose **direct** re-probe succeeds |
| F15 | Auto-wake: any probe classifying `unreachable` runs the ladder once inside the existing concurrent fan-out, then re-probes; success annotates the row `woke via <via>`; applies to `status`, `tui`, and `update` alike |
| F16 | `fleet wake [host...]`: explicit verb, prints the ladder rung by rung, `--json`, exit non-zero if any target stayed unreachable |
| F17 | TUI `w`: wake the selection (or cursor host) in the background lane; row ticks `waking ⠋ → woke/unreachable`; listed in `keyHelp` |
| F18 | Flags: `--no-wake` disables the ladder everywhere; `--wake-timeout` (default 8s) bounds it; both persistent on the root command |

## 5. Evaluation criteria (per feature)

Every rule below becomes a named test. Format: **trigger · fires · must-not-fire · pass**.

- **F1a** program start · one `collectOne` cmd per host + spinner tick issued · no synchronous SSH before first frame · `Init()` returns n+1 cmds; view with all-pending renders spinners.
- **F1b** `hostRowMsg` arrives · row replaces pending, list re-sorts worst-first · cursor alias unchanged by re-sort · frame shows the row; cursor still on its alias.
- **F1c** unreachable host resolves · red `unreachable` row · no hang/drop · fake error runner yields rendered row.
- **F2a** `r` in normal mode · all rows → pending, `collectOne` refired per host · selection + cursor survive · model shows n pending; selection set unchanged.
- **F2b** `r` while hosts are updating · only non-updating hosts re-polled · an `updating` host must never be double-owned (no `collectOne` fired for it) · exclusion test with 2 updating + 2 idle hosts.
- **F3a** every status class · its theme color; ASCII profile → plain text byte-identical to a golden frame · colors leak into tests · golden frame matches.
- **F4a** `j`/`k` at list edges · cursor clamps · no wrap, no panic on empty · bounds test.
- **F4b** `gg`/`G` · jump to first/last row and scroll viewport · `g` alone does nothing until second `g`; any other key cancels the pending `g` · sequence test.
- **F4c** `ctrl+d/u/f/b` · viewport moves half/full page and cursor stays visible · header never scrolls off · paging math test at both extremes.
- **F4d** `WindowSizeMsg` · viewport height/width adopt; rows re-fit · cursor stays visible after shrink · resize test.
- **F5a** `/` then text · regex compiled per keystroke; matches highlighted; smartcase (all-lower = `(?i)`) · typing `/` chars must not trigger normal-mode keys · mode-routing test.
- **F5b** invalid regex (e.g. `[`) · inline error, previous matches kept, input editable · no panic, no crash · error-state test.
- **F5c** `esc` in search · search cleared entirely; `enter` · pattern committed, mode → normal · committed pattern persists for n/N · state test.
- **F6a** `n`/`N` with committed pattern · cursor to next/prev match, wrapping · no-op without matches (status "no matches") · wrap test both directions.
- **F7a** `space` · toggles cursor row's alias in selection · acts on alias not index (re-sort keeps it) · toggle + re-sort test.
- **F7b** `v` then motions · range anchor→cursor selected live; `space`/`esc` commits/cancels · plain motions after esc don't extend · visual-mode test.
- **F8a** `u` with selection · confirm strip lists exactly the selected aliases in table order (+ precheck class) · `u` with empty selection targets cursor row only · target-list test.
- **F8b** confirm `y` · prechecks fired, background wave starts filling job slots · confirm `n`/`esc` → nothing runs, selection kept · declined test (mirror of keys-prune UC).
- **F8c** a background update completes · ok/fail + captured log recorded, host re-polled, next queued host takes the slot · a failing host must not stop the wave · slot-advance test with an erroring fake.
- **F8d** more targets than `--jobs` · at most `jobs` hosts in `running` at any instant · slots refill as completions arrive · saturation test (5 targets, jobs=2, assert running ≤ 2 at every step).
- **F8e** background update command · built with `BatchMode=yes` over the runner seam · never `tea.ExecProcess`, never suspends the TUI · argv assertion via recordingRunner.
- **F8f** keystrokes during a background wave · navigation/search/refresh all function · no mode lockout · interleaved-messages test (key events between completion msgs).
- **F9a** `--update-ref 'main; rm -rf ~'` · command errors before any host contact · valid refs accepted · reuse of `validRef` test pattern.
- **F9b** `--jobs 0` / negative · rejected at command start · `--jobs 1` = serial background (still no handoff) · flag-validation test.
- **F13a** precheck fails for a host (`sudo -n` non-zero) · host routed to `iaQueue`, runs via `ssh -t` handoff after the background wave completes · a passing host must never be handed off · routing test with a mixed fake fleet.
- **F13b** each handoff returns (ok, error, or declined) · result recorded, host re-polled, next handoff fired · the queue never wedges · fallback-advance test.
- **F10a** `s` · `ssh <host>` ExecProcess for cursor alias; on return TUI intact · no-op on empty fleet · cmd-construction test.
- **F11a** `?` · overlay renders complete keymap; any key closes · overlay suppresses normal-mode keys underneath · overlay test.
- **F12a** zero hosts · discover/add guidance rendered · no panic on any key · kept v1 test.
- **F14a** probe fails, ladder runs · rungs attempted in order retry → local-prime → peer-relay · a later rung must never run once an earlier one's direct re-probe succeeded · fake `Probe` succeeding at rung 1 records exactly one `Attempt`.
- **F14b** peer relay succeeds at the hop but the direct re-probe still fails · `Result.Woke == false`, class stays `unreachable` · **never** report woke on relay success alone · fake where `RunVia` returns nil but `Probe` keeps erroring.
- **F14c** `local-prime` rung · runs **only** when a `LocalAddrs` subnet contains a `Resolve`d target IP · must be skipped when the workstation is off-subnet (the WSL2 NAT case) · two-table test: on-subnet fires `PingLocal`, off-subnet records the rung skipped and fires nothing.
- **F14d** peer ordering · already-reachable-this-run peers first, then same-subnet · an unreachable peer must not be tried before a reachable one · ordering test over a mixed candidate set.
- **F14e** budget exhausted mid-ladder · ladder returns with `Woke false` and the attempts so far · no rung may outlive the deadline · cancelled `context` test asserting the next rung never fires.
- **F14f** no peers available (single-host fleet) · ladder degrades to retry (+ local-prime if on-subnet) · must not panic or dial itself · single-host test; target is never its own peer.
- **F14g** ladder work `Sleep`s only through the injected clock · unit tests complete with no real elapsed time · timing assertion in the reach suite.
- **F15a** `collect` with one unreachable host · ladder fires once for that host, then re-probes · a **reachable** host must never invoke the ladder · call-count assertion on a fake.
- **F15b** wake succeeds · `Row.Note == "woke via <via>"` and the class is the *re-probed* class, not `unreachable` · note must not appear for hosts that never slept · row-content test.
- **F15c** N unreachable hosts · all ladders run concurrently within the existing fan-out · wall clock ≈ one budget, not N · saturation test asserting max observed concurrency > 1 and total elapsed < N × budget.
- **F15d** `--no-wake` · no rung runs anywhere; `unreachable` reported immediately · the flag must also suppress auto-wake in the TUI · flag test across `status` + model.
- **F16a** `fleet wake <host>` on a wakeable host · prints each rung with its outcome, exits 0 · silent success is a failure (the ladder is the diagnostic) · golden-output test.
- **F16b** `fleet wake` on a dead host · exits non-zero, prints the exhausted ladder · must not report success · exit-code test.
- **F16c** `fleet wake --json` · emits the full `[]Attempt` per target · schema stable · JSON round-trip test.
- **F16d** wake against any target · issues only `echo $SSH_CONNECTION`, `ping`, and the probe · **no** command that writes to the target · recordingRunner argv assertion (mirrors F8e's discipline).
- **F17a** `w` with a selection · every selected host enters `waking` in the background lane · must not use `tea.ExecProcess` (never suspends the TUI) · argv + cmd-type assertion.
- **F17b** host in `waking` · `r` refresh skips it; `u` cannot claim it · the in-flight ownership invariant extends to wake · double-ownership test with mixed `updating`/`waking` rows.
- **F17c** wake completion · clears `waking`, records the outcome, re-fires `collectOne` for that host · queue must never wedge on a failed wake · completion-advance test.
- **F17d** `?` help · lists `w` · `keyHelp` stays the single source of truth (no second hand-written list) · overlay test asserting `w` present.
- **F18a** `--wake-timeout 0`/negative · rejected at command start · valid durations accepted · flag-validation test.
- **F18b** `ping` invocation · never passes `-W` · bounded by `exec.CommandContext` instead · argv assertion (GNU seconds vs BSD milliseconds trap, design §4.1).

## 6. Verification harness

- **Layer 1 — pure frames:** golden `View()` outputs under
  `lipgloss.SetColorProfile(Ascii)`; every visual state in design §7.
- **Layer 2 — model transitions:** table-driven `Update()` tests, one per §5
  rule; fake `runner.Fake`/`recordingRunner` for anything touching SSH.
- **Layer 3 — command seams:** `collectOne`, batch-queue sequencing and ssh
  cmd construction asserted via recorded argv (no terminal in CI).
- **Layer 4 — live (human-gated):** tmux capture of streaming, search,
  **two hosts updating concurrently while the operator navigates and
  refreshes**, an interactive-fallback handoff with a real sudo prompt, ssh
  action, post-update re-poll. Evidence committed per design §7.
- **Layer 5 — wake reproduction (human-gated):** the §1.1 incident replayed on
  real hardware — local neighbour table showing **no entry** for the sleeper,
  `fleet wake` escalating to `peer-relay`, the neighbour table then showing a
  resolved MAC, and the row landing on its true class with `woke via <peer>`.
  Only this layer proves the mechanism; every other layer proves the policy.
- **Coverage:** fleet module stays ≥ 60% floor; target cmd package ≥ 55%
  (view/model split makes previously-unreachable render logic testable).

## 7. Prerequisites / dependencies

`bubbles/spinner` (+ transitively present lipgloss) promoted to direct deps;
license gate must stay green. No install.sh / gff / CI wiring changes — the
`fleet` binary and build path are untouched.

## 8. Out of scope (and why)

Mouse, keymap config, watch mode, bubbles/table (design §3), any change to
headless command semantics, Windows-native terminal testing (lipgloss owns
degradation).

**Wake specifically excludes:**

- **Remediating the sleeper** (disabling Wi-Fi power save on the target).
  That is the real cure, but it reconfigures a machine's networking as a side
  effect of a read path — see design §2 non-goals. Documented as an operator
  runbook step instead.
- **Wake-on-LAN** (`sdk/wol`). WoL addresses a *powered-off* host and needs
  layer-2 reach plus a MAC map; this problem is a *powered-on* host that is
  merely asleep, and the workstation may have no layer-2 presence at all
  (design §1.1). Different failure, different tool.
- **Raw ARP / `arping` from the workstation.** A no-op under WSL2 NAT, which is
  the environment that exhibits the bug (design §1.1).
- **Persistent wake state / "known dead" caching** (design §3.1 option C).

## 9. Rollback

Revert the PR; v1 TUI returns. No persistent state anywhere.

> Produced via `superpowers:brainstorming`. Plan: `../plans/fleet-tui.md`.
