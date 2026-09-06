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
| G1 real pull | done (live) | Real SSH pull imported 4 marked hosts into a temp destination; the destination's own `already-here` block AND its ProxyCommand survived; provenance `imported-from=` stamped; timestamped backup written; second run reported "already current". NOTE: source and destination were the same machine reached over its LAN IP, so this proves the wire path and the merge, not two distinct hosts. |
| G2 malformed push rejected | done (live) | On a real host over ssh: the exact validation `remoteInstall` runs — `ssh -F <staged> -G fleet-validation-probe` — returned **255** for a config containing `Port notanumber` and **0** for a well-formed one. The mechanism genuinely discriminates; it is not a no-op that always passes. A full real push also succeeded end to end: 6 to 7 Host blocks, every pre-existing entry and comment preserved, a timestamped remote backup written, post-push probe answered. |
| G3 self-retarget refused | done (live) | Real push whose plan changed the target's own `HostName 192.168.0.201 -> 10.0.0.99`. Without the flag: `SKIP ... this would change how we reach fleet-selftest itself`, exit non-zero, nothing written. With `--allow-self-retarget --dry-run`: permitted, and the rendered file preserved the target's own entries. |
| G4 missing IdentityFile named | done (live) | Real pull of a source block naming `~/.ssh/id_does_not_exist`: the run reported `1 imported host(s) reference a key that is not on this machine`, named the alias and path, and warned it would report auth-failed until a key exists. |

## Suite state

`go test -race ./...` green (10 pkgs) · `go vet` clean · `gofmt` clean · cfgplan 90.8% · sshconf 94.5% · cmd 64.7%

## Notes

- Build verification uses a scratch binary; `opt/bin/fleet` is git-ignored but is the
  installed CLI, so only overwrite it when the operator asks to install for verification.

## Live-run caveat (do not overstate this evidence)

Every live gate used `fleet-selftest`, an alias pointing at THIS machine's LAN
address. That is a real SSH round trip — real transport, real remote read, real
remote staging, validation, backup, and install — but source and target are one
machine. It proves the wire path, the merge, and every guard. It does NOT prove
behaviour across two genuinely distinct hosts.

A true two-host run is blocked on access, not on code: the three other LAN hosts
in the fleet all report `auth-failed (permission denied)` — they answer SSH and
their host keys verify, but this workstation's public key is not in their
`authorized_keys`. `keys sync` cannot fix that; appending to a remote
`authorized_keys` needs the very access being established. Bootstrapping them
needs an interactive `ssh-copy-id` with a password. This is precisely the case
`bootstrapNeeded` exists to report honestly rather than paper over.
