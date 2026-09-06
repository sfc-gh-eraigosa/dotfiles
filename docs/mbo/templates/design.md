# <objective> — design

- **Slug:** <slug>
- **Date:** <YYYY-MM-DD>
- **Status:** Draft | Proposed | Approved | Superseded
- **Relates to:** issue #<n> / PR #<n>
- **Author(s):**

## 1. Problem / context
What's broken or wanted, grounded in verified facts (not assumptions).

## 2. Goals & non-goals

## 3. Options considered
2–3 approaches with trade-offs; lead with the recommendation and why.

## 4. Decision
The chosen shape; boundaries; what each unit does, how it's used, what it depends on.

## 5. Risks & blast radius

## 6. Rollback

## 7. Evidence expectations
How "it works" will be *shown*, not just asserted, during the build: name the proof
classes the eventual plan must capture (test-run captures, transcripts, demo
recordings, real-machine evidence) and any per-feature demo worth planning now. The
plan realizes this as `plans/<slug>/evidence/` folders — deciding the expectations at
design time is what makes the plan's validation section complete.

> Produced via `superpowers:brainstorming` (or an architecture-team `Workflow` for large work).
> Register the objective in `../index.md`. The matching spec goes in `../specs/<slug>.md`.
