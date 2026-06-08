# claude-config — implementation plan

- **Slug:** claude-config
- **Date:** 2026-06-07
- **Status:** In-progress
- **Relates to:** spec `../specs/claude-config.md` · PR #126 · consumer: playground PR #73

## 1. Summary & verdict

Rename `claude-toggle` → `claude-config` and add an opt-in `--remote-control`
launch flag alongside the existing opt-in YOLO flag, both via sentinel files
under `$XDG_CONFIG_HOME/claude`. This plan is **retroactive** (the code shipped
in PR #126) and folds in the architecture-team review verdict.

**Review verdict:** *needs-changes → addressed.* Must-fixes resolved:
- **Prompt-swallow blocker** (`--remote-control [name]` greedily eats the prompt):
  fixed by injecting an explicit name `--remote-control "$(basename "$PWD")"` via
  a bash/zsh **array** (`command claude "${CLAUDE_LAUNCH_FLAGS[@]}" "$@"`).
- **Security default-posture** (remote default-ON across synced machines): fixed
  by flipping to **opt-in/default-OFF** (`remote.enabled`, symmetric with YOLO) —
  supersedes the original inverted `remote.disabled` sentinel.
- **No real test coverage** (only a blind `grep 'print'`): fixed by extracting
  `claude_launch_flags()` and adding 13 behavioral cases incl. the prompt-swallow
  regression guard.
- **MBO non-compliance**: this spec + plan + `index.md` row + PR-body links.
- **Contract drift risk**: `CLAUDE_YOLO_FILE` marked as a frozen cross-repo
  contract (comment + guard test).

**Follow-up scope (binary consolidation):** a machine could accumulate two
`claude` binaries — the orchestrated npm/brew one and the official native
installer's `~/.local` copy — differing in version with a PATH-order foot-gun.
`claude_install.sh` is made the single source of truth: after installing the
canonical copy it removes the native install (guarded/idempotent), and
`claude-config doctor` reports the resolved binary and flags duplicates.

## 2. File inventory

| Path | Purpose | Implements |
| :-- | :-- | :-- |
| `ai/claude/aliases.sh` | `claude` wrapper, `claude_launch_flags`, `claude-config` tool (+`doctor`), config vars, contract comment, security note | spec §3, §4, §7 |
| `ai/claude/aliases_test.sh` | 35-case driver: source-level + behavioral + doctor | spec §5, §6 |
| `opt/scripts/system/claude_install.sh` | sourceable refactor; `resolve_canonical_claude` + `cleanup_conflicting_installs` (single canonical binary) | spec §4.7, §7 |
| `opt/scripts/system/claude_install_test.sh` | 10-case driver for the cleanup decision (sandboxed `$HOME`) | spec §5.7 |
| `docs/machine-local-overrides.md` | refreshed override example (drop removed `_claude_yolo_enabled`); opt-in note | spec §2, §8 |
| `opt/profiles/.zshrc` | comment refresh (`claude-config`) | — |
| `opt/profiles/.bashrc` | comment refresh (`claude-config`) | — |
| `docs/mbo/specs/claude-config.md` | the spec | — |
| `docs/mbo/plans/claude-config.md` | this plan | — |
| `docs/mbo/index.md` | Active-table registration | — |

`install.sh` already calls `claude_install.sh` (no edit). Both `*_test.sh` are
auto-discovered by `make shell-test` (no wiring). `install_claude_skills.sh`
already symlinks `aliases.sh`.

## 3. Interface contracts

```sh
# Populates global array CLAUDE_LAUNCH_FLAGS. TTY passed in for testability.
claude_launch_flags <tty_state> <args...>     # tty_state = "tty" | anything else
#   yolo.enabled            -> += --dangerously-skip-permissions   (any mode)
#   remote.enabled & tty &  -> += --remote-control "$(basename "$PWD")"
#     not (-p|--print in args)
claude()                                       # tmux-anchor; command claude "${CLAUDE_LAUNCH_FLAGS[@]}" "$@"
claude-config [status] | yolo on|off | remote on|off
```

Frozen: `CLAUDE_YOLO_FILE` = abs path to YOLO sentinel (read by playground
`claude-local`; see spec §7).

## 4. TDD build order

1. **Source-level guards** — write greps (no `_` helpers; `CLAUDE_YOLO_FILE=`
   present; explicit-name injection present). *done-when:* greps pass against the
   rewritten file.
2. **`claude_launch_flags` extraction** — refactor decision logic into the pure,
   array-populating function. *done-when:* defaults inject nothing; yolo/remote
   gates behave per spec §5.
3. **Prompt-swallow regression** — assert the user prompt is never an element of
   `CLAUDE_LAUNCH_FLAGS` and `--remote-control` is followed by a name. *done-when:*
   case green.
4. **TTY / print / false-positive gates** — non-TTY, `-p`, `--print`, and
   `"about -p"` cases. *done-when:* all green.
5. **opt-in flip** — `remote.enabled` create/remove + status default OFF.
   *done-when:* config cases green.

## 5. Verification mapping

| Spec rule | Test case (aliases_test.sh) |
| :-- | :-- |
| 1 yolo toggle | `claude-config yolo on/off creates/removes yolo sentinel` |
| 2 remote toggle | `claude-config remote on/off creates/removes enabled sentinel` |
| 3 defaults | `remote/yolo defaults OFF`, `status reports … OFF by default` |
| 4 yolo flag | `yolo on/off: …dangerously-skip-permissions` |
| 5a remote flag | `remote on: injects --remote-control with explicit name` |
| 5b TTY gate | `remote on but non-tty: no --remote-control` |
| 5c print gate | `remote on but -p/--print print mode: no --remote-control` |
| 5d prompt-swallow | `user prompt is NOT captured into the flags`, `yolo+remote on: … prompt preserved` |
| 5e print false-positive | `prompt containing '-p' is not mistaken for print mode` |
| 6 no `_` helpers | `no underscore-prefixed helper functions` |
| 7a native cleanup | `removes native install when canonical is confirmed`, `keeps native install when no canonical is confirmed`, `skips when canonical also resolves under ~/.local` |
| 7b cleanup safety | `keeps a convenience symlink that points at the canonical binary`, `no-op when there is no native install`, `cleanup never touches ~/.claude (documented)` |
| 8a doctor report | `doctor prints the resolved binary line` |
| 8b doctor duplicates | `doctor: single binary => no warning, exit 0`, `doctor: multiple binaries => WARNING + nonzero`, `doctor: dedups repeated PATH entries` |
| §7 contract | `CLAUDE_YOLO_FILE cross-repo contract var preserved`, `remote-control passes an explicit session name` |

## 6. Integration & rollout

`aliases.sh` is symlinked into `~/.config/claude/` by `install_claude_skills.sh`
and sourced by the profiles — no new wiring. Manual acceptance: on a machine,
`claude-config remote on`, then in a TTY run `claude "hello"` and confirm the
session starts with remote-control and the prompt is delivered; `claude-config
remote off` to revert. Verify a sibling `claude-local` shell still applies YOLO
from `CLAUDE_YOLO_FILE` and never receives `--remote-control`.

### 6.1 Build leaves / DAG

Not broken out — single PR (#126), no parallel leaves.

> Retroactive plan capturing the shipped PR #126 + the architecture-team review
> must-fixes. Update `../index.md` state as it moves.
