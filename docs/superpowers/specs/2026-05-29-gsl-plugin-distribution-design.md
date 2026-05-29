# gsl as a Claude Code plugin — distribution design

**Status:** Draft for review
**Date:** 2026-05-29
**Issue:** _(filled in once tracking issue is opened)_

## Problem

Today, `gsl` (the Go-based powerline status line) ships only as part of a full `~/git/dotfiles` install:

- `install.sh` runs `src/gsl/build.sh`, which compiles `gsl` to `~/opt/bin/gsl`.
- `install.sh` symlinks `ai/claude/statusline-command.sh` → `~/.claude/statusline-command.sh`.
- The user manually points `statusLine.command` in `~/.claude/settings.json` at that shim (or `install.sh` does, depending on path).
- Gemini CLI picks up `/gsl-status` from `ai/gemini/commands/gsl-status.toml`.

This means anyone who wants `gsl` in their Claude Code session has to clone the entire dotfiles repo and run `install.sh` — which globally rewrites their `$HOME` (symlinks for `.zshrc`, `.bashrc`, `.tmux.conf`, etc.). That's a hard sell for coworkers or other sessions that just want the status line.

We want a path where a user can opt into `gsl` alone, without adopting the whole dotfiles environment.

## Goals

1. A user can install `gsl` into Claude Code in two commands, without cloning dotfiles.
2. The dotfiles repo itself consumes its own plugin (dogfooding via `ai/plugins.yaml`).
3. `src/gsl/` stays the single source of truth for the Go code — no fork, no duplication.
4. Existing dotfiles users see no regression: status line keeps working across the migration.

## Non-Goals

- **Gemini extension parity for external users.** Dotfiles users keep their `/gsl-status` Gemini command via `ai/gemini/commands/gsl-status.toml`. Publishing a Gemini extension is a separate ecosystem and a separate piece of work.
- **Prebuilt binary releases (for now).** v1 builds from source on first SessionStart. If `go install` friction shows up in practice, we add a GitHub Release workflow later and switch `ensure-binary.sh` to download-first-build-fallback. The plugin contract doesn't change.
- **Windows support.** Not currently supported by `build.sh`; out of scope here.
- **Moving `src/gsl/`.** Stays put. The plugin is a thin wrapper over the existing Go module.

## Decisions (from brainstorm)

| Decision | Choice | Rejected alternatives |
|---|---|---|
| Plugin source location | Stay in dotfiles, plugin layout in-tree | New repo / read-only mirror — both add release-cadence overhead |
| Binary delivery | Build on install (requires Go on host) | GitHub Releases download / commit prebuilt binaries / hybrid — all heavier for v1 |
| `statusLine` wiring | `/gsl-install` slash command (opt-in patch with confirmation) | Auto-patch settings.json silently / docs-only nudge |

## Distribution model

The dotfiles repo becomes a Claude Code marketplace. We add `.claude-plugin/marketplace.json` at the repo root declaring `dotfiles` as a marketplace with one plugin (`gsl`) whose source is `./plugins/gsl`.

External users install with:

```sh
claude plugin marketplace add wenlock/dotfiles
claude plugin install gsl@wenlock-dotfiles
/gsl-install         # one-time opt-in: patches ~/.claude/settings.json
```

No `~/git/dotfiles` clone required.

**Dogfooding.** `ai/plugins.yaml` gains a row for `gsl: { plugin: gsl@wenlock-dotfiles }`. `sync-plugins` installs gsl the same way external users do, so we eat our own dogfood every sync. Today's `install.sh` block that builds gsl + symlinks the shim is **deleted** — the plugin owns that path.

**Migration.** Existing dotfiles users have `~/.claude/settings.json` pointing at `~/.claude/statusline-command.sh`. The next `sync-plugins` run installs the plugin (which builds the binary into the plugin tree). Until the user runs `/gsl-install`, the old shim path still works because the binary is still at `~/opt/bin/gsl`. After `/gsl-install`, settings.json points at `${CLAUDE_PLUGIN_ROOT}/scripts/statusline-command.sh` and the legacy shim symlink becomes unused (and can be reaped in a follow-up).

## Repo layout

```
~/git/dotfiles/
  .claude-plugin/
    marketplace.json            # declares dotfiles as a marketplace, lists "gsl"
  plugins/
    gsl/
      .claude-plugin/plugin.json
      hooks/hooks.json          # SessionStart -> scripts/ensure-binary.sh
      scripts/
        ensure-binary.sh        # builds gsl if ${CLAUDE_PLUGIN_ROOT}/bin/gsl missing
        statusline-command.sh   # the shim (execs ${CLAUDE_PLUGIN_ROOT}/bin/gsl render)
        gsl-install.sh          # patches user settings.json (idempotent, with backup)
      commands/
        gsl-install.md          # /gsl-install slash command
        gsl-status.md           # /gsl-status (moved from ai/claude/commands/)
      skills/gsl-status/
        SKILL.md                # symlink -> ../../../src/gsl/skill/SKILL.md
      source/                   # symlink -> ../../src/gsl
  src/gsl/                      # unchanged Go source — single source of truth
```

**Source-sharing trick.** `plugins/gsl/source` is a git-tracked symlink pointing at `../../src/gsl`. Git preserves symlinks on clone, and when the marketplace clones the dotfiles repo into its cache, `plugins/gsl/source` resolves correctly because the whole repo is cloned together. Same trick for `plugins/gsl/skills/gsl-status/SKILL.md → ../../../src/gsl/skill/SKILL.md`.

**Fallback if symlinks misbehave.** Physically move `src/gsl/` → `plugins/gsl/source/` and update `build.sh` callers. More churn but more explicit. v1 tries symlinks first — git handles them fine on Linux/macOS, which matches our platforms. If a downstream user reports symlink breakage (Windows clone, zip download), we revisit.

