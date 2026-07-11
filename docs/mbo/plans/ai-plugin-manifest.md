# AI Plugin Manifest — Design Spec

**Status:** Draft (design approved, pending spec review)
**Date:** 2026-05-25
**Owner:** Edward Raigosa

## Problem

The 12 Claude Code plugins this machine relies on (superpowers, github,
code-review, skill-creator, claude-md-management, pr-review-toolkit, gopls-lsp,
remember, deploy-on-aws, aws-serverless, aws-core, mcp-apps) are enabled only in
`ai/claude/settings.json` — which is **gitignored** (per-host runtime state).
A fresh clone therefore does **not** reproduce the plugin set: `install.sh`
never installs or enables them. We want a tracked, declarative source of truth
that `install.sh` consumes, structured so the same file can also describe
Gemini CLI extensions as equivalents appear.

## Goals

- One tracked manifest is the source of truth for AI-assistant plugins.
- `install.sh` installs + enables the listed Claude plugins on a fresh clone.
- A standalone `sync-plugins` command re-runs the same logic anytime.
- The manifest schema supports per-platform attributes (`claude:`, `gemini:`)
  so Gemini extensions can be added later without a format change.

## Non-Goals (YAGNI)

- **No pruning/uninstall** of plugins not in the manifest (ensure-only/additive).
- **No version pinning** per plugin (track latest from the marketplace).
- **No real Gemini sources yet** — the Gemini code path exists but is a no-op
  until rows declare `gemini.source`.
- Claude's marketplace plugins and Gemini's git-URL extensions are **separate
  ecosystems**; most current plugins are Anthropic-only with no Gemini analogue.
  Shared *skills* are already synced independently via `sync-skills`.

## Design Decisions (from brainstorming)

| Decision | Choice |
|----------|--------|
| Compatibility model | One manifest, per-platform nested blocks |
| Format | YAML (rich per-platform attributes) |
| Sync behavior | Ensure-only (additive); never removes |
| Manifest path | `ai/plugins.yaml` (cross-assistant, tracked via `!ai/**`) |
| Command | `sync-plugins` (script + alias), called from `install.sh` |
| YAML parser | mikefarah `yq` — the `jq`-analogue, added as a core tool |

## Components

### 1. Manifest — `ai/plugins.yaml`

Tracked (covered by the `!ai/**` allowlist rule). Schema:

```yaml
# AI-assistant plugins/extensions — source of truth.
# Ensure-only: install.sh installs + enables what's listed; never removes anything.
marketplaces:
  claude-plugins-official:
    claude: anthropics/claude-plugins-official   # arg to `claude plugin marketplace add`

plugins:
  - name: superpowers                            # logical/display name
    enabled: true                                # true => install + enable
    claude: { plugin: superpowers@claude-plugins-official }
    notes: "Skill framework + brainstorming/TDD/etc."
  - name: aws-core
    enabled: true
    claude: { plugin: aws-core@claude-plugins-official }
  # Future Gemini equivalent (illustrative — none today):
  # - name: some-tool
  #   enabled: true
  #   gemini: { source: https://github.com/owner/repo }
```

Semantics:

- `enabled: true` → install **and** enable for each platform block present.
- `enabled: false` → **parked**: documented but not installed/enabled. (Keeps
  ensure-only clean — we never force-disable.)
- A missing `claude:` / `gemini:` block → that tool is skipped for that row.
- `marketplaces` maps a marketplace id to its per-platform source so the sync
  script can add it before installing.

Seed content: the 12 plugins currently enabled in `ai/claude/settings.json`,
all `claude-plugins-official`, all `enabled: true`.

### 2. `yq` provisioning (mikefarah Go build)

Why not the apt `yq`: on Debian/Ubuntu/WSL the apt `yq` package is the
**kislyuk Python** wrapper with incompatible syntax. We standardize on the
**mikefarah** Go `yq` everywhere. This mirrors the existing `sops` pattern.

- **`opt/profiles/packages.tsv`**: add row `yq   -` (brew installs mikefarah yq
  on macOS; apt column `-` because the distro package is the wrong tool).
