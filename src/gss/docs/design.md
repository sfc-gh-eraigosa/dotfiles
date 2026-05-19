# gss feature — Design

## Problem

A single repo checkout can only hold one branch's working state. When two
Claude/Gemini sessions try to build different features in the same repo at the
same time, they collide: branch hops blow away the other session's working
state, `gss push` sweeps in unrelated dirty files, and there is no way to keep
each session's mental model intact.

## Goals

- One isolated working directory per in-flight feature.
- A new Claude/Gemini session for a new feature is a single command away.
- No accidental pushes; no accidental cleanup.
- Draft PRs as durable, sharable checkpoints — including enough metadata to
  resume work after a long pause.
- Reuse what's already in the repo (`gss` for git safety, `tmux-mgr` for tmux
  panes, `gh` for PR plumbing). Don't reimplement.

## Non-goals

- Replacing `gss push` / `gss pr` for non-parallel work.
- Automating merge / squash strategy — leave to GitHub.
- Coordinating *content* between concurrent features (we surface conflicts;
  humans resolve them).
- Cross-machine sync of in-flight features.

## Concepts

| Concept    | Definition                                                              |
|------------|-------------------------------------------------------------------------|
| Feature    | A named unit of in-flight work, 1:1 with a git branch and a worktree.   |
| Worktree   | `git worktree` checkout at `~/.config/gss/worktrees/<repo>/<feature>`.  |
| Checkpoint | Push + draft-PR refresh. The unit of "save your work to GitHub".        |
| Registry   | JSON file tracking active features per repo.                            |

## Filesystem layout

```
~/.config/gss/
├── approval.token              # existing gss approval mechanism
└── worktrees/
    └── <repo-name>/
        ├── registry.json       # active features for this repo
        └── <feature-name>/     # the worktree itself
            ├── FEATURE.md      # resume metadata + freeform notes
            └── …               # checkout of feature/<name>
```

`registry.json` schema:

```json
{
  "features": [
    {
      "name": "parallel-worktrees",
      "branch": "feature/parallel-worktrees",
      "worktree": "/Users/me/.config/gss/worktrees/dotfiles/parallel-worktrees",
      "started_at": "2026-05-17T10:34:00Z",
      "base_commit": "abc123",
      "pr_url": "https://github.com/me/dotfiles/pull/42",
      "pr_state": "draft"
    }
  ]
}
```

## Command surface

### `gss feature start <name> [--goal "..."]`

1. Validate name (`[a-z0-9-]+`, no `/`).
2. Reject if `feature/<name>` exists locally, on origin, or in registry.
3. Fetch origin; capture `base_commit = origin/main`.
4. `git worktree add ~/.config/gss/worktrees/<repo>/<name> -b feature/<name> <base_commit>`.
5. Write `FEATURE.md` from template. If `--goal` was omitted, emit a warning
   ("Goal not set — fill in FEATURE.md before first checkpoint").
6. Insert into registry.
7. Decide: reuse current window or spawn pane?
   - Reuse if no other active feature has a live tmux pane in the same session.
   - Spawn otherwise (see tmux-mgr integration below).
8. Print worktree path and next-step hint.

### `gss feature list`

Read registry; for each entry, show: name, branch, worktree, dirty (via
`git -C <wt> status --porcelain`), last commit timestamp, PR URL & state.
Reconciles registry against `git worktree list` to drop stale entries.

### `gss feature checkpoint [--message "..."]`

1. Refuse if cwd isn't inside a registered worktree.
2. `git fetch origin && git rebase origin/main` — abort cleanly on conflict; user resolves in the worktree.
3. `git push -u origin feature/<name>`.
4. Render PR body from `FEATURE.md` + auto-section (recent commits, files changed, time since last checkpoint).
5. If `pr_url` empty in registry → `gh pr create --draft --base main --title <title> --body-file <tmp>`.
6. Else → `gh pr edit <num> --body-file <tmp>`.
7. Update registry with PR URL/state.

PR title comes from the first H1 of `FEATURE.md` (template seeds `# Feature: <name>`, easy to rename).

### `gss feature pr [--ready]`

Promote the draft PR to ready-for-review (`gh pr ready <num>`), or open ready directly if no PR yet.

