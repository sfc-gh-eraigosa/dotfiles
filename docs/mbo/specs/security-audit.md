# Unattended Weekly Security Audit — spec

**Status:** building · **Slug:** `security-audit` · **Owner:** edward-raigosa

## Problem

The `weekly-security-audit` Claude scheduled task needed a human twice per run:
computer-use approvals cannot happen in unattended runs (observed 2026-08-16:
grants stored on the task did not carry into the run), and terminals are
click-tier, so Claude cannot type or paste into PowerShell even when granted.
Result: a "scheduled" audit that stalls until the user replies and pastes.

## Requirements

- **R1 — no human in the loop**: a weekly audit run completes with zero
  approvals, zero pastes, zero computer-use calls.
- **R2 — comprehensive anomaly lenses (v2)**: collection covers ~40 read-only,
  non-admin lenses across persistence (registry ASEPs, services, tasks, WMI
  subscriptions), defense evasion (Defender health/exclusions/events, log
  integrity), network exposure + C2 (firewall, listeners, portproxy, outbound,
  hosts/DNS/proxy), process anomaly (user-writable paths, masquerade), accounts
  & privilege, patch/integrity posture, and 7-day event-log signals (7045/7040/
  4104/CodeIntegrity). Each lens is ATT&CK-mapped and grounded in what a non-admin
  user can actually read (verified by live host probing). Section 1 does not
  exclude `C:\Windows\Temp\`; null-`PathName` services stay visible.
- **R2a — no silent blindness**: every section resolves to items, `(none)`,
  `!! COLLECTION ERROR: <msg>`, or `## ADMIN-REQUIRED (<Type>)`. A lens that
  fails is reported as unverified; a lens that needs elevation renders a
  byte-stable admin-gap marker (exception TYPE only). The analysis may not call
  the host clean while any COLLECTION ERROR is present. (Previously a failed
  lens wrote an empty section that read as "clean".)
- **R2b — false-positive discipline**: version strings normalized to `X.X`;
  `[USER-WRITABLE-PATH]` alarms only on unsigned images; scheduled-task
  suspicion keys on the executable not incidental args; Defender 5007 churn
  collapses to a count; servicing start-type flaps `[CHURN]`-tagged; 4104
  self-excludes the collector's own script; `[STABLE]` vs `[VOLATILE]` blocks
  separate strict-diff from triage-only surfaces. Validated by a live read-only
  run against a real Windows 11 host (non-admin).
- **R2c — non-admin coverage honesty**: the Security event log, Defender
  exclusion contents, and Secure Boot state are admin-only; the collector
  surfaces each as an explicit `## ADMIN-REQUIRED` gap, never a misleading
  `(none)`, and documents how to light them up (Event Log Readers / elevation).
- **R2d — detection only**: the collector never changes configuration (spec
  scope is anomaly reporting for a weekly review, not remediation).
- **R11 — availability-tolerant cadence**: a laptop is frequently asleep at any
  fixed time, so collection must not depend on the machine being up at one
  moment. The task fires **hourly across an evening window** (default
  17:00→00:00) with `StartWhenAvailable`, and the collector carries a
  `-OncePerDay` gate so only the first fire of the day does real work. Measured:
  a real collection is ~28s wall / ~19s CPU; the gate is ~4ms. Net contract:
  **at most one collection per day, at the first moment the host is available.**
- **R12 — cheap and stable**: the repeated fires must not be a CPU tax. Task
  `Priority 7`, collector `-LowPriority` (BelowNormal),
  `MultipleInstances=IgnoreNew` (a slow run is never double-started by the next
  hourly fire), 10-minute `ExecutionTimeLimit`. The gate keys on the canonical
  report's write date, so a run that dies before writing leaves the gate open and
  the next hour retries — self-healing, not sticky.
- **R13 — separate urgency from summary**: the full inventory is wanted weekly
  (Saturday), but anything *urgent* is wanted the same day. Two Claude prompts:
  `weekly-security-audit` (full summary, owns the baseline) and
  `daily-security-triage` (urgent-only, keeps no baseline, one line when clean).
  A daily task that always speaks trains the reader to ignore it, so silence is
  the triage task's designed default.
- **R14 — provable provenance**: it must be answerable, not assumable, *which*
  collector is actually running. Three copies exist (repo → deployed → installed)
  and only `install.sh` refreshes the middle one, so `-Status` compares the
  installed and deployed copies by `COLLECTOR_VERSION` **and** SHA-256 and states
  `UP TO DATE` / `STALE` outright. Each report also self-identifies via its
  `COLLECTOR: vN` header line.
- **R15 — provable liveness**: `-Status` must answer "is the job set up and
  running?" without the reader decoding HRESULTs — it decodes `LastTaskResult`
  (e.g. `267011` → "task has NOT YET RUN"), shows the schedule incl. repetition,
  report freshness, and the dated `history\` count as the strongest evidence.
- **R3 — opt-in, fail-closed**: gated by `install.windows.security-audit`
  (`boolDefault: false`); absent gff/flag ⇒ the step must NOT run. This
  inverts the repo's fail-open `gff_on` convention deliberately.
- **R4 — no admin**: per-user scheduled task; read-only queries only.
- **R5 — baseline is state, not config**: the Claude SKILL.md (holding the
  evolving baseline) is seeded only when absent, never overwritten.
- **R6 — resilient handoff**: analysis reads the task-folder copy first, the
  canonical `%USERPROFILE%\Claude\SecurityAudit\latest-audit.txt` second, an
  optional Google Drive copy third; a broken pipeline is reported, not worked
  around.
- **R7 — laptop reality**: collection uses `StartWhenAvailable` +
  battery-friendly settings; analysis tolerates ≤3-day-old data, flags older.
- **R8 — portability**: no hardcoded usernames/paths; `$env:USERPROFILE` on
  the Windows side, `${HOME}`/`wslpath` on the WSL side.

## Non-goals

- Elevated lenses (autoruns-style kernel/driver enumeration) — out of scope.
- Scheduling the Claude-side task from the installer — app-managed; the doc
  ships the one-line instruction instead.
- Fleet distribution (see `fleet` objective) — this is single-host.

## Acceptance criteria

1. `install.sh` with the flag unset/false → `SKIP (gff: … opt-in …)` line; no
   task, no files.
2. `gff set install.windows.security-audit true && ./install.sh` → task
   `ClaudeSecurityAuditCollector` registered (daily, user-level), first
   report written, SKILL.md seeded when absent.
3. Re-running the installer never modifies an existing SKILL.md.
4. `setup-security-audit.ps1 -Status` reports task state + report freshness;
   `-Uninstall` removes task+collector but keeps data and baseline.
5. A Claude `weekly-security-audit` run completes unattended by reading
   `latest-audit.txt` (verified on wenlock's Vivobook after enabling).
6. `make lint-shell` / portability scan / markdownlint pass on the diff.
7. `opt/lib/gff_test.sh` covers the `gff_opt_in` truth table (unset / `true` /
   `false` / `TRUE` / `1` / `'true '` / empty), the key mangling, the
   gff-absent-from-PATH case, and an explicit assertion that `gff_on` and
   `gff_opt_in` disagree on an unset var — the inversion IS the feature, and a
   refactor that unified them would pass every other case. Green under **both**
   `bash` and `dash` (the F9 cross-shell gate).
