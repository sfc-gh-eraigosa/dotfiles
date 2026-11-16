# eli5 — research evaluation

- **Slug:** `eli5`
- **Date:** 2026-10-05 (all maintenance numbers observed this date)
- **Status:** evaluated — reject-but-build-the-feature
- **Relates to:** issue [#368](https://github.com/sfc-gh-eraigosa/dotfiles/issues/368) (gss feature `skill-research`, worker `dossiers`)
- **Target:** requested as "eli5" with no owner given. Chosen candidate: <https://github.com/DreambigOu/ELI5>
  (Andrew Ou); runner-up: the `eli5/` skill in <https://github.com/anthropics/claude-plugins-community>.
- **Verdict:** **reject-but-build-the-feature** — steal the audience matrix and eval cases; write our own
  ~30-line `ai/skills/eli5/SKILL.md` + `evals/evals.json`. Weighted score 71 / 110 (3.2 / 5).

Produced with the `research-evaluation` skill. Active rubric weighting (from `gff list research-rubric.*`):
security **critical**; value, adversarial, borrowable **high**; setup/licensing, stability, quality,
business **medium**; demo **low**. Gates: adversarial section required, docker-or-skip demo.

## 1. Problem / context

An "Explain Like I'm 5" skill tailors explanations to an audience. With no owner given, identification
came first: `gh search repos` / `gh search code` for eli5 variants returned 40+ SKILL.md hits, mostly
personal dotfiles copies and registry scrapes. Ruled out: TeamHG-Memex/eli5 and eli5-org/eli5 (ML
explainability library), facebookresearch/ELI5 (QA dataset), swapagarwal/awesome-eli5 (reading list).

| Rank | Candidate | Stars / signal | License | Shape |
| :-- | :-- | :-- | :-- | :-- |
| 1 (chosen) | DreambigOu/ELI5 (Andrew Ou) | 1,658★, 93 forks; blog post | MIT (holder blank) | 1 SKILL.md (8 KB), audience-tailored text, 3 evals + A/B runner |
| 2 | anthropics/claude-plugins-community `eli5/` (Thariq Shihipar, 2026-08-21) | official community marketplace (repo 4.5k★) | MIT (manifest) | 5-line SKILL.md: `/eli5 <topic>` → HTML artifact |
| 3 | isas1/skills (`eli5-succinct`) | 212★ | MIT | prompt-only family |
| 4 | yizhiyanhua-ai/fireworks-open-eli5 | 189★ | Apache-2.0 | visual explainers; 8 scripts (higher risk) |
| 5 | qqyumidi/eli5-plus, sscodeai/eli5, kidskoding/explain-like-im-five | ≤25★ | MIT/various | HTML-explainer variants |
| adj. | alexgreensh/attention-span | 1,304★ | AGPL-3.0 | output styles, not eli5; scripts/hooks |

#1 has the most adoption of any standalone `eli5` skill and is the usual blog reference; #2 is the most
official channel but only a few lines of prompt and a different job (picture artifact, not audience-tuned text).

## 2. Nine-dimension dossier

### (a) Value — score 2/5

- Adds a table of 20+ audiences (ages, grades, roles, relationships), a 4-step structure (what → analogy →
  detail → so-what), per-audience tone/jargon rules, and an "Age 5" default.
- Author's A/B: 83.3% vs 41.6% assertion pass rate — n=3, one run each, Claude grading author-written
  assertions. Weak evidence; most of the gain is the "manager" framing (0% → 50%).
- Overlap: Anthropic `learn` and `doc-coauthoring`, plain one-sentence instructions, the research glossary
  links, and the Artifact tool (all of #2's value). No existing eli5 skill, so no direct collision.
- Real gain is small: a stable trigger that behaves the same in Claude Code and Antigravity via `sync-skills`.

### (b) Setup cost & licensing — score 4/5

- Install: `cp -r skills/eli5` — no manifest, no dependencies; for us a drop into `ai/skills/eli5/`.
- License: MIT, but LICENSE reads "Copyright (c) 2026" with no holder (issue #1, open since 2026-09-03, no
  reply). Usable, attribution ambiguous. #2 declares MIT only in plugin.json; its LICENSE file was removed
  on 2026-08-21 inside an Apache-2.0 repo.
- Telemetry: none. The eval runner calls the user's own `claude` CLI, only when run by hand.

### (c) Adversarial review — score 2/5

1. **Triggers too broadly.** "Even partial matches like 'explain to my wife' or 'tell my boss' should
   trigger" — "draft a message to tell my boss the deploy slipped" would become an explainer.
2. **Lets accuracy slide.** "80% accuracy is better than 100% accurate" — technical simplifications should
   be flagged, not silently wrong.
3. **Stereotyped audience tables** (wife/husband → "household tasks") undercut its own "never talk down" rule.
4. **Weak eval evidence;** 1.6k stars with no commits since 2026-03-18 suggests blog-driven popularity.
5. **Fixed "Age 5" default** even for engineers; "smart non-specialist" is more useful here.
6. **Candidate #2** depends on harness artifact support; degrades headless or in Antigravity; no evals.

Steelman: harmless, cheap, better than nothing for audience framing; the 4-step structure is good practice.

### (d) Security & safety — score 5/5 (critical)

- `skills/eli5/SKILL.md` is pure markdown: no scripts, hooks, `allowed-tools`, shell, URLs or fetches; no
  [prompt-injection](../../../ai/skills/research-evaluation/references/glossary.md#prompt-injection),
  exfiltration or persistence wording. Only tool-adjacent line: "Read the relevant code files" (read-only).
- `eli5-workspace/run-evals.py` (dev-only, not installed): stdlib imports; only subprocess calls are
  `claude -p <prompt> --output-format text` and `which claude`; no `shell=True`, no permission bypass.
- #2's 5-line SKILL.md read in full: clean.
- [Supply chain](../../../ai/skills/research-evaluation/references/glossary.md#software-supply-chain-surface):
  single-maintainer personal repo; vendoring removes the future-malicious-edit risk. Only
  fireworks-open-eli5 and attention-span carry executable code; neither was chosen.

### (e) Stability — score 4/5

A single static file. Last commit 2026-03-18 (10 commits over 2 days); matches current SKILL.md
frontmatter; not archived. The risk is abandonment, not churn.

### (f) Quality & support — score 2/5 (observed 2026-10-05)

- 2026-03-17 → 03-18: all 10 commits, one author — [bus factor](../../../ai/skills/research-evaluation/references/glossary.md#bus-factor) 1.
- 2026-09-03: issue #1 (LICENSE names no holder) opened; unanswered on 2026-10-05.
- 2026-10-05: 1,658★, 93 forks, 0 merged PRs, 1 open issue — effectively unmaintained.
- Writing quality is good (clear structure, worked examples, skill-creator-style evals).
- #2: 5 commits on 2026-08-21 by an Anthropic employee — better provenance, almost no content.

### (g) Demo — skipped — score 3/5 (neutral)

Pure prompt: no binaries, network or install steps to sandbox, so docker adds nothing. A meaningful demo is
the behavioral A/B (`claude -p` with/without the skill), i.e. the skill-creator eval loop — run it later
against the version we build.

### (h) Borrowable features — build vs adopt — score 4/5

| Gap / feature | Value | Build sketch | Worth it? |
| :-- | :-- | :-- | :-- |
| Audience matrix (age/grade/role → tone, analogy, depth) | med | trimmed table (roles + 3 age bands, drop gendered rows), MIT attribution | yes |
| 4-step structure | med | 4 lines in SKILL.md | yes |
| Narrow trigger | high (avoids hijacking) | explicit "ELI5 / explain like I'm N / explain to <audience>" only; never "tell my boss" | yes, key fix |
| Accuracy guard | high | "simplify, never falsify"; one-line "what I glossed over" for technical audiences | yes |
| Default audience | med | smart non-specialist adult unless "5" is said | yes |
| HTML picture mode (#2) | low-med | on "visual"/"picture", emit an artifact; degrade to text headless | optional |
| Eval corpus | high (repo rule) | reuse the 3 cases + 2 negative-trigger cases | yes |
| A/B runner (`run-evals.py`) | low | covered by skill-creator | no |

Build cost: ~30–40 lines of SKILL.md + a 5-case evals.json, under 1 hour — about the same as forking and
editing, and fully ours.

### (i) Business outcomes — score 2/5

- **Time saved:** small — a handful of uses a month explaining infra/agent concepts to non-technical people.
- **Team/communication:** could standardize "explain this PR/incident to a manager"; overlaps doc-coauthoring.
- **Revenue / strategic:** none. Hidden cost of adopting as-is is false triggers polluting sessions; ROI is
  positive only if built narrowly.

## 3. Decision

Reject both DreambigOu/ELI5 and the marketplace `eli5` plugin, and build the feature: author
`ai/skills/eli5/SKILL.md` ourselves, borrowing the audience matrix and 4-step structure (credit Andrew Ou,
MIT), with a narrow trigger, a "simplify, never falsify" guard, a smart-non-specialist default, and an
optional HTML visual mode in the style of #2. Ship `evals/evals.json` with positive and negative trigger
cases so `make skill-evals` passes. Both candidates are clean on security; this verdict is driven by value
and quality, not risk.

## 4. Follow-ups (not in this PR)

- Build `ai/skills/eli5/` (SKILL.md + 5-case `evals/evals.json`, incl. "tell my boss the deploy slipped" as
  a must-not-trigger case), with `.gitignore` `!`-rules.
- Run the skill-creator with-skill vs baseline loop against our version.
- Optional HTML-artifact visual mode, text fallback for headless/Antigravity.
