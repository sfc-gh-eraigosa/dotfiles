# AI Teams: Install, Route, Validate, and Self-Improve — Design

- **Date:** 2026-06-01
- **Status:** Draft (design review)
- **Feature / worker:** `ai-teams-install/edward-raigosa/design`
- **Branch:** `feature/ai-teams-install/edward-raigosa/design`

---

## 1. Context & Problem

`ai/teams/` contains five specialized agent "teams" (web, go, terraform-aws, ai-ci,
architecture), each a folder of `the_*.md` persona files. Today these files are **inert
documentation**:

- `install.sh` never references `teams`; neither do `install_gemini_skills.sh`,
  `install_claude_skills.sh`, or `sync-skills.sh`.
- `sync-skills.sh` only globs `src/*/skill/`, `src/*/SKILL.md`, `ai/skills/*`, and
  `.gemini/skills/*` — `ai/teams/` matches none of these.
- The team folders have no `GEMINI.md` / `CLAUDE.md` symlinks, so even passive
  navigation is absent.
- The persona files use a **custom comment-header convention** (`# Persona:`,
  `# Aliases:`, `# Model:` block) that no tool consumes natively, and the model slugs
  inside them are **already stale** (`claude-sonnet-4-5`, `claude-opus-4-5` vs the
  current `claude-opus-4-8`, `claude-sonnet-4-6`).

Both target CLIs *do* support native custom subagents, but with **different schemas**,
and two more targets (Antigravity, Ollama) use entirely different mechanisms. So
installing teams is a **transform-and-resolve** job, not a symlink job.

### Native agent formats (confirmed)

| Tool | Install location | Format | System prompt | Model selector |
|------|------------------|--------|---------------|----------------|
| **Claude Code** | `~/.claude/agents/` (recursive; subfolders organize, `name` is the invocation slug) | Markdown + YAML frontmatter | file body | `model: opus\|sonnet\|haiku\|<id>\|inherit` + `effort: low\|medium\|high\|xhigh\|max` |
| **Gemini CLI** | `~/.gemini/agents/*.md` | Markdown + YAML frontmatter | file body | `model: <id>\|inherit` + `temperature: 0.0–2.0` |
| **Antigravity** | `~/.config/antigravity/agents/*.yaml` *(unconfirmed; see §17)* | Pure YAML | `instructions:` field | `model: <id>` |
| **Ollama** | Modelfile → `ollama create <name>` | `FROM` + `SYSTEM` + `PARAMETER` | `SYSTEM` block | `FROM <model>` |

**Routing constraint that shapes the whole design:** subagents **cannot invoke other
subagents** in either CLI (Claude restricts the `Agent` tool to the main session;
Gemini explicitly forbids subagent→subagent calls). Therefore a "router subagent" is a
dead end — it could only *recommend*, never dispatch. Routing must live in the **main
session** and is driven by each agent's `description`, assisted by a generated index.

---

## 2. Goals / Non-goals

### Goals
1. Make `ai/teams/` agents **installed and discoverable** for Claude, Gemini,
   Antigravity, and Ollama from a single source of truth.
2. Drive everything from the agent **header parameters**; decouple personas from
   churning model names via a **common model-map resource** that maps an abstract
   **tier** → concrete model + effort/temperature per tool.
3. Enable **right-team / right-member routing** (e.g. a `*.go` feature uses the Go team,
   not the web team), with a **squad** layer for cross-team tasks.
4. **Validate** source + map with zero new dependencies.
5. Provide a **routing eval** that gates merges at **≥90%** top-1 team *and* member
   accuracy across both runners.
6. Provide a **self-improvement skill** that closes the loop from eval misroutes back to
   source edits.

### Non-goals
- Training or fine-tuning models.
- Auto-`ollama pull` of base models (heavy; opt-in only).
- Re-architecting the existing skills/plugins install paths.
- Defining Antigravity/Ollama *runtime* behavior beyond emitting their artifacts.

---

## 3. Decisions (locked during brainstorming)

