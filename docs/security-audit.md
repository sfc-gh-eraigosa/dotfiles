# security-audit — unattended weekly Windows security audit (opt-in)

A Claude scheduled task that audits the Windows host weekly used to require a
human in the loop twice: unattended runs cannot approve computer-use access, and
terminals are only ever granted at "click" tier, so Claude can never type or
paste into PowerShell anyway. This feature removes the human from the loop by
**decoupling collection from analysis**:

```text
Windows Task Scheduler                          Claude scheduled task (weekly)
ClaudeSecurityAuditCollector (daily 09:00)      weekly-security-audit
  security-audit-collect.ps1  ──────────────►     reads latest-audit.txt from its
  read-only queries, writes:                      own task folder (uploads), diffs
  %USERPROFILE%\Claude\SecurityAudit\             against the baseline embedded in
    latest-audit.txt + history\                   its SKILL.md, reports anomalies —
  %USERPROFILE%\Claude\Scheduled\                 no computer use, no approvals,
    weekly-security-audit\latest-audit.txt        no user interaction
```

**Opt-in and fail-closed**: gated by gff flag `install.windows.security-audit`
(`boolDefault: false`). Unlike the fail-open `gff_on` gates, this step uses
`gff_opt_in` / `Test-GffOptIn` — it runs **only** when the flag resolves to
exactly `true`, so it can never install itself by accident on a machine where
gff (or the flag export) is missing.

## Pieces

| File | Role |
| :-- | :-- |
| `opt/Desktop/Apps/scripts/setup-security-audit.ps1` | Installer: registers the collector task, seeds the Claude task prompt. `-Status` / `-Uninstall` / `-At HH:mm` |
| `opt/Desktop/Apps/scripts/security-audit-collect.ps1` | Read-only collector (services, scheduled tasks, startup commands, AppData/Temp processes, Defender) |
| `opt/Desktop/Apps/scripts/security-audit-skill.template.md` | Claude analysis-task prompt template (bootstrap-baseline mode) |
| `opt/lib/gff.sh` (`gff_opt_in`) + `lib/gff.ps1` (`Test-GffOptIn`) | Fail-closed opt-in gates |
| `opt/bin/install_windows.sh` (`run_security_audit_setup`) | install.sh wiring — runs at the deferred Windows phase, after the Desktop deploy |

## Enable

```sh
gff set install.windows.security-audit true
./install.sh          # the setup runs in the deferred Windows phase (after the deploy)
```

Or run it standalone from PowerShell (manual run = explicit intent, no flag
needed; no admin either — it is a per-user scheduled task):

```powershell
powershell -ExecutionPolicy Bypass -File $env:USERPROFILE\Desktop\Apps\scripts\setup-security-audit.ps1
```

Note the Desktop may be OneDrive-redirected; the deploy step handles that, and
the installer copies the collector to `%USERPROFILE%\Claude\SecurityAudit` so
the scheduled task never executes from a OneDrive-dehydratable path.

## One-time Claude-side step

The installer seeds the analysis prompt at
`%USERPROFILE%\Claude\Scheduled\weekly-security-audit\SKILL.md` **only if it does
not already exist** — the audit baseline lives inside that file and evolves with
each accepted change, so the installer must never clobber it. Registering the
schedule itself is app-managed: open Claude and say
"schedule the weekly-security-audit task for Saturdays at 10:00" (any time after
the daily 09:00 collection works).

On the first analysis run the task is in **bootstrap-baseline mode**: it
summarizes the report, asks you to confirm anything unrecognized, then embeds
the accepted baseline into its own SKILL.md.

## Operations

```powershell
setup-security-audit.ps1 -Status      # task state, last/next run, report freshness
setup-security-audit.ps1 -At 07:30    # reinstall with a different collection time
setup-security-audit.ps1 -Uninstall   # removes task + collector; keeps data + baseline
```

## Security posture & caveats

- The collector is **read-only** (CIM/WMI, `Get-ScheduledTask`, `Get-Process`,
  `Get-MpComputerStatus`); it needs no admin and makes no network calls. The
  only write outside `%USERPROFILE%\Claude` is an optional copy of the report
  into a local Google Drive sync folder (`My Drive`) when one exists, giving the
  analysis run a fallback read path via the Drive connector.
- **A broken lens is never silence.** Each section resolves to one of three
  states — items, `(none)` (ran, found nothing), or
  `!! COLLECTION ERROR: <msg>` (the lens itself failed). The analysis prompt is
  required to report an errored section as *unverified* and is forbidden from
  calling the machine clean while one is present. Without this an unavailable
  `Get-MpComputerStatus` produced an empty DEFENDER section that read exactly
  like "Defender is fine".
- **Section 1 deliberately does not exclude `C:\Windows\Temp\`.** The
  non-Microsoft heuristic skips services under `\Windows\`, but that directory
  is user-writable by default and is a standard persistence location, so
  excluding it would blind the audit's highest-value lens. Services with a null
  `PathName` also stay visible rather than being filtered away.
- The collector task itself shows up in audit section 2 — the analysis prompt
  expects it (`ClaudeSecurityAuditCollector`).
- `StartWhenAvailable` + battery-friendly settings mean a laptop asleep at 09:00
  collects on wake; the analysis task tolerates data up to 3 days old before
  flagging staleness.
- Claude scheduled runs only fire while the Claude app is open; a missed
  Saturday run executes on next launch and reads whatever the latest collection
  was — the timestamp line keeps this honest.
