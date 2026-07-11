# gsl ultra — design

- **Slug:** gsl-ultra
- **Date:** 2026-07-11
- **Status:** Proposed
- **Relates to:** issue [#158](https://github.com/sfc-gh-eraigosa/dotfiles/issues/158) · successor to `gsl-visual-improvements` (#54 / PR #55, **merged**) · absorbs #31
- **Author(s):** Edward Raigosa (planned via the MBO pipeline + an 8-dimension audit `Workflow`)

> **Provenance.** Every defect cited below is a **confirmed** finding from an 8-dimension audit
> `Workflow` (70 raised → 39 survived adversarial verification → 31 refuted). The headline four
> were then re-verified **by hand against the live binary** on 2026-07-11; those measurements are
> quoted inline. This design does not restate anything the audit could not reproduce.

## 1. Problem / context

`gsl` renders the powerline status line for Claude Code and Antigravity (`agy`). Its skeleton is
good — `Detect → Format(level) → Fit` is a clean I/O-once pure-render pipeline
(`sdk/gsl/internal/render/detect.go`), the subprocess seams are injectable and fake-tested, and
the golden suite is real. `go build`, `go vet` and `go test ./...` are all green.

That green is misleading. **The two things the user relies on most are non-functional in
production**, and the suite is green *because its fixtures encode the bug*:

**1.1 — The MCP count is always zero.** `connectedToken` (`internal/mcp/cache.go:134`) is
`"✓ Connected"` — **U+2713**. The real CLI emits **U+2714** `✔`. A codepoint census of live
`claude mcp list` output on this machine:

```
6x  ✔  U+2714       ← what the CLI actually prints
5x  ✘  U+2718
0x  ✓  U+2713       ← what gsl searches for
```

`parseConnectedCount` therefore returns 0 while **6 servers are genuinely connected and 5 are
failing**. The user can see neither. `internal/mcp/cache_test.go:51-55` hand-transcribed this
machine's real server names but retyped the glyphs as `✓`/`✗`, so CI validates a format the CLI
has never produced.

**1.2 — …and it could never have rendered anyway.** `runnerTimeout = 500ms`
(`internal/mcp/cache.go:21`), but `claude mcp list` **dials every server** and was measured at
**3.45 s / 3.73 s / 4.10 s**. The subprocess is SIGKILLed on 100% of renders. `writeCache` is only
reached on success, so the cache is *never seeded*, the 60 s TTL short-circuit *never engages*, and
every render spawns a fresh ~3.5 s Node process that is then killed — a continuous process-churn
storm, orphaning the `uvx` stdio servers it had begun spawning.

**1.3 — The line is always 80 columns wide.** `cmd/statusline.go:126` resolves width as
`$COLUMNS → ioctl(stdout) → 80`. `$COLUMNS` is a **shell variable that is not exported** to child
processes (verified: `env | grep -c '^COLUMNS='` → `0`), so that branch is **dead code in
production**. Claude Code pipes stdout, so the ioctl fails. Claude Code sends no `terminal_width`;
`agy status` sends no payload at all. **Every real render lands on the hardcoded 80.** A 200-column
terminal gets a line compacted for 80.

**1.4 — The model name renders as the word `context)`.** `shortenModelName`
(`internal/render/seg_ai_data.go:153`) returns the **last space-separated word**. Reproduced live:

```
$ echo '{"model":{"display_name":"Claude Opus 4.8 (1M context)"},…}' | gsl render
📁 dotfiles · 🏠 ⑂6 · 🤖 context) 🧠 42% · ⏰ 03:49 PM
                        ^^^^^^^^ the model name
```

It also byte-slices `name[:8]`, which is rune-unsafe.

**1.5 — Structural defects behind those four.** `Fit`'s phase-2 loop is `for len(active) > 1`, so
the *final* segment is never truncated — `Fit(20)` returns a 27–33-column line and the status bar
**wraps** (`internal/render/detect.go:275,292`). `repoData.format(st, _ int)` **discards the
compaction level** (`seg_repo_data.go:84`), so repo is 44 columns at every level and can only be
dropped whole. `gh.PR`'s error path returns `(nil, nil)` **without** `writeCache`
(`internal/gh/pr.go:126`), so a negative result is never cached and `gh pr view` — a **~760 ms
network call** — re-runs on every single turn for any branch without a PR (i.e. `main`). None of
the three exec seams set `cmd.WaitDelay`, so a grandchild holding the stdout pipe defeats *every*
timeout (measured **5121 ms against an 800 ms deadline**).

**1.6 — `agy` is a guess.** The quota bucket keys in `internal/payload/payload.go:133`
(`3p-5h`, `gemini-5h`, …) were **invented**; `agy` models quota as a *dynamic map*, so a key rename
silently deletes the display. There are **zero** captured agy payloads in `testdata/`.
`internal/theme/settings.go:71` reads `ui.theme`, but agy writes a top-level `colorScheme`.

**1.7 — The TUI is a fixture demo.** `internal/preview/model.go` renders only fakes, has no
lipgloss, no key map, no alt-screen, cannot load the user's real config and cannot save. Its
`segmentEnabled` **map** is aliased across bubbletea's value-receiver `Model` copies, so `Update`
mutates the caller's model (`model.go:123`).

## 2. Goals & non-goals

**Goals**
- The MCP segment reports **truth**: a connected / failed / needs-auth breakdown, correct counts,
  and it renders **without ever blocking a render**.
- The line is **exactly as wide as the host's real terminal**, and `DisplayWidth(line) ≤ cols`
  **always** — no wrap, no gratuitous over-compaction.
- Every rendered glyph is **legible**: no unpainted blocks, no stray wedges, contrast-safe
  foregrounds on every palette.
- Render latency is bounded and **no network call sits in the hot path**.
- `agy` is a **first-class host**, validated against a **real captured payload** — not a guess.
- `gsl config` (bare) is a **bubbletea config studio** that previews against the real repo and
  writes the config back.
- The suite proves all of the above by **property**, not by example — and its fixtures are
  **captured, never retyped**.

**Non-goals**
- No new *segments* (token history #34, tmux bar #33 stay separate objectives).
- No change to the `Detect → Format → Fit` architecture — it is sound; we fix its bugs.
- Not shipping gsl as a standalone plugin (#56 remains separate).
- No animated/streaming status line.

## 3. Options considered

### 3.1 How to get MCP status (the pivotal choice)

- **A — Keep parsing `claude mcp list` inline, just fix the glyph + raise the timeout.**
  *Rejected.* It treats the symptom. A 3.5–4.1 s command can never be inline in a per-turn
  render; raising the timeout to 5 s converts a broken count into a 5 s stall. And matching a
  *decorated glyph* is what broke it — U+2713 vs U+2714 is exactly the class of drift that
  recurs on the next CLI release.

- **B — Status source of truth = the CLI, but decoupled from the render (chosen).**
  Parse the **status keyword**, not the sigil: split each line on the last `" - "` and match
  `connected` / `failed` / `needs authentication` / `connecting` **case-insensitively**, treating
  U+2713/U+2714/U+2705 only as a *secondary* signal. Prefer a machine-readable `--json` mode if
  the CLI has one (probe once, cache the capability). Then **stale-while-revalidate**: a render
  *always* serves the cache instantly and *never* probes inline; when the entry is stale, fork a
  **detached** background refresh. Cache is keyed on `sha256(cwd)` — today it is a global constant
  holding a cwd-dependent value, so repo A poisons repo B (reproduced: renders `5/1`).
  Keep the offline JSON config scan as the *fallback* denominator, extended to plugin caches and
  deduplicated by server **name** with `local > project > user` precedence, minus
  `disabledMcpjsonServers`.
  *Chosen because it is the only option where a render can never stall and the count can never be
  silently wrong: the parser keys on words, and the fixtures are captured bytes.*

- **C — Reimplement MCP health-checking natively in Go (dial each server ourselves).**
  *Rejected.* We would own transport, auth, and the MCP handshake for every server type — a large
  surface to duplicate what `claude mcp list` already does, and it would drift from Claude's own
  notion of "connected".

### 3.2 Where terminal width comes from

- **A — Keep `$COLUMNS` first.** *Rejected:* it is not exported to child processes; the branch is
  dead in production and merely *looks* like it works.
- **B — A single injectable `resolveColumns()` with an explicit, logged precedence (chosen):**
  `payload.terminal_width` → `ioctl(stdout)` → **`ioctl(stderr)`** → `$COLUMNS` → configurable
  fallback (default **120**, not 80). The **stderr probe is the insight**: Claude pipes *stdout*
  but routinely leaves *stderr* attached to the tty — a free source of the real width when the
  payload omits it. The function returns `(cols, source string)` so the source is testable and
  loggable, and the four host scenarios become a table test rather than a guess.

### 3.3 Who owns colour

- **A — Today: `paint()` and `joinPowerline()` each resolve colour independently.** *Rejected.*
  They disagree: `joinPowerline` nests the fg emit inside `if bg != ""` but emits the chevron
  **unconditionally**, so a colour that resolves to `""` (a literal `"default"`, a hex value, an
  unknown key) yields an **unpainted block + a white wedge + a stray trailing triangle**.
- **B — One `resolveBlockColors(st, key) → (bg, fg)` that ALWAYS yields a concrete pair (chosen).**
  Parse `name | 0-255 | #rrggbb` → RGB; downsample by terminal capability; when only a bg is given,
  compute **WCAG relative luminance** and pick a contrast-safe fg. Both `paint()` and
  `joinPowerline()` consume it, so a block and its chevron can never disagree *by construction*.
  This also kills the hardcoded `white` fg on every filled block (white-on-yellow ≈ **1.35:1**).

## 4. Decision

Six workstreams, each an independently shippable PR. Landing order is **WS1 → WS3 → WS2 → WS6 →
WS4 → WS5** — correctness and latency first (they have a 100% hit rate and no dependencies), the
showcase TUI last (so it showcases working parts).

| WS | Name | Size | Depends on |
| :-- | :-- | :-- | :-- |
| **WS1** | Width & Fit correctness | S/M | — |
| **WS3** | Latency & timeout hardening | S | — |
| **WS2** | MCP status, done properly | M | WS4 (soft — text-only ships standalone) |
| **WS6** | agy parity + install hygiene | S | — |
| **WS4** | Style/theme engine | L | WS1 |
| **WS5** | The `gsl config` TUI | L | WS1, WS2, WS4 |

**WS1 — Width & Fit correctness.** `resolveColumns()` per decision 3.2. **Hard truncation** in
`Fit`: after the `len(active) > 1` loop, truncate the surviving raw text (pre-ANSI) with an
ellipsis so `DisplayWidth ≤ cols` is guaranteed, not hoped for. Real compaction levels in
`repoData.format` (drop worktree count → `PR#157`→`#157` → ellipsize). `shortenModelName` becomes
a **model alias table** (Opus/Sonnet/Haiku/Gemini) → first meaningful word → **rune-safe** prefix.

**WS3 — Latency & timeout hardening.** `writeCache` on `gh.PR`'s **error and empty** paths
(negative caching, shorter TTL on timeout). `cmd.WaitDelay` + `SysProcAttr{Setpgid: true}` +
**process-group kill** on all three seams, so a pipe-holding grandchild can no longer defeat a
deadline. **Delete the serial `git.Status`** at `statusline.go:93` and thread the resulting
`*git.Info` into `render.Deps` (4 git processes → 2; stops draining the shared 1 s budget).
Nil-guard `gh.PR`. Demote expected non-zero exits (no-PR, not-a-repo) from WARN to Debug — they
currently emit ~1.6k spurious warnings that drown real `segment.panic` records.

**WS2 — MCP status, done properly.** Per decision 3.1. Render shape is the **adaptive breakdown**
the user chose, degrading down the existing compaction ladder:

```
level 0    6✓ 5✗ 1!      ← failures visible when there is room
level 1    6✓ 5✗
level 2    6/12          ← collapse to the ratio
level 3    6
level 4   (dropped)
```

connected → `ok` role (green), failed → `err` (red), needs-auth → `warn` (yellow). Ships as plain
text first; the tint arrives with WS4's state roles. Fix the `N/0` guard (`seg_ai_data.go:118`
tests `configured>0 || active>0` but the formatter keys on `active>=0`, so it would render `6/0`
the moment the parse is fixed).

**WS6 — agy parity + install hygiene.** Model quota as `map[string]QuotaWindow` with **suffix
heuristics** (`5h`/`hour` vs `week`/`weekly`/`7d`) and render whatever buckets arrive — no invented
keys. **Capture a real agy payload** into `internal/payload/testdata/agy_live.json`. Read agy's
top-level `colorScheme` (with `ui.theme` as legacy fallback). Replace the `ln -sf` at
`opt/scripts/system/install_claude_skills.sh:145` with a copy, per the repo's copy-not-symlink
rule. Rewrite `sdk/gsl/skill/SKILL.md` for the post-#157 wiring.

**WS4 — Style/theme engine.** `resolveBlockColors()` per decision 3.3. New schema: `extends`,
`separator{style,bridge}`, `glyphs{nerdfont|emoji|ascii}`, `palette` (curated:
catppuccin/tokyonight/nord/gruvbox/dracula), per-role `colors{fg,bg}`, `fill`, `padding`,
`priority[]`, and a top-level `color_mode: auto|never|16|256|truecolor`. Honour `NO_COLOR` and
`TERM=dumb`. Add **state roles** (`ok/warn/err/muted`) that WS2's MCP counts and git-dirty consume.
Move the `prBadge` tint into the join layer so segments stay escape-free (the `segment.go:12`
invariant — `prBadge` currently embeds a bare `ansiReset` mid-segment). Validate
`config set style` against builtins ∪ user styles; add `gsl config validate`.

**WS5 — The `gsl config` TUI.** **Bare `gsl config` launches the studio**; `gsl config <sub>` keeps
its current CLI behaviour. `gsl preview --once` stays hermetic (fixtures + fixed clock) so the CI
goldens can never flake. lipgloss layout, `bubbles/key` + `bubbles/help`, `tea.WithAltScreen`.
Panels: live status line against the **real** config/repo · a **width ruler** (drag cols 20→200 and
watch the Fit tier change) · **MCP health panel** (per-server connected/failed/needs-auth) ·
style/palette picker with instant repaint · segment reorder + priority · per-role colour editor ·
glyph-health check. `[w]` writes back via `config.Save` — the missing preview→apply loop. Make
`cfg.Segments[i].Enabled` the single source of truth and **delete the aliased `segmentEnabled`
map**. Hoist `Detect` out of `View()` into a `tea.Cmd`.

**Also absorbed:** issue **#31** (payload parse is all-or-nothing — one bad field discards the
whole payload) lands in WS6 as per-field decoding. Draft PR #37 is stale and will be superseded.

## 5. Risks & blast radius

| Risk | Mitigation |
| :-- | :-- |
| **The MCP parser drifts again** on the next Claude CLI release | Key on **words**, not sigils. Fixtures are **verbatim byte captures**, never retyped. A `//go:build integration` test shells the **real** CLI and asserts ≥1 status line parses — the only defence against drift. |
| A detached background refresh spawns runaway processes | Single-flight via a lock file; negative caching with exponential backoff; process-group kill; the refresher is capped and idempotent. |
| Width truncation cuts mid-grapheme (CJK, ZWJ emoji) | Truncate on **grapheme clusters** via `uniseg`, on raw pre-ANSI text; the width-invariant property test sweeps CJK + emoji fixtures across cols 1..200. |
| Contrast logic regresses a palette someone likes | `TestContrast_AllBuiltinPalettes` gates WCAG ≥ 4.5:1; user config **always wins** over computed fg. |
| Colour-engine rewrite churns the goldens and hides a real diff | Goldens regenerate behind `-update`; emoji goldens asserted byte-identical for the default palette; review diffs with `cat -v`. |
| Coverage dips below the `sdk/` 60% floor | New units are pure functions with injected I/O — table-driven. `scripts/test.sh` gates. |
| Six parallel workers collide | `gss feature conflicts --json` is a pre-fan-out gate; the §6 DAG gives each leaf a disjoint path set. |

**Blast radius:** confined to `sdk/gsl` plus two shell touch-points
(`opt/scripts/system/install_claude_skills.sh`, `install.sh`'s `build.sh` invocation). No other
`sdk/` module. Default behaviour with no config and a wide terminal is *strictly better* — same
segments, correct width, correct model name, working MCP counts.

## 6. Rollback

Each workstream is an independent, separately-revertable PR with no forward-only migration:

- **WS1/WS3/WS6** are bug fixes — reverting restores the (broken) prior behaviour, nothing else.
- **WS2** is additive behind the existing MCP part; reverting restores `active/configured` (which
  renders nothing today, so the floor is unchanged). The cache file is versioned; an unreadable
  entry degrades to a cold miss.
- **WS4** ships the new schema as a **superset** — every existing `config.json` keeps working
  (`extends` defaults to the current builtins). Reverting drops the new keys, which older code
  already ignores.
- **WS5** adds a *new entrypoint* (bare `gsl config`); `gsl config <sub>` and `gsl preview` are
  untouched, so reverting removes the studio and leaves every existing path intact.

> Produced via an 8-dimension audit `Workflow` (architecture-team lenses, adversarially verified).
> Registered in `../index.md`. Matching spec: `../specs/gsl-ultra.md` · plan: `../plans/gsl-ultra.md`.
