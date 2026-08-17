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

> **Companion feature:** [`security-hardening.md`](./security-hardening.md) applies the
> opt-in posture changes that close this audit's two visibility gaps (Security-log access
> and the TaskScheduler event channel) and adds Defender ASR rules in audit mode. Detection
> lives here; the reversible hardening lives there.

## Pieces

| File | Role |
| :-- | :-- |
| `opt/Desktop/Apps/scripts/setup-security-audit.ps1` | Installer: registers the collector task, seeds the Claude task prompt. `-Status` / `-Uninstall` / `-At HH:mm` |
| `opt/Desktop/Apps/scripts/security-audit-collect.ps1` | Read-only collector — ~40 anomaly lenses (below). Params `-StdOut` / `-BaseDir` / `-Days` for ad-hoc/test runs |
| `opt/Desktop/Apps/scripts/security-audit-skill.template.md` | Claude analysis-task prompt: per-section triage (CRITICAL/WARN/INFO), STABLE-vs-VOLATILE handling, known-benign rules, bootstrap-baseline mode |
| `opt/scripts/system/security-audit-collect_test.sh` | Test driver — static contract checks (CI) + live read-only run assertion (WSL/Windows) |
| `opt/lib/gff.sh` (`gff_opt_in`) + `lib/gff.ps1` (`Test-GffOptIn`) | Fail-closed opt-in gates |
| `opt/bin/install_windows.sh` (`run_security_audit_setup`) | install.sh wiring — runs at the deferred Windows phase, after the Desktop deploy |

## What the collector checks (non-admin, read-only, ~40 lenses)

Each lens is grounded in what a non-admin user can actually read on Windows 11 (verified by
live probing), mapped to MITRE ATT&CK, and tuned against false positives. Sections resolve to
`<data>` / `(none)` / `!! COLLECTION ERROR` / `## ADMIN-REQUIRED` — a blocked lens is never
silently empty. High-churn surfaces split into a `[STABLE]` (strict-diff) and `[VOLATILE]`
(triage-only) block.

| Group | Lenses (section ids) |
| :-- | :-- |
| **Persistence — registry ASEPs** | Run/RunOnce all hives (A1), high-signal Policies\Run + RunOnceEx (A2), Winlogon Shell/Userinit/Notify (A3), IFEO debuggers + AppInit_DLLs (A4), logon-script + `COR_PROFILER` .NET-injection + win.ini + `$PROFILE` + Active Setup (A6), UAC-bypass/file-handler hijacks (A7), COM hijack — user-writable/scriptlet HKCU CLSID (A5) |
| **Persistence — execution** | non-MS auto-start services + signature + path hygiene (B1), non-MS scheduled tasks + hidden/suspicious-action (B2), Startup-folder items (B3), WMI event-subscription persistence (B4) |
| **Defense evasion — Defender** | core status: RTP/tamper/behavior/cloud/sig-age (C1), exclusions (C2 — contents admin-only, gap surfaced honestly), detection events 1116/1117 (C3), security-relevant config-change 5001/5007 incl. exclusion-change (C4) |
| **Defense evasion — log integrity** | log-cleared 104/1102 (D1), channel health with `<absent>` markers (D2) |
| **Network — exposure** | firewall profiles (E1), TCP listeners well-known-strict/dynamic-volatile + signature (E2), non-default SMB shares (E3), portproxy pivot rules (E4), remote-access service state — RDP/SSH/VNC/TeamViewer/AnyDesk (E5) |
| **Network — anomaly / C2** | outbound to public IPs, process-strict/endpoint-volatile (F1), hosts-file entries (F2), DNS servers physical-strict/virtual-volatile (F3), proxy/AutoConfigURL (F4) |
| **Process & binary** | processes from user-writable paths + signature + parent (G1), critical-name masquerade off System32 (G2) |
| **Accounts & privilege** | local users + pw flags (H1), privileged-group membership (H2), built-in account state + autologon (H3), hidden users (H4) |
| **Patch & integrity posture** | patch staleness + WU services + WSUS-redirect (I1), UAC/SmartScreen/Secure-Boot (I2), CodeIntegrity unsigned-load events (I3) |
| **Event-log signals (7d)** | 7045 service-install w/ XML extraction (J1), 7040 start-type-change churn-tagged (J2), 4104 suspicious PowerShell self-excluded (J3) |
| **Coverage gap** | consolidated Security-log visibility gap — 4720/4624/1102 need admin (K) |

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
summarizes the report, explicitly lists the higher-risk items (unsigned
autostarts, Administrators members, WMI consumers, portproxy rules, new ASEP
entries) for you to confirm before they become baseline — a guard against
baselining an already-compromised host — then embeds the accepted baseline into
its own SKILL.md.

