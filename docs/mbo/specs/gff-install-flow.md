# install.sh flag-flow refactor — prompt-early / Windows-last + early export — spec

- **Slug:** gff-install-flow
- **Date:** 2026-07-26
- **Status:** Approved
- **Relates to:** gff objective #180 (closed) · TRACKING §10 rows 3–5 (`../plans/gff/TRACKING.md`) · spec `gff.md` §F9 · owner-proposed 2026-07-26

## 1. Goal

`gff set install.windows.<x> false && ./install.sh` just works — no calling-shell
`set -a; eval …` incantation, on every machine including a fresh one, for **all** 43
flags including the UAC-elevated Windows gates. The interactive prompts stay
front-loaded (sudo, y/n/s within the first minute); the Windows execution moves to
the end of the run where the gff bootstrap has already materialized the flags. The
"never ask again" skip state moves out of a hidden sentinel file into a gff
user-override — visible in `gff list`/TUI, reversible with `gff unset`.

## 2. Use cases

- **Owner disables a Windows component / actor:** owner on WSL. Trigger: `gff set
  install.windows.wispr-flow false; ./install.sh`. Flow: early ask records y; run
  proceeds; Windows executes last with flags exported by the in-script bootstrap;
  the elevated batch receives the flag via the `-GffEnv` argument. Acceptance: the
  elevated log shows `SKIP (gff: install.windows.wispr-flow=false)`; no manual
  export step was needed.
- **Owner permanently opts out of Windows setup:** trigger: answers `[s]` at the
  early prompt. Flow: the deferred executor runs `gff set
  install.windows.desktop-deploy false` (sentinel file only if gff is genuinely
  unavailable). Acceptance: `gff list install.windows.desktop-deploy` shows
  `false · user-override`; the next run never prompts; `gff unset` re-enables.
- **Legacy machine with the old sentinel:** flow: any run that finds
  `~/.config/dotfiles/.skip_windows_setup` AND a working gff migrates it (gff set +
  delete file). Acceptance: sentinel gone, override present, behavior identical.
- **Fresh system, first run:** no gff anywhere. Flow: early export no-ops
  (fail-open); prompts behave as today; bootstrap builds gff mid-run; Windows-last
  still sees exported flags. Acceptance: run completes identically to today's
  all-on behavior when no overrides exist.
- **Unattended run answered y earlier:** UAC fires at the END of the run. If the
  prompt times out / is cancelled, the run must say so loudly with a rerun hint —
  never silently skip. Acceptance: warning text names the elevated script path.

## 3. Architecture

Three touched components, boundaries unchanged elsewhere:

- **`install.sh`** — orchestration only: (a) NEW early-export block immediately
  after the `opt/lib/gff.sh` source (fail-open, `command -v gff` guarded);
  (b) the early Windows block becomes `install_windows.sh --ask`; (c) NEW deferred
  execution `install_windows.sh --deferred` at the tail, after the nerd-font
  section and before the Wispr banner (the `WIN_SETUP_MARKER` contract is
  unchanged: `--deferred` writes it, the banner reads it).
- **`opt/bin/install_windows.sh`** — gains a mode switch. `--ask`: WSL detect →
  skip-state check (legacy sentinel OR `gff_on install.windows.desktop-deploy`) →
  existing `/dev/tty` y/n/s prompt → persist choice to
  `~/.cache/dotfiles/win-setup-choice`. `--deferred`: read+delete the choice file,
  re-check `gff_on`, then today's full deploy + PowerShell chain (incl. the
  WSLENV `/w` builder). No argument = today's monolithic flow (direct-run
  compatibility). `[s]` handling delegates to gff (fallback: sentinel).
- **PowerShell pair** — `setup-apps.ps1` serializes its `GFF_INSTALL_WINDOWS_*`
  env into a `;`-separated `NAME=value` string and passes it to the elevated child
  as a `-GffEnv` parameter (crossing the UAC boundary as an argument, since env
  does not survive `Start-Process -Verb RunAs` — TRACKING §10 row 5).
  `setup-elevated.ps1` validates each pair against
  `^GFF_INSTALL_WINDOWS_[A-Z_]+=(true|false|[a-z0-9,-]+)$` and seeds `$env:` so
  the existing `Test-GffOn` works unmodified. Elevated log moves to
  `$env:USERPROFILE\setup-elevated.log`.