| # | Decision | Choice |
|---|----------|--------|
| D1 | Model selector in header | **Abstract `tier:` hint → resolved by `model-map.yaml`** |
| D2 | Transform mechanism | **Install-time transformer script** (`install_ai_teams.sh`, bash + `yq`) |
| D3 | Target tools | **All four**: Claude, Gemini, Antigravity, Ollama (graceful-skip on absent tools) |
| D4 | Source format | **Refactor team files to YAML frontmatter** (Approach A) |
| D5 | Routing | **Compiled descriptions + generated index/command** (team-scoped paths; two-level team→member) |
| D6 | Validation | **Zero-dep `yq` assertions** (`validate.sh`), no JSON-Schema tool |
| D7 | PR scope | **Everything in one PR** (all three phases implemented) |
| D8 | Eval gate | **Blocking CI gate, both runners**, team ≥90% AND member ≥90% |
| D9 | Grouping | **Per-team folders + cross-team squads** in `teams.yaml` |

---

## 4. Architecture Overview

```
                ai/teams/  (single source of truth)
  ┌───────────────────────────────────────────────────────────┐
  │  teams.yaml        teams registry + squads                 │
  │  model-map.yaml    tier → {claude,gemini,antigravity,ollama}│
  │  _partials/*.md    shared prompt fragments (compose:)       │
  │  <team>/the_*.md   YAML frontmatter + persona body          │
  │  eval/cases.yaml   labeled task → {team, member}            │
  │  validate.sh       zero-dep yq assertions                   │
  └───────────────────────────────────────────────────────────┘
                              │
                 install_ai_teams.sh (transformer)
        parse frontmatter (yq) → resolve tier → compose body
                              │
        ┌──────────┬──────────┴───────────┬─────────────┐
        ▼          ▼                      ▼             ▼
   Claude      Gemini               Antigravity      Ollama
 ~/.claude/   ~/.gemini/          ~/.config/        ~/.config/ollama/
  agents/      agents/             antigravity/      teams/*.Modelfile
  teams/       teams/              agents/*.yaml      + ollama create
        │          │
        └────┬─────┘
             ▼
   Discoverability: INDEX.md · /team (Claude) · team.toml (Gemini) ·
   per-team GEMINI.md + CLAUDE.md→GEMINI.md symlinks · root CLAUDE.md row
             │
             ▼
   Phase 2: route-eval (claude + gemini headless) → CI gate ≥90%
   Phase 3: teams-tune skill → propose source edits → regenerate → re-eval
```

Components are independent units with clear interfaces:
- **Source data** (declarative; no logic).
- **Transformer** (pure function: source + map → per-tool artifacts).
- **Per-tool emitters** (each owns one output format; isolated and independently testable).
- **Validator** (read-only assertions over source data).
- **Eval harness** (read-only over emitted descriptions; produces a score).
- **Self-improvement skill** (proposes diffs to source; never edits generated output).

---

## 5. Source-of-truth Layout

```
ai/teams/
  GEMINI.md                 # nav doc; CLAUDE.md -> GEMINI.md symlink
  CLAUDE.md -> GEMINI.md
  teams.yaml                # team registry + squads
  model-map.yaml            # tier → per-tool model/effort/temperature
  validate.sh               # zero-dep yq assertions (source + map + teams.yaml)
  INDEX.md                  # GENERATED by installer (committed copy regenerated on sync)
  _partials/
    common-safety.md        # repo safety + privacy rules (every agent)
    repo-conventions.md     # $HOME portability, gss workflow, etc.
    handoff-footer.md       # cross-member handoff protocol
  <team>/                   # web | go | terraform-aws | ai-ci | architecture
    GEMINI.md               # per-team nav; CLAUDE.md -> GEMINI.md
    CLAUDE.md -> GEMINI.md
    the_*.md                # refactored: YAML frontmatter + persona body
  eval/
    cases.yaml              # labeled routing dataset
    route-eval.sh           # runner + scorer (Phase 2)
```

All of `ai/teams/**` is already git-tracked via the `!ai/**` allowlist rule, so new
files appear in `git status` without new `.gitignore` entries. (Verified during design:
`docs/superpowers/specs/` is likewise covered by `!docs/**`.)

---

## 6. Schemas

### 6.1 Agent frontmatter (`<team>/the_*.md`)

