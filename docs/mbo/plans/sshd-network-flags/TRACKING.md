# sshd + WSL networking as gff flags — live state ledger

- **Slug:** sshd-network-flags
- **Started:** 2026-10-18
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Cursor:** [`TODO.md`](./TODO.md)
- **Plan (source of truth):** [`../sshd-network-flags.md`](../sshd-network-flags.md) · spec [`../../specs/sshd-network-flags.md`](../../specs/sshd-network-flags.md)

> **Update after EVERY task.** Status: `todo · in-progress · blocked · done`.
> **Evidence** = the exact command run plus its real result. A row is `done` only with a
> commit SHA **and** evidence. Never write a result you did not observe.

## 0. Worker registry

| Leaf/worker | Worker ref | Branch | Worktree path | PR | State |
| :-- | :-- | :-- | :-- | :-- | :-- |
| (single, no breakout) | — | `fix/wsl-distro-detect` | `~/git/dotfiles` | #409 | open (draft) |

## 1. Task ledger

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| 1 gff helpers | todo | | | |
| 2 install.sh requires gff | todo | | | |
| 3 sshd flag + migration | todo | | | |
| 4 wsl-network flag + detect/ask | todo | | | |
| 5 sshd-setup uses the flag | todo | | | |
| 6 set-wsl-network.ps1 | todo | | | |
| 7 deferred --wsl-network step | todo | | | |
| 8 fleet WSLNET_ANSWER | todo | | | |
| 9 docs + registration + e2e | todo | | | |

## 2. Feature → proof matrix (from spec §5)

| Feature | Automated proof | Human/live proof | Notes |
| :-- | :-- | :-- | :-- |
| F1 working gff required | [ ] `gff_test.sh` (a)–(e) + STATIC | [ ] CI Build job green | |
| F2 sshd is a flag | [ ] STATIC gate checks | [ ] H1 | |
| F3 sshd migration | [ ] `sshdflags_test.sh` | [ ] H1 (`~/.sshd.env` gone) | |
| F4 detection | [ ] `wslnet_test.sh` detect | [ ] H1 (`nat`), H2 (`bridged`) | |
| F5 recommend + ask | [ ] `wslnet_test.sh` + `sshd-setup_test.sh` | [ ] — (no fresh machine in the fleet) | |
| F6 apply | [ ] `wsl-network_test.sh` STATIC + LIVE | [ ] H1 (no UAC), H2 (unchanged) | |
| U5 fleet | [ ] fleet `cmd` tests | [ ] H3 | |

## 3. Validation done-when — the stop condition

- [ ] Tasks 1–9 `done`, each with a SHA and evidence
- [ ] `make shell-test lint-shell lint-portability lint-markdown` green
- [ ] `./scripts/test.sh` green (fleet coverage floor)
- [ ] PR #409 CI green
- [ ] H1 laptop: migration + `nat` recorded, no prompt, no UAC, LAN ssh works
- [ ] H2 k3s workstation: `bridged` recorded, `.wslconfig` byte-identical
- [ ] H3 `fleet update` wave: no stalled host
- [ ] PR #409 description covers all commits; `index.md` row → `in-review`

## 4. Blockers & escalations

| Date | Task | Blocker | Command + observed output | Resolution |
| :-- | :-- | :-- | :-- | :-- |

## 5. Session log (append-only — never rewrite history)

| Date | Session | What advanced |
| :-- | :-- | :-- |
| 2026-10-18 | planning | Spec approved by the operator; plan + trio written; build not started |
