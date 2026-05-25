---
name: google-docs
description: Integration for interacting with Google Docs and Google Workspace using the 'gws' CLI.
---
# Google Docs Integration Skill

This skill allows Gemini to interact with Google Workspace (Docs, Drive, Gmail, etc.) using the `gws` CLI.

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
