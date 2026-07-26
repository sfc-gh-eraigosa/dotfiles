# gff — one-shot implementation playbook

- **Slug:** gff
- **Date:** 2026-07-25
- **Status:** Ready to execute
- **Plan (source of truth):** [`../gff.md`](../gff.md) · spec [`../../specs/gff.md`](../../specs/gff.md) · design [`../../designs/gff.md`](../../designs/gff.md)
- **Objective anchors:** issue #180 · design PR #181 · `docs/mbo/index.md` row `gff`

> This file is the **procedure**. It does not restate the plan — it tells a fresh agent
> session how to execute the plan end to end, feature by feature, TDD step by TDD step,
> resumably. Every technical detail lives in the plan; every reference below is a plan
> section number. **The plan wins any conflict with this file.**

---

## 0. The three files and how they fit together

| File | Role | Written by |
| :-- | :-- | :-- |
| `IMPLEMENTATION.md` (this) | The procedure: preconditions, worker/DAG order, the per-task loop, gates, hard rules, kickoff prompt | Read-only during the run (except §7 corrections) |
| [`TRACKING.md`](./TRACKING.md) | The live **state ledger**: per-task status/commit/evidence, the §7.4 feature-proof matrix, the IH/IA scenario checklist, an append-only session log | Updated after **every** task |
| [`TODO.md`](./TODO.md) | The **cursor**: every plan task expanded into ordered, one-line TDD micro-steps | Checkboxes ticked as you go |

**Resumption rule:** the first unchecked box in `TODO.md` is the next thing to do. `TRACKING.md`
says what has been proven. Nothing else needs to be reconstructed.

---

## 1. Preconditions (verify before touching code)

1. **Design PR #181 is merged to `main`.** Confirm the plan is on main:
   `git -C "${HOME}/git/dotfiles" fetch origin && git -C "${HOME}/git/dotfiles" show origin/main:docs/mbo/plans/gff.md | head -5`
   If it is not merged, STOP — the leaf workers must branch from a main that already
   carries the plan and these tracking files.
2. **Toolchain present:**
   - Go pinned by `${HOME}/git/dotfiles/.go-version` (1.26.1) — `go version`
   - `git`, `gh` (authenticated), `gss` on `PATH` — `gss feature list`
   - `protoc` (**regeneration only**, P1-T2 / CI): `command -v protoc` — install via
     `apt-get install protobuf-compiler` / `brew install protobuf`. Contributors build
     from the committed `gen/` output, so a missing protoc blocks only P1-T2 and
     `make gff-proto-check`.
   - Optional: `pwsh` (P2-T4 check; falls back to the P2-T5 human run), a real WSL
     terminal (P2-T5, VD-1 — both are **human-evidenced**, not automatable).
3. **Feature `gff` exists in the gss registry** (it does — the `design` worker created it):
   `gss feature list | grep -A3 '^Feature: gff'`
4. **Read plan §3 in full** (frozen contracts: proto, file formats, Go interfaces, CLI
   contract, shell contract) before writing a single line. §3 is law — see §5 below.
5. **Read plan §6.1** (the leaf/DAG table) and §7 (the validation plan: §7.1 coverage
   bars, §7.2 IH-*/IA-* scenarios, §7.3 demo script, §7.4 proof matrix, §7.5 done-when).

---

## 2. Leaf → gss worker map, in DAG order

Per plan §6.1. **`p1-engine` is the base leaf and blocks everything else.** After it
merges, `p2-instrument`, `p3-tui`, and `p4-gen` are path-disjoint and run in parallel;
`vd-demo` additionally consumes `p2-instrument` (and, for its TUI addendum, `p3-tui`).

```
                  ┌──> p2-instrument ──┐
                  │                    │
   p1-engine ────>├──> p3-tui ─────────┼──> vd-demo
   (blocking)     │                    │
                  └──> p4-gen ─────────┘
```

