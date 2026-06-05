# Execution Plan: `gsl` status line

Companion to [`gsl-status-line.md`](./gsl-status-line.md). That document is the
**design** (what & why); this is the **execution plan** (the ordered tasks, the
full file manifest, the test plan, and the integration plan). Where the two
overlap, the design doc is authoritative on intent and this doc is authoritative
on sequencing and acceptance gates.

## How to read this

- Work is grouped into **four checkpoints (CP1–CP4)** that map 1:1 onto a gss
  feature-worker stack on feature **gsl** (`feature/gsl/edward-raigosa/impl` and
  its workers). CP1↔CP2 parallelize once the seam interfaces land; CP3 depends on
  both; CP4 is final.
- Every task carries an **acceptance gate** — the objective check that closes it.
  A checkpoint is done only when all its task gates pass **and** the global gates
  (`go build ./...`, `check-deps.sh`, `go test ./... -cover` ≥ target) are green.
- Conventions are inherited verbatim from `sdk/gss`: goenv-aware `build.sh`,
  version stamping via `-X .../internal/version.*`, an `os/exec` **seam** confined
  to `internal/{git,mcp,gh}` and enforced by `scripts/check-deps.sh`, TDD with
  `≥60%` coverage per package (`src/CLAUDE.md`).

---

## Task breakdown by checkpoint

### CP1 — Scaffolding & core (no external calls yet)

| # | Task | Files | Gate |
| --- | --- | --- | --- |
| 1.1 | Create `sdk/gsl/` module (`go mod init github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl`), `VERSION`, `LICENSE` (Apache-2.0), `README.md` stub | `sdk/gsl/{go.mod,VERSION,LICENSE,README.md}` | `go build ./...` succeeds on empty tree |
| 1.2 | Copy `sdk/gss/build.sh` → `sdk/gsl/build.sh`, swap names/paths/module to `gsl`; output `~/opt/bin/gsl`; stamp version | `sdk/gsl/build.sh` | `bash sdk/gsl/build.sh` builds & installs; `gsl version` prints stamped version |
| 1.3 | Copy `sdk/gss/scripts/check-deps.sh`, adjust seam allowlist to `internal/{git,mcp,gh}`; allow cobra in `cmd/`, bubbletea in `internal/preview` | `sdk/gsl/scripts/check-deps.sh` | check-deps green on scaffold; fails if `os/exec` added outside seams |
| 1.4 | cobra root + `Execute()` (mirror `sdk/gss/cmd/root.go`); wire `main.go` | `sdk/gsl/main.go`, `sdk/gsl/cmd/root.go` | `gsl --help` lists command stubs |
| 1.5 | `internal/version` (var + `Get()` fallback, copy gss) | `sdk/gsl/internal/version/version.go` | `gsl version --json` returns version/commit/date |
| 1.6 | `internal/payload` — defensive parse of Claude stdin JSON; **all fields pointers**; empty stdin → empty struct, no error | `sdk/gsl/internal/payload/payload.go` + `testdata/` | unit tests in 2.x; compiles, parses sample fixture |
| 1.7 | `internal/config` — `Config`, `Default()`, `Load/Save`, segment + style fields, toggle helpers; missing file → defaults | `sdk/gsl/internal/config/config.go` | `Default()` returns working config; round-trips |
| 1.8 | Define seam interfaces + `fake/` runners for `git`, `mcp`, `gh` (no real exec yet) | `sdk/gsl/internal/{git,mcp,gh}/exec.go`, `.../fake/runner.go` | interfaces compile; fakes satisfy them |

**CP1 done when:** `go build ./...` + check-deps green; `gsl --help`/`version` work; no panic with no config file present.

### CP2 — Detection & render (parallel with CP1 after 1.8)

