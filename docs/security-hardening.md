# security-hardening — opt-in Windows security hardening

Follow-up to [`security-audit.md`](./security-audit.md). That feature delivers **detection**
(a weekly, read-only, ~40-lens anomaly report). This one applies a small, reversible set of
**posture changes** that close two of the audit's known blind spots and trim a couple of
high-value attack surfaces — without changing how you use the machine.

**Opt-in and fail-closed**: gated by gff flag `install.windows.security-hardening`
(`boolDefault: false`), enforced with the same `gff_opt_in` gate the audit uses. It runs
**only** when the flag resolves to exactly `true`, so a step that edits group membership, an
event channel, and Defender policy can never fire by accident.

## What it does (three actions)

| # | Action | Closes / reduces |
| :-- | :-- | :-- |
| 1 | Adds you to the local **`Event Log Readers`** group | The audit collector runs non-elevated, so its Security-log lenses report `## ADMIN-REQUIRED` every week. Membership lights up 1102 (log cleared), 4720 (new user), 4624/4625 (logons), 4728/4732 (group changes) — **with no collector change**. |
| 2 | Enables **`Microsoft-Windows-TaskScheduler/Operational`** | This channel is off by default, so task-*creation* forensics don't exist. The audit can see current tasks, but never the moment one appeared. |
| 3 | Turns on 5 Defender **ASR rules in AUDIT MODE ONLY** | Surfaces would-be-blocked behavior as events (1122/1125/1132/1134) with **zero** user-visible impact, so you get evidence before deciding to enforce. |

### The five ASR rules

