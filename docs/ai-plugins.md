# AI Assistant Plugins & Extensions

This repository provides a **unified, declarative framework** for managing the capabilities of your AI assistants. By using a single source of truth, we ensure that both **Claude Code** and **Antigravity CLI** (`agy`) have the tools they need to be effective while staying reproducible across different machines.

- **Manifest**: [`ai/plugins.yaml`](../ai/plugins.yaml)
- **Infrastructure Hub**: [`ai/AGENTS.md`](../ai/AGENTS.md)
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
Plugins are installed from Anthropic's official marketplace (`claude-plugins-official`), plus any extra marketplaces listed in `plugins.yaml` (e.g. `karpathy-skills` ← `forrestchang/andrej-karpathy-skills`).
- **Manage**: `claude plugin list` / `install` / `enable`; `claude plugin marketplace add <owner/repo>`.

### Antigravity CLI
Plugins are git-source based, installed via `agy plugin install <source>`. Legacy Gemini CLI extension repositories (e.g., the `gemini-cli-extensions` organization) remain valid sources.
- **Manage**: `agy plugin list` / `install` / `enable`; `agy plugin import gemini|claude` imports a legacy setup.

---

## 🛠️ The Plugin Catalog

We maintain a mapping of equivalent capabilities across both assistants.

| Feature / Tool | Claude Plugin | Antigravity Plugin Source |
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
| **Karpathy Guidelines** | `andrej-karpathy-skills` (marketplace `karpathy-skills`) | — (Claude-only) |

## 📖 Feature Highlights

### **superpowers — Workflow Intelligence**
Provides the core skills that drive *how* work gets done: brainstorming, writing/executing plans, TDD, systematic debugging, and requesting code reviews. 
- **Claude**: Automatically activates via the `Skill` tool.
- **Antigravity**: Automatically activates via `agy`'s skill mechanism.

### **code-review — Quality Assurance**
Focused correctness review of the current diff or a specific PR.
- **Try**: `/code-review` (Claude) or use the code-review agent (Antigravity).

---

## ➕ Adding a New Plugin

To add an extension to your environment, add a row to [`ai/plugins.yaml`](../ai/plugins.yaml):

```yaml
  - name: my-new-tool
    enabled: true
    claude: { plugin: my-tool@claude-plugins-official }
    antigravity: { source: https://github.com/owner/repo }
```

After editing, run `sync-plugins`. The sync engine handles the platform-specific installation commands (e.g., running `agy plugin install <source>` non-interactively with a timeout guard).
