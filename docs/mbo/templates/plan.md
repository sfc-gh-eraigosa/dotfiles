# <objective> — implementation plan

- **Slug:** <slug>
- **Date:** <YYYY-MM-DD>
- **Status:** Draft | Approved | In-progress | Done
- **Relates to:** spec `../specs/<slug>.md` · issue #<n> · PR #<n>

## 1. Summary & verdict
What this builds; any evaluation/review verdict and how must-fixes are addressed.

## 2. File inventory
Exact paths for every artifact + what each contains + which spec section it implements.
Include touch-points OUTSIDE the new dir (install.sh, scripts/test.sh, sync-skills, GEMINI.md, CI…).

| Path | Purpose | Implements |
| :-- | :-- | :-- |

## 3. Interface contracts
CLI/stdin-stdout, schemas, key function signatures, orchestration pseudocode.

## 4. TDD build order
Numbered phases, **tests first**, each with: what to write · how to verify · **done-when** gate.

## 5. Verification mapping
Each spec evaluation rule → its named test case (traceability).

## 6. Integration & rollout
Wiring (build/test discovery, docs, skills), and any manual acceptance checklist.

> Produced via `superpowers:writing-plans`. Execute with `superpowers:executing-plans` /
> `subagent-driven-development`, TDD throughout. Update `../index.md` state as it moves.
