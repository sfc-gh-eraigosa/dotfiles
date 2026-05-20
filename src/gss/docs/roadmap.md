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
