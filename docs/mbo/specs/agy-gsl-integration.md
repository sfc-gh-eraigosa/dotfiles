# agy-gsl-integration — spec

- **Slug:** agy-gsl-integration
- **Date:** 2026-07-10
- **Status:** Approved
- **Relates to:** spec `../specs/agy-gsl-integration.md` · plan `../plans/agy-gsl-integration.md`

## 1. Goal
Provide dynamic, real-time Go Status Line (gsl) integration for the Antigravity CLI (agy) by automating the configuration of its status line block during installation and provisioning.

## 2. Use cases
* **U1: Fresh Installation**
  * Actor: Developer / install script
  * Trigger: Run `install.sh` or `install_antigravity_skills.sh`
  * Flow:
    1. Installs/compiles the `gsl` binary.
    2. Copies the status line command shim into `~/.gemini/config/statusline-command.sh`.
    3. Merges the status line configuration block into `~/.gemini/antigravity-cli/settings.json`.
  * Acceptance criteria: The status line block is correctly configured and enabled, referencing `bash ~/.gemini/config/statusline-command.sh`.
* **U2: Preserve Existing Settings**
  * Actor: Developer / install script
  * Trigger: Run `install_antigravity_skills.sh` on a host that already has customizations in `~/.gemini/antigravity-cli/settings.json`.
  * Flow: Same as U1.
  * Acceptance criteria: Existing keys in `settings.json` (e.g. `colorScheme`) are preserved, while the status line configuration is successfully merged.

## 3. Architecture
* Component: `install_antigravity_skills.sh`
* Component: `statusline-command.sh`
* Interface: `~/.gemini/antigravity-cli/settings.json` configured with:
  ```json
  "statusLine": {
    "type": "command",
    "command": "bash ~/.gemini/config/statusline-command.sh",
    "enabled": true
  }
  ```

## 4. Behavior / features
* Automatically copies `ai/claude/statusline-command.sh` to `~/.gemini/config/statusline-command.sh` during `install_antigravity_skills.sh`.
* Backs up `~/.gemini/antigravity-cli/settings.json` to `settings.json.bak` once.
* Seeds `settings.json` with `{}` if missing.
* Deep-merges the `statusLine` configuration block using the shared `apply-forced-settings.sh` script.

## 5. Evaluation criteria (per feature)
* Trigger `install_antigravity_skills.sh` -> files exist, `statusLine` is configured.
* Trigger `install_antigravity_skills.sh` with existing configuration -> custom keys are not wiped.

## 6. Verification harness
* Scripted tests in `install_antigravity_skills_test.sh`:
  - Assert that on a fresh run, `statusline-command.sh` is copied, `settings.json` is created, and the `statusLine` block is set.
  - Assert that on a dirty run (existing config keys), they are preserved and `statusLine` is successfully merged.

## 7. Prerequisites / dependencies
* `jq` must be installed.
* `apply-forced-settings.sh` must be accessible.

## 8. Out of scope (and why)
* Designing new segments: we reuse the existing `gsl` segments.

## 9. Rollback
* Restore `~/.gemini/antigravity-cli/settings.json` from `settings.json.bak`.
