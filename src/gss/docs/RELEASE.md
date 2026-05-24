# gss v1.0 — Release & Migration Note

This is the one-page release note for the `gss` v1.0 refactor, landed by
the final integration PR (**PR-61** in [`plan.md`](./plan.md)). It is the
document a user upgrading from a pre-refactor `gss` should read first.

For the full design see [`design.md`](./design.md); for post-v1 direction
see [`roadmap.md`](./roadmap.md).

---

## What v1.0 is

v1.0 re-platforms `gss` onto a tested `internal/` package set and adds a
**parallel-feature workflow** (`gss feature`) on top of the classic
commit/push/PR tool. Three things changed:

1. **The classic commands were rewired, not rewritten.** `gss push`,
   `gss pr`, `gss scan`, `gss status`, and `gss sync` now delegate to
   tested `internal/` packages. Their **output is byte-identical** to the
   old tool — the [stable output strings](./plan.md#stable-output-strings)
   that the `/sync`, `/gss-pr`, `/gss-scan`, and `/gss` slash commands
   grep against are preserved and golden-tested.

2. **`gss feature` is new.** An 11-verb subtree (`start`, `worker add`,
   `list`, `checkpoint`, `conflicts`, `pr`, `rebase`, `restack`, `done`,
   `merged`, `audit`) manages parallel feature work in isolated worktrees,
   each producing its own draft PR. See
   [design.md → gss feature](./design.md).

3. **`tmux-mgr` delegates worktree lifecycle to `gss`.** `gss` is now the
   **sole owner** of agent worktrees; `tmux-mgr`'s old `pkg/workspace/`
   was removed.

## Migration steps (upgrading from pre-refactor `gss`)

1. **Rebuild the binary.** Run `src/gss/build.sh`. This is **required**:
   `tmux-mgr` shells out to `gss feature worker add --json`, and a stale
   `gss` (without `--json`) fails with a cryptic `unknown flag: --json`.
   If you see that error, rebuild — see carry-forward #16 in
   [`STATE.md`](./STATE.md).

2. **Classic workflows need no changes.** `/sync`, `/gss-pr`,
   `/gss-scan`, `/gss` keep working unchanged — same flags, same output.

3. **Initialize config (optional).** `gss` reads
   `~/.config/gss/config.yaml`. Run `gss config print` to write a
   first-run stub and dump the effective config; `gss config check`
   validates the file and confirms `git` / `gh` resolve on `PATH`.
   Missing config falls back to built-in defaults — nothing breaks if you
   skip this.

4. **Adopt legacy tmux-mgr sessions.** If you have agent sessions created
   by the pre-refactor `tmux-mgr`, run
   `tmux-mgr internal migrate-to-gss` (idempotent, supports `--dry-run`)
   to adopt them into the `gss` registry. New sessions need no migration.

5. **New dependencies are all permissive.** `gopkg.in/yaml.v3` (MIT +
   Apache-2.0), `golang.org/x/text` (BSD-3-Clause), `github.com/gofrs/flock`
   + `golang.org/x/sys` (BSD-3-Clause). All Allowed per
   [src/CLAUDE.md → Library standards](../../src/CLAUDE.md); `go.sum`
   carries no transitive copyleft.

## Release gate (verified before PR-61 merges)

- [ ] `go test ./...` clean
- [ ] `go-licenses check ./...` clean (CI gate, PR-50)
- [ ] `safety_guard_test.sh` — all cases pass
- [ ] Manual smoke of `/sync`, `/gss-pr`, `/gss-scan`, `/gss` against the
      freshly built binary

## Known gaps (non-blocking, tracked)

These are logged in [`STATE.md`](./STATE.md) carry-forward #11–16 and are
intentionally deferred:

- `gss feature worker update` not yet wired (#11).
- `gh` login precedence not used for user resolution; falls through to
  git email → `$USER` (#12).
- `gss feature start --purpose` one-shot convenience not implemented
  (#13).
- `gss feature checkpoint --message` not supported (#14).
- `gss feature conflicts` requires `--feature` rather than defaulting to
  all features (#15).
- No runtime `gss` version/capability check from `tmux-mgr`; a stale
  binary fails loudly but cryptically (#16) — mitigated by the
  `e2e-gss-integration.sh` regression guard.

## What's deliberately out of scope for v1.0

See [`roadmap.md`](./roadmap.md): local secret scanning, cross-machine
sync hardening, and alternative worktree backends (overlayfs, sandboxfs,
dockerfs, tmpfs). The `git` worktree backend is the only one that ships.
