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
	./opt/scripts/system/install_claude_skills.sh

.PHONY: claude-test
claude-test: ## Run Claude Code sanity check (CLI, links, hooks, 27-case hook test suite)
	./ai/claude/scripts/sanity_check.sh

.PHONY: claude-hook-test
claude-hook-test: ## Run safety_guard hook test suite only
	./ai/claude/hooks/safety_guard_test.sh