### `gss feature done <name> [--force]`

1. Read registry entry.
2. Refuse if worktree is dirty.
3. Refuse if PR is open and unmerged. `--force` overrides.
4. `git worktree remove <path>`.
5. `git branch -D feature/<name>` (local; remote left to GitHub auto-delete).
6. Remove from registry.

## Conflict protection

| Risk                                    | Mitigation                                |
|-----------------------------------------|-------------------------------------------|
| Two worktrees check out same branch     | Prevented by git itself.                  |
| `feature/foo` accidentally reused       | `start` checks local + origin + registry. |
| Drift from `main` discovered at merge   | `checkpoint` rebases on origin/main first.|
| Two features edit the same file silently| `feature list` flags overlap (v1.1).      |
| Stale registry after crashes            | `feature list` reconciles vs `git worktree list` each run. |

## PR lifecycle

```
start ──► (work) ──► checkpoint ──► [draft PR]
                          │
                          └──► checkpoint ──► [draft PR updated]
                                  │
                                  └──► pr --ready ──► [ready PR]
                                          │
                                          └──► (review, merge on GitHub)
                                                  │
                                                  └──► done ──► [worktree pruned]
```

A feature can sit at "draft PR" indefinitely. The PR body is the durable
record. Resume = `cd` to worktree (path is in PR body) + open Claude.

## FEATURE.md template

```markdown
# Feature: {{name}}

- **Started**: {{date}}
- **Branch**: feature/{{name}}
- **Base commit**: {{base_commit}}
- **Worktree**: {{worktree_path}}
- **Resume**: `cd {{worktree_path}} && claude`

## Goal
{{goal_or_placeholder}}

## Decisions & notes
<!-- append freely; rendered verbatim into PR body -->

## Open questions
<!-- append freely -->
```

PR body rendered at checkpoint time:

```markdown
<!-- gss feature checkpoint -->

{{contents of FEATURE.md}}

---

## Auto-generated

- Last checkpoint: {{timestamp}}
- Commits since last checkpoint: {{n}}
- Files changed: {{file_list}}
- Recent commits:
  - {{sha}} {{subject}}
  - …
```

## tmux-mgr integration

`gss feature start` decides between two modes:

- **Reuse current window**: no spawn; print `cd <path>` and let the caller (or the slash command wrapper) do it.
- **Spawn pane**: invoke `tmux-mgr pane spawn --cwd <worktree> --cmd <ai-engine>` to split a new pane below the current one and launch the appropriate AI engine.

Engine detection precedence:
1. `$CLAUDECODE` set → `claude`.
2. `$GEMINI_CLI` set → `gemini`.
3. Parent process name match.
4. `--engine` flag.
5. Prompt.

`tmux-mgr` gains a minimal new verb (`pane spawn`) that does *not* own a
worktree (unlike `agent start`). This keeps the boundary clean: `gss` owns
filesystem + git state; `tmux-mgr` owns tmux panes.

## Decisions

- **PR title**: first H1 of `FEATURE.md` (template seeds `# Feature: <name>`).
- **`done` strictness**: refuses while PR is open & unmerged; `--force` overrides.
- **`--goal`**: optional at `start`; emits a warning when missing so `FEATURE.md` doesn't ship to the PR body empty.
- **Worktree location**: `~/.config/gss/worktrees/<repo>/<feature>` — central, symmetric with `tmux-mgr`.
- **Branch naming**: `feature/<name>` — no timestamp, matches `gss pr` style minus suffix.
- **Push policy**: never implicit. `start` does not push. `checkpoint` and `pr` are the only push paths.
- **Cleanup policy**: never implicit. Panes and worktrees survive until `done` is called.

## Open design questions

1. Cross-feature edit-overlap detection — v1 or v1.1?
2. Should we ever auto-checkpoint (e.g. on Claude session end), or always explicit?
3. Multi-machine: nice-to-have but registry would need to live in the repo, which conflicts with the "don't pollute the checkout" goal.
4. Engine detection fallback: if env detection fails, prompt or default to Claude?
5. Remote branch deletion on `done`: leave to GitHub auto-delete-on-merge, or delete explicitly?
