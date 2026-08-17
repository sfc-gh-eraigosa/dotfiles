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
| `opt/Desktop/Apps/scripts/setup-security-audit.ps1` | Installer: registers the collector task (hourly evening window), seeds **both** Claude task prompts. `-Status` (health + version proof) / `-Uninstall` / `-At HH:mm` / `-WindowHours N` |
| `opt/Desktop/Apps/scripts/security-audit-collect.ps1` | Read-only collector — ~40 anomaly lenses (below). Params `-StdOut` / `-BaseDir` / `-Days` / `-OncePerDay` / `-LowPriority` |
| `opt/Desktop/Apps/scripts/security-audit-skill.template.md` | **Weekly** analysis prompt: per-section triage (CRITICAL/WARN/INFO), STABLE-vs-VOLATILE handling, known-benign rules, bootstrap-baseline mode |
| `opt/Desktop/Apps/scripts/security-triage-skill.template.md` | **Daily** urgent-only triage prompt: one line when clean, escalates only what needs action today; keeps no baseline of its own |
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

## Collection schedule (why hourly, not one fixed time)

A single fixed daily time is fragile: if the laptop is asleep at that moment, the
day is simply missed. So the Windows task fires **every hour across an evening
window** — by default **17:00 → 00:00** — and the collector's `-OncePerDay` gate
makes every fire after the first one essentially free:

| | Cost |
| :-- | :-- |
| Real collection (first fire of the day) | ~28s wall, ~19s CPU |
| Gate check (every later fire) | ~4ms — a single file stat |

The gate keys on the canonical `latest-audit.txt` write date, so "already have
today's data" is the real signal. A run that dies *before* writing it leaves the
gate open, so the next hour retries automatically. Combined with
`StartWhenAvailable`, a machine that was off all evening still collects at the
next opportunity. Net: **at most one real collection per day, at the first moment
the machine is actually available.**

CPU hygiene is deliberate: task `Priority 7` (background tier), the collector runs
`-LowPriority` (BelowNormal), `MultipleInstances=IgnoreNew` so a slow run is never
double-started by the next hourly fire, and a 10-minute `ExecutionTimeLimit` caps
any runaway.

Change the window with `-At` / `-WindowHours`:

```powershell
setup-security-audit.ps1 -At 18:00 -WindowHours 6   # 18:00 -> 00:00
setup-security-audit.ps1 -At 09:00 -WindowHours 0   # single fixed fire, old behavior
```

## Two Claude tasks: weekly summary + daily triage

The installer seeds **two** prompts, each **only if it does not already exist** —
the audit baseline lives inside the weekly one and evolves with each accepted
change, so the installer must never clobber it.

| Task | Cadence | Job |
| :-- | :-- | :-- |
| `weekly-security-audit` | Saturday | The **full summary** — complete inventory, baseline diff, and baseline maintenance |
| `daily-security-triage` | Daily (evening) | **Urgent-only smoke alarm** — one line when clean, speaks up only for things that need action *today* |

The split exists so a daily cadence doesn't destroy the signal: a task that says
something every day trains you to ignore it. The triage prompt keeps **no baseline
of its own** (the weekly task owns that) and is explicitly instructed to stay
quiet — its urgent list is limited to live malware detections, protection being
turned off, log clears, new admins/hidden users, high-signal ASEP entries,
unsigned processes from user-writable paths, and network-pivot changes.

Registering the schedules is app-managed. Open Claude and say:

```text
schedule the weekly-security-audit task for Saturdays at 10:00
schedule the daily-security-triage task daily at 23:30
```

Pick analysis times *after* the collection window so each run reads a fresh
report.

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

## Verifying the pipeline

`-Status` is the single command that answers both "is it running?" and "is it the
current script?". It is read-only and needs no elevation:

```powershell
powershell -ExecutionPolicy Bypass -File $env:USERPROFILE\Desktop\Apps\scripts\setup-security-audit.ps1 -Status
```

