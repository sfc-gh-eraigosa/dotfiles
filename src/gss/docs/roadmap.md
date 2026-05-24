# gss Roadmap — post-v1 items

This file collects design and configuration entries for capabilities
that are **deliberately out of scope for v1.0** but committed to the
project's future direction. Items here are not blockers; they're
intentionally deferred decisions that nonetheless need a documented
home so future contributors don't re-litigate them.

For v1.0 itself, see [`design.md`](./design.md). The "Roadmap" section
inside `design.md` cross-links here.

---

## WORKER.md secret scanning

**Status**: deferred from v1.0 (open-question C resolved as option 3 —
out of scope; rely on GitHub's native secret scanning).

**Why it's deferred**: GitHub already runs server-side secret scanning
on every push and on draft-PR content. For v1.0, that's the safety net.
Shipping a local pre-push scanner in `gss` is a non-trivial dependency
choice (which regex set? entropy-based? language-aware?) that we don't
need to make until real-world incidents tell us where the gaps are.
The "draft PR is the safety net" stance for worker mode is preserved.

**What v1.0 does** (unchanged from `design.md`):

- `gss feature checkpoint` renders `WORKER.md` and `FEATURE.md` content
  verbatim into the PR body.
- Marker tokens (`<!-- gss:* -->`) authored in user content are stripped
  before stitching (per the hardening checklist) so the cross-link
  rewrite can't be subverted.
- No regex, no entropy check, no allow/deny list runs locally.
- The user is expected to enable GitHub repo-level secret scanning;
  detected leaks surface as GitHub alerts post-push.

**What a future version (post-v1) will add**

When the team chooses to enable a local scanner, the behaviour adopted
will be **strip-and-warn** (the option 2 that was on the table for C):

- Before stitching `WORKER.md` / `FEATURE.md` into the PR body and
  before the body is written to the draft PR, scan each line against a
  configurable pattern set.
- Lines that match are *redacted in place* — replaced with a stable
  token like `[gss-redacted: <pattern-name>]` — and the original line
  appended to a synthesized `## Redacted` section at the bottom of the
  `WORKER.md` (visible to the agent in the worktree only; never
  surfaced into the PR body).
- A one-line stderr notice during `checkpoint` reports the redaction
  count and pattern names hit, so the agent knows to fix `WORKER.md`
  before the next checkpoint.
- The render still completes (non-blocking), so a single accidental
  paste doesn't strand the worker.

### Configuration (when enabled)

The config block will live under `secrets:` in
`~/.config/gss/config.yaml`. All keys optional; v1.0 ignores the block
entirely. The first version to *honour* the block will bump the
config `schema_version` so users know when it becomes active.

```yaml
# Reserved for a future gss release; ignored in v1.0.
secrets:
  scan_mode: off             # off | strip-and-warn | reject
                             #   off              (v1.0 default): no local scan.
                             #   strip-and-warn   (committed future default once enabled):
                             #                    redact matched lines, append to
                             #                    WORKER.md ## Redacted, render
                             #                    continues, exit 0 with stderr notice.
                             #   reject           (opt-in stricter mode): refuse to
                             #                    render the PR body, exit non-zero,
                             #                    agent must edit WORKER.md before
                             #                    the next checkpoint.

  patterns:
    # Each pattern is name + regex. Built-in pack ships with sensible
    # defaults (AWS access keys, GitHub PATs, generic high-entropy 40+
    # char tokens, env-style KEY=VALUE for known secret KEYs).
    # Users can extend or replace via the mode below.
    builtin: true            # include the gss-shipped default set
    mode: append             # append | replace
                             #   append (default): user patterns add to built-ins
                             #   replace: user patterns are the only set
    list:                    # optional user patterns
      - name: my-org-token
        regex: 'MYORG_[A-Z0-9]{32}'

  redaction_token: "[gss-redacted: %s]"   # %s = matched pattern name
  redacted_section_header: "## Redacted"  # in WORKER.md
```

### Behaviour when `scan_mode != off`

- `gss feature checkpoint` and `gss feature checkpoint --auto` both
  run the scanner on every render. The scanner is **on the local
  render path only** — it never inspects the git working tree, only
  the text being rendered into the PR body.
- The scanner runs *after* marker-token stripping (so a redacted line
  inside a stripped marker isn't double-counted) but *before* the
  stack-section rewrite.
- For `--auto` (process hook), redaction never blocks: even in
  `reject` mode, `--auto` falls back to `strip-and-warn` behaviour so
  the close-hook can't strand the worker.
- For interactive `checkpoint`, `reject` exits non-zero with a
  structured error (`ErrSecretsDetected`, new in the
  `internal/errors/` set) listing the matched pattern names and the
  file/line refs in `WORKER.md`.

### Implementation pointer (when prioritised)

- New package `internal/secrets/` with `Scan(text) []Match` and
  `Render(text, mode) (redacted string, matches []Match)`.
- Default pattern set lives at `internal/secrets/patterns.yaml`
  embedded via `//go:embed`.
- License audit: the pattern set itself is original to this project
  (no copy of `gitleaks`, `trufflehog`, or other GPL'd corpora);
  cite this in the introducing PR.
- Tests use `testdata/secrets/{aws,gh,env,clean}.txt` fixtures and
  table-drive the three modes.

### Out of scope even for the future version

- **Network-based scanners** (calling out to a secrets-detection
  service). gss stays self-contained.
- **Auto-rotation** (revoking a leaked credential, opening a fixing
  PR). That's another tool's job; gss only detects + redacts in the
  PR body.
- **Inspecting git tree content** beyond what's being rendered. The
  scanner is a render-time filter, not a pre-commit hook. Git
  pre-commit hooks (e.g. `gitleaks` installed by the user) remain
  the right tool for tree-wide scanning.

---

## Cross-machine sync

**Status**: explicit non-goal for v1.0 (open-question E resolved as
options 2 + 3 combined — silent best-effort behaviour, plus a
[`gss feature audit`](./design.md#gss-feature-audit) recovery tool).

**The scenario**: a user syncs `~/.config/gss/` across machines (via
Dropbox, restic, rsync, an NFS home, etc.). Two hosts can then edit
the same `registry.json` concurrently, claim overlapping worktree
paths, push commits that the other side doesn't know about, or open
PRs that the other side's registry doesn't reference.

**Why v1.0 doesn't refuse**: refusing on detect (option 1) would
inconvenience the much larger population of single-machine users to
defend against a configuration we explicitly don't support. The
audit tool gives users a way out when they realise their setup
caused divergence, without dictating their backup / sync strategy.

### Failure modes you should expect when syncing

This list is intentional documentation, not a bug report. Each item
is a "yes, that will happen, here's what to do about it":

| Symptom on host B | Likely cause | Recovery |
|-------------------|--------------|----------|
| Registry row points at a worktree directory that doesn't exist locally | Worktree was created on host A, not synced (or synced after host B's last `gss` run cached the absence) | `gss feature audit --repair` drops the orphan row. |
| `gss feature checkpoint` says "PR not found" | PR URL in registry references a PR another host created against a fork or with credentials this host lacks | Re-run `gh auth status` to confirm identity; if right user, retry. If wrong account, `gss feature audit --repair` clears the PR URL and the next checkpoint creates a fresh PR. |
| Two registry rows claim the same `branch` | Both hosts ran `worker add` against the same NWO + name | `gss feature audit` flags as `error`; the user picks one to keep, `gss feature done --force --worker <ref>` removes the loser. |
| `git push --force-with-lease` rejects | Other host advanced the branch | Pull / rebase manually on the worktree, then `gss feature checkpoint`. Audit doesn't try to resolve this — it requires human judgement. |
| `auto-promote-on-merge` fires "for nothing" | Host A merged a bottom; host B's registry didn't know it was the bottom | This is benign — the PR ready-flip is idempotent. Audit will reconcile next run. |

### Setup recommendations for users who do sync anyway

- **Do not sync `~/.config/gss/worktrees/<owner>/<repo>/<feature>/`
  directory contents** (the actual git checkouts). Sync only
  `registry.json` if you must sync anything — the worktrees are git
  state, and git already has a sync mechanism (push/pull). Layering
  Dropbox over git working trees is the loud-failure path.
- **Run `gss feature audit` at the start of every session on a
  fresh host.** Treat it the way you treat `git status` on first
  open.
- **One host = one writer at a time.** Conflicts compound; auditing
  out N hosts of simultaneous edits is much harder than auditing
  out one.

### Possible future hardening (not committed)

If cross-machine sync becomes a more frequent failure mode in
practice, the following are candidate v1.x items, none currently
prioritised:

- Host UUID embedded in `registry.json` writes, with a "last-writer-
  wins / both-readable" merge tool surface.
- Per-host worktree namespacing
  (`~/.config/gss/worktrees/<host>/<owner>/<repo>/...`) so concurrent
  hosts don't collide on disk paths.
- A `gss feature audit --watch` mode that runs in the background and
  surfaces drift via a desktop notification.

---

## Worktree backend options

**Status**: the `Backend` interface ships in v1.0 (PR-20/21) with exactly
**one** implementation — the `git` backend. Every alternative below is
doc-only / `// TODO` for v1.0. The interface (see
[design.md → Worktree backend abstraction](./design.md#worktree-backend-abstraction))
was deliberately shaped so each of these is "one new sub-package + one
`worktree.Register(<name>, New)` call" with **zero changes to callers** —
`checkpoint`, `rebase`, `push`, and PR plumbing keep operating on a plain
git repo at the worktree path.

### Why alternative backends at all

The `git` backend runs `git worktree add` per worker: a full working tree
on disk. For a few workers on a small repo that's fine. It stops being
fine when:

- the repo is large (a monorepo worker is O(GB); spin-up is
  seconds-to-minutes),
- you want **dozens** of ephemeral workers (fan-out orchestration),
- you want stronger isolation than "same uid, different directory" — i.e.
  the agent code is untrusted.

### The hard contract every backend must preserve

A backend may virtualize the inodes underneath, but the path it returns
**must be a working git repo**: `git status` / `git log` / `git commit` /
`git push` behave normally at that path. The rest of `gss` is
backend-agnostic and must stay that way. Any approach that breaks native
git semantics at the worktree path is disqualified, however fast it is.

### Candidate backends

#### 1. `git` — shipped in v1.0

Thin wrapper over `git worktree add/remove/list` + `git status
--porcelain=v2 -b`. Zero dependencies, correct git semantics for free,
portable across Linux/macOS/WSL. The baseline against which the others
are judged. Cost: O(repo size) disk per worker, slow spin-up on big
repos.

#### 2. `overlayfs` — kernel copy-on-write (Linux)

Maintain one shared **lower** layer (a bare or full base clone); each
worker gets its own **upper** + **work** dirs, kernel-mounted as an
overlay at the worker path. Copy-on-write: unchanged files are shared,
only edits consume space.

- **Pros**: O(changed-files) disk, fast spin-up, one shared object DB.
- **Cons**: Linux-only (macOS has no kernel overlay); needs
  `CAP_SYS_ADMIN` or unprivileged user namespaces; any background
  `git gc` on the lower layer must be coordinated; whiteout-file handling
  on teardown.
- **License gate (critical)**: use the **kernel** overlay filesystem via
  `mount -t overlay` / `mount(2)` inside a user namespace (Linux ≥ 5.11
  supports unprivileged overlay in a userns). Do **not** depend on
  **`fuse-overlayfs`** — it ships under **GPL-2.0** (verify against its
  upstream `LICENSE` before any work), which is **banned** for anything
  we vendor or recommend in a skill (see
  [src/CLAUDE.md → Banned licenses](../../src/CLAUDE.md)). The kernel
  filesystem is the OS, not a linked dependency, so it carries no license
  obligation on our statically-linked binary; the GPL userspace helper
  does and is therefore out.

#### 3. `sandboxfs` — Bazel's FUSE symlink filesystem ("bazel-based")

The Bazel team built **`sandboxfs`**
([github.com/bazelbuild/sandboxfs](https://github.com/bazelbuild/sandboxfs),
**Apache-2.0** ✓ — verify the upstream `LICENSE`), a FUSE filesystem that
materializes a tree from a declarative mapping near-instantly: read-only
entries pass through to a shared base, writable entries land in a
per-worker scratch dir. As a gss backend: one shared base checkout (the
"lower"), each worker a sandboxfs mount that passes through unchanged
files and copies-on-write only what an agent actually touches.

- **Pros**: near-instant create, O(touched-files) disk, works on Linux
  **and** macOS (via FUSE / macFUSE), Apache-2.0 (the reason it is
  preferable to `fuse-overlayfs` for the COW story).
- **Cons**: FUSE runtime dependency (macFUSE install friction; some hosts
  forbid FUSE); the mapping must present a real `.git` so git tooling
  works; `sandboxfs` is maintenance-light upstream.
- **License gate**: `sandboxfs` itself is clean (Apache-2.0). **macFUSE**
  is **review-required** — recent macFUSE releases are *not* clearly
  OSI-permissive; pin to a permissive version after verifying its
  `LICENSE`, or treat macOS COW as unsupported. The "bazel-based" idea
  more broadly is a CAS-backed symlink forest (Bazel's own output-tree
  trick); `sandboxfs` is the concrete, license-clean way to get there
  without us reimplementing a content store.

#### 4. `dockerfs` — container-overlay + optional process isolation

Two flavors, increasing isolation:

- **(a) fs-only**: use a container runtime's `overlay2` graph driver to
  get the same COW story as `overlayfs`, managed through container
  tooling rather than raw mounts.
- **(b) full sandbox**: the worker's worktree is a bind-mount / volume
  inside a throwaway container **and the agent process itself runs inside
  that container**. This is the only backend that isolates the process
  tree + network + filesystem, not just the filesystem — the right call
  for untrusted agent code.

- **Runtime**: Docker Engine (**Apache-2.0** ✓) or Podman (**Apache-2.0**
  ✓, rootless-first) — verify each upstream `LICENSE`.
- **Pros**: strongest isolation, reproducible per-worker toolchain, works
  on macOS/Windows through the runtime's VM.
- **Cons**: daemon dependency, heaviest spin-up, image/toolchain
  management overhead.
- **License / security gates**:
  - Docker **Desktop** (the macOS/Windows bundle) carries **commercial
    licensing terms** — a skill must recommend the Apache-2.0 **engine**
    or **Podman**, never Desktop.
  - Rootless container stacks frequently pull in **`fuse-overlayfs`
    (GPL-2.0, banned)** — prefer the kernel `overlay` snapshotter or the
    `native` snapshotter.
  - Mounting the Docker socket into anything an agent controls is a
    privilege-escalation surface; document it loudly and default to
    rootless.

#### 5. `tmpfs` — RAM-backed scratch trees

A `git` (or overlay) worktree whose path lives on `tmpfs`. Fastest IO,
zero persistence — dies on reboot, so only for genuinely disposable
workers; **checkpoint-to-PR before teardown is mandatory**. Smallest
change of all (compose the `git` backend with a tmpfs mount point).
Capacity-bounded by RAM.

### Comparison matrix

| Backend | OS support | Extra dep (license) | Disk/worker | Spin-up | Isolation | Ships in |
|---|---|---|---|---|---|---|
| `git` | Linux/macOS/WSL | none | O(repo) | slow on big repos | uid + dir | **v1.0** |
| `overlayfs` | Linux only | kernel overlay (none vendored) | O(changed) | fast | uid + dir + COW | post-v1 |
| `sandboxfs` | Linux/macOS (FUSE) | sandboxfs (Apache-2.0) + FUSE runtime | O(touched) | very fast | uid + dir + COW | post-v1 |
| `dockerfs` | Linux/macOS/Win (VM) | Docker/Podman engine (Apache-2.0) | O(changed) | slowest | process + net + fs | post-v1 |
| `tmpfs` | Linux/macOS | none (RAM) | O(repo), in RAM | fastest IO | uid + dir, ephemeral | post-v1 |

### Selecting & configuring a backend

`worktree.backend` sets the default; `--backend <name>` on
`gss feature start` / `worker add` overrides per-call (already specced in
[design.md → Selecting a backend](./design.md#selecting-a-backend)). The
creating backend is persisted per-worker in `registry.json`
(`Info.Backend`), so teardown always uses the backend that made the
worktree — flipping config never retroactively migrates a live worker.

Backend-specific tuning lives under a `worktree.backends.<name>:`
sub-block (reserved; v1.0 ignores it):

```yaml
# Reserved for a future gss release; ignored in v1.0.
worktree:
  backend: git              # git (v1.0) | overlayfs | sandboxfs | dockerfs | tmpfs
  backends:
    overlayfs:
      base_clone: ~/.config/gss/base/<owner>/<repo>   # shared lower layer
      userns: true                                     # unprivileged mount
    sandboxfs:
      base_clone: ~/.config/gss/base/<owner>/<repo>
      sandboxfs_bin: sandboxfs                         # must resolve on PATH
    dockerfs:
      runtime: podman        # podman (rootless, preferred) | docker
      image: gss-worker:latest
      isolate_process: true  # (b) run the agent inside the container
      mount_docker_socket: false   # never default-on; escalation surface
    tmpfs:
      size: 2g               # per-worker cap; bounded by host RAM
```

### License gate (read before implementing ANY of these)

Per [src/CLAUDE.md → Library standards](../../src/CLAUDE.md), every
companion CLI or library must be Allowed/Flag-for-review and verified
against its **actual upstream `LICENSE`** (badges/marketing don't count):

- ✅ **sandboxfs**, **Docker Engine**, **Podman** — Apache-2.0.
- ❌ **fuse-overlayfs** — GPL-2.0; never vendor or recommend. Use kernel
  overlay instead.
- ⚠️ **macFUSE** — review-required; recent releases are not clearly
  OSI-permissive. Pin a permissive version or drop macOS COW support.
- ✅ **kernel overlayfs / tmpfs** — these are the OS, not a linked
  dependency; no obligation on our binary.

### Out of scope even post-v1

- **Network filesystems as a backend** (NFS/SMB worktrees) — git
  semantics over a network FS are a known footgun.
- **Remote / distributed worktrees** (the worker tree on another host) —
  that's the cross-machine story, tracked under
  [Cross-machine sync](#cross-machine-sync), not the backend abstraction.
- **Auto-migrating live workers between backends** — `Info.Backend` is
  immutable for a worker's lifetime by design; migration = `done` the old
  worker, `worker add` a new one.
