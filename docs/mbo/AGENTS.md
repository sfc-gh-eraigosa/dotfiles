# MBO — Management By Objective (design · spec · plan)

This folder is the **single home for all objective-driven design work**. Every PR-sized
objective flows through the same repeatable pipeline and leaves a consistent trail of
artifacts here, tracked in [`index.md`](./index.md).

> **AI agents: read this before starting any design / spec / plan / "let's build X" work.**
> It tells you which skill workflow to run, where the artifacts go, and how to track them.

## The pipeline (every objective, same shape)

```
issue  ──►  classify  ──►  design  ──►  spec  ──►  plan  ──►  build (gss worktree)  ──►  PR
(or gss          (pick a       (mbo/      (mbo/      (mbo/      (TDD)                       land
 draft PR)        skill flow)   designs/)  specs/)    plans/)                                + update index.md
```

1. **Capture the objective** — from a GitHub **issue** or a **`gss feature` draft PR** you're
   actively working in. One objective = one slug (e.g. `prping`, `sdk-migration`).
2. **Classify the task** and run the matching **skill workflow** (table below).
3. **Design** (`mbo/designs/<slug>.md`) — for novel/architectural work; the "why & shape".
   Skip for trivial objectives.
4. **Spec** (`mbo/specs/<slug>.md`) — validated requirements + acceptance/eval criteria
   (produced by `superpowers:brainstorming`).
5. **Plan** (`mbo/plans/<slug>.md`) — the TDD-ordered implementation plan
   (produced by `superpowers:writing-plans`).
6. **Register** the objective in [`index.md`](./index.md) (slug, artifact links, issue(s),
   PR(s), state).
7. **Build** on a `gss feature` worktree (draft PR), TDD, then land via `gss`. Update the
   objective's **state** in `index.md` as it moves.

Not every objective needs all three artifacts — small features may be spec→plan only; a pure
ADR may be design-only. **Always** register it in `index.md`.

## Task type → skill workflow (pick the right one)

| If the objective is… | Run this workflow | Artifacts |
| :-- | :-- | :-- |
| **A new skill** (or editing one) | `skill-creator:skill-creator` → `superpowers:writing-skills` | spec (optional) + the skill + its `evals/evals.json` (validate with `make skill-evals`; see [`ai/skills/AGENTS.md`](../../ai/skills/AGENTS.md)) |
| **A dotfiles feature** (shell, `opt/`, `ai/`, profiles, install) | `superpowers:brainstorming` → `superpowers:writing-plans` → `superpowers:test-driven-development`; obey the root `CLAUDE.md` conventions | spec + plan |
| **A Go CLI under `sdk/`** | `brainstorming` → `superpowers:writing-plans` → `tdd`; mirror `sdk/gss` (cobra `cmd/`, `internal/` + mockable runner, `internal/version` ldflags); engage **go-team** agents (`go-goarch` for interfaces) | design + spec + plan |
| **A Go RPC / gRPC service** | `go-goarch` (proto/interface + boundaries) → `writing-plans` → `go-godev` / `go-goqa` | design + spec + plan |
| **A UI / web component** | `brainstorming` (use its visual companion) → **web-team** (`web-fe`, `web-webarch`) → `writing-plans` | design + spec + plan |
| **AWS infra / serverless** | `deploy-on-aws:*` / `aws-core:*` / **terraform-aws** team | design + plan |
| **A large audit / migration / multi-subsystem change** | a **`Workflow`** fanned out to the **architecture team** (sysarch/principal/secarch/adversary) → `writing-plans` | design + plan |
| **CLAUDE.md / docs upkeep** | `claude-md-management:revise-claude-md` / `claude-md-improver` | — |

> The new **`mbo-plan`** skill (built with `skill-creator`) automates steps 1–6 for the common
> case: it starts from an issue or a `gss` draft PR, classifies the task, runs the right
> workflow, lays the artifacts down here with the right names, and updates `index.md`.

## Conventions

- **Slug = the join key.** An objective's design / spec / plan share one slug:
  `mbo/designs/<slug>.md`, `mbo/specs/<slug>.md`, `mbo/plans/<slug>.md`. The folder encodes the
  type; the filename is the objective. Dates live *inside* the doc + in `index.md`, not in the
  filename (legacy date-prefixed files are kept as-is; new work uses the bare slug).
- **Templates** live in [`templates/`](./templates) — copy `design.md` / `spec.md` / `plan.md`
  to start, so every artifact has the same headers (status, relates-to, evaluation criteria…).
- **One objective per PR** where practical; the PR description links its `mbo/` artifacts.
- **`index.md` is the source of truth** for what exists and its state — update it whenever an
  artifact is added or a state changes.
- **State lifecycle:** `idea → designing → specifying → planning → building → in-review → merged
  → done` (or `parked` / `superseded`).
- Per-directory docs rule still applies: this dir has `AGENTS.md` + `CLAUDE.md → AGENTS.md`,
  linked from the root `AGENTS.md`/`CLAUDE.md` Repository Structure.

## Build-breakout policy (CAP-B/C)

When an objective's build is parallelized (the `mbo-plan` skill's optional breakout), these rules
are **policy** — the skill is the procedure that applies them.

**Single source of truth for the dependency graph.** The plan doc `docs/mbo/plans/<slug>.md` §6
("Build leaves / DAG" — an edge list + per-leaf `done-when` gate + blocking-first order) is the
**authoritative, reviewable** graph. Everything else is a generated/mirrored projection:
- the **design issue body** mirrors it as a small mermaid/ASCII DAG (GitHub renders mermaid);
- GitHub's native **sub-issue tree + progress bar** show per-leaf state automatically;
- the **gss registry** (`gss feature list`) is the live *runtime* projection (worker `--base` = the
  in-edge);
- `index.md`'s per-objective leaf sub-table is an **index**, not a second source.

Edit the graph in the plan doc; everything else regenerates from it.

**One isolation mechanism per leaf** — they don't compose over the same paths:

| Mechanism | Owns | Use for a leaf when… |
| :-- | :-- | :-- |
| `gss feature worker` worktree | branch + draft PR + stack/registry (system of record) | the leaf **produces committed code + a draft PR** (the default for every build leaf) |
| harness worktree isolation (`EnterWorktree`; the Workflow tool's worktree-isolation option only if its schema exposes one — confirm at call time) | an ephemeral throwaway worktree for one agent run | the leaf is **read-only / analysis** (an audit, survey, design probe) that emits findings, not commits |
| tmux-mgr panes | long-lived interactive agent sessions across worktrees | you want **human-observable** agents to attach to and steer, driving the gss-worker worktrees |

Normal build path: `gss feature worker add` creates each leaf's worktree + draft PR (system of
record); the Workflow / `/team` agent works **inside that existing worktree** — it does not create
a second one. Reserve ephemeral harness worktree isolation for read-only leaves. Never point an
ephemeral worktree and a gss worker at the same paths.

See [`README.md`](./README.md) for the human-facing overview.
