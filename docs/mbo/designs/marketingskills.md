# marketingskills — research evaluation

- **Slug:** `marketingskills`
- **Date:** 2026-10-05 (all maintenance numbers observed this date)
- **Status:** evaluated — adopt selectively
- **Relates to:** issue [#366](https://github.com/sfc-gh-eraigosa/dotfiles/issues/366) (gss feature `skill-research`, worker `dossiers`)
- **Target:** <https://github.com/coreyhaines31/marketingskills> (Corey Haines, corey.co / Conversion Factory
  agency). Only repo by that name; no collisions. Evaluated against HEAD `dda3841f` (2026-10-02), plugin v2.11.17.
- **Verdict:** **adopt selectively** — cherry-pick and vendor ~5 skills pinned to a SHA; do not install the
  plugin and do not copy `tools/clis`. Weighted score 75 / 110 (3.41 / 5).

Produced with the `research-evaluation` skill. Active rubric weighting (from `gff list research-rubric.*`):
security **critical**; value, adversarial, borrowable **high**; setup/licensing, stability, quality,
business **medium**; demo **low**. Gates: adversarial section required, docker-or-skip demo.

## 1. Problem / context

marketingskills is a pack of 50 Markdown skills (SKILL.md + `references/` + `evals/evals.json`) for
SaaS/founder marketing: CRO, copywriting, SEO audit, AI-SEO/GEO, schema, pricing, launch, free-tools
(engineering-as-marketing), directory submissions, ads, churn, revops and more. 48 of the 50 read a shared
product-marketing context file (`.agents/product-marketing.md`) first. The user is a solo developer with
side projects, not a growth team; the question is which, if any, are worth their context cost.

## 2. Nine-dimension dossier

### (a) Value — score 3/5

- Content is dense, practical and opinionated, richer than the generic Anthropic marketing plugin skills.
  free-tools, launch, directory-submissions, ai-seo, pricing and schema fit a developer-founder well.
- Overlap: `marketing:seo-audit` vs `seo-audit` is a direct name collision; also `marketing:email-sequence`,
  `content-creation`/`draft-content`, `competitive-brief`, `campaign-plan`, `performance-report`, and
  `design:ux-copy`. About 12 of 50 overlap heavily, and every description triggers aggressively.
- Context cost: 50 descriptions ≈ 38 KB ≈ 9.5K always-on tokens per session (bodies 720 KB, load on
  trigger). Too heavy for an always-on load in a mostly Go/shell repo.

### (b) Setup cost & licensing — score 4/5

- License: MIT ("Copyright (c) 2025 Corey Haines") — vendoring with attribution is fine.
- Install paths: `/plugin marketplace add` + `/plugin install marketing-skills` (no hooks, MCP or
  commands), `npx skills`, `npx skillkit`, or a Codex plugin — minutes either way.
- Telemetry: none in skills or manifest. README carries `?ref=marketingskills` self-referral links only.
- `evals/evals.json` is a superset of our skill-creator schema, so `make skill-evals` should accept vendored
  copies. Our path: vendor into `ai/skills/<name>/`, `sync-skills` links it; needs `.gitignore` `!`-rules.

### (c) Adversarial review — score 2/5

1. **Paid-placement drift.** A "Verified Partners" program (Converly, Ploy); Introw is promoted in 5 SKILL.md
   bodies (launch, referrals, sales-enablement, revops, co-marketing). Owner issue #653 (2026-10-02) plans
   to add Ploy into site-architecture, programmatic-seo, ai-seo and launch. An auto-updating install would
   pull new placements in silently.
2. **Release churn.** 61 releases since Feb 2026, 16 in Oct 1–3 alone; v2.0 renamed 17 skills.
3. **Trigger pollution.** 50 aggressive descriptions, 12 collisions with the installed marketing plugin;
   "audit this page" could fire cro/seo-audit inside dotfiles work.
