# Local Binaries (opt/bin)

This directory is reserved for compiled binaries and local executable artifacts. It is ignored by Git to prevent repository pollution.

## Binaries

These tools are built from source (typically in `src/`) and deposited here during installation or updates:

- `gss`: Git Safe Sync.
- `tmux-mgr`: Tmux Management tool.
- `wol`: Wake-on-LAN utility.
- `discuss`: Communication tool.
- `vault`: HashiCorp Vault binary.
- `git-sizer`: Git repository analyzer.
- `docker` / `docker-compose`: **Committed cross-platform shims, not binaries.** Real executables so non-interactive shells (make, scripts) resolve a working docker — the `.bash_aliases` docker.exe fallback is an alias and only exists interactively. Resolution order: Docker Desktop's WSL-integration Linux CLI (self-gating: mount only exists on WSL) → any other real docker on PATH via `type -aP` (the whole story on macOS / plain Linux / Raspberry Pi) → `docker.exe` over interop (self-gating: binfmt files only exist on WSL; credential helper dir appended to PATH). The failure message is platform-aware: WSL gets the Desktop/interop advice, everything else gets "install Docker Engine". `docker-compose` delegates to `docker compose` (v2).
- `docker-credential-desktop.exe`: **Committed WSL shim.** Docker Desktop's integration writes `credsStore: "desktop.exe"` into `~/.docker/config.json` on every init and symlinks `/usr/bin/docker-credential-desktop.exe` into the `/Docker/host/bin` mount — which never appears on some machines, so every build/pull dies with "error getting credentials". This wrapper (ahead of `/usr/bin` on PATH) forwards the credential-helper protocol to the real Windows helper over interop.

## Committed scripts

Hand-written shell tools tracked in this directory (not build artifacts):

- `claude-pick`: **Repo picker for Claude sessions.** Discovers every git repo under `$HOME/github`, `$HOME/git`, `$HOME/.herdr/worktrees` and `$HOME/.config/gss/worktrees` — matching `.git` of *any* type, so **git worktrees are included** (in a worktree `.git` is a file, not a directory) — offers them via `fzf` (else a numbered menu) plus a clone-a-new-repo entry, then starts Claude there in a tmux session named `claude-<repo>`, reattaching if it already exists. **Sets no launch flags**: the pane runs a bare `claude` under `$SHELL -ic`, so the `claude()` wrapper in `ai/claude/aliases.sh` owns `--remote-control` (via `claude-config remote on`), YOLO and the `tmux-mgr pane anchor`. Setting them here too would duplicate `--remote-control` and clobber the explicit session name the wrapper passes on purpose. Session names are sanitized (`.` and `:` → `-`) because tmux rewrites those characters when it *creates* a session but not when it *looks one up*. Needs a terminal; `--print <target>` emits the command instead for non-interactive callers, `--list` just prints the discovered repos. Tests: `claude-pick_test.sh`.
- `claude-rc-boot`: unattended sibling of `claude-pick` — starts a named `claude --remote-control` session at WSL logon, idempotently. No picker, always `$HOME`. See [docs/claude-rc-autostart.md](../../docs/claude-rc-autostart.md). Tests: `claude-rc-boot_test.sh`.

## Usage

This directory should be in your `$PATH`. It is populated by the `./install.sh` script or individual project build scripts.
