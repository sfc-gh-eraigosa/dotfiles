# Progressive Disclosure for Agent Configuration

- **Date**: 2026-08-07
- **Status**: Proposed (first tranche implemented in the same PR as this doc)
- **Scope**: root `AGENTS.md`, per-directory `AGENTS.md` tree, `ai/skills/`, skill frontmatter

## What progressive disclosure means

Keep the always-loaded agent context minimal and push detail into files loaded on demand.
Anthropic's Agent Skills model formalizes it as three levels: (1) frontmatter `name` +
`description` (~100 tokens, preloaded for every skill), (2) the SKILL.md body (read only when the
skill triggers, target < 500 lines), (3) bundled `references/` files (read only when the task
needs them). The same shape applies to repo docs: a thin root `AGENTS.md` that links down, with
per-directory `AGENTS.md` files discovered on demand. Root-file litmus test (Anthropic best
practices): *"Would removing this line cause Claude to make mistakes?"* — if not, it doesn't
belong in always-loaded context.

Sources: [Agent Skills engineering post](https://www.anthropic.com/engineering/equipping-agents-for-the-real-world-with-agent-skills),
[Effective context engineering](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents),
[skill authoring best practices](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices),
[Claude Code best practices](https://code.claude.com/docs/en/best-practices).

## Where this repo stands (2026-08-07 audit)

The architecture is already right: 33/33 `CLAUDE.md` files are symlinks to `AGENTS.md`, the root
documents a "Progressive Loading" guideline, skills load on demand via `sync-skills`, and leaf
`AGENTS.md` files are appropriately small (7–35 lines). The leak: the root file accreted
operational detail and reached **~4,730 tokens loaded every session** — the largest file in the
tree, an inversion of the ideal thin-root shape.

## Refactor plan (ranked by per-session token savings)

| # | Refactor | ~Tokens saved | Status |
|---|---|---|---|
| 1 | Extract the `.gitignore` allowlist walkthrough to [docs/gitignore-allowlist.md](../../gitignore-allowlist.md); keep a 3-line rule + link | ~600 | **this PR** |
| 2 | Deduplicate portability sub-bullets that restate the linked [shell-portability spec](../specs/shell-portability.md) (§2 rules, §5 checklist); keep the MUST-follow + enforcing-gate sentences | ~400 | **this PR** |
| 3 | Trim gss push mechanics (two-call token recipe, new-branch `--set-upstream` story) to a pointer — the full text already lives in [sdk/gss/skill/SKILL.md](../../../sdk/gss/skill/SKILL.md) §3, loaded exactly when pushing | ~375 | **this PR** |
| 4 | Extract the `install.sh` Windows/WSL interactivity flow to [docs/install-windows.md](../../install-windows.md); trim the AI-config provisioning bullet to its core rule + design-doc link | ~400 | **this PR** |
| 5 | Tighten Repository Structure annotations (gff, teams, plugins, Desktop bullets restate their sub-AGENTS.md) | ~250–300 | follow-up |
| 6 | Compress the longest skill frontmatter descriptions (`mbo-plan` ≈ 275 tokens; also `remote-claude-session`, `wispr-flow-debug`, `git-machete`, `teams-tune`) — descriptions are always-loaded in the skills list | ~400–500 | follow-up |
| 7 | Introduce the `references/` three-level pattern (currently unused repo-wide): split `ai/skills/mbo-plan/SKILL.md` (290 lines) routing tables and `sdk/gss/skill/SKILL.md` (165 lines) edge-case narratives into on-demand reference files | body-size, not per-session | follow-up |

Items 1–4 shrink the root from ~4,730 to roughly **~2,800 tokens (−40%)** with zero information
loss — every extracted paragraph is preserved verbatim at its new location and linked from the
rule that remains.

## Tooling adopted

All three license-vetted skills that maintain this discipline are **already declared** in
[ai/plugins.yaml](../../../ai/plugins.yaml):

| Skill | Source | License | Role |
|---|---|---|---|
| `claude-md-management` (`claude-md-improver`, `/revise-claude-md`) | anthropics/claude-plugins-official | Apache-2.0 | Audits CLAUDE.md files against quality templates; the ongoing thin-root enforcement loop |
| `skill-creator` | anthropics/skills via claude-plugins-official | Apache-2.0 | Scaffolds new skills with the three-level `references/` + `scripts/` layout (item 7) |
| `superpowers:writing-skills` | obra/superpowers | MIT | Methodology guardrails: lean SKILL.md, split-out heavy references, frontmatter discipline |

Suggested cadence: run `claude-md-management:claude-md-improver` after any PR that grows the root
`AGENTS.md`, and use `skill-creator` for item 7.

## Evaluation checklist (for future audits)

1. Root `AGENTS.md` stays under ~200 lines / ~2,500–3,000 tokens; every line passes the deletion test.
2. Root is an index — detail lives in linked sub-docs with a 1–3 sentence abstract per link.
3. Every documented directory keeps its `AGENTS.md` + `CLAUDE.md` symlink pair.
4. Sometimes-relevant workflows are skills, not root content.
5. Skill descriptions are trigger-quality and ≤ ~2–3 sentences where possible.
6. SKILL.md bodies stay < 500 lines; overflow splits into `references/` linked one level deep.
7. No rule stated in two layers (root + spec/skill) — the detailed copy wins, root keeps a pointer.
