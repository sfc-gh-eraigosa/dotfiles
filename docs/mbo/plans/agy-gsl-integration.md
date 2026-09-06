# agy-gsl-integration — implementation plan

- **Slug:** agy-gsl-integration
- **Date:** 2026-07-10
- **Status:** Approved
- **Relates to:** spec `../specs/agy-gsl-integration.md`

## 1. Summary & verdict
Implement dynamic status line configuration in the Antigravity CLI installer, mirroring the Claude Code setup but with path isolation (copying the shim to `~/.gemini/config/`).

## 2. File inventory
| Path | Purpose | Implements |
| :-- | :-- | :-- |
| `opt/scripts/system/install_antigravity_skills.sh` | Provisions agy settings and shim | Spec §4 |
| `opt/scripts/system/install_antigravity_skills_test.sh` | Unit tests for provisioning | Spec §6 |
| `ai/antigravity/settings.forced.json` | Declarative settings block to force-merge | Spec §3, §4 |

## 3. Interface contracts
* `settings.forced.json`:
  ```json
  {
    "statusLine": {
      "type": "command",
      "command": "bash ~/.gemini/config/statusline-command.sh",
      "enabled": true
    }
  }
  ```

## 4. TDD build order
1. **Create `ai/antigravity/settings.forced.json`**
   - Write the JSON configuration block. (Done)
2. **Modify `opt/scripts/system/install_antigravity_skills_test.sh`** (Test-first)
   - Add assertions for `statusline-command.sh` copy, backup, and `settings.json` merge.
   - Run tests to confirm they fail.
3. **Modify `opt/scripts/system/install_antigravity_skills.sh`**
   - Copy shim, handle backups, seed file, and merge using `apply-forced-settings.sh`.
   - Run tests to confirm they pass.

## 5. Verification mapping
* Fresh Install -> `install_antigravity_skills_test.sh` (Fresh host assertions)
* Legacy keys -> `install_antigravity_skills_test.sh` (Idempotency and backup preservation assertions)

## 6. Integration & rollout
* Checked in via `gss feature checkpoint`.
