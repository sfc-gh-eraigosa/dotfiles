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
| T7 | todo | — | — |
| T8 | todo | — | — |
| T9 | todo | — | — |
| T10 | todo | — | — |
| T11 | todo | — | — |
| T12 | todo | — | — |

## §6 Human-evidenced gates

| Gate | State | Evidence |
| :-- | :-- | :-- |
| G1 real two-machine pull | todo | — |
| G2 malformed push rejected | todo | — |
| G3 self-retarget refused | todo | — |
| G4 missing IdentityFile named | partial | unit-verified (`TestMissingIdentitiesNamesAbsentKeys`); real two-machine run still outstanding |

## Suite state

`go test -race ./...` green (9 pkgs) · `go vet` clean · `gofmt` clean · cfgplan 90.8% · sshconf 94.5%

## Notes

- Build verification uses a scratch binary; `opt/bin/fleet` is git-ignored but is the
  installed CLI, so only overwrite it when the operator asks to install for verification.
