# Security Scan green + required — implementation plan

- **Slug:** security-scan-green
- **Date:** 2026-10-15
- **Status:** In-progress
- **Relates to:** spec `../specs/security-scan-green.md` · issue #394 · design PR #395 · fix PR (worker `fix`) · promotion PR (worker `promote`, after a green `main`)

## 1. Summary & verdict
Two PRs. **fix** (base `main`): the Makefile policy plus every per-module change, each tightening
test-first, until `make security` is green locally and in CI. **promote** (after `fix` merges and a
green `main` run is observed): drop `continue-on-error`, add the Mergify protection line, mirror the
ruleset. gosec `v2.29.0` renumbers the taint variants of the excluded classes (G702 command
injection, G703 path traversal) and adds G704 (SSRF); the design's exclusion covers the first two by
the same rationale, G704 is handled at its two call sites with a justified `#nosec` (design §4
amended).

## 2. File inventory
| Path | Purpose | Implements |
| :-- | :-- | :-- |
| `Makefile` (`security` target) | pinned gosec, flags, rationale comment | F1 |
| `sdk/gsl/internal/{config/config.go,gh/pr.go,mcp/cache.go,usage/usage.go}` (+ tests) | 0700/0600, `os.CreateTemp` | F2 |
| `sdk/gss/internal/{approval/approval.go,config/config.go,repo/cache.go,registry/lock.go,feature/{auto,paths,start,worker}.go,errors/errors.go}` (+ tests) | 0700/0600/0750; G101 nosec | F2, F3 |
| `sdk/tmux-mgr/{pkg/agent/session.go,pkg/tmux/tmux.go,cmd/{alias,migrate,agent}.go}` (+ tests) | 0700/0600; rc-file append nosec | F2, F3 |
| `sdk/fleet/cmd/update_init.go` (+ test) | 0600 | F2 |
| `sdk/gff/{internal/overrides/overrides.go,cmd/{export,gen}.go}` | 0750 helper dir; nosec on generated/export output | F2, F3 |
| `sdk/gcfg/cmd/{export,schema}.go` | nosec: shareable export | F3 |
| `sdk/wlink/internal/resolvconf/snapshot.go` | nosec: system files + snapshot filenames | F3 |
| `sdk/ghapp/pkg/ghapp/manifest.go` | nosec G704 | F3 |
| `docs/mergify.md`, `.github/workflows/security-scan.yml`, `.github/mergify.yml` | promotion + rationale pointer | F4 |
| `docs/mbo/plans/security-scan-green/evidence/**` | captured gate output | §7 |

## 3. Interface contracts
- Makefile: `$(GOPATH)/bin/gosec` installed via `go install github.com/securego/gosec/v2/cmd/gosec@v2.29.0`
  (not `command -v gosec`), invoked as
  `gosec -quiet -exclude-generated -nosec-require-justification -exclude=G204,G304,G702,G703 ./...`.
- `#nosec` form: `// #nosec G306 -- <consumer and why>` on the flagged line.
- Modes: private dirs `0o700` (under `$HOME`/XDG) or `0o750` (inside a git worktree); private files
  `0o600`; temp files via `os.CreateTemp(dir, pattern)` then rename.

## 4. TDD build order
1. **Policy** — edit Makefile; verify `make security` runs the pinned gosec with the flags (evidence:
   `evidence/policy/make-security-before.txt`, the remaining-findings list). Done-when: output lists only
   the §2 findings.
2. **gsl** — RED: `TestSave_PrivateModes` (config), `TestWriteCache_PrivateModes` (gh, mcp), usage dir
   mode; GREEN: modes + CreateTemp. Done-when: `go test ./...` green, gosec `Issues : 0` for sdk/gsl.
3. **gss** — RED: mode tests for approval dir, config stub, repo cache, registry dir, FEATURE.md /
   WORKER.md; GREEN; nosec G101. Done-when: as above for sdk/gss.
4. **tmux-mgr** — RED: session file + layout file modes; GREEN; nosec on the rc append. Done-when: as above.
5. **fleet, gff, gcfg, wlink, ghapp, libs** — RED where a writer is tested (fleet update plan); nosec with
   consumer elsewhere. Done-when: each module `Issues : 0`.
6. **Whole-repo** — `make security` → `OK: no vulnerability or misuse findings`; all module test suites
   green; `make lint-go`. Done-when: fix PR CI green incl. Security Scan.
7. **Promote** (separate PR, after merge + green `main`): workflow + mergify + docs; ruleset by hand;
   red proof PR. Done-when: a PR with a deliberate G306 is held by the queue.

## 5. Verification mapping
F1 → task 1 evidence; F2 → the named mode tests + per-module gosec evidence; F3 → the bare-`#nosec`
negative proof (`evidence/policy/bare-nosec-rejected.txt`); F4 → task 7 evidence.

## 6. Integration & rollout
Workers: `security-scan-green/edward-raigosa/fix` (base main) then `.../promote`. No build/test discovery
changes. `docs/mergify.md` promotion section updated in the promote PR.

### 6.1 Build leaves / DAG
| Leaf | Owns (paths) | Consumes | `done-when` gate | Blocking? |
| :-- | :-- | :-- | :-- | :-- |
| fix | Makefile + all `sdk/**` sites above | — | `make security` OK; module tests green; CI Security Scan green | yes (base for promote) |
| promote | `.github/workflows/security-scan.yml`, `.github/mergify.yml`, `docs/mergify.md` | fix merged + green `main` | red proof PR held by the queue | no |

## 7. Validation & evidence (show the work)
`plans/security-scan-green/evidence/`: `policy/` (before/after `make security`, bare-nosec rejection),
`<module>/gosec.txt` + `go-test.txt` per module, `promote/` (green main run URL, red proof PR URL).
Coverage bars unchanged (sdk ≥60%, enforced by the existing per-module gates).

> Execute with the trio in `./security-scan-green/` (IMPLEMENTATION / TRACKING / TODO).
