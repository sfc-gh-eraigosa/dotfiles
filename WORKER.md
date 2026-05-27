# ai-plugins: plan

- **Description**: Design spec for the AI plugin manifest
- **Worker**: edward-raigosa/plan
- **Branch**: feature/ai-plugins/edward-raigosa/plan
- **Base branch**: main

## Goal
Author and checkpoint docs/plans/ai-plugin-manifest.md; build out manifest + sync-plugins + yq in later workers

## Decisions & notes

**What:** Design spec (`docs/plans/ai-plugin-manifest.md`) for a declarative,
tracked manifest of AI-assistant plugins/extensions.

**Why:** The 12 Claude Code plugins are enabled only in the gitignored
`ai/claude/settings.json`, so a fresh clone never reproduces them. The manifest
becomes the source of truth that `install.sh` consumes.

**Approved design choices:**
- `ai/plugins.yaml` — one cross-assistant manifest, per-platform nested blocks
  (`claude:` / `gemini:`); tracked via `!ai/**`.
- **Ensure-only** (additive): install + enable what's listed, never remove.
  `enabled: false` = parked (documented, not installed).
- New `opt/scripts/system/sync-plugins.sh` (+ `sync-plugins` alias), invoked by
  `install.sh` after the Claude CLI install. Supports `--dry-run`.
- YAML parsed with **mikefarah `yq`**, added as a core tool: `yq -` in
  `packages.tsv` (brew) + new `opt/scripts/system/install_yq.sh` fetching the
  official binary on Linux/WSL (mirrors `install_sops.sh`; adds `armv7l→arm`).
- Gemini extension path implemented but a no-op today (no rows have sources).

**Scope of this worker:** the spec doc only. Implementation (manifest, scripts,
install.sh wiring) lands in follow-up workers.

## Open questions
- None blocking. Real Gemini extension sources are out of scope until equivalents exist.
