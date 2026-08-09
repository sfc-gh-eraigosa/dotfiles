# gss feature — Design

## Problem

A single repo checkout can only hold one branch's working state. When two
Claude/Antigravity sessions try to build different features in the same repo at the
same time, they collide: branch hops blow away the other session's working
state, `gss push` sweeps in unrelated dirty files, and there is no way to keep
each session's mental model intact.

A single feature is also rarely a one-agent job. We routinely want several
agents working on the same feature in parallel — one on the API, one on the
UI, one on docs — each in its own worktree, each producing its own PR. Those
PRs naturally want to stack: the UI sits on top of the API, docs sit on top of
the UI, and the bottom of the stack targets `main`.

## Scope: what gss owns vs. what tmux-mgr owns

`gss` is a **git + GitHub + worktree** tool. It manages:

- Worktrees under `~/.config/gss/worktrees/<owner>/<repo>/<feature>/...`.
- Feature branches (creation, naming, restack, deletion).
- Commits, pushes, rebases.
- GitHub PR creation / edit / promotion via `gh`.
- The feature/worker registry (a flat JSON of git facts).

`gss` knows nothing about tmux, panes, or which agent CLI is running. It does
not spawn processes; it does not detect "engines". The output of every
`gss feature` command is a path, a branch name, and/or a PR URL — caller
chooses what to do next.

`tmux-mgr` is the **process / pane orchestrator**. It owns:

- Spawning a pane at a given cwd with a given command.
- Deciding which agent CLI (`claude`, `agy`, …) to launch.
- Tracking pane ↔ worker mappings.
- Firing close hooks when a pane / session ends.

`tmux-mgr` *uses* `gss` (it shells out to `gss feature worker add`,
`gss feature checkpoint --auto`, `gss feature list --json`). `gss` never
calls `tmux-mgr`. The dependency arrow goes one way.

## Goals

- One isolated working directory per in-flight worker.
- Multiple workers per feature, each owned by a named agent / purpose.
- A new Claude/Antigravity session for a new worker is a single command away.
- No accidental pushes; no accidental cleanup.
- Draft PRs as durable, sharable checkpoints — including enough metadata to
  resume work after a long pause.
- First-class stacked PRs: a worker can be based on another worker's branch,
  and `gss` keeps the PR descriptions cross-linked so the stack stays
  navigable on GitHub.
- Reuse what's already in the repo (`gss` for git safety, `tmux-mgr` for tmux
  panes, `gh` for **all** PR / remote-branch plumbing). Don't reimplement.
- Expose a clean, scriptable surface that `tmux-mgr` (or any other
  orchestrator) can drive — including a non-interactive auto-checkpoint
  command suitable for pane-close hooks.
- Surface cross-worker file conflicts cheaply so the orchestrator can react
  before the merge step.
- **`gss` keeps behaving like `gss`** when invoked from a normal branch /
  default checkout. The multi-worker flow only engages under
  `gss feature …` (or when invoked from inside a feature worktree).

## Non-goals

- Replacing `gss push` / `gss pr` for non-parallel work.
- Automating merge / squash strategy — leave to GitHub.
- Coordinating *content* between concurrent workers (we surface conflicts;
  humans resolve them).
- Cross-machine sync of in-flight features.
- Fully automated stack re-targeting after a mid-stack force-push (we re-target
  on parent-merge; arbitrary stack rewrites stay manual).

## Concepts

| Concept    | Definition                                                                |
|------------|---------------------------------------------------------------------------|
| Feature    | A named workstream. Has 1..N workers and a shared base commit.            |
| Worker     | One agent's slice of a feature, identified by `<user>/<purpose>[-<sfx>]`. |
| Worktree   | `git worktree` at `~/.config/gss/worktrees/<owner>/<repo>/<feature>/<user>/<purpose>[-<sfx>]`. |
| Stack      | The chain formed when a worker's `base_branch` points at another worker.  |
| Checkpoint | Push + draft-PR refresh for one worker, with stack cross-links rewritten. |
| Registry   | JSON file tracking features and their workers per repo.                   |

## Naming

### Worker identity

A worker is identified by three parts:

| Part      | Source                                                              |
|-----------|---------------------------------------------------------------------|
| `user`    | `gh api user --jq .login`, else slug of `git config user.email`, else `$USER`. Override with `--user`. |
| `purpose` | Short kebab-case role from `--purpose` (e.g. `api`, `ui`, `docs`, `tests`). May be an alias defined by the calling agent. |
| `suffix`  | Empty by default. A short word (3–5 lowercase letters) drawn at random from a curated 256-word list (see [Suffix wordlist](#suffix-wordlist)) appended **only** when needed to make the worker globally unique within the feature. Passing `--suffix` forces a random draw even when not strictly needed; the flag does **not** accept a user-supplied value — custom or override strings are rejected. |

The display form is `user/purpose` (or `user/purpose-suffix` if a suffix was
needed). Example: `eraigosa/api`, `eraigosa/ui-moss`.

### Branches

```
feature/<name>/<user>/<purpose>[-<suffix>]
```

Examples:

- `feature/parallel-worktrees/eraigosa/api`
- `feature/parallel-worktrees/eraigosa/ui-moss`
- `feature/parallel-worktrees/bot42/docs`

The branch path mirrors the worktree path under
`worktrees/<owner>/<repo>/<feature>/`,
so the merge step in the main repo can read the branch name and figure out
which worker it came from (and therefore which feature / agent).

### Repo identity (name-with-owner)

A single user often clones repos with the same short name from different
orgs (`anthropic/dotfiles`, `eraigosa/dotfiles`, `someone-else/dotfiles`).
A worktree root keyed by short name alone collides; so `gss` keys
worktrees and registry entries by **name-with-owner (NWO)** — the
canonical `<owner>/<repo>` form used by GitHub and `gh`.

Storage: nested directories on disk (`<owner>/<repo>/`), since `/` is the
natural canonical form and the filesystem already supports it.

```
~/.config/gss/worktrees/<owner>/<repo>/...
```

Resolution: gss reads NWO from `gh repo view --json nameWithOwner -q
.nameWithOwner`, caching the result per-repo. Falls back to parsing
`origin`'s URL if `gh` isn't authenticated. User can override with
`--repo <owner>/<repo>` on any subcommand.

### Worktree path

```
~/.config/gss/worktrees/<owner>/<repo>/<feature>/<user>/<purpose>[-<suffix>]/
```

Four-level nesting on disk:
- `<owner>/<repo>/` — repo identity (NWO).
- `<feature>/` — the named workstream.
- `<user>/<purpose>[-<suffix>]/` — the specific worker. Matches the
  trailing two segments of the branch.

A `cd` into any depth lands you somewhere meaningful: the repo root lets
you `ls` to see all features for the repo; the feature root lets you `ls`
to see all active workers; `<user>/` groups your own work; the leaf is the
worktree itself.

### Worker reference (`worker_ref`)

The canonical string identifier for a worker, used everywhere a worker
needs to be named on the command line, in JSON, in env vars, in PR
body cross-links, and in tmux pane metadata. Grammar:

```
worker_ref ::= <feature> "/" <user> "/" <purpose> [ "-" <suffix> ]
```

- `/` is the only segment separator.
- `<suffix>`, when present, is joined to `<purpose>` with `-` (not `/`).
- No leading/trailing whitespace; no other punctuation.

This **exact** string is the value of:

- the `--worker <ref>` flag on every command that accepts one
  (`checkpoint --auto`, `done`, `restack`, `worker update`, `worker
  attach` / `detach` in v1.1).
- the `worker_ref` field returned by `gss feature worker add --json`
  and `gss feature list --json`.
- the `TMUX_MGR_WORKER_REF` pane environment variable set by
  `tmux-mgr pane spawn`.
- the `<worker-ref>` positional in `feature done [<worker-ref>]`.

Display form in human output (`feature list`, error messages) MAY
omit the leading `<feature>/` when context makes it unambiguous, but
JSON and flag values always include it.

### Validation grammar

The identifier segments are constrained so that branch names, paths,
PR titles, and shell-level usage all stay safe:

- `<feature>`: `^[a-z][a-z0-9-]{0,30}[a-z0-9]$` (2–32 lowercase ASCII,
  starts with a letter, ends alphanumeric, hyphens allowed mid-string).
- `<user>`: same regex as `<feature>`.
- `<purpose>`: same regex as `<feature>`, with the additional rule that
  `<purpose>` MUST NOT match any word in the embedded suffix wordlist
  (avoids `purpose-purpose` confusion after a suffix draw).
- `<suffix>`: drawn from the embedded wordlist; never accepted as a
  caller-supplied value.
- `--description`: NFC-normalized Unicode, 1–240 code points,
  printable characters only (no control characters except U+0020
  space). Newlines, ANSI escapes, and the marker tokens
  `<!-- gss:*-* -->` are stripped before persistence.

Validation runs at every call site that accepts these as input —
flags, JSON payloads via `--spawned-by-json`, and registry reads
(reject on load if a persisted value violates current grammar; this
is how a future-tightened regex shakes out legacy data).

### Uniqueness

`gss feature worker add` rejects a worker if **any** of these collide with an
existing worker (in the registry, on disk, or on origin):

- worktree path
- branch name
- PR title slug

If `user/purpose` would collide, gss appends a randomly-drawn suffix from the
curated wordlist and tries again (up to 5 attempts before failing loudly).
Passing `--suffix` forces a random draw even when not strictly needed (e.g.
to give a worker a memorable second-segment name on purpose). The flag is
boolean-style — it never accepts a caller-supplied word, by design. This
keeps every suffix that ever appears in a branch / path traceable back to
the embedded list, makes audit and grep-ability deterministic, and avoids a
class of "looks like a real suffix but isn't" mistakes.

### Suffix wordlist

Exactly **256** short (3–5 letter), lowercase, evocative words drawn from
nature, weather, terrain, plants, minerals, and small everyday objects.
Embedded into the binary at build time via `//go:embed
internal/identity/wordlist.txt`.

Users can extend (or, with explicit opt-in, replace) the embedded list via
`suffixes.wordlist` + `suffixes.wordlist_mode` in `config.yaml`:

- `wordlist_mode: append` (default) — user's words are added to the
  built-in 256. Duplicates are silently de-duped. The effective pool
  size is `256 + len(unique_user_words)`. This is the right knob for
  most users: a few in-jokes or project-specific words on top of a
  large, well-curated base.
- `wordlist_mode: replace` — the built-in 256 is ignored; the user's
  list **is** the pool. Requires ≥ 16 words or gss refuses to start
  (a tiny pool guarantees frequent collisions and surprised users).
  Mostly useful for deterministic test fixtures and vanity setups.

Both modes still enforce the per-word constraints (3–5 lowercase ASCII
letters, no digits/punctuation). `gss config check` reports the final
effective pool size and surfaces any rejected user words.

Sample (first 32 of 256 — full list lives at
`sdk/gss/internal/identity/wordlist.txt`):

```
moss  lake  fern  silt  dune  pine  kelp  reef
sage  mist  peak  cove  tide  wave  snow  hail
rain  dell  glen  moor  tarn  brae  crag  holm
mere  gill  burn  linn  lawn  gale  flax  leaf
```

Why 256: with `k` workers already sharing a `user/purpose` pair, a uniform
draw + 5 retries has collision probability `(k/256)^5`. At `k=10`
(realistic upper bound) that's ~9×10⁻⁸; at `k=50` (worst plausible case)
still ~3×10⁻⁴. 256 = 2⁸ is also a clean byte-aligned size if we ever want
deterministic indexing from a hash. Why 3–5 letters: keeps the leaf
segment short and grep-able while opening a pool large enough to actually
reach 256 distinct, distinguishable words.

Curation rules for the embedded list:

- 3–5 lowercase ASCII letters; no digits, hyphens, or punctuation.
- No homophones or near-twins (`son`/`sun`, `tale`/`tail`) in the same
  list.
- No words that commonly appear as `purpose` segments (`api`, `web`,
  `cli`, `docs`, `ui`, `fix`, `log`, `app`, `lib`, `cmd`, `bin`, `gen`).
- No proper nouns, brand names, profanity, or words with charged
  cultural meaning.
- Pronounceable, easy to type one-handed.

The list is reviewed manually at PR time; `internal/identity/wordlist_test.go`
enforces the count (256), the length range (3–5), and the no-duplicate
invariant on every build.

## Filesystem layout

```
~/.config/gss/
├── config.yaml                          # user config (see Configuration)
├── approval.token                       # existing gss approval mechanism
└── worktrees/
    └── <owner>/
        └── <repo>/
            ├── registry.json            # active features + workers for this repo
            └── <feature-name>/
                ├── FEATURE.md           # shared feature-level metadata
                └── <user>/
                    ├── .gss-meta/                   # gss-internal scaffolding, OUTSIDE the worktrees (#132)
                    │   └── <purpose>[-<suffix>]/
                    │       └── WORKER.md            # worker notes + resume info; auto-removed by `feature done`
                    └── <purpose>[-<suffix>]/
                        └── …            # checkout of feature/<name>/<user>/<purpose>[-<sfx>] (NO WORKER.md inside)
```

> **WORKER.md placement (issue #132).** `WORKER.md` is **leaf-keyed** under
> `<user>/.gss-meta/<purpose>[-<suffix>]/`, deliberately one level keyed by the
> worker leaf so two workers under the same `<feature>/<user>/` (e.g. `api` and
> `impl`) get **distinct** files. It is written **outside** the git worktree so
> it can never appear in the consumer repo's `git status` — without touching the
> consumer's `.gitignore`. `feature worker add` seeds it, `checkpoint --auto`
> appends its log there, and `feature done` removes the leaf's `.gss-meta` dir.

`registry.json` schema:

```json
{
  "schema_version": 1,
  "features": [
    {
      "name": "parallel-worktrees",
      "started_at": "2026-05-17T10:34:00Z",
      "base_commit": "abc123",
      "default_base_branch": "main",
      "description": "Move worktree mechanics out of tmux-mgr into gss",
      "workers": [
        {
          "user": "eraigosa",
          "purpose": "api",
          "suffix": "",
          "branch": "feature/parallel-worktrees/eraigosa/api",
          "worktree": "/Users/me/.config/gss/worktrees/eraigosa/dotfiles/parallel-worktrees/eraigosa/api",
          "base_branch": "main",
          "backend": "git",
          "restack_count": 0,
          "started_at": "2026-05-17T10:34:00Z",
          "description": "Implement gss feature worker add + registry writes",
          "spawned_by": {
            "engine": "claude",
            "session_id": "c1a2b3c4-5e6f-7a8b-9c0d-1e2f3a4b5c6d",
            "pane_id": "%17",
            "tmux_mgr_session": "coder-1715890123",
            "started_at": "2026-05-17T10:34:00Z"
          },
          "pr_url": "https://github.com/me/dotfiles/pull/42",
          "pr_state": "draft"
        },
        {
          "user": "eraigosa",
          "purpose": "ui",
          "suffix": "moss",
          "branch": "feature/parallel-worktrees/eraigosa/ui-moss",
          "worktree": "/Users/me/.config/gss/worktrees/eraigosa/dotfiles/parallel-worktrees/eraigosa/ui-moss",
          "base_branch": "feature/parallel-worktrees/eraigosa/api",
          "backend": "git",
          "started_at": "2026-05-17T11:02:00Z",
          "description": "Wire feature list / conflicts output rendering",
          "spawned_by": {
            "engine": "antigravity",
            "session_id": "g-9z8y7x6w-5v4u-3t2s",
            "pane_id": "%19",
            "tmux_mgr_session": "ui-1715893740",
            "started_at": "2026-05-17T11:02:00Z"
          },
          "pr_url": "https://github.com/me/dotfiles/pull/43",
          "pr_state": "draft"
        }
      ]
    }
  ]
}
```

### Description and provenance fields

Two new fields on every worker make `gss feature list` actually
informative and let later forensics trace a worktree back to the AI
session that created it.

- **`description`** (string, required at create time, max 240 chars). A
  one-line human/agent summary of what this worker is doing. Rendered
  in `gss feature list`, in the PR body, and in the worker's
  `WORKER.md` template header. The feature itself also gets an
  optional `description` for the same purpose at the feature level.
  Suggested style: imperative verb phrase ("Add /api/v1/users
  endpoints", "Refactor session struct to drop RepoRoot"). The agent
  driving the worker is expected to keep this current via
  `gss feature worker update --description "…"` as the work narrows.

- **`spawned_by`** (object, required when the caller is a non-human
  agent). Captures the AI session provenance at creation. Fields:
  - `engine` — `claude` | `antigravity` | `manual` | other identifier
    the caller chooses. `manual` is used when a human ran `gss feature
    worker add` directly. The legacy value `gemini` (retired Gemini CLI)
    may appear in old rows and is still accepted; new records are
    normalized to `antigravity`.
  - `session_id` — the engine's native session identifier. Claude
    Code exposes one (e.g. via `$CLAUDE_SESSION_ID` or its config);
    Antigravity CLI has its own. gss does not introspect the engine — it
    persists whatever the caller passes.
  - `pane_id` — tmux pane id (e.g. `%17`) when the worker is being
    driven from a tmux pane. Optional.
  - `tmux_mgr_session` — the `tmux-mgr` session record id, when the
    worker was spawned via `tmux-mgr agent start`. Optional.
  - `started_at` — ISO 8601 timestamp when the agent attached.

`spawned_by` is **write-once at create time**, never overwritten by
later checkpoints or rebases — it documents who *originally* opened
the worktree. If a different AI session later resumes the worker, the
new session is recorded in an append-only `sessions` array (deferred
to v1.1; see Open design questions). For v1, the original `spawned_by`
plus the commit log is enough provenance.

The CLI surface:

- `gss feature worker add` gains `--description "..."`, `--engine
  <id>`, `--session-id <id>`, `--pane-id <id>`, and
  `--tmux-mgr-session <id>` flags. All optional except `--description`,
  which is required (gss refuses to create a worker without one — the
  "useful worktree explanation" goal becomes a hard contract). A
  single `--spawned-by-json '<object>'` shorthand is accepted for
  callers like `tmux-mgr` that already assemble the object.
- `gss feature worker update --description "..."` updates an existing
  worker's description and re-renders the PR body and `WORKER.md`.
- `gss feature list` shows `description` per row by default; `--with
  spawned_by` adds an `engine / session_id` column.
- `gss feature start` gains an analogous `--description "..."` for the
  feature-level description.

## Worktree backend abstraction

`gss` currently creates worktrees with `git worktree add`. We expect to
experiment later with **alternative backends** — overlayfs-mounted
copy-on-write checkouts (shared base, cheap to spin up dozens), tmpfs
scratch trees, or whatever else turns out to be faster on the host. To
keep that future open without rewriting the rest of `gss`, every caller
inside `gss` talks to the worktree through a narrow Go interface, not
to `git worktree` directly.

### The interface

```go
// internal/worktree/backend.go

package worktree

type CreateReq struct {
    Path       string  // absolute target path; backend owns the inode
    Branch     string  // branch to materialize at HEAD
    BaseBranch string  // upstream branch (used for tracking + rebase later)
    BaseCommit string  // optional pinned commit; "" → tip of BaseBranch
}

type Info struct {
    Path       string
    Branch     string
    HeadSHA    string
    BaseBranch string
    Backend    string  // "git", "overlayfs", ...
}

type Status struct {
    Clean       bool
    Staged      []string
    Dirty       []string  // tracked, modified, not staged
    Untracked   []string
    Ahead       int       // commits ahead of BaseBranch on origin
    Behind      int
}

type Backend interface {
    // Name returns the backend ID stored in registry.json so a future
    // gss run knows which backend created a given worktree.
    Name() string

    // Create materializes a worktree per req. Implementations must be
    // idempotent enough that a partial failure can be retried after
    // Remove.
    Create(req CreateReq) (Info, error)

    // Remove tears down the worktree. Must refuse if Status reports
    // dirty unless force=true.
    Remove(path string, force bool) error

    // List enumerates worktrees managed by this backend under the
    // configured root.
    List(root string) ([]Info, error)

    // Status returns the working-state summary used by `gss feature
    // list`, `conflicts`, and `checkpoint --auto`.
    Status(path string) (Status, error)
}
```

Notes:

- The interface is **scoped to worktree lifecycle** only. Commits,
  pushes, PR plumbing, and rebases live elsewhere and continue to
  operate on a regular git repo at the worktree path — that contract is
  what every backend must preserve. An overlayfs backend that hides a
  real `.git` directory under the mount must still present `git status`
  / `git log` / `git push` semantics at the worktree path.
- `Info.Backend` is persisted in `registry.json` per worker, so a
  worker created by the `git` backend can still be cleaned up by the
  `git` backend even if the user has since flipped the default to
  `overlayfs`. `gss` looks up the backend from the registry entry, not
  from current config, when operating on an existing worker.
- Backends register themselves in `internal/worktree/registry.go` via
  `worktree.Register("git", New)`. `worktree.Open(name)` returns the
  configured backend. Adding a new backend = one new sub-package + one
  `Register` call.

### v1 implementation: `git` backend

A thin wrapper around `git worktree add/remove/list` and `git status
--porcelain=v2 -b`. No new dependencies. This is the only backend that
ships in v1; everything else stays a `// TODO` doc-only package.

### Future backends (overlayfs, sandboxfs, dockerfs, tmpfs)

Alternative backends — kernel `overlayfs` copy-on-write, Bazel's
Apache-2.0 `sandboxfs`, container/`dockerfs` (with optional full process
isolation), and RAM-backed `tmpfs` scratch trees — are all deferred past
v1.0. The interface above lets us add each as one new sub-package + one
`Register` call without touching callers. Architecture, the per-backend
trade-off matrix, config schema, and the **license gate** (kernel overlay
is fine; the GPL-2.0 `fuse-overlayfs` helper is banned; macFUSE is
review-required) are documented in
[roadmap.md → Worktree backend options](./roadmap.md#worktree-backend-options).

### Selecting a backend

Config (see Configuration): `worktree.backend` picks the default.
`--backend <name>` on `gss feature start` / `worker add` overrides
per-call. Once a worker exists, its `Info.Backend` from the registry
wins; flipping config does not retroactively migrate.

## Configuration

`gss` reads a single YAML config at `~/.config/gss/config.yaml`. All keys
are optional — missing keys take built-in defaults. A `--config <path>`
flag overrides the location. Environment variables `GSS_*` override
individual keys (e.g. `GSS_WORKTREE_ROOT=/tmp/wt`).

Resolution order: built-in default → `~/.config/gss/config.yaml` →
env var → `--flag`.

```yaml
# ~/.config/gss/config.yaml
#
# All keys optional. Shown values are the built-in defaults.

paths:
  worktree_root: ~/.config/gss/worktrees      # parent of <owner>/<repo>/...
  registry_dir:  ~/.config/gss/worktrees      # registry.json sits beside worktrees
  state_dir:     ~/.config/gss                # for approval.token, caches, etc.

worktree:
  backend: git                                # git | overlayfs (future)
  options:                                    # per-backend, free-form map
    # git backend takes no options today.
    # overlayfs backend (future) would take e.g.:
    #   lower_dir: ~/.config/gss/overlay/lower/<owner>/<repo>
    #   upper_root: ~/.config/gss/overlay/upper
    #   work_root:  ~/.config/gss/overlay/work

tools:
  # Only tools gss itself shells out to. Each is an absolute path or a
  # PATH-resolvable name. tmux, git-spice (gs), and jj are intentionally
  # absent — gss does not invoke them. tmux is tmux-mgr's concern; gs / jj
  # are wrapped by an agent skill, not by gss.
  git: git
  gh:  gh

defaults:
  base_branch:   main          # the default_base_branch for new features
  branch_prefix: feature       # branches become <prefix>/<name>/<user>/<purpose>...
  user:                        # override the auto-detected git/gh username
  engine_hint:                 # advisory only; gss does not launch agents

behavior:
  auto_update_refs:        true   # set rebase.updateRefs=true on every worktree
  auto_promote_on_merge:   true   # gss feature merged promotes the single
                                  #   direct child draft→ready when the merged
                                  #   worker has exactly one child. Fan-outs
                                  #   (≥2 direct children) are NOT auto-promoted;
                                  #   gss re-targets bases but leaves promotion
                                  #   to the human (see `feature merged`).
  conflict_scan_on_list:   true   # gss feature list runs `conflicts` and shows overlap
  delete_remote_on_done:   false  # rely on GitHub auto-delete-on-merge instead
  force_with_lease:        true   # use --force-with-lease on rebase pushes

github:
  # `gh` already knows the host from auth; this is for repos hosted off GitHub.com.
  host:                          # default: inferred from `gh auth status`
  default_repo_resolution: gh    # gh | origin | manual

suffixes:
  # Extra suffix words to feed into the random draw. Words must be 3-5
  # lowercase ASCII letters; duplicates of built-in words are silently
  # ignored. This only affects the *pool* draws happen from — the CLI
  # still never accepts a user-supplied --suffix value (see Naming →
  # Suffix wordlist).
  wordlist: []                   # extra words appended to the built-in 256
  wordlist_mode: append          # append | replace
                                 #   append  (default): user's wordlist is
                                 #     added on top of the built-in 256.
                                 #   replace: user's wordlist *is* the pool;
                                 #     the built-in 256 is ignored. Must
                                 #     supply at least 16 words in this mode
                                 #     or gss refuses to start (avoids tiny
                                 #     pools that cause guaranteed collisions).
```

Bootstrapping: on first run, `gss` writes a fully-commented stub of this
file with all defaults if it doesn't exist. `gss config check` validates
the file (and confirms `git`, `gh` are present at the configured paths).
`gss config print` dumps the effective resolved config.

## GitHub interaction — `gh` only

Every interaction with GitHub goes through the `gh` CLI. `gss` never speaks
to the GitHub REST/GraphQL API directly, and it never pushes a PR with raw
`git` if a `gh` verb exists for the job. This keeps auth, retries, and
default-repo resolution consistent with everything else in the dotfiles.

| Operation              | gh verb                                                 |
|------------------------|---------------------------------------------------------|
| Push branch + open PR  | `gh pr create --draft --base <base> --head <branch> --title … --body-file …` |
| Update PR body / base  | `gh pr edit <num> --base <base> --body-file …`          |
| Promote draft → ready  | `gh pr ready <num>`                                     |
| Inspect PR state       | `gh pr view <num> --json state,isDraft,mergeable,…`     |
| List PRs for the repo  | `gh pr list --state all --json …`                       |
| Check out a PR locally | `gh pr checkout <num>` (used by `gss feature resume`)   |
| Comment on a PR        | `gh pr comment <num> --body …`                          |
| Delete remote branch   | `gh api -X DELETE repos/{owner}/{repo}/git/refs/heads/<branch>` (only on explicit `gss feature done --delete-remote`) |

Pre-flight: `gss feature` subcommands that touch GitHub run a one-time
`gh auth status` check and `gh repo set-default` resolution at startup,
caching the resolved `<owner>/<repo>` in the registry. `git push` is still
used for the bare "move commits to origin" case during `checkpoint` /
`rebase` — but PR creation is always `gh pr create` (which pushes the head
ref itself if it isn't on origin yet, so for a *first* checkpoint we skip
the explicit `git push` and let `gh` do it).

## Command surface

### Plain `gss` (unchanged)

`gss push`, `gss pr`, `gss sync` invoked from a normal repo checkout on the
default branch (or any branch that is **not** a registered feature worker
branch) behave exactly as today. No worktree, no registry, no stacking logic.

`gss` distinguishes the two modes by checking whether `pwd` resolves to a
worktree in the registry. If yes → feature mode. If no → classic mode.

### `gss feature start <name> [--purpose <p>] [--base <branch>] [--goal "..."]`

1. Validate name (`[a-z0-9-]+`, no `/`).
2. Reject if a feature with that name already exists in the registry, or if
   `feature/<name>/...` branches exist locally or on origin (stale leftovers
   block reuse until cleaned).
3. Fetch origin; capture `base_commit = origin/<--base or main>` and store
   `default_base_branch` on the feature.
4. Write the feature row + create `<feature>/FEATURE.md` from template.
5. If `--purpose` was given, immediately call `worker add` with that purpose
   so the user lands in a usable worktree.
6. Print feature path (`worktrees/<owner>/<repo>/<feature>/`) and a hint to add more
   workers.

### `gss feature worker add [<feature>] --purpose <p> --description "..." [--base <branch>] [--user <u>] [--suffix] [--goal "..."] [--engine <id>] [--session-id <id>] [--pane-id <id>] [--tmux-mgr-session <id>] [--spawned-by-json <obj>]`

1. Resolve `<feature>` from arg, cwd, or single-feature-in-registry shortcut.
2. Resolve `user` (see Naming → Worker identity), `purpose`, optional
   `suffix`.
3. Resolve `base_branch`:
   - Default: feature's `default_base_branch` (typically `main`).
   - Else: any existing worker branch on the same feature (this is how you
     opt into a stack).
   - `gss` refuses bases outside the feature or `main` (no accidental cross-
     feature stacking).
4. Allocate a unique `(user, purpose, suffix)` tuple, retrying suffix draws
   on collision.
5. `git worktree add <path> -b <branch> <base_branch>`.
6. Write `WORKER.md` from template (resume hint, branch, base, parent worker
   if stacked, goal).
7. Insert into registry.
8. Print the worker's worktree path + branch + base (machine-readable with
   `--json`). gss does not spawn a pane or launch an agent — the caller
   (typically `tmux-mgr`) decides what to do with the path.

### `gss feature list [--feature <name>] [--tree]`

Read registry; show:

- Feature name, started_at, default base branch.
- Workers under each feature, grouped by user.
- Per worker: branch, base_branch, dirty state, last commit, PR URL & state.
- `--tree` renders stack relationships (parents above children, indentation
  for stack depth).

Reconciles registry against `git worktree list` and `gh pr list` to drop
stale entries / refresh PR states.

### `gss feature checkpoint [--message "..."]`

Per-worker; refuses if cwd isn't inside a registered worker worktree.

1. `git fetch origin`.
2. `git rebase origin/<base_branch>` — abort cleanly on conflict; user
   resolves in the worktree.
3. Render PR body from `WORKER.md` + `FEATURE.md` excerpts + auto-section
   (recent commits, files changed, time since last checkpoint) + **stack
   section** (see Stacked PRs below).
4. If `pr_url` empty in registry → `gh pr create --draft --base <base_branch>
   --head <branch> --title <title> --body-file <tmp>` (this pushes the
   branch as a side effect, so we don't need a separate `git push`).
5. Else → `git push --force-with-lease origin <branch>` if the rebase moved
   commits, followed by `gh pr edit <num> --base <base_branch> --body-file
   <tmp>` (the `--base` update matters when a parent merged and we
   re-targeted).
6. After this worker's PR is updated, gss walks the stack and refreshes the
   "Stack" section of every sibling/parent/child PR body via `gh pr edit
   <num> --body-file <tmp>` so cross-links stay accurate.
7. Update registry with PR URL/state (sourced from `gh pr view --json …`).

PR title comes from the first H1 of `WORKER.md` (template seeds
`# <feature>: <purpose>`, e.g. `# parallel-worktrees: ui`).

### `gss feature pr [--ready]`

Promote a draft PR to ready-for-review. By default, gss refuses to promote a
non-bottom PR while its parent is still draft (this would surprise reviewers
who expect the bottom merged first). `--force` overrides.

### `gss feature rebase`

Convenience: rebase the current worker on its current `base_branch` and
update the PR. Useful when a parent worker has new commits but you don't want
a full checkpoint yet.

### `gss feature restack <worker> --onto <branch>`

Manual stack edit: re-target a worker's branch onto a new base. Force-pushes
the worker's branch and updates its PR's `base`. Updates registry. Walks the
stack to fix dependent workers.

**Side effect on auto-promote eligibility**: every call increments the
target worker's `restack_count` by 1 (and increments
`restack_count` on every descendant whose effective base also moved). A
worker with `restack_count > 0` is permanently excluded from
auto-promote-on-merge (see [`gss feature merged`](#gss-feature-merged-worker-ref-or-auto-detect)).
This is the laundering mitigation for the
[stack lifetime invariant](#design-review-deep-pass) (resolution #17):
once you've moved a worker's base after creation, the reviewer's
approval of any future merged ancestor no longer transitively covers
this worker, so promotion to ready must be a human decision.

### `gss feature checkpoint --auto --worker <ref>`

A non-interactive variant designed to be safe to call from a process hook
(such as `tmux-mgr`'s pane-close hook). Behaviour deltas vs. plain
`checkpoint`:

- Takes an explicit `--worker <feature>/<user>/<purpose>[-<sfx>]` so it
  doesn't depend on cwd (the calling shell is often already gone).
- Silent on no-op. If the worktree is clean and the branch is already
  pushed at the latest commit, exits 0 with nothing to do.
- If the worktree is dirty, auto-stages tracked changes only (never
  `git add -A`; respects the same explicit-file rule as classic gss) and
  commits with `chore(wip): auto-checkpoint @ <ISO timestamp>`. Untracked
  files are listed in the WIP commit body but **not** added.
- Never prompts. On any condition that would normally prompt (conflict
  during rebase, merge in progress, detached HEAD, etc.) it skips the
  push/PR step, writes a one-line diagnostic into the worker's
  `WORKER.md` under `## Auto-checkpoint log`, and exits non-zero so the
  caller can surface it.
- Only ever touches **draft** PRs. If the worker's PR is already marked
  ready, `--auto` updates the body but refuses to push new commits
  (avoids surprising reviewers).
- Idempotent: re-running back-to-back is a no-op.

### `gss feature conflicts [--feature <name>] [--json]`

Lightweight cross-worker overlap report. For every active worker in the
given feature (or the cwd-resolved feature, or every feature), gss reads
the worker's working diff (`git diff --name-only <base_branch>...HEAD` plus
`git status --porcelain`) and emits the set of paths touched by more than
one worker:

```
feature parallel-worktrees
  src/registry/load.go
    - eraigosa/api    (4 commits, dirty)
    - eraigosa/ui-moss (1 commit)
  sdk/gss.go
    - eraigosa/api
    - bot42/docs
```

With `--json`, returns a structured object the orchestrator can pipe into
a rebase tool. gss itself does **not** attempt to resolve overlaps — it
just surfaces them. tmux-mgr (or a human) decides whether to invoke a
rebase / restack tool (see Companion rebase tooling below) to keep the
files in sync before merge.

### `gss feature done [<worker-ref>] [--force]`

- With no `<worker-ref>`: removes the current cwd's worker.
- With `<worker-ref>` (e.g. `eraigosa/ui-moss` or `<feature>/<user>/<purpose>`):
  removes that worker.

Refuses if:

- worktree is dirty (unless `--force`);
- PR is open & unmerged (unless `--force`);
- another active worker still has this worker as its `base_branch` (unless
  `--force`, in which case dependents get re-targeted to this worker's
  `base_branch` — i.e. the merge case, but eager).

Steps:

1. `git worktree remove <path>`.
2. `git branch -D <branch>` (local).
3. Remove from registry.
4. **Empty-feature cleanup** — if removing this worker leaves the
   feature with zero workers:
   - In **interactive (classic) mode**, prompt the user with the
     three options (delete row + `FEATURE.md`, retain, retain only if
     `FEATURE.md` was modified).
   - In **non-interactive worker mode** (cwd inside a registered
     worktree, or `--worker <ref>` was passed, or the call came from
     a process hook), gss never prompts. It runs a deterministic
     check: compare the on-disk `FEATURE.md` against the renderer's
     output for the template seeded with the feature's stored
     metadata. If the file is **byte-identical to the template
     output** (no human/agent edits beyond what the template would
     produce), gss removes the feature row + `FEATURE.md`. If the
     file differs, gss **retains** the row + file silently and emits
     a one-line stderr notice naming the orphaned feature and the
     path of the preserved `FEATURE.md`, so a later `gss feature
     list` surfaces it for the human to deal with.
   - The template-comparison check is normalized for trailing
     whitespace and final-newline differences only; any substantive
     edit (added section, modified bullet, prose appended to
     `## Decisions & notes`) wins retention.

### `gss feature audit [--feature <name>] [--repair] [--json]`

Passive scanner that walks the registry and surfaces inconsistencies
between recorded state and reality. Intended both for routine
"is my registry sane?" checks and for recovering from the cross-machine
sync conflicts cross-machine sync can produce (see open-question E
resolution and [`roadmap.md`](./roadmap.md#cross-machine-sync)).

Checks performed (per feature, per worker):

- **Worktree exists**: `stat <worktree_path>` succeeds and contains a
  `.git` file (or backend-appropriate equivalent).
- **Branch exists locally**: `git rev-parse --verify <branch>` succeeds.
- **Branch matches recorded base**: the local branch's merge-base
  against its `base_branch` is reachable (i.e. the worker hasn't
  diverged so far that its base_branch is unreachable).
- **PR exists remotely**: `gh pr view <num>` returns non-404 when
  `pr_url` is set.
- **PR base matches**: the live PR's `baseRefName` matches the
  registry's `base_branch`.
- **No stale child references**: every worker listed as a child
  (i.e. whose `base_branch` is this worker's branch) is itself in
  the registry.
- **`spawned_by` schema valid**: required fields present per the
  Description and provenance rules.
- **`schema_version` matches**: the registry's recorded version is
  what this gss binary supports; older = needs migration, newer =
  refuse to repair, surface only.

Default mode (no `--repair`) is **read-only** — output is a structured
report (`--json` for machine consumers; human-readable table by
default) listing each finding with severity (`info`, `warn`, `error`)
and a suggested remedy. Exit code is non-zero if any `error`-severity
finding exists.

`--repair` attempts deterministic fixes:

- Worker's worktree missing on disk → drop the registry row + emit a
  notice naming the legacy path (in case the user moved it).
- Worker's local branch missing → drop the registry row (worktree is
  unusable without its branch).
- PR URL 404s → clear `pr_url` and `pr_state` in the registry,
  preserve the rest of the worker.
- PR base diverged from registry → update registry to match PR (the
  PR is authoritative since the human edited it).
- Stale child reference → drop the child's `base_branch` to the
  feature's `default_base_branch`, log it.
- Anything that would require a force-push, a branch rename, or any
  modification of `git`'s ref state is **never** done by `--repair`.
  Those cases are reported as `error`-severity and require human
  action via `gss feature restack` or manual git.

`audit` does not call `gh pr create` / `gh pr edit` / `gh pr ready`
ever. It is strictly observational + registry-local repair. The
authoritative-side rule for conflict resolution is: **observable
state wins over registry**. If GitHub says a PR is closed, the
registry's `pr_state: "draft"` is wrong; if the worktree directory is
gone, the registry's row is wrong; etc. The audit tool only ever
makes the registry match observed reality, never the reverse.

### `gss feature merged <worker-ref>` (or auto-detect)

Called explicitly, or invoked by `gss feature list` when it notices a PR
flipped to `merged`. Re-targets every worker whose `base_branch` was this
worker's branch onto **this worker's** former `base_branch` (the merge has
collapsed one stack level). Updates each affected PR's `base` and rewrites
stack sections.

When the merged worker was the **bottom** of a stack (its `base_branch` was
the feature's `default_base_branch` — typically `main`), gss auto-promotes
the next draft to ready **only when both of these hold**:

1. **The stack is linear at that level**: the merged worker had exactly
   one direct child. Fan-outs (≥2 direct children) are mechanically
   re-targeted but never auto-promoted.
2. **The single direct child has `restack_count == 0`**: its
   `base_branch` has been a single, unchanging value since the worker
   was created. Any `restack --onto` against the child (or against a
   transitive parent that altered the child's effective base)
   permanently disqualifies it from auto-promote, even if a later
   restack returns it to its original base. The lifetime invariant is
   what makes auto-promote safe: it guarantees the bottom that the
   reviewer just approved is the same base the child has been built
   on since day one, so the "approve" decision transitively covers
   the child's view of the world.

When both conditions hold, gss flips the single child draft → ready via
`gh pr ready <num>`.

When either condition fails, gss still performs **all the mechanical
re-targeting** — each child's `base_branch` is reset to the merged
worker's former base, the PR's `base` is updated via `gh pr edit`,
stack sections are rewritten — but **does not promote any draft to
ready**. gss emits a one-line stderr notice naming the un-promoted
children with the disqualifying reason (`fan-out` or
`restacked <n> times`) and points at
`gss feature pr --ready --worker <ref>` so the human can ratify
explicitly.

Workers above the next level (grandchildren and below) always stay
draft until their own parent merges. `--no-auto-ready` disables the
auto-promote leg for the call (mechanical re-targeting still runs).

## Stacked PRs

A stack is a chain of workers where each `base_branch` points at another
worker's branch. Stacks form naturally:

```
main
 └─ feature/auth/alice/api          (PR #42 — base: main)
     └─ feature/auth/alice/ui-moss  (PR #43 — base: PR #42's branch)
         └─ feature/auth/bob/docs   (PR #44 — base: PR #43's branch)
```

### PR body — stack section

Every checkpoint rewrites a fenced section in each PR body so reviewers can
navigate the stack without leaving GitHub:

```markdown
<!-- gss:stack-begin -->
## Stack

This PR is part of a stack on **feature/auth**.

- #42 — eraigosa/api (base: `main`) ← parent of this PR
- **#43 — eraigosa/ui-moss (base: `feature/auth/eraigosa/api`)** ← you are here
- #44 — bot42/docs (base: `feature/auth/eraigosa/ui-moss`)

Review bottom-up. Merge bottom-up; gss will re-target the rest of the stack
when a parent merges.
<!-- gss:stack-end -->
```

The marker comments make the section idempotent — gss rewrites between the
markers without touching free-form body content.

### "Main" PR

When the user (or another tool) asks for "the feature PR", gss treats **the
bottom of the stack** (whichever worker has `base_branch = main` /
default_base_branch) as canonical and links it prominently from every other
PR in the stack. The bottom PR's body also lists every descendant PR as a
table for at-a-glance progress.

### Merge flow on the main repo

When the user runs `git rebase` / merge on the main repo (the regular
checkout, not a worktree), they will see branches named
`feature/<name>/<user>/<purpose>[-<sfx>]`. From the branch name alone, the
main-repo workflow can:

- Identify the feature, user, and purpose.
- Look up the worker in the registry (if present locally) to get full
  context.
- Choose to follow the branch's stack (rebase children after parent merges)
  or squash the whole feature flat.

`gss feature merged` automates the "follow the stack" case after a merge
lands.

## FEATURE.md template

Shared across all workers on a feature; consumed by every checkpoint to seed
the feature-level context in PR bodies.

```markdown
# Feature: {{name}}

- **Description**: {{description}}
- **Started**: {{date}}
- **Default base**: {{default_base_branch}}
- **Base commit**: {{base_commit}}

## Goal
{{goal_or_placeholder}}

## Workers
<!-- gss rewrites this list at every checkpoint; one line per worker:
     - {{user}}/{{purpose}}{{suffix?}} — {{description}} ({{spawned_by.engine}}, PR {{pr_url}}) -->

## Decisions & notes
<!-- append freely; surfaced in every worker's PR body -->
```

## WORKER.md template

Per-worker, lives in the worker's worktree.

```markdown
# {{feature}}: {{purpose}}

- **Description**: {{description}}
- **Worker**: {{user}}/{{purpose}}{{suffix?}}
- **Branch**: {{branch}}
- **Base branch**: {{base_branch}}
- **Worktree**: {{worktree_path}}
- **Resume**: `cd {{worktree_path}} && {{spawned_by.engine|default:claude}}`
- **Spawned by**: {{spawned_by.engine}} session `{{spawned_by.session_id}}`
  (tmux pane {{spawned_by.pane_id}}, tmux-mgr session
  `{{spawned_by.tmux_mgr_session}}`) at {{spawned_by.started_at}}

## Goal
{{goal_or_placeholder}}

## Decisions & notes
<!-- append freely; rendered verbatim into PR body -->

## Open questions
```

PR body rendered at checkpoint:

```markdown
<!-- gss:feature-begin -->
{{FEATURE.md Goal + Decisions excerpt}}
<!-- gss:feature-end -->

<!-- gss:worker-begin -->
{{WORKER.md body}}
<!-- gss:worker-end -->

<!-- gss:stack-begin -->
{{stack section, see Stacked PRs}}
<!-- gss:stack-end -->

---

## Auto-generated

- Last checkpoint: {{timestamp}}
- Commits since last checkpoint: {{n}}
- Files changed: {{file_list}}
- Recent commits:
  - {{sha}} {{subject}}
```

## Conflict protection

| Risk                                       | Mitigation                                            |
|--------------------------------------------|-------------------------------------------------------|
| Two worktrees check out same branch        | Prevented by git itself.                              |
| `user/purpose` accidentally reused         | `worker add` checks registry + branches + auto-suffix.|
| Drift from base discovered at merge        | `checkpoint` rebases on `base_branch` first.          |
| Stale `base_branch` after a parent merges  | `gss feature merged` re-targets children.             |
| Two workers edit the same file silently    | `feature list` flags overlap (v1.1).                  |
| Stale registry after crashes               | `feature list` reconciles vs `git worktree list` and `gh pr list`. |
| Plain `gss` accidentally pushes a worker   | gss in classic mode refuses to operate on a branch matching `feature/*/*/*` unless `--allow-feature-branch` is passed. |

## Lifecycle

```
feature start ──► worker add ──► (work) ──► checkpoint ──► [draft PR]
                       │                          │
                       │                          └──► checkpoint ──► [draft PR updated]
                       │                                  │
                       └──► worker add (stacked) ──► checkpoint ──► [draft PR stacked on parent]
                                                          │
                                                          └──► pr --ready ──► [ready PR]
                                                                  │
                                                                  └──► (review, merge bottom-up on GitHub)
                                                                          │
                                                                          └──► feature merged ──► [stack re-targeted]
                                                                                  │
                                                                                  └──► feature done ──► [worker pruned]
```

A feature can sit at "draft PRs" indefinitely. The PR bodies are the durable
record. Resume = `cd` to worker worktree (path is in PR body) + open agent.

## How `tmux-mgr` uses `gss`

`gss` exposes a small, stable, scriptable surface; `tmux-mgr` is the
expected primary consumer but the surface is generic.

The expected `tmux-mgr` flow:

1. User asks `tmux-mgr` to start an agent on a new worker for a feature.
2. `tmux-mgr` shells out to `gss feature worker add … --json` and reads
   the returned worktree path + branch + base.
3. `tmux-mgr` decides which agent CLI to launch (`claude` / `antigravity` / …)
   — engine detection, env, parent-process inspection are entirely on
   the `tmux-mgr` side.
4. `tmux-mgr` spawns a pane at the worktree path, runs the chosen agent
   CLI, and remembers the pane ↔ worker mapping.
5. When the pane / session closes, `tmux-mgr`'s close hook fires:
   `gss feature checkpoint --auto --worker <feature>/<user>/<purpose>[-<sfx>]`.
6. Periodically (or on demand) `tmux-mgr` calls
   `gss feature conflicts --json` and surfaces overlapping files to the
   user, optionally invoking a rebase tool (see Companion rebase tooling).

`gss` does not import, link, or shell out to `tmux-mgr`. It does not detect
engines. It does not own pane state. This keeps `gss` usable from a plain
shell, from a slash command, or from any other orchestrator — and keeps
`tmux-mgr` free to evolve its own multi-pane / multi-engine logic without
changing `gss`.

## Interaction model: worker mode is non-interactive

The worktree flows in this design exist to serve **tmux-mgr agent workers**.
Workers are autonomous agent CLIs running in their own panes; there is no
human attached to the worker's stdin/stdout to answer prompts in real time.
So every command that runs inside a worker worktree is **non-interactive by
default**:

- No "are you sure?" confirmation prompts.
- No "type the message" editors opening unless an explicit
  `--message <text>` was provided.
- No "select an option" pickers.
- No `gh auth login` flow triggers (gss surfaces an auth failure as a
  non-zero exit + structured error instead).

This applies to `gss feature checkpoint`, `pr`, `rebase`, `restack`,
`merged`, and `done` when invoked **from within a registered worker
worktree** (or with `--worker <ref>`). Push, PR creation, draft → ready
promotion, and remote re-targeting all happen without human gating —
the agent is trusted to have built the right state, and the PR being a
**draft** is the safety net for human review later.

Classic `gss` (invoked from a regular checkout on the default branch)
keeps its existing safety prompts and confirmation flow exactly as
today — that path *is* the human-in-the-loop path and the
[[git-safe-sync]] philosophy still owns it. The mode switch is the
same one used to choose classic vs. feature behaviour: cwd inside a
registered worker worktree → non-interactive worker mode; otherwise →
classic interactive mode.

Implications:

- `--auto` is **not** a separate behaviour for worker checkpoints; it
  becomes a tighter variant (silent-on-no-op, never even touches the
  PR body if nothing changed). Plain worker-mode `checkpoint` is
  already non-interactive.
- `gss` worker-mode commands always emit machine-readable progress
  on stderr (line-prefixed) and JSON on stdout when `--json` is set,
  so `tmux-mgr` and the agent can parse outcomes deterministically.
- Anything that genuinely requires a human (a rebase conflict it
  can't resolve, an auth refresh, a force-push refusal) **fails fast
  with a structured error** and leaves the worker in a state a human
  can pick up by `cd`-ing in. It never blocks waiting.

## tmux-mgr refactor plan

Today's `tmux-mgr` (at `sdk/tmux-mgr/`) owns its own worktree creation
under `~/.config/tmux-mgr/worktrees/<session-id>` and shells out to
`git worktree add` directly (`pkg/workspace/workspace.go:24–58`). Once
`gss` ships, that responsibility moves to `gss feature worker add` and
`tmux-mgr` gains a pane-close hook for auto-checkpoint. This section
captures the concrete deltas so the refactor can be scheduled.

### What goes away

1. **`pkg/workspace/`** is deleted. `gss feature worker add` becomes the
   single entry point for worktree creation.
2. **Branch auto-naming inside tmux-mgr** (currently
   `<agentName>-<timestamp>`) is removed. Branches are now
   `feature/<name>/<user>/<purpose>[-<sfx>]` exclusively, generated by
   `gss`.
3. **`currentRepoRoot()` in `cmd/agent.go`** is removed; `gss` resolves
   repo identity via `gh repo view`.
4. **Direct `git worktree remove`** in `runAgentCleanup()` is replaced
   by `gss feature done --worker <ref>`.
5. **`~/.config/tmux-mgr/worktrees/`** ceases to exist on new
   installs. Existing entries are migrated (see below) or treated as
   obsolete.

### What changes

1. **`cmd/agent.go` `runAgentStart()`**:
   - Accept new flags: `--feature <name>` (or auto-create) and
     `--purpose <p>` (defaults to the agent definition's name).
   - Replace `workspace.CreateWorkspace(sessionID)` with a shell-out:
     ```
     gss feature worker add <feature> \
       --purpose <p> \
       --description "<task-description>" \
       --user <u> \
       --engine <claude|antigravity|…> \
       --session-id "$CLAUDE_SESSION_ID"  # or $ANTIGRAVITY_SESSION_ID \
       --pane-id "<tmux pane id>" \
       --tmux-mgr-session "<session record id>" \
       --json
     ```
     The agent definition's `--task-description` becomes the worker
     `description` (truncated to 240 chars). Engine/session-id sourcing
     stays inside tmux-mgr — `DetectHost()` already decides the engine
     and can read the engine-native session env var from the parent
     shell.
   - Read `worktree_path`, `branch`, `base_branch`, and `worker_ref`
     (`<feature>/<user>/<purpose>[-<sfx>]`) from the JSON.
   - Persist `worker_ref` as the canonical identifier in the tmux-mgr
     session file.

2. **`pkg/agent/session.go` `Session` struct**:
   - Drop `RepoRoot` (gss handles repo identity).
   - Add `WorkerRef string` — the canonical gss worker identifier.
   - `WorktreePath` and `PaneID` stay; `WorktreePath` is now sourced
     from gss rather than synthesised.
   - Storage stays at `~/.config/tmux-mgr/sessions/<id>.json`. A
     tmux-mgr Session and a gss Worker are different concepts that map
     one-to-one; both records can coexist.

3. **`cmd/agent.go` `runAgentCleanup()`**:
   - Read `WorkerRef` from the session JSON.
   - Call `gss feature done --worker <WorkerRef>` (forward `--force`).
   - Continue to `tmux kill-pane` and remove `<id>.json`.

4. **`cmd/agent.go` `DetectHost()` / `pkg/agent/model.go`**: unchanged.
   Engine detection (Claude vs Antigravity, model tier selection) stays in
   `tmux-mgr` by the boundary rule.

5. **`pkg/tmux/tmux.go` `CreatePane()`**: minor — set
   `TMUX_MGR_WORKER_REF=<ref>` in the pane environment so the close-hook
   wrapper can find the worker without parsing tmux options.

### What's new

1. **Pane-close auto-checkpoint** (absent in `tmux-mgr` today). Two
   options; pick one:
   - **(A) Wrapper command — recommended**. `tmux-mgr` launches the
     agent CLI through a shim:
     ```
     tmux-mgr internal pane-wrap --worker-ref <ref> -- <agent-cli-cmd>
     ```
     The shim execs the agent and, on its exit, runs
     `gss feature checkpoint --auto --worker <ref>` synchronously,
     forwarding the exit code. Localised; no global tmux config change.
   - **(B) Global tmux `pane-died` hook**. `set-hook -g pane-died 'run
     "tmux-mgr internal on-pane-died #{pane_id}"'`. Requires intrusive
     global tmux config; only worth it if (A) misses user-initiated
     kills.
   Ship (A); document (B) as a fallback.

2. **`tmux-mgr feature start <name>` convenience verb**:
   - Calls `gss feature start <name>` and, if `--purpose <p>` was
     given, also runs `agent start` against the new worker.
   - Becomes the canonical "start a feature with one agent" path.

3. **`tmux-mgr feature add-agent <feature> --purpose <p>`**:
   - Shells out to `gss feature worker add`.
   - Spawns a new pane at the returned worktree path with the resolved
     engine.
   - Records the pane → worker mapping in
     `~/.config/tmux-mgr/sessions/`.

4. **`tmux-mgr feature status [<name>]`** (light, optional):
   - Reads `gss feature list --json` and `gss feature conflicts --json`.
   - Annotates with pane liveness from `tmux list-panes`.
   - One-shot "what's happening across this feature?" view.

5. **One-shot migration (`tmux-mgr internal migrate-to-gss`)**:
   - Full one-way migrator per open-question F resolution. Single
     run, no rollback. Always supports `--dry-run` which prints every
     planned action without executing.
   - For every legacy session at `~/.config/tmux-mgr/sessions/*.json`:
     a. Read worktree path, agent name, branch name, repo root.
     b. `git -C <path> rev-parse HEAD` → captured for the new registry
        row's `base_commit`.
     c. `git -C <path> remote get-url origin` → resolve NWO via
        `gh repo view` for the canonical `<owner>/<repo>`.
     d. Derive a synthetic feature name: kebab-case of the legacy
        branch's pre-timestamp segment, prefixed `migrated-` if it
        collides with an existing feature. The legacy
        `<agentName>-<timestamp>` decomposes to
        `feature = migrated-<agentName>` (or just `<agentName>` if
        unique) and `purpose = "main"` for the single migrated worker.
     e. Compose `spawned_by` with `engine: "manual"`,
        `tmux_mgr_session: <legacy-session-id>`,
        `started_at: <legacy session file mtime>`. `session_id` and
        `pane_id` are recorded as `"migrated"` literal strings (the
        original engine session is gone by definition).
     f. Write the new gss registry row, `FEATURE.md`, and `WORKER.md`
        under `<worktrees_root>/<owner>/<repo>/<feature>/<user>/<purpose>/`.
        `<user>` resolves via gss's normal identity rules.
     g. Rename the legacy branch with
        `git -C <path> branch -m <old> feature/<feature>/<user>/<purpose>`,
        then `git worktree move <legacy-path> <new-gss-path>`.
     h. Update the legacy `~/.config/tmux-mgr/sessions/<id>.json` in
        place: add a `WorkerRef` field, replace `WorktreePath` with
        the new path, drop `RepoRoot`. The legacy file stays so the
        running tmux pane (if any) can still find its worker.
     i. On any per-session failure, log the session id and the
        failure reason, skip it, and continue. Migration is
        all-best-effort within a deterministic order; partial
        migrations are explicitly OK.
   - After every session is processed, `migrate-to-gss` prints a
     summary table (`<legacy-id>  →  <worker-ref>  (status)`) and
     writes the same summary to
     `~/.config/gss/migrate-to-gss.log` for audit.
   - Idempotent: re-running on an already-migrated session reads its
     updated `WorkerRef` and skips it.

### What stays the same

- `session`, `window`, `pane`, `save`, `restore`, `capture` verbs that
  don't touch worktrees.
- `pkg/tmux/tmux.go` pane creation and layout management.
- `pkg/agent/model.go` model tier selection.
- Engine / host detection logic.
- The skill file's overall shape; it just stops talking about
  worktrees as a tmux-mgr concern.

### Skill doc updates

There are two distinct skill audiences and they should not be conflated:

1. **Orchestrator agents** (running outside any worker; using
   `tmux-mgr` to spawn workers) → `sdk/tmux-mgr/skill/SKILL.md`.
2. **Worker agents** (running *inside* a worker worktree, driving the
   actual code change) → `sdk/gss/skill/SKILL.md`.

The boundary mirrors the code dependency: skill files don't leak
implementation across the boundary, just like the tools don't.

#### `sdk/tmux-mgr/skill/SKILL.md` — orchestrator surface only

The orchestrator skill should **not** mention `gss` at all. From the
orchestrator's point of view, `tmux-mgr agent start` produces a pane
with an agent running in an isolated working directory; how that
directory is materialised (git worktree, overlayfs, future backends)
is an internal detail of `tmux-mgr` and its dependencies. Leaking
`gss feature worker add` into this skill would force every
orchestrator agent to learn two tools to do one job, and would couple
the skill file to a refactor that's invisible from the orchestrator
seat.

Concrete edits:

- Replace today's "tmux-mgr creates an isolated git worktree" wording
  with "tmux-mgr provides each agent an isolated working directory."
  No mention of `git worktree`, no mention of `gss`.
- Drop any reference to `~/.config/tmux-mgr/worktrees/` paths if any
  exist in the skill today; replace with "the working directory
  `tmux-mgr` returns from `agent start`".
- Document the auto-checkpoint behaviour as observable orchestrator-
  side fact: "when an agent's pane closes, any uncommitted work is
  saved to a draft PR automatically; the orchestrator does not need to
  do anything to make this happen." No mention of `checkpoint --auto`
  or `gss`.
- Add a one-line pointer for orchestrators that need to inspect work
  across panes: "use `tmux-mgr feature status` for an aggregate view"
  — without explaining that it shells out to `gss feature list`.

If an orchestrator agent ever needs deeper plumbing (rare — they
shouldn't), they can read the gss skill or the design doc explicitly.
The default skill stays narrow.

#### `sdk/gss/skill/SKILL.md` — worker / power-user surface

This is the file that documents gss for the agent driving the work
inside a worker worktree, plus for humans using gss directly from a
plain checkout. It covers:

- Classic `gss push / pr / sync` on the default branch (existing
  behaviour, unchanged).
- Worker-mode rules: non-interactive, draft PRs as checkpoints, when
  to call `checkpoint`, `rebase`, `restack`, `conflicts`, `pr
  --ready`, `done`.
- Stacked PR mental model: parent / child base branches, the bottom
  PR is canonical, merge bottom-up.
- Description hygiene: keep `--description` current via `worker
  update` as scope sharpens; it's what every cross-feature view shows.
- When to reach for the companion `git-spice` skill (rebase aborted,
  `conflicts` reports overlap) and when to escalate to `jj`.

The gss skill does mention tmux-mgr — but only in the "if you got
here from a tmux-mgr-spawned pane, here's what's already true about
your environment" sense, not as a thing the worker calls.

### Why split the skills

- **One tool, one mental model per skill file.** Agents are easier to
  steer when each skill is internally coherent.
- **Refactor immunity.** When the gss/tmux-mgr internals change (new
  worktree backend, new auto-checkpoint mechanism, anything else),
  only one skill file changes. The orchestrator skill stays stable
  for as long as `tmux-mgr agent start` keeps its observable contract.
- **Audit and review.** A reviewer evaluating tmux-mgr changes
  shouldn't need to load gss context to understand the skill update,
  and vice versa.

### Order of work

1. Land `gss` v1 (this design doc) — `worker add`, `checkpoint --auto`,
   `done`, and `list --json` must exist before the refactor starts.
2. Implement the pane-wrap shim and `tmux-mgr internal pane-wrap`.
3. Swap `runAgentStart()` and `runAgentCleanup()` to delegate.
4. Delete `pkg/workspace/` and dead helpers (`currentRepoRoot`,
   timestamp branch naming).
5. Add the new `tmux-mgr feature *` convenience verbs.
6. Update `SKILL.md`.
7. Run `tmux-mgr internal migrate-to-gss` once on the developer's
   machine.

The migration is one-way: once tmux-mgr delegates, there is no fallback
to the old internal worktree code, by design.

## Code layout

`gss` lives under `sdk/gss/` in the dotfiles repo, alongside `tmux-mgr`,
and is built in Go (matches `tmux-mgr` and gives us a single static
binary).

The CLI layer follows Cobra's idiomatic structure: every cobra command
lives under `cmd/gss/`, mirroring the user-visible subcommand tree. No
`internal/cli/` package and no separate `root.go` — `main.go` owns the
root command directly. The `cmd/gss/**/*.go` layer is **command wiring
only** (flag parsing, args validation, calling into `internal/`); all
business logic lives under `internal/`.

### Existing features that must survive the refactor

The current `gss` binary already ships these top-level commands and
safety primitives. **None are removed**; every one is ported into the
new layout below — only their *file location* and the *package they
import from* change. Functional behaviour, flags, exit codes, and
on-disk paths (`~/.config/gss/approval.token`, `backup/gss-TIMESTAMP`
local branches) are preserved.

| Existing command | Behaviour to retain                                              | Today's file (pre-refactor) | New home                                  |
|------------------|-------------------------------------------------------------------|-----------------------------|-------------------------------------------|
| `gss status`     | Show working-tree dirty files (`git status --porcelain`).         | `sdk/gss/cmd/status.go`     | `cmd/gss/status.go` + `internal/status/`  |
| `gss push`       | Backup branch → sync (rebase) → push → auto-PR on feature branch. **Approval-token gated**; `--force-autonomous` bypass retained. | `sdk/gss/cmd/push.go`       | `cmd/gss/push.go` + `internal/classic/push.go` (uses `internal/approval`, `internal/backup`, `internal/sync`, `internal/gh`) |
| `gss sync`       | Fetch origin, `git pull --rebase`, surface conflicts.             | `sdk/gss/cmd/sync.go`       | `cmd/gss/sync.go` + `internal/sync/`      |
| `gss backup`     | Create `backup/gss-TIMESTAMP` local safety branch.                | `sdk/gss/cmd/backup.go`     | `cmd/gss/backup.go` + `internal/backup/`  |
| `gss scan [dir]` | Walk filesystem, list `[DIRTY]` repos via `git status --porcelain`. | `sdk/gss/cmd/scan.go`     | `cmd/gss/scan.go` + `internal/scan/`      |
| `gss pr`         | If on default branch, create `feature/gss-YYYYMMDD-HHMMSS` + push + open PR. **Approval-token gated**. | `sdk/gss/cmd/pr.go`     | `cmd/gss/pr.go` + `internal/classic/pr.go` |
| `gss diff`       | Show staged + unstaged diff.                                      | `sdk/gss/cmd/diff.go`       | `cmd/gss/diff.go`                          |
| `gss version`    | Print version / commit / dirty / build date; `--json` supported.  | `sdk/gss/cmd/version.go`    | `cmd/gss/version.go` + `internal/version/` |

**Safety primitives that must survive verbatim** (extracted from
today's `cmd/push.go` and `cmd/pr.go` into their own packages so
both the classic flow *and* anywhere else in `gss` can call them):

- **Approval token handshake** (`~/.config/gss/approval.token` —
  contains the HEAD SHA, verified-then-consumed by `gss push` and
  `gss pr`). Ported into `internal/approval/`.
- **`--force-autonomous` bypass flag** on `push` / `pr`. Retained
  with **unchanged behaviour on classic checkouts** (bypasses the
  approval-token gate so CI / autonomous flows still work). Two
  scope rules:
  - **Not propagated** to any `gss feature *` command — worker mode
    is already non-interactive by design and has no approval gate
    to bypass.
  - **Refused inside a worker worktree** with `ErrWrongMode` (per
    resolution #22). Pre-existing CI scripts running outside worker
    worktrees see no change.
- **Pre-push backup branch** (`backup/gss-TIMESTAMP`). Ported into
  `internal/backup/` and called automatically before `push`.
- **Rebase-by-default sync** (`git pull --rebase` rather than merge).
  Ported into `internal/sync/`.
- **Feature-branch auto-PR detect/create** via `gh pr view` / `gh pr
  create --fill`. Lives in `internal/classic/push.go` calling
  `internal/gh/`.

**Slash-command wrappers** at `ai/claude/commands/gss.md`,
`ai/claude/commands/sync.md`, `ai/claude/commands/gss-pr.md`,
`ai/claude/commands/gss-scan.md` continue to work unchanged — they
shell out to the `gss` binary by name, and the new binary preserves
every command name and flag they invoke. The skill file at
`sdk/gss/skill/SKILL.md` is updated only to add the new `feature`
verbs; classic behaviour stays as documented.

**Build / distribution** is preserved:
- `sdk/gss/VERSION` stays as the version source-of-truth.
- `sdk/gss/build.sh` still produces the binary; the new layout updates
  the `go build` target to `./cmd/gss` (was `./cmd`).
- Install destination `~/opt/bin/gss` is unchanged.

The new `internal/classic/` package referenced earlier collects the
ported `push` and `pr` orchestration (which used to live inline in
`cmd/push.go` and `cmd/pr.go`) so the cobra leaves stay thin and the
safety primitives can be reused.

```
sdk/gss/
├── docs/
│   └── design.md                       # this document
├── cmd/
│   └── gss/
│       ├── main.go                     # root cobra.Command + main();
│       │                               #   registers classic top-level cmds
│       │                               #   plus the config / feature subtrees.
│       ├── status.go                   # RETAINED: `gss status`
│       ├── push.go                     # RETAINED: `gss push` (approval-gated)
│       ├── sync.go                     # RETAINED: `gss sync`
│       ├── backup.go                   # RETAINED: `gss backup`
│       ├── scan.go                     # RETAINED: `gss scan [dir]`
│       ├── pr.go                       # RETAINED: `gss pr` (approval-gated)
│       ├── diff.go                     # RETAINED: `gss diff`
│       ├── version.go                  # RETAINED: `gss version [--json]`
│       ├── config/                     # package `config` — `gss config …`
│       │   └── config.go               #   print | check
│       └── feature/                    # package `feature` — `gss feature …`
│           ├── feature.go              #   parent cobra.Command + RegisterAll()
│           ├── start.go                #   `gss feature start`
│           ├── worker.go               #   `gss feature worker add|update`
│           ├── list.go                 #   `gss feature list`
│           ├── checkpoint.go           #   `gss feature checkpoint` (+ --auto)
│           ├── conflicts.go            #   `gss feature conflicts`
│           ├── pr.go                   #   `gss feature pr [--ready]`
│           ├── rebase.go               #   `gss feature rebase`
│           ├── restack.go              #   `gss feature restack`
│           ├── done.go                 #   `gss feature done`
│           └── merged.go               #   `gss feature merged`
├── internal/                           # business logic — no cobra here
│   ├── config/                         # YAML loader + env + flag merge
│   │   ├── config.go
│   │   └── config_test.go
│   ├── repo/                           # NWO detection, default-repo cache
│   │   └── nwo.go
│   ├── git/                            # thin wrappers over `git` exec
│   │   ├── exec.go                     # shared exec helper
│   │   ├── branch.go
│   │   └── rebase.go                   # incl. update-refs handling
│   ├── worktree/                       # worktree backend abstraction
│   │   ├── backend.go                  # Backend interface, CreateReq, Info, Status
│   │   ├── registry.go                 # Register / Open (backend lookup)
│   │   ├── git/
│   │   │   ├── git.go                  # v1 default: `git worktree` wrapper
│   │   │   └── git_test.go
│   │   └── overlayfs/
│   │       └── doc.go                  # placeholder for future overlay impl
│   ├── gh/                             # thin wrappers over `gh` exec
│   │   ├── exec.go
│   │   ├── pr.go                       # create/edit/ready/view/list
│   │   └── repo.go
│   ├── registry/                       # registry.json read/write/reconcile
│   │   ├── registry.go
│   │   ├── schema.go                   # Feature, Worker structs
│   │   └── reconcile.go                # vs git worktree + gh pr
│   ├── identity/                       # user/purpose/suffix resolution
│   │   ├── user.go                     # gh login → email slug → $USER
│   │   ├── purpose.go
│   │   ├── suffix.go
│   │   ├── wordlist.go                 # loader + uniform-draw API
│   │   ├── wordlist.txt                # 256 words, go:embed'd
│   │   └── wordlist_test.go            # enforces count=256, len 3-5, unique
│   ├── stack/                          # stacked-PR logic
│   │   ├── stack.go                    # parent/child computation
│   │   ├── body.go                     # PR body rendering w/ markers
│   │   └── restack.go                  # re-target on merge / restack
│   ├── feature/                        # high-level feature orchestration
│   │   ├── start.go
│   │   ├── worker.go
│   │   ├── checkpoint.go
│   │   ├── auto.go                     # --auto path (silent, hook-safe)
│   │   ├── conflicts.go
│   │   ├── merged.go
│   │   └── done.go
│   ├── classic/                        # RETAINED: gss push / pr orchestration
│   │   ├── push.go                     #   backup → sync → push → auto-PR
│   │   └── pr.go                       #   default-branch → feature/gss-* + PR
│   ├── approval/                       # RETAINED: ~/.config/gss/approval.token
│   │   ├── approval.go                 #   write / verify / consume
│   │   └── approval_test.go
│   ├── backup/                         # RETAINED: backup/gss-TIMESTAMP branch
│   │   └── backup.go
│   ├── sync/                           # RETAINED: fetch + pull --rebase
│   │   └── sync.go
│   ├── scan/                           # RETAINED: dirty-repo walker
│   │   └── scan.go
│   ├── status/                         # RETAINED: porcelain status formatter
│   │   └── status.go
│   ├── version/                        # RETAINED: build-time vars (commit/date/dirty)
│   │   └── version.go
│   ├── tmpl/                           # markdown templates
│   │   ├── tmpl.go                     # `//go:embed *.md.tmpl` → embed.FS
│   │   ├── feature.md.tmpl
│   │   └── worker.md.tmpl
│   └── errors/                         # sentinel errors + exit codes
│       ├── errors.go                   # ErrRebaseConflict, ErrAuthRequired,
│       │                               #   ErrDirtyWorktree, ErrLockHeld,
│       │                               #   ErrRegistryStale, ErrNotInWorker,
│       │                               #   ErrPRReadyNeedsToken, ErrInvalidIdent,
│       │                               #   ErrWrongMode, ErrConflictMarker
│       └── exitcodes.go                # stable int map; emitted on os.Exit
├── testdata/                           # fixtures for table-driven tests
└── skill/
    └── SKILL.md                        # agent skill instructions
```

Cobra wiring pattern:

- `cmd/gss/main.go` defines the root `gss` command, calls
  `feature.Register(root)` and `config.Register(root)`, then runs
  `root.Execute()`. No global `var rootCmd` — the root is local to
  `main()`.
- Each subpackage (`cmd/gss/feature`, `cmd/gss/config`) exposes a
  single `Register(parent *cobra.Command)` function. That function
  builds its subtree and attaches it to `parent`. The subpackage is
  responsible for wiring its own leaf commands; `main.go` only knows
  about the top-level groups.
- Leaf command files (`start.go`, `worker.go`, …) each define
  unexported `func newStartCmd() *cobra.Command` etc., called by
  `feature.go`'s `Register`. Flag binding, arg validation, and the
  `RunE` shim live here. The `RunE` body is short: parse flags →
  call into `internal/feature/...` → format output.

Conventions:

- `cmd/gss/main.go` stays under ~50 lines: flag globals + the two
  `Register` calls + `Execute`.
- `cmd/gss/**/*.go` contains **no business logic**. If a function in
  this tree does more than parse flags and call into `internal/`,
  move it.
- Anything that exec's an external tool lives in `internal/git/` or
  `internal/gh/`. Higher layers never `os/exec` directly.
- `internal/feature/*` is the only layer that orchestrates multiple
  subsystems; it depends on `git`, `gh`, `registry`, `stack`,
  `identity`, `worktree`.
- `internal/identity/wordlist.go` uses `//go:embed wordlist.txt` so
  the list is compiled into the binary and ships with no external
  file.
- Tests live next to source files (`*_test.go`); per `src/CLAUDE.md`,
  aim for >60% coverage with table-driven tests for naming, stacking,
  registry reconcile, and PR body rendering. The `cmd/gss/**` layer
  is intentionally thin enough that its own coverage need not hit
  60% in isolation — `internal/` carries the testable weight.

### Pinned external dependencies

Per [src/AGENTS.md → Library standards](../../AGENTS.md), every direct
dep is pinned at design time with its LICENSE blob cited at the
introducing PR. v1 starts with:

| Module                          | License                | Purpose                       |
|---------------------------------|------------------------|-------------------------------|
| `github.com/spf13/cobra`        | Apache-2.0             | CLI framework (already used). |
| `gopkg.in/yaml.v3`              | Apache-2.0 / MIT dual  | `config.yaml` parsing.        |
| `github.com/gofrs/flock`        | BSD-3-Clause           | `registry.json` advisory lock.|
| `golang.org/x/sys`              | BSD-3-Clause           | `unix.Flock`, `syscall.Stat`. |

Anything beyond this list is an additive PR with explicit license citation per
the AGENTS.md process. CI must run `go-licenses check ./...` (Apache-2.0)
as a required step in `build.sh`.

### Test seams (interfaces every external boundary flows through)

To make `go test ./...` green offline and unit-testable per the
TDD-first rule, the following interfaces are pinned. Every leaf
package's `*_test.go` swaps the real implementation for a recording
fake:

```go
// internal/git/exec.go
type Runner interface {
    Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// internal/gh/exec.go
type Client interface {
    PRCreate(ctx context.Context, opts PRCreateOpts) (PR, error)
    PREdit  (ctx context.Context, num int, opts PREditOpts) error
    PRReady (ctx context.Context, num int) error
    PRView  (ctx context.Context, num int) (PR, error)
    PRList  (ctx context.Context, filter PRFilter) ([]PR, error)
    RepoView(ctx context.Context) (RepoInfo, error)
    AuthStatus(ctx context.Context) error
}

// internal/identity/suffix.go
type RNG interface {
    DrawWord() string  // uniform over the embedded wordlist
}

// internal/feature/clock.go
type Clock interface {
    Now() time.Time
}
```

Every `internal/feature/*` orchestrator (`Service` struct) takes
these as constructor parameters; main wires concrete impls;
`*_test.go` wires deterministic fakes. No package in `internal/`
above the wrapper layer ever calls `os/exec`, `time.Now`, or
`math/rand` directly.

### Build-time version

`build.sh` embeds version metadata via `-X` ldflags targeting a
single source of truth: `…/gss/internal/version`. Concrete:

```
go build \
  -ldflags "-X $PKG/internal/version.Version=$(cat VERSION) \
            -X $PKG/internal/version.Commit=$(git rev-parse HEAD) \
            -X $PKG/internal/version.BuildDate=$(date -u +%FT%TZ) \
            -X $PKG/internal/version.Dirty=$(git diff --quiet || echo dirty)" \
  -o $OUT ./cmd/gss
```

`cmd/gss/version.go` is a thin cobra leaf that reads from
`internal/version` and renders. No build vars live in package `main`.

## Companion rebase tooling

Stacked-PR workflows generate non-trivial rebase / restack work:

- After a parent worker rewrites history, every descendant needs its
  base updated (and possibly conflict-resolved against new commits).
- After `gss feature merged`, dependent workers get re-targeted onto the
  next base — git happily lets the branch ref move, but a follow-up
  rebase is usually needed.
- `gss feature conflicts` surfaces overlap; the actual merge of those
  overlaps is still git's job.

`gss` itself does not ship a stack-rebase implementation. Everything below
respects the project's permissive-license policy
(see [src/AGENTS.md → Library standards & license selection](../../AGENTS.md)):
GPL / AGPL / LGPL CLIs are ineligible regardless of feature fit, and
cloud-gated proprietary CLIs are likewise ruled out. The choices are:

1. **Built-in baseline (always on, no deps)**: `gss` sets
   `git config rebase.updateRefs true` in every worktree it creates. Git
   2.38+'s `--update-refs` automatically restacks descendant branches
   during an interactive rebase, which covers the common
   single-stack-restack case with zero external dependencies. Git itself
   ships under GPL-2.0 *as a binary* but we only invoke it as a tool, not
   link or distribute it, so this is consistent with the policy.

2. **Default companion (agent skill wraps it)**:
   [`git-machete`](https://github.com/VirtusLab/git-machete). **MIT
   licensed** ([LICENSE](https://github.com/VirtusLab/git-machete/blob/master/LICENSE)),
   actively released (v3.41.0, May 2026), native GitHub PR integration
   (`git machete github create-pr`, `... retarget-pr`,
   `... checkout-prs`), tree-of-branches model with explicit "definition
   file" that mirrors `gss`'s feature → worker → purpose hierarchy well
   enough to drive. Verbs we'd lean on: `git machete status`,
   `git machete traverse` (interactive restack with conflict prompts),
   `git machete update --no-interactive-rebase`, `git machete fork-point`.
   Small, stable verb set ideal for wrapping as a Claude Code /
   Antigravity CLI plugin skill that the agent invokes when
   `gss feature conflicts` reports overlap or when a rebase aborts.

3. **Rescue mode (opt-in, advanced)**:
   [`git-branchless`](https://github.com/arxanas/git-branchless). **Dual
   Apache-2.0 / MIT** ([LICENSE-APACHE](https://github.com/arxanas/git-branchless/blob/master/LICENSE-APACHE)
   / [LICENSE-MIT](https://github.com/arxanas/git-branchless/blob/master/LICENSE-MIT)).
   Best-in-class `git restack` + `git undo` + event-log smartlog; when a
   multi-level stack rebase produces cascading conflicts that
   `git`/`git-machete` can't unwind linearly, `git branchless undo`
   rewinds to any earlier ref state cleanly and `git restack` reapplies.
   Maintenance caveat: last tagged release was v0.10.0 in Oct 2024,
   though `main` continues to receive commits. Acceptable risk for an
   *optional* companion; we don't depend on it for the common path.

4. **Optional sync layer (smaller cases)**:
   [`git-town`](https://github.com/git-town/git-town). **MIT licensed**
   ([LICENSE](https://github.com/git-town/git-town/blob/main/LICENSE)),
   actively released (v23.0.1, May 2026), GitHub-PR-native. Useful for
   the simpler "keep my feature branches in sync with main" case where
   full stack tooling is overkill. We don't ship a skill for it in v1;
   listed here so it's not rediscovered later as a "missed option".

### Next step

Prototype a thin `git-machete` plugin skill at
`src/git-machete/skill/SKILL.md` that teaches the agent to drive
`git machete status` / `traverse` / `update` when `gss` reports a
rebase abort or `conflicts` overlap. Evaluate `git-branchless` as a
follow-on rescue skill only if `git-machete` proves insufficient.

## Decisions

- **Multi-worker is first-class**: a feature is a 1..N container, not a
  synonym for "one branch".
- **Worker name = `<user>/<purpose>[-<suffix>]`**: human-readable, matches
  trailing branch segments, mechanically unique.
- **Suffix policy**: empty when unambiguous; wordlist draw on collision;
  `--suffix` forces a draw even when not needed but **never accepts a
  user-supplied value** (suffixes always originate from the embedded
  list, making them grep-able and audit-friendly). Wordlist is a
  curated 256-word static list (3–5 letters, lowercase, nature /
  weather / terrain) compiled into `gss` itself via `go:embed`. See
  [Suffix wordlist](#suffix-wordlist).
- **Branch = `feature/<name>/<user>/<purpose>[-<sfx>]`**: branch path mirrors
  worktree path so the main repo can read intent from the branch alone.
- **Stacking via `--base`**: opt-in, scoped to within the same feature or
  `main`.
- **PR cross-links via fenced section**: gss rewrites `<!-- gss:stack-* -->`
  markers idempotently; free-form body content is preserved.
- **Bottom-of-stack is canonical "feature PR"**.
- **Plain `gss` stays plain**: classic mode is invoked any time cwd is not a
  registered worker; feature mode is opt-in via `gss feature …`.
- **PR title**: first H1 of `WORKER.md`.
- **`done` strictness**: refuses while PR is open & unmerged or dependents
  exist; `--force` overrides.
- **`--goal`**: optional at worker creation; warning if missing.
- **Worktree location**: `~/.config/gss/worktrees/<owner>/<repo>/<feature>/<user>/<purpose>[-<sfx>]/`.
- **Push policy**: never implicit. `worker add` does not push. `checkpoint`,
  `pr`, `rebase`, and `restack` are the only push paths.
- **Cleanup policy**: never implicit. Panes and worktrees survive until
  `done` is called.
- **Auto-checkpoint**: opt-in via the tmux-mgr pane close hook calling
  `gss feature checkpoint --auto`. Only touches draft PRs; never prompts.
- **Engine detection fallback**: default to `claude` with a stderr notice
  when env detection fails; `--engine` overrides.
- **Remote branch deletion**: not gss's job. We rely on GitHub's
  *Automatically delete head branches* repo setting to clean the remote
  ref on merge. `gss feature done` only removes the local branch +
  worktree + registry entry.
- **GitHub I/O**: all PR / remote-PR operations go through `gh` (see
  GitHub interaction section above).
- **Repo identity**: keyed by name-with-owner (`<owner>/<repo>`), stored
  as nested directories on disk. Resolved from `gh repo view`, falling
  back to `origin` URL parsing.
- **Configuration**: single YAML at `~/.config/gss/config.yaml`, all keys
  optional. Env overrides via `GSS_*`. See Configuration section.
- **Stack rebase tooling**: not part of `gss`. Baseline is git's own
  `rebase.updateRefs`; companion agent skill wraps `git-spice` for
  cross-branch restack; `jj` documented as rescue mode.
- **Cross-worker overlap detection**: ships in v1 as `gss feature
  conflicts` — a passive report. `gss` does not auto-resolve; the
  orchestrator (typically `tmux-mgr` via the companion skill) decides
  whether to invoke a rebase tool.
- **Auto-promote on merge (linear stacks only)**: when the bottom of
  a stack merges, gss promotes the single direct draft child to ready
  via `gh pr ready` **only if the merge resolves a linear (1-child)
  stack at that level**. Fan-outs (≥2 direct children) get the
  mechanical re-targeting (base branches updated, PR `base` rewritten,
  stack sections refreshed) but are left at draft for a human to
  promote with `gss feature pr --ready --worker <ref>`. Toggleable
  via `behavior.auto_promote_on_merge` or `--no-auto-ready`.
  Rationale: the reviewer's approval of the bottom doesn't transitively
  cover sibling work; with a fan-out, picking the next sibling to
  review is judgement, not mechanics.
- **Empty features are allowed**: `gss feature start` creates a feature
  row with zero workers. Adding workers (and spawning the agent pane
  that lives in them) is `tmux-mgr`'s job; `gss` doesn't insist on a
  first worker because `gss` doesn't know whether the caller wants one
  yet.
- **Scope boundary with tmux-mgr**: `gss` owns git + GitHub + worktrees
  + registry. `tmux-mgr` owns panes, agent processes, engine detection,
  close hooks. Dependency is one-way: `tmux-mgr` → `gss`.
- **Worker description is required**: `gss feature worker add` refuses
  to create a worker without `--description`. The description is shown
  in `feature list`, the PR body, and `WORKER.md`; agents update it via
  `worker update` as scope sharpens. This makes "what is that
  worktree?" answerable from `gss feature list` alone, without opening
  the agent transcript.
- **AI session provenance is captured at create time**: the
  `spawned_by` object (engine, session_id, pane_id, tmux_mgr_session,
  started_at) is persisted **once** and never overwritten, so any
  worktree can be traced back to the AI session that opened it. gss
  does not introspect the engine; the caller supplies the values.
- **Worktree backend is pluggable**: every worktree lifecycle call goes
  through the `worktree.Backend` interface. v1 ships only the `git`
  backend; overlayfs (and other COW schemes) can be added as new
  sub-packages without touching callers. Backend choice is per-worker
  and persisted in the registry, so config flips don't break existing
  workers.
- **`--force-autonomous` refuses inside a worker worktree**
  (resolution #22): classic `gss push --force-autonomous` and
  `gss pr --force-autonomous` are retained verbatim on regular
  checkouts but error with `ErrWrongMode` when invoked from a
  registered worker. Keeps the dangerous-flag semantics
  unambiguous and lets `safety_guard.sh` gate them with a single
  cwd-aware regex.

## Roadmap

### v1.1 — Append-only session history

`spawned_by` records the AI session that *opened* a worker. In practice
a long-lived worker is resumed by many sessions over its lifetime — a
human pops in, a different agent takes over, the original agent comes
back the next day. v1.1 adds an append-only `sessions: [...]` array to
each worker so the full attach/detach history is preserved alongside
the create-time `spawned_by`.

Schema delta:

```json
{
  // existing worker fields (description, spawned_by, branch, …) unchanged.
  "sessions": [
    {
      "engine": "claude",
      "session_id": "c1a2b3c4-...",
      "pane_id": "%17",
      "tmux_mgr_session": "coder-1715890123",
      "attached_at": "2026-05-17T10:34:00Z",
      "detached_at": "2026-05-17T18:12:44Z",
      "reason": "pane-closed"        // pane-closed | session-killed | handoff | unknown
    },
    {
      "engine": "claude",
      "session_id": "d2e3f4a5-...",
      "pane_id": "%21",
      "tmux_mgr_session": "coder-1716044800",
      "attached_at": "2026-05-18T09:20:00Z",
      "detached_at": null,            // currently attached
      "reason": null
    }
  ]
}
```

Rules:

- **Append-only**: gss never edits or deletes a session row once
  written. Only the trailing row's `detached_at` is mutated (from
  `null` to a timestamp) on detach.
- **First row equals `spawned_by`**: the creation session is mirrored
  into `sessions[0]` for uniform iteration; `spawned_by` itself stays
  authoritative for "who opened this worker?" queries so v1.0 readers
  keep working.
- **Caller-driven**: gss does not detect attach/detach itself. The
  orchestrator (typically `tmux-mgr`) calls
  `gss feature worker attach --worker <ref> --engine … --session-id
  …` and `... worker detach --worker <ref> --reason …` at the right
  moments (pane spawn, pane close, explicit handoff).
- **`reason` enum stays small** so it's grep-friendly:
  `pane-closed | session-killed | handoff | unknown`.
- **Display**: `gss feature list --with sessions` adds a column
  showing the count of sessions plus the currently-attached one;
  `gss feature worker history <ref>` dumps the full timeline.
- **Tied to the tmux-mgr refactor**: `tmux-mgr`'s pane-spawn and
  pane-close-hook paths gain the `attach`/`detach` calls as a thin
  extension of the v1 plumbing, so adopting v1.1 is one wiring change
  on the orchestrator side.

This is the only roadmap item presently committed beyond v1; everything
else lives in feature requests rather than the design.

## Design review (deep pass)

The design was reviewed end-to-end by four reviewer personas drawn from
`$HOME/<REPO_ROOT>/.agents/workflows/` — Architect, Security Engineer,
QA Specialist, Lead Developer — surfacing roughly seventy distinct
items across structural integrity, security, testability, and
implementation feasibility. This section captures the **resolutions
adopted into the design** (most landed as inline edits above) and the
**questions still open for human decision**.

### Resolutions adopted in this revision

The following review findings have been resolved directly in the design
above. Each entry names the issue, the resolution, and where the
resolution now lives.

1. **`worker_ref` canonical grammar.** Pinned at
   `<feature>/<user>/<purpose>[-<suffix>]`. Same string everywhere
   (flag, JSON, env var, positional). See [Worker reference](#worker-reference-worker_ref).
2. **Identifier validation grammar.** `<feature>`, `<user>`, `<purpose>`
   constrained to `^[a-z][a-z0-9-]{0,30}[a-z0-9]$`; `<purpose>` MUST
   NOT collide with the wordlist; `--description` is NFC-normalised
   printable Unicode ≤ 240 code points with marker tokens stripped.
   See [Validation grammar](#validation-grammar).
3. **YAML library pinned.** `gopkg.in/yaml.v3` (Apache-2.0/MIT dual).
   See [Pinned external dependencies](#pinned-external-dependencies).
4. **Error contract.** New `internal/errors/` with sentinels
   (`ErrRebaseConflict`, `ErrAuthRequired`, `ErrDirtyWorktree`,
   `ErrLockHeld`, `ErrRegistryStale`, `ErrNotInWorker`,
   `ErrPRReadyNeedsToken`, `ErrInvalidIdent`, `ErrWrongMode`,
   `ErrConflictMarker`) and a stable `exitcodes.go` map. See
   [Code layout](#code-layout).
5. **Test seams.** `git.Runner`, `gh.Client`, `feature.Clock`,
   `identity.RNG` interfaces pinned; orchestrators in
   `internal/feature/*` are `Service` structs taking these as
   constructor params. See [Test seams](#test-seams-interfaces-every-external-boundary-flows-through).
6. **Build-time version.** `-X` ldflag target is
   `…/gss/internal/version.<var>`; `cmd/gss/version.go` is a
   thin reader. See [Build-time version](#build-time-version).
7. **Missing `internal/tmpl/tmpl.go`.** Added to the code layout
   with `//go:embed *.md.tmpl`. See [Code layout](#code-layout).
8. **`spawned_by` is informational only.** Documented to be audit
   data, never the basis for a trust or control decision. See
   [Description and provenance](#description-and-provenance-fields).
9. **Mode detection rule.** When cwd is a registered worker worktree
   and the user runs classic `gss push`/`pr`/`sync`, gss refuses
   with `ErrWrongMode` and points at the equivalent feature command.
   No silent re-routing.
10. **`registry.json` locking & permissions.** Mode `0600`; every
    read-modify-write protected by `flock` on
    `<repo>/.registry.lock`; writes use tmp-file + atomic `rename(2)`;
    refuse to operate if `stat(registry.json).uid != geteuid()`.
    `gofrs/flock` pinned as the BSD-3-Clause lock primitive.
11. **`gss feature pr --ready` is approval-token gated.** Closes the
    security gap where a worker could silently flip a draft to ready.
    See [`gss feature pr`](#gss-feature-pr---ready) (token required
    for `--ready`).
12. **NWO cache pinned.** Lives at `<worktrees_root>/.nwo`; refreshed
    on `gh repo view` cache miss; `--repo` is read-only shadow; cache
    invalidates when `git remote get-url origin` diverges.
13. **`Backend.Enumerate(root)`** added to the worktree backend
    interface so reconcile doesn't hard-code `git worktree list`.
14. **Companion CLI version pin & verb allowlist.** Captured as a
    hardening item — pin a tested version range, install via a
    controlled method, document a verb allowlist, cite LICENSE blob
    SHA in `SKILL.md`, refresh on bump.
15. **Pinned external dependencies & license CI gate.** Table of v1
    deps with licenses; `go-licenses check ./...` required in
    `build.sh`.
16. **Auto-promote-on-merge restricted to linear stacks.** Resolved
    open-question A as option 3: when the merged bottom has exactly
    one direct child, auto-promote that child draft → ready.
    Fan-outs (≥2 direct children) get the mechanical re-targeting
    only; promotion is left to a human via
    `gss feature pr --ready --worker <ref>`. Rationale: the
    reviewer's "approve" on the bottom doesn't transitively cover
    siblings; with a fan-out, picking the next sibling to review is
    judgement, not mechanics. See [`gss feature merged`](#gss-feature-merged-worker-ref-or-auto-detect).
17. **Auto-promote requires a never-restacked worker (stack
    lifetime invariant).** Resolved open-question B as option 2: a
    worker is eligible for auto-promote-on-merge only if its
    `base_branch` has been a single, unchanging value since the
    worker was created. Any `restack --onto` call against the
    worker permanently disqualifies it from auto-promote, even if
    later restacks return it to its original base. This is tracked
    in the registry via a per-worker `restack_count` integer
    (default 0, incremented at every `restack`); auto-promote
    eligibility is `restack_count == 0`. A restack also clears the
    eligibility for the worker's descendants if it changed their
    transitive base. The check runs inside `gss feature merged`
    *before* `gh pr ready` fires — disqualified workers get the
    mechanical re-targeting (their PR base is updated) but stay
    draft, with a stderr notice naming why
    (`worker <ref> restacked N times; promotion left to human`).
    Closes the laundering attack where an agent silently re-points
    a draft at a just-merged base and rides the cascade to ready.
    See [`gss feature merged`](#gss-feature-merged-worker-ref-or-auto-detect)
    and [registry schema](#filesystem-layout).
18. **WORKER.md secret scanning — out of scope for v1.0.** Resolved
    open-question C as option 3: gss does not scan rendered content
    locally; we rely on GitHub's server-side secret scanning as the
    safety net. The eventual strip-and-warn behaviour (and the
    `secrets:` config block that will activate it) is fully specified
    in [`roadmap.md`](./roadmap.md#workermd-secret-scanning) so future
    contributors don't re-litigate the design.
19. **Empty-feature cleanup in worker mode — template-diff comparison.**
    Resolved open-question D as option 3: when `gss feature done`
    removes the last worker on a feature in non-interactive (worker)
    mode, gss byte-compares the on-disk `FEATURE.md` against the
    renderer's output for the original template and stored metadata.
    Identical → remove the feature row + `FEATURE.md`. Different →
    silently retain both, log a one-line stderr notice naming the
    orphaned feature. Whitespace / final-newline differences only do
    not count as edits. Closes the contradiction between the
    non-interactive rule and the "preserve notes" rule. See
    [`gss feature done`](#gss-feature-done-worker-ref---force).
20. **Cross-machine `~/.config/gss/` sync — best-effort + audit tool.**
    Resolved open-question E as options 2+3 combined: cross-machine
    sync remains a non-goal (no host-UUID refusal); gss operates
    best-effort if a synced registry is encountered. To handle the
    conflicts that will arise, v1.0 ships
    `gss feature audit` (see [Command surface](#command-surface)) —
    a passive scanner that reports registry / worktree / PR / branch
    inconsistencies and, with `--repair`, attempts deterministic
    fixes (drop registry rows for missing worktrees, drop PR URLs for
    PRs that 404, refuse repairs that would force-push). The
    "documented expected conflicts" half of the resolution lives in
    [`roadmap.md`](./roadmap.md#cross-machine-sync) (added in the
    same revision).
21. **`tmux-mgr internal migrate-to-gss` — full one-way migrator.**
    Resolved open-question F as option 1: v1.0 ships a complete
    migrator that scans `~/.config/tmux-mgr/sessions/*.json`,
    derives a synthetic feature name from each legacy session,
    resolves NWO via `gh repo view`, composes a `spawned_by` with
    `engine: "manual"`, writes registry rows + `FEATURE.md` +
    `WORKER.md` per session, renames the legacy
    `<agentName>-<timestamp>` branch to
    `feature/<synthetic-name>/<user>/<purpose>`, moves the worktree
    from `~/.config/tmux-mgr/worktrees/<id>` to the new gss path
    via `git worktree move`, and updates the tmux-mgr session JSON
    to reference the new path and worker ref. Estimated 300–500 LOC.
    One-way (no rollback), with a `--dry-run` mode that prints
    every action without performing it for human inspection before
    commit. See [tmux-mgr refactor plan → migration](#tmux-mgr-refactor-plan).
22. **`--force-autonomous` inside a worker worktree errors with
    `ErrWrongMode`.** Resolved open-question G as option 1: classic
    `gss push --force-autonomous` and `gss pr --force-autonomous`
    invoked from a registered worker worktree refuse to run, exit
    with the `ErrWrongMode` exit code, and emit a single-line
    diagnostic naming the worker ref and pointing at the feature
    equivalent (e.g. `gss feature checkpoint --worker <ref>`). Three
    reasons: (a) symmetric with resolution #9 (plain `gss push` from
    a worker already errors); (b) `--force-autonomous` is the wrong
    place for silent semantic shifts — its whole purpose is to be
    unambiguous and audit-friendly; (c) `safety_guard.sh` can
    pattern-match the dangerous shape with one new regex rather
    than encoding mode-aware rewrite logic. CI / autonomous scripts
    that previously ran `gss push --force-autonomous` against the
    main checkout continue to work unchanged; the same script
    running in a worker worktree fails loudly so the engineer can
    `cd` out or replace the line with `gss feature checkpoint
    --auto`. See [Plain `gss` (unchanged)](#plain-gss-unchanged)
    and the hardening checklist below for the safety-guard
    extension.

### Open questions awaiting human decision

None. Every open question (A–G) is now resolved; the design is
decision-complete and ready to drive `plan.md` (see [Next
steps](#next-steps)).

### Pre-v1 hardening checklist

The following are commitments, not open questions. v1.0 tag is
blocked until each is checked.

- [ ] Extend `ai/hooks/safety_guard.sh` + `safety_guard_test.sh`
      to gate `gss feature pr --ready`, `gss feature merged`,
      `gss feature restack`, (outside worker-mode) `gss feature
      checkpoint`, and `gss (push|pr) --force-autonomous` invoked
      from a registered worker worktree (the resolution-#22 refusal
      path). Per `CLAUDE.md`, add ≥1 `assert_exit 0` and ≥1
      `assert_exit 2` case per new pattern before merging.
- [ ] Implement the approval-token gate for `gss feature pr --ready`
      (the open-question B decision may add `restack` here).
- [ ] Pin `git-machete` version range and verb allowlist in
      `src/git-machete/skill/SKILL.md`. Cite LICENSE blob SHA at the
      pinned version. Repeat for `git-branchless` if shipped.
- [ ] Add `go-licenses check ./...` as a required `build.sh` step
      and CI job; decline merges that introduce a banned license.
- [ ] Implement `internal/registry/lock.go` with `gofrs/flock` and
      CAS via `schema_version`; reject mismatched schema versions.
- [ ] Enforce input validation regex for `--purpose`, `--user`,
      `--feature`, `--description` on every call site (flag, JSON,
      env). Reject on first violation with `ErrInvalidIdent`.
- [ ] PR-body marker-escaping pass: strip `<!-- gss:* -->` token
      sequences from user-authored `WORKER.md` and `FEATURE.md`
      before stitching the PR body.
- [ ] ANSI / control-character stripping for any output that hits
      stdout (`feature list`, error messages) — defence against
      terminal injection via `--description`.
- [ ] Commit golden-output snapshot fixtures for every retained
      classic command (`status`, `push`, `sync`, `backup`, `scan`,
      `pr`, `diff`, `version`) at `testdata/golden/classic/` *before*
      the port begins. Add a "Stable output strings" subsection
      pinning the substrings the slash commands grep against.
- [ ] Implement deterministic test seams (`Runner`, `Clock`, `RNG`,
      `gh.Client`) with fakes under each leaf package's `*_test.go`
      before the orchestrators are written.
- [ ] Add `Backend.Enumerate` to the worktree interface + a
      contract-test suite at `internal/worktree/backendtest/contract.go`
      that every backend must pass.
- [ ] Document the JSON error envelope
      (`{"error": {"code": "...", "message": "...", "worker": "...",
      "details": {...}}}`) and the exit-code table; pin both in
      `internal/errors/`.
- [ ] Add `--dry-run` to `gss feature checkpoint --auto` so the
      pane-close hook is end-to-end testable without `exec`.
- [ ] Permissions audit: `~/.config/gss/registry.json` and
      `~/.config/gss/approval.token` mode `0600`; refuse to operate
      on uid mismatch.
- [ ] Multi-process race tests: `TestConcurrentWorkerAdd`,
      `TestDoneRacingCheckpoint`, `TestMergedCascadesTwoLevels`.
- [ ] Edge-case enumeration: worker with no commits since base;
      force-pushed parent; network drop mid-`gh pr create`; APFS
      case-folding (`feature/UI` vs `feature/ui`); branch already
      on origin from a prior aborted run; `gh` rate-limit during
      reconcile; broken `.git` link in a registered worktree;
      WORKER.md / FEATURE.md edited externally between checkpoints.
      Each gets a test name pinned in design before implementation.

### TDD implementation order

Lands bottom-up; each step unblocks the next. Adopted from the
Lead-Developer review.

1. `internal/errors/` — sentinels + exit codes.
2. `internal/git/exec.go` + `internal/gh/exec.go` — `Runner` /
   `Client` interfaces + recording fakes.
3. `internal/version/` — single source of `-X` ldflag vars.
4. `internal/config/` — YAML + env + flag merge.
5. `internal/identity/` — wordlist + suffix + user/purpose resolution.
6. `internal/approval/`, `internal/backup/`, `internal/sync/`,
   `internal/scan/`, `internal/status/` — port classic primitives
   one by one with golden snapshots.
7. `internal/registry/` with `flock`.
8. `internal/worktree/` (git backend + `backendtest/contract.go`).
9. `internal/classic/push.go` + `pr.go` — orchestrate ported
   primitives; flip `cmd/gss/push.go` and `cmd/gss/pr.go` to the
   new entrypoints at the same time.
10. `internal/stack/` — PR body markers + restack math.
11. `internal/tmpl/` with `tmpl.go` embed.
12. `internal/feature/` — `Service` structs with injected deps.
13. `cmd/gss/feature/` — cobra wiring.
14. tmux-mgr `pane-wrap` shim + `runAgentStart` / `runAgentCleanup`
    swap; delete `pkg/workspace/` *after* migrate-to-gss runs.
15. `tmux-mgr internal migrate-to-gss` per open-question F.

### Escalations (flagged, non-blocking)

Items where a deeper investigation is warranted but which do not
block v1:

1. **Cross-machine sync handling** (open question E).
2. **`gh pr ready` ↔ GitHub branch-protection auto-update**
   interaction during the auto-promote cascade — opaque from
   outside; will require a prototype to characterise.
3. **Overlayfs backend ↔ git `.git` assumptions** — the announced
   `Backend` interface may need to grow before overlayfs is
   actually viable; defer until the prototype tells us so.
4. **PR body marker collisions with third-party tools** (Renovate,
   Conventional Comments, Linear) — investigate before they're
   reported as bugs.
5. **Formal threat model for the autonomous worker as adversary** —
   current model treats the agent as semi-trusted; an explicit
   threat model would let us reason about the
   `pr --ready` gate, `restack` laundering, and secret-scanning
   trade-offs as a coherent whole rather than ad-hoc rules.

## Next steps

The design is **decision-complete** — all 22 review resolutions are
recorded (see [Resolutions adopted in this revision](#resolutions-adopted-in-this-revision))
and the [Open questions](#open-questions-awaiting-human-decision)
queue is empty. The prompt below is the **canonical instruction**
that turns this design into a working v1.0. It is self-contained:
pasted verbatim into any fresh Claude Code session (or said as a
single turn here) it should drive the rest of the work without
further setup.

The prompt is staged so you can pause between phases — say
"write the plan" to get only Phase 1+2, then review `plan.md`, then
say "execute the plan" to trigger Phase 3.

The plan-writer **must absorb every resolution from the Design
review section** as concrete deliverables. The Phase 1 instructions
below name the resolution-driven deliverables that the design now
requires beyond the bare TDD implementation order, so the plan
cannot accidentally drop them.

### Execution prompt (verbatim, ready to copy)

> **Goal**: deliver `gss` v1.0 from this design, as rapidly as is
> compatible with human-reviewable stacked PRs and a working test
> suite at every step.
>
> **Source of truth**: `$HOME/<REPO_ROOT>/sdk/gss/docs/design.md`.
> Every PR description and every plan entry must cite the design
> section(s) it implements (anchor + line range) so the reviewer can
> trace code to intent without re-reading the design.
>
> **Reviewer personas**: agent definitions live at
> `$HOME/<REPO_ROOT>/.agents/workflows/`
> — `the_architect.md`, `the_lead_developer.md`,
> `the_qa_specialist.md`, `the_security_engineer.md`,
> `the_devops_engineer.md`, `the_tech_writer.md`, `the_planner.md`,
> `the_captain.md`, `the_researcher.md`,
> `the_observability_member.md`. Subagents adopt these personas by
> reading the relevant file first.
>
> **Concurrency rule**: at most **5** subagents may be in flight at
> once. Background long-running agents with `run_in_background: true`
> so the orchestrator stays responsive. The orchestrator session
> itself does not count toward the 5.
>
> ---
>
> #### Phase 1 — Write the plan
>
> Author `$HOME/<REPO_ROOT>/sdk/gss/docs/plan.md`. It must:
>
> 1. Translate every step of the design's
>    [TDD implementation order](#tdd-implementation-order) (steps
>    1–15) into one or more **deliverables**, each sized to land as
>    a single stacked PR reviewable in ≤ 20 minutes.
> 2. For each deliverable, pin:
>    - **Title** (one line, prefixed by step number, e.g. `02a:
>      internal/git Runner interface`).
>    - **Design sections implemented** (anchor + line range).
>    - **Files added / modified** (precise paths).
>    - **Tests added first** (TDD-first per `src/CLAUDE.md`).
>    - **Acceptance criteria**: tests pass, coverage ≥ 60% for
>      the touched package, golden snapshots (where applicable)
>      unchanged or explicitly updated with a note, manual smoke
>      step.
>    - **Upstream PR(s) it stacks on** (the immediate parent).
>    - **Reviewer personas** assigned (one or more from the list
>      above). At minimum: Lead Developer for code, QA for tests,
>      plus Security for hardening-checklist items and Architect
>      for new packages or boundary changes.
> 3. Absorb every item from the
>    [Pre-v1 hardening checklist](#pre-v1-hardening-checklist) into
>    an explicit deliverable. No checklist item may be implicit or
>    dropped.
> 4. **Absorb every Design-review resolution (#1–#22)** as a
>    concrete deliverable or as an explicit cross-reference to a
>    deliverable that already covers it. The following resolutions
>    are **load-bearing for v1.0** and the plan MUST name at least
>    one deliverable for each — call them out explicitly in the
>    plan's "Resolutions coverage" table so a reviewer can verify
>    no resolution slipped:
>
>    | # | Resolution                                                                | Concrete deliverable(s) the plan owes |
>    |---|---------------------------------------------------------------------------|----------------------------------------|
>    | 1 | `worker_ref` canonical grammar                                            | Identity package: parser, formatter, exhaustive table tests covering every flag/JSON/env-var site. |
>    | 2 | Identifier validation grammar (`<feature>` / `<user>` / `<purpose>` regex; `<purpose>` ∉ wordlist; `--description` NFC + control-char stripping) | Validation package called from every input site; reject path for each invalid case is its own test. |
>    | 3 | YAML library pinned (`gopkg.in/yaml.v3`)                                  | One-line dep add with LICENSE-URL citation in the introducing PR. |
>    | 4 | `internal/errors/` package with sentinels + exit-code map                 | First deliverable per TDD order — every downstream package imports from it. |
>    | 5 | Test seams (`git.Runner`, `gh.Client`, `feature.Clock`, `identity.RNG`)   | Land each interface with its fake before any consumer. |
>    | 6 | Build-time version via `internal/version` + updated `build.sh` ldflags    | One deliverable, paired with the existing `gss version` port. |
>    | 7 | `internal/tmpl/tmpl.go` with `//go:embed *.md.tmpl`                       | One deliverable, before any template-rendering consumer. |
>    | 8 | `spawned_by` is informational-only                                        | Doc + test asserting no code path reads `spawned_by.engine` for branching. |
>    | 9 | Mode-detection rule: classic verbs in worker cwd → `ErrWrongMode`         | Cwd-detector deliverable; tests for every classic verb × in/out-of-worker combination. |
>    | 10 | `registry.json` 0600, flock + atomic rename, uid check                   | `internal/registry/lock.go` + `TestConcurrentWorkerAdd` + `TestDoneRacingCheckpoint`. |
>    | 11 | `gss feature pr --ready` requires approval token                         | Reuse `internal/approval` from classic; new test pinning the refusal path with no token. |
>    | 12 | NWO cache (`.nwo` file, invalidation on origin URL change)               | `internal/repo` deliverable with cache hit/miss tests. |
>    | 13 | `Backend.Enumerate(root)` added                                          | Update interface + git backend impl + `backendtest/contract.go`. |
>    | 14 | Companion CLI version pin & verb allowlist (git-machete)                 | `src/git-machete/skill/SKILL.md` deliverable with LICENSE blob SHA citation. |
>    | 15 | Pinned external deps & `go-licenses` CI gate                              | `build.sh` CI step + first-time `go-licenses check` invocation. |
>    | 16 | Auto-promote-on-merge: linear stacks only (resolution of question A)     | `internal/feature/merged` deliverable enforcing 1-child precondition + `TestMergedFanoutDoesNotPromote`. |
>    | 17 | Auto-promote requires `restack_count == 0` (resolution of question B)    | `restack_count` field on registry schema + `internal/feature/restack` increments it + `internal/feature/merged` checks it + `TestMergedRestackedChildDoesNotPromote`. |
>    | 18 | WORKER.md secret scanning deferred to roadmap (resolution of question C) | No code; doc deliverable confirming `roadmap.md` is committed and design points at it. |
>    | 19 | Empty-feature cleanup via template byte-diff (resolution of question D)  | `internal/feature/done` deliverable: template-renderer + comparator + `TestDoneOnEmptyFeatureMatchingTemplate` + `TestDoneOnEmptyFeatureWithEdits`. |
>    | 20 | Cross-machine sync: best-effort + `gss feature audit` (resolution of question E) | New `cmd/gss/feature/audit.go` + `internal/feature/audit` package with read-only and `--repair` modes; full check matrix from the design lands as a table-driven test. |
>    | 21 | `tmux-mgr internal migrate-to-gss` full one-way migrator (resolution of question F) | tmux-mgr deliverable: 9-step procedure + `--dry-run` mode + idempotent re-runs. Land *last* in the tmux-mgr refactor sequence so `pkg/workspace/` deletion follows. |
>    | 22 | `--force-autonomous` in worker cwd → `ErrWrongMode` (resolution of question G) | One test pinning the refusal in classic `push` and `pr`; one `safety_guard.sh` regex + assert_exit pair. |
>
>    Plus the v1.1 roadmap item (sessions[] append-only history) is
>    **out of scope** for the plan — do not include deliverables for
>    it. Likewise, anything in
>    [`roadmap.md`](./roadmap.md) (WORKER.md scanner, cross-machine
>    sync hardening) is out of scope. The plan covers v1.0 only.
> 5. Identify **parallel batches** — deliverables that share no
>    upstream dependency — so Phase 3 can fan out within the
>    5-agent cap. Use the TDD implementation order as the spine;
>    `internal/errors`, test seams, and `internal/version` are
>    serialized (every later package depends on them) but the leaf
>    classic-port packages (`internal/scan`, `internal/status`,
>    `internal/diff`) can run in parallel once the seams are in.
> 6. Encode the **rework protocol** (see below) so the executor
>    knows what to do when the human requests changes on an
>    open PR.
> 7. Include a **Resolutions coverage table** as a top-level
>    section of `plan.md`: every row of the table above maps to
>    `[deliverable IDs]` in the plan. Reviewers verify coverage by
>    skimming this table alone, not by re-reading the design.
> 8. End with a **Definition of Done** for v1.0:
>    - Every plan deliverable merged into `test_gss`.
>    - Every Pre-v1 hardening checklist item ticked.
>    - Resolutions-coverage table fully populated (every resolution
>      cited as covered by ≥ 1 merged deliverable).
>    - A final integration PR opened against `dotfiles:main` with
>      a one-page release/migration note.
>    - Classic slash commands (`/sync`, `/gss-pr`, `/gss-scan`,
>      `/gss`) verified working against the new binary via their
>      existing prompts.
>
> The plan must reference the design rather than duplicate it.
> Each plan entry is a checklist row the executor flips to done as
> it merges.
>
> #### Phase 2 — Set up the validation workspace
>
> 1. In `$HOME/<REPO_ROOT>`, create a branch named `test_gss`
>    off the current default branch. This is the integration
>    trunk for the stacked PR series; we don't push to `dotfiles`
>    until v1.0 is validated end-to-end.
> 2. Until `gss feature worker add` itself lands (around plan step
>    12), use plain `git worktree` for any parallel work. Once
>    `worker add` is mergeable, switch to **dogfooding** — every
>    subsequent deliverable's working tree is itself created by
>    the gss being built. This eats our own dog food and surfaces
>    integration issues earliest.
> 3. Confirm tooling: `go`, `gh` authenticated, `git`,
>    `go-licenses`. Bail loudly if anything is missing.
>
> #### Phase 3 — Execute
>
> Walk `plan.md` top-to-bottom. For each deliverable:
>
> 1. **Lead Developer subagent** writes tests *first*, then the
>    implementation. Tests must fail before the implementation
>    lands; passing tests at first commit is a TDD violation.
> 2. **QA Specialist subagent** runs `go test ./...`,
>    `go-licenses check ./...`, and (if the deliverable touches
>    the hook) `ai/hooks/safety_guard_test.sh`. Verifies
>    coverage delta. Rejects on regression, flake, or coverage
>    drop.
> 3. **Security Engineer subagent** audits any deliverable that
>    touches: the hardening checklist, `pr --ready`, `merged`,
>    `restack`, the approval token, the registry, the worktree
>    backend, or any new external dep.
> 4. **Architect subagent** spot-reviews when the deliverable
>    crosses the `cmd/gss/` ↔ `internal/` boundary or introduces
>    a new package.
> 5. **Tech Writer subagent** reviews any change that touches a
>    `SKILL.md`, the design doc, the README, or user-facing
>    output strings.
> 6. After every assigned reviewer signs off, open the PR
>    against the **immediate upstream PR** (stacked, not against
>    `main`), with a body that:
>    - Names the design sections implemented.
>    - Lists the stack position (parent PR `#N`, children
>      `[#M, #O]`).
>    - Embeds the assigned reviewers' summary lines.
>    - Pins the acceptance criteria the deliverable satisfies.
> 7. Wait for human merge or rework request. On rework:
>    - Address feedback as a new commit on the existing PR
>      branch — do not collapse history.
>    - If the change is structural and affects downstream PRs,
>      mark them "needs restack" in `plan.md`, restack them on
>      the updated parent (`git rebase --update-refs` or
>      `git-machete`), `--force-with-lease`, and refresh each
>      downstream PR body's stack cross-links.
>    - Never re-order the stack without explicit human approval.
>
> #### Validation gates (per PR, before opening)
>
> 1. `go test ./...` green.
> 2. `go-licenses check ./...` clean (no banned licenses per
>    `src/AGENTS.md`).
> 3. `build.sh` produces a working binary.
> 4. `safety_guard_test.sh` green if the hook is touched.
> 5. Deliverable-specific manual smoke step from `plan.md` is
>    executed and recorded in the PR body.
> 6. Coverage for the touched package ≥ 60% (or the deliverable
>    explicitly documents why not, per the design's coverage
>    relaxation rule for orchestrator packages).
>
> #### Definition of "execution complete"
>
> - Every `plan.md` deliverable merged into `test_gss`.
> - Every Pre-v1 hardening checklist item ticked.
> - A final integration PR opened from `test_gss` into
>   `dotfiles:main` containing a one-page migration / release
>   note for the user, and a checklist confirming all of the
>   above.
> - The classic slash commands (`/sync`, `/gss-pr`, `/gss-scan`,
>   `/gss`) verified working against the new binary via their
>   existing prompts — no regressions visible to the user from
>   the slash command surface.

### What to do when this prompt fires

If the user says **"write the plan"**: execute Phases 1+2 only,
commit `plan.md` on a working branch, open a PR for the plan
itself against `dotfiles:main`, and stop. Wait for human review.

If the user says **"execute the plan"** (with `plan.md` already
merged): execute Phase 3 end-to-end, stopping only at validation
failures or human rework requests.

If the user says both in one turn: chain them.
