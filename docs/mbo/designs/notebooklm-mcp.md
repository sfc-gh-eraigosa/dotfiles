# Declarative MCP-server provisioning (notebooklm-mcp) — design

- **Slug:** notebooklm-mcp
- **Date:** 2026-06-19
- **Status:** Proposed
- **Relates to:** PR (this branch) · integrates [PleasePrompto/notebooklm-mcp](https://github.com/PleasePrompto/notebooklm-mcp)
- **Author(s):** Claude (architecture team: sysarch · secarch · principal) + repo owner

## 1. Problem / context

We want NotebookLM access available to both Claude Code and Gemini CLI, provisioned the same
declarative, reproducible way as everything else in this repo. The target,
`PleasePrompto/notebooklm-mcp`, is **not** a Claude marketplace plugin — it is a **standalone
MCP server** registered with `claude mcp add notebooklm -- npx notebooklm-mcp@<ver>` (Gemini:
`gemini mcp add`).

Verified facts grounding this design:
- `ai/plugins.yaml` + `sync-plugins.sh` provision Claude **marketplace plugins** and Gemini
  **git extensions** only. There is **no code path for raw MCP servers** in the repo today
  (confirmed by reading both files — they call `claude plugin install/enable` and
  `gemini extensions install`, never `claude mcp add`).
- `claude mcp add` default scope is **`local`/project** (writes `~/.claude.json` under the cwd) —
  verified against the live CLI. `--scope user` writes the global top-level `mcpServers`.
- `claude mcp add` on a duplicate prints "already exists" and **exits 0**; `gemini mcp add` is
  idempotent ("updated", exit 0) — both verified.
- gsl's `sdk/gsl/internal/mcp/detect.go` counts whatever servers exist in `~/.claude.json`
  **dynamically** — no hardcoded list — so it surfaces a new server automatically (no Go change).
- The server drives a **visible Chrome** for a one-time Google login and persists cookies in a
  per-user Chrome profile; it bundles Patchright (a stealth-automation Playwright fork). Runtime
  needs Node ≥18 + Chrome.

## 2. Goals & non-goals

**Goals**
- A declarative, ensure-only manifest + engine that registers standalone MCP servers for Claude
  + Gemini at fresh-`install.sh` time, mirroring the repo's existing sync engines.
- Integrate `notebooklm` as the first server, safely.

**Non-goals**
- Running the interactive Google auth (a manual runtime step — never scripted).
- Supporting HTTP/SSE transport (stdio only).
- Any Go change to gsl (its MCP count is already data-driven).

## 3. Options considered

- **(A) New `ai/mcp.yaml` + `opt/scripts/system/sync-mcp.sh`** (+ companion test), mirroring
  `sync-plugins.sh`. **← recommended.** Matches the repo's "one mirrored sync engine per concern"
  pattern (`sync-skills`/`sync-plugins`/`sync-teams`). Raw MCP servers are a distinct concern
  (different verbs, config files, idempotency, runtime) and the schema grows fields plugins don't
  have (transport, pinned version, scope).
- **(B) Extend `ai/plugins.yaml` + teach `sync-plugins.sh`.** Rejected: couples two unrelated
  lifecycles and churns `sync-plugins_test.sh`'s hardcoded plugin/extension counts; mixes
  marketplace-TUI guards with MCP concerns.
- **(C) One-off non-declarative install line.** Rejected: abandons the declarative/reproducible
  ethos; invisible to a "how is this host configured" review.

**Language:** bash (not Go). The engine is a thin idempotent shell-out over `claude mcp` /
`gemini mcp`, identical in shape to the three sibling engines, and reuses their entire hardening
toolbox. gsl needs no change. (The user asked to involve the go team; the honest finding is there
is no Go *implementation* work — so the go team's role is a *verification* leaf: confirm gsl's
detector surfaces the new server.)

## 4. Decision

Build **Option A** in bash:
- `ai/mcp.yaml` — `servers:` list; per-row `enabled`, `transport: stdio`, `command`, `args`
  (pinned version), and presence blocks `claude: {}` / `gemini: {}`.
- `opt/scripts/system/sync-mcp.sh` — ensure-only engine, structurally cloned from
  `sync-plugins.sh` (setsid+timeout `GUARD`, `</dev/null`, `yq` precheck, per-tool CLI-absent
  skip, all-failures-non-fatal, `--dry-run`). Registers at **`--scope user`**. Idempotent via
  tolerate-"already exists" (no `mcp list` precheck — `list` can connect/spawn the server, which
  would violate the no-auth-at-install boundary). Refuses non-stdio transport.
- `opt/scripts/system/sync-mcp_test.sh` — hermetic fake-CLI test (real `yq`), 28 assertions.
- `install.sh` — non-fatal call right after the `sync-plugins` block.
- Security: pinned `@2.0.0`, stdio-only, register-only, defensive `.gitignore` for the Chrome
  profile, a `safety_guard.sh` deny rule for `notebooklm-mcp --host 0.0.0.0`.
- Docs: `docs/ai-mcp.md` + cross-refs.

## 5. Risks & blast radius

- **Scope bug (highest):** missing `--scope user` would bind the server to install.sh's cwd.
  Mitigated by hardcoding `--scope user` and asserting it in the test.
- **Supply chain:** `notebooklm-mcp` is third-party, browser-automating, Google-auth software.
  Mitigated by a **pinned** version (never `@latest`), reviewed on bump.
- **Credential exposure:** the Chrome profile is a live Google session. Kept at its default
  `$HOME` path; defensive `.gitignore` prevents an in-repo relocation from being committed.
- **Headless hosts:** Pi/Jetson/CI can't run Chrome. Registration is a harmless config write that
  succeeds; auth/usage simply unavailable; install never fails.
- Blast radius of the new files is additive; the only shared-file edits are an additive
  `install.sh` block, two GEMINI.md bullets, an additive `.gitignore` block, and one new
  `safety_guard.sh` rule (test-first per repo policy).

## 6. Rollback

Delete `ai/mcp.yaml`, `opt/scripts/system/sync-mcp*.sh`, `docs/ai-mcp.md`; revert the additive
`install.sh` / `.gitignore` / `safety_guard.sh` / GEMINI.md hunks. Already-registered servers are
left in place (ensure-only never removes); a user removes one with `claude mcp remove --scope user
notebooklm` / `gemini mcp remove --scope user notebooklm` if desired.

> Produced via an architecture-team fan-out (sysarch/secarch/principal). Spec: `../specs/notebooklm-mcp.md`.