4. **[Bus factor](../../../ai/skills/research-evaluation/references/glossary.md#bus-factor) of 1.** 542 of
   ~680 commits by the owner (next: 29); one person's content plus a commercial funnel.
5. **Generic-LLM substitutability.** Much is well-known canon (AIDA, Cialdini, Hormozi); marginal lift
   needs a with-skill vs baseline measurement first.
6. **Wrong audience for half the pack.** revops, sales-enablement, sms, influencer, events, attribution are
   dead weight for a solo developer.

### (d) Security & safety — score 3.5/5 (critical)

- **Install-time execution:** none — no hooks, MCP servers, postinstall or curl|bash. The `npx skills` /
  `npx skillkit` paths run third-party npm code; avoid them.
- **Executable code, outside `skills/`:** `tools/clis/*.js` — 78 zero-dependency Node CLIs, one per vendor
  API, reading 93 distinct `*_API_KEY`-style env vars and calling `fetch()`; no child_process/exec/eval.
  Skills reference them (e.g. `node tools/clis/github-prospects.js stargazers ... --enrich`). An agent with
  our env could hit paid APIs or send email. **Do not vendor `tools/`.**
- **Commands in references:** curl recipes against Reddit/HN Algolia/Bluesky/Google News RSS,
  `npx hyperframes`, `npx create-video@latest`, `npx playwright install chromium --with-deps`,
  `npm install -g agent-browser` — benign intent, third-party npx on demand.
- **[Prompt injection](../../../ai/skills/research-evaluation/references/glossary.md#prompt-injection):**
  none hostile; `audit-guardrails.md` explicitly treats fetched pages as data.
- **Promotional content** inside SKILL.md bodies — disclosed in REGISTRY.md, not in the skills.
- **Repo automation:** "Coreybot" auto-commits with `contents:write`; actions pinned by tag, not SHA.
- Net: safe to vendor selected SKILL.md + references after a diff read; unsafe as a floating plugin.

### (e) Stability — score 2/5

Created 2026-01-15; 61 releases (Feb 3, Mar 3, Apr 4, May 6, Jun 5, Jul 18, Aug 5, Sep 1, Oct 1–3: 16);
VERSIONS.md is 104 KB. Breaking renames in v2.0 (page-cro + form-cro folded into cro). Content is stable;
the surface area is not.

### (f) Quality & support — score 4/5 (observed 2026-10-05)

| Signal | Value |
| :-- | :-- |
| Stars / forks | 53,386 / 7,939 |
| Last push | 2026-10-03 |
| Open issues+PRs | 131 (~110 PRs, ~21 issues; only 1 open issue from a non-owner) |
| Contributors | 45; top 542 commits, next 29 — bus factor 1 |
| Engineering | per-skill evals.json (5–25 KB), CI: validate-skill, regression tests, check-versions |

Issue themes: eval benchmarks (#479, #480, #484, #497), new skills, distribution (#499, #703), partner
integration (#653). PR review backlog is real.

### (g) Demo — skipped (plan recorded) — score 3/5

Not run: pure-Markdown content, risk is from content not code. Plan: `docker run --rm -it --network none`
(egress only to the model API) with node + Claude Code; copy in only `skills/{free-tools,launch,pricing,copywriting}`
and a fake `.agents/product-marketing.md`; run the skill-creator with-skill vs baseline loop on each
skill's evals.json, with the Anthropic marketing plugin present to check trigger collisions. No `$HOME`
mount, no API-key env, no `tools/`. Recommended as the gate before vendoring more than 5 skills.

### (h) Borrowable features — build vs adopt — score 5/5

| Gap | Value | Build sketch | Worth it? |
| :-- | :-- | :-- | :-- |
| Product-marketing context file read by every marketing skill | high | vendor `product-marketing`; our skills read `.agents/product-marketing.md` | yes, steal the pattern |
| Engineering-as-marketing | high | vendor `free-tools` (7 KB) | yes |
| Launch playbook (PH/HN/directories) | high | vendor `launch` (15 KB) + optional `directory-submissions` (26 KB, strip partner/npx lines) | yes |
| Pricing/packaging | med-high | vendor `pricing` (13 KB) (+ `paywalls` 6 KB) | yes |
| AI-search (GEO/AEO) visibility | med-high | vendor `ai-seo` (31 KB) + `schema` (5 KB) | yes, SHA pin |
| Copywriting / copy-editing | med (overlaps) | vendor or rely on existing plugin | optional, pick one source |
| SEO audit | low (collides) | none | no |
| Email/cold-email/SMS/ads/revops/etc. | low | none | no |
| `tools/clis` vendor API wrappers | low (we have MCP connectors) | none | no (security cost) |
| Marketing council persona-sim | low | 1-page prompt | maybe later |
| Per-skill evals.json | pattern already ours | n/a | confirms our approach |

Cherry-pick set (~5–6 skills, ~1.5K description tokens): product-marketing, free-tools, launch, pricing,
ai-seo (+ schema). Rename on collision (e.g. `mkt-` prefix), tighten descriptions, remove Introw/partner
rows and npx/npm -g lines, keep the MIT notice, record the source SHA per folder.

### (i) Business outcomes — score 4/5

- **Time saved:** a few hours per side-project launch from the launch/pricing/landing-copy checklists.
- **Cost savings:** a free alternative to courses or an agency for early pricing/launch reviews.
- **Revenue potential:** the main reason to adopt — free-tools, launch, directory-submissions, pricing and
  ai-seo directly support turning side-project CLIs into traffic and paid conversions, if the user ships.

## 3. Decision

Adopt selectively. Do not add the pack to `ai/plugins.yaml`: a floating plugin brings daily churn,
paid-placement drift, ~9.5K always-on tokens and 12 trigger collisions. Instead vendor product-marketing,
free-tools, launch, pricing and ai-seo (+ schema) into `ai/skills/` at pinned SHA `dda3841f`, each with a
provenance note, partner rows and npx lines stripped, and descriptions narrowed; run `make skill-evals`,
then the skill-creator with-skill vs baseline loop in the docker sandbox from (g) before expanding the set.

## 4. Follow-ups (not in this PR)

- Vendor the 5–6 skill cherry-pick set with `.gitignore` `!`-rules and per-folder SHA provenance.
- Run the sandboxed with-skill vs baseline eval, including collision checks against `marketing:*`.
- Re-check issue #653 (Ploy into core skills) before any refresh of ai-seo/launch.
- Consider the shared product-context-file pattern for our own skill families.
