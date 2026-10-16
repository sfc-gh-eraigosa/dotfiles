# Security Scan green + required — design

- **Slug:** security-scan-green
- **Date:** 2026-10-15
- **Status:** Proposed
- **Relates to:** issue #394 / design PR (pending)
- **Author(s):** Edward Raigosa, Claude

## 1. Problem / context

`.github/workflows/security-scan.yml` ("Security Scan (govulncheck + gosec)", dotfiles#103)
runs `make security` on every PR and push to `main`. The Makefile target loops over every
`sdk/*` module, runs `govulncheck ./...` then `gosec -quiet ./...`, and exits non-zero if
**any** module reports **any** finding. The job carries `continue-on-error: true` and is not in
the Mergify required list, so it never blocks — and it is red on every run, including `main`
(verified on the latest `main` run: "security findings in: sdk/fleet(gosec) sdk/gcfg(gosec)
sdk/gff(gosec) sdk/ghapp(gosec) sdk/gsl(gosec) sdk/gss(gosec) sdk/libs(gosec)
sdk/tmux-mgr(gosec) sdk/wlink(gosec)").

A permanently red check is worse than none: nobody reads it, so a genuine new finding (or a
real govulncheck CVE) would land unnoticed. `docs/mergify.md` already records the intent —
"promote once the backlog is triaged to zero, in the same merge-first-then-ruleset order".

**The backlog, measured on `main` (156 gosec findings; govulncheck: no vulnerabilities in
called code in any module):**

| Module | Total | By rule |
| :-- | --: | :-- |
| tmux-mgr | 36 | G204×16 G304×8 G302×5 G301×4 G306×3 |
| fleet | 36 | G204×23 G304×12 G306×1 |
| gss | 22 | G301×8 G204×5 G306×4 G304×3 G101×1 G302×1 |
| wlink | 19 | G306×9 G204×3 G304×3 G101×2 G301×1 G302×1 |
| gsl | 19 | G304×8 G301×4 G204×3 G306×3 G404×1 |
| gff | 12 | G103×4 G301×3 G304×3 G204×1 G306×1 |
| ghapp | 5 | G204×3 G304×2 |
| gcfg | 5 | G304×3 G306×2 |
| libs | 2 | G304×2 |

By class:

| Rule | Count | What it is here |
| :-- | --: | :-- |
| G204 subprocess launched with variable | 54 | The tools' purpose: `git`, `gh`, `ssh`, `tmux`, `wsl.exe` with user-chosen branch / host / session arguments. Argument validation is at the call sites (e.g. #96 added `--` + base-name validation to the rebase path). |
| G304 file read via variable | 42 | Reading config / registry / cache / payload files at paths derived from `$HOME`, `$XDG_*`, or a CLI flag. |
| G306 WriteFile mode > 0600 | 23 | Mix of private state written 0644 and files that must stay world-readable. |
| G301 MkdirAll mode > 0750 | 21 | Mostly 0755 on private state directories. |
| G302 Chmod mode > 0600 | 8 | tmux-mgr alias/agent scripts (0755 on purpose), wlink resolv.conf snapshot, gss feature auto script. |
| G103 unsafe | 4 | All in `sdk/gff/gen/gff/v1/features.pb.go` — protoc-generated. |
| G101 hardcoded credential | 3 | The gss error-code constant for "PR ready needs an approval token" and two wlink snapshot *filename* constants; the rule matches the identifier's name, not a secret. |
| G404 weak random | 1 | `gsl/internal/mcp/cache.go` temp-file suffix from `math/rand`, annotated `//nolint:gosec` — which gosec does not honour (that is golangci-lint's syntax). |

## 2. Goals & non-goals

**Goals**
1. `make security` exits 0 on `main` and stays 0: every finding either fixed or suppressed with a
   written, reviewable reason.
2. The policy (which rules are excluded and why) lives in one place in the repo, not scattered.
3. The job becomes a required check so a new finding blocks a PR.
4. No behaviour change for users: files that other programs or other users must read keep
   their modes.

**Non-goals**
- Rewriting subprocess call sites to avoid variables (G204): the argument validation story is
  per call site and already handled where it matters (#96); a blanket rewrite is churn.
- Dependency bumps for govulncheck: it is already clean.
- staticcheck / golangci rule changes (separate, already promoted).

## 3. Options considered

1. **Policy exclusion for G204/G304 + real fixes for the rest + justified `#nosec` where a
   mode is intentional — RECOMMENDED.** Honest about what gosec can and cannot tell us here:
   G204/G304 are structural to CLI tools that run git and read config, so excluding them
   repo-wide with a written rationale is the truthful policy; the permission/credential/random
   findings are cheap and mostly real, so fix them. Leaves a small, reviewable set of `#nosec`
   lines each carrying a reason (enforced by `-nosec-require-justification`).
2. **Suppress everything inline (`#nosec` on all 156 lines).** Gets green fastest but buries the
   policy in 156 comments, trains everyone to paste `#nosec`, and fixes nothing real.
3. **Fix everything "properly", no exclusions.** G204 would need every `exec.Command` argument
   wrapped in an allow-list type; G304 every path through a cleaner + root check. Weeks of churn
   for tools whose inputs are the user's own shell arguments, with no attacker in the threat
   model (single-user dotfiles). Rejected as cost without benefit.

## 4. Decision

**Policy, in `Makefile` (`security` target) — one place, commented:**

```
gosec -quiet -exclude-generated -nosec-require-justification -exclude=G204,G304 ./...
```

- `-exclude=G204,G304`: rationale as in §1; recorded in the Makefile comment and in
  `docs/mergify.md`'s promotion section.
- `-exclude-generated`: protoc output (`gff/gen`) is not ours to annotate (clears G103).
- `-nosec-require-justification`: every remaining `#nosec` must say why
  (`#nosec G306 -- resolv.conf must be world-readable`).

**Fixes, by class (one leaf per module in the plan; each leaf is independently testable):**

| Class | Rule | Fix |
| :-- | :-- | :-- |
| Private state dirs (registries, caches, approval token dir, worktrees, config dirs) | G301 | `0o750` → prefer `0o700` for `$HOME`-private state. |
| Private state files (config.json, registry, caches, tokens, usage records) | G306 | `0o600`. |
| Files other programs/users must read: wlink's resolv.conf / wsl.conf snapshots and restores, gcfg/gff exported schemas, tmux-mgr alias + agent launcher scripts (0755), gss feature `auto` hook | G302/G306 | Keep the mode; add `#nosec G30x -- <why>` with the consumer named. |
| gsl mcp cache temp suffix | G404 | `os.CreateTemp` in the target dir (atomic rename preserved); drop the inert `//nolint`. |
| "token"/"file" identifier constants | G101 | `#nosec G101 -- error code / snapshot filename, not a credential`. |

**Promotion (last, in the order `docs/mergify.md` prescribes):** remove `continue-on-error`,
add `check-success = Security Scan (govulncheck + gosec)` to `.github/mergify.yml`'s
"required CI green" protection, then the branch ruleset. The workflow stays on plain
`pull_request` triggers (it is not label-gated; see the livelock note in its header — once
required, the header's rationale is re-checked against `docker-image.yml`'s guard and the
guard copied if needed).

## 5. Risks & blast radius

- **Tightening a mode breaks a consumer** (e.g. a 0600 file another user or a systemd unit
  reads). Mitigation: the fix table above names the consumer for every kept mode; each module
  leaf runs its existing e2e/unit suites; wlink's snapshot/restore has a round-trip test.
- **Making the check required before it is green on `main` deadlocks the queue** (documented
  in `docs/mergify.md`). Mitigation: promotion is its own final leaf, merged only after a green
  `main` run is observed.
- **Runner toolchain drift**: one PR run failed with gosec's importer rejecting the Go 1.26 export
  data ("export data version 5 is greater than maximum supported version 4"). The Makefile
  installs `gosec@latest` lazily; pin a gosec version that supports the repo's `.go-version`
  so a required check cannot flake on tool/toolchain mismatch.

## 6. Rollback

Each leaf is a single revertable PR. Promotion is a one-line revert in
`.github/mergify.yml` plus restoring `continue-on-error: true`.

## 7. Evidence expectations

- Per module leaf: the `make security` output for that module showing `Issues : 0`, captured
  under `plans/security-scan-green/evidence/<module>/`, plus the module's own test run.
- Promotion leaf: a green "Security Scan" check on a `main` push, and a PR whose check turns
  red when a deliberate `os.WriteFile(..., 0o644)` is introduced (then reverted) — proof the
  gate bites.

> Produced via the `mbo-plan` skill after the user chose "make the whole scan green" with an
> issue + design doc. Spec/plan follow in `../specs/` and `../plans/` with the execution trio.
