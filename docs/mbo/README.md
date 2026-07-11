# MBO — Management By Objective

A small, repeatable system for turning an **objective** (a GitHub issue, or a `gss feature`
draft PR you're working in) into shipped work, leaving a consistent paper trail.

Every objective flows: **issue → design → spec → plan → build → PR**, and its artifacts live
in three flat folders here:

| Folder | Artifact | Produced by |
| :-- | :-- | :-- |
| [`designs/`](./designs) | the *why & shape* (architecture, trade-offs, ADR) | `superpowers:brainstorming` / architecture-team `Workflow` |
| [`specs/`](./specs) | validated requirements + acceptance/eval criteria | `superpowers:brainstorming` |
| [`plans/`](./plans) | the TDD-ordered implementation plan | `superpowers:writing-plans` |

[`index.md`](./index.md) tracks every objective — its artifacts, linked issue(s) and PR(s),
and lifecycle state. [`templates/`](./templates) holds the starting skeletons.

**Start here:** read [`AGENTS.md`](./AGENTS.md) — it routes each task type to the right skill
workflow and explains the pipeline and conventions. For the common case, use the **`mbo-plan`**
skill, which automates capture → classify → design/spec/plan → index registration.

## Why "flat"?

The folder encodes the artifact *type*; the filename is the *objective slug*. An objective's
design/spec/plan share one slug, so they're trivially correlated, and `index.md` is the single
join across issues, PRs, and state. No deep nesting, no per-objective folders to spelunk.