| # | Task | Files | Gate |
| --- | --- | --- | --- |
| 2.1 | `internal/git`: `status --porcelain=v2 --branch` + `git stash list` via `CommandContext` (~800ms); parse staged/unstaged/untracked/ahead/behind/stash | `internal/git/status.go` | unit tests pass (scripted porcelain, clean, not-a-repo, detached, timeout) |
| 2.2 | `internal/git/worktree.go`: `IsLinked` (`--git-dir` vs `--git-common-dir`), `Count` (`git worktree list --porcelain`), `Toplevel` | `internal/git/worktree.go` | root vs linked detection + count tests pass |
| 2.3 | `internal/mcp`: `ConfiguredCount` (instant parse of `~/.claude.json` + `./.mcp.json`, honor `CLAUDE_CONFIG_DIR`); `ActiveCount` (cache `${XDG_CACHE_HOME:-~/.cache}/gsl/mcp.json`, refresh ≤60s via `claude mcp list`, ~500ms) | `internal/mcp/{detect.go,cache.go}` | configured-count + active-count + **cache-hit-no-spawn** tests pass |
| 2.4 | `internal/repo`: root/worktree detect; `registry.go` reads `${XDG_CONFIG_HOME:-~/.config}/gss/worktrees/registry.json` (guard `schema_version`), match toplevel/branch → feature name + `pr_url`→number + `pr_state` | `internal/repo/{detect.go,registry.go}` + `testdata/registry.json` | registry parse + match + schema-bump-ignored tests pass |
| 2.5 | `internal/gh`: `gh pr view --json number,state` via seam, cached `${XDG_CACHE_HOME:-~/.cache}/gsl/pr-<branch>.json` 60s, ~800ms; PR# fallback only | `internal/gh/pr.go` | PR# parse + **cache-hit-no-spawn** + absent-gh-omit tests pass |
| 2.6 | `internal/repo/pr.go`: `repo.PR(branch, toplevel)` — registry first, gh fallback | `internal/repo/pr.go` | registry-hit avoids gh; missing registry → gh fallback |
| 2.7 | `internal/style`: `Style` struct, `builtins.go` (`powerline` default + `emoji`), `resolve.go` (deep-merge user `styles` over built-in; unknown→powerline+warn; `glyphs:ascii` forces ASCII) | `internal/style/{style.go,builtins.go,resolve.go}` | lookup/unknown/merge/new-style/ascii tests pass |
| 2.8 | `internal/render`: `segment.go`, `glyphs.go`, `render.go` (concurrent segments, parent ctx, per-segment deadline) + `seg_dirgit.go`, `seg_repo.go`, `seg_ai.go`, `seg_time.go` | `internal/render/*.go` | golden line per style × (root,worktree); ordering/disable; tz fallback tests pass |

**CP2 done when:** all detection packages ≥ coverage target; render produces correct golden lines for both built-in styles in root & worktree fixtures; timeout tests prove no hang.

### CP3 — CLI, preview & install wiring

| # | Task | Files | Gate |
| --- | --- | --- | --- |
| 3.1 | `cmd/render.go` — read stdin payload, print line(s) (Claude path) | `cmd/render.go` | sample payload → one powerline line |
| 3.2 | `cmd/status.go` — no stdin, pretty render; payload-only segments self-omit | `cmd/status.go` | `gsl status` → dir+git+repo+time, ai omitted |
| 3.3 | `cmd/config.go` — `get/set/enable/disable/toggle [<segment>]` + `config style <name>` / `--list` | `cmd/config.go` | toggle/style-switch/list reflected on next render |
| 3.4 | `cmd/preview.go` + `internal/preview` — bubbletea TUI (segment toggle, **style cycle**, 1s tick) + `--once` golden frame | `cmd/preview.go`, `internal/preview/{ui.go,fixtures.go}` | `gsl preview --once` golden; TUI toggles/cycles/ticks |
| 3.5 | `cmd/version.go` — `version [--json]` | `cmd/version.go` | matches version output |
| 3.6 | **NEW shim** `ai/claude/statusline-command.sh` (`chmod +x`): read stdin, `exec ~/opt/bin/gsl render` if present, else minimal bash fallback | `ai/claude/statusline-command.sh` | with binary → gsl line; without → bash fallback line |
| 3.7 | **EDIT** `install_claude_skills.sh`: symlink shim with backup-if-not-symlink guard + `chmod +x` | `opt/scripts/system/install_claude_skills.sh` | re-run → `~/.claude/statusline-command.sh` symlink, executable |
| 3.8 | **EDIT** `sync-skills.sh`: add `"gsl"` to build loop; add `gsl) dest_name="gsl-status" ;;` to name case | `opt/scripts/system/sync-skills.sh` | `--build` builds gsl, links `~/.claude/skills/gsl-status` + `~/.agents/skills/gsl-status` |
| 3.9 | **NEW** `ai/gemini/commands/gsl-status.toml` (mirror `gss.toml`): `description` + prompt with `!{gsl status}` + `{{args}}` | `ai/gemini/commands/gsl-status.toml` | `install_gemini_skills.sh` auto-links it (glob, no script edit) |
| 3.10 | `skill/SKILL.md` for gsl (drives `gsl-status` skill) | `sdk/gsl/skill/SKILL.md` | links via sync-skills |

