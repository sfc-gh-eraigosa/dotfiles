# The `.gitignore` allowlist pattern

This repo's `.gitignore` starts with `*`, which blocks **every path** by default. Files become
visible to git only when an explicit `!`-rule opts them back in. The consequence is non-obvious
and bites new contributors (human and agent): if you create `sdk/gss/docs/plan.md` and don't see
it in `git status`, the file is **not lost** — it's been silently ignored by the default `*`
because no `!`-rule covers its path yet. This is also why a freshly-created top-level folder
(say `~/git/dotfiles/notes/`) won't show up in `git status` at all — git treats it as ignored,
not as untracked, and skips it entirely.

## Whenever you add a new file or directory

First check whether the path is covered by an existing `!`-rule. Top-level paths already opted in
include `!src/**`, `!opt/**`, `!ai/**`, `!system/**`, `!docs/**`, `!.config/**`, `!.github/**`,
`!.devcontainer/**`, `!scripts/**`, plus per-file allowlists for `README.md`, `CONTRIBUTING.md`,
`Makefile`, `install.sh`, `LICENSE`, `Dockerfile`, `azure-pipelines.yml`, `**/AGENTS.md`,
`**/CLAUDE.md`. Anything outside those trees is invisible by default.

## Verify, don't assume

After creating a new file, run `git status --short -- <path>` and `git check-ignore -v <path>`.
If the file does not appear in `git status`, add an `!`-rule to `.gitignore` **before** attempting
to stage. Never paper over this with `git add -f` — forced adds bypass the policy and leave the
next contributor confused about whether the path is supposed to be tracked.

## When in doubt, opt in explicitly

Prefer narrow rules (`!sdk/gss/**`) over broad ones (`!**/*.md`). Document any path that is
*intentionally* local-only with an inline `.gitignore` comment explaining why (examples:
`opt/.DS_Store`, `ai/claude/settings.json`, `sdk/tmux-mgr/tmux-mgr` — each has a comment line
describing the reason).

## Worked example — `sdk/gss/docs/`

The design document is stage-able only because `!src/**` opts in the entire `src/` tree. If we
ever moved gss docs out of `src/` (e.g. to a top-level `design/` folder), the new path would be
invisible until we added `!design/` and `!design/**` rules. Conversely, anything we add *under*
`src/` is auto-tracked — no per-file rule needed. The same logic explains why
`sdk/gss/docs/design.md` shows in `git status`: it's covered by `!src/**`, not by a per-doc rule.

## Ground rules

- **Never rely on "it's already covered by `*`"** as a substitute for a documented decision.
  A path is either explicitly tracked or explicitly local-only; ambiguity is a bug.
- **Note on `docs/` at the repo root**: `!docs/` and `!docs/**` are in `.gitignore` so the
  top-level `docs/` tree is tracked. This is distinct from `sdk/gss/docs/` (covered by `!src/**`)
  and from `opt/docs/` (covered by `!opt/**`).