| Order | Leaf | Tasks | Owns (paths — plan §6.1) | Starts after |
| :-- | :-- | :-- | :-- | :-- |
| 1 | `p1-engine` | P1-T1 … P1-T11 | `sdk/gff/**` (excl. `internal/tui`, `cmd/gen.go`), `.github/workflows/gff-ci.yml` | — |
| 2a | `p2-instrument` | P2-T1 … P2-T5 | `.github/gff/**`, `opt/lib/gff.sh*`, `install.sh`, `opt/bin/install_windows.sh`, `opt/Desktop/Apps/scripts/{lib/gff.ps1,setup-apps.ps1,setup-elevated.ps1}` | `p1-engine` merged |
| 2b | `p3-tui` | P3-T1 | `sdk/gff/internal/tui/**`, `sdk/gff/cmd/tui.go` | `p1-engine` merged |
| 2c | `p4-gen` | P4-T1 | `sdk/gff/cmd/gen.go`, `cmd/gen_test.go`, `cmd/testdata/gen.golden` | `p1-engine` merged |
| 3 | `vd-demo` | VD-1 | `sdk/gff/scripts/demo.sh` | `p1-engine` + `p2-instrument` merged |

### 2.1 Creating a worker

Run from `${HOME}/git/dotfiles`. One worker per leaf; `--purpose` is the leaf name.

```sh
gss feature worker add --feature gff --purpose p1-engine --engine claude --json \
  --description "P1 engine: proto schema, resolver, registry, core CLI, SDK, CI, e2e harness (#180)"

gss feature worker add --feature gff --purpose p2-instrument --engine claude --json \
  --description "P2: dotfiles flag inventory + gff_on shell gate + install.sh/PowerShell gating (#180)"

gss feature worker add --feature gff --purpose p3-tui --engine claude --json \
  --description "P3: bubbletea TUI — browse, provenance, toggle (#180)"

gss feature worker add --feature gff --purpose p4-gen --engine claude --json \
  --description "P4: gff gen typed-accessor codegen (#180)"

gss feature worker add --feature gff --purpose vd-demo --engine claude --json \
  --description "VD-1: narrated end-to-end demo script + recorded evidence (#180)"
```

**Capture `worker_ref`, `branch`, and `worktree_path` verbatim from the `--json`
output.** Never reconstruct them from a naming guess — gss may apply a suffix. Record
them in `TRACKING.md` next to the leaf. All work for a leaf happens **inside its
`worktree_path`**, never in `${HOME}/git/dotfiles`.

### 2.2 Landing a leaf and unblocking the next

1. `gss feature checkpoint` from inside the worktree (or `--worker <ref>` from
   elsewhere) — rebases, pushes, creates/updates the **draft** PR. **Not token-gated.**
2. When the leaf's done-when gate (§4) is green, promote:
   `gss feature pr --ready --worker <ref>` — **token-gated**, see §5.7.
3. After the PR merges: `gss feature merged --worker <ref>` (token-gated) re-targets
   children, then create the next leaves' workers off the updated `main`.

**Do not create `p2/p3/p4` before `p1-engine` merges** unless you deliberately stack
them with `--base <p1-engine branch>`; in that case you MUST run `gss feature merged`
on `p1-engine` afterwards so the children re-target. The unstacked path (land P1, then
branch the rest from `main`) is the recommended default — it is what plan §6.1's
"pairwise disjoint once `p1-engine` merges" describes.

---

## 3. The execution loop — run this for EVERY task

This is the whole job. One iteration per plan task (P1-T1 … VD-1). Do not batch tasks;
do not skip the FAIL observation.

1. **Locate.** Open `TODO.md`, find the first unchecked box. Its `### <TASK-ID>` heading
   names the plan task. Open the matching section of `../gff.md` and read it fully —
   including the code sketches, which are normative content, not illustration.
2. **RED — write the failing test first.** Author exactly the test(s) the plan's task
   names (test file, function, assertions). No production code yet.
3. **Run it and VERIFY IT FAILS.** Execute the exact command in the micro-step
   (`go test ./internal/schema/`, `bash opt/lib/gff_test.sh`, …). **Read the output.**
   A test that passes before implementation is not a test — fix the test, do not proceed.
   Record the failure mode in one line (you will paste it into `TRACKING.md` evidence).
4. **GREEN — implement.** Write the minimum production code that satisfies the frozen
   contract in plan §3. Do not invent API surface the plan did not specify.
5. **Run it and VERIFY IT PASSES.** Same command. Then the task's extra gates: coverage
   (`-cover`, see §4/§7.1 bars), `go vet ./...`, `make lint-shell && make lint-portability`
   for any shell edit, `git check-ignore -v <path>` for any new path.
6. **Update the ledgers** — both, in the same breath:
   - `TODO.md`: tick every micro-step you completed.
   - `TRACKING.md`: set the task row to `done`, paste the commit SHA (after step 7) and
     a one-line evidence string (the passing command + result, e.g.
     `go test ./internal/resolve/ -cover -> ok, 96.4%`). Tick any §7.4 matrix cells and
     IH-*/IA-* scenarios this task proved.
