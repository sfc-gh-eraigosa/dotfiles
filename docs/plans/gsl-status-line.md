# Plan: `gsl` — a Go status line for Claude Code (+ Gemini on-demand)

## Context

The user wants a powerline-style status line for their AI CLIs, wired into `./install.sh`,
that mirrors their zsh/p10k prompt and surfaces AI-session info. They prototyped with
`npx ccstatusline@latest` but it can't meet the full requirement set, and they want Gemini
covered too. Research established:

- **ccstatusline is insufficient**: 60+ widgets (git, context %, model, session + weekly
  usage) but **no time/date segment, no MCP segment, Claude-only**, and it re-spawns `npx`
  per refresh. Config lives at `~/.config/ccstatusline/settings.json`; it just writes a
  `statusLine.command` into Claude's settings.
- **Claude Code's `statusLine`** pipes a JSON payload on stdin after every assistant turn
  (300ms debounce — already satisfies "update after every prompt") and natively provides
  `cwd`, `model.display_name`, `context_window.used_percentage`/tokens, and
  `rate_limits.five_hour`/`seven_day` (= the requested "subscription usage left for day/week").
  Git status must be computed via `git`; **MCP servers are NOT in the payload**.
- **Gemini CLI has no scriptable status line** — only built-in footer toggles
  (`display.footer`, `hideCWD`, `hideModelInfo`, `hideContextSummary`) and a `/hooks` system.
  So a live custom line is realistically Claude-only; Gemini gets an on-demand command.
- The repo strongly favors **Go tools in `src/`** (gss, tmux-mgr, wol) built via `build.sh`
  to `~/opt/bin`, surfaced as a `SKILL.md` and linked by `sync-skills.sh`. The Claude settings
  template already points `statusLine.command` at `bash ~/.claude/statusline-command.sh`
  — a file that does not yet exist. **That shim is the missing hook.**

**Decisions (confirmed with the user):**
1. Build a new stdlib-only Go binary named **`gsl`** (Go Status Line) at `src/statusline/`,
   compiled to `~/opt/bin/gsl`.
2. Gemini: a **pretty on-demand `/gsl-status`** slash command (+ skill `gsl-status`) that runs
   the same binary in plain-text mode; document Gemini's built-in footer toggles.
3. MCP segment: **best-effort cached** — instant "configured" count + an "active" count from
   `claude mcp list` refreshed at most once/60s.

**Outcome:** one fast (~ms startup, zero-dep) binary renders a 3-part powerline status line
— `dir+git` · `AI(context/model/MCP/rate-limits)` · `date/time` — live in Claude, on demand
in Gemini, fully toggleable via a config file and a configuration skill.

---

## Approach

### New tool: `src/statusline/` (module `github.com/wenlock/dotfiles/statusline`)

Mirror the `src/gss` conventions: `build.sh` (goenv-aware `go`, version stamped via
`-X .../internal/version.*` from a `VERSION` file, output to `~/opt/bin/gsl`, then run
`scripts/check-deps.sh`), an `os/exec` **seam** confined to specific packages and enforced by
`check-deps.sh`, a `skill/SKILL.md`, Apache-2.0 `LICENSE`. **Stdlib only** (JSON, time, os/exec)
— no cobra, no external deps — so `go.mod` stays require-free and the license gate is trivially
green (per `src/CLAUDE.md` license + TDD rules).

```
src/statusline/
  main.go  go.mod  VERSION  build.sh  README.md  LICENSE
  cmd/            root.go (switch os.Args[1]) render.go status.go config.go version.go (+ _test)
  internal/
    version/      version.go        (copy gss pattern)
    payload/      payload.go        (defensive parse of Claude stdin JSON; all fields pointers)
    config/       config.go         (Config, Default(), Load/Save, toggle helpers)
    git/          exec.go (Runner seam) status.go (GitInfo)  fake/runner.go
    mcp/          detect.go cache.go (Configured instant + Active cached/60s)  fake/runner.go
    render/       segment.go glyphs.go render.go + seg_dirgit.go seg_ai.go seg_time.go
  skill/SKILL.md  scripts/check-deps.sh  docs/design.md
```

**Subcommands** (manual `flag.FlagSet` dispatch):
- `gsl render` (default) — read JSON payload on stdin, print powerline line(s). Claude's path.
- `gsl status` — no stdin; pretty on-demand render (Gemini `/gsl-status` + CLI). Payload-only
  segments self-omit.