```text
Task     : ClaudeSecurityAuditCollector  [Ready]
Schedule : daily at 17:00  repeating every PT1H for PT7H
LastRun  : 8/17/2026 17:04:11
LastResult: 0 (0x00000000 : success)
NextRun  : 8/17/2026 18:00:00
Runs     : powershell.exe ... -File "...\security-audit-collect.ps1" -OncePerDay -LowPriority

Collector identity (what the task actually executes):
  installed : v3     sha256 11DFF4D54302B021  2026-08-16 19:23
  deployed  : v3     sha256 11DFF4D54302B021  2026-08-16 19:23
  => UP TO DATE (installed is byte-identical to the deployed copy)

Report   : C:\Users\<you>\Claude\SecurityAudit\latest-audit.txt
           produced by v3, written 8/17/2026 17:04 (1h ago, 22157 bytes)
History  : 6 dated report(s) (newest audit-2026-08-17.txt)

Claude task prompts:
  weekly-security-audit  SKILL.md present
  daily-security-triage  SKILL.md present
```

### 1. Proving you're running the latest collector

There are **three** copies of the collector and they can drift:

```text
repo  opt/Desktop/Apps/scripts/security-audit-collect.ps1
  │  install.sh  (Desktop deploy — every run)
  ▼
deployed  %USERPROFILE%\Desktop\Apps\scripts\security-audit-collect.ps1
  │  setup-security-audit.ps1  (ONLY when the installer is re-run)
  ▼
installed %USERPROFILE%\Claude\SecurityAudit\security-audit-collect.ps1   ← what the task runs
```

The drift that matters is the second hop: `install.sh` refreshes the **deployed**
copy on every run, but the **installed** copy only updates when the installer is
re-run. `-Status` compares them by **SHA-256** and by the `COLLECTOR_VERSION`
marker, and states the verdict outright:

- `=> UP TO DATE` — installed is byte-identical to deployed. This is proof, not inference.
- `=> STALE: installed differs from deployed` — the task is running an old script.
  Fix by re-running the installer (it prints the exact command).

Two independent cross-checks, in case you don't trust the tool reporting on itself:

- **Every report carries the version that produced it** — line 2 of
  `latest-audit.txt` reads `COLLECTOR: v3 …`. If the report says `v2`, a `v2`
  collector produced it, regardless of what is installed now.
- **Hash it yourself**:

  ```powershell
  Get-FileHash $env:USERPROFILE\Claude\SecurityAudit\security-audit-collect.ps1 -Algorithm SHA256
  Get-FileHash $env:USERPROFILE\Desktop\Apps\scripts\security-audit-collect.ps1 -Algorithm SHA256
  ```

### 2. Proving the job is set up and running

`-Status` reports task health directly, including a **decoded** `LastTaskResult`
(Task Scheduler returns raw HRESULTs, which are meaningless as bare integers):

| Result | Meaning |
| :-- | :-- |
| `0 (0x00000000)` | success — the last run completed |
| `267011 (0x41303)` | **task has NOT YET RUN** — registered but never fired |
| `267009 (0x41301)` | currently running |
| `267010 (0x41302)` | task is disabled |
| `267014 (0x41306)` | terminated by the user |
| `2147750687 (0x8004131F)` | an instance was already running (expected with hourly fires) |

**The strongest evidence is the data, not the task metadata.** A dated file in
`history\` per day proves it really ran, unattended, repeatedly:

```powershell
Get-ChildItem $env:USERPROFILE\Claude\SecurityAudit\history | Sort-Object Name -Descending | Select-Object -First 7
```

Raw Windows equivalents, if you'd rather not trust the wrapper:

```powershell
Get-ScheduledTask -TaskName ClaudeSecurityAuditCollector | Get-ScheduledTaskInfo
Get-ScheduledTask -TaskName ClaudeSecurityAuditCollector | Select-Object -Expand Triggers
Start-ScheduledTask -TaskName ClaudeSecurityAuditCollector      # force a run right now
```

`Start-ScheduledTask` is the fastest end-to-end smoke test: run it, wait ~30s, then
re-run `-Status` and confirm `LastResult: 0` and a fresh `written` timestamp. (If
today's report already exists the run will hit the `-OncePerDay` gate and exit in
milliseconds without rewriting it — that is the gate working, not a failure. To
force a real collection, run the collector directly without `-OncePerDay`.)

If the task never seems to fire, check Task Scheduler's own history — it is
**disabled by default**, which the companion
[security-hardening](./security-hardening.md) feature turns on.

## Operations

```powershell
setup-security-audit.ps1 -Status      # task health, version proof, report freshness
setup-security-audit.ps1 -At 18:00 -WindowHours 6   # reinstall with a different window
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
