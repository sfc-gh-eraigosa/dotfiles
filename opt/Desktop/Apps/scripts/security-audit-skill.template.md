---
name: weekly-security-audit
description: Weekly Windows security audit - diff a pre-collected read-only anomaly report (persistence, defense-evasion, network, process, accounts, posture) against a saved baseline and report only genuine surprises. Unattended, no computer use.
---

Run the weekly security audit of this Windows machine by ANALYZING THE PRE-COLLECTED
REPORT. Do NOT use computer use, do NOT call request_access, and do not ask the user to
paste anything - this task runs fully unattended. Collection is done separately by the
Windows scheduled task "ClaudeSecurityAuditCollector" (security-audit-collect.ps1,
installed by dotfiles setup-security-audit.ps1), which writes a fresh read-only report
daily. You never run commands; you read one text file and reason about it.

## DATA SOURCE (try in order; use the first that works)

1. Read `latest-audit.txt` from your uploads folder (this task's own folder receives it).
2. Read `%USERPROFILE%\Claude\SecurityAudit\latest-audit.txt` (canonical copy).
3. If a Google Drive connector is available, search it for `security-audit-latest.txt`.
4. If none are readable: report that the collection pipeline is broken - the Windows task
   `ClaudeSecurityAuditCollector` may be missing or failing. Suggest running:
   `powershell -ExecutionPolicy Bypass -Command "& ([Environment]::GetFolderPath('Desktop') + '\Apps\scripts\setup-security-audit.ps1') -Status"`
   (resolve the Desktop like that — with OneDrive Backup on, `%USERPROFILE%\Desktop`
   does not exist)
   Do NOT fall back to computer use.

## REPORT SHAPE

The header carries `AUDIT TIMESTAMP`, `COLLECTOR: vN`, `WINDOW: Nd`, `ELEVATED`, host, and
OS build. Then ~30 lettered sections (A1..K). **Every section resolves to exactly one of
these four states — they mean very different things, so tell them apart:**

- **`<data lines>`** - the lens found items; analyze them.
- **`(none)`** / `<empty>` / `<absent>` - the lens ran and genuinely found nothing. CLEAN.
- **`!! COLLECTION ERROR: <msg>`** - the lens itself FAILED. This is NOT clean. Report the
  section as *unverified this run* and quote the error. Never summarize the machine as clean
  while any COLLECTION ERROR is present.
- **`## ADMIN-REQUIRED (<Type>): <what>`** - the data needs elevation the collector does not
  have (Security event log, Secure Boot). This is an EXPECTED, standing coverage gap, not an
  anomaly. State once that these lenses are unavailable non-admin; do not re-alarm each week.
- **`HIDDEN-BY-POLICY (contents UNKNOWN without elevation)`** (C2 exclusions) - the same kind
  of standing gap: `HideExclusionsFromLocalUsers=True` hides Defender exclusion CONTENTS from a
  non-admin. Treat it exactly like `## ADMIN-REQUIRED` (note once, don't re-alarm). The only
  non-admin signal that an exclusion CHANGED is a C4 `[EXCLUSION-CHANGE]` event - that IS
  CRITICAL.

Many sections split into **`[STABLE]`** and **`[VOLATILE]`** blocks. Strict-diff the STABLE
block (any change = investigate). The VOLATILE block (ephemeral ports, per-run outbound
endpoints, virtual-adapter DNS, dynamic RPC ports) is for triage only - **do NOT alarm on
its churn**; use it only to corroborate a STABLE-block finding.

**Event-log sections** (C3, C4, D1, I3, J1, J2) list dated events inside the window. Compare
the *set and kind* of events against the baseline, not exact timestamps. Lines tagged
`[CHURN]` (BITS/Windows-Update/servicing start-type flapping) are benign noise - ignore
unless the target is a *security* service being disabled.

**A `[SIG:*]` or `[USER-WRITABLE]` tag is alarm-worthy WHEREVER it appears - including inside a
`[VOLATILE]` block.** The volatile-block rule ignores the *identity* that churns (the ephemeral
port number, the specific remote endpoint), NOT the signature verdict on the owning process: an
unsigned / user-writable process listening on a dynamic port (E2 `[VOLATILE-DYNAMIC]`) or holding
a public connection is a real backdoor indicator. Read every block for those two tags; only
suppress churn on the port/endpoint identity itself.

## FRESHNESS

The first line is `AUDIT TIMESTAMP: ...`. If it is older than 3 days, note the staleness
(the collector may not be running - suggest `-Status`) but still analyze the data.

## HOW TO TRIAGE - severity tiers

Report findings grouped by severity, most-severe first. Report **only deltas from baseline
and standing red flags** - never re-list the whole benign inventory.

- **CRITICAL (act now)** - real-time protection / tamper protection OFF (C1); a firewall profile
  DISABLED (E1); UAC disabled `EnableLUA=0` (I2); a NEW member of Administrators (H2); an enabled
  Administrator/Guest account that was disabled (H3); a Defender `[EXCLUSION-CHANGE]` or
  `[REALTIME-DISABLED]` event (C4); **a Defender detection event (C3, 1116/1117 = live malware
  hit)**; **any change to Winlogon `Shell`/`Userinit` beyond the exact defaults, or any HKCU
  Winlogon value set (A3)**; **any IFEO `Debugger` or non-empty `AppInit_DLLs` (A4)**; **any value
  appearing in A2 (`Policies\Explorer\Run`/`RunOnceEx` - these should be empty)**; a new WMI
  `__EventConsumer` running a command/script (B4); a new `COR_PROFILER`/`UserInitMprLogonScript`
  or handler-hijack (A6/A7); an unsigned service or process from a user-writable path (B1/G1); a
  `[MASQUERADE]` critical process off System32 (G2); a new listener on an exposed well-known port,
  or ANY listener/connection whose process carries a `[SIG:*]`/`[USER-WRITABLE]` tag (E2/F1); a new
  portproxy rule (E4); a rogue DNS server on a physical adapter (F3); a hosts-file entry hijacking
  a security/OS domain (F2); **`Security log cleared` OR `System log cleared` (D1 - anti-forensics)**;
  a new `AutoAdminLogon`/`[DEFAULTPASSWORD-STORED]` (H3); a new `[HIDDEN-LOGIN-SCREEN]` user (H4).
- **WARN (review)** - a new non-Microsoft auto-start service or scheduled task (B1/B2 - identify
  what it belongs to first); **a new Run-key value (A1) or Startup-folder item (B3) or COM
  server (A5)**; a new `[SUSPICIOUS-ACTION]` task (B2); a new CodeIntegrity unsigned-load target
  that is not a known browser/vendor (I3); a new 7045 service install, especially `[USER-WRITABLE]`
  (J1); a new remote-access service present/running (E5); a password-not-required enabled account
  (H1); proxy `ProxyServer`/`AutoConfigURL` newly set (F4); a jump in `keyword-suspicious` 4104
  count with `[REVIEW]` lines you can't explain (J3).
- **INFO (note, don't alarm)** - version-string churn in paths/names/services; VOLATILE-block
  *identity* changes (ports/endpoints); `[CHURN]` start-type flaps; benign vendor updater
  services/tasks re-registering; new patches; per-run outbound endpoint rotation; the drift of
  `SignatureAge(days)`, `LastQuickScanEnd`, `records=N`, `(Nd ago)`, and the suppressed-count
  lines in C4/J3 (all increment normally). **But: a `records=N` that DROPS on any channel (D2)
  corroborates a D1 log-clear - escalate that; and a `SignatureAge` above ~7 days (C1) is a WARN.**

## KNOWN-BENIGN PATTERNS (do NOT flag these)

- **Version churn**: treat `Vendor Updater Service (148.0.7778)` and `...(150.0.8001)` as the
  same item; the collector already normalizes many versions to `X.X` - ignore residual churn.
- **Vendor autostarts**: browser/updater/vendor tray services and tasks (Chrome/Edge auto-launch,
  OneDrive, Discord/Slack/Spotify/Steam/Zoom updaters, Razer/Asus/vendor services) are baseline.
- **Browser CodeIntegrity events**: Chrome/Edge loading `vulkan-1.dll`/`vk_swiftshader.dll`, and
  screen-capture hooks (OBS `graphics-hook64.dll`) flagged by CodeIntegrity, are routine - note
  only a NEW, non-browser, non-vendor unsigned-load target.
- **The collector's own artifacts**: the `ClaudeSecurityAuditCollector` task; the benign built-in
  WMI `SCM Event Log Filter`; this dotfiles machine's own `macOS Hotkeys` / PowerToys tasks.
- **Servicing churn**: 5007 "configuration changed" volume (already suppressed to a count),
  BITS/TrustedInstaller/WaaSMedic `[CHURN]` start-type flaps.

Always identify what a new item belongs to before alarming, and recommend removal steps only
for genuinely unwanted items. Do NOT propose changing configuration the user runs deliberately
(firewall posture, installed vendor tools, the portproxy the user set up) - only surface the
*surprise*.

## BASELINE

BASELINE: none recorded yet - **BOOTSTRAP MODE**. On the first successful run: treat the report
as the baseline candidate. Summarize each section group briefly (persistence / defense-evasion /
network / process / accounts / posture), then UPDATE THIS TASK (via the scheduled-task update
tool) embedding the accepted baseline in the section below, replacing this bootstrap text. Record
per-section what is known-good on THIS host (the services, tasks, Run keys, listeners, admin-group
members, portproxy rules, WMI subscriptions) so later runs diff against it. On later runs, keep
the baseline current the same way: fold in user-accepted changes and append a dated line to
AUDIT HISTORY.

**Baseline-poisoning guard (first run only).** The first snapshot is auto-accepted as ground
truth, so if the host is ALREADY compromised at bootstrap the persistence gets baked in as
"known-good" and never alarms again. So on the FIRST run, do NOT silently baseline the
higher-risk items - explicitly LIST them in the report for the user to confirm before they
become baseline: any unsigned service or user-writable-path autostart (B1/G1), every
Administrators member (H2), any WMI `__EventConsumer` (B4), any portproxy rule (E4), any
non-empty A2/A6/A7 entry, and any enabled non-standard local user (H1/H3). Frame them as "please
confirm these are expected" rather than as alarms. Everything else may be baselined silently.

If everything matches the baseline AND every section ran (no COLLECTION ERROR), tell the user in
one or two sentences that all is clean, noting the standing ADMIN-REQUIRED coverage gaps once.

BASELINE DATA: (none yet - bootstrap)

AUDIT HISTORY: (none yet)