```yaml
---
name: the_go_developer          # required; file slug
team: go                        # required; must equal parent folder & a teams.yaml key
role: godev                     # required; short member slug, unique within team
tier: standard                  # required; must exist in model-map.yaml
description: ""                  # optional author override; else compiled (see §8.1)
domain: "Go services, CLIs, libraries"   # required; one-line human domain
file_globs: ["**/*.go", "go.mod", "go.sum"]   # required; non-empty
keywords: [go, golang, cobra, grpc]      # required; non-empty
use_when: "Idiomatic Go, Cobra CLIs, gRPC, *.go changes, race/fuzz testing"   # required
avoid_when: "Frontend/TS, infra/Terraform, prose-only docs"                   # required
color: purple                   # optional; named color (Claude only)
symbol: "🐹"                     # optional; cosmetic, used in INDEX
context_strategy: standard      # optional; aggressive|standard|deep (informational)
compose:                        # optional; ordered; default = [__body__] only
  - _partials/common-safety.md
  - _partials/repo-conventions.md
  - __body__
  - _partials/handoff-footer.md
---

You are **The Go Developer**, ... (persona body becomes part of the system prompt)
```

Required keys: `name, team, role, tier, domain, file_globs, keywords, use_when,
avoid_when`. Everything else optional.

### 6.2 `model-map.yaml` — the common resource

```yaml
# Single place model churn is fixed. Personas never name a model.
tiers:
  fast:
    claude:      { model: haiku,            effort: low }
    gemini:      { model: gemini-2.0-flash, temperature: 0.4 }
    antigravity: { model: gpt-4.1-mini,     effort: low }
    ollama:      { model: "qwen2.5-coder:1.5b", num_ctx: 4096 }
  standard:
    claude:      { model: sonnet,           effort: medium }
    gemini:      { model: gemini-2.5-flash, temperature: 0.3 }
    antigravity: { model: gpt-4.1,          effort: medium }
    ollama:      { model: "qwen2.5-coder:7b", num_ctx: 8192 }
  think:
    claude:      { model: sonnet,           effort: high }
    gemini:      { model: gemini-2.5-flash, temperature: 0.2 }
    antigravity: { model: gpt-4.1,          effort: high }
    ollama:      { model: "qwen2.5:7b",     num_ctx: 8192 }
  deep-think:
    claude:      { model: opus,             effort: high }
    gemini:      { model: gemini-2.5-pro,   temperature: 0.2 }
    antigravity: { model: o3,               effort: max }
    ollama:      { model: "gemma3:27b",     num_ctx: 8192 }
```

> The current `MODEL_PARAMS.md` table seeds these values; once `model-map.yaml` exists it
> becomes the authority and `MODEL_PARAMS.md` is rewritten to point at it (no duplicated
> slugs).

### 6.3 `teams.yaml` — registry + squads

```yaml
teams:
  web:           { display: "Web Development",   symbol: "🎨", color: purple, default_tier: standard }
  go:            { display: "Go Development",    symbol: "🐹", color: cyan,   default_tier: standard }
  terraform-aws: { display: "Terraform & AWS",   symbol: "☁️",  color: orange, default_tier: standard }
  ai-ci:         { display: "AI & CI",           symbol: "🤖", color: green,  default_tier: standard }
  architecture:  { display: "Architecture",      symbol: "🏛️", color: blue,   default_tier: deep-think }

squads:
  fullstack-feature: [web/fe, web/api, go/godev, architecture/sysarch]
  secure-release:    [go/gosec, web/websec, architecture/secarch]
  infra-rollout:     [terraform-aws/infra, terraform-aws/platform, terraform-aws/cloudsec]
```

Squad members are `<team>/<role>` references; the validator asserts each resolves to a
real agent.

### 6.4 `compose:` partials mechanism

The transformer builds each agent's final system prompt by concatenating, **in order**,
the items in `compose:`. The sentinel `__body__` is replaced by the file's own markdown
body; every other entry is a path (relative to `ai/teams/`) to a shared `.md` fragment.
Default when `compose:` is omitted: `[__body__]`. This keeps repeated content
(safety/privacy rules, repo conventions, handoff protocol) DRY — one partial edit
propagates to all agents on the next sync.

---

## 7. Transformer / Installer — `opt/scripts/system/install_ai_teams.sh`

Mirrors `install_gemini_skills.sh` / `install_claude_skills.sh`. Bash + `yq`
(mikefarah, already installed by `install_yq.sh` and used by `sync-plugins`).

**Algorithm (per source `the_*.md`):**
1. `yq` reads frontmatter into shell vars.
2. Resolve `tier` against `model-map.yaml` → `{model, effort/temperature/num_ctx}` per tool.
3. Assemble system prompt from `compose:` (read partials + `__body__`).
4. Compile `description` (see §8.1) unless author override present.
5. Call each per-tool emitter (independent; failure of one warns + continues).

**Idempotent:** regenerating over an unchanged source + map yields byte-identical output
(stable key order, no timestamps in artifacts). Re-running the installer never produces a
diff.

