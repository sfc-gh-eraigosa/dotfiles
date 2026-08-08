---
name: research-evaluation
description: >-
  Discover and evaluate external tools/projects before adopting them, producing a
  consistent research dossier and (when the repo has one) a docs/mbo research design
  doc + tracking issue per target. Use when the user wants to "research X", "evaluate
  X before we adopt it", "look up X and tell me if it's useful", "start a research
  MBO for X", or gives a list of candidate tools to investigate — even a single
  target. Runs the eight-dimension rubric: (a) value to us, (b) setup cost +
  licensing, (c) adversarial review, (d) security & safety, (e) stability,
  (f) quality & support, (g) a docker-sandboxed hands-on demo (docker-or-skip),
  (h) borrowable features (build-vs-adopt — could we build just the valuable
  bits ourselves instead of adopting the whole tool?).
  Fans multiple targets out to parallel research agents; NOT_FOUND is a valid,
  recorded outcome for targets that can't be identified through searches.
---

# research-evaluation — evaluate before adopting

Turn "should we use <tool>?" into a consistent, evidence-backed evaluation instead of
an ad-hoc impression. Works for one target or a batch; generalizes to any repo.

## The rubric (all eight, every target)

| Dim | Question |
| :-- | :-- |
| (a) **Value** | What is it worth *to us* — which of our real problems does it solve, and what do we already have that overlaps or conflicts? |
| (b) **Setup cost & licensing** | Install steps, prerequisites, pain points; the exact license, commercial tiers, telemetry/data terms. |
| (c) **Adversarial review** | The case AGAINST adopting: negatives, dangers, unknown pitfalls, failure modes, lock-in. Written to refute (a). |
| (d) **Security & safety** | Known and unknown gotchas: what it executes, what data it touches/stores/sends, supply-chain surface, CVEs/advisories. |
| (e) **Stability** | Likelihood it destabilizes our workflow or running services; maturity, breaking-change history, blast radius. |
| (f) **Quality & support** | Maintenance signals **with the observation date**: stars, contributors/bus factor, release cadence, issue responsiveness, docs, last commit. |
| (g) **Demo** | A workable sandboxed demo to validate first-hand: quickstart + real use case + success criteria. **Docker if possible; otherwise skip the demo entirely** — no unsandboxed demos. |
| (h) **Borrowable features (build-vs-adopt)** | For each valuable capability the tool has that our stack lacks, could we implement *just that feature* in our existing setup more simply than adopting the whole tool? Table it: gap → value → build-it-ourselves sketch → worth it? Ground the sketches in what we already run. This can flip the verdict to **reject-but-build-the-feature** — often the simpler, safer conclusion. |

## Procedure

### 1 — Pin the target list and the destination

- Collect the targets from the user (names may be fuzzy — "omni route", "headroom").
- Detect the repo's research home, in order: `docs/mbo/` (use its
  `templates/research.md` if present, else the design template) → any project-local
  MBO dir → fall back to `docs/research/` (create it). One **slug** per target.
- **Locate before create** at every step: existing design doc, existing issue
  (`gh issue list --search "<slug> in:title"`), existing index row. Re-runs must
  reconcile, not duplicate.

### 2 — Research (fan out for >1 target)

For each target, run a web-enabled research agent (parallel, background, one per
target) that must:

1. **Identify** the canonical project: GitHub first, then docs/blogs. Try name
   variants; rule out name collisions explicitly. If no confident identification
   after honest searching → verdict **NOT_FOUND**, listing the searches tried and
   closest candidates — this is a valid terminal outcome, not a failure.
2. Gather evidence for all eight dimensions. Ground (f) in the repo API (stars,
   contributors, last push, open issues) and **state the observation date**. Mine
   the issue tracker for (c)/(d)/(e) — open bugs are the adversarial goldmine. For
   (h), diff the tool's capabilities against what we already run and ask which
   valuable gaps are cheaply buildable in-house.
3. Draft the (g) demo as concrete commands (compose file / docker run), sandboxed:
   throwaway dirs, localhost-only ports, never real credentials or OAuth tokens,
   full teardown. If docker genuinely can't work, write "no docker demo — skip".
4. Write the full dossier to a scratch file; return only a ≤15-line executive
   summary (verdict, URL, license, one-line value, top risk, demo feasible).

Context the agents need to judge (a): a short paragraph on our environment and
what we already run (memory systems, orchestration, infra) — pass it in the prompt.

### 3 — Produce the artifacts

Per FOUND target: condense the dossier into the research design doc
(`designs/<slug>.md` from the research template) ending in a **Verdict**:
`adopt / adopt selectively / park (gated on demo) / reject / reject-but-steal-the-pattern
/ reject-but-build-the-feature` — one paragraph, grounded in the sections (including the
(h) build-vs-adopt call). Per NOT_FOUND target: a stub doc recording the searches +
candidates, state `not-found`.

### 4 — Track

- Create/update one issue per target (title `<slug>: research evaluation — <name>`,
  body = one-liner + design-doc path + verdict) and put the number in the doc.
- Register every target in the index (`docs/mbo/index.md` or the fallback's
  README): slug, doc link, issue, state (`evaluated — adopt/reject`, `parked`,
  `not-found`).
- Land the docs via the repo's normal review flow (here: a **gss draft PR** —
  confirm before push per gss rules).

### 5 — Demos (optional follow-up, on request or per verdict)

Run the (g) plans for targets whose verdict is gated on the demo (`park`) or where
first-hand validation was requested. Capture evidence per the repo's show-and-tell
convention (e.g. `demos/<date>-<slug>/`). Never run an installer on a real host
before its container run.

## Hard rules

- **Docker-or-skip** for demos — no "just try it on the host".
- **Adversarial section is mandatory** — an evaluation with no case-against is
  marketing, not research.
- **Always run the (h) build-vs-adopt check** — never conclude "adopt" without first
  asking whether the valuable features could be built into our own stack more simply.
- **Dated observations** — every quality/maintenance number carries the date.
- **NOT_FOUND stops that target** — record it and move on; don't force a match
  onto a name-collision.
- **Never route real subscription/OAuth credentials into a tool under evaluation.**
