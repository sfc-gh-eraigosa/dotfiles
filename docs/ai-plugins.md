# AI Assistant Plugins

This repo keeps the set of Claude Code plugins **declarative and reproducible**.
The source of truth is [`ai/plugins.yaml`](../ai/plugins.yaml); `install.sh`
installs and enables everything listed there on a fresh clone, and you can
re-sync any time with the `sync-plugins` command.

- **Design:** [`docs/plans/ai-plugin-manifest.md`](plans/ai-plugin-manifest.md)
- **Sync engine:** [`opt/scripts/system/sync-plugins.sh`](../opt/scripts/system/sync-plugins.sh)
- **Marketplace:** all of the plugins below come from Anthropic's official
  marketplace `claude-plugins-official` (`anthropics/claude-plugins-official`).

## Quickstart

```bash
# Preview what would be installed/enabled (no CLIs required beyond yq):
bash opt/scripts/system/sync-plugins.sh --dry-run

# Install + enable everything in the manifest (needs the `claude` CLI on PATH):
sync-plugins            # the alias; same as: bash opt/scripts/system/sync-plugins.sh

# Verify:
claude plugin list
```

The sync is **ensure-only**: it installs and enables what the manifest lists and
never removes anything. Re-running it is safe and quiet — already-enabled
plugins report "already enabled" rather than erroring.

### Adding or removing a plugin

1. Edit [`ai/plugins.yaml`](../ai/plugins.yaml): add a row (or set `enabled: false`
   to "park" one — documented but not installed).
   ```yaml
     - name: my-plugin
       enabled: true
       claude: { plugin: my-plugin@claude-plugins-official }
   ```
2. Run `sync-plugins`.

(Removal is intentionally manual — uninstall with `claude plugin uninstall <name>`
and drop the row. The sync never uninstalls on its own.)

## The plugins we use

| Plugin | What it does | First thing to try |
|--------|--------------|--------------------|
| **superpowers** | Workflow skills that drive *how* work gets done — brainstorming, writing/executing plans, TDD, systematic debugging, code review, git worktrees | Just ask to build something ("let's design X") — it auto-runs brainstorming first |
| **github** | GitHub integration for PRs, issues, and repo operations | "open a PR for this branch" |
| **code-review** | Focused correctness review of the current diff or a PR | `/code-review` |
| **pr-review-toolkit** | Multi-agent PR review: code-reviewer, silent-failure-hunter, type-design-analyzer, comment-analyzer, pr-test-analyzer, code-simplifier | `/pr-review-toolkit:review-pr` |
| **skill-creator** | Create, edit, and benchmark your own skills | `/skill-creator:skill-creator` |
| **claude-md-management** | Audit and improve `CLAUDE.md` files; capture session learnings | `/claude-md-management:revise-claude-md` |
| **remember** | Persistent session memory + clean session handoff | `/remember` |
| **gopls-lsp** | Go language-server integration (definitions, references, diagnostics) for Go code | Open a `.go` file and ask for callers of a function |
| **deploy-on-aws** | Analyze a codebase and deploy it to the right AWS services; AWS architecture diagrams | `/deploy-on-aws:deploy` |
| **aws-serverless** | Lambda (incl. durable functions), API Gateway, Step Functions, SAM/CDK serverless patterns | "build a Lambda behind API Gateway" |
| **aws-core** | Broad AWS skills: CloudFormation, CDK, IAM, observability, containers, Bedrock, billing, SDKs | "review this CloudFormation template" |
| **mcp-apps** | Build MCP servers with interactive UI "apps" (MCP Apps SDK) | `/mcp-apps:create-mcp-app` |

> Tip: list everything currently available in a session with `/help`, or inspect a
> single plugin's components and token cost with `claude plugin details <name>`.

## First-usage examples

**superpowers — let the workflow drive the work.** Most of its skills activate
automatically. Saying "let's build a new CLI for X" triggers the brainstorming
skill, which turns the idea into a spec, then a plan, then implementation. You
can also invoke a skill explicitly, e.g. start a focused debugging session or a
TDD loop when you begin a bugfix.

**code-review — check a change before merging.**
```text
/code-review            # reviews the current branch diff for correctness bugs
/code-review 24         # review GitHub PR #24
```

**remember — save and resume context.**
```text
/remember               # writes session state so the next session can continue cleanly
```
Memory lives under `~/.claude/projects/.../memory/` and is summarized back into
each new session automatically.

**deploy-on-aws — ship an app.**
```text
/deploy-on-aws:deploy   # analyzes the codebase and proposes/executes an AWS deployment
```
Or just ask "estimate the AWS cost of running this" / "generate the infrastructure."

## Gemini CLI

Today the manifest installs **nothing** for Gemini — and that's expected. The 12
plugins above are Claude marketplace plugins; Gemini's extension ecosystem is
separate (git-URL based), and none of these have a Gemini equivalent. Gemini
already shares this repo's *skills* through the `sync-skills` linker (every
`SKILL.md` is linked into both `~/.claude/skills` and `~/.agents/skills`), plus
its own policies and commands under `ai/gemini/`.

The plumbing for Gemini extensions **is** in place, so adding one later is a
one-line change. Give a plugin row a `gemini` block:

```yaml
  - name: some-tool
    enabled: true
    claude: { plugin: some-tool@claude-plugins-official }   # optional
    gemini: { source: https://github.com/owner/repo }       # git URL or local path
```

Then `sync-plugins` runs `gemini extensions install <source>` for it (skipped
automatically when the `gemini` CLI isn't on PATH). Manage them directly with
`gemini extensions list` / `update` / `disable`.
