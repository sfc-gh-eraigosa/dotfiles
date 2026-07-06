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

## Merge queue

`main` is gated behind GitHub's [merge queue](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/managing-a-merge-queue).
Direct pushes are blocked; every PR lands via the queue.

**How it works.** After approval, click **Merge when ready** on the PR. GitHub
enqueues the PR, builds a synthetic merge commit that combines your branch
with the current queue head, and re-runs the required status checks against
that synthetic commit. PRs are merged FIFO. If the queue detects a failure
mid-flight, GitHub auto-removes the offending PR and re-builds the remaining
queue without it — your earlier-queued neighbours don't pay for your failure.

**Required status checks** (must pass on the synthetic `merge_group` commit
before `main` advances):

- `lint` — golangci-lint + shellcheck + markdownlint + actionlint
- `unit-tests` — Go suite, including per-module coverage floors
- `shell-tests` — every `*_test.sh` driver under `make shell-test`
- `build-and-validate` — Docker image build + integration tests

**Queue policy: rebase merge.** The queue uses `REBASE` as the merge method,
preserving linear history on `main` and matching the expectations of the
`gss feature merged` retargeting logic (it walks parent links, not merge
commits). **No squash, no merge commits** — the queue itself serializes
landings, so we don't need either to keep history readable.

**On queue failure.** GitHub removes your PR from the queue and posts a
failure comment. Fix the regression locally, push to your branch, re-request
review if anything substantive changed, then click **Merge when ready**
again to re-enter the queue at the back of the line.

**Why not just rely on PR checks?** A PR check passes against your branch
plus whatever `main` looked like when you opened the PR. The queue re-runs
those same checks against the *future* state of `main` (your PR rebased onto
everything queued ahead of it), catching semantic conflicts (drifted
interfaces, removed flags) that branch-time CI misses.
