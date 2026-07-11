# <objective> — implementation plan

- **Slug:** <slug>
- **Date:** <YYYY-MM-DD>
- **Status:** Draft | Approved | In-progress | Done
- **Relates to:** spec `../specs/<slug>.md` · issue #<n> · PR #<n>

## 1. Summary & verdict
What this builds; any evaluation/review verdict and how must-fixes are addressed.

## 2. File inventory
Exact paths for every artifact + what each contains + which spec section it implements.
Include touch-points OUTSIDE the new dir (install.sh, scripts/test.sh, sync-skills, AGENTS.md, CI…).

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

### 6.1 Build leaves / DAG (fill in only if the build will be broken out — `mbo-plan` CAP-B)
The **authoritative** dependency graph for parallel execution. A leaf owns a disjoint set of
paths and depends on others only through a *frozen interface* declared in §3. Edge `A → B` =
"B depends on A's interface" (must be a DAG; a cycle means re-cut the split). **Blocking** leaves
(whose interface others import) are built first (Interface-First).

| Leaf | Owns (paths) | Consumes (in-edges) | `done-when` gate | Blocking? |
| :-- | :-- | :-- | :-- | :-- |
| <iface> | <paths> | — | <gate> | yes (base) |
| <impl>  | <paths> | <iface> | <gate> | no |

This table is mirrored (not re-authored) into the design-issue body (as a small mermaid/ASCII
DAG) and realized as `gss feature` workers (`--base` = the in-edge). See the `mbo-plan` skill.

> Produced via `superpowers:writing-plans`. Execute with `superpowers:executing-plans` /
> `subagent-driven-development`, TDD throughout. Update `../index.md` state as it moves.
