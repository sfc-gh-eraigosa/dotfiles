# gss — Git Safe Sync (`sdk/gss`)

`gss` is the canonical commit + push path for this repo: safe sync, dirty-repo
scanning, approval-gated pushes, and feature-worktree orchestration.

- **Module path:** `github.com/sfc-gh-eraigosa/dotfiles/sdk/gss`
- **Binary:** `gss` (installed to `~/opt/bin/gss` by `install.sh`)
- **External install:** `go install github.com/sfc-gh-eraigosa/dotfiles/sdk/gss@<tag>` (tags: `sdk/gss/vX.Y.Z`)
- **Build:** `bash sdk/gss/build.sh` (injects version via `-ldflags -X .../sdk/gss/internal/version.*`)
- **Skill:** [`skill/SKILL.md`](./skill/SKILL.md) drives the `gss` / `git-safe-sync` agent skill.
- **License/dep gate:** `scripts/check-deps.sh` (CI runs it as `GSS_STRICT_CHECK=1`).

> `tmux-mgr` invokes the installed `gss` binary at runtime, so always rebuild
> (`build.sh`) after changing gss source. See [README](./README.md) and [docs/](./docs/).
