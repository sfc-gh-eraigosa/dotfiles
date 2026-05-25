# Evaluation: Google Workspace Integration

This document provides instructions for evaluating the Google Docs and Workspace integration skill.

## Goal
Verify that the `google-docs` skill and the `gws` CLI are correctly configured and can:
1.  **Search** for files in Google Drive.
2.  **Read** content from a Google Doc.
3.  **Perform** Workspace operations (Gmail, Calendar, etc.) assuming full API access.

## Evaluation Steps (Verification Script)

### 1. Prerequisites
Ensure you are authenticated:
```bash
gws auth status
# or if not authenticated:
gws auth login
```

### 2. Search & List (Drive)
List files in Drive to verify basic connectivity:
```bash
gws drive files list --params '{"pageSize": 5}'
```
**Success Criteria**: Returns a JSON list of 5 files from Drive.

### 3. Read Document (Docs)
Identify a Google Doc File ID from the previous step and try to read its content:
```bash
gws drive files get --fileId <FILE_ID> --alt media
```
**Success Criteria**: Returns the raw text/content of the document.

### 4. Cross-Service Verification (Gmail/Calendar)
Verify that other services are accessible under the same authentication:
- **Gmail**: `gws gmail threads list --params '{"maxResults": 1}'`
- **Calendar**: `gws calendar calendarList list`

## Pass/Fail Criteria
- [ ] `gws` is installed and in the PATH.
- [ ] Authentication via `gws auth login` (or `client_secret.json`) is successful.
- [ ] Drive search returns valid file metadata.
- [ ] Docs content can be retrieved.
- [ ] Permissions cover Gmail, Calendar, and Sheets as expected.

## Gotchas for Evaluators
- **Scopes**: If any command fails with "Insufficient Permission", re-authenticate and ensure all Workspace scopes were granted during the OAuth flow.
- **Quota**: Be mindful of Google API quotas during intensive evaluation.
