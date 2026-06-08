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
  teams) per docs/mbo/GEMINI.md. Also anchors the objective to a GitHub design issue
  + a gss draft PR, and — when the plan is ready and it's time to BUILD — asks whether to
  "break out the work / parallelize / divide it across the teams": a dependency graph of
  leaf tasks, each a gss feature worker with its own worktree, draft PR, and linked
  GitHub sub-issue, fanned out via a Workflow, blocking tasks first.
---

# mbo-plan — plan an objective the MBO way

This repo keeps all objective-driven design work in **`docs/mbo/`** as consistent
`design → spec → plan` artifacts, tracked in `docs/mbo/index.md`. This skill drives that
pipeline so every objective is captured, classified, routed to the right workflow, written
in the right place with the right shape, and tracked — instead of ad-hoc docs scattered around.

**Source of truth:** read `docs/mbo/GEMINI.md` first. It owns the pipeline, the task-type →
skill-workflow routing table, the slug/naming conventions, and the state lifecycle. This skill
is the *procedure*; that file is the *policy*. If they ever disagree, GEMINI.md wins.

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

### 5 — Anchor the objective: design ISSUE + design DRAFT PR (CAP-A)
Every objective gets **two GitHub anchors**, both recorded in `index.md`:

1. **A design ISSUE** — the durable tracker for the objective (survives across PRs).
   - Locate it: if the user named `#N`, use it. Else search:
     `gh issue list --search "<slug> in:title" --state open --json number,title`.
   - Create it if absent (confirm first): `gh issue create --title "<slug>: <objective>"
     --body "<one-liner + links to docs/mbo/ artifacts>"`.
   - This issue is the **parent** for the build sub-issues in CAP-C, and the objective's **live
     dashboard**: GitHub's native sub-issue tree + progress bar (`subIssuesSummary`) reflect build
     state automatically. **Do not hand-maintain a task-list checklist** — on this endpoint a
     Markdown task list does not drive the rollup (`trackedIssues` is empty); native sub-issues are
     the authoritative progress facet. Body = the objective one-liner + links to the `docs/mbo/`
     artifacts + a small mermaid/ASCII DAG mirrored from plan §6 (sub-issues give order + progress,
     but **not** edges — the DAG fills that gap; GitHub renders mermaid).
2. **A design DRAFT PR** — where the `docs/mbo/...` artifacts are committed and reviewed.
   - If you're already on a `gss feature` worker / draft PR, commit the artifacts there.
   - If not, open one as draft. Prefer a `gss feature` worker (`gss feature start <slug>` →
     `worker add`) so the design PR and the CAP-C build PRs live in one stack; fall back to
     `gss pr` (draft) for a standalone doc PR.
   - **Per the repo's gss rules, confirm via the interactive prompt before any
     `git add`/`commit`/`gss push`/`gss pr`.** Never run `install.sh` from a worker worktree.

**Always locate before create** (re-running on an existing objective must not spawn a duplicate):
search by slug, create only on a miss, confirm the create, record the resulting numbers in
`index.md` immediately so the next run finds them there. The **issue body** and **PR description**
link the `docs/mbo/` artifacts; `index.md` records issue#, PR#, and state. The slug is the join key.