- `gsl config get|set|enable|disable|toggle [<segment>]` — drives the configuration skill.
- `gsl version [--json]`.

**Segments** (ordered, individually toggleable):
1. **dirgit** — `~`-abbreviated cwd basename + branch + p10k-style counts: staged `+N`,
   unstaged `!N`, untracked `?N`, stashes, ahead `⇡N`, behind `⇣N`. One
   `git -C <dir> status --porcelain=v2 --branch` + `git stash list`, via the `git.Runner` seam
   (~1s timeout). Mirrors `opt/profiles/.p10k.zsh` `my_git_formatter` glyph semantics.
2. **ai** — from the Claude payload: `model.display_name`, `context_window.used_percentage`
   + tokens vs `context_window_size`, MCP `active/configured`, and rate-limit usage for the
   5-hour and 7-day windows (`rate_limits.five_hour`/`seven_day` + `resets_at`). Returns
   `("", false)` (self-omits) when there is no payload (Gemini/CLI mode) or a field is null.
3. **time** — current time in a configurable tz (default `America/Los_Angeles`/PST) with
   Nerd-Font calendar + clock glyphs: day-of-week, month/day, `HH:MM`, tz abbreviation.

**MCP detection** (`internal/mcp`): `ConfiguredCount(cwd)` parses `~/.claude.json`
(top-level `mcpServers` + `projects.<cwd>.mcpServers`) and `./.mcp.json`, honoring
`CLAUDE_CONFIG_DIR` — instant, no subprocess. `ActiveCount(...)` reads a cache at
`${XDG_CACHE_HOME:-~/.cache}/gsl/mcp.json`; if older than 60s it runs `claude mcp list`
through the `mcp.Runner` seam with a 2s timeout, parses connected vs failed markers, and
rewrites the cache; on timeout/error it falls back to the last cache (never blocks the line).

**Config + on/off** (`internal/config`): JSON at `${XDG_CONFIG_HOME:-~/.config}/gsl/config.json`,
with `enabled` (master), an ordered `segments[]` (each `{type, enabled, options}`), `timezone`,
time/date formats, `glyphs` (`nerdfont`|`ascii`), powerline separators, and `theme`. **Missing
file ⇒ sane defaults** (the binary works with zero config). Two on/off layers, both honored:
the harness `statusLine.command` key (hard off = remove it) and `config.enabled` (soft off:
`gsl render` prints empty stdout) plus per-segment toggles — all reachable via `gsl config …`.

### Wiring into install (exact edits)

- **NEW `ai/claude/statusline-command.sh`** (committed, `chmod +x`): the shim the settings
  template already references. Reads stdin once; if `~/opt/bin/gsl` exists, `exec`s
  `~/opt/bin/gsl render` forwarding the payload; otherwise prints a minimal bash fallback
  (`basename $PWD` + `git branch` + `date`) so the line never breaks pre-build.
- **`opt/scripts/system/install_claude_skills.sh`**: after the commands-linking block, add a
  guarded `ln -sf "$BASE_DIR/ai/claude/statusline-command.sh" "$CLAUDE_HOME/statusline-command.sh"`
  (same backup-if-not-symlink guard used for settings.json/aliases.sh) + `chmod +x`.
- **`opt/scripts/system/sync-skills.sh`**: line 97 build loop → add `"statusline"`; the name
  `case` (lines 114-116) → add `statusline) dest_name="gsl-status" ;;` so the skill links to
  `~/.claude/skills/gsl-status` and `~/.agents/skills/gsl-status`.
- **NEW `ai/gemini/commands/gsl-status.toml`** (mirror `ai/gemini/commands/gss.toml`):
  `description` + a pretty `prompt` blurb with `!{gsl status}` substitution and `{{args}}`.
  Auto-linked — `install_gemini_skills.sh` already globs `ai/gemini/commands/*.toml`
  (line 56), no script edit needed.
- **No edits required** to: `.gitignore` (`!src/**` already tracks `src/statusline/**`; the
  binary lives outside the repo), `install.sh` (already runs sync-skills + both install
  scripts), `Makefile` (`make bin` auto-discovers `src/*/build.sh`), and
  `ai/claude/settings.json.template` (the existing `Bash($HOME/opt/bin/*:*)` allow rule already
  covers `gsl`; the `statusLine.command` line is already correct).

### Files to create / modify (summary)