- **`opt/scripts/system/install_yq.sh`**: a clone of `install_sops.sh`.
  - Fetches the official release binary into `~/opt/bin` on Linux/WSL.
  - OS map: `Linux→linux`, `Darwin→darwin`.
  - Arch map: `x86_64→amd64`, `arm64|aarch64→arm64`, `armv7l→arm`
    (the `armv7l→arm` case adds 32-bit Raspberry Pi support beyond what
    `install_sops.sh` covers).
  - Asset name: `yq_${OS}_${ARCH}` from the mikefarah/yq releases.
  - Idempotent: probes `yq --version` and exits early if present (so it
    no-ops on macOS after brew, and repairs a dangling symlink otherwise).
- **`install.sh`**: invoke `install_yq.sh` right beside the sops step, with the
  same `|| echo "WARNING: ... continuing."` guard.

### 3. `opt/scripts/system/sync-plugins.sh`

The ensure-only engine. Mirrors `sync-skills.sh` conventions (standalone,
idempotent, safe to re-run).

Flow:

1. Resolve `BASE_DIR`; locate `ai/plugins.yaml`.
2. Require `yq` on PATH; error with a clear pointer to `install_yq.sh` if absent.
3. **Claude path** (skipped entirely if `claude` is not on PATH):
   - For each entry under `marketplaces` with a `claude` source:
     `claude plugin marketplace add <source>` (idempotent).
   - For each `plugins[]` row where `enabled == true` and `claude.plugin` set:
     `claude plugin install <plugin@marketplace>` then `claude plugin enable <plugin>`.
4. **Gemini path** (skipped if `gemini` is not on PATH):
   - For each `enabled` row with `gemini.source`:
     `gemini extensions install <source>`.
   - No-op today: no rows declare `gemini.source`.
5. `--dry-run`: parse the manifest and print every planned action; mutate nothing.

Parsing uses `yq` to emit flat, tab-separated records that a `while IFS=$'\t' read`
loop consumes — keeping the bash side simple.

### 4. Wiring + alias

- **`install.sh`**: call `sync-plugins.sh` in the AI-configuration area, **after**
  `claude_install.sh` (the `claude` CLI must exist first). Guard with a
  non-fatal warning on failure, consistent with neighboring steps.
- **`ai/claude/aliases.sh`**: add a `sync-plugins` alias mirroring `sync-skills`
  (e.g. `alias sync-plugins='bash $HOME/opt/scripts/system/sync-plugins.sh'`),
  resolving the path portably.

### 5. settings.json

- The manifest + CLI become authoritative for *which* plugins exist and are
  enabled. The gitignored per-host `ai/claude/settings.json` keeps
  `enabledPlugins` only as runtime state written by the CLI.
- `settings.json.template` already omits `enabledPlugins` — no change needed.
- The repo no longer hand-curates a plugin list inside settings.

## Data Flow

```
ai/plugins.yaml ──(yq)──> sync-plugins.sh ──> claude plugin marketplace add
                                          └──> claude plugin install + enable
                                          └──> gemini extensions install (future)
install.sh ── calls ──> install_yq.sh (ensure yq) ──> sync-plugins.sh
```

## Error Handling

- Missing `yq`: hard error in `sync-plugins.sh` with the exact remediation
  (`install_yq.sh`); `install.sh` keeps going via its `|| echo WARNING` guard.
- Missing `claude`/`gemini` CLI: that platform's loop is skipped (not an error).
- `claude plugin install` failure on one row: log it and continue to the next
  row (one bad plugin shouldn't abort the whole sync).
- Re-runs are idempotent: marketplace-add, install, and enable are all no-ops
  when already satisfied.

## Testing

- `sync-plugins.sh --dry-run` against the committed manifest: asserts it parses
  and lists all 12 Claude plugins with the right `install`/`enable` actions.
- `install_yq.sh` re-run with `yq` present: clean no-op.
- Manual: on a host without the plugins, a run installs + enables all 12; a
  second run reports everything already satisfied.

## Files Touched

| File | Change |
|------|--------|
| `ai/plugins.yaml` | **new** — the manifest (seeded with 12 plugins) |
| `opt/scripts/system/install_yq.sh` | **new** — mikefarah yq fetch (Linux/WSL) |
| `opt/scripts/system/sync-plugins.sh` | **new** — ensure-only sync engine |
| `opt/profiles/packages.tsv` | add `yq   -` |
| `install.sh` | call `install_yq.sh` (near sops) + `sync-plugins.sh` (after claude install) |
| `ai/claude/aliases.sh` | add `sync-plugins` alias |
| `opt/scripts/system/GEMINI.md` (registry) | document the two new scripts |
