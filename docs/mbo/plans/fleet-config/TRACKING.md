# fleet-config — evidence ledger

A row is `done` **only** with a commit SHA *and* observed output. "Tests should pass"
is not evidence; pasted output is.

| Task | State | Commit | Observed evidence |
| :-- | :-- | :-- | :-- |
| T1 | done | `9ca258c` | 4 tests PASS; add/update/unchanged + marker scope + no-blank-on-omission |
| T2 | done | `a88a04d` | hostile fixture yields no exec directive; NotImported=[LocalCommand PermitLocalCommand ProxyCommand]; Includes=1 |
| T3 | done | `ae2501b` | 5 tests PASS; ProxyCommand/comment/marker preserved across a HostName rewrite |
| T4 | done | `a88a04d` | provenance imported-from=src; second Build Empty; re-apply byte-identical; no reorder |
| T5 | done | `639fe9d` | 4 tests PASS; command log proves the read path issues nothing that writes; trust failure reports `host key unverified` |
| T6 | done | `64a8564` | reused existing writeConfig (already backs up + 0600) rather than duplicating; loopback guard live-verified: refuses a self-source |
| T7 | done | `639fe9d` | missingIdentities wired into config pull; stat-only probe pinned by TestKeyReadinessOnlyEverStatsAPath |
| T8 | done | `0e8d7a0` | `keys sync --host` present in --help; unknown alias rejected; bootstrapNeeded lists auth-failed+unreachable |
| T9 | done | `0e8d7a0` | 6 tests PASS incl. validation-rejects-malformed and merge-not-replace (a real bug caught pre-ship) |
| T10 | done | `0e8d7a0` | diff renders both directions and writes nothing; identical configs diff empty both ways |
| T11 | done | `HEAD` | p/P declared in keyHelp, no duplicate bindings, blocked while another path owns the row |
| T12 | done | `HEAD` | 9 invariants added to sdk/fleet/AGENTS.md, each naming its pinning test |

## §6 Human-evidenced gates

| Gate | State | Evidence |
| :-- | :-- | :-- |
| G1 real pull | done | Real SSH pull imported 4 marked hosts into a temp destination; the destination's own `already-here` block AND its ProxyCommand survived; provenance `imported-from=` stamped; timestamped backup written; second run reported "already current". NOTE: source and destination were the same machine reached over its LAN IP, so this proves the wire path and the merge, not two distinct hosts. |
| G2 malformed push rejected | unit only | `TestRemoteInstallValidatesBeforeMoving` proves the live config survives and the staging file is cleaned up. NOT exercised against a real host — that needs a machine we can safely break. |
| G3 self-retarget refused | unit only | `TestSelfRetargetIsDetected` + `TestSelfRetargetIgnoresAnAdd`. No live run: needs a second host. |
| G4 missing IdentityFile named | unit only | `TestMissingIdentitiesNamesAbsentKeys`. The live pull's keys all existed, so no miss was triggered. |

## Suite state

`go test -race ./...` green (10 pkgs) · `go vet` clean · `gofmt` clean · cfgplan 90.8% · sshconf 94.5% · cmd 64.7%

## Notes

- Build verification uses a scratch binary; `opt/bin/fleet` is git-ignored but is the
  installed CLI, so only overwrite it when the operator asks to install for verification.
