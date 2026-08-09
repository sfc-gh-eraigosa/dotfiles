# docs/ — repository documentation

Reference guides and the repo's objective-driven design system.

## Start here for design work

> **Any "let's design / plan / build X" work — an issue, a `gss feature` draft PR, a new
> feature, skill, CLI, or service — starts in [`mbo/`](./mbo/AGENTS.md)** (Management By
> Objective). It routes the task to the right skill workflow and lays down consistent
> design → spec → plan artifacts tracked in [`mbo/index.md`](./mbo/index.md).

- [`mbo/`](./mbo/) — design · spec · plan artifacts + the objective tracker. Read
  [`mbo/AGENTS.md`](./mbo/AGENTS.md) first.

## Reference guides (not objective artifacts)

- [`ai-plugins.md`](./ai-plugins.md) — the Claude/Antigravity plugin catalog + mapping.
- [`machine-local-overrides.md`](./machine-local-overrides.md) — the `~/.zshrc.local` pattern.
- [`claude-code-support.md`](./claude-code-support.md) — supported Claude Code version range, cross-version compatibility rules, and the upgrade-audit runbook.
- [`gitignore-allowlist.md`](./gitignore-allowlist.md) — the `*`-default `.gitignore` allowlist: opted-in paths, verification recipe, worked examples.
- [`install-windows.md`](./install-windows.md) — `install.sh` Windows/WSL interactivity flow, `[y]/[s]` semantics, gff overrides.
- [`mergify.md`](./mergify.md) — the merge model: Mergify queue, solo-maintainer rules and rationale, external-contributor review gate, AI-work traceability, break-glass.
