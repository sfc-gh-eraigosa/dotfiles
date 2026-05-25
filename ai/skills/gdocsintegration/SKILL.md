---
name: google-docs
description: Integration for interacting with Google Docs and Google Workspace using the 'gws' CLI.
---
# Google Docs Integration Skill

This skill allows Gemini and Claude to interact with Google Workspace (Docs, Drive, Gmail, etc.) using the `gws` CLI.

## Capabilities

- **Read Docs**: Read content from Google Docs.
- **Search Drive**: Search for files in Google Drive.
- **Manage Files**: Create, update, or delete files in Workspace.

## Usage

### 1. Authenticate
Ensure you are authenticated with Google Workspace:
```bash
gws auth login
```

### 2. Common Commands

- **List Drive files**:
  ```bash
  gws drive files list
  ```
- **Read a Doc**:
  ```bash
  gws drive files get --fileId <FILE_ID> --alt media
  ```
- **Search for a Doc by name**:
  ```bash
  gws drive files list --q "name = 'My Document' and mimeType = 'application/vnd.google-apps.document'"
  ```

## Guidelines

- **JSON Output**: `gws` returns JSON by default. Use `jq` to parse it if needed.
- **File IDs**: Most operations require a `fileId`. Use the list or search commands to find it.

## Gotchas

- **Rate Limits**: Google APIs have rate limits. Avoid making hundreds of requests in a tight loop.
- **Scopes**: If you get "Insufficient Permission" errors, you may need to re-run `gws auth login` and ensure all requested scopes are approved.
- **Binary Format**: When downloading non-Google formats (like PDFs or Word docs), use appropriate flags or helpers to handle the binary stream.
- **Large Docs**: Extremely large documents might hit token limits if read in their entirety. Consider reading them in chunks or summarizing if supported by the underlying API.
