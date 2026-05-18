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
3.  **Use your AI assistant**: This repo is agent-first. Either Gemini CLI or Claude Code can help you refactor, test, or document your changes — both read the same skills and progressive context.

## 🧪 Testing Infrastructure

The testing suite consists of:
- **Unit Tests**: Go-based tests located in the `src/` modules.
- **Integration Tests**: Docker-based tests that verify the entire environment, including script discovery and tool safeguards.

Before submitting a pull request, please ensure `make test` passes successfully.