> **Where the dependency graph lives (single source):** the **plan doc**
> `docs/mbo/plans/<slug>.md` §6's **"Build leaves / DAG"** subsection (see §6b for its shape: an
> edge list + per-leaf `done-when` gate + blocking-first order) is the authoritative, reviewable
> graph; everything else (design-issue mermaid DAG, GitHub sub-issue tree, `gss feature list`,
> `index.md`'s leaf sub-table) is a generated/mirrored projection. This is **policy** — the
> normative statement lives in [`docs/mbo/GEMINI.md` § Build-breakout policy](../../../docs/mbo/GEMINI.md);
> this skill is the procedure that applies it.

### 6 — BUILD breakout: decompose → dependency graph → Workflow fan-out (CAP-B, optional)
When the **plan is approved** and it's time to build, **ASK the user** (via the interactive prompt)
whether to break the work out for parallel team execution, or build it sequentially in one PR.
**Default to NOT breaking out** — parallelism pays off only when leaves are genuinely independent
and the integration cost is lower than the serial cost. Only if they choose parallel:

**6a. Cut the plan into LEAF tasks.** A leaf is the smallest unit one worker can finish without
touching another leaf's files. Derive leaves from the plan's **§2 file inventory + §3 interface
contracts**: group files that change together behind one interface into one leaf.
- **Good leaf:** owns a disjoint set of paths; depends on others only through a *frozen interface*
  (a Go interface, a proto message, a CLI/stdout contract, a file format) declared in the plan —
  not through shared implementation. Independently testable, with a **`done-when` gate** recorded
  in its plan §6 row — the objective check that closes its sub-issue and promotes its draft PR.
  Reuse the repo's existing bars (the `sdk/` Go ≥60% coverage gate,
  `superpowers:verification-before-completion`); don't invent per-leaf criteria. A leaf with no
  testable gate isn't really a leaf.
- **False split (merge it back):** two "leaves" edit the same files, or one needs the other's
  internals, or the interface between them is still in flux. Splitting these trades one PR for two
  PRs plus a perpetual rebase. **When in doubt, fewer, larger leaves.**

**6b. Build the DEPENDENCY GRAPH.** For each leaf list what it *consumes* (an interface another
leaf *produces*). Edge `A → B` = "B depends on A's interface". This must be a DAG — a cycle means
the split is wrong, merge the cycle into one leaf. Classify:
- **BLOCKING (sequence first):** a leaf whose interface others import — typically the
  `go-goarch`-owned interface/proto/contract leaf. Do these first so downstream leaves compile
  against a frozen contract. *Interface-First.*
- **LEAF (parallel):** no outgoing edges, or depends only on already-merged blocking leaves. These
  fan out concurrently and finish fastest.

Record the graph in the **plan's §6** so it's reviewable and survives into CAP-C. The
`templates/plan.md` §6 ("Integration & rollout") has **no structured slot** for it, so add a
concrete **"Build leaves / DAG"** subsection there — leaf → owns-paths → consumes (in-edges) →
`done-when` gate → blocking?. Frozen interfaces themselves stay in §3; §6 references them:

```
### 6.x Build leaves / DAG   (authoritative graph — mirrored to issue body & gss bases)
| Leaf | Owns (paths) | Consumes (← edge) | done-when gate | Blocking? |
| :-- | :-- | :-- | :-- | :-- |
| iface | pkg/foo/iface.go | — | go build + ≥60% cov on stub | yes (base) |
| api   | pkg/foo/api/**   | iface (§3 Foo iface) | go test ./api ≥60% + verification-before-completion | no |
```

`A → B` means B's row lists A under *Consumes*. The order column (Blocking? = yes) is the
blocking-first sequence for §6c / CAP-C. Keep this table in sync with the gss worker bases (§7).

**6c. Fan out via a Workflow.** Sequence the blocking leaves first; then dispatch the parallel
leaves with the **Workflow tool** (`/workflows`), routing each leaf to the right team via `/team`
(go → go-team, web → web-team, CI → ai-ci, infra → terraform-aws) per the same routing table in
`docs/mbo/GEMINI.md`.

**Pick ONE isolation mechanism per leaf — they don't compose over the same paths.** Which mechanism
maps to which kind of leaf (`gss feature worker` for code-producing leaves, ephemeral harness
worktree isolation for read-only leaves, tmux-mgr panes for human-observable sessions) is **policy**,
stated normatively in [`docs/mbo/GEMINI.md` § Build-breakout policy](../../../docs/mbo/GEMINI.md).

So the normal build path is: `gss feature worker add` creates each leaf's worktree+PR (system of
record), then the Workflow/`/team` agent (optionally surfaced via tmux-mgr) *works inside that
existing worktree* — it does **not** create a second one. Reserve ephemeral harness worktree
isolation (`EnterWorktree` is the verified tool; whether the Workflow tool exposes an equivalent
`isolation` option must be **confirmed against the Workflow tool's own schema at call time** — it is
not assertable from this repo) for read-only leaves. Never point an ephemeral worktree and a gss
worker at the same paths.

