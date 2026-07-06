# AI Assistant MCP Servers

This repo provisions **standalone MCP servers** (Model Context Protocol) the same
declarative, reproducible way it provisions plugins — but as a **separate concern**.

- **Manifest**: [`ai/mcp.yaml`](../ai/mcp.yaml)
- **Sync engine**: [`opt/scripts/system/sync-mcp.sh`](../opt/scripts/system/sync-mcp.sh)
- **Companion test**: `opt/scripts/system/sync-mcp_test.sh`
- **Design / spec / plan**: [`docs/mbo/designs/notebooklm-mcp.md`](./mbo/designs/notebooklm-mcp.md) · [spec](./mbo/specs/notebooklm-mcp.md) · [plan](./mbo/plans/notebooklm-mcp.md)

## Why a separate manifest from `ai/plugins.yaml`?

`ai/plugins.yaml` installs Claude **marketplace plugins** (`claude plugin install`) and
Gemini **git extensions** (`gemini extensions install`). A raw **MCP server** is a different
thing: it's registered with `claude mcp add` / `gemini mcp add`, lands in a different config
file (`~/.claude.json` / `~/.gemini/settings.json`), and has different idempotency and runtime
semantics. Following the repo's "one mirrored sync engine per concern" pattern (`sync-skills`,
`sync-plugins`, `sync-teams`), MCP servers get their own manifest + `sync-mcp` engine.

## The sync workflow

`install.sh` registers the servers listed in the manifest automatically (right after
`sync-plugins`). Re-sync manually any time:

```bash
# Preview (no changes; previews even when the claude/gemini CLIs are absent)
sync-mcp.sh --dry-run

# Register the enabled servers at USER scope for Claude + Gemini
sync-mcp.sh
```

**Ensure-only**: it registers what's listed and never removes anything. Idempotent —
re-runs report `(<name> already registered …)` and make no changes. Servers are registered at
**user scope** (`--scope user`), so they're global and project-independent (not bound to the
directory `install.sh` happened to run from).

## The catalog

| Server | Package | Tools (sample) | Hosts |
| :-- | :-- | :-- | :-- |
| `notebooklm` | [`notebooklm-mcp@2.0.0`](https://github.com/PleasePrompto/notebooklm-mcp) | `ask_question`, `add_source`, `generate_audio`, notebook library CRUD | desktop only (needs Node ≥18 + Chrome) |

## Adding a server

Add a row to [`ai/mcp.yaml`](../ai/mcp.yaml), then run `sync-mcp.sh`:

```yaml
servers:
  - name: my-server
    enabled: true
    transport: stdio                 # stdio only (see Security)
    command: npx
    args: ["-y", "my-server-mcp@1.2.3"]   # PIN the version — never @latest
    claude: {}                       # present (even empty) => register for Claude
    gemini: {}                       # present (even empty) => register for Gemini
```

---

## notebooklm — first-time setup (one-time, manual)

`install.sh`/`sync-mcp.sh` only **register** the server (write its config entry). They do **not**
log you in. NotebookLM access needs a one-time Google sign-in that you run yourself:

1. **Prerequisites (desktop host):** Node ≥18 and Chrome/Chromium installed, with a display
   (WSL needs WSLg). Headless hosts (Pi/Jetson/CI) can't complete the browser login.
2. **Authenticate once** — run the server's `setup_auth` tool from your assistant (it opens a
   **visible Chrome**; log into your Google account). Cookies are persisted in a per-user Chrome
   profile **outside this repo**:
   - Linux: `~/.local/share/notebooklm-mcp/chrome_profile/`
   - macOS: `~/Library/Application Support/notebooklm-mcp/chrome_profile/`
   - Windows: `%APPDATA%\notebooklm-mcp\chrome_profile/`
3. **Use it** — `ask_question`, `add_source`, etc. To reset: `re_auth` / `cleanup_data`.

### Headless / non-desktop hosts

Registration is a harmless config write that succeeds everywhere, so `install.sh` never fails on
a Pi/Jetson/CI runner. The server simply won't connect there (no Chrome) — auth and usage are
unavailable until you run it on a desktop host. To skip registration entirely on a given host,
set `enabled: false` for the row.

---

## Security

`notebooklm-mcp` is a **third-party** package that drives a real browser and holds your **Google
session**. The repo's defaults are deliberately conservative — keep them:

- **Version is pinned, never `@latest`.** `@latest` would run unpinned third-party code (with
  reach over your Google session) on every start. Bump the pin in `ai/mcp.yaml` only after review.
- **stdio transport only.** Never run it with `--transport http --host 0.0.0.0`: that exposes an
  *unauthenticated* endpoint that proxies your Google session to the whole network.
  `ai/hooks/safety_guard.sh` blocks the `0.0.0.0` bind as defense-in-depth. Loopback
  (`127.0.0.1`) and stdio are fine.
- **Install = register only; auth is a separate, manual step.** No installer or sync ever runs
  `setup_auth`/the browser login — an unattended installer must never launch a credential flow.
- **The Chrome profile is a live Google session.** It stays at its default `$HOME` path, **never
  inside the repo** (`.gitignore` has defensive rules so a mistakenly-relocated profile can't be
  committed). Treat it as a top-tier credential: `0700` perms, exclude it from backups/cloud-sync,
  and revoke via Google → Security → "Sign out of all sessions" + the server's `cleanup_data`.
- **Runtime data egress.** Tools like `add_source` / `ask_question` move data to *your* Google
  account under the persisted session, and `download_audio` writes files — these run at assistant
  runtime (not covered by the PreToolUse hooks). Drive them only with content you intend to share
  with Google NotebookLM.
