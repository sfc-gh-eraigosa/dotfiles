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
| **Help** | `make help` | Show all available development targets. |

### Development Workflow

1.  **Make your changes**: Adhere to the project's style and architectural patterns.
2.  **Validate locally**: Run `make test` to ensure your changes don't break existing functionality.
3.  **Use Gemini**: This repo is agent-first. You can ask Gemini to help you refactor, test, or document your changes.

## 🧪 Testing Infrastructure

The testing suite consists of:
- **Unit Tests**: Go-based tests located in the `src/` modules.
- **Integration Tests**: Docker-based tests that verify the entire environment, including script discovery and tool safeguards.

Before submitting a pull request, please ensure `make test` passes successfully.