| Action | Path |
| --- | --- |
| NEW (tool tree) | `src/statusline/**` — Go source, `build.sh`, `VERSION`, `skill/SKILL.md`, `scripts/check-deps.sh`, tests |
| NEW | `ai/claude/statusline-command.sh` (shim) |
| NEW | `ai/gemini/commands/gsl-status.toml` |
| EDIT | `opt/scripts/system/install_claude_skills.sh` (symlink shim) |
| EDIT | `opt/scripts/system/sync-skills.sh` (build loop + `gsl-status` case) |

### Patterns to reuse (don't reinvent)

- `src/gss/build.sh` — copy the goenv-aware, version-stamping build script verbatim, swapping
  names/paths to `gsl`.
- `src/gss/internal/git` + its `fake/` runner — the `os/exec` seam + fakeable Runner pattern
  for both `git` and `mcp`; `src/gss/scripts/check-deps.sh` — copy and adjust the seam
  allowlist to `internal/git` + `internal/mcp`.
- `src/gss/internal/version` — version var + `Get()` fallback pattern.
- `opt/profiles/.p10k.zsh` `my_git_formatter` — authoritative glyph semantics
  (`⇡`/`⇣` ahead/behind, `+`/`!`/`?` staged/unstaged/untracked) to mirror.
- `ai/gemini/commands/gss.toml` — exact TOML command format for `gsl-status.toml`.
- `opt/scripts/system/install_claude_skills.sh` settings.json/aliases.sh symlink-with-backup
  blocks — copy that shape for the shim symlink.

---

## Verification

1. **Build**: `bash src/statusline/build.sh` → `gsl built and installed to ~/opt/bin/gsl`,
   check-deps passes. (`make bin` also works.)
2. **Tests/coverage** (TDD, >60% per pkg per `src/CLAUDE.md`):
   `cd src/statusline && go test ./... -cover`. Seams faked via `git/fake` + `mcp/fake`;
   payload fixtures under `internal/payload/testdata/`. Cases per package:
   - payload: full fixture; malformed JSON → error; **empty stdin → empty struct, no error**;
     `used_percentage:null` → nil; `seven_day` absent but `five_hour` present.
   - git: scripted porcelain=v2 (staged/unstaged/untracked, ahead 2/behind 1, stashes); clean
     repo → zeros; not-a-repo error; detached HEAD.
   - mcp: configured count from temp `~/.claude.json` + `.mcp.json` (CLAUDE_CONFIG_DIR honored);
     active from fake `claude mcp list` (mixed ✓/✗); **cache hit <60s ⇒ Runner not called**;
     stale cache + timeout ⇒ last value.
   - render/config: segment ordering + disable; master `enabled=false` ⇒ empty output;
     tz render + bad-tz→UTC fallback; nerdfont vs ascii glyphs; Default()/Load-missing/round-trip.
3. **Render (Claude path)**: pipe a sample payload —
   `printf '%s' '{"cwd":"'"$PWD"'","model":{"display_name":"Sonnet"},"context_window":{"used_percentage":42,"total_input_tokens":84000,"context_window_size":200000},"rate_limits":{"five_hour":{"used_percentage":12.5,"resets_at":"2026-05-24T20:00:00Z"}}}' | ~/opt/bin/gsl render`
   → one powerline line: dir+git, AI(model/ctx%/tokens/MCP/5h+7d), time.
4. **On-demand (Gemini/CLI)**: `~/opt/bin/gsl status` (no stdin) → dir+git+time, AI omitted.
5. **Toggle**: `gsl config disable time` → time gone; `gsl config enable time` restores;
   `gsl config disable` (master) → empty output.
6. **Wiring**: `bash opt/scripts/system/sync-skills.sh --build` builds gsl + links
   `~/.claude/skills/gsl-status` and `~/.agents/skills/gsl-status`;
   `bash opt/scripts/system/install_claude_skills.sh` → `~/.claude/statusline-command.sh`
   symlink exists & executable; `bash opt/scripts/system/install_gemini_skills.sh` →
   `~/.gemini/commands/gsl-status.toml` linked.
7. **Shim fallback**: `mv ~/opt/bin/gsl ~/opt/bin/gsl.bak`, pipe a payload into
   `bash ~/.claude/statusline-command.sh` → minimal bash line; restore binary.
8. **Live Claude pickup**: start a Claude Code session; after an assistant turn the status
   line renders (harness pipes the live payload → shim → `gsl render`).
