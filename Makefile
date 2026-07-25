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
bin: ## Rebuild all binaries in ./sdk
	@for d in sdk/*; do \
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

.PHONY: skill-evals
skill-evals: ## Validate agent-skill eval corpora (ai/skills/*/evals/evals.json) deterministically
	./opt/scripts/system/skill-eval.sh --check

.PHONY: sdk-bump
sdk-bump: ## Report sdk/<tool> modules whose source changed since their last tag (conventional-commit semver). Read-only; CI applies the bump on merge.
	./opt/scripts/system/bump-sdk-version.sh --check

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
lint: check-legacy-paths lint-go lint-shell lint-markdown ## Run all linters (go, shell, markdown)


.PHONY: check-legacy-paths
check-legacy-paths: ## Fail if a legacy Go module path or src/<tool> reappears (Go lives in sdk/)
	@echo "==> checking for legacy Go module paths (must be 0)..."
	@if grep -rInE 'github\.com/(wenlock|eraigosa)/dotfiles' \
		--include='*.go' --include='go.mod' --include='build.sh' --exclude-dir=.git . ; then \
		echo "ERROR: legacy module path in Go source — use github.com/sfc-gh-eraigosa/dotfiles/sdk/<tool>"; \
		exit 1; \
	fi
	@for t in gss gsl wol tmux-mgr; do \
		if [ -e "src/$$t/go.mod" ]; then \
			echo "ERROR: Go module src/$$t exists — Go modules live in sdk/"; exit 1; \
		fi; \
	done
	@echo "OK: no legacy Go module paths; no Go modules under src/"
.PHONY: lint-go
lint-go: ## Lint Go modules (gofmt + golangci-lint, per-module)
	@echo "==> gofmt check across sdk/..."
	@unformatted=$$(gofmt -l ./sdk 2>/dev/null); \
		if [ -n "$$unformatted" ]; then \
			echo "gofmt found unformatted files:"; \
			echo "$$unformatted"; \
			exit 1; \
		fi
	@for d in sdk/*; do \
		if [ -f "$$d/go.mod" ]; then \
			echo "==> golangci-lint run ($$d)"; \
			(cd "$$d" && golangci-lint run ./...) || exit 1; \
		fi; \
	done

.PHONY: lint-shell
lint-shell: ## Lint shell scripts with shellcheck
	@echo "==> shellcheck (all *.sh files)"
	# Phase 5 (issue #46) extended the scope to opt/profiles/ so the
	# user-facing shell entrypoints (.bashrc, .profile, .bash_aliases,
	# .docker.sh, .goenv.sh, .nano_profile, .xsessionrc, .bash_logout)
	# get linted too. Dotfile profiles are listed EXPLICITLY (not via a
	# `.*` glob) because bash globs skip dotfiles by default and
	# `shopt -s dotglob` is not portable across make-invoked shells. The
	# list mirrors install.sh's symlink targets — when a new profile lands
	# in opt/profiles/, add it here too.
	#
	# .zshrc is INTENTIONALLY EXCLUDED: shellcheck only supports
	# sh/bash/dash/ksh and emits SC1071 (error) on zsh files, which would
	# fail the whole job regardless of `-S warning`. Zsh-specific linting
	# would need a different tool (zsh-syntax-check or `zsh -n`); see
	# .ci-baseline-issues.md for the deferred-work note.
	#
	# Phase 8 (issue #46 / shell-portability) extended coverage to sdk/**,
	# src/**, and opt/bin/** to close the gap on ~17 previously-unlinted
	# scripts (build.sh, test helpers, setup scripts, utilities).
	#
	# This target is STRICT — it exits non-zero on any finding (shellcheck
	# -S warning returns non-zero even for warnings) — so do not add `|| true`.
	# The shellcheck backlog has been driven to ZERO, and the shell-lint.yml
	# workflow now runs this WITHOUT continue-on-error: a new shellcheck warning
	# is a hard CI failure. Genuinely-intentional patterns carry an inline
	# `# shellcheck disable=SCxxxx # <reason>`. (docker-image.yml's broader
	# `make lint` may still be warn-only for its own non-shell backlog.)
	@files=$$(find opt/scripts ai sdk src opt/bin -name '*.sh' -type f ! -path '*/.*' ! -path '*/opt/google-cloud-sdk/*' 2>/dev/null) ; \
		profile_dotfiles="opt/profiles/.bashrc opt/profiles/.profile opt/profiles/.bash_aliases opt/profiles/.bash_logout opt/profiles/.docker.sh opt/profiles/.goenv.sh opt/profiles/.nano_profile opt/profiles/.xsessionrc" ; \
		profile_sh=$$(find opt/profiles -maxdepth 1 -name '*.sh' -type f 2>/dev/null) ; \
		shellcheck -x -S warning install.sh $$files $$profile_sh $$profile_dotfiles

# Cross-shell / cross-OS portability scan. Catches the class that shellcheck
# and `bash -n` MISS: dash (/bin/sh) parse breakage — the Raspberry Pi LightDM
# login-loop class — and macOS BSD-coreutil / bash-3.2 hazards. ENFORCING:
# `--strict` exits non-zero on any Tier 1 (POSIX breakage) or Tier 2 (macOS
# hazard) finding, so a newly introduced non-portable script fails CI. The
# backlog is at zero (see docs/mbo/specs/shell-portability.md); to exempt a
# reviewed line add a trailing `# portability-ok: <reason>` comment.
.PHONY: lint-portability
lint-portability: ## Cross-shell/OS portability scan (dash + macOS) — ENFORCING
	@opt/scripts/system/shell-portability-scan.sh --strict

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

gff-proto: ## Regenerate sdk/gff proto Go code (raw protoc, go.mod-pinned plugin; output is committed)
	bash sdk/gff/scripts/genproto.sh

gff-proto-check: gff-proto ## Regenerate and fail if committed sdk/gff/gen/ output drifts
	git diff --exit-code -- sdk/gff/gen/
