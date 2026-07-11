# Gemini Extension Integration — Plan

**Status:** Draft
**Date:** 2026-05-25
**Owner:** Gemini CLI Agent

## Objective

Update the AI plugin manifest (`ai/plugins.yaml`) to include Gemini CLI extensions as equivalents to the existing Claude plugins. This allows `sync-plugins.sh` to automatically install and enable extensions for both assistants.

## Research Findings

The Gemini CLI supports extensions via `gemini extensions install <source>`, where `<source>` is typically a GitHub repository URL. A curated gallery exists at `https://geminicli.com/extensions`.

### Mapping Claude Plugins to Gemini Extensions

| Claude Plugin | Gemini Equivalent | Gemini Source (GitHub URL) |
| :--- | :--- | :--- |
| `superpowers` | `superpowers` | `https://github.com/obra/superpowers` |
| `github` | `conductor` | `https://github.com/gemini-cli-extensions/conductor` |
| `code-review` | `code-review` | `https://github.com/gemini-cli-extensions/code-review` |
| `skill-creator` | `agent-creator` | `https://github.com/jduncan-rva/gemini-agent-creator` |
| `pr-review-toolkit` | `code-review` | `https://github.com/gemini-cli-extensions/code-review` |
| `remember` | `mcp-local-memory` | `https://github.com/Beledarian/mcp-local-memory` |
| `mcp-apps` | `mcp-toolbox` | `https://github.com/gemini-cli-extensions/mcp-toolbox` |

*Note: For AWS-specific plugins, no direct 1-to-1 equivalents were found in the featured list, but Gemini-native GCP extensions (Vertex, Cloud Run) are available if needed in the future.*

## Implementation Plan

### 1. Update `ai/plugins.yaml`
Modify the manifest to add `gemini: { source: ... }` blocks to the relevant plugins.

### 2. Verify `sync-plugins.sh`
The current implementation of `sync_gemini` in `opt/scripts/system/sync-plugins.sh` already supports `gemini.source`. 

```bash
sync_gemini() {
    # ...
    run gemini extensions install "$source"
    # ...
}
```

### 3. Testing
- Run `sync-plugins --dry-run` to ensure Gemini extensions are correctly parsed.
- Run `sync-plugins` (manual) to verify installation.

## Proposed Changes to `ai/plugins.yaml`

```yaml
plugins:
  - name: superpowers
    enabled: true
    claude: { plugin: superpowers@claude-plugins-official }
    gemini: { source: https://github.com/obra/superpowers }
  - name: github
    enabled: true
    claude: { plugin: github@claude-plugins-official }
    gemini: { source: https://github.com/gemini-cli-extensions/conductor }
  - name: code-review
    enabled: true
    claude: { plugin: code-review@claude-plugins-official }
    gemini: { source: https://github.com/gemini-cli-extensions/code-review }
  - name: skill-creator
    enabled: true
    claude: { plugin: skill-creator@claude-plugins-official }
    gemini: { source: https://github.com/jduncan-rva/gemini-agent-creator }
  - name: pr-review-toolkit
    enabled: true
    claude: { plugin: pr-review-toolkit@claude-plugins-official }
    gemini: { source: https://github.com/gemini-cli-extensions/code-review }
  - name: remember
    enabled: true
    claude: { plugin: remember@claude-plugins-official }
    gemini: { source: https://github.com/Beledarian/mcp-local-memory }
  - name: mcp-apps
    enabled: true
    claude: { plugin: mcp-apps@claude-plugins-official }
    gemini: { source: https://github.com/gemini-cli-extensions/mcp-toolbox }
```