### 7 — Per-leaf workers, sub-issues, and graph state (CAP-C)
Lean on **`gss feature`** — it already is the worktree + draft-PR + dependency-graph engine. Do
**not** reinvent worktree/PR plumbing. One leaf = one worker.

- **One feature per objective:** `gss feature start <slug> --goal "<objective>"` (once).
- **One worker per leaf:** `gss feature worker add --feature <slug> --purpose <leaf> --description
  "<leaf goal>" --base <base> --json` (`--description` and `--purpose` are required; `--json` emits
  `{worker_ref,branch,worktree_path,base_branch}` — capture it to script the worktree `cd` and the
  sub-issue cross-link). **`--base` IS the dependency edge:** a leaf depending on blocking leaf *X*
  is created `--base <X's branch>` so it stacks on X; independent leaves omit `--base` and inherit
  the feature's default base (`main`). This makes the 6b DAG literal in the stack — so **create
  blocking leaves before their dependents**, or the base branch won't exist yet. Tag provenance with
  `--engine claude` / `--pane-id` / `--session-id` when a Workflow or tmux-mgr agent will drive it.
- **Per-leaf draft PR:** in each worker worktree, `gss feature checkpoint` rebases, pushes, and
  **creates the draft PR on first run / updates it thereafter**, refreshing the stack section. This
  is the per-leaf draft-PR verb — the classic `gss push`/`pr` are *refused* inside a worker worktree.
  Confirm via the interactive prompt before the push. `checkpoint --auto` is the non-interactive
  variant for hooks (`--auto --dry-run` previews); don't use `--auto` to bypass the mandatory human
  confirmation in an interactive run.
