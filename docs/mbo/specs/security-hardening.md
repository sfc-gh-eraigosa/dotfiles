# Opt-in Windows Security Hardening — spec

**Status:** building · **Slug:** `security-hardening` · **Owner:** edward-raigosa

Follow-up to [`security-audit`](./security-audit.md) (PR #225). That objective delivered
**detection**; this one delivers a small, reversible set of **posture changes** that make
the detection better and reduce a couple of high-value attack surfaces.

## Problem

The weekly audit landed with three known gaps that hardening — not more collector code —
is the right fix for:

1. **The collector is half-blind to the Security event log.** It runs non-elevated, so
   section K reports `## ADMIN-REQUIRED (UnauthorizedAccessException)` every week and the
   account-integrity lenses (1102 log-clear, 4720 new-user, 4624/4625 logons, 4728/4732
   group-add) are never audited. Microsoft ships a purpose-built group for exactly this —
   `Event Log Readers` — and the account simply is not in it (verified empty on the target
   host, 2026-08-16).
2. **Task-creation forensics do not exist.** `Microsoft-Windows-TaskScheduler/Operational`
   is **disabled by default** (verified `IsEnabled=False` on the target host), so the
   audit's B2/J-series scheduled-task lenses can only ever see current *state*, never the
   *event* of a task being created — the thing an investigation needs.
3. **No attack-surface reduction at all.** `Get-MpPreference` reports **0** configured ASR
   rules. Defender's ASR rules are available on Windows 11 Home via local PowerShell, and
   several map directly to techniques the audit already hunts for (LSASS credential theft,
   obfuscated scripts, vulnerable signed drivers).

## Requirements

- **R1 — opt-in, fail-closed**: gated by a NEW flag `install.windows.security-hardening`
  (`boolDefault: false`), enforced at the `install.sh` call site with the existing
  `gff_opt_in` helper. Absent gff / unset flag ⇒ the step must NOT run. Reuses the
  fail-closed gates landed in #225; nothing new is invented.
- **R2 — audit mode only, never enforce**: ASR rules are configured with
  `AttackSurfaceReductionRules_Actions = AuditMode` **only**. This is a developer machine
  (Docker, WSL, AutoHotkey, games, Electron apps); enforcement is a **separate, later
  decision** taken only after the weekly audit shows a clean audit-mode window. The script
  must never set `Enabled`/Block.
- **R3 — never clobber existing configuration**: use `Add-MpPreference` (additive), never
  `Set-MpPreference` (which overwrites the whole rule set). A rule already configured by
  the user in any mode is left exactly as-is and reported, not overwritten.
- **R4 — precisely reversible**: `-Uninstall` reverts **only what this script actually
  changed**. The script's own GUID list is the source of truth for ASR; a small state file
  records whether the group membership and the channel were changed *by us*, so an uninstall
  never disables a channel the user had already enabled or removes a pre-existing group
  membership. A rule the user has since **promoted to Block is deliberately left alone**.
- **R5 — idempotent**: every action is individually idempotent and safe to re-run. A second
  run makes no changes and says so per action.
- **R6 — single UAC prompt**: the script self-elevates once (`Start-Process -Verb RunAs`),
  mirroring `setup-autostart.ps1`. `-Status` is read-only and must run **without** elevation.
- **R7 — verify, don't assume**: every action reads its state back after writing and reports
  a warning if it did not take effect. Microsoft documents that tamper-protected writes
  "might appear to succeed but are actually blocked"; ASR is not tamper-protected, but the
  script must not rely on that.
- **R8 — every GUID verified against first-party docs**: the five ASR GUIDs are transcribed
  from the official Microsoft Learn ASR rules reference, not from memory, and the rule name
  is carried next to each GUID in the script so a reviewer can re-verify.
- **R9 — do not touch the audit**: no changes to `security-audit-collect.ps1`, the Claude
  scheduled task, or anything under `%USERPROFILE%\Claude\`. The weekly audit detects the
  resulting posture change on its own and folds it into its baseline with user confirmation.
- **R10 — portability**: no hardcoded usernames or absolute home paths; `$env:` on the
  Windows side, `${HOME}`/`wslpath` on the WSL side.

## Non-goals

- **Enforcement (Block mode)** — explicitly out of scope; see R2 and the promotion path in
  `docs/security-hardening.md`.
- **Exclusion tuning** — Defender exclusions are admin-only to read on this host, and adding
  exclusions is the opposite of hardening; out of scope.
- **Group Policy / Intune delivery** — this is a single-host, PowerShell-local objective
  (Home edition has no `gpedit`). Fleet delivery is the `fleet` objective's problem.
- **LSA protection / Credential Guard / Secure Boot / BitLocker** — larger posture changes
  with reboot and recovery-key implications; a separate objective if ever wanted.

## The three actions

| # | Action | Mechanism | Why |
| :-- | :-- | :-- | :-- |
| 1 | Add the current user to `Event Log Readers` | `Add-LocalGroupMember` on well-known SID **S-1-5-32-573** (the group *name* is localized; the SID is not) | Lights up the audit's Security-log lenses (K, D1-1102, H-series) on the next collection — no collector change needed |
| 2 | Enable `Microsoft-Windows-TaskScheduler/Operational` | `wevtutil sl <channel> /e:true`, verified via `Get-WinEvent -ListLog` | Creates the task-creation forensic trail the audit's task lenses cannot otherwise see |
| 3 | Five ASR rules in **AuditMode** | `Add-MpPreference -AttackSurfaceReductionRules_Ids … -AttackSurfaceReductionRules_Actions AuditMode` | Surfaces would-be blocks as events 1122/1125/1132/1134 with zero user-visible impact |

**The five ASR rules** (GUIDs verified against the Microsoft Learn ASR rules reference):

| GUID | Rule | Category |
| :-- | :-- | :-- |
| `56a863a9-875e-4185-98a7-b882c64b5ce5` | Block abuse of exploited vulnerable signed drivers | Standard protection |
| `9e6c4e1f-7d60-472f-ba1a-a39ef669e4b2` | Block credential stealing from the Windows LSASS | Standard protection |
| `d4f940ab-401b-4efc-aadc-ad5f3c50688a` | Block all Office applications from creating child processes | Productivity apps |
| `3b576869-a4ec-4529-8536-b80a7769e899` | Block Office applications from creating executable content | Productivity apps |
| `5beb7efe-fd9a-4556-801d-275e5ffc04cc` | Block execution of potentially obfuscated scripts | Script |

Deliberately **excluded** from the starter set on a dev machine: `Block process creations
originating from PSExec and WMI commands` (Docker/WSL tooling leans on WMI),
`Block untrusted and unsigned processes that run from USB`, and
`Block executable files unless they meet a prevalence/age/trusted-list criterion`
(hostile to locally-built binaries). They can be added later from the audit's evidence.

## Acceptance criteria

1. Flag unset/false → `install.sh` prints
   `SKIP (gff: install.windows.security-hardening is opt-in and not enabled)`; nothing is
   changed on the host.
2. `gff set install.windows.security-hardening true && ./install.sh` → one UAC prompt; all
   three actions applied; each verified by read-back; a state file is written.
3. Re-running the installer changes nothing and reports all three actions as already-applied
   (idempotence).
4. `-Status` runs **non-elevated** and reports, per action: current state, and for ASR the
   per-GUID mode (`AuditMode` / `Block` / not configured).
5. `-Uninstall` reverts exactly the tracked changes: removes the group membership only if we
   added it, disables the channel only if we enabled it, removes only our ASR GUIDs, and
   **skips any rule the user promoted to Block**. With no state file it changes nothing.
6. After the next logon, the weekly audit's section K stops reporting
   `## ADMIN-REQUIRED` for the Security log, D2 shows the TaskScheduler channel
   `enabled=True`, and C1 shows `ASRrules(count) = 5` — all detected by the existing
   collector with **no collector change**.
7. `make lint-shell` / `make lint-portability` / `make lint-markdown` pass; `.github/gff/features.yaml`
   parses and the new flag resolves `false` by default (`gff get` = `false`).

## Known caveat (documented, not fixed)

**Group membership is applied to the access token at logon.** Adding the account to
`Event Log Readers` does not affect any already-running process, including the audit
collector's scheduled task. The Security-log lenses light up **after the next sign-out /
sign-in (or reboot)** — the installer says so explicitly and `docs/security-hardening.md`
repeats it, so a "why is section K still ADMIN-REQUIRED?" question has an answer in the docs.
