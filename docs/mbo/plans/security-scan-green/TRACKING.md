# Security Scan green + required — live state ledger

- **Slug:** security-scan-green
- **Started:** 2026-10-15
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Cursor:** [`TODO.md`](./TODO.md)
- **Plan (source of truth):** [`../security-scan-green.md`](../security-scan-green.md) · spec [`../../specs/security-scan-green.md`](../../specs/security-scan-green.md)

> Status: `todo · in-progress · blocked · done`. A row is `done` only with a commit SHA **and** observed output.

## 0. Worker registry
| Leaf | Worker ref | Branch | PR | State |
| :-- | :-- | :-- | :-- | :-- |
| design | security-scan-green/edward-raigosa/design | feature/security-scan-green/edward-raigosa/design | #395 | merged 2026-10-16 |
| fix | security-scan-green/edward-raigosa/fix | feature/security-scan-green/edward-raigosa/fix | #396 | in-review |
| promote | security-scan-green/edward-raigosa/promote | feature/security-scan-green/edward-raigosa/promote | #397 (draft, base = fix) | prepared; hold until #396 merges + green main |

## 1. Task ledger
| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| 1 policy | done | c1a6a16 + review-1 | `make security` → `OK: no vulnerability or misuse findings` (`evidence/policy/make-security-after.txt`); bare `#nosec G306` still reported (`evidence/policy/bare-nosec-rejected.txt`) | gosec v2.29.0 + x/tools v0.52.0 + x/vuln v1.9.0 built from tools/security; a release gosec cannot read Go 1.27 export data |
| 2 gsl | done | c1a6a16 + review-1 | 5 tests RED→GREEN (incl. `TestWriteCache_RenameFailureLeavesNoTempFile`); `evidence/gsl/gosec.txt` (regenerated, real scan) 0 findings; `evidence/gsl/go-test.txt` ok | CreateTemp replaces the math/rand suffix; temp removed on any failure |
| 3 gss | done | c1a6a16 + review-1 | 3 mode tests RED→GREEN; `evidence/gss/gosec.txt` (regenerated) 0 findings; 22 internal packages ok; every dir under ~/.config/gss now one 0700 constant | registry dir mkdir sits behind the lock → not test-reachable, tightened anyway; cmd/TestIsDirty fails identically on main (pre-existing) |
| 4 tmux-mgr | done | c1a6a16 | SaveSession mode test RED→GREEN; `evidence/tmux-mgr/gosec.txt` (regenerated) 0; go test ok | rc-file append keeps 0644 with justification (no O_CREATE) |
| 5 fleet/gff/gcfg/wlink/ghapp/libs | done | c1a6a16 + review-1 | fleet plan-mode test updated 0644→0600 (RED→GREEN); gosec 0 in each (`evidence/<module>/gosec.txt`, regenerated) | wlink: copies vs marker files now carry accurate justifications; ghapp G704 justified |
| 6 whole-repo | done | 6b34943 | local: `make security` OK, `make lint-go` 0 issues ×10; CI on #396: Security Scan **pass** (first green run), 14/14 checks pass (`evidence/ci/pr396-checks.txt`) | ready-for-merge after the /code-review round |
| 7 promote | todo | | | separate PR |

## 2. Feature → proof matrix (from spec §5)
| Feature | Automated proof | Human/live proof | Notes |
| :-- | :-- | :-- | :-- |
| F1 policy | [x] make security runs pinned gosec (`evidence/policy/make-security-after.txt`) | — | |
| F2 tighten | [x] mode tests + per-module gosec zero (`evidence/<module>/`) | — | |
| F3 keep+justify | [x] bare-#nosec negative proof (`evidence/policy/bare-nosec-rejected.txt`) | — | |
| F4 promote | [ ] CI green on main | [ ] ruleset edit, red proof PR | |

## 3. Validation done-when — the stop condition
- [x] `make security` prints `OK: no vulnerability or misuse findings` (local, Go 1.27.2)
- [x] every module's tests green (except the pre-existing gss cmd/TestIsDirty environment failure, identical on main)
- [ ] fix PR merged; Security Scan green on `main` (green on the PR: `evidence/ci/pr396-checks.txt`)
- [ ] promote PR merged; red proof PR held by the queue

## 4. Blockers & escalations
| Date | Task | Blocker | Command + observed output | Resolution |
| :-- | :-- | :-- | :-- | :-- |

## 5. Session log (append-only)
| Date | Session | What advanced |
| :-- | :-- | :-- |
| 2026-10-16 | review-1 | `/code-review 396 high`: 8 findings. The serious one: the per-module `evidence/<module>/gosec.txt` files were a FAILED invocation (zsh passed the flags as one word → gosec printed its usage; my count grep read 0) and the before-baseline came from a gosec that could not type-check (826 export-data errors). Both regenerated with split flags and the pinned build: before = 56 findings on main, after = 0. Code fixes: temp-file cleanup on rename failure (test first), accurate wlink marker justifications, one 0700 constant across gss, pinned-version banner (`go list -m`), setup-go cache-dependency-path, documented gitleaks trigger. |
| 2026-10-16 | ci | #396 CI settled: Security Scan pass, 14 pass / 0 fail. Promote leaf prepared as stacked draft #397 (strict workflow, Mergify line, docs); merges only after #396 + green main. |
| 2026-10-16 | build (fix) | Tasks 1–5 done in one commit c1a6a16 on PR #396; gitleaks tripped on a go.sum checksum line (allowlisted with rationale) and the privacy hook on a constant whose name contains "Token" (nosec moved to the line above). |
| 2026-10-15 | design → build | Design #395 approved; spec/plan/trio written; `fix` worker created; gosec v2.29.0 baseline under the policy flags: 60 findings left to fix/justify (G301 18, G306 21, G302 8, G101 3, G404 1, G704 2 + taint G702/G703 excluded by policy). |
