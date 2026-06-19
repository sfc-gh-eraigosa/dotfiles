# Declarative MCP-server provisioning (notebooklm-mcp) — implementation plan

- **Slug:** notebooklm-mcp
- **Date:** 2026-06-19
- **Status:** In-progress
- **Relates to:** spec `../specs/notebooklm-mcp.md` · design `../designs/notebooklm-mcp.md` · PR (this branch)

## 1. Summary & verdict

Add a declarative, ensure-only engine (`ai/mcp.yaml` + `sync-mcp.sh`) that registers standalone
MCP servers for Claude + Gemini at user scope, and integrate `notebooklm-mcp@2.0.0` as the first
server. Built bash-first, mirroring `sync-plugins.sh`. The architecture team's must-fixes are all
addressed: **`--scope user`** (hardcoded + tested), **pinned version** (not `@latest`),
**stdio-only** (refused otherwise + a `safety_guard` 0.0.0.0 deny), **register-only / never auth**
(no `mcp list` precheck that could spawn the server), and **profile-never-in-repo** (defensive
`.gitignore`). go-team role = verify gsl surfaces the new server (no Go code change needed).

## 2. File inventory

| Path | Purpose | Implements |
| :-- | :-- | :-- |
| `ai/mcp.yaml` | manifest; `notebooklm` row (pinned, stdio, claude+gemini) | spec §3, §4 |
| `opt/scripts/system/sync-mcp.sh` | ensure-only engine (clone of sync-plugins hardening) | spec §4, §6 |
| `opt/scripts/system/sync-mcp_test.sh` | hermetic fake-CLI test (28 assertions) | spec §5, §6 |
| `install.sh` (edit) | non-fatal call after the sync-plugins block | spec §2 |
| `.gitignore` (edit) | defensive re-ignore of `**/chrome_profile/` | design §5 |
| `ai/hooks/safety_guard.sh` (edit) | deny `notebooklm-mcp --host 0.0.0.0` (Section 9) | spec §4.8 |
| `ai/hooks/safety_guard_test.sh` (edit) | 5 notebooklm cases (3 allow / 2 deny) | spec §5 |
| `docs/ai-mcp.md` | human catalog + setup + security | spec §1 |
| `docs/ai-plugins.md` (edit) | cross-ref to ai-mcp.md | — |
| `ai/GEMINI.md` (edit) | manifest table + Key Commands | — |
| `opt/scripts/system/GEMINI.md` (edit) | `sync-mcp.sh` registry bullet | — |
| `GEMINI.md` (edit) | Repository Structure `ai/mcp.yaml` bullet | — |
| `docs/mbo/{designs,specs,plans}/notebooklm-mcp.md` | MBO artifacts | — |
| `docs/mbo/index.md` (edit) | register the objective | — |

## 3. Interface contracts

- Manifest: `servers: [ { name, enabled, transport: stdio, command, args: [...], claude:{}, gemini:{} } ]`.
- Engine emits, per enabled server + present tool block:
  - `claude mcp add --scope user <name> -- <command> <args...>`
  - `gemini mcp add --scope user --transport stdio <name> <command> <args...>`
- Idempotency: capture output; if it matches `already exists`/`already configured` → quiet skip.
- Testability hook: `SYNC_MCP_MANIFEST=<path>` overrides the manifest.

## 4. TDD build order

1. **Manifest + engine + test** — write `sync-mcp_test.sh` cases, then `sync-mcp.sh` + `ai/mcp.yaml`.
   *Done-when:* `bash sync-mcp_test.sh` → FAIL=0; `shellcheck -x -S warning` clean. ✅ (28/28)
2. **safety_guard 0.0.0.0 deny** — add test cases first (3 allow / 2 deny), then the hook rule.
   *Done-when:* the 5 notebooklm cases pass; hook shellcheck clean. ✅
3. **install.sh wiring** — additive non-fatal block after sync-plugins.
   *Done-when:* `bash -n install.sh` clean; block present. ✅
4. **`.gitignore` defense** — re-ignore `**/chrome_profile/`; verify a relocated profile is
   invisible to git and the manifest/scripts stay tracked. *Done-when:* `git status` confirms. ✅
5. **Docs + GEMINI.md + MBO artifacts + index** — cross-refs and registry. *Done-when:* links
   resolve; index row added. ✅
6. **go-team verification** — confirm `sdk/gsl/internal/mcp` counts the new server (read-only).
   *Done-when:* go-team reports detector is data-driven / adds a fixture if warranted.
   ✅ go-goqa verdict: `ConfiguredCount` = `len(cfg.McpServers)`, no hardcoded list; 18 tests pass,
   74.5% cov; **no gsl code change required** — the new server is counted automatically.
7. **Adversarial review** — architecture-adversary: **SHIP**, 11/11 must-dos confirmed; one
   low-severity safety_guard false-negative (`--host=0.0.0.0` equals/quoted form) **fixed** (regex
   tightened to `[[:space:]=]+["']?` + 2 new test cases). ✅

## 5. Verification mapping

Every spec §5 row maps to a named assertion in `sync-mcp_test.sh` (the "Pass" column quotes the
exact test label) or the notebooklm cases in `safety_guard_test.sh`. See spec §5.

## 6. Integration & rollout

- `make lint-shell` auto-discovers the new `*.sh` (no Makefile edit).
- `install.sh` runs `sync-mcp.sh` after `sync-plugins`; non-fatal.
- Manual acceptance (desktop): run the server's `setup_auth` once, then `ask_question` (see
  `docs/ai-mcp.md`). Not part of CI.

### 6.1 Build leaves / DAG

Built as a **single cohesive PR** (not broken out) — the manifest, engine, test, wiring, and docs
are tightly coupled and small; parallel leaves would trade one PR for several plus rebases. The
only separable unit is the read-only go-team gsl verification, which touches no shared paths and
can run independently. No `gss feature` fan-out for this objective.

> Produced after the architecture-team design. Execute TDD throughout; update `../index.md` state.