> **Operational linchpin:** the diff-vs-baseline model depends on the unattended
> analysis run being able to **self-edit its own `SKILL.md`** (via the
> scheduled-task update tool) to persist accepted state. If that write path is
> not available in the runtime, every run stays in bootstrap mode and re-summarizes
> the full inventory weekly. Verify the first run actually rewrites the baseline
> section; if it can't, treat the weekly output as a fresh full inventory rather
> than a diff.

## Operations

```powershell
setup-security-audit.ps1 -Status      # task state, last/next run, report freshness
setup-security-audit.ps1 -At 07:30    # reinstall with a different collection time
setup-security-audit.ps1 -Uninstall   # removes task + collector; keeps data + baseline
```

## Security posture & caveats

- The collector is **read-only** (CIM/WMI, registry reads, `Get-WinEvent`,
  `Get-NetTCPConnection`, `Get-MpComputerStatus`, `Get-AuthenticodeSignature`);
  it needs no admin and makes no network calls. **Detection only — it never
  changes configuration.** The only write outside `%USERPROFILE%\Claude` is an
  optional copy of the report into a local Google Drive sync folder (`My Drive`)
  when one exists, giving the analysis run a fallback read path.
- **A broken or blocked lens is never silence.** Each section resolves to
  exactly one of four states — items, `(none)`, `!! COLLECTION ERROR: <msg>`
  (the lens threw), or `## ADMIN-REQUIRED (<Type>)` (needs elevation we don't
  have). The analysis prompt reports an errored section as *unverified* and may
  not call the host clean while one is present. This is why an unavailable
  `Get-MpComputerStatus` can never masquerade as "Defender is fine".
- **Non-admin coverage limits (surfaced honestly, not hidden).** Three high-value
  surfaces are unreadable without elevation and render an `## ADMIN-REQUIRED`
  marker rather than a misleading `(none)`: the **Security event log** (4720
  new-user, 4624/4625 logons, 1102 clear-audit), **Defender exclusion contents**
  (`HideExclusionsFromLocalUsers=True` on this host — an attacker-added exclusion
  is then detectable non-admin *only* via the C4 5007 config-change event echo),
  and **Secure Boot state**. To light these up, add the collector account to
  *Event Log Readers* or run it elevated — that is exactly what the companion
  **[security-hardening](./security-hardening.md)** feature does (opt-in, one UAC
  prompt, reversible), after which section K reports data instead of a gap.
- **False-positive discipline is built in.** Version strings in paths/service
  names are normalized to `X.X`; the `[USER-WRITABLE-PATH]` alarm fires only on
  *unsigned* binaries (so Defender's own `ProgramData\...\Platform` services
  don't false-flag); scheduled-task suspicion keys on the *executable*, not
  incidental log-file arguments; Defender 5007 update-churn collapses to a count;
  BITS/servicing start-type flaps are `[CHURN]`-tagged; and the PowerShell 4104
  lens self-excludes the collector's own script (a keyword scan otherwise flags
  its own source). `[STABLE]` blocks are strict-diffed; `[VOLATILE]` blocks
  (ephemeral ports, per-run endpoints, virtual-adapter DNS) are triage-only.
- **Section 1 deliberately does not exclude `C:\Windows\Temp\`.** The
  non-Microsoft heuristic skips services under `\Windows\`, but that directory
  is user-writable and a standard persistence location, so excluding it would
  blind a high-value lens.
- The collector task itself, and this host's own dotfiles artifacts (the
  `macOS Hotkeys`/PowerToys tasks, the `refresh-wsl-portproxy` rule, the built-in
  `SCM Event Log Filter` WMI subscription) show up in the report — the analysis
  prompt lists them as expected baseline.
- `StartWhenAvailable` + battery-friendly settings mean a laptop asleep at 09:00
  collects on wake; the analysis task tolerates data up to 3 days old before
  flagging staleness.
- Claude scheduled runs only fire while the Claude app is open; a missed
  Saturday run executes on next launch and reads whatever the latest collection
  was — the timestamp line keeps this honest.
