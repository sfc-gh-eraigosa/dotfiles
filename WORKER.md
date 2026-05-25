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
- **Full API Coverage**: We assume the user wants access to all major Workspace APIs (Calendar, Gmail, Drive, Docs, Sheets) by default.

## Authentication Methods: Pros & Cons
We support two primary ways to authenticate `gws`:

1. **Pre-provisioned `client_secret.json`**:
   - **Pros**: Can be fully automated (non-interactive); ideal for CI/CD or managed workstations.
   - **Cons**: Requires manual creation of a Desktop OAuth client in the Google Cloud Console; requires secure distribution of the secret file.
2. **Interactive `gws auth setup`**:
   - **Pros**: Very user-friendly; automates API enablement and credential creation via the browser.
   - **Cons**: Requires manual interaction during the installation process; requires `gcloud` to be installed and authenticated for full automation.

## Open questions
- **Authorization**: We will default to requesting full scopes for the primary Workspace services to ensure the assistant is useful out-of-the-box.
