# gemini-plugins: plan

- **Description**: Create a plan for Gemini plugin installation
- **Worker**: edward-raigosa/plan
- **Branch**: feature/gemini-plugins/edward-raigosa/plan
- **Base branch**: main

## Goal
Integrate Gemini CLI extensions into the declarative AI plugin manifest (`ai/plugins.yaml`) and the `sync-plugins` engine, providing equivalents to the existing Claude plugins.

## Decisions & notes
- **Manifest Integration**: Added `gemini.source` attributes to `ai/plugins.yaml`.
- **Non-interactive Installation**: Updated `sync-plugins.sh` to use `--consent` and `--skip-settings` for Gemini extensions, ensuring they install cleanly during `install.sh`.
- **Worktree Safety**: Added a critical warning to `GEMINI.md` to prevent running `install.sh` from worktrees, protecting the host `$HOME` state.
- **Documentation**: Updated `docs/ai-plugins.md` to reflect full Gemini support.

## Verification
- Verified non-interactive installation via `./install.sh` from the main repository.
- Verified active extensions via `gemini extensions list`.
- Verified `sync-plugins --dry-run` correctly parses both Claude and Gemini blocks.