## SessionStart binary build

`scripts/ensure-binary.sh` (called by `hooks/hooks.json` SessionStart):

```sh
#!/usr/bin/env bash
set -euo pipefail
BIN="${CLAUDE_PLUGIN_ROOT}/bin/gsl"
SRC="${CLAUDE_PLUGIN_ROOT}/source"
[[ -x "$BIN" ]] && exit 0                      # already built
command -v go >/dev/null 2>&1 || {              # surface clear error once
    echo "[gsl] 'go' not on PATH — install Go to build the status line binary." >&2
    exit 0                                       # never block session start
}
mkdir -p "${CLAUDE_PLUGIN_ROOT}/bin"
( cd "$SRC" && BIN_DIR="${CLAUDE_PLUGIN_ROOT}/bin" bash build.sh ) >/dev/null
```

**Decision:** teach `build.sh` to honor a `BIN_DIR` env var (defaulting to `~/opt/bin` as today). The plugin sets `BIN_DIR=${CLAUDE_PLUGIN_ROOT}/bin` so the binary lives where the plugin expects without an extra symlink step. Five-line change to `build.sh`; existing callers unaffected.

The hook never exits non-zero, so a missing Go toolchain won't break session start — the shim's existing fallback (`cat > /dev/null; printf basename+branch+time`) takes over and the line still renders, just plainer.

## `/gsl-install` slash command

`scripts/gsl-install.sh` does:

1. Reads `~/.claude/settings.json` (creates `{}` if missing).
2. Writes a backup to `~/.claude/settings.json.gsl-backup.<UTC-timestamp>`.
3. If `statusLine.command` is unset or already points at a gsl shim path, sets it to `bash "${CLAUDE_PLUGIN_ROOT}/scripts/statusline-command.sh"`.
4. If `statusLine.command` is set to something else (custom user statusLine), refuses to overwrite, prints the exact line to add manually, exits 0.
5. Verifies `${CLAUDE_PLUGIN_ROOT}/bin/gsl render` works against a synthetic JSON payload before committing the settings write — fails closed if the binary is missing.

Uses `jq` for the JSON edit. The SessionStart hook prints a one-line nudge `gsl: run /gsl-install to wire up the status line` if statusLine isn't pointing at the plugin shim — fires only once per cache version (state file under `${CLAUDE_PLUGIN_ROOT}/.nudged`) to avoid noise.

## Testing

- **Unit tests for `gsl-install.sh`:** drive it with synthetic `~/.claude/settings.json` fixtures (missing file, empty object, conflicting statusLine, already-correct statusLine, malformed JSON). Assert backup created, exit codes, idempotency. Mirrors the existing `safety_guard_test.sh` pattern.
- **Unit tests for `ensure-binary.sh`:** mock `go` absent vs. present, assert idempotent re-runs, assert non-zero exit never happens.
- **Integration smoke:** `scripts/e2e-gsl-plugin.sh` (new) — `claude plugin install` into a throwaway `$HOME` (via `HOME=/tmp/gsl-e2e ...`), verify SessionStart builds binary, verify `/gsl-install` patches settings, verify `gsl render` against a fixture returns expected output.
- **Existing `src/gsl/` test suite is unchanged** — the plugin is a packaging concern, not a code change.

## Out of scope (explicit reminders)

- **Gemini parity** — preserved for dotfiles users via the existing `ai/gemini/commands/gsl-status.toml`. External plugin-only users on Gemini are not addressed by this work.
- **Prebuilt binary releases** — deferred. See Non-Goals.
- **Removing `src/gsl/`** — stays put.
- **Reaping the legacy `~/.claude/statusline-command.sh` symlink** — left in place for one release cycle to keep migration safe; reap in a follow-up after we've confirmed `/gsl-install` works in the wild.

## Risks and open questions

- **Symlink portability.** If a downstream user clones dotfiles via a tool that flattens symlinks (some Windows clients, some zip exports), `plugins/gsl/source` breaks. Mitigation: the `ensure-binary.sh` hook can detect a broken symlink and print a clear error. Fallback plan documented above (move source physically).
- **Marketplace clone size.** Adding dotfiles as a marketplace means external `gsl` users clone the entire dotfiles tree (~tens of MB). This is awkward but not blocking — the same cache footprint as any other big-monorepo marketplace.
- **`build.sh` `BIN_DIR` retrofit.** Need to confirm no other caller assumes `~/opt/bin/gsl` as a hardcoded build target. Quick grep before the retrofit.
- **CLAUDE_PLUGIN_ROOT in scripts.** Verify that `CLAUDE_PLUGIN_ROOT` is set in the SessionStart hook context and in the slash-command shell context — both expected per the Claude Code plugin spec, but worth a smoke test on first run.

## Implementation order (for the follow-up plan)

1. Add `BIN_DIR` env support to `src/gsl/build.sh` (backwards-compatible default).
2. Create `.claude-plugin/marketplace.json` and `plugins/gsl/` skeleton with symlinks.
3. Write `ensure-binary.sh`, `statusline-command.sh`, `gsl-install.sh` + their tests.
4. Migrate `/gsl-status` command from `ai/claude/commands/` into `plugins/gsl/commands/` (delete the original).
5. Add e2e smoke test under `scripts/e2e-gsl-plugin.sh`.
6. Add `gsl` row to `ai/plugins.yaml`, delete the gsl block from `install.sh`.
7. Run `sync-plugins`, verify the plugin installs cleanly into the local environment.
8. Update `docs/ai-plugins.md` to mention gsl and the marketplace pattern.

A detailed implementation plan lives in a follow-up `writing-plans` artifact, not this spec.
