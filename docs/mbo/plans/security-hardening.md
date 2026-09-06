# Opt-in Windows Security Hardening — plan

**Status:** building · **Slug:** `security-hardening` · **Spec:** [../specs/security-hardening.md](../specs/security-hardening.md)

Small feature: spec → plan, built in a single PR (no execution trio). Follow-up to
[`security-audit`](./security-audit.md) — reuses its gates and call-site pattern verbatim.

## Tasks

| # | Task | Files |
| :-- | :-- | :-- |
| T1 | New opt-in flag `install.windows.security-hardening` (`boolDefault: false`) | `.github/gff/features.yaml` |
| T2 | Hardening installer: self-elevating (single UAC), idempotent, read-back verified, state-tracked; `-Status` (non-elevated) / `-Uninstall` | `opt/Desktop/Apps/scripts/setup-security-hardening.ps1` |
| T3 | install.sh wiring: `run_security_hardening_setup` sibling of `run_security_audit_setup`, fail-closed via `gff_opt_in`, rc-captured + logged, at the deferred Windows phase after the deploy | `opt/bin/install_windows.sh` |
| T4 | User guide: enable flow, what each action does, the logon caveat, audit-mode → enforce promotion path, rollback | `docs/security-hardening.md` |
| T5 | Cross-links + inventory: audit doc → hardening doc, scripts inventory, docs reference index | `docs/security-audit.md`, `opt/Desktop/Apps/scripts/AGENTS.md`, `docs/AGENTS.md` |
| T6 | MBO registration | `docs/mbo/specs/security-hardening.md`, `docs/mbo/plans/security-hardening.md`, `docs/mbo/index.md` |

**No `gff_opt_in` / `Test-GffOptIn` changes** — T1/T3 consume the fail-closed gates exactly
as PR #225 landed them. That is the point of the precedent; re-implementing it would be the bug.

## Design decisions (and why)

- **Group identified by well-known SID `S-1-5-32-573`, not the name.** `Event Log Readers` is
  localized; the SID is not. Verified on the target host that `Get-LocalGroup -SID` resolves it.
- **Target user passed as a SID through the elevation boundary.** `Start-Process -Verb RunAs`
  can be satisfied by a *different* admin account, so an elevated child that reads
  `$env:USERNAME` could add the wrong principal. The non-elevated parent captures
  `[WindowsIdentity]::GetCurrent().User.Value` and passes it as `-TargetUserSid`.
- **`Add-MpPreference`, never `Set-MpPreference`.** `Set-` overwrites the entire rule set;
  `Add-` merges. Rules already present are skipped and reported with their current mode.
- **State file (`%ProgramData%\dotfiles\security-hardening.state.json`).** Needed for R4: without
  it, `-Uninstall` cannot tell "we enabled this channel" from "it was already on", and would
  silently weaken a posture the user set deliberately. ASR uses the in-script GUID list as its
  source of truth (per the spec), with the state file recording which of those were *actually*
  added by us.
- **Elevated child logs to a file the parent prints.** The UAC child gets its own window that
  closes on exit, so its output would otherwise be invisible — the parent `-Wait`s, then
  echoes `%ProgramData%\dotfiles\security-hardening.log` and propagates the child's exit code
  (so `install_windows.sh`'s rc capture is meaningful).
- **Native-command stderr is guarded.** `wevtutil` writes to stderr on failure, which under
  `$ErrorActionPreference='Stop'` becomes a terminating `NativeCommandError` — the exact trap
  that bit `net localgroup` in the audit collector. The call drops the preference locally and
  checks `$LASTEXITCODE`, then verifies via `Get-WinEvent -ListLog`.

## Validation

Run in the build sandbox (Linux — no PowerShell in CI):

- `bash -n` + dash `sh -n` on every touched shell file — pass.
- `make lint-shell` (shellcheck) — pass.
- `opt/scripts/system/shell-portability-scan.sh --strict` — Tier 1: 0, Tier 2: 0.
- `make lint-markdown` on added/changed markdown — 0 issues.
- `.github/gff/features.yaml` parses; `gff get install.windows.security-hardening` = `false`
  and `gff export --shell` emits `GFF_INSTALL_WINDOWS_SECURITY_HARDENING=false` (fail-closed
  confirmed end-to-end).
- PowerShell **syntax** validated via `PSParser::Tokenize` over WSL interop (parse-only; the
  script is never executed during the build because it mutates machine state).

**Read-only pre-flight verified on the real Windows 11 host** (informs the design, changes
nothing): `Get-LocalGroup -SID S-1-5-32-573` → `Event Log Readers`, membership **empty**;
`Microsoft-Windows-TaskScheduler/Operational` → `IsEnabled=False`; `Get-MpPreference` → **0**
ASR rules configured. So all three actions are genuinely needed on this machine.

**Requires a Windows host** (the enabling machine, at flag-flip time) — see the PR body's
post-merge test plan for the full matrix: AC1 (skip), AC2 (apply + verify), AC3 (idempotence),
AC4 (`-Status` non-elevated), AC5 (`-Uninstall` precision incl. the promoted-to-Block skip),
AC6 (the weekly audit observing the posture change after the next logon).

## Rollback

`setup-security-hardening.ps1 -Uninstall` (reverts only tracked changes), then
`gff set install.windows.security-hardening false` (or `gff unset`). ASR rules the user
promoted to Block are intentionally retained; remove those manually if that is the intent.