7. **Commit** with the plan's **exact** commit message for that task. Stage files by
   explicit name — never `git add -A` / `git add .`. Commit trailer:
   `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.
8. **Checkpoint.** `gss feature checkpoint` from inside the worktree. This pushes and
   refreshes the draft PR, so the run is resumable from the remote at any point.
9. **Next task.** Return to step 1.

**If a step fails and you cannot fix it:** mark the task `blocked` in `TRACKING.md` with
the failing command and its output, append a session-log line, checkpoint, and move to
the next *independent* task (a different leaf) if one exists. Never mark a task `done`
without evidence; never claim a gate is green without having run it.

---

## 4. Done-when gates, per leaf (copied from plan §6.1)

A leaf is finished — and its PR may be promoted to ready — only when its gate is green.

| Leaf | `done-when` gate | Blocking? |
| :-- | :-- | :-- |
| `p1-engine` | gff-ci green: vet + tests, **≥90% cover**, proto regen clean, **e2e harness green**; `build.sh` installs a working binary | **yes (base)** |
| `p2-instrument` | `make lint-shell` + `make lint-portability` clean; `gff lint` clean on the inventory; **P2-T5 human evidence** posted | no |
| `p3-tui` | teatest suite green; overall coverage still **≥90%** | no |
| `p4-gen` | golden test green; generated output vets | no |
| `vd-demo` | demo transcript + evidence posted on the PR (plan §7.3, VD-1) | no |

Per-task sub-gates (plan §7.1): `internal/resolve` **≥95%**, `internal/schema` **≥90%**,
`sdk/gff` overall **≥90%** (the repo-wide `sdk/` floor is 60% — gff deliberately exceeds it).

### 4.1 The overall stop condition (plan §7.5)

Stop the one-shot run only when **all** of these hold:

- `gff-ci.yml` fully green: vet, unit tests at **≥90%** coverage, the `e2e` job (every
  IH-1…IH-10 and IA-1…IA-12 subtest), proto-regen clean, `go run .` zero-install smoke.
- The VD-1 demo transcript **and** the P2-T5 real-install evidence are posted on the PR(s).
- Every row of the §7.4 feature→proof matrix is checked in `TRACKING.md` **and** in the
  leaf PR descriptions — a feature without all three proofs (unit / integration / demo)
  is not done.
- `docs/mbo/index.md` state updated per leaf; #180 closed only when all four leaves land.

---

## 5. Hard rules (non-negotiable)

1. **Plan §3 is frozen law.** The proto schema (§3.1), file formats (§3.2), Go
   interfaces (§3.3), CLI contract incl. exit codes (§3.4), and shell contract (§3.5)
   are contracts other leaves compile against. If implementation reveals a genuine
   defect in a frozen contract, **stop, record it in `TRACKING.md`, and escalate** — do
   not silently change it. Changing §3 invalidates work in parallel leaves.
2. **Coverage bar is 90%** for `sdk/gff` overall (95% for `internal/resolve`, 90% for
   `internal/schema`), enforced in `gff-ci.yml`. Coverage is a floor, not a target to
   game — the e2e harness covers the thin `cmd/` paths the unit bar cannot.
3. **No buf. Ever.** Codegen is raw `protoc` + a go.mod-pinned `protoc-gen-go` behind
   `make gff-proto` (plan §4 P1-T2). Generated Go under `sdk/gff/gen/gff/v1/` is
   **committed**; `make gff-proto-check` must show a clean `git diff`.
4. **Shell portability lint before every shell commit.** Any `.sh` / sourced fragment /
   `install.sh` edit: run `make lint-shell && make lint-portability` and require clean.
   `opt/lib/gff.sh` must be **POSIX/dash-safe** (no `[[`, no arrays) and pass under both
   `bash` and `sh`. All shell gates **fail open** — a missing/broken gff runs every step.
5. **`.gitignore` allowlist verification for every new path.** The repo's `.gitignore`
   starts with `*`; a new file is invisible until an `!`-rule covers it. Before staging
   any newly created path: `git status --short -- <path>` and `git check-ignore -v <path>`.
   Expected: `!sdk/**` covers `sdk/gff/**`, `!.github/**` covers `.github/gff/`,
   `!opt/**` covers `opt/lib/gff.sh`, `!docs/**` covers these tracking files. If a path
   is ignored, add a **narrow** `!`-rule with an explanatory comment — never `git add -f`.
   P1-T2 deliberately adds an **ignore** rule for `sdk/gff/.bin/` (the protoc plugin
   binary) with a comment.
6. **Writes go only to `~/.config/gff/`.** gff never writes repo or system files.
   No test may write outside `t.TempDir()`; no test may touch the network.
7. **The two-call gss token recipe.** `gss feature pr --ready`, `gss feature merged`,
   and `gss feature restack` mutate remote state and are token-gated. Issue the token as
   its own Bash call, then the gss command as a **separate** call:
   - call 1: `mkdir -p ~/.config/gss && git rev-parse HEAD > ~/.config/gss/approval.token`
   - call 2: `gss feature pr --ready --worker <ref>`

   Chaining them with `&&` is blocked by `safety_guard.sh` by design. The token is
   HEAD-bound — commit **before** generating it, or it goes stale.
   `gss feature checkpoint` is **not** token-gated; use it freely after every task.
8. **Confirmation before publishing.** Any `git add` / `git commit` / `gss push` /
   `gss pr` requires the user's explicit confirmation via the interactive question tool,
   per the repo's Git Workflow rule. In an approved autonomous one-shot run, that
   approval is granted once for the whole run by the user's kickoff — but promoting a
   draft PR to **ready** (`--ready`) and `gss feature merged` always re-confirm.
9. **Docs discoverability.** New documented directories get `AGENTS.md` plus a
   `CLAUDE.md -> AGENTS.md` symlink (`sdk/gff/` in P1-T1). Add the `sdk/gff` line to the
   root `CLAUDE.md` Repository Structure (plan §6).
10. **Privacy.** Never write absolute home paths, real usernames, or hostnames into
    tracked files, commit messages, or PR bodies — use `${HOME}` / `~` / `${USER}`.
    The `privacy_guard` hook enforces this and will block violations.
11. **Never run `install.sh` from a worker worktree.** It creates absolute symlinks in
    `${HOME}`. P2-T5's human-evidenced run happens from `${HOME}/git/dotfiles` on the
    merged branch, in a real interactive terminal (it prompts; a backgrounded or piped
    run silently skips the Windows phase).
12. **Evidence before assertions.** Report failures and skipped steps faithfully. Do not
    write "PASS" into `TRACKING.md` for a command you did not run.

---

## 6. Command cheat-sheet

All Go commands run from `<worktree>/sdk/gff/`; all `make` targets from the worktree root.

```sh
# unit tests + coverage (the bar)
go test ./... -coverprofile=cover.out && go tool cover -func=cover.out | tail -1
go test ./internal/resolve/ -cover      # >=95%
go test ./internal/schema/  -cover      # >=90%
go vet ./...

# proto (regeneration only; committed output must stay clean)
make gff-proto            # -> bash sdk/gff/scripts/genproto.sh
make gff-proto-check      # regen + git diff --exit-code -- sdk/gff/gen/

# binary-level e2e (P1-T11)
make gff-e2e              # builds gff to a temp dir, runs: go test -tags e2e ./e2e/

# build + install the binary
bash build.sh             # -> ${HOME}/opt/bin/gff
${HOME}/opt/bin/gff version

# zero-install entrypoint smoke (F11)
go run . version

# shell gates (mandatory before any shell commit)
make lint-shell && make lint-portability
bash opt/lib/gff_test.sh && sh opt/lib/gff_test.sh    # bash AND dash

# allowlist verification for a new path
git status --short -- <path> && git check-ignore -v <path>

# worker lifecycle
gss feature checkpoint                                  # from inside the worktree
mkdir -p ~/.config/gss && git rev-parse HEAD > ~/.config/gss/approval.token   # call 1
gss feature pr --ready --worker gff/<user>/<purpose>     # call 2 (token-gated)
```

---

## 7. Resuming, recovery, and corrections

- **Resume:** read `TRACKING.md` (what is proven) then `TODO.md` (first unchecked box).
  Re-run that task's last verification command to confirm the ledger matches reality
  before continuing — the ledger is a claim, the command is the proof.
- **Ledger drift:** if `TRACKING.md` says `done` but the test fails, trust the test.
  Reset the row to `in-progress`, note the drift in the session log, and redo the task.
- **Rebase / conflicts:** `gss feature conflicts` reports cross-worker overlap. If a
  checkpoint or rebase aborts on conflict, use the `git-machete` skill to restack; gss
  remains the single writer for PRs and the registry.
- **A frozen contract looks wrong:** stop, write the finding into the `TRACKING.md`
  session log with the mechanism (not speculation), and escalate to the objective owner.
  Do not unilaterally edit plan §3.
- **Correcting this file:** IMPLEMENTATION.md is procedure, not contract — fix a wrong
  command here freely and note it in the session log. Never edit `../gff.md`,
  `../../specs/gff.md`, or `../../designs/gff.md` as part of the build.

### 7.1 Run-2 procedure corrections (learned 2026-07-26, binding for future runs)

1. **`gss feature merged` takes the worker ref POSITIONALLY** — there is no
   `--worker` flag on that subcommand (unlike `checkpoint`/`pr`).
2. **Verify every new worker's base against `origin/main`.** `worker add` can
   branch from a stale local `main`; before any work: `git -C <wt> log --oneline -1`
   must equal `origin/main`'s tip, else `git fetch origin main:main` and reset the
   fresh (zero-commit) branch onto it.
3. **`gss feature checkpoint` regenerates the PR body every run** — re-apply any
   custom body (proof matrices, evidence links) AFTER the final checkpoint, via
   `gh pr edit --body-file`, preserving the `gss:stack` markers.
4. **`gss feature pr --ready` can be double-bound**: `safety_guard.sh` validates
   the approval token against the SESSION cwd's HEAD while gss validates it
   against the WORKER branch HEAD — unsatisfiable when they differ. Fallback:
   `gh pr ready <n>` (same outcome; gss reads PR state live).
5. **Owner-directed leaf restructuring is normal.** p3+p4 landed as one combined
   `p34-tui-gen` PR (#187) on owner request; the original leaf PRs were closed as
   superseded and their TRACKING rows annotated — the DAG in the plan stays the
   design truth, the registry/PRs reflect the runtime shape.
6. **Ledger single-writer:** exactly one branch carries TODO/TRACKING/index edits
   at a time (p2 → vd-demo this run) so parallel leaf PRs never conflict on docs.
7. **Registry rows may be pruned when a feature's PRs merge** — surviving
   branches/worktrees continue via plain `git` (plain pushes are not token-gated)
   + `gh` for PR state.
8. **Real-binary demos catch what in-process tests cannot** (the KeySpace bug,
   the tmux OSC-11 theme bug, the overlap-namespace bug all surfaced only in
   tmux-driven runs of the compiled binary). Budget a live tmux validation pass
   per TUI-facing change; capture frames into the evidence tree.

---

## 8. Kickoff prompt (always CURRENT — history lives in git)

> **Maintenance rule:** this section holds exactly ONE prompt — the one that
> starts the NEXT work session. When a session ends, replace it with the next
> iteration's prompt (past prompts are in `git log -- <this file>`; never
> accumulate them here). Build each prompt from the template below.

### 8.1 The prompt template (reuse for every gff session)

```
**<One-line mission for this session.>**

Read first, completely: docs/mbo/plans/gff/IMPLEMENTATION.md (procedure; §5 hard
rules; §7.1 run corrections), docs/mbo/plans/gff/TRACKING.md (what is proven;
§8 stop condition; §10 blockers), docs/mbo/plans/gff/TODO.md (the cursor — first
unchecked box), then the plan sections this session touches: <§refs>.

Orchestration: you are the orchestrator — delegate implementation to subagents
when parallelism helps, but verify every gate YOURSELF before committing
(evidence before assertions), keep git/gss in your own hands, stage by explicit
name, one commit per task with the plan's message, checkpoint/push after every
task, ledger single-writer on <branch>. TDD strictly: RED observed before GREEN.
Validate TUI/UX work against the REAL binary in tmux and commit the transcripts
into docs/mbo/plans/gff/evidence/.

Scope for this session: <ordered task list>.

Human-in-the-loop stops (never fake): <list>. ASK before any PR promotion or
merge. Blocked → TRACKING §10 with the real failing output, move to the next
independent task. Done when: <this session's done-when>.
```

### 8.2 CURRENT prompt — install.sh flag-flow refactor (owner-proposed 2026-07-26)

> **Plan and build the install.sh flag-flow refactor — prompt-early /
> Windows-last + early export — then drain the gff backlog.**
>
> Read first, completely: `docs/mbo/plans/gff/IMPLEMENTATION.md` (§5 hard
> rules, §7.1 run corrections), `docs/mbo/plans/gff/TRACKING.md` (§8 — should
> be fully green; §10 rows 3–5 — the open observations this session inherits),
> then plan `../gff.md` §3.5 (the frozen shell contract) and §4 P2-T3/P2-T4
> (the gating sites being refactored).
>
> State on entry: the gff objective is CLOSED — #180 closed, all leaves merged
> (#182/#184/#187/#188), P2-T5 evidence posted, `docs/mbo/index.md` says
> `done`. This session opens a NEW small objective (mbo-plan it: spec + plan,
> design optional) building on gff. Work rides a `gss feature` worker or a
> plain branch + `gh pr` at the owner's choice.
>
> Orchestration: you are the orchestrator — verify every gate YOURSELF before
> committing (evidence before assertions), stage by explicit name, one commit
> per task, `make lint-shell && make lint-portability` before every shell
> commit, validate interactive flows in a REAL terminal (never backgrounded /
> piped — the prompt is silently skipped), commit transcripts into
> `docs/mbo/plans/gff/evidence/`.
>
> Scope for this session (the lead item is owner-proposed, 2026-07-26):
> 1. **install.sh: prompt-early / Windows-last + early flag export.**
>    - Move the Windows phase EXECUTION (the `install.windows.desktop-deploy`
>      gate + `install_windows.sh` call, today ~line 67) to the END of
>      install.sh — after the gff bootstrap eval (~line 355–366) — so `GFF_*`
>      is materialized in-script and `install.windows.*` overrides work with
>      zero calling-shell steps, INCLUDING on a fresh system (requirement
>      chain: goenv → Go 1.26.1 → `sdk/gff/build.sh` → bootstrap eval, all
>      satisfied by then).
>    - Keep interactivity front-loaded: capture the y/n/s answer EARLY
>      (extend install_windows.sh's sentinel mechanism: `--ask` records the
>      choice up front, `--deferred` executes it at the end without
>      re-prompting). The UAC prompt then fires at the END of the run — it
>      MUST fail loud if it times out unattended (clear message + rerun hint),
>      never silently skip.
>    - Add a top-of-script EARLY EXPORT: if `command -v gff` succeeds,
>      `set -a; eval "$(gff export --shell …)"; set +a` before the first gate,
>      fail-open — so the pre-bootstrap gates (`install.system/shell/pkg/
>      tools.*`, today lines ~67–297) honor overrides from run 2 onward. The
>      mid-script bootstrap stays as the authoritative refresh.
>    - **gff-controlled permanent skip (owner note):** replace the "[s] skip
>      and never ask again" sentinel FILE with a gff user-override —
>      `gff set install.windows.desktop-deploy false` — so gff becomes the
>      single source of skip state (visible in `gff list`/TUI, reversible via
>      `unset`). Honor + migrate the legacy sentinel file if present.
>    - Real-terminal WSL validation of all four paths (y / n / s / flag-off)
>      + evidence transcript.
> 2. **UAC env-propagation follow-up** — resolve TRACKING §10 row 5 per what
>    the P2-T5 run actually showed (var crossed ⇒ close the observation; var
>    dropped ⇒ design the pass-through fix, e.g. args instead of env across
>    `Start-Process -Verb RunAs`).
> 3. **Wire the three unenforced `install.windows.*` flags**
>    (`claude-rc-autostart`, `sshd`, `portproxy`) ONLY if their standalone
>    scripts gain invocation sites (TRACKING §10 row 4); otherwise leave
>    declared-but-unenforced and documented.
> 4. **Multi-source install.sh story** (`gff sources`-driven) — design sketch
>    only, no build.
> 5. Optional: TUI gif/asciinema for `sdk/gff/README.md`.
>
> Human-in-the-loop stops (never fake): every real-terminal install.sh
> validation run, every merge. ASK before any PR promotion or merge. Blocked →
> TRACKING §10 with the real failing output. Done when: item 1 is merged with
> real-terminal evidence and items 2–3 are either resolved or explicitly
> re-documented as deferred.
>
> Human-in-the-loop stops (never fake): the P2-T5 run itself, every merge,
> every worktree deletion. Blocked → TRACKING §10 with the real failing output.
> Done when: TRACKING §8 is fully green, #180 is closed, index.md says done.

> Companion to plan `../gff.md`. Update `../../index.md` state as each leaf moves
> (`planning → building → in-review → merged`).
