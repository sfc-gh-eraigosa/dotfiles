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
- **R2 — same lenses**: collection preserves the five audit sections and their
  exact filters/format so the existing baseline comparison carries over.
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