**Graceful degradation (matches `sync-plugins` "continuing" pattern):** a missing tool
home dir, missing `ollama`, or unconfirmed Antigravity path → emit a `WARNING` and skip
that tool only; never fail the whole install.

### 7.1 Per-tool emitters

| Emitter | Output path | Key mapping |
|---------|-------------|-------------|
| Claude | `~/.claude/agents/teams/<team>/<role>.md` | frontmatter `name: <team>-<role>` *(subfolder is organizational only)*, `description`, `model`, `effort`, `color`, `tools`; body = composed prompt |
| Gemini | `~/.gemini/agents/teams/<team>/<role>.md` (fallback flat `<team>-<role>.md`) | `name: <team>-<role>`, `description`, `model`, `temperature`, `tools`; body = composed prompt |
| Antigravity | `~/.config/antigravity/agents/<team>-<role>.yaml` | `name: <team>-<role>`, `description`, `model`, `instructions:` = composed prompt |
| Ollama | `~/.config/ollama/teams/<team>/<role>.Modelfile` → `ollama create teams-<team>-<role>` (only if `ollama` on PATH) | `FROM <model>`, `PARAMETER num_ctx`, `SYSTEM <composed prompt>` |

> **Naming note:** the frontmatter `name` must be lowercase letters + hyphens and unique
> within scope, so the invocation slug is `<team>-<role>` (e.g. `go-godev`, `web-fe`) for
> Claude, Gemini, and Antigravity alike. Claude subfolders (`teams/<team>/`) only organize
> files on disk; the colon-scoped `plugin:team:member` form applies to plugin-packaged
> agents, not the loose user agents we install here. Ollama model names fold the team in
> as `teams-<team>-<role>`.

---

## 8. Target Layouts — multi-team grouping

Grouping is inherent in the source folders and mirrored per tool using each tool's
namespacing. Worked example (`go`, `web`, `architecture`):

**Claude** (recursive scan; subfolder organizes, `name` is the invocation slug):
```
~/.claude/agents/teams/
  go/godev.md              name: go-godev          → @agent-go-godev
  go/goqa.md               name: go-goqa           → @agent-go-goqa
  web/fe.md                name: web-fe            → @agent-web-fe
  architecture/sysarch.md  name: architecture-sysarch
```

**Gemini / Antigravity** (group via name prefix; nest files, fall back to flat):
```
~/.gemini/agents/teams/go/godev.md       name: go-godev
~/.config/antigravity/agents/go-godev.yaml  name: go-godev
```

**Ollama** (Modelfiles nested on disk; flat namespaced model names):
```
~/.config/ollama/teams/go/godev.Modelfile   → ollama create teams-go-godev
                       web/fe.Modelfile      → ollama create teams-web-fe
```

**Naming convention (consistent everywhere):** member slug = `role`; invocation `name` =
`<team>-<role>` (`go-godev`, `web-fe`) for Claude/Gemini/Antigravity; Ollama model name =
`teams-<team>-<role>`. Claude subfolders are organizational only. Adding a team = new
`ai/teams/<team>/` folder + `teams.yaml` row; all targets regenerate.

### 8.1 Compiled `description` (the routing signal)

Both CLIs route on `description`. The emitter compiles a high-signal, negatively-scoped
description from the frontmatter:

```
[<team> team · <role>] <domain>. Use PROACTIVELY for: <use_when>.
Matches files: <file_globs joined>. Keywords: <keywords joined>.
Do NOT use for: <avoid_when>.
```

Negative scoping (`Do NOT use for …`) is the lever that stops the web team firing on
`.go` files. An author may override with an explicit `description:` when needed.

---

## 9. Routing & Discoverability

- **Auto-delegation** in the main session, driven by the compiled descriptions above.
- **Team-scoped install paths** keep members namespaced and reduce cross-team collisions.
- **`ai/teams/INDEX.md`** (generated, committed): a table grouped by team (and a squads
  section), `(domain, file_globs, keywords) → team → member`, for two-level
  team→member selection.
- **`/team` slash command (Claude)** at `ai/claude/commands/team.md` and **Gemini
  `team.toml`** at `ai/gemini/commands/team.toml`: render the index and accept a team or
  squad argument (`/team go`, `/team fullstack-feature`) to surface a curated set.
- **Squads**: `teams.yaml` `squads:` entries pull curated cross-team members for
  multi-team tasks; honored by `/team`, `INDEX.md`, and the eval.
