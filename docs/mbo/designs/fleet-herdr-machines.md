# fleet-herdr-machines — reach a fleet host's herdr from fleet tui and from inside herdr — design

- **Slug:** fleet-herdr-machines
- **Date:** 2026-10-06
- **Status:** Proposed
- **Relates to:** amends [`fleet-connect`](./fleet-connect.md) §4.6 (the herdr provider, leaf D,
  [#292](https://github.com/sfc-gh-eraigosa/dotfiles/issues/292), not yet built) · builds on
  [#370](https://github.com/sfc-gh-eraigosa/dotfiles/pull/370) (`~/.local/bin/herdr` link) ·
  design issue [#371](https://github.com/sfc-gh-eraigosa/dotfiles/issues/371) · design PR [#372](https://github.com/sfc-gh-eraigosa/dotfiles/pull/372)
- **Author(s):** Edward Raigosa, Claude

## 1. Problem / context

Every fleet host runs herdr with coding agents in it. Today, reaching those agents means
`ssh <host>` and then `herdr`, which nests a second herdr inside the local one: both use the
`ctrl+a` prefix, so every chord has to be typed twice. The operator wants two things:

1. from `fleet tui`, go to a host's herdr;
2. from inside herdr, see the fleet's hosts and act on them.

What was verified on 2026-10-06 (herdr 0.9.3 locally):

- **herdr has saved SSH machines.** `herdr machine add <ssh-target> --label <l>` saves a host;
  every open local herdr client then shows it **in its own sidebar**, with its workspaces under
  it, its agents in the combined agent list, and its workspaces in the `prefix+w` switcher.
  Verified end to end on `<nano>`: the operator saw the machine appear and could drive it. No
  nesting, one prefix.
- **`herdr --machine <label> <command>`** runs any CLI command against the saved machine
  without a TUI (`herdr --machine nano workspace list` returned the remote workspaces).
- **No CLI or socket call switches a client's view to a machine.** The docs
  (`herdr.dev/docs/connecting-machines/`) say selection happens in the UI, and "selecting a
  machine in the UI does not retarget CLI commands running in an existing pane". The API schema
  has `workspace.focus`/`agent.focus`, which act on the server they are sent to, and
  `client_shell.surface.set`, an internal client lease, not a selector. `machine list --json`
  exposes a read-only `selected` flag.
- **How `machine add` finds the remote binary:** `HERDR_REMOTE_BINARY` (local override), then
  `command -v herdr` in the remote's **non-interactive** ssh shell, then `~/.local/bin/herdr`.
  A missing or older binary, or an older *running server*, needs approval in an interactive
  terminal; from a non-TTY it refuses and saves nothing. Our installer put herdr in `~/opt/bin`,
  which that shell never has on PATH, so before #370 `machine add` tried to install a second,
  unmanaged copy. #370 links `~/.local/bin/herdr` to the managed binary; after a `fleet update`,
  `machine add <nano>` saved with no prompt.
- **A running older server is the remaining prompt.** `<spark>` runs a 0.9.1 server with live
  agents. `machine add` asks to **stop it** to update, which ends those agents. Background
  connections "never answer prompts or install, update, restart, or hand off a server".
- **`fleet-connect` already plans a herdr provider** (§4.6) whose attach action is
  `herdr --remote <alias> --session <name>` as a TUI handoff. Inside herdr, that is the nesting
  this design avoids. Its binary resolution (`command -v`, `~/opt/bin`, `~/.local/bin`) predates
  #370.

## 2. Goals & non-goals

**Goals**

- G1. One fleet tui key saves the cursor host (or the selection) as a herdr machine, so it shows
  up in the operator's herdr. Already saved = no-op. Approval prompts herdr raises are shown to
  the operator, never answered or suppressed by fleet.
- G2. One CLI verb does the same for scripts and the plugin: `fleet herdr sync [host...]`,
  defaulting to every in-fleet host except the local one.
- G3. A small herdr plugin, shipped from this repo, puts fleet one key away inside herdr: it
  opens `fleet tui` in a herdr popup, and offers a "sync fleet hosts as machines" action.
- G4. Keep `fleet-connect` §4.6's herdr attach (`c`, `herdr --remote`) as designed, and give
  "show this host in herdr" its own key, `R`, instead of overloading `c`. Two keys with two
  meanings, each the same everywhere.

**Non-goals**

- Switching the operator's herdr view programmatically. herdr has no API for it; we tell the
  operator `prefix+w` instead. If herdr adds one, it slots into G1 with no other change.
- Restarting or updating a remote herdr server from fleet. That ends the agents on it; it stays
  the operator's call at herdr's own prompt.
- A second host inventory. `~/.ssh/config` `#fleet` blocks stay the only one; herdr machines are
  a projection of it, never a source.
- Removing machines when a host leaves the fleet (`fleet remove`). Could come later; v1 only
  adds.
- Reimplementing fleet's host list as a native herdr plugin UI.

## 3. Options considered

**A. fleet does the work; the herdr plugin is a thin launcher (recommended).** fleet gains one
TUI key and one CLI verb that wrap `herdr machine list --json` / `herdr machine add`. The plugin
is two shell actions: open `fleet tui` in a herdr popup, and run `fleet herdr sync`. Everything
the operator already knows in fleet tui (status, update, wake, ssh, history) is available in
herdr for free, with fleet's guards intact. One implementation, testable in Go with the
existing runner seam.

**B. A standalone herdr plugin with its own host UI.** It would parse `fleet status --json` and
draw its own list. It duplicates fleet tui's rendering, selection, and update guards in a second
language, and drifts the first time fleet adds a column. Rejected.

**C. Documentation only.** Tell the operator to run `herdr machine add` per host. Zero code, but
every new host and every reinstall is a manual step, and nothing ties it to the fleet inventory.
Rejected as the end state; it is the fallback today.

**Within A, where the herdr attach lives:** `fleet-connect`'s herdr provider (#292) is three PRs
away from being buildable (it needs the protocol #290 and registry #291). The save-as-machine
piece does not need that framework, so it ships now as a small package the provider later
reuses, instead of waiting for it.

## 4. Decision

Option A, in three units.

### 4.1 `internal/herdrmach` (new, pure + one runner seam)

- `List(ctx, r runner.Local) ([]Machine, error)` runs `herdr machine list --json` locally.
  `Machine{ID, Label, Target, Session string; Enabled, Selected bool}` mirrors the verified JSON.
- `Saved(ms []Machine, alias string) (Machine, bool)` matches on `Target == alias` (the ssh
  alias), not on label, so a renamed machine is still recognised.
- `AddArgv(alias string) []string` returns `["herdr","machine","add",alias,"--label",alias]`.
  The label is the fleet alias, so the herdr sidebar and fleet tui use the same name.
- `LocalBinary()` resolves the local herdr the same way `fleet-connect` §4.6 `Deps.LocalBinary`
  would, so the provider can adopt this package unchanged.
- No remote probing of its own. Whether a host *has* herdr stays with `fleet-connect`'s provider.
  Here, `machine add` is the probe, and its error text is surfaced verbatim.

### 4.2 fleet (TUI key + CLI verb)

- **TUI key `R`** ("remote herdr"; free in today's keymap: `r` is refresh, `h`/`H` are taken).
  It acts on the
  selection, else the cursor host, by the same `updateTargets()` rule as update and wake.
  - Skips: the local host (`isLocalHost`, the `⌂` row), hosts with an update in flight, hosts
    already saved (by target).
  - No local herdr: a status-line message, no action.
  - Otherwise it runs `machine add` as a **`tea.ExecProcess` handoff**, like `s`, one host at a
    time, so herdr's install or stop-server prompt reaches the operator. Background execution is
    wrong here: herdr refuses without a TTY.
  - Afterwards: inside herdr, the status line says `<alias> saved — prefix+w to switch to it`;
    outside herdr, `<alias> saved — open it with herdr --remote <alias>`.
  - Added to `keyHelp` and to `pkg/provider.ReservedKeys` (`R` is not reserved today), which
    `TestEveryFleetKeyIsReservedAgainstProviders` enforces.
- **CLI `fleet herdr sync [host...]`.** It defaults to every in-fleet host except the local one,
  serially. Saved hosts print `already saved`. A host whose `machine add` needs approval is not
  attempted from a non-TTY: it prints `needs approval — run: herdr machine add <alias>` and the
  verb exits non-zero, so a script sees it. With a TTY it hands off like the TUI key.
  `--dry-run` prints the argv; `--json` reports per-host `saved | already | needs-approval |
  failed`.

### 4.3 The herdr plugin `fleet` (shipped from this repo)

- Lives at `ai/herdr/plugins/fleet/` (`herdr-plugin.toml` + `scripts/*.sh`); no build step.
- Two `[[actions]]`:
  - `open-fleet`: opens a herdr popup (80% × 80%) running `fleet tui`. Quitting fleet closes it.
  - `sync-machines`: opens a temporary pane running `fleet herdr sync`, so approval prompts
    still have a terminal.
- Keys in `ai/herdr/plugins/fleet.toml`: `prefix+shift+h` → `open-fleet` (free in herdr 0.9.3
  defaults and in our managed config). No key for sync; it is reachable from fleet tui (`a`,
  then `R`).
- Install: a row in `ai/herdr/plugins.tsv` and the gff flag `install.herdr-plugin.fleet`, **on by
  default** (operator decision, 2026-10-06) wherever `install.tools.fleet` is on. Because the plugin's source is this repo, the
  installer **copies** it to `~/.config/herdr/plugins/local/fleet/` and runs `herdr plugin link`
  on the copy, which follows the repo rule "copy into well-known `$HOME` paths; no new symlinks
  into the checkout". The installer's existing "a `herdr plugin link`ed plugin is left alone"
  branch needs one exception for this repo-owned row, keyed on the copy's path.
- The scripts call `fleet` by name. A host without fleet gets a popup that says so and exits.

### 4.4 Amendment to `fleet-connect` §4.6 (applied in its own spec/plan when #292 is built)

- The herdr provider's `c` stays attach (`herdr --remote <alias> --session <name>`) in every
  context, as planned. Inside herdr that nests, so the provider's row label points at `R`
  (`c: attach · R: show in herdr`). `R` is a fleet key and works on any host row, including
  from the drill-down. Providers can't declare it, so the meaning can't fork (operator decision,
  2026-10-06: a separate key, not a context-dependent `c`).
- Remote binary resolution order becomes `command -v herdr`, then `~/.local/bin/herdr`. That is
  herdr's own order, and after #370 both resolve to the managed binary. `~/opt/bin/herdr` stays a
  fallback for hosts not yet updated.

### 4.5 Operator decisions (review of #372, 2026-10-06)

1. "Show in herdr" gets its own key, `R`, rather than changing what `c` does inside herdr.
2. The `fleet` herdr plugin is on by default.
3. `prefix+w` is an acceptable way to switch to a saved machine until herdr offers an API for it.

## 5. Risks & blast radius

| Risk | Likelihood | Mitigation |
| :-- | :-- | :-- |
| `machine add` stops a running older server and ends its agents (`<spark>` today) | Medium | fleet never answers the prompt. herdr asks the operator in the handoff and describes what it will stop. The CLI verb refuses from a non-TTY. The `fleet tui` status line warns when the host's update captured a herdr version change. |
| `fleet update` replaces the binary but the running server stays old, so the next `machine add` prompts | High (every update) | Documented. The prompt is herdr's and is correct. A follow-up could restart idle servers, but only an idle one, and only behind a flag. Out of scope. |
| herdr renames `machine list --json` fields or the subcommand | Low–Medium (0.x) | Pure parser with captured fixtures and the herdr version in each fixture's header. An unknown shape degrades to "cannot read herdr machines", never to a duplicate add. |
| Plugin popup runs `fleet tui` without a usable terminal size | Low | Popup sized in percent; fleet tui already handles narrow frames (the #306 measure-and-correct `View()`). |
| Saved machines accumulate for hosts that left the fleet | Low | Non-goal for v1. `herdr machine remove` is manual. Noted for a follow-up. |
| A plugin from this repo is unsandboxed code | Low | It is our own code, reviewed like any script here, and runs no network fetch or build. |

Blast radius: local herdr's saved-machine list, plus the remote herdr server **only** when the
operator approves herdr's own prompt. No fleet inventory change, no ssh-config change.

## 6. Rollback

- `gff set install.herdr-plugin.fleet false`, then `herdr plugin unlink fleet`.
- The `R` key and `fleet herdr sync` are additive; reverting the PR removes them.
- Saved machines persist in herdr. Remove one with `herdr machine remove <id>`. The remote
  server keeps running, which herdr documents.

## 7. Evidence expectations

- **Unit:** `herdrmach` parser over **captured** `herdr machine list --json` (0.9.3) including
  an empty list and an unknown-field shape. `Saved` matches on target, not label. `AddArgv`
  with a hostile alias stays one argv element. TUI tests: `R` skips local, in-flight and saved
  hosts, issues one `ExecProcess` per remaining target in order, and sets the right status line
  for `HERDR_ENV` on/off.
- **CLI transcripts:** `fleet herdr sync --dry-run`, `--json` over a fake runner, and the
  non-TTY `needs-approval` exit code.
- **Live (operator):** `fleet herdr sync` on a host with no running server (`<nano>` is
  already saved, so it is the `already saved` case; a fresh host is the `saved` case), then a
  screenshot of the machine in the herdr sidebar. `prefix+shift+h` opens fleet tui in a popup,
  and `R` on a host shows it in the sidebar.
- **Install:** `install_herdr_test.sh` cases for the repo-owned plugin row (copied, linked,
  re-run is a no-op, a flag set to `false` installs nothing).

> Produced via `superpowers:brainstorming`-style option analysis in a live session with the
> operator (2026-10-06). Register the objective in `../index.md`. The matching spec goes in
> `../specs/fleet-herdr-machines.md`.
