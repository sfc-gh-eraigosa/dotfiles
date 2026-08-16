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

## Validation

Run in the build sandbox (Linux — no Windows/PowerShell available):

- `bash -n` + dash `sh -n` on every touched shell file — pass.
- `opt/scripts/system/shell-portability-scan.sh --strict` (whole repo) — Tier 1: 0, Tier 2: 0.
- `.github/gff/features.yaml` YAML parse + flag-shape check — pass.
- markdownlint on added/changed markdown — run locally/CI (`make lint-markdown`);
  shellcheck via `make lint-shell` in CI (neither tool was available in the sandbox).

Requires a Windows host (the enabling machine, at flag-flip time):

- `setup-security-audit.ps1` end-to-end: register → first report → seed → `-Status`.
- One unattended Claude `weekly-security-audit` run reading the report (spec AC5).

## Rollback

`setup-security-audit.ps1 -Uninstall`, `gff unset install.windows.security-audit`
(or `gff set … false`). Data/baseline folders are kept and documented.