**CP3 done when:** all subcommands work; both wiring scripts link correctly on a clean re-run; Gemini command linked; shim fallback verified with binary moved aside.

### CP4 — Docs & finalize

| # | Task | Files | Gate |
| --- | --- | --- | --- |
| 4.1 | `sdk/gsl/README.md` (usage, subcommands, config schema, styles) | `sdk/gsl/README.md` | renders; matches behavior |
| 4.2 | `sdk/gsl/docs/design.md` (in-tree design pointer) + polish `SKILL.md` | `sdk/gsl/docs/design.md` | consistent with this plan |
| 4.3 | Fold final decisions back into `docs/plans/gsl-status-line.md` + check this execution plan's boxes | `docs/plans/*` | plan reflects shipped reality |

---

## File manifest (complete)

| Action | Path | CP | Purpose |
| --- | --- | --- | --- |
| NEW | `sdk/gsl/go.mod`, `go.sum` | 1 | module `github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl`; deps cobra (Apache-2.0), bubbletea (MIT), yaml |
| NEW | `sdk/gsl/VERSION`, `LICENSE` | 1 | version source; Apache-2.0 |
| NEW | `sdk/gsl/build.sh` | 1 | goenv-aware build, version stamp, install to `~/opt/bin/gsl`, run check-deps |
| NEW | `sdk/gsl/scripts/check-deps.sh` | 1 | seam + license gate (`os/exec` only in `internal/{git,mcp,gh}`) |
| NEW | `sdk/gsl/main.go`, `cmd/root.go` | 1 | cobra root + `Execute()` |
| NEW | `sdk/gsl/cmd/{render,status,config,preview,version}.go` | 3 | subcommand tree |
| NEW | `sdk/gsl/internal/version/version.go` | 1 | version var + `Get()` |
| NEW | `sdk/gsl/internal/payload/payload.go` (+ `testdata/`) | 1 | Claude stdin JSON parse (pointer fields) |
| NEW | `sdk/gsl/internal/config/config.go` | 1 | config schema, defaults, load/save, toggles |
| NEW | `sdk/gsl/internal/git/{exec,status,worktree}.go` + `fake/runner.go` | 1–2 | git seam, porcelain v2, worktree |
| NEW | `sdk/gsl/internal/mcp/{detect,cache}.go` + `fake/runner.go` | 1–2 | configured (instant) + active (cached) MCP counts |
| NEW | `sdk/gsl/internal/repo/{detect,registry,pr}.go` (+ `testdata/registry.json`) | 2 | root/worktree, gss registry reader, PR# resolve |
| NEW | `sdk/gsl/internal/gh/{exec,pr}.go` + `fake/runner.go` | 1–2 | gh seam, cached `gh pr view` fallback |
| NEW | `sdk/gsl/internal/style/{style,builtins,resolve}.go` | 2 | style struct, powerline+emoji built-ins, merge |
| NEW | `sdk/gsl/internal/render/{segment,glyphs,render,seg_dirgit,seg_repo,seg_ai,seg_time}.go` | 2 | concurrent segment renderer |
| NEW | `sdk/gsl/internal/preview/{ui,fixtures}.go` | 3 | bubbletea preview TUI + fixtures |
| NEW | `sdk/gsl/skill/SKILL.md` | 3 | `gsl-status` skill |
| NEW | `sdk/gsl/README.md`, `docs/design.md` | 4 | docs |
| NEW | `ai/claude/statusline-command.sh` | 3 | shim the settings template already references |
| NEW | `ai/gemini/commands/gsl-status.toml` | 3 | Gemini on-demand command |
| EDIT | `opt/scripts/system/install_claude_skills.sh` | 3 | symlink shim (backup-guard + chmod) |
| EDIT | `opt/scripts/system/sync-skills.sh` | 3 | build-loop `gsl` + `gsl) gsl-status` case |
| NONE | `.gitignore` (`!src/**` covers tree), `install.sh`, `Makefile` (`make bin` auto-discovers), `ai/claude/settings.json.template` (allow-rule + `statusLine.command` already correct) | — | no edits required |

