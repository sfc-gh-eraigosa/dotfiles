# Contributing to Dotfiles

Thank you for your interest in contributing to the Dotfiles project! This document provides guidelines and instructions for contributing and validating changes.

## 🛠️ Development & Validation

This repository uses a `Makefile` to orchestrate development tasks. This ensures that all contributions are validated against the same standards used in CI.

### Common Development Targets

| Mode | Command | Description |
| :--- | :--- | :--- |
| **All Tests** | `make test` | Runs unit tests and full Docker integration tests. |
| **Unit Tests** | `make unit-test` | Runs Go unit tests for `src/` binaries. |
| **Integration** | `make integration-test` | Runs Docker-based integration tests. |
| **Build Image** | `make build` | Builds the Docker image used for testing. |
| **Claude Install** | `make claude-install` | Install (or update) Claude Code CLI and link skills/commands/settings. |
| **Claude Sanity** | `make claude-test` | Run Claude Code sanity check (CLI, links, hooks). |
| **Hook Tests** | `make claude-hook-test` | Run the 27-case `safety_guard.sh` test suite. |
| **Help** | `make help` | Show all available development targets. |

### Development Workflow

1.  **Make your changes**: Adhere to the project's style and architectural patterns.
2.  **Validate locally**: Run `make test` to ensure your changes don't break existing functionality.
3.  **Use your AI assistant**: This repo is agent-first. Either Antigravity CLI (`agy`) or Claude Code can help you refactor, test, or document your changes — both read the same skills and progressive context.

## 🧪 Testing Infrastructure

The testing suite consists of:
- **Unit Tests**: Go-based tests located in the `src/` modules.
- **Integration Tests**: Docker-based tests that verify the entire environment, including script discovery and tool safeguards.

Per-module Go coverage minimums are enforced by `scripts/test.sh unit`
(invoked by `make unit-test`). Thresholds are declared in the
`COVERAGE_MIN` map at the top of `scripts/test.sh`; a module under its
floor fails CI. HTML reports are written to `coverage/<mod>.html` and
uploaded as the `coverage-report` artifact by the workflow.

Before submitting a pull request, please ensure `make test` passes successfully.

## Merge queue (Mergify)

`main` is gated behind a **Mergify merge queue** — the full model, every rule,
and the reasoning behind them live in [docs/mergify.md](./docs/mergify.md).
The short version:

- **Maintainer flow**: get CI green, apply the **`ready-for-merge`** label
  (or comment `@mergifyio queue`), and walk away. Mergify updates the PR in
  place against the latest `main`, re-runs the required checks on the updated
  code, and **squash-merges** (the only allowed merge method). Serialized
  entries make stale-green merges impossible. After landing, the label swaps
  to `merged-by-mergify` automatically.
- **External contributors**: your PR additionally requires an approving
  review from the maintainer before it can queue or merge — the label alone
  never lands an unreviewed external change. `dependabot[bot]` is exempt
  (GitHub-operated, trusted source).
- **On queue failure**: Mergify removes the PR from the queue with a status
  comment. Fix, push, and re-label to re-enter.
- **Why a queue at all**: a PR check passes against your branch plus whatever
  `main` looked like at the time; the queue re-validates against the `main`
  your PR will actually land on, catching semantic conflicts branch-time CI
  misses.
