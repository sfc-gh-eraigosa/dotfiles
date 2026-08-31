# fleet-config — evidence ledger

A row is `done` **only** with a commit SHA *and* observed output. "Tests should pass"
is not evidence; pasted output is.

| Task | State | Commit | Observed evidence |
| :-- | :-- | :-- | :-- |
| T1 | todo | — | — |
| T2 | todo | — | — |
| T3 | todo | — | — |
| T4 | todo | — | — |
| T5 | todo | — | — |
| T6 | todo | — | — |
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
| G4 missing IdentityFile named | todo | — |

## Notes

- Build verification uses a scratch binary; `opt/bin/fleet` is git-ignored but is the
  installed CLI, so only overwrite it when the operator asks to install for verification.
