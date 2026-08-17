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
`updating`, or resolved; refresh excludes `updating` hosts; every update
completion clears `updating`, records ok/fail, and re-fires `collectOne`.

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

## 9. Rollback

Revert the PR; v1 TUI returns. No persistent state anywhere.

> Produced via `superpowers:brainstorming`. Plan: `../plans/fleet-tui.md`.
