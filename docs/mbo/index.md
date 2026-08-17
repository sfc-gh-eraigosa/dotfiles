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
| `research-evaluation` | the `ai/skills/research-evaluation/` skill (rubric: value · setup/licensing · adversarial · security · stability · quality · docker-or-skip demo) | — | — | a private repo's issues #235–#240 (first six-target run) | (this PR) | in-review |
| `prping` | — | [spec](./specs/2026-06-05-prping-design.md) | [plan](./plans/2026-06-05-prping-implementation-plan.md) | — | #127 | in-review |
| `agy-gsl-integration` | — | [spec](./specs/agy-gsl-integration.md) | [plan](./plans/agy-gsl-integration.md) | — | (this PR) | building |
| `mbo` | this folder + the `mbo-plan` skill (`ai/skills/mbo-plan/`) | — | — | — | #127 | in-review |
| `worker-md-placement` | [design](./designs/worker-md-placement.md) | [spec](./specs/worker-md-placement.md) | [plan](./plans/worker-md-placement.md) | [#132](https://github.com/sfc-gh-eraigosa/dotfiles/issues/132) | #133 | merged |
| `memory-provisioning` | [design](./designs/memory-provisioning.md) | [spec](./specs/memory-provisioning.md) | [plan](./plans/memory-provisioning.md) | [#134](https://github.com/sfc-gh-eraigosa/dotfiles/issues/134) | #135 | in-review |
| `shell-portability` | — | [spec](./specs/shell-portability.md) | — | — | (this PR) | building |
| `sshd-setup` | — | [spec](./specs/sshd-setup.md) | [plan](./plans/sshd-setup.md) | [#169](https://github.com/sfc-gh-eraigosa/dotfiles/issues/169) | [#170](https://github.com/sfc-gh-eraigosa/dotfiles/pull/170) | building |
| `gsl-visual-improvements` | [design](./designs/gsl-visual-improvements.md) | [spec](./specs/gsl-visual-improvements.md) | [plan](./plans/gsl-visual-improvements.md) | [#54](https://github.com/sfc-gh-eraigosa/dotfiles/issues/54) | #55 | merged |
| `gsl-ultra` | [design](./designs/gsl-ultra.md) | [spec](./specs/gsl-ultra.md) | [plan](./plans/gsl-ultra.md) | [#158](https://github.com/sfc-gh-eraigosa/dotfiles/issues/158) (absorbs [#31](https://github.com/sfc-gh-eraigosa/dotfiles/issues/31)) | [#196](https://github.com/sfc-gh-eraigosa/dotfiles/pull/196) (consolidates #159 #161 #165 #166 #167) | in-review (4/7 leaves in #196; mcp·style·tui not started — plan §6.2) |
| `claude-config` | — | [spec](./specs/claude-config.md) | [plan](./plans/claude-config.md) | — | #126 | in-review |
| `gff` | [design](./designs/gff.md) | [spec](./specs/gff.md) | [plan](./plans/gff.md) | [#180](https://github.com/sfc-gh-eraigosa/dotfiles/issues/180) | [#181](https://github.com/sfc-gh-eraigosa/dotfiles/pull/181) · p1-engine [#182](https://github.com/sfc-gh-eraigosa/dotfiles/pull/182) · p2 [#184](https://github.com/sfc-gh-eraigosa/dotfiles/pull/184) · p3+p4 [#187](https://github.com/sfc-gh-eraigosa/dotfiles/pull/187) · vd-demo [#188](https://github.com/sfc-gh-eraigosa/dotfiles/pull/188) · WSLENV fix [#191](https://github.com/sfc-gh-eraigosa/dotfiles/pull/191) | done (all leaves merged; P2-T5 real-install evidence captured 2026-07-26; next-iteration backlog in plans/gff/IMPLEMENTATION.md §8.2) |
| `fleet` | [design](./designs/fleet.md) | [spec](./specs/fleet.md) | [plan](./plans/fleet.md) + [trio](./plans/fleet/) | [#222](https://github.com/sfc-gh-eraigosa/dotfiles/issues/222) | design [#223](https://github.com/sfc-gh-eraigosa/dotfiles/pull/223) (merged) · build [#224](https://github.com/sfc-gh-eraigosa/dotfiles/pull/224) | building (13/15 tasks; live prune + live update + ssh-key-sync retirement outstanding) |
| `gff-install-flow` | — | [spec](./specs/gff-install-flow.md) | [plan](./plans/gff-install-flow.md) | — | planning [#193](https://github.com/sfc-gh-eraigosa/dotfiles/pull/193) (merged) · build [#194](https://github.com/sfc-gh-eraigosa/dotfiles/pull/194) (ready) | in-review (all 6 tasks done; 4-run owner matrix green incl. the elevated-log wispr SKIP; awaiting merge) |
| `security-audit` | — | [spec](./specs/security-audit.md) | [plan](./plans/security-audit.md) | — | [#225](https://github.com/sfc-gh-eraigosa/dotfiles/pull/225) | merged (collector v2: ~40 ATT&CK lenses, adversarially reviewed; Windows-host AC1–AC5 pending at flag-flip) |
| `security-hardening` | — | [spec](./specs/security-hardening.md) | [plan](./plans/security-hardening.md) | — | (this PR) | building |

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