## 4. Behavior / features

- **F1 early export** — pre-bootstrap gates (`install.system/shell/pkg/tools.*`)
  honor overrides from run 2 onward; no-op without a gff binary.
- **F2 prompt-early / Windows-last** — y/n/s captured in the first minute;
  execution after the bootstrap; `install.windows.*` overrides work with zero
  manual steps on any machine state.
- **F3 gff-owned skip state** — `[s]` ⇒ `gff set install.windows.desktop-deploy
  false`; sentinel is fallback-only and migrated away when possible.
- **F4 UAC argument hand-off** — elevated gates (wispr-flow, copilot-key,
  ahk-autostart) become reachable; closes TRACKING §10 row 5.
- **F5 readable elevated log** — evidence readable without a second elevation.
- **F6 loud UAC failure** — cancelled/timed-out elevation prints a warning naming
  the script and how to rerun it; never a silent skip.

## 5. Evaluation criteria (per feature)

- **F1**: with `GFF_…` absent and gff on PATH + an override set, a pre-bootstrap
  gate's key resolves false before line ~67 · must-not-fire: fresh system (no gff)
  changes nothing · pass: probe run shows the early SKIP.
- **F2**: prompt appears before any package work; no Windows execution before the
  bootstrap; `--deferred` runs exactly once per `--ask` answer `y` · edge: Ctrl-C
  between ask and deferred leaves a stale choice file that the next run overwrites
  at `--ask` time · pass: ordered transcript.
- **F3**: `[s]` ⇒ override present (`gff list` shows `user-override`), no sentinel;
  sentinel+gff ⇒ migrated; sentinel+no-gff ⇒ honored · pass: state inspection
  after each path.
- **F4**: elevated log shows `SKIP (gff: install.windows.wispr-flow=false)` when
  that flag is false — the originally-scripted P2-T5 line, now achievable · must-
  not-fire: with no overrides, elevated batch behaves as today · edge: malformed
  `-GffEnv` pair is rejected (validation regex) and logged, batch continues
  fail-open · pass: elevated log capture.
- **F5**: `cat` of the user-profile log works from WSL via `/mnt/c` with no
  elevation · pass: the validation transcript includes it.
- **F6**: declining the UAC prompt produces the warning + rerun hint in the WSL
  terminal · pass: captured in the `[y]`-then-cancel probe or code-inspection +
  one live decline.

## 6. Verification harness

- **Automated:** `bash -n` on both scripts; `make lint-shell && make
  lint-portability` clean (mandatory per shell commit); a non-interactive test
  driver for the mode switch (`--ask` with no tty ⇒ the existing `__notty__`
  message, choice-file write/read/delete round-trip, `[s]`-to-gff delegation with
  a stubbed `gff`) mirroring the `opt/lib/gff_test.sh` assert style.
- **Human-evidenced (real WSL terminal, never piped/backgrounded):** the four-path
  matrix — `[y]` with `install.windows.wispr-flow=false` (expect the elevated-log
  SKIP), `[n]`, `[s]` (expect the gff override), and
  `install.windows.desktop-deploy=false` (expect no prompt, one SKIP line).
  Transcripts committed under `docs/mbo/plans/gff/evidence/F09-gating/`.

## 7. Prerequisites / dependencies

gff merged main (all four leaves + #191 `/w` fix). No new tools. The plan §3.5
shell contract stays frozen except the already-owner-approved `/w` correction;
this spec adds behavior around it, not changes to `gff_on`/`Test-GffOn` semantics.

## 8. Out of scope (and why)

- The three unenforced `install.windows.*` flags (`claude-rc-autostart`, `sshd`,
  `portproxy`) — their scripts still have no invocation sites (TRACKING §10 row 4).
- Multi-source install.sh (`gff sources`-driven) — separate design sketch.
- TUI gif/asciinema for the README — optional, independent.
- Any reordering of the Linux-side component blocks — only the Windows phase moves.

## 9. Rollback

Single revert of the feature PR restores today's flow. The gff override written by
`[s]` is data, not code — it keeps working (or is simply unread) across a revert
because `install.windows.desktop-deploy` gating predates this change. A migrated
(deleted) sentinel would need `[s]` to be re-answered after a revert — acceptable.

> Produced via `superpowers:brainstorming`. The matching plan goes in `../plans/gff-install-flow.md`.
> Registered in `../index.md`.
