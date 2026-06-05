# Fix gss module path so external `go install` resolves

- **Date:** 2026-06-04
- **Status:** Proposed
- **Relates to:** issue [#114](https://github.com/sfc-gh-eraigosa/dotfiles/issues/114) (*mismatched module path blocks `go install` for external consumers*)
- **Authors:** architecture team (systems architect, with the Go domain architect to own the mechanical rename)
- **Decision owner:** repo owner. Key calls: (1) the go-install-able module path is **`github.com/sfc-gh-eraigosa/dotfiles/src/gss`** — the org rename **and** the `src/` path segment, not the org rename alone that #114 proposes; (2) **public vs. org-private module posture** (§6.1, requires an explicit owner answer before tagging).

---

## 1. TL;DR — what #114 gets right, and the load-bearing correction

Issue #114 reports a real defect: `src/gss/go.mod` declares
`module github.com/wenlock/dotfiles/gss`, but the canonical repository is
`github.com/sfc-gh-eraigosa/dotfiles`. Because the module path does not match the
repository that serves it, **external `go install` cannot consume the tool** — a
consumer who runs `go install github.com/wenlock/dotfiles/gss@latest` (or the
sfc-gh org equivalent) gets a hard failure, not a binary.

The defect is real and independently reproduced. **But the issue's fix is wrong
in a way that would leave `go install` just as broken.** #114 proposes a sed
rename of the *org* only (`wenlock` → `sfc-gh-eraigosa`) plus `go mod tidy`. That
is insufficient: the module physically lives at **`src/gss/`**, while the declared
path `.../dotfiles/gss` implies the package sits at `gss/` under the repo root.
There is no `go.mod` at the repo root. So the org-only target
`github.com/sfc-gh-eraigosa/dotfiles/gss` resolves to *"module found but does not
contain package"* — still broken.

The correct module path is **`github.com/sfc-gh-eraigosa/dotfiles/src/gss`**: the
org rename **plus** the missing `src/` segment. This adopts #114's *intent* and
corrects its *mechanics*.

> **Note on the sibling "convention."** `src/tmux-mgr` already carries the `src/`
> segment, so it is the right exemplar for *structure*. But it declares the org
> `eraigosa` — a **third** org variant, neither `wenlock` nor the canonical
> `sfc-gh-eraigosa`. So tmux-mgr matches the **`src/` segment** convention but is
> **not** a clean exemplar of the **org**; it has its own wrong-org defect and is
> itself not externally installable. The ADR (§9) therefore lists tmux-mgr among
> the siblings to migrate, not as the gold standard.

## 2. Context / Problem (confirmed facts)

All facts below were verified by direct inspection, not taken on trust.

| # | Confirmed fact | How verified |
|---|---|---|
| C1 | `src/gss/go.mod` line 1 is `module github.com/wenlock/dotfiles/gss`. | Direct read of `src/gss/go.mod`. |
| C2 | The git remote is `git@github.com:sfc-gh-eraigosa/dotfiles.git`. The declared module org (`wenlock`) does not match the canonical repo owner. | `git remote -v`. |
| C3 | There is **no `go.mod` at the repo root**; the module lives at `src/gss/`. The declared path `.../dotfiles/gss` implies `gss/` at the repo root — a path that does not exist. | Repo tree inspection. |
| C4 | **100** `.go` files import the `github.com/wenlock/dotfiles/gss` prefix and must be rewritten in lockstep with `go.mod`: **99** under `src/gss/cmd/` + `src/gss/internal/`, **plus `src/gss/main.go`** (the `package main` at the module root, which imports `.../gss/cmd`). The rewrite scope is the **whole `src/gss/` tree**, not just `cmd/`+`internal/`. | `grep -rl 'github.com/wenlock/dotfiles/gss' src/gss --include='*.go' \| wc -l` (=100); `grep -l … src/gss/main.go` (matches). |
| C5 | The `main` package is at the **module root** (`src/gss/main.go`, `package main`). `src/gss/cmd/` is `package cmd` (a library). **There is no `src/gss/cmd/gss/` directory.** Therefore the install target is the module root `.../src/gss`, **not** `.../src/gss/cmd/gss`. | `head src/gss/main.go`; `ls src/gss/cmd/gss` → not found. |
| C6 | The sibling `src/tmux-mgr/go.mod` uses `github.com/eraigosa/dotfiles/src/tmux-mgr` — has the `src/` segment but a **third org variant** (`eraigosa`). `gsl`/`wol` carry wrong-org + missing-`src/`. | `head -3 src/tmux-mgr/go.mod`, `src/gsl/go.mod`, `src/wol/go.mod`. |
| C7 | `github.com/wenlock/dotfiles` returns HTTP 301 → `github.com/sfc-gh-eraigosa/dotfiles` (owner rename redirect). | Reporter-scouted; consistent with C2. See §3.1 for why this does **not** rescue `go install`. |
| C8 | No `gss/v*` (nor `src/gss/v*`) version tag exists. Consumers resolve only to `@latest` pseudo-versions even after the path is corrected. | `git tag` list. |
| C9 | `src/gss/go.sum` exists (committed). A module-path rename changes the module's *own* path, which is not recorded in `go.sum`, so the rename is expected to leave `go.sum` **byte-identical**. | `ls src/gss/go.sum`. |
| C10 | `src/gss/build.sh` lines **35–38** embed the old prefix in **four** `-X` ldflags (`…/gss/internal/version.{Version,Commit,BuildDate,Dirty}`). All four must move. | `grep -n version src/gss/build.sh`. |

**Impact.** External consumers (the issue cites `snowflake-eng/dev-config` across
~18 workspaces — reporter-asserted, unverifiable from here, but the install
blockage itself is proven independent of that number) cannot `go install` gss.
The only workaround today is clone-and-build-from-`src`, or a `replace` directive
in the consumer's `go.mod`.

**Severity: medium.** A clone-and-build workaround exists; no version tag exists
yet so consumers are on pseudo-versions regardless; the asserted blast radius is
unverified. Held above low because the public module identity is genuinely
unusable via the standard `go install` path.

## 3. Root cause — module-path resolution

`go install <path>@<version>` resolves a package by treating the import path as a
**module identity**, then fetching the module and verifying that the module's own
`go.mod` *declares the same path*. Two independent things must line up:

1. **Repository identity** — the host portion (`github.com/<owner>/<repo>`) must
   point at a repo the Go tooling (via the module proxy / `?go-get=1` meta) can
   reach and that serves a module.
2. **Module identity** — the module's `go.mod` must declare a path that the
   requested import path falls under, *and* the package must exist at the implied
   subdirectory within the module.

gss fails **both**:

- The declared org (`wenlock`) is not the canonical repo owner (`sfc-gh-eraigosa`)
  — repository-identity mismatch.
- The declared path omits the `src/` segment where the module actually lives —
  module-identity / subdirectory mismatch.

**Empirically reproduced (proxy reachable — these are genuine path defects, not
network failures):**

| Install target | Result | Meaning |
|---|---|---|
| `…/wenlock/dotfiles/gss@latest` | proxy 404 for the `wenlock` path | wrong org; repo identity unreachable as a module |
| `…/sfc-gh-eraigosa/dotfiles/gss@latest` (the issue's org-only fix) | *"module … found but does not contain package …/gss"* | org fixed, but module lives at `src/gss`, not `gss` — **still broken** |
| `…/sfc-gh-eraigosa/dotfiles/src/gss@latest` | *"module declares its path as: github.com/wenlock/dotfiles/gss"* | reaches the real module; now the only remaining mismatch is the **declared path** inside `go.mod` — exactly what this fix corrects |

The third row is the gold-standard signal: Go found the module at `…/src/gss` and
the *only* thing it objected to was the path string in `go.mod`. Fix that string
to `github.com/sfc-gh-eraigosa/dotfiles/src/gss` and the chain closes.

### 3.1 The 301-redirect nuance — why it does *not* rescue `go install`

`github.com/wenlock/...` 301-redirects to `github.com/sfc-gh-eraigosa/...` (C7).
It is tempting to assume Go follows that redirect and the rename "just works."
It does not:

- The 301 resolves **repository identity** (a browser/git-clone convenience). It
  does **not** rewrite **module identity**. The module's `go.mod` still declares
  `wenlock`, so even a followed redirect ends in the *"module declares its path
  as …"* rejection.
- Go's discovery uses the `?go-get=1` meta endpoint for the **subpath**, and
  `https://github.com/wenlock/dotfiles/gss?go-get=1` returns **HTTP 404** — the
  rename redirect is not honored for the go-get subpath handler. The proxy
  likewise returns 404 for the `wenlock` module path while returning 200 for the
  canonical `sfc-gh-eraigosa` repo.

So the redirect is a **red herring** for this fix. The block is not "the org was
renamed and Go hasn't caught up"; it is "the `go.mod` path string is wrong on two
counts." Relying on the redirect is not a fix. The redirect also has a
**security half-life** — see §6.1.A.

## 4. Decision

**Rename the gss module to `github.com/sfc-gh-eraigosa/dotfiles/src/gss`** and
rewrite every importing file to match.

- This is the org rename #114 asks for **plus** the `src/` segment its diagnosis
  missed. Both are required; the org-only rename is explicitly rejected because it
  leaves `go install` broken (§3, row 2).
- It standardizes gss onto the structural sibling convention
  (`github.com/<owner>/dotfiles/src/<tool>`). The full convention —
  *every module under `src/` declares `github.com/sfc-gh-eraigosa/dotfiles/src/<tool>`*
  (canonical org **and** `src/` segment) — should be recorded in an ADR (§9) so the
  defective siblings (`tmux-mgr`, `gsl`, `wol` — all three carry an org and/or
  `src/` defect) are migrated consistently rather than ad hoc.
- It is paired with a **version tag** (`src/gss/v0.1.0`) and a documented
  external-install command, without which consumers stay on pseudo-versions even
  after the path is correct (C8).

The owner (`sfc-gh-eraigosa`) is hard-coded in the path. This re-breaks `go
install` on any *future* owner rename; a vanity import path (a `go-import` meta
served from a stable domain) would be more durable but is out of scope here and
noted as future work (§8).

## 5. Implementation plan

Mechanical rename routed to the **Go domain architect**; the path/tag/convention
decisions are owned here. Named verification surfaces (and why the rename does or
does not touch each) are listed in §7.

**Phase 0 — baseline.**
- From the repo root: `bash scripts/test.sh` (the canonical runner — gss coverage
  gate **70%**; it discovers the module by **directory name**, not module path, so
  the rename does not affect discovery) to capture a green baseline before
  touching anything.
- `cd src/gss && go build ./...` clean.
- `bash src/gss/scripts/check-deps.sh` (the license/dep gate; CI runs it as
  `GSS_STRICT_CHECK=1`) green — it operates on `./...` and is path-independent, so
  it should stay green across the rename. Capture the baseline now so a post-rename
  failure is unambiguous.

**Phase 1 — rewrite the module path and all imports (whole-module scope).**
- `src/gss/go.mod` line 1 → `module github.com/sfc-gh-eraigosa/dotfiles/src/gss`.
- Sed-rewrite **all 100** importing `.go` files across the **entire `src/gss/`
  tree** — `src/gss/cmd/`, `src/gss/internal/`, **and `src/gss/main.go`** (do not
  scope to `cmd/`+`internal/`; that misses `main.go` and the build will fail):
  `github.com/wenlock/dotfiles/gss` → `github.com/sfc-gh-eraigosa/dotfiles/src/gss`.
  Use a precise, anchored pattern (the old prefix is unique) and re-grep the whole
  module to confirm **zero** residual occurrences.
- Update **all four** `-X github.com/.../gss/internal/version.*` ldflags in
  `src/gss/build.sh` (lines **35–38** — Version, Commit, BuildDate, Dirty). Move
  every one; leaving any embeds a stale prefix.

**Phase 2 — verify the build and tests locally.**
- `cd src/gss && go build ./...` — must be clean.
- `bash scripts/test.sh` — green (same as the Phase 0 baseline).
- `bash src/gss/scripts/check-deps.sh` — green (same as baseline; no new
  transitive licenses, per `src/CLAUDE.md` policy).
- `go mod tidy` — expected **no-op**: the renamed path is the module's own and is
  not recorded in `go.sum`, so `go.sum` must come back **byte-identical** (C9). A
  non-empty `go.sum` diff is a **red flag**, not a wave-through — stop and
  investigate. Optionally `GOFLAGS=-mod=readonly go mod verify` to confirm all
  transitive deps still verify and the rename masked no pre-existing mismatch.
- Rebuild via `src/gss/build.sh` and smoke-test the binary (`gss --version`,
  `gss help`) — `tmux-mgr` shells out to the installed gss, so a stale/broken
  binary breaks it silently. Regression guard:
  `src/tmux-mgr/scripts/e2e-gss-integration.sh`.

**Phase 3 — companion tasks (required for a *complete* fix, not polish).**
- **Decide module posture first (§6.1.B5).** Public-installable vs. org-private
  (`GOPRIVATE`) changes whether the first fetch is broadcast to the public
  sumdb/proxy. This is an owner decision and must be answered **before** tagging
  (the first public fetch is irreversible — §6.1.B4).
- **Tag `src/gss/v0.1.0`.** Go subdirectory modules are versioned with a
  path-prefixed tag of the form `src/gss/vX.Y.Z` (not a bare `v0.1.0`). This is
  what lets consumers pin `…/src/gss@v0.1.0` instead of a pseudo-version (C8).
  Create it as an **annotated, signed** tag and protect the `src/gss/v*` tag
  namespace (§6.1.C).
- **README "External Install" section** (this is a **net-new** section —
  `src/gss/README.md` has no install content today) documenting the
  canonical command against the **module root** (there is no `cmd/gss` package —
  C5):
  ```
  go install github.com/sfc-gh-eraigosa/dotfiles/src/gss@v0.1.0
  ```
- **ADR** under `docs/adr/` recording the module-path convention, the tagging
  scheme, the sibling-migration list, and the supply-chain posture (§9).

## 6. Risk & blast-radius analysis

- **In-repo (100 files, single module).** A missed file fails the build loudly —
  low risk, caught immediately by Phase 2. The blast radius is contained to one
  module; no cross-module imports reference gss internally. `main.go` is the
  easy-to-miss file (it sits outside `cmd/`+`internal/`) and is called out
  explicitly in Phase 1.
- **`build.sh` ldflags (four lines).** Easy to overlook because they live outside
  the Go source tree; missing even one of the four leaves a stale prefix and
  `gss --version` reports wrong/empty data. Phase 1 names all four; Phase 2
  smoke-tests `--version`; `scripts/test.sh` also Docker-smoke-tests `gss version`,
  independently exercising the ldflags path.
- **`tmux-mgr` coupling.** `tmux-mgr` invokes the installed gss binary at runtime.
  A broken rebuild breaks it without a compile error in tmux-mgr itself (its unit
  tests mock the runner). Mitigated by the Phase 2 rebuild + smoke test and the
  `src/tmux-mgr/scripts/e2e-gss-integration.sh` guard.
- **External consumers.** This is a **breaking change to the public module
  identity** — the old path was never installable, so no working consumer is
  regressed, but any consumer who pinned the old path in a `replace` directive
  must update it. Documented in the README/ADR. The `v0.1.0` tag gives them a
  stable handle going forward.
- **Future owner rename.** Hard-coded `sfc-gh-eraigosa` re-breaks on the next
  rename. Accepted for now; vanity-import path noted as future work (§8).
- **Sibling drift.** `tmux-mgr` (wrong org), `gsl`, `wol` (wrong org + missing
  `src/`) all still carry the defect. Out of scope for this PR, but the ADR makes
  the convention explicit so they are migrated the same way.

### 6.1 Supply-chain threat model (publishing a public module identity)

Tagging this module turns its path into a **public, immutable supply-chain
identity** that arbitrary third parties can `go install` and that the
proxy/sumdb will cache permanently. Relevant STRIDE vectors: **Spoofing**
(typosquat of the abandoned `wenlock` path), **Tampering** (tag re-pointing),
**Repudiation/Elevation** (no provenance on the published binary).

**A. Typosquat / namespace-abandonment risk on the old path
(severity: medium-to-high, conditional — gated on a verification below).**
The concern is the inverse of "nobody depends on the old path": once we stop
owning it, *can an attacker own it?* If the GitHub login `wenlock` is
re-registrable (GitHub recycles a username after rename/deletion unless retired),
an attacker who registers `github.com/wenlock`, creates a `dotfiles` repo, and
declares `module github.com/wenlock/dotfiles/gss` would serve
**attacker-controlled code to any consumer, doc, shell history, or stale `go.mod`
still referencing the old path** on a fresh `go install`. The 301 redirect (C7)
does **not** protect against this: GitHub drops the rename redirect the moment the
old login is re-registered, so the redirect is a ticking dependency on GitHub's
retention policy, not a safe park.
- **Gating verification (do before tagging):** confirm whether `github.com/wenlock`
  can be re-registered by a third party (GitHub username availability +
  rename-redirect retention). This one fact sets the final severity: parked
  redirect → lower; re-registrable → live RCE vector for any stale reference.
- **Defensive action:** claim/retain `github.com/wenlock` (or confirm GitHub has
  it reserved/redirect-locked) and record the owner of that hold.
- **Purge the old string repo-wide,** not just the 100 Go files: grep the **whole
  repo** (READMEs, CI configs, shell aliases, `build.sh`) and notify downstream
  consumers (`snowflake-eng/dev-config`) to purge any `go install
  github.com/wenlock/...` line — each is a live typosquat/redirect target.

**B. Checksum DB / proxy posture.**
- **B4 — first public fetch is permanent (severity: low-to-medium, cheap now /
  expensive later).** The first time `src/gss/v0.1.0` is fetched through the public
  proxy, its hash is written to the append-only `sum.golang.org` transparency log.
  **Deleting the git tag does not purge the sumdb entry** — a wrong tag is
  permanent in the log. This is the mechanism behind §10's "roll forward, never
  re-point" rule (sumdb immutability, not just proxy caching).
- **B5 — public vs. org-private posture (severity: medium; explicit owner
  decision).** Because the canonical repo is under the Snowflake org
  (`sfc-gh-eraigosa`), decide deliberately whether this module is meant to be a
  world-installable public artifact or internal-only. Internal-only consumers
  would set `GOPRIVATE=github.com/sfc-gh-eraigosa/*` so the hash is **never**
  broadcast to the public sumdb/proxy — which also changes the typosquat math.
  This is a genuine design fork; surface it via `AskUserQuestion`:
  **(a) public artifact (Recommended if external `go install` is the goal —
  sumdb-tracked, anyone can install)** vs. **(b) org-private (`GOPRIVATE`, no
  public proxy/sumdb broadcast)**. Resolve before Phase 3 tagging.

**C. Release-tag integrity & provenance.**
- **C7 — sign the tag (severity: low-to-medium).** Create `src/gss/v0.1.0` as an
  **annotated, GPG/SSH-signed** tag so the release point is verifiable; record the
  signing-key owner.
- **C8 — protect the tag namespace (severity: low-to-medium).** Add a GitHub tag
  protection rule on `src/gss/v*` so a released tag cannot be force-moved. This is
  the *enforcement* behind §10's "treat as immutable" prose — prose alone is
  advisory.
- **C9 — build provenance (follow-up, not a blocker).** `build.sh` self-asserts
  version/commit via `-ldflags -X` (C10). If gss ever ships as a release artifact
  (not just `go install` from source), add SLSA/provenance attestation — recorded
  alongside the vanity-import item in §8.

## 7. Verification / test plan

A passing local build is **necessary but not sufficient** — `go build` does not
network-resolve the module path, so it cannot prove `go install` works. The real
proof requires the corrected module to be **committed, pushed, and tagged** (the
proxy serves committed code, not the working tree).

**Named verification surfaces (and their relationship to the rename):**
- `scripts/test.sh` — canonical runner, **70%** gss coverage gate; discovers the
  module by **directory name** (not module path), so discovery is unaffected.
- `src/gss/scripts/check-deps.sh` — license/dep gate; operates on `./...`,
  path-independent; CI runs it as `GSS_STRICT_CHECK=1`, so it is a **post-push
  acceptance signal** as well as a local one.
- `src/gss/go.sum` — committed; rename is expected to leave it byte-identical (C9).
- `src/tmux-mgr/scripts/e2e-gss-integration.sh` — runtime regression guard for the
  tmux-mgr→gss coupling.

**Steps:**
1. **Local (pre-merge):** `go build ./...` clean; `bash scripts/test.sh` green
   (matches Phase 0 baseline); `bash src/gss/scripts/check-deps.sh` green;
   `go mod tidy` leaves `go.sum` byte-identical (non-empty diff ⇒ stop);
   `gss --version` / `gss help` smoke pass;
   `grep -r 'github.com/wenlock/dotfiles/gss' src/gss` returns **nothing**, and a
   whole-repo grep for the old string (§6.1.A) returns nothing outside expected
   historical references.
2. **Path-resolution sanity (pre-merge, local scratch module):** in a throwaway
   module, `go install <local-corrected-path>` (or a `replace` pointing at the
   working tree) builds RC=0 — confirms the corrected `go.mod` path no longer
   triggers *"module declares its path as …"*.
3. **End-to-end (post-merge, post-tag — the decisive test):** from a **clean
   module cache** (`go clean -modcache`), on a machine that is **not** the repo
   checkout:
   ```
   go install github.com/sfc-gh-eraigosa/dotfiles/src/gss@v0.1.0
   ```
   must produce a runnable `gss` binary. This is the acceptance gate; until it
   passes, the fix is not done. (Note the **module-root** path — there is no
   `cmd/gss` package, C5.)
4. **CI (post-push):** the `check-deps.sh` license gate stays green.
5. **Regression guard:** run `src/tmux-mgr/scripts/e2e-gss-integration.sh` to
   confirm `tmux-mgr` still drives the rebuilt gss.

## 8. Alternatives considered

- **Org-only sed rename (the issue's proposed fix).** **Rejected** — proven
  insufficient (§3, row 2): the module lives at `src/gss`, so
  `…/dotfiles/gss@latest` resolves to *"does not contain package."* Merging #114
  as written would close the issue while leaving `go install` broken.
- **`replace` directive in each consumer's `go.mod`.** Lets a consumer point the
  (wrong) module path at a local clone or fork. **Rejected as the fix** — it is a
  per-consumer workaround that pushes the burden onto ~18 downstream workspaces
  and does nothing for the public `go install` path. Fine as an *interim*
  consumer-side mitigation; not a repo fix.
- **Vendoring gss into each consumer.** Copies the source into the consumer tree.
  **Rejected** — defeats the point of a shared installable tool, duplicates
  source, and drifts.
- **`GOPRIVATE` / `GONOSUMDB`.** Bypasses the public proxy/checksum DB. **Rejected
  *as the install fix*** — it changes *how* Go fetches, not *what the module
  declares*; the mismatched-path rejection still fires. **But `GOPRIVATE` is a live
  option for the separate posture decision** in §6.1.B5 (internal-only module),
  where it is the correct mechanism, not a workaround.
- **Rely on the GitHub 301 rename redirect.** **Rejected** — §3.1: the redirect
  resolves repo identity, not module identity, and the `?go-get=1` subpath returns
  404. It is a red herring, and §6.1.A shows it is also a security liability once
  the old namespace is re-registrable.
- **Vanity import path (`go-import` meta on a stable domain).** Genuinely better
  for long-term durability (survives owner renames). **Deferred** — out of scope
  for an urgent install-unblock; requires a hosted meta endpoint and is a separate
  decision. Recorded as future work in the ADR, alongside SLSA/provenance
  attestation (§6.1.C9).

## 9. ADR + documentation follow-up

- **New ADR under `docs/adr/`** recording: (1) the module-path convention
  `github.com/sfc-gh-eraigosa/dotfiles/src/<tool>` (canonical org **and** `src/`
  segment); (2) the subdirectory tag scheme `src/<tool>/vX.Y.Z`; (3) the
  sibling-migration list — **`tmux-mgr` (org `eraigosa`), `gsl`, `wol`**, each of
  which carries an org and/or `src/` defect and is therefore *not* a clean
  exemplar — onto the same convention; (4) the **supply-chain posture**:
  public-vs-private decision (§6.1.B5), the defensive `wenlock`-namespace hold
  (§6.1.A), signed + protected release tags (§6.1.C); (5) vanity-import path and
  provenance attestation as deferred future work.
  - **`docs/adr/` does not exist yet** (only `docs/designs/` does). Two house-style
    obligations apply **unconditionally** when this tree is created:
    1. Add the **`GEMINI.md` + `CLAUDE.md -> GEMINI.md`** symlink pair in
       `docs/adr/` (per the repo's "documented directory" rule), and link it from
       the root `CLAUDE.md`/`GEMINI.md` Repository Structure section.
    2. The `.gitignore` allowlist already covers it (`!docs/` + `!docs/**`), so the
       files are stage-able without a new rule — but verify with
       `git status --short -- docs/adr/`.
  - **Open question for the Architecture team (do not decide unilaterally):**
    whether decision records belong in a **new `docs/adr/` tree** or in the
    **existing `docs/designs/`** convention (date-prefixed filenames). Introducing
    a second parallel decision-record location needs explicit justification;
    default to `docs/designs/` unless the ADR series is deliberately split out.
- **gss README** — net-new "External Install" section (the file has none today)
  with the canonical module-root command
  `go install github.com/sfc-gh-eraigosa/dotfiles/src/gss@v0.1.0`.

## 10. Rollback

Low-risk to reverse. The change is a path-string rename plus a tag:

- **Pre-merge:** discard the branch — nothing in `$HOME` or any installed config
  is touched by this change (it is purely in-repo Go source + build script).
- **Post-merge, pre-tag:** revert the rename commit. The build returns to the
  prior (already non-installable) state; no consumer was relying on a working
  `go install`, so nothing downstream regresses.
- **Post-tag:** a published `src/gss/v0.1.0` tag must be treated as immutable. If
  the tagged module is wrong, **roll forward** with `src/gss/v0.1.1` rather than
  deleting/moving the tag. The mechanism: once the tag is fetched once through the
  public proxy, its hash is written permanently to the `sum.golang.org`
  transparency log (§6.1.B4) — **deleting the git tag does not purge the sumdb
  entry**, and module-proxy caches make tag deletion hostile to consumers.
  Deleting a tag is acceptable **only** if it was provably never fetched
  (unverifiable in practice — prefer roll-forward). The `src/gss/v*` tag-protection
  rule (§6.1.C8) enforces this so a tag cannot be silently force-moved.
