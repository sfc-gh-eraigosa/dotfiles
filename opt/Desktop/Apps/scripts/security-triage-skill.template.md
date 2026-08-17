---
name: daily-security-triage
description: Daily URGENT-ONLY security triage - read the pre-collected audit report and speak up only if something needs action today. Silent when clean.
---

Run a DAILY URGENT-ONLY triage of this Windows machine by reading the pre-collected
report. Do NOT use computer use, do NOT call request_access, and do not ask the user to
paste anything - this runs fully unattended.

**This task is the smoke alarm, not the weekly report.** Its companion task
`weekly-security-audit` produces the full Saturday summary with the complete inventory and
baseline maintenance. Your job is the opposite: stay quiet unless something needs attention
*today*. Optimizing for silence is what makes the alarm meaningful — a daily task that
always says something trains the user to ignore it.

## DATA SOURCE (try in order; use the first that works)

1. Read `latest-audit.txt` from your uploads folder (this task's own folder receives it).
2. Read `%USERPROFILE%\Claude\SecurityAudit\latest-audit.txt` (canonical copy).
3. If a Google Drive connector is available, search it for `security-audit-latest.txt`.
4. If none are readable: say the collection pipeline looks broken and suggest
   `setup-security-audit.ps1 -Status`. Do NOT fall back to computer use.

## FRESHNESS GATE (check this first)

The first line is `AUDIT TIMESTAMP: ...`. The collector runs every evening, so:

- **Fresh (< 36h)** — triage normally.
- **Stale (36h–4 days)** — the machine may simply have been off. Mention it in ONE line only
  if you are already reporting something else; a stale report alone is not urgent.
- **Very stale (> 4 days)** — that IS worth one line on its own: the collector has probably
  stopped. Suggest `setup-security-audit.ps1 -Status`.

## WHAT COUNTS AS URGENT

Report **only** these. Everything else waits for the Saturday summary.

- Any section carrying `!! COLLECTION ERROR` (a lens failed — the machine is partly unaudited).
- **C1/C4** — real-time or tamper protection OFF; a `[REALTIME-DISABLED]` or
  `[EXCLUSION-CHANGE]` event.
- **C3** — any Defender detection event (1116/1117). A live malware hit is always urgent.
- **D1** — `Security log cleared` or `System log cleared` (anti-forensics).
- **E1** — a firewall profile DISABLED.
- **H2/H3/H4** — a new member of Administrators; a previously-disabled Administrator/Guest
  account now enabled; a new `[HIDDEN-LOGIN-SCREEN]` user; a new `AutoAdminLogon` /
  `[DEFAULTPASSWORD-STORED]`.
- **A2/A3/A4/A6/A7** — anything appearing in the high-signal ASEPs (`Policies\Explorer\Run`,
  `RunOnceEx`), a Winlogon `Shell`/`Userinit` deviation, an IFEO `Debugger`, a non-empty
  `AppInit_DLLs`, a `COR_PROFILER`/logon-script entry, or a per-user handler hijack.
- **B1/B4/G1/G2** — an unsigned service or process from a user-writable path; a new WMI
  `__EventConsumer` running a command; a `[MASQUERADE]` critical process off System32.
- **E4/F2/F3/F4** — a new portproxy rule; a hosts entry hijacking a security/OS domain; a
  rogue DNS server on a *physical* adapter; a newly-set proxy / `AutoConfigURL`.
- Any listener or public connection whose process carries `[SIG:*]` or `[USER-WRITABLE]`,
  **including inside a `[VOLATILE]` block** (the port churns; the signature verdict does not).

## WHAT IS NOT URGENT (never report daily)

Version-string churn; `[CHURN]`-tagged start-type flaps; new vendor/updater services and
tasks; `(none)` sections; `## ADMIN-REQUIRED` / `HIDDEN-BY-POLICY` standing gaps; new
patches; `[VOLATILE]` port/endpoint identity changes; `SignatureAge`, `records=`, `(Nd ago)`
counters drifting; browser CodeIntegrity events. All of that belongs to the Saturday summary.

## OUTPUT CONTRACT

- **Nothing urgent** → reply with exactly one short line, e.g.
  `Daily triage: clean (report <age>h old, all sections ran).` Nothing more.
- **Something urgent** → lead with the single most severe finding in the first sentence, then
  a short bullet per item: what changed, which section, and the one action to take. Keep the
  whole thing under ~10 lines; the Saturday task does depth.
- Never call the machine clean while a `!! COLLECTION ERROR` is present — say which lens is
  blind instead.
- Do **not** rewrite the baseline. Baseline maintenance belongs to `weekly-security-audit`;
  if you see a change that looks legitimate, note it and let the weekly run absorb it.

BASELINE: this task deliberately keeps **no** baseline of its own — it reads the same report
and applies the urgency rules above. The authoritative baseline lives in the
`weekly-security-audit` task's SKILL.md.
