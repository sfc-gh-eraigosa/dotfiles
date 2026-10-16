# Security Scan green + required — spec

- **Slug:** security-scan-green
- **Date:** 2026-10-15
- **Status:** Approved (design #395 approved by the maintainer 2026-10-15)
- **Relates to:** issue #394 / PR #395 / design `../designs/security-scan-green.md`

## 1. Goal
`make security` (govulncheck + gosec on every `sdk/` module) exits 0 on `main` and stays 0, under a
written gosec policy that lives in one place, and the "Security Scan (govulncheck + gosec)" job
becomes a required check so a new finding blocks a PR instead of being ignored.

## 2. Use cases
- **Maintainer / opens a PR that adds `os.WriteFile(p, b, 0o644)` for private state** → CI runs
  `make security` → the Security Scan check is red and the merge queue refuses the PR → acceptance:
  the check reports the file:line and rule (G306).
- **Maintainer / runs `make security` locally** → same gosec version and flags as CI (pinned) →
  acceptance: the local result matches CI (no "works on my machine" drift).
- **Maintainer / needs an intentional world-readable file** (resolv.conf restore, exported schema,
  shell alias file) → adds `#nosec G30x -- <consumer>` on that line → acceptance: gosec accepts it
  only with the justification (`-nosec-require-justification`); a bare `#nosec` is still a finding.
- **Reviewer / wants to know why G204/G304/G702/G703 never fire** → reads the Makefile comment next
  to the flags and `docs/mergify.md`'s promotion section → acceptance: the rationale names the
  tools' purpose (run git/gh/ssh/tmux with user arguments, read config at `$HOME` paths) and the
  call-site validation precedent (#96).

## 3. Architecture
Three independently testable units:
1. **Policy** — the `security` target in `Makefile`: pinned gosec (`v2.29.0`, the first release whose
   importer reads the Go 1.27 export data the repo's `.go-version` produces), flags
   `-quiet -exclude-generated -nosec-require-justification -exclude=G204,G304,G702,G703`, and a
   comment block carrying the rationale. Verified by running the target.
2. **Fixes** — per module, every remaining finding is one of: tighten (private state → `0o700` dirs /
   `0o600` files, `os.CreateTemp` for temp names), or keep with a justified `#nosec` naming the
   consumer. Verified by (a) a mode-asserting unit test next to each tightened writer and (b) the
   module's gosec run reporting zero issues.
3. **Promotion** — `.github/workflows/security-scan.yml` loses `continue-on-error`;
   `.github/mergify.yml`'s "required CI green" protection gains
   `check-success = Security Scan (govulncheck + gosec)`; the GitHub ruleset mirrors it (manual,
   UI). Verified by a green run on `main` and a deliberately red PR.

## 4. Behavior / features
- **F1 policy**: pinned gosec, exclusion list with rationale, justification required on `#nosec`.
- **F2 tighten**: gsl (config, PR cache, MCP cache + CreateTemp, usage dir), gss (approval token dir,
  config, repo cache, registry dir, feature dirs + FEATURE.md/WORKER.md), tmux-mgr (sessions, layouts,
  migrate log, agent.log, config dir, aliases.sh), fleet (default update plan), gff (atomic-write dir).
- **F3 keep + justify**: gcfg export/schema (user-requested shareable export), gff `gen` output
  (generated source committed to the repo) and export dir, wlink snapshots/restores and the
  resolv.conf chmod (system files every process must read), tmux-mgr append to the user's rc file
  (mode unused: no `O_CREATE`), ghapp manifest POST (G704: `APIURL` is the operator's GitHub API
  base), G101 identifier constants (gss error code, wlink snapshot filenames).
- **F4 promote**: required check, merge-first-then-ruleset.

## 5. Evaluation criteria (per feature)
| Feature | Trigger | Fires | Must-not-fire | Edge | Pass |
| :-- | :-- | :-- | :-- | :-- | :-- |
| F1 | `make security` | installs/uses gosec v2.29.0; passes the flag set | a locally newer/older gosec on PATH is not used | gosec cache stale | target prints the gosec version and `OK: no vulnerability or misuse findings` |
| F2 | writer called in a unit test | file mode `0600`, dir mode `0700`/`0750` as specified | existing contents/behaviour change | pre-existing file with wider mode (left as is: WriteFile does not chmod) | mode assertions pass; module gosec `Issues : 0` |
| F3 | gosec run | zero findings | a `#nosec` without `--` justification | a new unjustified `#nosec` | gosec fails on a bare `#nosec` (proven once in evidence) |
| F4 | PR with a new G306 | Security Scan check red, Mergify holds | green `main` deadlocking (check required before green) | runner toolchain drift | green `main` run observed before promotion; red proof PR captured |

## 6. Verification harness
- Unit: mode-asserting tests per tightened writer (`os.Stat(...).Mode().Perm()`), run with each
  module's existing `go test ./...`; coverage gates unchanged (sdk ≥60%).
- Static: `make security` locally with the pinned gosec; evidence captured per module under
  `../plans/security-scan-green/evidence/<module>/gosec.txt`.
- CI: the Security Scan workflow on the fix PR (must be green before the promotion leaf), then on
  `main` after merge.
- Human-evidenced: the ruleset edit (GitHub UI) and the red proof PR.

## 7. Prerequisites / dependencies
Design #395 approved (done). Go `1.27.2` per `.go-version`; gosec `v2.29.0`; govulncheck `@latest`.

## 8. Out of scope (and why)
Rewriting G204/G304/G702/G703 call sites (design §2); dependency bumps (govulncheck clean);
golangci rule changes (already promoted separately).

## 9. Rollback
Revert the fix PR (single commit set) and/or the promotion PR (two-line change).

> Produced via the `mbo-plan` pipeline after design approval. Plan: `../plans/security-scan-green.md`.
