---
name: weekly-security-audit
description: Weekly Windows security check - diff services/startup/scheduled tasks against baseline, verify Defender status
---

Run the weekly security audit of this Windows machine by ANALYZING THE PRE-COLLECTED
REPORT. Do NOT use computer use, do NOT call request_access, and do not ask the user to
paste anything - this task is designed to run fully unattended. Collection is done
separately by the Windows scheduled task "ClaudeSecurityAuditCollector"
(security-audit-collect.ps1, installed by dotfiles setup-security-audit.ps1), which
writes a fresh report daily.

DATA SOURCE (try in order; use the first that works):

1. Read latest-audit.txt from your uploads folder (this task's own folder receives it).
2. Read %USERPROFILE%\Claude\SecurityAudit\latest-audit.txt (canonical copy).
3. If a Google Drive connector is available, search it for "security-audit-latest.txt"
   and read that.
4. If none are readable: report that the collection pipeline is broken - the Windows
   task 'ClaudeSecurityAuditCollector' may be missing or failing. Suggest running:
   powershell -ExecutionPolicy Bypass -File %USERPROFILE%\Desktop\Apps\scripts\setup-security-audit.ps1 -Status
   Do NOT fall back to computer use.

FRESHNESS: the first line is "AUDIT TIMESTAMP: ...". If it is older than 3 days, note
the staleness (collector may not be running) but still analyze the data.

THE REPORT has 5 sections: (1) NON-MICROSOFT AUTO-START SERVICES,
(2) NON-MICROSOFT ACTIVE SCHEDULED TASKS, (3) STARTUP COMMANDS,
(4) PROCESSES FROM APPDATA/TEMP, (5) DEFENDER.

COMPARE against the BASELINE below and report ONLY:

- new or removed items (identify what a new item belongs to before alarming the user;
  recommend removal steps only for genuinely unwanted items)
- anything running from AppData\Roaming, AppData\Local\Temp, Downloads, or other
  unusual paths
- Defender problems: real-time protection off, signatures older than ~1 week, or no
  recent quick scan
- do NOT flag version-number churn (updater services embed version strings - treat
  "Vendor Updater Service (NNN)" as the same item across versions)
- expected: the collector's own artifacts (task 'ClaudeSecurityAuditCollector';
  Win32 lens quirks vary by machine - see baseline notes once established).
If everything matches, tell the user in one or two sentences that all is clean.

BASELINE: none recorded yet - BOOTSTRAP MODE. On the first successful run: treat the
report as the baseline candidate. Summarize each section briefly, ask the user to
confirm anything unrecognized, then UPDATE THIS TASK (via the scheduled-task update
tool) embedding the accepted baseline in this section, replacing this bootstrap text.
On later runs, keep the baseline current the same way: fold in user-accepted changes
and append a dated line to AUDIT HISTORY below.

AUDIT HISTORY: (none yet)