- **Navigation**: per-team `GEMINI.md` + `CLAUDE.md→GEMINI.md` symlinks, a root
  `ai/teams/GEMINI.md`, and a Repository-Structure row in the root `CLAUDE.md`.

---

## 10. Validation & Linting — `ai/teams/validate.sh` (zero-dep)

Pure `yq` + bash assertions, run **at install time (before emitting)** and **in CI**:

1. **Syntax**: `yq` parses every `the_*.md` frontmatter, `model-map.yaml`, `teams.yaml`
   (parse failure ⇒ error).
2. **Required keys** present on each agent (§6.1).
3. **Enums/refs**:
   - `tier` ∈ `model-map.yaml` tiers.
   - `team` == parent folder name AND ∈ `teams.yaml` teams.
   - `color` ∈ Claude's allowed set (`red,blue,green,yellow,purple,orange,pink,cyan`).
   - every `compose:` partial path exists.
   - `file_globs`, `keywords` non-empty arrays.
   - each squad member `<team>/<role>` resolves to a real agent.
   - `role` unique within a team.
4. **No dangling tiers/teams** (every tier used exists; every team folder has a
   `teams.yaml` row and ≥1 agent).

Companion `install_ai_teams_test.sh` (mirrors `sync-plugins_test.sh`) asserts:
- tier resolution (e.g. `deep-think` → Claude `opus`/`high`, Gemini `gemini-2.5-pro`/`0.2`),
- each emitter produces valid, parseable frontmatter/Modelfile,
- idempotency (re-run ⇒ no diff),
- graceful-skip when a tool dir / `ollama` is absent,
- `compose:` ordering + partial inlining.

---

## 11. Routing Eval Harness (`ai/teams/eval/`)

### 11.1 Dataset — `cases.yaml`
```yaml
cases:
  - task: "Add a Cobra subcommand with race-tested coverage"
    expect: { team: go, member: godev }
  - task: "Fix the login page Content-Security-Policy header"
    expect: { team: web, member: websec }
  - task: "Design a multi-account AWS landing zone"
    expect: { team: terraform-aws, member: cloudarch }
  # … ≥ ~40 cases spanning all teams/members, incl. ambiguous + squad cases
```

### 11.2 Runner — `route-eval.sh`
For each case, feed the **compiled descriptions + task** to the model in **headless mode**
and capture a single constrained pick (the agent id). Runs against **both** Claude and
Gemini headless. Determinism mitigations: pinned model IDs, fixed sampling, best-of-N
with majority vote.

### 11.3 Scoring & gate
- **Top-1 team accuracy** and **top-1 member accuracy** per runner.
- Prints a confusion matrix (rows = expected team, cols = predicted).
- **Blocking CI gate (D8): fails the build when team < 90% OR member < 90% on either
  runner.**

### 11.4 CI-gate risk & mitigations (explicit, per design discussion)
LLM routing is non-deterministic; a hard two-runner gate needs Anthropic + Google
credentials, adds per-PR cost, and can flake. Mitigations baked into the spec:
- Pinned model IDs + fixed sampling + best-of-N majority vote to reduce variance.
- Secrets via GitHub Actions OIDC/secrets; the job is skipped (not failed) when
  credentials are absent on forks, and required only on the protected merge path.
- Option to move the gate to `merge_group` / nightly if per-push cost or flake proves
  unacceptable — recorded as a follow-up toggle, not a redesign.

---

## 12. Self-Improvement Skill (Phase 3) — `teams-tune`

A shared skill at `src/teams-tune/skill/SKILL.md` (linked to both CLIs by `sync-skills`,
per the repo's shared-skill convention; modeled on `skill-creator`'s eval/variance
tooling):

1. Run `route-eval.sh`; read accuracy + confusion matrix.
2. For each misroute, propose concrete edits to the **source** frontmatter
   (`description`/`use_when`/`avoid_when`/`keywords`/`file_globs`) — never to generated
   output.
3. Optionally A/B test description variants.
4. Re-run `install_ai_teams.sh` + `route-eval.sh`; loop until team ≥90% AND member ≥90%.
5. Surface redundant/overlapping members or coverage gaps as suggestions.

Because edits target the source of truth, improvements persist across every regeneration.

---

## 13. Install Wiring & Aliases

