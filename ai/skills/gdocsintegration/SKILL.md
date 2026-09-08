---
name: google-docs
description: Integration for interacting with Google Docs and Google Workspace using the 'gws' CLI.
---
# Google Docs Integration Skill

This skill allows Antigravity and Claude to interact with Google Workspace (Docs, Drive, Gmail, etc.) using the `gws` CLI.

## Capabilities

- **Read Docs**: Read content from Google Docs.
- **Search Drive**: Search for files in Google Drive.
- **Manage Files**: Create, update, or delete files in Workspace.

## Usage

### 1. Authenticate & Setup

Check your current authentication state:
```bash
gws auth status
```

#### Option A: Automated Setup via gcloud (Recommended)
If you have `gcloud` installed, `gws` can provision the GCP project and desktop OAuth client automatically:
```bash
gcloud auth login
gws auth setup --login
```

#### Option B: Manual GCP Console Setup
If setting up credentials manually:
1. In the [Google Cloud Console](https://console.cloud.google.com/), enable the **Google Drive API** and **Google Docs API**.
2. Configure the **OAuth consent screen** (set user type, and add your email under Test Users if in Testing status).
3. Under **APIs & Services > Credentials**, create an **OAuth client ID** of type **Desktop app**.
4. Save the downloaded client secret file to `~/.config/gws/client_secret.json`.
5. Run the login flow:
   ```bash
   gws auth login
   ```

### 2. Common Commands

- **List Drive files**:
  ```bash
  gws drive files list --params '{"pageSize": 10}'
  ```
- **Read a Doc**:
  ```bash
  gws drive files get --fileId <FILE_ID> --alt media
  ```
- **Search for a Doc by name**:
  ```bash
  gws drive files list --params '{"q": "name = '\''My Document'\'' and mimeType = '\''application/vnd.google-apps.document'\''"}'
  ```

## Guidelines

- **JSON Output**: `gws` returns JSON by default. Use `jq` to parse it if needed.
- **File IDs**: Most operations require a `fileId`. Use the list or search commands to find it.

## Gotchas

- **`Error 401: invalid_client`**: If `gws auth login` fails with *"The OAuth client was not found"*, `~/.config/gws/client_secret.json` likely contains the unedited template placeholder (`YOUR_CLIENT_ID...`) seeded during installation. Replace that file with a valid desktop client secret from Google Cloud Console, or run `gws auth setup --login`.
- **Rate Limits**: Google APIs have rate limits. Avoid making hundreds of requests in a tight loop.
- **Scopes**: If you get "Insufficient Permission" errors, you may need to re-run `gws auth login` and ensure all requested scopes are approved.
- **Binary Format**: When downloading non-Google formats (like PDFs or Word docs), use appropriate flags or helpers to handle the binary stream.
- **Large Docs**: Extremely large documents might hit token limits if read in their entirety. Consider reading them in chunks or summarizing if supported by the underlying API.
