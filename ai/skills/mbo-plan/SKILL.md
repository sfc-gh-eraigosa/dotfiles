---
name: mbo-plan
description: >-
  Plan a new objective end-to-end in this repo's docs/mbo Management-By-Objective
  system — turn a GitHub issue or a gss draft-PR worktree into consistent
  design/spec/plan artifacts and track it in docs/mbo/index.md. Use this whenever
  the user wants to START planning or designing a new feature, skill, CLI, or
  service: "plan this issue", "let's design X", "spec this out", "write a plan
  for #N", "start an MBO / a design doc", or when they're working in a gss draft
  PR and want to scope the work — even if they don't say "mbo". Routes the task to
  the right skill workflow (skill-creator, brainstorming→writing-plans, the go/web
  teams) per docs/mbo/GEMINI.md.
---

# mbo-plan — plan an objective the MBO way

This repo keeps all objective-driven design work in **`docs/mbo/`** as consistent
`design → spec → plan` artifacts, tracked in `docs/mbo/index.md`. This skill drives that
pipeline so every objective is captured, classified, routed to the right workflow, written
in the right place with the right shape, and tracked — instead of ad-hoc docs scattered around.

**Source of truth:** read `docs/mbo/GEMINI.md` first. It owns the pipeline, the task-type →
skill-workflow routing table, the slug/naming conventions, and the state lifecycle. This skill
is the *procedure*; that file is the *policy*. If they ever disagree, the GEMINI.md wins.

## Procedure

### 1 — Capture the objective
Find out what you're planning and pin a short **slug** (e.g. `prping`, `sdk-migration`):

- **From an issue:** `gh issue view <N> --json number,title,body,labels` — the title/body is
  the objective; note the issue number for `index.md`.
- **From a gss draft PR worktree** (the common "I'm working on this right now" case): detect the
  current branch and its PR — `gh pr view --json number,title,body,isDraft,headRefName` (no arg
  uses the current branch). If there's no PR yet but you're on a `gss feature` worktree, the
  branch name + the user's intent define the objective; offer to open the draft PR with
  `gss pr` so there's something to attach to.
- Derive the slug from the title (short, kebab-case, no date — dates live inside the docs and in
  `index.md`).

### 2 — Classify and route
Decide the task type and pick the matching **skill workflow** from the routing table in
`docs/mbo/GEMINI.md`. The common cases:

| Objective is… | Workflow to run |
| :-- | :-- |
| a **new skill** | `skill-creator:skill-creator` → `superpowers:writing-skills` |
| a **dotfiles feature** (shell/`opt/`/`ai/`/install) | `superpowers:brainstorming` → `superpowers:writing-plans` → TDD |
| a **Go CLI under `sdk/`** | `brainstorming` → `writing-plans`; mirror `sdk/gss`; engage **go-team** (`go-goarch`) for interfaces |
| a **Go RPC/gRPC service** | `go-goarch` (proto/boundaries) → `writing-plans` → `go-godev`/`go-goqa` |
| a **UI / web** piece | `brainstorming` (visual companion) → **web-team** → `writing-plans` |
| **AWS infra/serverless** | `deploy-on-aws:*` / `aws-core:*` / **terraform-aws** team |
| a **large audit/migration** | a `Workflow` fanned out to the **architecture team** → `writing-plans` |

Don't reinvent the analysis — *invoke the skill* the table points to. This skill's job is to make
sure the right one runs and the output lands in the right place.

### 3 — Produce the artifacts (from templates)
Copy the relevant starter(s) from `docs/mbo/templates/` and let the routed workflow fill them:

- **Design** → `docs/mbo/designs/<slug>.md` (from `templates/design.md`) — only for novel /
  architectural work; skip for trivial objectives.
- **Spec** → `docs/mbo/specs/<slug>.md` (from `templates/spec.md`) — the `brainstorming` output:
  goal, use cases, evaluation criteria per feature, verification harness.
- **Plan** → `docs/mbo/plans/<slug>.md` (from `templates/plan.md`) — the `writing-plans` output:
  file inventory, TDD build order, traceability.

Keep the slug identical across the three so they correlate. Use the bare slug as the filename.

### 4 — Register in the index
Add or update the objective's row in `docs/mbo/index.md`: slug, links to whichever artifacts
exist, issue number(s), PR number(s), and the **state** (`idea → designing → specifying →
planning → building → in-review → merged`). The index is how anyone finds what exists and where
it stands — never skip it.

### 5 — Attach to a gss draft PR and land
Artifacts belong on a branch, reviewable:

- If you're already on a `gss feature` worktree / draft PR, commit the `docs/mbo/...` artifacts
  there. Per the repo's gss rules, **confirm via the interactive prompt before any
  `git add`/`commit`/`gss push`/`gss pr`**.
- If not, open one (`gss pr`, draft) so the design work has a home before implementation starts.
- Put links to the objective's `docs/mbo/` artifacts in the PR description, and keep the
  `index.md` state current as the work moves from planning → building → in-review.

## Notes
- **One slug, one objective, ideally one PR.** Resist spreading an objective's artifacts under
  varied names — the slug is the join key.
- **Lead with facts.** Ground designs in what's verified in the repo (read the code, run the
  greps), not assumptions — the same discipline the templates and `superpowers:brainstorming`
  expect.
- **The pipeline is partial-order, not rigid.** A bugfix may be plan-only; an ADR may be
  design-only. Always register it; produce only the artifacts the objective warrants.
