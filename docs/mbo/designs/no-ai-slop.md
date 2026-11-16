# no-ai-slop — research evaluation

- **Slug:** `no-ai-slop`
- **Date:** 2026-10-05 (all maintenance numbers observed this date)
- **Status:** evaluated — reject-but-steal-the-pattern
- **Relates to:** issue [#367](https://github.com/sfc-gh-eraigosa/dotfiles/issues/367) (gss feature `skill-research`, worker `dossiers`)
- **Target:** <https://github.com/petergyang/no-ai-slop> (Peter Yang, "Behind the Craft" / creatoreconomy.so).
  Requested as `petegryang/no-ai-slop` (typo for `petergyang`). Evaluated against HEAD of `main` (last push
  2026-09-02, release v1.0.6).
- **Verdict:** **reject-but-steal-the-pattern** — build an adapted in-repo skill under MIT attribution; do
  not install upstream via `npx skills` or the paste-install prompt. Weighted score 82 / 110 (3.73 / 5).

Produced with the `research-evaluation` skill. Active rubric weighting (from `gff list research-rubric.*`):
security **critical**; value, adversarial, borrowable **high**; setup/licensing, stability, quality,
business **medium**; demo **low**. Gates: adversarial section required, docker-or-skip demo.

## 1. Problem / context

no-ai-slop "removes 20+ patterns of AI slop from any piece of writing". Identification: `petegryang`,
`peteryang` and `petegyang` all 404; `petergyang/no-ai-slop` matches (MIT, 11,932 stars). Collisions ruled
out: realrossmanngroup/no_ai_slop_writing_rules, danhlovejoy/no-slop, walterwritesai/no-slop-ai-humanizer-rewriter,
kartikkabadi/no-ai-slop, RocStone/roc-no-ai-slop-zh; manavmishra/ZeroSlop credits it as prior art.

What ships into the agent: `skills/no-ai-slop/SKILL.md` (10.8 KB), `eval.md` (3.2 KB self-check),
`agents/openai.yaml` (inert metadata) — pure Markdown + YAML, no executable code. `scripts/build_plugin.py`
and CI stay in the repo. This repo has a lot of agent-written prose (docs/mbo, PR bodies, commits,
AGENTS.md) and no prose-style skill today.

## 2. Nine-dimension dossier

### (a) Value — score 3/5

- Two modes: *Edit* (minimum-effective edit of a draft, voice preserved, plus "What changed") and *Detect*
  (name each pattern with quoted line + fix, no rewrite). ~17 principles, a 26-word "banned outright" list,
  ~19 named patterns (binary contrasts, throat-clearing, colon reveals, weasel attribution, recap endings,
  formatting slop, em dashes, ...) and the "portability test".
- The gap is real, but the shape is off: it edits a pasted draft; our main need is authoring-time
  constraints on agent output plus an optional review pass. The narrow description limits ambient firing.
- Overlap is low (`design:ux-copy`, `marketing:brand-review`, `internal-comms`, `doc-coauthoring`,
  `comment-analyzer` — none ships an anti-slop catalog). The conflict is with our own conventions.

### (b) Setup cost & licensing — score 5/5

- License: MIT (Copyright Peter Yang); adapting with attribution is fine.
- Offered install paths: paste an install prompt into your agent (unpinned), `npx skills add ... --yes`
  (third-party CLI, unpinned), or the ChatGPT/Codex curated plugin. For us: drop into `ai/skills/<name>/`
  and let `sync-skills` link it — ~5 minutes.
- Telemetry: none (PRIVACY.md: no server, no account). No `evals/evals.json` — we would author one.

### (c) Adversarial review — score 3/5

1. **Banned words misfire on technical prose.** The list includes `harness`, `realm`, `robust`, `leverage`,
   `elevate`, `facilitate`, `streamline`, `foster`, `beacon`; this repo uses "eval harness", "UAC
   elevation", "realm". Open PR #51 (2026-09-06, unmerged) documents exactly this.
2. **Interactive by design.** Asks for audience/goal in plain prose — stalls or guesses in headless runs and
   conflicts with our AskUserQuestion convention.
3. **Self-eval loop cost.** ~25-question eval.md check, fix, re-check on every edit.
4. **Clashes with house style.** "Formatting slop" and the em-dash rule collide with our bold-lead-in,
   dense-bullet, em-dash-heavy AGENTS.md / design-doc style.
5. **Doc/skill drift.** README advertises a removed "Generate slop for fun" mode (#49).
6. **Churn in the ambient layer.** Rules changed materially in Aug–Sep; unpinned installs shift behavior.
7. **Thin evidence.** Only #45 (ZeroSlop author, non-independent, 18 drafts): score 76.3 → 28.4, 17/18 key
   details kept, −13.7% length, 2nd of 4 tools. #42 "IT DIDN'T WORK" unanswered.
8. **Plugin ID collision** (#43) in the Codex catalog — irrelevant if vendored.
9. **Upsell surface** in the README only; no promo in the skill.

### (d) Security & safety — score 5/5 (critical)

- SKILL.md and eval.md read in full: editorial guidance only — no tool use, shell, URLs, file/secret reads,
  hooks, [MCP](../../../ai/skills/research-evaluation/references/glossary.md#mcp-model-context-protocol)
  or [prompt-injection](../../../ai/skills/research-evaluation/references/glossary.md#prompt-injection) text.
- `build_plugin.py` (not installed): stdlib only, writes under `dist/`, no network or subprocess.
- CI actions SHA-pinned, `contents: read` by default.
- Residual risk is [supply chain](../../../ai/skills/research-evaluation/references/glossary.md#software-supply-chain-surface):
  the recommended install paths fetch unpinned content with full agent privileges. Mitigation: vendor a
  reviewed copy at a pinned commit.

### (e) Stability — score 3/5

Created 2026-07-07; 23 commits; 7 releases v1.0.0–v1.0.6 in ~2 months; last push 2026-09-02. One Markdown
file, so the format is stable; the rule semantics are not. No changelog beyond auto-generated notes.

### (f) Quality & support — score 3/5 (observed 2026-10-05)

| Signal | Value |
| :-- | :-- |
| Stars / forks / watchers | 11,932 / 818 / 37 |
| Open issues+PRs | 22 |
| Contributors | petergyang 22 commits, tmchow 1 — [bus factor](../../../ai/skills/research-evaluation/references/glossary.md#bus-factor) 1 |
| Cadence | bursty: Jul 7 launch, Jul 22–Aug 6 burst, single commit Sep 2 |
| Responsiveness | ~15 community issues/PRs since Jul 29–Sep 9 with no maintainer reply (#24, #31, #36, #37, #38–#40, #42, #43, #45, #48, #50–#53) |

Writing quality is high (concrete before/after per pattern, voice preservation). Derivatives in Chinese,
Japanese, Korean and French show broad uptake. The ideas are the asset.

### (g) Demo — feasible, not executed — score 3/5

```bash
docker run --rm -it -e ANTHROPIC_API_KEY \
  -v "$PWD/vendored/no-ai-slop:/home/agent/.claude/skills/no-ai-slop:ro" \
  -v "$PWD/fixtures:/fixtures:ro" --network=<egress-to-api-only> node:22-slim \
  sh -c 'npm i -g @anthropic-ai/claude-code && for f in /fixtures/*.md; do
    claude -p "/no-ai-slop $(cat $f)" > /out/$(basename $f).with;
    claude -p "Edit for clarity: $(cat $f)" > /out/$(basename $f).base; done'
```

Fixtures: 3 PR bodies, 2 docs/mbo sections, 1 AGENTS.md section, 1 commit message, plus 2 drafts with
"test harness", "UAC elevation", "Kerberos realm", "robust regression" as canaries. Grade banned-pattern
count, term-of-art preservation, length delta, fact retention, and stalls under `-p`. Not run only because
it needs an API key and egress; not gating, since we are not adopting upstream as-is.

### (h) Borrowable features — build vs adopt — score 5/5

| Gap | Value | Build sketch | Worth it? |
| :-- | :-- | :-- | :-- |
| Named anti-slop pattern catalog | high | `ai/skills/prose-style/SKILL.md`: copy "Patterns to cut" with MIT header, keep portability test + examples | yes (~1 h) |
| Banned words without term-of-art misfires | high | keep ordinary-English bans; move harness/realm/robust/leverage/... to a conditional list (mirrors PR #51) | yes |
| Detect-only review of PR bodies / design docs | med | keep Detect mode; explicit-invocation trigger | yes |
| Authoring-time guardrail (ambient) | med | 6–10 line "Prose" list in root CLAUDE.md | yes (~10 min) — reconcile with house style first |
| Headless safety | med | headless → proceed with stated assumption; else AskUserQuestion | yes |
| `evals/evals.json` | required by `make skill-evals` | 8–10 cases: edit, detect, term-of-art canary, headless no-stall, non-trigger | yes |
| Deterministic banned-phrase linter | low-med | small script in `opt/scripts`, CI warn-only | maybe later |
| Edit-mode self-eval loop | low | drop; costly | no |
| ChatGPT/Codex packaging | none | n/a | no |

### (i) Business outcomes — score 2/5

- **Time saved:** small — ~5–15 min/week of rewriting agent-written PR bodies and design docs.
- **Cost savings:** none direct; the self-eval loop raises token spend; a CLAUDE.md list costs a few
  hundred tokens per session.
- **Revenue potential:** none; marginal reputational value of cleaner public docs.

## 3. Decision

Do not install upstream: unpinned ambient behavior, term-of-art misfires on our technical docs,
plain-prose questions that stall headless agents, and an unresponsive single maintainer. Steal the
pattern: build `ai/skills/prose-style` (name open) derived from SKILL.md under MIT attribution, with PR
#51's conditional word list, Detect mode for PR/design-doc review, headless-safe questioning and an
`evals/evals.json`. Security is clean, so this verdict is driven by fit, not risk.

## 4. Follow-ups (not in this PR)

- Build `ai/skills/prose-style` with the evals corpus from (h).
- Decide whether our em-dash/bold-heavy house style should change, then optionally add a short "Prose"
  list to root CLAUDE.md.
- Optionally run the (g) docker demo with term-of-art canaries before shipping.
- Maybe later: a warn-only banned-phrase linter for PR bodies and docs.
