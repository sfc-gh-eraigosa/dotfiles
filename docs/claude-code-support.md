# Claude Code — supported version range & upgrade guide

This repo provisions `~/.claude` for a **fleet of hosts running different Claude Code
versions**. This doc is the source of truth for which versions the config must work on,
the compatibility rules that keep one shared config safe across them, and the runbook
for auditing new Claude Code releases.

## Supported version range

| Bound | Version | Why | Last verified |
|---|---|---|---|
| **Floor** | `v2.1.195` | Newest host that predates `/rc` (Remote Control, introduced v2.1.196, 2026-06-29) | 2026-08-08 |
| **Latest verified** | `v2.1.226` | Primary WSL host (`claude --version`; CLI auto-updater tracks the `latest` channel) | 2026-08-09 |

When a floor host is upgraded, raise the floor here and delete any version guards the new
floor makes unnecessary. When auditing a new release, update "Latest verified" + the log below.

## Compatibility rules (why one config works on both)

1. **Unknown `settings.json` keys are ignored** by older versions (at worst a startup
   warning). Additive keys are safe to provision fleet-wide.
2. **Unknown hook events never fire** on versions that predate them — wiring a new event
   (e.g. `DirectoryAdded`) is inert on the floor version, active on latest.
3. **Never pin `model` in shared config.** Model aliases (`claude-opus-5`,
   `claude-fable-5`) don't exist on older versions and error when forced. `model` stays a
   host-local key; the template and forced subset must never carry it.
4. **New commands/flags need a version guard in skills.** A skill that runs `/rc`,
   `claude --remote-control`, or any post-floor CLI surface must probe
   `claude --version` first (see `ai/skills/remote-claude-session`).
5. **Verify key names against the live schema, not changelog prose.** The 2026-08-08
   audit caught three plausible-but-wrong keys from changelog-based research
   (`sandbox.paths[].mode` → actually `sandbox.credentials.files[]`;
   `crossSessionInbound` / `dialogExpiry` → don't exist; the real key is
   `isolatePeerMachines`). The `update-config` skill dumps the full authoritative
   schema for the installed version — use it as the gate before provisioning any key.
6. **Some sandbox keys are user/managed-scope only** (`sandbox.network.strictAllowlist`,
   `sandbox.filesystem.disabled`, TLS termination) — project `.claude/settings.json` is
   ignored for them. Provision them via `~/.claude/settings.json` (this repo's job).
7. **The forced subset stays minimal**: `apply-forced-settings.sh` replaces only
   `hooks`, `statusLine`, `permissions.deny`, `permissions.ask` (and unions
   `permissions.allow`). New always-enforced surfaces go there; per-host defaults go in
   `settings.json.template`; everything else is host-owned.

## Behavioral deltas inside the current range (no config surface, but affects workflows)

- `/code-review` runs as a **background subagent** since v2.1.218 — output arrives async.
- `/verify` and `/code-review` **no longer auto-run** after tasks (v2.1.215) — invoke
  explicitly.
- `/ultrareview` is now `/code-review ultra` (old alias still works).
- First push of a new branch: `gss push` handles `--set-upstream` (see the gss skill).

## Upgrade audit runbook

Run this whenever a floor/latest host changes, or on demand ("bring the config current"):

1. **Establish the window.** `claude --version` on every fleet host → floor and latest.
   Diff the window against the official changelog:
   <https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md>
2. **Enumerate config-surface changes** in the window: new `settings.json` keys, hook
   events, skill/plugin format fields, CLI flags, deprecations. Docs:
   <https://code.claude.com/docs>. Skip pure-UX changes.
3. **Classify each** as: additive key (safe) / new hook event (safe-inert) / new
   command-flag (needs skill version guard) / behavior change (document) / deprecation
   (migrate).
4. **Verify exact key names against the live schema** (rule 5 above) before writing any
   JSON. Changelog prose gets key names wrong.
5. **Apply by tier**: forced subset for always-enforced hooks/permissions; template for
   new-host defaults; skill edits (with version guards) for new CLI surfaces; this doc
   for behavior deltas.
6. **Test**: `make lint-portability` for any new hook scripts, run the hook test drivers
   (`ai/hooks/*_test.sh`), `make skill-evals` for skill edits.
7. **Record**: update the version table above and append to the adoption log below.

## Adoption log

| Date | Change | Config surface | Min version | Status |
|---|---|---|---|---|
| 2026-08-08 | `DirectoryAdded` hook → `dir_added_guard.sh` (warns on sensitive dirs) | `settings.forced.json` hooks | v2.1.221 (inert below) | adopted |
| 2026-08-08 | `isolatePeerMachines: true` (approval gate for cross-machine SendMessage via Remote Control) | `settings.json.template` | v2.1.224-verified (ignored below) | adopted |
| 2026-08-08 | Inert `sandbox` block: `enabled: false` + `credentials.files` deny for `~/.ssh`, `~/.aws`, `~/.gnupg`; `network.strictAllowlist` pre-set | `settings.json.template` | v2.1.221 (ignored below) | parked (flip `enabled` per host; populate `allowedDomains` first, and mind gss paths — `~/.config/gss` must stay readable) |
| 2026-08-08 | Version probe in `remote-claude-session` skill — the `--remote-control` flag needs ≥v2.1.154 (all fleet hosts OK), `/rc` slash command ≥v2.1.196, `claude remote-control` server mode ≥v2.1.200 | `ai/skills/remote-claude-session/SKILL.md` | v2.1.154 | adopted |
| 2026-08-08 | Plugin source `sha`/`ref` pinning noted as supply-chain hardening candidate for `sync-plugins` | `ai/plugins.yaml` comment | v2.1.224-verified | candidate |
| 2026-08-08 | `skillListingMaxDescChars` noted — caps per-skill description tokens in every session; pairs with the frontmatter-compression follow-up in the [progressive-disclosure design](./mbo/designs/2026-08-07-progressive-disclosure.md) | — | v2.1.224-verified | candidate |
