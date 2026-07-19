# MBO Index — objective tracker

The source of truth for every design objective: its artifacts, linked issue(s) and PR(s), and
lifecycle state. **Update this whenever an artifact is added or a state changes.**

State lifecycle: `idea → designing → specifying → planning → building → in-review → merged →
done` (also `parked`, `superseded`).

## TODO

- **Move state tracking out of this hand-maintained table → [#131](https://github.com/sfc-gh-eraigosa/dotfiles/issues/131).**
  The **catalog** here (objectives → design/spec/plan + issue/PR links) is worth keeping, but the
  `State` column and the **Active vs Merged/historical** split are painful to maintain by hand and
  drift from reality (a PR merges, the row still says `in-review`). The plan is to adopt
  [GitHub Project #4](https://github.com/users/sfc-gh-eraigosa/projects/4) as the live state
  source-of-truth, with skills to register objectives and keep `Status` in sync — leaving this file
  as the catalog (hand-edited or generated). See #131 for the full problem statement and scope.

## Active

| Slug | Design | Spec | Plan | Issue(s) | PR(s) | State |
| :-- | :-- | :-- | :-- | :-- | :-- | :-- |
| `prping` | — | [spec](./specs/2026-06-05-prping-design.md) | [plan](./plans/2026-06-05-prping-implementation-plan.md) | — | #127 | in-review |
| `agy-gsl-integration` | — | [spec](./specs/agy-gsl-integration.md) | [plan](./plans/agy-gsl-integration.md) | — | (this PR) | building |
| `mbo` | this folder + the `mbo-plan` skill (`ai/skills/mbo-plan/`) | — | — | — | #127 | in-review |
| `worker-md-placement` | [design](./designs/worker-md-placement.md) | [spec](./specs/worker-md-placement.md) | [plan](./plans/worker-md-placement.md) | [#132](https://github.com/sfc-gh-eraigosa/dotfiles/issues/132) | #133 | merged |
| `memory-provisioning` | [design](./designs/memory-provisioning.md) | [spec](./specs/memory-provisioning.md) | [plan](./plans/memory-provisioning.md) | [#134](https://github.com/sfc-gh-eraigosa/dotfiles/issues/134) | #135 | in-review |
| `shell-portability` | — | [spec](./specs/shell-portability.md) | — | — | (this PR) | building |
| `sshd-setup` | — | [spec](./specs/sshd-setup.md) | [plan](./plans/sshd-setup.md) | [#169](https://github.com/sfc-gh-eraigosa/dotfiles/issues/169) | [#170](https://github.com/sfc-gh-eraigosa/dotfiles/pull/170) | building |
| `gsl-visual-improvements` | [design](./designs/gsl-visual-improvements.md) | [spec](./specs/gsl-visual-improvements.md) | [plan](./plans/gsl-visual-improvements.md) | [#54](https://github.com/sfc-gh-eraigosa/dotfiles/issues/54) | #55 | in-review |
| `claude-config` | — | [spec](./specs/claude-config.md) | [plan](./plans/claude-config.md) | — | #126 | in-review |

## Merged / historical (pre-MBO — tracking columns best-effort)

| Slug | Design | Spec | Plan | Issue(s) | PR(s) | State |
| :-- | :-- | :-- | :-- | :-- | :-- | :-- |
| `sdk-migration` | — | — | [plan](./plans/2026-06-04-sdk-migration-plan.md) | #114 | #119–#123 | merged |
| `ai-config-home-provisioning` | [design](./designs/2026-06-02-ai-config-home-provisioning.md) | — | — | — | #111, #113 | merged |
| `tmux-mgr-remote-command` | [design](./designs/2026-05-30-tmux-mgr-remote-command.md) | — | — | #63 | — | merged |
| `gws-auth` | [design](./designs/gws-auth-design.md) | — | — | — | — | merged |
| `wispr-flow-hover-dictate` | — | [spec](./specs/2026-05-30-wispr-flow-hover-dictate-design.md) | [plan](./plans/2026-05-30-wispr-flow-hover-dictate.md) | — | #70 | merged |
| `flow-calibration-mode` | — | [spec](./specs/2026-05-31-flow-calibration-mode-design.md) | [plan](./plans/2026-05-31-flow-calibration-mode.md) | — | — | merged |
| `wispr-flow-config` | — | [spec](./specs/2026-05-31-wispr-flow-config-design.md) | [plan](./plans/2026-05-31-wispr-flow-config.md) | — | — | merged |
| `ai-teams-install` | — | [spec](./specs/2026-06-01-ai-teams-install-design.md) | [plan](./plans/2026-06-01-ai-teams-install-plan.md) | — | — | merged |
| `windows-setup-consolidated-elevation` | — | — | [plan](./plans/2026-06-01-windows-setup-consolidated-elevation.md) | #58, #59 | — | merged |
| `ai-plugin-manifest` | — | — | [plan](./plans/ai-plugin-manifest.md), [impl](./plans/ai-plugin-manifest-implementation.md) | — | — | merged |
| `gemini-extension-integration` | — | — | [plan](./plans/gemini-extension-integration.md) | — | — | merged |
| `gsl-status-line` | — | — | [plan](./plans/gsl-status-line.md), [exec](./plans/gsl-status-line-execution.md) | #33, #34 | — | merged |

> Legacy entries were migrated into MBO on 2026-06-06 from `docs/{designs,plans}` and
> `docs/superpowers/{specs,plans}`. Filenames are kept as-was; new objectives use the bare
> `<slug>` convention (see `AGENTS.md`).