GUIDs are transcribed from the official
[Microsoft Learn ASR rules reference](https://learn.microsoft.com/defender-endpoint/attack-surface-reduction-rules-overview#asr-rules)
and the rule name travels next to each GUID **in the script**, so any reviewer can re-verify
the pair against that table.

| GUID | Rule | Category |
| :-- | :-- | :-- |
| `56a863a9-875e-4185-98a7-b882c64b5ce5` | Block abuse of exploited vulnerable signed drivers | Standard protection |
| `9e6c4e1f-7d60-472f-ba1a-a39ef669e4b2` | Block credential stealing from the Windows LSASS | Standard protection |
| `d4f940ab-401b-4efc-aadc-ad5f3c50688a` | Block all Office applications from creating child processes | Productivity apps |
| `3b576869-a4ec-4529-8536-b80a7769e899` | Block Office applications from creating executable content | Productivity apps |
| `5beb7efe-fd9a-4556-801d-275e5ffc04cc` | Block execution of potentially obfuscated scripts | Script |

**Deliberately excluded** from the starter set on a developer machine: *Block process
creations originating from PSExec and WMI commands* (Docker and WSL tooling lean on WMI),
*Block untrusted and unsigned processes that run from USB*, and *Block executable files
unless they meet a prevalence, age, or trusted list criterion* (hostile to locally-built
binaries). Add them later from the audit's own evidence rather than preemptively.

## Enable

```sh
gff set install.windows.security-hardening true
./install.sh          # runs in the deferred Windows phase, after the Desktop deploy
```

A **UAC prompt** appears — the script self-elevates once. (If the y/n Windows customization
also runs, that chain raises its own separate prompt.)

Or run it standalone from PowerShell (a manual run is explicit intent, so no flag needed):

```powershell
& "$([Environment]::GetFolderPath('Desktop'))\Apps\scripts\setup-security-hardening.ps1"
```

**Resolve the Desktop, never hardcode `%USERPROFILE%\Desktop`** — with OneDrive Backup on
(the Windows 11 default) the real Desktop is `…\OneDrive\Desktop` and
`%USERPROFILE%\Desktop` does not exist at all. From **cmd.exe** (where `$env:` is not
expanded either), use:

```bat
powershell -ExecutionPolicy Bypass -Command "& ([Environment]::GetFolderPath('Desktop') + '\Apps\scripts\setup-security-hardening.ps1') -Status"
```

> ### ⚠️ Group membership takes effect at your NEXT LOGON
>
> Windows builds your access token when you sign in, so adding you to `Event Log Readers`
> does **not** affect any already-running process — including the audit collector's scheduled
> task. **Sign out and back in (or reboot)** before expecting the audit's section K to stop
> reporting `## ADMIN-REQUIRED`. This is the single most common "why didn't it work?" here.

## Operations

```powershell
setup-security-hardening.ps1 -Status      # read-only; runs WITHOUT elevation
setup-security-hardening.ps1 -Uninstall   # reverts only what this script changed
```

`-Status` reports each action's current state, and for ASR the per-GUID mode
(`AuditMode` / `Block` / not configured), plus whether a state file exists.

## How the weekly audit sees this

You don't have to tell the audit anything. On its next run the existing collector observes
the posture change on its own, and the analysis task folds it into the baseline with your
confirmation:

| Audit section | Before | After |
| :-- | :-- | :-- |
| **K** (Security-log gap) | `## ADMIN-REQUIRED (UnauthorizedAccessException)` | Security log readable — account-change lenses report data |
| **D2** (channel health) | `Microsoft-Windows-TaskScheduler/Operational enabled=False` | `enabled=True` with a rising record count |
| **C1** (Defender status) | `ASRrules(count) = 0` | `ASRrules(count) = 5` |

That is the intended feedback loop: hardening changes posture, the audit proves it changed.

## Promotion path: audit mode → enforce

Audit mode is the starting point **by design** — this is a dev machine (Docker, WSL,
AutoHotkey, games, Electron apps) and a wrong Block rule breaks real work. Promote only with
evidence:

1. **Let it run** for at least one full weekly audit cycle (ideally 2–4 weeks).
2. **Look for audit-mode hits** — Event Viewer → *Applications and Services Logs → Microsoft →
   Windows → Windows Defender → Operational*, event IDs **1122/1125/1132/1134**. Zero hits for
   a rule over the window means enforcing it is very likely safe.
3. **Promote one rule at a time**, never the whole set, from an elevated PowerShell:

   ```powershell
   Set-MpPreference -AttackSurfaceReductionRules_Ids <GUID> -AttackSurfaceReductionRules_Actions Enabled
   ```

   (`Enabled` = Block. Use `Add-MpPreference` semantics carefully — `Set-MpPreference` with a
   *single* id/action pair updates just that rule.)
4. **If something breaks**, drop it back to `AuditMode` (or `Disabled`) rather than adding a
   broad exclusion — exclusions weaken every rule that honors them.

A promoted rule is treated as a deliberate decision: **`-Uninstall` will not remove it**, it
reports it and leaves it in place.

## Rollback

```powershell
setup-security-hardening.ps1 -Uninstall
```

then `gff set install.windows.security-hardening false` (or `gff unset …`).

Reversal is **precise, not best-effort**. A state file at
`%ProgramData%\dotfiles\security-hardening.state.json` records what this script *actually*
changed, so uninstall:

- removes you from `Event Log Readers` **only if it added you**;
- disables the TaskScheduler channel **only if it enabled it** (never one you turned on
  yourself);
- removes **only its own five ASR GUIDs**, and **skips any rule you promoted to Block**;
- with **no state file**, changes nothing and says so — it refuses to guess.

## Security posture & caveats

- **Audit mode blocks nothing.** Until you deliberately promote a rule, ASR only generates
  events. The hardening cannot break an application.
- **Nothing here touches the audit pipeline.** No changes to `security-audit-collect.ps1`,
  the Claude scheduled task, or anything under `%USERPROFILE%\Claude\`.
- **Elevation is required and requested once.** `-Status` never elevates; install and
  `-Uninstall` self-elevate via `Start-Process -Verb RunAs`. Because the elevated window
  closes on exit, its output is mirrored to
  `%ProgramData%\dotfiles\security-hardening.log`, which the parent prints.
- **Every action verifies itself.** Microsoft documents that protected writes "might appear
  to succeed but are actually blocked", so each action reads its state back and warns if it
  did not take effect rather than reporting a false success. (ASR is not tamper-protected,
  but the script does not rely on that being true forever.)
- **Existing configuration is never clobbered.** ASR uses `Add-MpPreference` (additive), never
  `Set-MpPreference` (which replaces the whole rule set). A rule you already configured in any
  mode is left exactly as it is and reported.
- **The group is resolved by well-known SID** `S-1-5-32-573`, not by name — `Event Log
  Readers` is localized on non-English Windows and would not match.
- **`Event Log Readers` grants read access to the Security log.** That is the point (it is the
  least-privileged way to get it), but it is a real, if small, privilege grant: an attacker who
  compromises this account can now read security events they previously could not. On a
  single-user workstation that is a good trade for the audit visibility; on a shared machine,
  weigh it.