- **Per-leaf SUB-ISSUE linked to the design issue (native sub-issues — GitHub does the tracking):**
  native sub-issues are GA on this repo (verified: `addSubIssue` mutation +
  `Issue.subIssuesSummary` present; the REST `sub_issues` endpoint returns `201`). `gh issue create`
  has no `--parent` flag (gh 2.92), so create the child then link via the REST endpoint. The path
  addresses the parent by issue **number**, but `sub_issue_id` is the child's **REST database id**
  (`.id`) — *not* its issue number and *not* its GraphQL `node_id` (the #1 gotcha):
  ```
  child=$(gh issue create --title "<slug>/<leaf>: <goal>" \
            --body "Part of #<N> · plan §6 · PR: (pending)" | grep -oE '[0-9]+$')
  cid=$(gh api repos/{owner}/{repo}/issues/$child --jq .id)          # REST .id — NOT number/node_id
  gh api --method POST repos/{owner}/{repo}/issues/<N>/sub_issues -F sub_issue_id=$cid   # 201 = linked
  ```
  Idempotent — list first, skip if present:
  `gh api repos/{owner}/{repo}/issues/<N>/sub_issues --jq '.[].number'`. The parent's progress bar
  updates automatically when the child closes as `completed` (the default close reason / PR auto-close).
- **PR ↔ sub-issue (the load-bearing link):** put `Closes #<sub-issue>` in the **worker's draft-PR
  body** (the `gss feature checkpoint` PR). That fills the Development panels and **auto-closes the
  sub-issue as completed on merge**, ticking the parent dashboard with zero manual steps. Set the
  sub-issue's `PR:` line to `#<pr>` once the PR exists so the cross-link shows before merge.
- **Overlap = a false split surfaced:** `gss feature conflicts --json` lists paths touched by >1
  worker. Any overlap means two leaves aren't independent — merge them or re-cut the boundary
  (run it as a dry-run gate on proposed boundaries *before* fan-out), don't paper over it with rebases.
- **State:** `gss feature list --tree` (or `--feature <slug> --json`) renders the live stack/graph —
  the gss registry is **authoritative** for worker/PR/base state; don't hand-maintain a parallel
  copy. In `index.md`, under the objective's row add a small **leaf sub-table** mirroring only what
  gss doesn't track — the leaf → team → sub-issue mapping — with PR#/state read from
  `gss feature list --json`, not guessed:

  ```
  ### <slug> — build leaves (feature: `<slug>`; graph in plans/<slug>.md §6; `gss feature list --tree`)
  | Leaf | Team | Worker | Sub-issue | Draft PR | Blocking? |
  | :-- | :-- | :-- | :-- | :-- | :-- |
  | iface  | go-goarch | <slug>/.../iface | #<sub> | #<pr> | yes (base) |
  | api    | go-godev  | <slug>/.../api   | #<sub> | #<pr> | no |
  ```

### 8 — Integrate: blocking-first, then reconcile leaves (CAP-C)
- **Land blocking leaves first** (they're the stack bottom). When a worker's PR merges,
  `gss feature merged` re-targets its children onto the merged base and (when the stack is linear and
  the single child has `restack_count 0`) auto-promotes the next draft to ready — this *is* the
  integration walk down the DAG. A wide fan-out (one blocking leaf → N parallel children) does **not**
  auto-promote: re-target the children onto the merged base and promote each manually.
- **Re-target a moved edge** with `gss feature restack <worker> --onto <newBase>` (note: restack
  permanently opts a worker out of auto-promote — minimize manual restacks).
- **Tear down** a finished leaf with `gss feature done <worker>`; close its sub-issue; advance the
  objective's `index.md` state (`building → in-review → merged → done`). **Design issue end-state:**
  for a design-only objective, close it when the design PR lands; when CAP-B/C is used it's the
  build-tracking parent — close it only when every sub-issue is closed.

## Notes
- **One slug, one objective.** Slug joins the design issue, the design PR, the `gss feature`, and
  all `docs/mbo/` artifacts. Per-leaf workers/sub-issues/PRs hang off that one objective.
- **gss is the single writer for PRs and the registry.** Workflow/team agents and tmux-mgr *drive*
  work inside worker worktrees; only `gss feature checkpoint/merged/restack/done` mutate
  branch/PR/stack state. Local rebase conflicts → the `git-machete` skill; gss still owns PRs.
- **Re-entrant & resumable.** CAP-A/C are long-running — assume the skill is re-invoked mid-flight.
  Every create is **locate-before-create** (issue, PR, worker, sub-issue), so a second run reconciles
  instead of duplicating. Recover live state from the systems of record, not memory:
  `gss feature list --feature <slug> --json` (workers/PRs/bases), `gh api .../sub_issues` (links),
  `index.md` (slug → issue/PR). If the registry drifts from the worktrees/remote, repair with
  `gss feature audit` before continuing — **observable state wins over the registry**. Worked drift
  recovery (e.g. a worker's worktree was deleted out-of-band, or a PR 404s):
  ```
  gss feature audit --feature <slug> --json   # 1. inspect: lists missing worktrees, 404 PRs, diverged bases
  gss feature audit --feature <slug> --repair # 2. deterministic registry-local fixes only — never force-pushes,
                                              #    renames a branch, or calls a mutating gh verb; non-zero exit on error-severity
  gss feature list --feature <slug> --tree    # 3. reconcile: confirm the stack/graph matches the plan §6 DAG
  ```
  `--repair` heals the registry; it does **not** recreate a deleted worktree or reopen a PR — for a
  genuinely lost leaf, re-run `gss feature worker add` (locate-before-create skips live ones) and
  re-`checkpoint`. Sync the `index.md` leaf sub-table and sub-issue `PR:` lines from the repaired
  `gss feature list --json` afterward.
- **Lead with facts.** Ground designs and leaf boundaries in what's verified in the repo (read the
  code, run the greps), not assumptions.
- **The pipeline is partial-order, not rigid.** A bugfix may be plan-only and single-PR (skip
  CAP-B/C entirely); an ADR may be design-only. Always register it in `index.md`; produce only the
  artifacts and breakout the objective warrants.
