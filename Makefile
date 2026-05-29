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
test: shell-test ## Run all tests (shell-test + scripts/test.sh all)
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

# -----------------------------------------------------------------------------
# Shell test framework (issue #46 phase 2)
# -----------------------------------------------------------------------------
# Each `*_test.sh` is a standalone bash driver using ai/_test_helpers.sh.
# Discovery scans ai/, opt/scripts/, opt/bin/, opt/profiles/, and the
# repo root for any *_test.sh (so install_test.sh at the root is picked
# up). opt/google-cloud-sdk is excluded — it's a vendored SDK that ships
# its own scripts. NOTE: dotfile-prefixed driver names (e.g.
# `.bash_aliases_test.sh`) are picked up by `find -name '*_test.sh'`
# because find pattern matching does not skip dotfiles (only the shell
# glob does).
#
# Run a single driver standalone: `bash path/to/foo_test.sh`
# -----------------------------------------------------------------------------

.PHONY: shell-test
shell-test: ## Run all *_test.sh shell test drivers (uses ai/_test_helpers.sh)
	@echo "==> shell-test (discovering *_test.sh)"
	@drivers=$$( \
		{ \
			find ai opt/scripts opt/bin opt/profiles -maxdepth 6 -name '*_test.sh' -type f 2>/dev/null; \
			find . -maxdepth 1 -name '*_test.sh' -type f 2>/dev/null; \
			find scripts -maxdepth 1 -name '*_test.sh' -type f 2>/dev/null; \
		} | grep -v '^./opt/google-cloud-sdk' | sort -u \
	) ; \
	if [ -z "$$drivers" ]; then \
		echo "no shell test drivers found"; exit 0; \
	fi ; \
	pass=0; fail=0; failed_files=""; \
	for f in $$drivers; do \
		echo "----"; \
		echo "RUN: $$f"; \
		if bash "$$f"; then \
			pass=$$((pass+1)); \
		else \
			fail=$$((fail+1)); \
			failed_files="$$failed_files $$f"; \
		fi ; \
	done ; \
	echo "===="; \
	echo "shell-test: $$pass passed, $$fail failed"; \
	if [ "$$fail" -gt 0 ]; then \
		echo "Failed drivers:$$failed_files"; \
		exit 1; \
	fi