---

## Test plan

**Strategy:** TDD; seams faked via `git/fake` + `mcp/fake` + `gh/fake`; payload fixtures under `internal/payload/testdata/`, registry fixtures under `internal/repo/testdata/`. Run `cd sdk/gsl && go test ./... -cover`. Per-package coverage targets (floor `60%` per `src/CLAUDE.md`):

| Package | Target | Key cases |
| --- | --- | --- |
| payload | ~95% | full fixture; malformed JSON → error; **empty stdin → empty struct, no error**; `used_percentage:null` → nil; `seven_day` absent / `five_hour` present |
| git | 70% | scripted porcelain=v2 (staged/unstaged/untracked, ahead 2/behind 1, stashes); clean → zeros; not-a-repo error; detached HEAD; **timeout (fake sleeps past deadline) → degrade, no hang** |
| mcp | 60% | configured from temp `~/.claude.json` + `.mcp.json` (`CLAUDE_CONFIG_DIR` honored); active from fake `claude mcp list` (mixed ✓/✗); **cache hit <60s ⇒ spy NOT called**; stale + timeout → last value |
| repo | 70% | root (`--git-dir`==`--git-common-dir`) vs linked (differ); worktree count (1, 4); registry match by toplevel **and** branch, `pr_url`→number, `pr_state` + feature name; worker w/o `pr_url`; **bumped `schema_version` ⇒ ignored → gh fallback**; missing registry → gh |
| gh | 60% | PR# from fake `gh pr view` (open/draft/none); **cache hit <60s ⇒ spy NOT called**; timeout / gh absent / no remote → omit |
| style | 80% | built-in lookup (`powerline`,`emoji`); unknown → `powerline` + stderr warn; user `styles` **deep-merge** over built-in (one field) and **new style**; `glyphs:ascii` forces ASCII table |
| config | 70% | `Default()`; load-missing → defaults; round-trip; toggle; `style` set/list; bad-tz fallback |
| render | 75% | ordering + disable; master `enabled=false` → empty; **repo indicator per style (powerline tinted root/wt vs emoji 🏠/🌳)**; PR#/count omission; `name` modes (feature/worker/branch/off); tz + bad-tz→UTC; nerdfont/emoji/ascii; **golden line at fixed time+payload, per style × (root,worktree)** |
| cmd | 80% | each subcommand happy path + flag parsing; `render` from stdin; `status` no-stdin omits ai |
| preview | 60% | `--once` golden frame; bubbletea model toggle/**style-cycle**/tick transitions |

**End-to-end timeout proof (non-unit):** prepend a `PATH` dir with `git`/`claude`/`gh` that `sleep 5`; `time ~/opt/bin/gsl status` returns within the render budget (soft ~150ms / hard ~1000ms) with graceful degradation.

**Golden tests:** fixed clock + fixed payload, asserted per style × {root, worktree}; `gsl preview --once` snapshot.

---

## Integration plan

**Order matters** — build the binary before linking the skill, link the shim before relying on the live status line.

1. **Build & install:** `bash sdk/gsl/build.sh` → `~/opt/bin/gsl`; check-deps green. (`make bin` auto-discovers it.)
2. **Skill sync:** `sync-skills.sh` build loop gains `gsl`; the name `case` maps `gsl) dest_name="gsl-status"`, so `bash opt/scripts/system/sync-skills.sh --build` links `~/.claude/skills/gsl-status` and `~/.agents/skills/gsl-status`.
3. **Claude shim:** `install_claude_skills.sh` symlinks `ai/claude/statusline-command.sh` → `~/.claude/statusline-command.sh` (backup-if-not-symlink guard, `chmod +x`). The settings template's `statusLine.command` already points at this path, and the `Bash($HOME/opt/bin/*:*)` allow-rule already covers `gsl render`.
4. **Gemini command:** `ai/gemini/commands/gsl-status.toml` is auto-linked by `install_gemini_skills.sh`'s existing `*.toml` glob — no script edit.
5. **Full installer:** `./install.sh` already runs sync-skills + both install scripts, so a fresh clone wires everything with no new install.sh edit.

