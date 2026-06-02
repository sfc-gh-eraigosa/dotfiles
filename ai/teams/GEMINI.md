# AI Agent Teams

This directory contains specialized agent teams for use with Claude Code, Gemini CLI, Anti-Gravity CLI, and Ollama (local). See [`README.md`](./README.md) for the full team catalog and [`MODEL_PARAMS.md`](./MODEL_PARAMS.md) for the model-parameter convention.

## Push Safety — Required Reading for All Agents

> Any agent working in `sfc-gh-eraigosa/dotfiles` **must never push directly to `main`**.

When you finish work in this directory:

1. **Create a feature branch**: `git checkout -b ai/teams/<your-work>`
2. **Commit your changes** (stage files by name, never `git add -A`)
3. **Open a Draft PR** via `gss pr --title "<subject>" --body "<What/Why/Impact/Testing>"` or `gh pr create --draft`
4. **Do not merge** — leave the PR for human review

`safety_guard.sh` rule 12 will block any `git push origin main` before it reaches the network. This is an agent-layer guard, not a branch-protection rule.

## Teams Overview

| Team | Folder | Agents | Tier |
|------|--------|--------|------|
| Web Dev | `web/` | 5 | Fast → Deep Think |
| Go Dev | `go/` | 4 | Fast → Deep Think |
| Terraform & AWS | `terraform-aws/` | 4 | Fast → Deep Think |
| AI & CI | `ai-ci/` | 4 | Fast + Deep Think |
| Architecture | `architecture/` | 4 | Deep Think only |

See [`README.md`](./README.md) for full agent listings, aliases, and model assignments.
