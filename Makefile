.DEFAULT_GOAL := help

IMAGE_NAME ?= dotfiles-test

.PHONY: help
help: ## Display this help message
	@echo "Dotfiles Development and CI Workflow"
	@echo "This Makefile is for contributing and validating changes to the dotfiles repo."
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: bin
bin: ## Rebuild all binaries in ./src
	@for d in src/*; do \
		if [ -f "$$d/build.sh" ]; then \
			echo "Building $$d..."; \
			bash "$$d/build.sh"; \
		fi; \
	done

.PHONY: build
build: ## Build the docker image used for testing
	docker build -t $(IMAGE_NAME) .

.PHONY: test
test: ## Run all tests using scripts/test.sh
	./scripts/test.sh all

.PHONY: unit-test
unit-test: ## Run unit tests only
	./scripts/test.sh unit

.PHONY: integration-test
integration-test: ## Run integration tests only
	./scripts/test.sh integration

.PHONY: claude-install
claude-install: ## Install (or update) Claude Code CLI and link skills/commands/settings
	./opt/scripts/system/claude_install.sh
	./opt/scripts/system/sync-skills.sh
	./opt/scripts/system/install_claude_skills.sh

.PHONY: claude-test
claude-test: ## Run Claude Code sanity check (CLI, links, hooks, 27-case hook test suite)
	./ai/claude/scripts/sanity_check.sh

.PHONY: claude-hook-test
claude-hook-test: ## Run safety_guard hook test suite only
	./ai/hooks/safety_guard_test.sh

# -----------------------------------------------------------------------------
# Lint targets (issue #46 phase 1)
# -----------------------------------------------------------------------------
# These are the canonical entry points for repo-wide linting. CI invokes
# `make lint` to fan out to all three sub-targets. Each sub-target may be
# invoked individually for fast local iteration.
#
# Tool requirements (install locally before running):
#   - golangci-lint  (Go)         https://golangci-lint.run/
#   - shellcheck     (shell)      https://www.shellcheck.net/
#   - markdownlint-cli2 (Markdown) https://github.com/DavidAnson/markdownlint-cli2
# -----------------------------------------------------------------------------

.PHONY: lint
lint: lint-go lint-shell lint-markdown ## Run all linters (go, shell, markdown)

.PHONY: lint-go
lint-go: ## Lint Go modules (gofmt + golangci-lint, per-module)
	@echo "==> gofmt check across src/..."
	@unformatted=$$(gofmt -l ./src 2>/dev/null); \
		if [ -n "$$unformatted" ]; then \
			echo "gofmt found unformatted files:"; \
			echo "$$unformatted"; \
			exit 1; \
		fi
	@for d in src/*; do \
		if [ -f "$$d/go.mod" ]; then \
			echo "==> golangci-lint run ($$d)"; \
			(cd "$$d" && golangci-lint run ./...) || exit 1; \
		fi; \
	done

.PHONY: lint-shell
lint-shell: ## Lint shell scripts with shellcheck
	@echo "==> shellcheck (opt/scripts, ai, install.sh)"
	@files=$$(find opt/scripts ai -name '*.sh' -type f 2>/dev/null) ; \
		shellcheck -x -S warning install.sh $$files

.PHONY: lint-markdown
lint-markdown: ## Lint markdown files with markdownlint-cli2
	@echo "==> markdownlint-cli2"
	@markdownlint-cli2 "**/*.md" "#opt/google-cloud-sdk" "#node_modules" "#**/node_modules"
