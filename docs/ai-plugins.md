# AI Assistant Plugins & Extensions

This repository provides a **unified, declarative framework** for managing the capabilities of your AI assistants. By using a single source of truth, we ensure that both **Claude Code** and **Gemini CLI** have the tools they need to be effective while staying reproducible across different machines.

- **Manifest**: [`ai/plugins.yaml`](../ai/plugins.yaml)
- **Infrastructure Hub**: [`ai/GEMINI.md`](../ai/GEMINI.md)
- **Sync Engine**: [`opt/scripts/system/sync-plugins.sh`](../opt/scripts/system/sync-plugins.sh)

## 🔄 The Sync Workflow

The `install.sh` script automatically provisions the extensions listed in the manifest. You can also re-sync manually at any time using the `sync-plugins` command.

```bash
# Preview the sync (dry-run)
sync-plugins --dry-run

# Execute the sync for the active assistant
sync-plugins
```

The synchronization is **ensure-only**: it installs and enables what is listed but never removes existing extensions.

## 🤖 Supported Assistants

### Claude Code
Plugins are installed from Anthropic's official marketplace (`claude-plugins-official`). 
- **Manage**: `claude plugin list` / `install` / `enable`.

### Gemini CLI
Extensions are git-URL based and sourced from repositories (e.g., the `gemini-cli-extensions` organization).
- **Manage**: `gemini extensions list` / `install` / `update`.

---

## 🛠️ The Plugin Catalog

We maintain a mapping of equivalent capabilities across both assistants.

| Feature / Tool | Claude Plugin | Gemini Extension Source |
| :--- | :--- | :--- |
| **Workflow Skills** | `superpowers` | `https://github.com/obra/superpowers` |
| **GitHub Integration** | `github` | `https://github.com/gemini-cli-extensions/conductor` |
| **Code Review** | `code-review` | `https://github.com/gemini-cli-extensions/code-review` |
| **Agent Creation** | `skill-creator` | `https://github.com/jduncan-rva/gemini-agent-creator` |
| **Local Memory** | `remember` | `https://github.com/Beledarian/mcp-local-memory` |
| **MCP Toolbox** | `mcp-apps` | `https://github.com/gemini-cli-extensions/mcp-toolbox` |
| **PR Toolkit** | `pr-review-toolkit` | (covered by code-review) |
| **Go LSP** | `gopls-lsp` | (native or via MCP) |
| **AWS Suite** | `aws-core`, `aws-serverless` | (GCP native equivalents available) |

## 📖 Feature Highlights

### **superpowers — Workflow Intelligence**
Provides the core skills that drive *how* work gets done: brainstorming, writing/executing plans, TDD, systematic debugging, and requesting code reviews. 
- **Claude**: Automatically activates via the `Skill` tool.
- **Gemini**: Automatically activates via the `activate_skill` tool.

### **code-review — Quality Assurance**
Focused correctness review of the current diff or a specific PR.
- **Try**: `/code-review` (Claude) or use the code-review agent (Gemini).

---

## ➕ Adding a New Plugin

To add an extension to your environment, add a row to [`ai/plugins.yaml`](../ai/plugins.yaml):

```yaml
  - name: my-new-tool
    enabled: true
    claude: { plugin: my-tool@claude-plugins-official }
    gemini: { source: https://github.com/owner/repo }
```

After editing, run `sync-plugins`. The sync engine handles the platform-specific installation commands (e.g., using `--consent` and `--skip-settings` for Gemini to ensure a non-interactive bootstrap).