**Rollout / on-off layers (both honored):**
- **Hard off:** remove `statusLine.command` from Claude settings.
- **Soft off:** `gsl config disable` (master) → `gsl render` prints empty stdout; per-segment `gsl config disable <segment>`.

**Fallback / safety:**
- Pre-build (no `~/opt/bin/gsl`): shim prints a minimal bash line (`basename $PWD` + branch + date) so the status line never breaks.
- Every subprocess is `CommandContext` with the budget in the design doc's timeout table; on timeout each segment degrades (omit / last cache) and the line still renders.

---

## Verification checklist (acceptance)

Mirrors the design doc's Verification section as closable gates:

- [x] **Build:** `bash sdk/gsl/build.sh` installs `~/opt/bin/gsl`; check-deps passes
- [x] **Tests:** `go test ./... -cover` ≥ target per package (git 94.3, repo 94.9, style 96.5, render 91.6, preview 83.5, cmd 80.7, payload 90.0, config 79.1, mcp 65.1, gh 63.0; `-race` clean)
- [x] **Render (Claude):** sample payload piped to `gsl render` → 4-part powerline line
- [x] **On-demand (Gemini/CLI):** `gsl status` (no stdin) → dir+git+repo+time, ai omitted
- [x] **Repo segment:** from a gss worktree → worktree indicator + feature name (`gsl`) + `⑂N`; `PR#` self-omits when the worker is unpushed; root/registry-moved/no-PR/lone-main paths covered by `internal/repo` unit tests (registry-first, `gh` fallback, schema-bump-ignored)
- [x] **Preview:** `gsl preview --once` one frame; TUI toggle/style-cycle/tick covered by `internal/preview` Model.Update tests
- [x] **Styles:** `gsl config style emoji` re-renders; `--list` shows `powerline`*,`emoji`(+user); unknown → powerline + warn; user override (fill-presence) reflected
- [x] **Toggle:** disable `time` removes it; enable restores; master disable → empty
- [x] **Timeout proof:** slow `git`/`gh`/`claude` on PATH → `gsl status` returns ~1s and degrades (verified with `exec sleep` fakes; `SystemRunner` uses `exec.CommandContext`)
- [x] **Wiring:** `sync-skills --build` links `gsl-status` (both dirs) + builds gsl; `install_claude_skills.sh` → executable shim symlink; `install_gemini_skills.sh` auto-links `gsl-status.toml` via its `*.toml` glob (verified under a temp `HOME`); `install.sh` builds the binary on a fresh clone
- [x] **Shim fallback:** binary moved aside → minimal bash line; `execfail`-safe; restored after
- [ ] **Live Claude pickup:** _deferred — requires activating the live status line in `~/.claude` (the `statusLine.command` + shim symlink); intentionally left as the user's opt-in step, not done autonomously. Run `./install.sh` (or `install_claude_skills.sh`) on the host to enable, then it renders via shim → `gsl render` after each turn._

## Definition of done

- [x] All four checkpoints (CP1–CP4) closed, each two-stage reviewed (spec + code-quality) with fixes folded in.
- [x] Coverage gate green (all packages ≥ their floor); `go vet` + `-race` clean.
- [x] **check-deps seam gate green**; **license gate SKIP** — `go-licenses` is not installed on the build host, so the optional license scan is skipped (set `GSL_STRICT_CHECK=1` after `go install github.com/google/go-licenses@latest` to enforce). Dependencies were **manually verified permissive**: cobra **Apache-2.0**, bubbletea **MIT**, transitives MIT/BSD — compliant with `src/CLAUDE.md`.
- [x] `./install.sh` on a clean clone wires gsl end-to-end (a `gsl` build block was added to `install.sh` — the plan had assumed no edit was needed, but install.sh builds each Go tool via an explicit block rather than auto-discovery).
- [x] Design doc updated to match shipped behavior (see "Shipped" note below).
- [x] Ships as the **`impl` PR #27** on the **gsl** feature, alongside the **plan** PR #21.
- [ ] Live Claude status-line activation — deferred to the user (see the acceptance gate above).
