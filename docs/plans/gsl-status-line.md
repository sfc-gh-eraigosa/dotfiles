# Plan: `gsl` — a Go status line for Claude Code (+ Gemini on-demand)

## Revisions from multi-agent review (PR #21)

A four-persona review (Architect, Researcher+Lead-Dev, Planner, QA) ran on this design.
**Unanimous verdict: SOUND-WITH-CHANGES** — no architectural blockers. Four required
changes (each a reviewer comment) are folded into the design below:

1. **Adopt cobra + bubbletea** (was: stdlib-only). cobra for the CLI (`cmd.Execute()`,
   logic in `internal/`, matching gss); bubbletea for the **preview TUI only — never in the
   per-turn render hot path**. Licenses verified: cobra **Apache-2.0**, bubbletea **MIT** →
   both allowed by `src/CLAUDE.md`; the license gate stays green.
2. **`src/gsl`** (was: `src/statusline`) + module path `github.com/wenlock/dotfiles/gsl`,
   cascaded through `build.sh` LDFLAGS and the `sync-skills.sh` case map.
3. **First-class `gsl preview`** — `--once` (single snapshot, for CI/golden tests) **and** an
   interactive bubbletea TUI (live-toggle segments/glyphs/theme, 1s clock tick), rendered
   against representative sample-payload fixtures.
4. **Concrete timeout/degrade budget** — `context.WithTimeout` + `exec.CommandContext`:
   git ~800ms, `claude mcp list` ~500ms, total render soft ~150ms / hard ~1000ms. Segments
   render **concurrently**, so the hard cap bounds the slowest single call (git ~800ms), not
   the sum — every per-call deadline sits below the ceiling and can actually fire. On timeout
   **degrade** (omit segment / use last MCP cache), **never block**.

## Follow-up requirement: `repo` segment (PR# + worktree awareness)

After the review, the user added a fifth requirement: a **fourth segment** — `repo` — between
`dirgit` and `ai`, surfacing repo/worktree context the existing segments don't:

- a **🏠 root / 🌳 worktree indicator** (blue / magenta; ASCII `[root]` / `[wt]`) showing whether
  the current checkout is the canonical repo root or a linked git worktree;
