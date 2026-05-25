# gdocsintegration: install-mechanic

- **Description**: Setup Google Docs workspace connection
- **Worker**: edward-raigosa/install-mechanic
- **Branch**: feature/gdocsintegration/edward-raigosa/install-mechanic
- **Base branch**: main

## Goal
Implement an installation mechanic for Google Docs integration. This includes:
- Creating a `google-cli-setup.sh` script to install `@google/gemini-cli` and `@googleworkspace/cli`.
- Setting up configuration templates in `ai/gws/`.
- Creating a new Gemini skill in `ai/skills/gdocsintegration/` to guide the assistant in interacting with Google Workspace.
- Integrating the setup into the main `install.sh`.

## Decisions & notes
- Used `@googleworkspace/cli` (gws) as the primary tool for Workspace interaction.
- Added a seeding mechanism for `config.json` to allow per-host configuration while maintaining a shared template.
- Integrated `google-cli-setup.sh` into `install.sh` to ensure a smooth bootstrapping experience.

## Open questions
- Should we provide a default `client_secret.json` or rely on `gws auth setup`?
- Are there specific Google Docs APIs that need pre-authorization?
