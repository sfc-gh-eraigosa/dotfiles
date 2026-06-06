# claude-config — spec

- **Slug:** claude-config
- **Date:** 2026-06-07
- **Status:** Approved
- **Relates to:** PR #126 · consumer: playground PR #73 (`claude-local`)

## 1. Goal

Give the `claude` shell wrapper a small, discoverable launch-config tool —
`claude-config` — that controls two opt-in launch flags via sentinel files under
`$XDG_CONFIG_HOME/claude`: YOLO (`--dangerously-skip-permissions`) and Remote
Control (`--remote-control` on interactive sessions). Both default **OFF**, so a
fresh or shared machine behaves like plain `claude` until the user explicitly
opts in per machine. Renames the former `claude-toggle` (YOLO-only) to
`claude-config`.

## 2. Use cases

- **Enable remote-control on a machine** — *actor:* user · *trigger:*
  `claude-config remote on` · *flow:* creates `~/.config/claude/remote.enabled`;
  subsequent interactive `claude` invocations inject `--remote-control <dir>` ·
  *acceptance:* sentinel exists; an interactive `claude "fix bug"` runs as
  `command claude --remote-control <dir> "fix bug"` with the prompt intact.
- **Run headless / piped unchanged** — *actor:* user/SDK · *trigger:*
  `claude -p "..."` or a piped/redirected stdin · *flow:* the wrapper detects
  print mode or a non-TTY and skips `--remote-control` · *acceptance:* no
  `--remote-control` injected; `--dangerously-skip-permissions` still honored if
  YOLO is on.
- **Inspect state** — *actor:* user · *trigger:* `claude-config` /
  `claude-config status` · *flow:* prints `yolo` and `remote` as `ON`/`OFF` ·
  *acceptance:* both default to `OFF` with no sentinels present.
- **Local-model sibling keeps working** — *actor:* playground `claude-local` ·
  *trigger:* user runs `claude-local` in the same shell · *flow:* it reads
  `${CLAUDE_YOLO_FILE:-}` and calls `command claude` directly · *acceptance:* it
  still applies YOLO from the same sentinel and is never given `--remote-control`
  (see §7 contract).

## 3. Architecture

Single sourced file `ai/claude/aliases.sh` (loaded by `.zshrc`/`.bashrc`; runtime
shell is always bash or zsh). Components, each independently testable:

- **Config vars** — `CLAUDE_CONFIG_DIR`, `CLAUDE_YOLO_FILE` (frozen cross-repo
  contract, see §7), `CLAUDE_REMOTE_ENABLED_FILE`.
- **`claude_launch_flags(tty_state, args…)`** — pure decision logic; populates the
  global array `CLAUDE_LAUNCH_FLAGS`. TTY state is **passed in** (not probed) for
  deterministic testing. Top-level name (no leading `_`) so Claude Code's
  shell-snapshot does not strip it.
- **`claude()` wrapper** — tmux-anchor (unchanged) → computes TTY state → calls
  `claude_launch_flags` → `command claude "${CLAUDE_LAUNCH_FLAGS[@]}" "$@"`.
- **`claude-config()` tool** — `yolo|remote on|off`, `status`.

## 4. Behavior / features

1. `claude-config yolo on|off` toggles `yolo.enabled`.
2. `claude-config remote on|off` toggles `remote.enabled`.
3. `claude-config` / `status` prints both states (default `OFF`).
4. YOLO sentinel present ⇒ inject `--dangerously-skip-permissions` (any mode).
5. Remote sentinel present **and** interactive TTY **and** not print mode ⇒ inject
   `--remote-control "$(basename "$PWD")"` (explicit name).
6. No `_`-prefixed helper functions anywhere in the file.

## 5. Evaluation criteria (per feature)

| # | Rule | Fires | Must-not-fire | Edge | Pass |
| :-- | :-- | :-- | :-- | :-- | :-- |
| 1 | yolo toggle | `yolo on` creates sentinel | `yolo off` leaves it | unknown subarg ⇒ exit 2 | sentinel state matches |
| 2 | remote toggle | `remote on` creates sentinel | `remote off` leaves it | unknown subarg ⇒ exit 2 | sentinel state matches |
| 3 | defaults | — | fresh source has neither sentinel | status shows both `OFF` | both `OFF` |
| 4 | yolo flag | yolo on ⇒ `--dangerously-skip-permissions` | yolo off ⇒ absent | present even when non-TTY | flag present/absent |
| 5a | remote flag | remote on + TTY + no print ⇒ `--remote-control` | remote off ⇒ absent | both yolo+remote ⇒ both flags | flag present |
| 5b | TTY gate | — | remote on + non-TTY ⇒ no `--remote-control` | piped stdin | absent |
| 5c | print gate | — | remote on + `-p`/`--print` ⇒ no `--remote-control` | flag anywhere in args | absent |
| 5d | **prompt-swallow guard** | remote on ⇒ `--remote-control` carries an explicit name | the user's prompt is **never** an element of `CLAUDE_LAUNCH_FLAGS` | prompt with spaces | prompt preserved as `"$@"` |
| 5e | print false-positive | prompt containing token `-p` is **not** print mode | — | `claude "about -p"` | `--remote-control` still injected |
| 6 | no `_` helpers | — | no `^_name()` in file | — | grep-negative passes |

## 6. Verification harness

`ai/claude/aliases_test.sh` (run: `bash ai/claude/aliases_test.sh`). Layers:
source-level greps (no `_` helpers; `CLAUDE_YOLO_FILE=` contract var present;
explicit-name injection present) + runtime behavioral cases that call
`claude_launch_flags` directly and inspect `CLAUDE_LAUNCH_FLAGS` (deterministic,
no binary shell-out). **31 cases, all green.** `bash -n` syntax-clean. The
prompt-swallow regression (rule 5d) is empirically grounded: the real binary
binds the optional `--remote-control [name]` arg greedily, so a bare flag would
consume the prompt — the explicit name prevents it.

## 7. Prerequisites / dependencies

- **CROSS-REPO CONTRACT (frozen):** the shell variable **`CLAUDE_YOLO_FILE`** —
  name and value semantics (absolute path to the YOLO sentinel) — is read by
  playground PR #73's nano-carried `claude-local` (`~/.config/nano/ollama-client.sh`),
  which sources into the same interactive shell, reads `${CLAUDE_YOLO_FILE:-}`,
  and calls `command claude` directly (never the `claude()` wrapper). Therefore:
  (a) `CLAUDE_YOLO_FILE` must not be renamed/inlined or have its path semantics
  changed; (b) the `--remote-control` injection cannot leak into `claude-local`
  (it bypasses the wrapper) — verified; (c) the `claude-toggle`→`claude-config`
  rename and the `yolo.enabled` filename are **not** part of the contract. An
  inline comment in `aliases.sh` and a guard test mark this coupling.

## 8. Out of scope (and why)

- Inverted/default-ON remote-control — rejected: default-OFF/opt-in is the
  agreed posture (symmetric with YOLO; quiet on fresh/shared/remote hosts).
- Committed/synced kill-switch or central policy — the sentinels are per-machine
  local by design (same model as the existing YOLO sentinel).
- Cross-host `claude-local` wiring (tunnel endpoints) — owned by the nano project.

## 9. Rollback

Revert PR #126. The sentinel files under `~/.config/claude/` are inert without
the wrapper; no global state is written (nothing touches `~/.claude/`).

> Aligned with the architecture-team review of PR #126 (sysarch/secarch/principal
> + adversary). The matching plan is `../plans/claude-config.md`. Registered in
> `../index.md`.