- in a worktree, the **gss feature name** it belongs to (e.g. `gsl`) — info `dirgit` doesn't show
  (it shows the worktree's own dir basename + branch); non-gss worktrees fall back to the dir name;
- the active **PR number** (`PR#21`, tinted by PR state), resolved **instantly from the gss
  registry** with `gh pr view` as a cached network fallback;
- a **worktree count** (`⑂N`, shown when N ≥ 2; ASCII `wtN`) — "how many worktrees do I have going",
  in the spirit of the MCP `active/configured` count.

All of it is one instant `registry.json` read plus two fast git calls (`rev-parse`,
`git worktree list`); only the `gh` fallback touches the network, and it is cached/timed out like
`claude mcp list`. The segment self-omits parts that don't apply, so a plain non-git dir shows
nothing extra. Details fold into the sections below.

---

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
  to `~/opt/bin`, surfaced as a `SKILL.md` and linked by `sync-skills.sh`. **gss already
  depends on cobra** — so cobra is the established pattern, not a new dependency risk. The
  Claude settings template already points `statusLine.command` at
  `bash ~/.claude/statusline-command.sh` — a file that does not yet exist. **That shim is the
  missing hook.**

**Decisions (confirmed with the user; revised per review):**
1. Build a new **cobra-based** Go binary named **`gsl`** (Go Status Line) at **`src/gsl/`**,
   compiled to `~/opt/bin/gsl`. **bubbletea** powers the preview TUI only.
2. Gemini: a **pretty on-demand `/gsl-status`** slash command (+ skill `gsl-status`) that runs
   the same binary in plain-text mode; document Gemini's built-in footer toggles.
3. MCP segment: **best-effort cached** — instant "configured" count + an "active" count from
   `claude mcp list` refreshed at most once/60s.
4. `repo` segment: **root-vs-worktree indicator + gss feature name + PR# + worktree count**,
   sourced from the gss `registry.json` (instant) and local git, with a cached `gh pr view`
   fallback for the PR number on non-gss branches.
5. **Configurable styles**: a named **style** bundles glyph set (per-segment icons), separator
   (`powerline`/`thin`/`space`), background-fill on/off, glyph mode (`nerdfont`/`emoji`/`ascii`),
   and theme colors. Two built-ins ship — **`powerline`** (default; NF tinted ` `/` ` repo
   glyph) and **`emoji`** (color-emoji markers, airy spacing). Users add or override styles in
   `config.json`; `gsl config style <name>` switches; `gsl preview` cycles them.

**Outcome:** one fast (~ms startup) binary renders a 4-part powerline status line —
`dir+git` · `repo(root/worktree · PR# · worktree-count)` · `AI(context/model/MCP/rate-limits)` ·
`date/time` — live in Claude, on demand in Gemini, with a `gsl preview` to see and tune it, fully
toggleable via a config file and a configuration skill.

---

## What it looks like (preview)

The line is rendered by the active **style** (see "Styles" below). Two built-ins ship; both show
the `repo` segment (2nd) differently at the repo root vs inside a linked worktree.

**`powerline` (default)** — NF tinted repo glyph (` ` root = blue, ` ` worktree = magenta) on a
neutral powerline; the glyph is foreground-tinted (which emoji can't be), so the root/worktree
colour reads cleanly:

```
root checkout:
  ~/dotfiles   main +2 !1 ?3 ⇡2 ⇣1     PR#42 ⑂4    Opus 4.7  42% 84k/200k  MCP 2/5  5h 12% · 7d 45%   Sat 05/24 15:30 PDT
└──────── dirgit ────────┘ └─── repo ───┘ └──────────────────────── ai ────────────────────────┘ └──────── time ───────┘

inside a worktree:
  …/gsl/…/plan   feature/gsl… +1      gsl PR#21 ⑂4    Opus 4.7  42% 84k/200k  MCP 2/5  5h 12% · 7d 45%   Sat 05/24 15:30 PDT
└──────── dirgit ─────────┘ └────── repo ──────┘ └──────────────────────── ai ────────────────────────┘ └──────── time ───────┘
```

**`emoji`** — color-emoji markers, thin separators, no background fills (airier; emoji carry the
root/worktree distinction by shape — 🏠 vs 🌳 — since emoji can't be ANSI-tinted):

```
📁 ~/dotfiles  🌿 main +2 !1   🏠 PR#42 ⑂4   🤖 Opus 4.7  🧠 42%  🔌 2/5   ⏰ Sat 15:30
📁 …/plan      🌿 feat/gsl +1   🌳 gsl PR#21 ⑂4   🤖 Opus 4.7  🧠 42%  🔌 2/5   ⏰ Sat 15:30
```

In both, `gsl` = the gss feature the worktree belongs to; `PR#21` is tinted by PR state (draft dim /
open normal); `⑂4` = 4 worktrees exist. Parts self-omit: no PR ⇒ no `PR#`, a lone main checkout ⇒
no `⑂N`, a non-git dir ⇒ the whole `repo` segment disappears.

**ASCII fallback** (any style with `glyphs: ascii`, or when no Nerd Font is detected):

```
[~/dotfiles] (main +2 !1 ?3 ^2 v1) | [root] PR#42 wt4 | Opus 4.7 42% 84k/200k MCP 2/5 5h 12% 7d 45% | Sat 05/24 15:30 PDT
[~/…/plan]   (feature/gsl… +1)     | [wt] gsl PR#21 wt4 | Opus 4.7 42% 84k/200k MCP 2/5 5h 12% 7d 45% | Sat 05/24 15:30 PDT
```

`gsl preview` renders these against fixture payloads (clean repo, dirty repo, high-context,
rate-limited, **root vs worktree**); the interactive TUI lets you toggle each segment, **cycle
styles**, switch glyphs/theme, and watch the clock tick before saving config.

---

## Approach

### New tool: `src/gsl/` (module `github.com/wenlock/dotfiles/gsl`)

Mirror the `src/gss` conventions: `build.sh` (goenv-aware `go`, version stamped via
`-X github.com/wenlock/dotfiles/gsl/internal/version.*` from a `VERSION` file, output to
`~/opt/bin/gsl`, then run `scripts/check-deps.sh`), an `os/exec` **seam** confined to
`internal/git`, `internal/mcp`, and `internal/gh` and enforced by `check-deps.sh`, a
`skill/SKILL.md`, Apache-2.0 `LICENSE`. **cobra** for dispatch (`cmd.Execute()` in `main.go`);
**bubbletea** imported only by the preview package. `check-deps.sh` allows cobra/bubbletea in
`cmd/` + `internal/preview`, and keeps `os/exec` out of everything except the three seams.

```
src/gsl/
  main.go  go.mod  go.sum  VERSION  build.sh  README.md  LICENSE
  cmd/            root.go (cobra root + Execute()) render.go status.go config.go
                  preview.go version.go (+ _test)
  internal/
    version/      version.go        (copy gss pattern)
    payload/      payload.go        (defensive parse of Claude stdin JSON; all fields pointers)
    config/       config.go         (Config, Default(), Load/Save, toggle helpers)
    git/          exec.go (Runner seam, CommandContext) status.go (GitInfo)
                  worktree.go (IsLinked, Count, Toplevel)  fake/runner.go
    mcp/          detect.go cache.go (Configured instant + Active cached/60s)  fake/runner.go
    repo/         detect.go (root/worktree via git) registry.go (gss registry.json reader)
                  pr.go (PR# from registry → gh fallback)
    gh/           exec.go (Runner seam, CommandContext) pr.go (gh pr view, cached/60s)  fake/runner.go
    style/        style.go (Style struct) builtins.go (powerline default + emoji) resolve.go
                  (merge user styles/overrides over built-ins)
    render/       segment.go glyphs.go render.go + seg_dirgit.go seg_repo.go seg_ai.go seg_time.go
    preview/      ui.go (bubbletea Model/Update/View) fixtures.go  (+ golden tests)
  skill/SKILL.md  scripts/check-deps.sh  docs/design.md
```

**Subcommands** (cobra command tree):
- `gsl render` (default) — read JSON payload on stdin, print powerline line(s). Claude's path.
- `gsl status` — no stdin; pretty on-demand render (Gemini `/gsl-status` + CLI). Payload-only
  segments self-omit.
- `gsl preview [--once]` — render against sample fixtures; default is an interactive bubbletea
  TUI (toggle segments/glyphs/theme, 1s clock tick); `--once` prints one frame and exits
  (CI/golden-test friendly).
- `gsl config get|set|enable|disable|toggle [<segment>]` — drives the configuration skill.
- `gsl config style <name>` / `gsl config style --list` — switch the active style / list built-ins
  + user-defined styles.
- `gsl version [--json]`.

**Segments** (ordered, individually toggleable):
1. **dirgit** — `~`-abbreviated cwd basename + branch + p10k-style counts: staged `+N`,
   unstaged `!N`, untracked `?N`, stashes, ahead `⇡N`, behind `⇣N`. One
   `git -C <dir> status --porcelain=v2 --branch` + `git stash list`, via the `git.Runner` seam
   (`CommandContext`, ~800ms timeout). Mirrors `opt/profiles/.p10k.zsh` `my_git_formatter`.
2. **repo** — repo/worktree context, all from local git + the gss registry (no payload needed,
   so it renders in Claude *and* Gemini/CLI mode). Left→right, each part self-omitting:
   **🏠 root / 🌳 worktree** indicator (root if `git rev-parse --git-dir` ==
   `--git-common-dir`, else linked worktree); in a worktree the **gss feature name**; the
   **PR number** (`repo.PR()` → registry first, `gh` fallback); and the **worktree count**
   (`git worktree list`, shown when ≥ 2). Glyphs honor `glyphs` (🏠/🌳/⑂ vs `[root]`/`[wt]`/`wtN`);
   colors root=blue / worktree=magenta (themeable). Self-omits entirely outside a git repo.
3. **ai** — from the Claude payload: `model.display_name`, `context_window.used_percentage`
   + tokens vs `context_window_size`, MCP `active/configured`, and rate-limit usage for the
   5-hour and 7-day windows (`rate_limits.five_hour`/`seven_day` + `resets_at`). Returns
   `("", false)` (self-omits) when there is no payload (Gemini/CLI mode) or a field is null.
4. **time** — current time in a configurable tz (default `America/Los_Angeles`/PST) with
   Nerd-Font calendar + clock glyphs: day-of-week, month/day, `HH:MM`, tz abbreviation.

### Performance budget & graceful degradation (review comment #4)

All subprocess work goes through the seams with `context.WithTimeout` + `exec.CommandContext`:

| Call | Timeout | On timeout/error |
| --- | --- | --- |
| `git status …` + `git stash list` | ~800ms | omit git detail (show cwd only) or last value |
| `git rev-parse` + `git worktree list` (repo seg) | ~800ms (shared git budget) | omit worktree indicator/count |
| `~/.config/gss/worktrees/registry.json` read (PR# + name) | none (instant file read) | parse error ⇒ fall through to `gh` |
| `gh pr view` (PR# fallback only) | ~800ms | last PR cache, else omit `PR#` |
| `claude mcp list` (active count) | ~500ms | fall back to last MCP cache, else configured-only |
| **whole `gsl render`** | soft ~150ms / hard ~1000ms | emit partial line (never block) |

The hard cap (~1000ms) sits above the slowest per-call deadline (git ~800ms) so each per-call
timeout can actually fire before the parent context cancels it. The render passes a parent
context to every segment and runs the segments **concurrently** (so wall-clock ≈ the slowest
single call, not the sum); a segment that exceeds its deadline — or the parent ceiling — is
skipped, the rest still render. Timeout behaviour is unit-tested with fake runners that sleep
past the deadline (no real hang), and an end-to-end check injects a slow `git`/`claude` on
`PATH` to prove the line returns within budget.

**MCP detection** (`internal/mcp`): `ConfiguredCount(cwd)` parses `~/.claude.json`
(top-level `mcpServers` + `projects.<cwd>.mcpServers`) and `./.mcp.json`, honoring
`CLAUDE_CONFIG_DIR` — instant, no subprocess. `ActiveCount(...)` reads a cache at
`${XDG_CACHE_HOME:-~/.cache}/gsl/mcp.json`; if older than 60s it runs `claude mcp list`
through the `mcp.Runner` seam with the timeout above, parses connected vs failed markers, and
rewrites the cache; a cache hit (<60s) spawns **no** subprocess (asserted via a spy runner in
tests).

**Repo/worktree detection** (`internal/repo` + `internal/git/worktree.go`): root-vs-worktree is
`git rev-parse --path-format=absolute --git-common-dir` vs `--git-dir` — equal ⇒ root, differ ⇒
linked worktree (verified against this repo). Worktree count is `git worktree list --porcelain`
(count `^worktree ` lines), via the `git.Runner` seam under the shared git budget. Feature name +
PR number come from `repo.PR(branch, toplevel)`: read `${XDG_CONFIG_HOME:-~/.config}/gss/worktrees/
registry.json` (guard on `schema_version`), match the current `--show-toplevel` (or branch) to a
worker row, and take `pr_url` (parse the trailing number) + `pr_state` + the parent feature `name`
— **instant, no subprocess**. Only when the registry is absent / the branch isn't found does it
fall back to `internal/gh`: `gh pr view --json number,state` through the `gh.Runner` seam, cached at
`${XDG_CACHE_HOME:-~/.cache}/gsl/pr-<branch>.json` for 60s (cache hit ⇒ no subprocess, spy-tested),
timed out as above; no PR / no `gh` / no remote ⇒ the `PR#` part self-omits.

**Config + on/off** (`internal/config`): JSON at `${XDG_CONFIG_HOME:-~/.config}/gsl/config.json`,
with `enabled` (master), an ordered `segments[]` (each `{type, enabled, options}`), `timezone`,
time/date formats, the active `style` (default `powerline`), and a `styles` map of user
definitions/overrides (see "Styles"). The `repo` segment's `options` carry `show_pr` (bool),
`show_count` (bool), and `name` (`feature`|`worker`|`branch`|`off`, default `feature`). **Missing
file ⇒ sane defaults** (the binary works with zero config; `repo` enabled, `style: powerline`).
Two on/off layers, both honored: the harness `statusLine.command` key (hard off = remove it) and
`config.enabled` (soft off: `gsl render` prints empty stdout) plus per-segment toggles — all
reachable via `gsl config …`.

**Styles** (`internal/style`): a *style* is a named bundle controlling presentation (segments and
their data are unchanged): `separator` (`powerline` ` `/`thin` ` `/`space`), `fill` (bool —
draw segment background blocks), `glyphs` (`nerdfont`|`emoji`|`ascii`), a per-segment `icons` map,
and `theme` colors (incl. `repo_root`/`repo_worktree`, default blue/magenta). `builtins.go` ships
two compiled-in styles — **`powerline`** (default: NF tinted repo glyph, filled powerline) and
**`emoji`** (emoji icons, thin separators, no fill). `resolve.go` produces the effective style:
look up the built-in named by `config.style`, then deep-merge any same-named entry from
`config.styles` over it (so users tweak one field or define a brand-new style). Unknown style name
⇒ fall back to `powerline` (warn to stderr, never crash). `glyphs: ascii` (or no Nerd Font on
`PATH`/`TERM`) forces the ASCII fallback table regardless of style. The render layer reads only the
resolved `Style`, so adding a style is data, not code. Example user styles a contributor could drop
in `config.json` (not shipped, but exercised by the design): a `glyphicon` style (NF icon per
segment), a `compact` style (emoji status indicators — ⚠️ dirty, 🟢/🟡/🔴 rate-limit, ◉/● PR
state), and a `spacious` style (no fill, fg-only glyphs, wide separators).

### Wiring into install (exact edits)

- **NEW `ai/claude/statusline-command.sh`** (committed, `chmod +x`): the shim the settings
  template already references. Reads stdin once; if `~/opt/bin/gsl` exists, `exec`s
  `~/opt/bin/gsl render` forwarding the payload; otherwise prints a minimal bash fallback
  (`basename $PWD` + `git branch` + `date`) so the line never breaks pre-build.
- **`opt/scripts/system/install_claude_skills.sh`**: after the commands-linking block, add a
  guarded `ln -sf "$BASE_DIR/ai/claude/statusline-command.sh" "$CLAUDE_HOME/statusline-command.sh"`
  (same backup-if-not-symlink guard used for settings.json/aliases.sh) + `chmod +x`.
- **`opt/scripts/system/sync-skills.sh`**: line 97 build loop → add `"gsl"`; the name
  `case` (lines 114-116) → add `gsl) dest_name="gsl-status" ;;` so the skill links to
  `~/.claude/skills/gsl-status` and `~/.agents/skills/gsl-status`.
- **NEW `ai/gemini/commands/gsl-status.toml`** (mirror `ai/gemini/commands/gss.toml`):
  `description` + a pretty `prompt` blurb with `!{gsl status}` substitution and `{{args}}`.
  Auto-linked — `install_gemini_skills.sh` already globs `ai/gemini/commands/*.toml`
  (line 56), no script edit needed.
- **No edits required** to: `.gitignore` (`!src/**` already tracks `src/gsl/**`; the
  binary lives outside the repo), `install.sh` (already runs sync-skills + both install
  scripts), `Makefile` (`make bin` auto-discovers `src/*/build.sh`), and
  `ai/claude/settings.json.template` (the existing `Bash($HOME/opt/bin/*:*)` allow rule already
  covers `gsl`; the `statusLine.command` line is already correct).

### Files to create / modify (summary)

| Action | Path |
| --- | --- |
| NEW (tool tree) | `src/gsl/**` — cobra Go source, `build.sh`, `VERSION`, `internal/{repo,gh}` (worktree + PR# detection), `internal/style` (powerline + emoji built-ins), `internal/preview` (bubbletea), `skill/SKILL.md`, `scripts/check-deps.sh`, tests |
| NEW | `ai/claude/statusline-command.sh` (shim) |
| NEW | `ai/gemini/commands/gsl-status.toml` |
| EDIT | `opt/scripts/system/install_claude_skills.sh` (symlink shim) |
| EDIT | `opt/scripts/system/sync-skills.sh` (build loop + `gsl-status` case) |

### Patterns to reuse (don't reinvent)

- `src/gss/build.sh` — copy the goenv-aware, version-stamping build script verbatim, swapping
  names/paths to `gsl`.
- `src/gss/cmd/root.go` — cobra root + `Execute()` shape.
- `src/gss/internal/git` + its `fake/` runner — the `os/exec` seam + fakeable Runner pattern
  reused for `git`, `mcp`, and `gh`; `src/gss/scripts/check-deps.sh` — copy and adjust the seam
  allowlist to `internal/git` + `internal/mcp` + `internal/gh` (and allow cobra/bubbletea in
  `cmd/`/`preview`).
- `gss feature list --json` / `~/.config/gss/worktrees/registry.json` — the worker rows
  (`branch`, `worktree`, `pr_url`, `pr_state`, parent `name`) the `internal/repo` registry reader
  parses; guard on the top-level `schema_version`.
- `src/gss/internal/version` — version var + `Get()` fallback pattern.
- `opt/profiles/.p10k.zsh` `my_git_formatter` — authoritative glyph semantics
  (`⇡`/`⇣` ahead/behind, `+`/`!`/`?` staged/unstaged/untracked) to mirror.
- `ai/gemini/commands/gss.toml` — exact TOML command format for `gsl-status.toml`.
- `opt/scripts/system/install_claude_skills.sh` settings.json/aliases.sh symlink-with-backup
  blocks — copy that shape for the shim symlink.

---

## Execution plan (checkpoints → gss feature worker stack)

Conservative sizing; the value is the sequencing. CP1↔CP2 are mostly parallelizable after the
seams land; CP3 depends on both; CP4 is final.

- **CP1 — Scaffolding & core.** `src/gsl` tree; cobra root + `Execute()`; `build.sh` +
  `scripts/check-deps.sh` (from gss, seam allowlist adjusted); `internal/{version,payload,config}`
  and the `git`/`mcp` seam interfaces + `fake/` runners. Gate: `go build ./...`, check-deps
  green, no panic on missing config.
- **CP2 — Detection & render.** `internal/git` (porcelain v2 + timeout + `worktree.go`),
  `internal/mcp` (configured instant + active cached/timeout + spy-tested cache hit),
  `internal/repo` (root/worktree detect + registry reader) and `internal/gh` (cached PR# fallback
  + spy-tested cache hit), `internal/style` (`powerline` + `emoji` built-ins + resolve/merge),
  `internal/render` (**4 segments** incl. `seg_repo.go`, style-driven glyphs/separators/fills,
  per-segment deadline).
- **CP3 — CLI, preview & wiring.** `cmd/{render,status,config,version,preview}` incl.
  `gsl config style`; `internal/preview` bubbletea TUI (segment toggle + **style cycle**) + `--once`
  golden snapshot; `ai/claude/statusline-command.sh` shim + `install_claude_skills.sh` symlink;
  `sync-skills.sh` build loop + `gsl) gsl-status` case; `ai/gemini/commands/gsl-status.toml`;
  `skill/SKILL.md`.
- **CP4 — Docs & finalize.** `README.md`, `SKILL.md` polish, fold final decisions into this doc.

---

## Verification

1. **Build**: `bash src/gsl/build.sh` → `gsl built and installed to ~/opt/bin/gsl`,
   check-deps passes. (`make bin` also works.)
2. **Tests/coverage** (TDD, ≥60% per pkg per `src/CLAUDE.md`; targets: payload ~95, git 70,
   mcp 60, repo 70, gh 60, style 80, config 70, render 75, cmd 80, preview 60):
   `cd src/gsl && go test ./... -cover`. Seams faked via `git/fake` + `mcp/fake` + `gh/fake`;
   payload fixtures under `internal/payload/testdata/`, registry fixtures under
   `internal/repo/testdata/`. Cases per package:
   - payload: full fixture; malformed JSON → error; **empty stdin → empty struct, no error**;
     `used_percentage:null` → nil; `seven_day` absent but `five_hour` present.
   - git: scripted porcelain=v2 (staged/unstaged/untracked, ahead 2/behind 1, stashes); clean
     repo → zeros; not-a-repo error; detached HEAD; **timeout (fake sleeps past deadline) →
     degrades, no hang**.
   - mcp: configured count from temp `~/.claude.json` + `.mcp.json` (CLAUDE_CONFIG_DIR honored);
     active from fake `claude mcp list` (mixed ✓/✗); **cache hit <60s ⇒ spy runner NOT called**;
     stale cache + timeout ⇒ last value.
   - repo: root detect (`--git-dir`==`--git-common-dir`) vs linked-worktree (differ), via fake
     `git rev-parse`; worktree count from fake `git worktree list --porcelain` (1, 4); registry
     parse from fixture `registry.json` — match by toplevel **and** by branch, `pr_url`→number,
     `pr_state` + feature `name`; worker with no `pr_url`; **bumped `schema_version` ⇒ ignored**
     (falls through to gh); missing registry ⇒ gh fallback.
   - gh: PR# from fake `gh pr view` (open/draft/none); **cache hit <60s ⇒ spy NOT called**;
     timeout/`gh` absent/no remote ⇒ omit.
   - style: built-in lookup (`powerline`, `emoji`); unknown name ⇒ `powerline` + stderr warn;
     user `styles` entry **deep-merges** over a built-in (override one field) and **defines a new
     style**; `glyphs: ascii` forces the ASCII table regardless of style.
   - render: segment ordering + disable; master `enabled=false` ⇒ empty output; **repo indicator
     per style — `powerline` NF tinted ` `/` ` (root blue / worktree magenta) vs `emoji` 🏠/🌳**;
     PR#/count omission, `name` modes (feature/worker/branch/off); tz render + bad-tz→UTC fallback;
     nerdfont/emoji/ascii glyphs; **golden line at fixed time + fixed payload, per style × (root,
     worktree)**.
   - preview: `--once` golden frame; bubbletea model toggle/**style-cycle**/tick transitions.
   - config: Default(); Load-missing → defaults; round-trip; toggle; `style` set/list; bad-tz
     fallback.
3. **Render (Claude path)**: pipe a sample payload —
   `printf '%s' '{"cwd":"'"$PWD"'","model":{"display_name":"Opus 4.7"},"context_window":{"used_percentage":42,"total_input_tokens":84000,"context_window_size":200000},"rate_limits":{"five_hour":{"used_percentage":12.5,"resets_at":"2026-05-24T20:00:00Z"}}}' | ~/opt/bin/gsl render`
   → one powerline line: dir+git, repo(root/worktree·PR#·count), AI(model/ctx%/tokens/MCP/5h+7d), time.
4. **On-demand (Gemini/CLI)**: `~/opt/bin/gsl status` (no stdin) → dir+git+repo+time, AI omitted
   (repo renders without a payload).
5. **Repo segment (root vs worktree)**: run `~/opt/bin/gsl status` from the repo root → the root
   indicator (` ` in the default `powerline` style, 🏠 under `emoji`) + any PR# for the current
   branch + `⑂N`; run it from inside a gss worktree (`~/.config/gss/worktrees/.../plan`) → the
   worktree indicator (` `/🌳) + `<feature>` + `PR#<n>` (from `registry.json`, no network) + `⑂N`.
   Move/rename the registry → PR# falls back to a cached `gh pr view`; on a plain repo with no PR →
   `PR#` self-omits; on a lone main checkout → `⑂N` self-omits.
6. **Preview**: `gsl preview --once` → one rendered frame; `gsl preview` → interactive TUI,
   toggle a segment (incl. `repo`), cycle styles, watch the clock tick, `q` to exit.
7. **Styles**: `gsl config style emoji` → line re-renders with emoji markers / thin separators;
   `gsl config style --list` shows `powerline`*, `emoji` (+ any user styles); an unknown name
   (`gsl config style nope`) → falls back to `powerline` with a stderr warning; add a `styles`
   entry to `config.json` overriding one field → reflected on next render.
8. **Toggle**: `gsl config disable time` → time gone; `gsl config disable repo` → repo segment
   gone; `gsl config enable …` restores; `gsl config disable` (master) → empty output.
9. **Timeout proof**: prepend a `PATH` dir with a `git` (and `claude`/`gh`) that `sleep 5` →
   `time ~/opt/bin/gsl status` returns within the render budget with graceful degradation.
10. **Wiring**: `bash opt/scripts/system/sync-skills.sh --build` builds gsl + links
    `~/.claude/skills/gsl-status` and `~/.agents/skills/gsl-status`;
    `bash opt/scripts/system/install_claude_skills.sh` → `~/.claude/statusline-command.sh`
    symlink exists & executable; `bash opt/scripts/system/install_gemini_skills.sh` →
    `~/.gemini/commands/gsl-status.toml` linked.
11. **Shim fallback**: `mv ~/opt/bin/gsl ~/opt/bin/gsl.bak`, pipe a payload into
    `bash ~/.claude/statusline-command.sh` → minimal bash line; restore binary.
12. **Live Claude pickup**: start a Claude Code session; after an assistant turn the status
    line renders (harness pipes the live payload → shim → `gsl render`).
