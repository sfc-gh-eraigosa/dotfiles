# Unattended Weekly Security Audit — plan

**Status:** building · **Slug:** `security-audit` · **Spec:** [../specs/security-audit.md](../specs/security-audit.md)

Small feature: spec → plan, built in a single PR (no execution trio).

## Tasks

| # | Task | Files |
| :-- | :-- | :-- |
| T1 | Opt-in fail-closed gates: `gff_opt_in` (POSIX) + `Test-GffOptIn` (PS) | `opt/lib/gff.sh`, `opt/Desktop/Apps/scripts/lib/gff.ps1` |
| T2 | Flag `install.windows.security-audit` (`boolDefault: false`) | `.github/gff/features.yaml` |
| T3 | Read-only collector (5 sections, stable formats, history, Drive fallback copy) | `opt/Desktop/Apps/scripts/security-audit-collect.ps1` |
| T4 | Installer: user-level daily task, collector install, SKILL.md seed-if-absent, `-Status`/`-Uninstall`/`-At` | `opt/Desktop/Apps/scripts/setup-security-audit.ps1` |
| T5 | Claude analysis prompt template (bootstrap-baseline mode, data-source fallback chain, no-computer-use rule) | `opt/Desktop/Apps/scripts/security-audit-skill.template.md` |
| T6 | install.sh wiring at the deferred Windows phase: extract `export_gff_wslenv`, add `run_security_audit_setup` (gated, rc-captured, logged) | `opt/bin/install_windows.sh` |
| T7 | Docs: user guide, scripts inventory, docs index, MBO registration | `docs/security-audit.md`, `opt/Desktop/Apps/scripts/AGENTS.md`, `docs/AGENTS.md`, `docs/mbo/*` |
| T8 | Tests for the fail-closed gate: `gff_opt_in` truth table + the `gff_on`/`gff_opt_in` inversion assertion, green under bash **and** dash | `opt/lib/gff_test.sh` |
| T9 | **v2 collector expansion**: ~40 ATT&CK-mapped non-admin lenses (persistence/defense-evasion/network/process/accounts/posture/event-logs), STABLE-vs-VOLATILE fencing, version normalization, admin-gap markers, 4104 self-exclusion — designed via an adversarially-reviewed lens-catalog workflow and validated by a live read-only run on a real Win11 host | `opt/Desktop/Apps/scripts/security-audit-collect.ps1` |
| T10 | Analysis prompt rewrite for the v2 sections: CRITICAL/WARN/INFO triage, STABLE/VOLATILE + event-set handling, known-benign rules, admin-gap handling | `opt/Desktop/Apps/scripts/security-audit-skill.template.md` |
| T11 | Collector test driver: static contract checks (CI) + live read-only run assertion (WSL/Windows) | `opt/scripts/system/security-audit-collect_test.sh` |

## Validation

Run in the build sandbox (Linux — no Windows/PowerShell available):

- `bash -n` + dash `sh -n` on every touched shell file — pass.
- `bash opt/lib/gff_test.sh` **and** `sh opt/lib/gff_test.sh` — 20/20 pass.
- `bash opt/scripts/system/security-audit-collect_test.sh` — 35/35 pass (static + live).
- `opt/scripts/system/shell-portability-scan.sh --strict` (whole repo) — Tier 1: 0, Tier 2: 0.
- `.github/gff/features.yaml` YAML parse + flag-shape check — pass; `gff export --shell`
  emits `GFF_INSTALL_WINDOWS_SECURITY_AUDIT=false` by default (fail-closed confirmed).
- `make lint-shell` (shellcheck) and `make lint-markdown` (markdownlint-cli2) — pass.

**v2 collector validated on a real Windows 11 host** via WSL PowerShell interop: the
collector was run read-only (`-StdOut`, no writes) against the live machine, iterated until
every one of ~30 emitted sections is clean (no `COLLECTION ERROR`, no empty section, honest
`ADMIN-REQUIRED` gaps only), and confirmed to surface real signals (a portproxy pivot, a
service install, CodeIntegrity unsigned-load events) with false positives suppressed. The
`setup-security-audit.ps1` end-to-end task registration (AC1–AC5) still requires a Windows
host at flag-flip time; the collector output contract itself is now machine-verified.

Requires a Windows host (the enabling machine, at flag-flip time):

- `setup-security-audit.ps1` end-to-end: register → first report → seed → `-Status`.
- One unattended Claude `weekly-security-audit` run reading the report (spec AC5).

## Rollback

`setup-security-audit.ps1 -Uninstall`, `gff unset install.windows.security-audit`
(or `gff set … false`). Data/baseline folders are kept and documented.
