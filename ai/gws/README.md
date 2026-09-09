# Google Workspace CLI Configuration (`ai/gws`)

This directory contains configuration templates and documentation for the Google Workspace CLI (`gws`).

## Configuration Overview

The `google-cli-setup.sh` installer provisions `gws` and seeds default configuration files:

- `ai/gws/config.json.template`: Template for `gws` runtime configuration, linked to `~/.gws/config.json`.
- `ai/gws/client_secret.json.template`: Template for OAuth client secrets, seeded into `~/.config/gws/client_secret.json` if absent.

> [!WARNING]
> The seeded `client_secret.json` contains placeholder strings (`YOUR_CLIENT_ID...`). You must configure real credentials before running `gws auth login`, or Google OAuth will fail with `Error 401: invalid_client`.

## Setup & Authentication

Verify your current authentication status at any time:

```bash
gws auth status
```

### Option 1: Automated Setup via `gcloud` (Recommended)

If you have the Google Cloud SDK (`gcloud`) installed, `gws` can create the GCP project, enable APIs, generate a Desktop OAuth client, and log in automatically:

```bash
# 1. Authenticate with Google Cloud
gcloud auth login

# 2. Run the automated setup and login wizard
gws auth setup --login
```

### Option 2: Manual GCP Console Setup

If you prefer to configure the OAuth client manually in the Google Cloud Console:

1. **Enable APIs**: Navigate to [APIs & Services > Library](https://console.cloud.google.com/apis/library) and enable:
   - Google Drive API
   - Google Docs API
   - (Optional) Gmail API, Google Calendar API
2. **Configure OAuth Consent Screen**: Under [APIs & Services > OAuth consent screen](https://console.cloud.google.com/apis/credentials/consent):
   - Choose **External** (or **Internal** for Google Workspace organizations).
   - If the app publishing status is **Testing**, add your account email under **Test users**.
3. **Create OAuth Client ID**:
   - Go to [APIs & Services > Credentials](https://console.cloud.google.com/apis/credentials).
   - Click **Create Credentials** > **OAuth client ID**.
   - Select Application type: **Desktop app**.
4. **Save Credentials**:
   - Save the downloaded OAuth credentials JSON to `~/.config/gws/client_secret.json`.
5. **Log In**:
   ```bash
   gws auth login
   ```

## Common Commands

- **List Drive files**:
  ```bash
  gws drive files list --params '{"pageSize": 10}'
  ```
- **Read document content**:
  ```bash
  gws drive files get --fileId <FILE_ID> --alt media
  ```
- **Search for a document**:
  ```bash
  gws drive files list --params '{"q": "name = '\''Doc Name'\'' and mimeType = '\''application/vnd.google-apps.document'\''"}'
  ```

## Troubleshooting

### `Error 401: invalid_client` ("The OAuth client was not found")

- **Cause**: `~/.config/gws/client_secret.json` exists but contains the unedited template placeholder (`"YOUR_CLIENT_ID.apps.googleusercontent.com"`).
- **Resolution**:
  - Run `gws auth setup --login` to generate valid credentials via `gcloud`.
  - Alternatively, replace `~/.config/gws/client_secret.json` with real Desktop app client credentials downloaded from Google Cloud Console.

### `Error 403: access_denied` / Insufficient Permission

- **Cause**: The OAuth consent was granted without required scopes, or the target API (Drive, Docs, Gmail) is not enabled in the GCP project.
- **Resolution**:
  - Ensure the relevant APIs are enabled in the Google Cloud Console.
  - Re-run `gws auth login` and check all permission checkboxes on the Google consent screen.

### `gcloud: command not found` or Not Logged In

- **Resolution**:
  - Run `opt/scripts/system/google-cli-setup.sh gcloud` to install the Google Cloud SDK.
  - Run `gcloud auth login` before running `gws auth setup`.
