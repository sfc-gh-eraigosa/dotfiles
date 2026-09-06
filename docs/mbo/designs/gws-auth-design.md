# Design: Google Workspace CLI (gws) Authentication

This document outlines the authentication strategies for the `gws` CLI integration, distinguishing between interactive user setup and automated CI/CD environments.

## Overview
The `gws` CLI uses OAuth 2.0 to access Google Workspace APIs. We support two primary modes of authentication to balance ease of use with automation requirements.

## 1. Interactive Setup (Primary)
This is the default workflow for local development environments.

### Workflow
1. User runs `gws auth setup` or `gws auth login`.
2. The CLI opens a browser for OAuth consent.
3. Upon approval, credentials (refresh/access tokens) are stored locally in `~/.config/gws/`.

### Pros
- Automated API enablement and credential creation (if using `gws auth setup`).
- User-friendly.
- No sensitive files to manage manually.

## 2. CI/CD & Automated Provisioning
This mode is intended for non-interactive environments where browser access is not possible.

### Workflow
1. A Desktop OAuth client is created manually in the Google Cloud Console.
2. The `client_secret.json` is downloaded.
3. The secret is stored in a secrets management system (e.g., GitHub Actions Secrets, HashiCorp Vault, or SOPS).
4. During provisioning, the secret is materialized to `~/.config/gws/client_secret.json`.

### Pros
- Fully non-interactive.
- Deterministic and reproducible.

### Security Considerations
- **Secret Management**: `client_secret.json` MUST NEVER be committed to the repository. Use the provided template (`ai/gws/client_secret.json.template`) for local setup only.
- **Scope Limitation**: For CI/CD, consider creating a dedicated service account or limiting OAuth scopes to only what is strictly necessary for the automated task.

## Future Improvements
- **Service Account Support**: Investigate native service account integration for server-to-server scenarios without a refresh token.
- **Automatic Secret Materialization**: Integrate with the existing `install_sops.sh` or a Vault-based setup to automatically fetch credentials.