- `install.sh`: add a block calling `install_ai_teams.sh` **after** `sync-skills` /
  `sync-plugins` (so `yq` and skill links already exist), before/around the gss build
  block. Guarded by `[ -f … ]` like the others.
- `validate.sh` runs first inside `install_ai_teams.sh`; a validation failure aborts the
  teams install only (warn + continue the overall bootstrap).
- New alias **`sync-teams`** (regenerate teams without a full install), added alongside
  `sync-skills` / `sync-plugins`.
- `opt/scripts/system/CLAUDE.md` (and `GEMINI.md` symlink) get a registry entry for
  `install_ai_teams.sh`.

---

## 14. Testing Strategy

| Test | Type | Asserts |
|------|------|---------|
| `validate.sh` | static | schema/enum/ref correctness of all source data |
| `install_ai_teams_test.sh` | unit | tier resolution, emitter output validity, idempotency, graceful-skip, compose ordering |
| `route-eval.sh` | integration/eval | ≥90% top-1 team & member, confusion matrix |
| CI workflow | gate | runs validate + unit tests on every PR; eval gate on protected path |

---

## 15. Risks & Open Items (verify before finalizing implementation)

1. **Antigravity agent path unconfirmed** — docs disagree (`~/.config/antigravity/agents/`
   vs `~/.gemini/antigravity-cli/plugins/`). **Action:** verify against the installed CLI;
   if unconfirmed, the Antigravity emitter skips (warns) and the agent is still recorded
   in the map/index. No build failure.
2. **Gemini agents recursion** — whether `~/.gemini/agents/` is scanned recursively is
   unconfirmed. **Action:** prefer nested `teams/<team>/`; fall back to flat
   `team-<team>-<role>.md` if only top-level is scanned. Emitter supports both via a flag
   detected at install time.
3. **CI eval cost/flakiness** — see §11.4.
4. **Ollama base-model availability** — Modelfiles always generated; `ollama create`
   opt-in; never auto-`pull`.
5. **`MODEL_PARAMS.md` duplication** — rewrite it to reference `model-map.yaml` so model
   slugs live in exactly one place.

---

## 16. Phasing (all implemented in one PR per D7, logically ordered)

- **Phase 1 — Foundation:** refactor `the_*.md` to frontmatter; add `model-map.yaml`,
  `teams.yaml`, `_partials/`, `validate.sh`; build `install_ai_teams.sh` (4 emitters);
  generate `INDEX.md` + `/team` + `team.toml`; add nav symlinks; wire `install.sh` +
  `sync-teams`; unit tests.
- **Phase 2 — Eval:** `eval/cases.yaml` + `route-eval.sh`; CI workflow with the blocking
  ≥90% two-runner gate.
- **Phase 3 — Self-improvement:** `src/teams-tune/skill/SKILL.md`.

---

## 17. File Manifest (created / modified)

**Created**
```
ai/teams/teams.yaml
ai/teams/model-map.yaml
ai/teams/validate.sh
ai/teams/GEMINI.md            (+ CLAUDE.md symlink)
ai/teams/_partials/{common-safety,repo-conventions,handoff-footer}.md
ai/teams/<team>/GEMINI.md     (+ CLAUDE.md symlink) × 5
ai/teams/eval/cases.yaml
ai/teams/eval/route-eval.sh
opt/scripts/system/install_ai_teams.sh
opt/scripts/system/install_ai_teams_test.sh
ai/claude/commands/team.md
ai/gemini/commands/team.toml
src/teams-tune/skill/SKILL.md
.github/workflows/teams-eval.yml   (or extend existing CI)
docs/superpowers/specs/2026-06-01-ai-teams-install-design.md  (this doc)
```

**Modified**
```
ai/teams/<team>/the_*.md        refactor comment-headers → YAML frontmatter (21 files)
ai/teams/MODEL_PARAMS.md        rewrite to reference model-map.yaml
ai/teams/README.md              note install + routing + squads
install.sh                      call install_ai_teams.sh
opt/scripts/system/CLAUDE.md    registry entry (+ GEMINI.md symlink already in sync)
ai/claude/aliases.sh / shared   sync-teams alias
CLAUDE.md (root)                Repository-Structure row for ai/teams install
```

**Generated at install (never committed)**
```
~/.claude/agents/teams/<team>/<role>.md
~/.gemini/agents/teams/<team>/<role>.md
~/.config/antigravity/agents/team-<team>-<role>.yaml
~/.config/ollama/teams/<team>/<role>.Modelfile
```
