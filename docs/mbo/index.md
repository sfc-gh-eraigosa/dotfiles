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
| `sdk-tui` | [design](./designs/sdk-tui.md) + guide `sdk/libs/tui/GUIDE.md` | [spec](./specs/sdk-tui.md) | [plan](./plans/sdk-tui.md) + [trio](./plans/sdk-tui/) | [#283](https://github.com/sfc-gh-eraigosa/dotfiles/issues/283) | design [#286](https://github.com/sfc-gh-eraigosa/dotfiles/pull/286) (draft) · lib [#288](https://github.com/sfc-gh-eraigosa/dotfiles/pull/288) (stacked on #286; draft — owner promotes it, token-gated) | in-review (shared TUI behaviors lib `sdk/libs/tui`: keymap · nav · prompt · search · cmdline · overlay; **blocks `gff-tui-vim`**; phase 3 = fleet port + gsl config studio) |
| `herdr` | [design](./designs/herdr.md) (research dossier: value · setup/licensing · adversarial · security · stability · quality · demo · borrowable · business) | — | — | [#260](https://github.com/sfc-gh-eraigosa/dotfiles/issues/260) | (this PR) | building (verdict **adopt selectively**: pinned-or-latest checksummed installer + gff flags + agent integrations; tmux-mgr stays the orchestration layer) |
| `spark-gpu-support` | [design](./designs/spark-gpu-support.md) | — | — | — | (this PR) | designing (3 fixes landed + verified on the DGX Spark: PATH clobber, zsh profile.d gap, GPU provisioning; spec/plan pending) |
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
| `fleet` | [design](./designs/fleet.md) | [spec](./specs/fleet.md) | [plan](./plans/fleet.md) + [trio](./plans/fleet/) | [#222](https://github.com/sfc-gh-eraigosa/dotfiles/issues/222) | design [#223](https://github.com/sfc-gh-eraigosa/dotfiles/pull/223) (merged) · build [#224](https://github.com/sfc-gh-eraigosa/dotfiles/pull/224) (merged) | merged (13/15 at merge; live follow-ups = plan tasks 11/14/15: declined-prune capture · live update of the stale host · ssh-key-sync retirement) |
| `wlink` | [design](./designs/wlink.md) | [spec](./specs/wlink.md) | [plan](./plans/wlink.md) + [trio](./plans/wlink/) | [#245](https://github.com/sfc-gh-eraigosa/dotfiles/issues/245) | [#242](https://github.com/sfc-gh-eraigosa/dotfiles/pull/242) — design docs **and** the build | in-review (built, reviewed — 12 findings fixed; CI green. **P14 partial:** write round trip proven against a temp root, 4 items unproven in plans/wlink/TRACKING.md §6) |
| `fleet-config` | [design](./designs/fleet-config.md) | [spec](./specs/fleet-config.md) | [plan](./plans/fleet-config.md) + [trio](./plans/fleet-config/) | — (conversation-driven; no design issue was filed) | build [#254](https://github.com/sfc-gh-eraigosa/dotfiles/pull/254) | in-review (12/12 tasks, 4/4 gates evidenced by live SSH runs — see [TRACKING](./plans/fleet-config/TRACKING.md) for the same-machine caveat; two strictly one-way verbs, deliberately no bidirectional `sync`) |
| `fleet-tui` | [design](./designs/fleet-tui.md) | [spec](./specs/fleet-tui.md) | [plan](./plans/fleet-tui.md) + [trio](./plans/fleet-tui/) | [#226](https://github.com/sfc-gh-eraigosa/dotfiles/issues/226) | design [#227](https://github.com/sfc-gh-eraigosa/dotfiles/pull/227) | planning |
| `fleet-update` | [design](./designs/fleet-update.md) | [spec](./specs/fleet-update.md) | [plan](./plans/fleet-update.md) + [trio](./plans/fleet-update/) | [#265](https://github.com/sfc-gh-eraigosa/dotfiles/issues/265) | design [#270](https://github.com/sfc-gh-eraigosa/dotfiles/pull/270) (draft) | in-review (leaves A `updplan` / B `updexec` / C `featflag` / D `cmd` built + code-reviewed; E TUI and F docs landing; live gates G1, G2-wire, G4 evidenced, **G2-live/G3/G5–G9 pending on the operator** — every fleet host is `behind`, so no host was safe for a mutating run; follow-up: the gff SDK link adds +5.56 MB — swap `featflag/gff.go` for a `gff get` shell-out behind the same `Source`. Config-driven multi-repo update DAG for `fleet update`: `~/.config/fleet/fleet.yaml` or the tracked `opt/etc/fleet/fleet.yaml`, `sync`/`run`/`gh-auth` steps, per-repo `local: skip\|rescue\|carry` + branch restore, retries/backoff/timeouts, gff `fleet.update.{enabled,config}`) |
| `gff-install-flow` | — | [spec](./specs/gff-install-flow.md) | [plan](./plans/gff-install-flow.md) | — | planning [#193](https://github.com/sfc-gh-eraigosa/dotfiles/pull/193) (merged) · build [#194](https://github.com/sfc-gh-eraigosa/dotfiles/pull/194) (ready) | in-review (all 6 tasks done; 4-run owner matrix green incl. the elevated-log wispr SKIP; awaiting merge) |
| `security-audit` | — | [spec](./specs/security-audit.md) | [plan](./plans/security-audit.md) | — | v2 [#225](https://github.com/sfc-gh-eraigosa/dotfiles/pull/225) (merged) · cadence+verification (this PR) | building (collector v3: evening hourly window + once-per-day gate, daily urgent triage, `-Status` version/liveness proof; Windows-host ACs pending at flag-flip) |
| `security-hardening` | — | [spec](./specs/security-hardening.md) | [plan](./plans/security-hardening.md) | — | [#228](https://github.com/sfc-gh-eraigosa/dotfiles/pull/228) | merged (Windows-host AC1–AC6 pending at flag-flip) |
| `agy-parity` | [design](./designs/agy-parity.md) | [spec](./specs/agy-parity.md) | [plan](./plans/agy-parity.md) + [trio](./plans/agy-parity/) | [#268](https://github.com/sfc-gh-eraigosa/dotfiles/issues/268) | design draft [#269](https://github.com/sfc-gh-eraigosa/dotfiles/pull/269) | in-review (built on #269, T1–T7 done: `agy-config` launch config, settings template, forced deny/ask, hooks.json merge, adapter credential-path ask, `dotfiles` plugin (commands + memory rules); 4 drivers + live-host evidence in plans/agy-parity/evidence; merged result 40/40 shell-test) |

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
