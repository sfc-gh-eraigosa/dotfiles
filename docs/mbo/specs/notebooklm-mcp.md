# Declarative MCP-server provisioning (notebooklm-mcp) — spec

- **Slug:** notebooklm-mcp
- **Date:** 2026-06-19
- **Status:** Draft
- **Relates to:** design `../designs/notebooklm-mcp.md` · PR (this branch)

## 1. Goal

On a fresh `install.sh` (or a manual `sync-mcp.sh`), every standalone MCP server listed in
`ai/mcp.yaml` is registered — at **user scope** — for both Claude Code and Gemini CLI, ensure-only
and idempotent. The first server is `notebooklm` (NotebookLM access). Registration writes config
only; the interactive Google sign-in stays a manual, user-run step.

## 2. Use cases

- **Fresh bootstrap.** *Actor:* operator running `install.sh`. *Trigger:* the post-plugins step.
  *Flow:* `sync-mcp.sh` reads the manifest → `claude mcp add --scope user …` + `gemini mcp add
  --scope user …` for each enabled server. *Acceptance:* notebooklm appears in `~/.claude.json`
  top-level `mcpServers`; install never blocks/fails; no browser opens.
- **Re-run / idempotency.** *Trigger:* re-running `install.sh` or `sync-mcp.sh`. *Flow:* both CLIs
  report already-registered. *Acceptance:* quiet `(notebooklm already registered …)`, no WARNING,
  no duplicate, exit 0.
- **Headless host.** *Actor:* Pi/Jetson/CI. *Flow:* registration succeeds (config write) though no
  Chrome exists. *Acceptance:* exit 0; auth/usage unavailable but not an install failure.
- **Preview.** *Trigger:* `sync-mcp.sh --dry-run`. *Acceptance:* prints the planned `claude mcp
  add` / `gemini mcp add` lines even with the CLIs absent; changes nothing.
- **First-time auth (manual).** *Actor:* user on a desktop. *Flow:* runs the server's `setup_auth`
  tool → visible Chrome → Google login → cookies persisted outside the repo. *Acceptance:* never
  triggered by install/sync.

## 3. Architecture

- `ai/mcp.yaml` — declarative source of truth (`servers:` list; mikefarah `yq`).
- `opt/scripts/system/sync-mcp.sh` — ensure-only engine; per-tool register functions; clone of
  `sync-plugins.sh` hardening. Independently testable via `SYNC_MCP_MANIFEST` override.
- `opt/scripts/system/sync-mcp_test.sh` — hermetic test (fake claude/gemini on a temp PATH; real
  `yq`; fixture manifests).
- `install.sh` — non-fatal call site after `sync-plugins`.
- `ai/hooks/safety_guard.sh` — deny rule for the `0.0.0.0` bind.

Data flow: manifest → yq → per-server `claude mcp add --scope user <name> -- <command> <args>` /
`gemini mcp add --scope user --transport stdio <name> <command> <args>`.

## 4. Behavior / features

1. Register enabled servers at user scope for each present tool block.
2. Idempotent: tolerate "already exists"/"already configured" as a quiet skip (no precheck that
   could connect/spawn the server).
3. Ensure-only: never `mcp remove`; `enabled: false` rows are skipped, not removed.
4. stdio only: refuse (warn + skip) any non-stdio transport.
5. Pinned version (manifest data), never `@latest`.
6. Non-interactive hardening: setsid+timeout `GUARD`, `</dev/null`, SIGKILL escalation.
7. Preconditions: `yq` absent → exit 1 (+ install_yq.sh pointer); CLI absent → graceful skip;
   dry-run previews regardless.
8. `safety_guard.sh` blocks `notebooklm-mcp … --host 0.0.0.0`.

## 5. Evaluation criteria (per feature) — every rule is a test in `sync-mcp_test.sh` / `safety_guard_test.sh`

| Feature | Fires | Must-not-fire | Pass |
| :-- | :-- | :-- | :-- |
| user-scope add | `--dry-run` plans `claude mcp add --scope user notebooklm -- npx -y notebooklm-mcp@2.0.0` | never plain `mcp add` without `--scope user` | "plans the Claude add at user scope…" |
| pinned version | manifest uses `@2.0.0` | `@latest` never appears in plan | "never registers an unpinned @latest version" |
| gemini add | plans `gemini mcp add --scope user --transport stdio notebooklm …` | — | "plans the Gemini add…" |
| parked | `enabled:false` server omitted from plan | parked name absent | "skips the parked (enabled:false) server" |
| stdio-only | `transport: http` → WARNING, no add | never registers http server | "warns on a non-stdio transport" |
| tool-presence | only the present `claude:`/`gemini:` block registers | no gemini add when block absent | "does not register Gemini when no gemini block…" |
| idempotent | re-run → `(notebooklm already registered …)` | no WARNING; no duplicate | "tolerates … as a quiet skip" + "no WARNING on idempotent re-run" |
| ensure-only | recorder shows no `mcp remove` | — | "ensure-only: never emits a remove" |
| CLI-absent | skip line + exit 0 | — | "exits 0 when the CLIs are absent" |
| yq-absent | exit 1 + install_yq.sh pointer | — | "exits 1 when yq is absent" |
| timeout `-k` | SIGTERM-ignoring add is SIGKILLed; overall exit 0 | no hang | "escalates to SIGKILL…" |
| setsid | add runs under setsid; pty case stays headless | no /dev/tty hang | "runs mcp add under setsid" + "headless-ok" |
| macOS keg setsid | brew keg setsid discovered when off PATH | — | "discovers keg-only util-linux setsid…" |
| 0.0.0.0 deny | `safety_guard` exit 2 on `notebooklm-mcp --host 0.0.0.0` | exit 0 on stdio / 127.0.0.1 / registration | safety_guard_test notebooklm cases |

## 6. Verification harness

- `bash opt/scripts/system/sync-mcp_test.sh` → `FAIL=0` (28 assertions; hermetic).
- `bash ai/hooks/safety_guard_test.sh` → the 5 notebooklm cases pass (full suite green on a
  regular checkout; the 2 gss-worktree-detection cases are environmental when run from inside a
  worktree).
- `make lint-shell` (shellcheck `-x -S warning`) clean — auto-discovers the new `*.sh`.
- `sync-mcp.sh --dry-run` works with the CLIs absent.

## 7. Prerequisites / dependencies

mikefarah `yq` (already required by `sync-plugins`); the Claude/Gemini CLIs (per-tool optional);
Node ≥18 + Chrome at the server's *runtime* (not at install).

## 8. Out of scope (and why)

- HTTP/SSE transport — security (unauthenticated session-proxy risk).
- Auto-auth — must never script a credential browser flow in an unattended installer.
- gsl display changes — its MCP count already picks up the new server; any distinct rendering is a
  separate gsl ticket.

## 9. Rollback

See design §6. Ensure-only never removes; manual `mcp remove --scope user` if needed.

> Produced alongside the architecture-team design. Plan: `../plans/notebooklm-mcp.md`.
