# sshd-setup — on-demand portable sshd provisioning — spec

- **Slug:** sshd-setup
- **Date:** 2026-07-16
- **Status:** Approved
- **Relates to:** plan `../plans/sshd-setup.md` · issue #169 · PR #(pending)

## 1. Goal

A tool you invoke **only when you want SSH access to a machine**: it installs/enables the
OS-native sshd, opens the local firewall, and seeds `authorized_keys` from the GitHub public
keys of the account derived from the machine's configured git credentials. One bash entry
point covers Linux/macOS/WSL; a companion PowerShell script covers Windows-native. Nothing
runs at boot/login unless the tool set it up — invoking it is the opt-in.

Motivating incident: this Windows+WSL host lost SSH access when a Windows update silently
removed the OpenSSH Server capability (`C:\ProgramData\ssh` remnant dated 2024-04, no
`sshd.exe`); WSL never had `openssh-server`, and dotfiles' `sshd_run.sh` is start-only (no
install, no install-check) with its login hook deliberately disabled (`opt/profiles/.bashrc:189`).

## 2. Use cases

- **Restore Windows-native sshd** — *actor:* admin PowerShell / *trigger:* `setup-sshd.ps1`
  / *flow:* add OpenSSH.Server capability → service auto-start + start → firewall rule 22 →
  seed keys / *accept:* `ssh <winuser>@<host>` from another machine succeeds after reboot.
- **Enable sshd inside WSL/Linux** — *actor:* user shell / *trigger:* `sshd-setup enable`
  / *flow:* detect platform → native-first check → `pkg-install` openssh-server if missing →
  enable+start service → firewall allow → seed keys → write `~/.sshd.env` / *accept:*
  `ssh localhost` key-auth succeeds; re-run converges (idempotent, no dupes).
- **Seed keys only** — *trigger:* `sshd-setup keys` / *accept:* GitHub keys of the derived
  account merged into `~/.ssh/authorized_keys`, deduped, perms 700/600.
- **Inspect** — *trigger:* `sshd-setup status` / *accept:* reports install/run/firewall/keys
  state, mutates nothing.
- **WSL → Windows bridge** — *trigger:* `sshd-setup enable` on WSL / *accept:* runs the ps1
  via interop when available, else prints the exact admin-PowerShell command (incl. portproxy
  for LAN reach of the WSL sshd).

## 3. Architecture

- `opt/bin/sshd-setup` (bash) — subcommands `status|enable|keys`, flag `--dry-run`.
  Internal units, each independently testable via command mocking: platform detection
  (`/proc/version` microsoft ⇒ wsl), sshd presence/run check, installer (delegates to
  `opt/bin/pkg-install`), service enabler (systemctl / `systemsetup -setremotelogin on`),
  firewall (ufw/firewalld if active), GitHub account derivation
  (`gh api user` → origin-owner → `git config github.user`), key seeding (fetch
  `https://github.com/<user>.keys`, refuse-empty, merge+dedupe).
- `opt/Desktop/Apps/scripts/setup-sshd.ps1` (admin) — capability install, service, firewall
  rule, `administrators_authorized_keys` + user keys, `-WslPortProxy [port]` optional switch.
- `docs/sshd-setup.md` — per-OS how-to + Windows+WSL walkthrough.
- Integration: writes `SSHD_LOGIN=true` to `~/.sshd.env` so the existing
  `opt/scripts/network/sshd_run.sh` / `install.sh` flow recognizes the host; adds **no**
  login/boot hook itself.

## 4. Behavior / features

F1 platform detect · F2 native-first (skip install when sshd present; never a second sshd) ·
F3 install-if-missing via pkg-install · F4 service enable+start (persistent across reboot
where the OS supports it) · F5 firewall allow 22 (only when a firewall is active) ·
F6 GitHub-account auto-derivation · F7 key seeding (idempotent, refuse-empty, perms) ·
F8 `~/.sshd.env` integration · F9 dry-run + status read-only · F10 Windows ps1 parity
(F2–F7 equivalents) + WSL handoff instructions.

## 5. Evaluation criteria (per feature)

| Rule | Fires | Must not fire | Pass |
| :-- | :-- | :-- | :-- |
| F1 wsl detect | `/proc/version` has `microsoft` | plain Linux | unit test (mocked file) |
| F2 native-first | sshd active ⇒ "already running", no install | sshd absent | mock systemctl both ways |
| F3 install | package absent ⇒ pkg-install called once | already installed | mock pkg-install, count calls |
| F5 firewall | ufw active ⇒ `ufw allow` | ufw inactive/absent ⇒ skip+note | mocked ufw |
| F6 derivation | gh absent ⇒ falls to origin owner; both absent ⇒ github.user; none ⇒ error | wrong-order precedence | unit test per fallback |
| F7 refuse-empty | empty keys response ⇒ exit non-zero, authorized_keys untouched | valid keys | fixture server/file |
| F7 idempotent | second run ⇒ 0 new lines | first run adds N | run twice, diff |
| F9 status/dry-run | no file/service mutation | — | run under read-only assertion |

## 6. Verification harness

`opt/bin/sshd-setup_test.sh` (mock-command pattern of `pkg-install_test.sh`), wired into
`scripts/test.sh` discovery; `make lint-shell` (shellcheck). ps1: PSScriptAnalyzer when
available. Human-evidenced acceptance on this Windows+WSL host (see plan §6): real
key-auth logins to both sshds after reboot.

## 7. Prerequisites / dependencies

`opt/bin/pkg-install` (exists) · `curl` · outbound HTTPS to github.com · sudo/admin at
invocation time (the tool prompts; it is not run from CI).

## 8. Out of scope (and why)

Non-22 ports & sshd_config hardening beyond defaults (native configs are fine; YAGNI) ·
auto-start of WSL distro at Windows boot (separate objective) · key revocation/rotation ·
nix packaging (nix absent on targets today) · macOS automated tests (no CI runner; manual).

## 9. Rollback

`enable` prints its inverse steps on completion; docs carry per-OS teardown
(`Remove-WindowsCapability` / service disable / firewall rule delete / `~/.sshd.env` removal).
Key seeding is additive-only to `authorized_keys` — remove lines by hand.

> Produced via `superpowers:brainstorming`. The matching plan goes in `../plans/sshd-setup.md`.
> Register / update `../index.md`.
