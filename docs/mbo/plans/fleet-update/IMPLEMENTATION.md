# fleet-update — implementation playbook

- **Slug:** fleet-update
- **Date:** 2026-09-02
- **Status:** Ready to execute
- **Plan (source of truth):** [`../fleet-update.md`](../fleet-update.md) · spec [`../../specs/fleet-update.md`](../../specs/fleet-update.md) · design [`../../designs/fleet-update.md`](../../designs/fleet-update.md)
- **Objective anchors:** issue [#265](https://github.com/sfc-gh-eraigosa/dotfiles/issues/265) · PR [#270](https://github.com/sfc-gh-eraigosa/dotfiles/pull/270) · `docs/mbo/index.md` row `fleet-update`

> This file is the **procedure**. It does not restate the plan — it tells a fresh agent
> session how to execute the plan, task by task, resumably. Every technical detail lives in
> the plan; every `§` reference below points there. **The plan wins any conflict.**

## 0. The three files

| File | Role | Written by |
| :-- | :-- | :-- |
| `IMPLEMENTATION.md` (this) | Procedure: preconditions, worker map, per-task loop, hard rules, kickoff prompt | Read-only during the run (except §7 corrections) |
| [`TRACKING.md`](./TRACKING.md) | Live **state ledger**: per-task status/commit/evidence, proof matrix, blockers, append-only session log | Updated after **every** task |
| [`TODO.md`](./TODO.md) | The **cursor**: plan tasks 1–27 expanded into ordered micro-steps | Checkboxes ticked as you go |

**Resumption rule:** the first unchecked box in `TODO.md` is the next action; `TRACKING.md`
says what has been proven. Re-run the last verification command before continuing —
the ledger is a claim, the command is the proof.

**Paths:** `<WT>` = the leaf's worktree path captured from `gss feature worker add --json`.
Go commands run from `<WT>/sdk/fleet/`; `gff lint` from `<WT>/sdk/gff/`; `gss` from inside `<WT>`.
Never work in `${HOME}/git/dotfiles` except where a step says so explicitly (live gates).

## 1. Preconditions

Verify each one before touching code. Every item has its command; paste the observed result
into the TRACKING §5 session log for the session that ran it.

| # | Precondition | Verify command | Expected |
| :-- | :-- | :-- | :-- |
| 1 | Design/plan PR is merged — the trio and plan are on `origin/main` (workers must branch from a main that carries them) | `git -C "${HOME}/git/dotfiles" fetch origin && git -C "${HOME}/git/dotfiles" show origin/main:docs/mbo/plans/fleet-update.md \| head -5 && git -C "${HOME}/git/dotfiles" show origin/main:docs/mbo/plans/fleet-update/TODO.md \| head -3` | both print their title lines; otherwise **STOP** — do not create workers |
| 2 | Go matches the repo pin (`.go-version` = 1.26.1) | `cd "${HOME}/git/dotfiles" && test "go$(cat .go-version)" = "$(go version \| awk '{print $3}')" && echo GO-OK` | `GO-OK` |
| 3 | git present (hosts need ≥ 2.23 for `stash push -u`, `symbolic-ref --short`, `merge-base --is-ancestor`; check the controller here, the hosts at the live gates) | `git --version` | a version line |
| 4 | `gh` authenticated (needed by `gss feature checkpoint`/`pr`, and by the gh-auth live gates) | `gh auth status` | `Logged in to github.com` |
| 5 | `gss` on `PATH` and the `fleet-update` feature registered | `gss feature list \| grep -A3 'fleet-update'` | the feature row. If absent: `cd "${HOME}/git/dotfiles" && gss feature start fleet-update --base main --description "config-driven multi-repo update DAG for fleet update (#TBD)"` then re-run |
| 6 | Baseline suite green in the main checkout (records the `cmd` pre-change coverage figure the plan requires — hard rule §5.2) | `cd "${HOME}/git/dotfiles/sdk/fleet" && gofmt -l . && go vet ./... && go test -race -cover ./... 2>&1 \| tee "${HOME}/git/dotfiles/docs/mbo/plans/fleet-update/evidence/cmd/baseline-before.txt"` | all `ok`; note the `cmd` percentage (60.7 % at planning time) in TRACKING §1 task 19 Notes |
| 7 | gff feature file lints clean before we add the `fleet` area | `cd "${HOME}/git/dotfiles/sdk/gff" && go run . lint ../../.github/gff/features.yaml` | no findings |
| 8 | Evidence tree exists and is tracked (the `.gitignore` allowlist opts `docs/**` in) | `git -C "${HOME}/git/dotfiles" ls-files docs/mbo/plans/fleet-update/evidence \| wc -l` | `8` (one `.gitkeep` per folder) |
| 9 | A reachable live host for the leaf D/E gates (human) — one you can afford to put on a feature branch and back | `fleet status <host>` | the host row is reachable |
| 10 | Read plan §3 in full (frozen contracts: §3.2 `updplan`, §3.3 `updexec` + RunHost algorithm, §3.4 exact remote strings, §3.5 `featflag` + gff yaml, §3.6 CLI), §4 (tasks 1–27 with test names and commit messages), §6.1 (leaf DAG), §7 (evidence) | — | — |

## 2. Worker map

Per plan §6.1. **Leaf A is the base and blocks B; B and C block D; D blocks E and F.**
C is independent of A/B and may run in parallel with them.

```
A ──► B ──┐
          ├──► D ──► E
C ────────┘     └──► F
```

| Order | Leaf | Purpose (`--purpose`) | Tasks | Owns (paths — plan §6.1) | In-edge / `--base` | Blocking? |
| :-- | :-- | :-- | :-- | :-- | :-- | :-- |
| 1 | A `updplan` | `updplan` | 1–5 | `sdk/fleet/internal/updplan/**` (+ the `gopkg.in/yaml.v3` require in `go.mod`/`go.sum` — see note below) | `main` (feature default) | **yes** (B) |
| 1 (parallel) | C `featflag` + deps | `featflag` | 17–18 | `sdk/fleet/internal/featflag/**`, `sdk/fleet/go.mod`, `go.sum` (gff require + `replace`), `.github/gff/features.yaml` | `main` (feature default) | **yes** (D) |
| 2 | B `updexec` (+ runner ctx) | `updexec` | 6–16 | `sdk/fleet/internal/updexec/**`, `sdk/fleet/internal/runner/**` | `--base` = A's branch | **yes** (D, E) |
| 3 | D `cmd` CLI | `cmd` | 19–23 | `sdk/fleet/cmd/update*.go`, `answers_store.go`, `status.go` | `--base` = B's branch (C lands to `main` first — see reconcile) | **yes** (E, F) |
| 4a | E TUI | `tui` | 24–26 | `sdk/fleet/cmd/tui*.go`, `runlog_test.go` | `--base` = D's branch | no |
| 4b | F docs | `docs` | 27 | `sdk/fleet/{AGENTS,README}.md`, `opt/etc/fleet/**`, `docs/mbo/**fleet-update**`, `docs/mbo/index.md` | `--base` = D's branch | no |

**Reconcile note (C before D):** C branches from `main` and is expected to merge before D starts.
D is stacked on B (A → B → D). When C merges, run `gss feature checkpoint` on A and B (it
rebases onto the updated base) *before* creating D so D's base already contains `featflag`.
If D is created before C merges, D must add C's branch as a second source manually — avoid it.
`go.mod` is touched by both A (yaml.v3) and C (gff + `replace`); the merge is two independent
`require` lines and resolves trivially — record it in TRACKING §4 only if it does not.

**Ledger single-writer:** exactly one active branch carries `TODO.md` / `TRACKING.md` / `index.md`
edits at a time (the currently blocking leaf; then F). Parallel leaves (A‖C, E‖F) update the
ledgers via the single writer's branch, never both.

### 2.1 Registry (fill VERBATIM from `gss feature worker add --json`)

Never reconstruct `worker_ref` / `branch` / `worktree_path` from a naming guess — gss may apply
a suffix. Copy the JSON fields as printed, here **and** in TRACKING §0.

| Leaf | `worker_ref` | `branch` | `worktree_path` | `base_branch` |
| :-- | :-- | :-- | :-- | :-- |
| A `updplan` | | | | |
| B `updexec` | | | | |
| C `featflag` | | | | |
| D `cmd` | | | | |
| E `tui` | | | | |
| F `docs` | | | | |

### 2.2 Creating a worker

Run from `${HOME}/git/dotfiles` (the main checkout, on an up-to-date `main`). One worker per
leaf; `--purpose` is the leaf name; `<A-branch>` etc. are the `branch` values from §2.1.

```sh
gss feature worker add --feature fleet-update --purpose updplan  --engine claude --json --description "Leaf A: internal/updplan — plan types, parse, validation, defaults merge, backoff, order, --ref shim (#TBD)"
gss feature worker add --feature fleet-update --purpose featflag --engine claude --json --description "Leaf C: internal/featflag fail-open gff resolution + adapter, go.mod deps, fleet.update.* flags (#TBD)"
gss feature worker add --feature fleet-update --purpose updexec  --engine claude --json --base <A-branch> --description "Leaf B: internal/updexec script builders + executor, runner.RunStreamCtx (#TBD)"
gss feature worker add --feature fleet-update --purpose cmd      --engine claude --json --base <B-branch> --description "Leaf D: fleet update rewired onto updplan/updexec/featflag — loadPlan, report, init, run log (#TBD)"
gss feature worker add --feature fleet-update --purpose tui      --engine claude --json --base <D-branch> --description "Leaf E: TUI background lane + handoff via fleet update, --file, plan-aware status (#TBD)"
gss feature worker add --feature fleet-update --purpose docs     --engine claude --json --base <D-branch> --description "Leaf F: AGENTS/README fleet.yaml docs, sample plan, invariants, MBO index (#TBD)"
```

After every `worker add`: `git -C <WT> log --oneline -1` must equal the tip of its base
(`origin/main` for A and C) — `worker add` can branch from a stale local base.

### 2.3 Landing a leaf and unblocking the next

1. `gss feature checkpoint` from inside `<WT>` — rebases, pushes, creates/updates the **draft**
   PR. Not token-gated; run it after every task.
2. When the leaf gate (§4) is green and its evidence file is committed, promote:
   `gss feature pr --ready --worker <ref>` — token-gated (§5.9), **ask the user first**.
3. After the PR merges: `gss feature merged <ref>` (ref is **positional**; token-gated) so the
   children re-target. Then checkpoint the next leaf.
4. Update TRACKING §0 state (`todo → building → in-review → merged`) and `docs/mbo/index.md`.

## 3. The execution loop (every task)

One iteration per plan task (1 … 27). Do not batch tasks; do not skip the FAIL observation.

1. **Locate:** first unchecked `TODO.md` box → its `### Task N` heading → open plan §4 task N
   and read it fully, plus the §3 contract it implements (the Go signatures and the §3.4 shell
   strings are normative, not illustration).
2. **RED:** write exactly the failing test(s) the plan names — same function names, in the
   package the plan puts them in. No production code yet. Run the micro-step's `RUN-RED`
   command; **verify it fails**; record the failure line (it goes into TRACKING evidence).
   A test that passes before implementation is not a test — fix the test, do not proceed.
3. **GREEN:** implement the minimum that satisfies the frozen contract in plan §3. Run the
   `RUN-GREEN` command; verify it passes.
4. **Gates:** `cd <WT>/sdk/fleet && gofmt -l . && go vet ./... && go test -race -cover ./...`
   (every task, plan §4 preamble); the task's `VERIFY` boxes (per-package coverage, `gff lint`,
   `go build`); `git status --short -- <path>` + `git check-ignore -v <path>` for any new path.
5. **Evidence:** `tee -a` the done-when command's output to
   `docs/mbo/plans/fleet-update/evidence/<leaf>/task-NN.txt` with a dated header
   (`# YYYY-MM-DD task NN — <command>`), append-only; commit it with the task.
6. **Ledgers:** tick `TODO.md`; set the TRACKING §1 row (status, commit SHA after step 7,
   evidence string `command -> result`); tick any §2 matrix cells this task proved.
7. **Commit** with the plan's **exact** message, staging by explicit name (never `-A` / `.`).
   Moved tests are listed by name in the commit body. Trailer per the session's attribution
   instructions. **Confirm via `AskUserQuestion` first** (§5.8).
8. **Checkpoint:** `gss feature checkpoint` from inside `<WT>`. Return to step 1.

**If a step fails and you cannot fix it:** set the row `blocked` in TRACKING §1, add a §4
blockers row with the failing command and its **real** output, append a §5 session-log line,
checkpoint, and move to the next *independent* task (another leaf) if one exists. Never mark
`done` without evidence; never claim a gate green without running it.

## 4. Done-when gates

Copied from plan §4 leaf gates and §6.1. A leaf's PR may be promoted to ready only when its
gate is green **and** the gate output is committed under `evidence/<leaf>/`.

| Leaf | `done-when` gate | Evidence folder | Blocking? |
| :-- | :-- | :-- | :-- |
| A `updplan` | tasks 1–5 done; `go test -race -cover ./internal/updplan` **≥ 90 %** | `evidence/updplan/` | **yes** (base) |
| B `updexec` | tasks 6–16 done; `go test -race -cover ./internal/updexec ./internal/runner` — **updexec ≥ 90 %**; moved invariant tests (`TestUpdateMakesExactlyOneNetworkCall`, `TestRescuePreservesUntrackedWork`) green under their original names; mux invariant (`TestEveryRemotePathCarriesTheMuxOptions`) green | `evidence/updexec/` | **yes** (D, E) |
| C `featflag` | tasks 17–18 done; `go test -race -cover ./internal/featflag` **≥ 90 %**; `cd sdk/gff && go run . lint ../../.github/gff/features.yaml` clean; `go build ./...` | `evidence/featflag/` | **yes** (D) |
| D `cmd` | tasks 19–23 done; `go test -race ./...` green; `cmd` coverage not below the §1.6 baseline; `bash sdk/fleet/build.sh` installs; live `fleet update <host> --dry-run` (no file, then a two-repo plan) and one live host run captured | `evidence/cmd/`, `evidence/e2e/` | **yes** (E, F) |
| E `tui` | tasks 24–26 done; every `tui_*` test green; live TUI transcript captured | `evidence/tui/` | no |
| F `docs` | task 27 done; every relative link in the touched docs resolves; README console demo is real output | `evidence/docs/`, `evidence/demo/` | no |

### 4.1 The overall stop condition (spec §6 + plan §7)

All of: every leaf gate above green with evidence; `updplan`/`updexec`/`featflag` ≥ 90 % each,
`cmd` not below baseline, repo floor `fleet=60` in `scripts/test.sh` unchanged; `gff lint`
clean; the nine human-evidenced gates **G1–G9** (spec §6) captured under `evidence/e2e/`
(G5 under `evidence/tui/`); every TRACKING §2 matrix row ticked; `docs/mbo/index.md` state
`merged`; issue [#265](https://github.com/sfc-gh-eraigosa/dotfiles/issues/265) closed only when all six leaves land. The full tickable list is TRACKING §3.

## 5. Hard rules (non-negotiable)

1. **Plan §3 is frozen law.** §3.2 `updplan` types/functions, §3.3 `updexec` types + RunHost
   algorithm, §3.4 exact remote shell strings and the interpolation policy, §3.5 `featflag` +
   the gff yaml block, §3.6 CLI. Other leaves compile against them. A genuine defect → stop,
   record it in TRACKING §4, escalate; never patch silently.
2. **Coverage:** `updplan`, `updexec`, `featflag` **≥ 90 %** each (asserted in each leaf gate,
   recorded in evidence); `cmd` **not below its §1.6 baseline** (record before/after); the repo
   floor `fleet=60` in `scripts/test.sh` is not edited. Coverage is a floor, never gamed.
3. **No test deleted.** Moved tests (`TestUpdateMakesExactlyOneNetworkCall`,
   `TestRescuePreservesUntrackedWork`, the four migrated in task 12, the ten `cmd` tests in
   task 20) keep their names and assertions and are listed in the commit body. Leaf B cannot
   edit `cmd/`, so a "moved" test gains its `updexec` copy in B and loses its `cmd` copy only
   in D task 20 — both copies stay green in between.
4. **Every impure edge injected:** `runner.Runner`, `StepIO`, `Output`, `Now`, `Sleep`, `Rand`,
   `featflag.Source`, filesystem via `XDG_CONFIG_HOME` / `t.TempDir()`, `os.Executable` for the
   handoff. No `time.Now()`, `time.Sleep`, `math/rand` or `os.Getenv("HOME")` reached directly
   from code under test.
5. **Test hygiene:** no test opens a socket, reads `$HOME` (`t.Setenv("HOME", t.TempDir())`),
   or touches `~/.config/gff`; no test writes outside `t.TempDir()`; the runner deadline test
   uses a stub `ssh` on a temp `PATH`, never a real host.
6. **Privacy:** never write the login username, real hostnames, or e-mail addresses into
   committed files, commit messages, PR bodies, or evidence captures — use `$HOME`, `<user>`,
   `<host>`. Scrub live-gate transcripts before committing (`sed` the hostname to `<host>`).
   The `privacy_guard` hook blocks violations.
7. **`.gitignore` allowlist:** `!sdk/**`, `!.github/**`, `!opt/**`, `!docs/**` cover the
   planned paths; `opt/etc/fleet/fleet.yaml` (task 27) needs its own narrow `!opt/etc/fleet/**`
   rule per plan §2. Verify every new path with `git status --short -- <path>` and
   `git check-ignore -v <path>`; never `git add -f`.
8. **Confirmation before publishing.** Any `git add` / `git commit` / `gss feature checkpoint`
   (it pushes) / `gss feature pr` requires the user's confirmation via `AskUserQuestion`. In an
   approved autonomous run that approval is granted once by the kickoff for commit + checkpoint,
   but **PR promotion (`--ready`), `gss feature merged`, and every live-host gate always
   re-confirm.**
9. **The two-call gss token recipe** for token-gated verbs (`pr --ready`, `merged`, `restack`):
   call 1 `mkdir -p ~/.config/gss && git rev-parse HEAD > ~/.config/gss/approval.token`;
   call 2 the gss command — **separate** Bash calls; chaining is blocked by `safety_guard.sh`.
   Commit before generating the token (it is HEAD-bound). `checkpoint` is not token-gated.
10. **Never run `install.sh` from a worktree** — it plants absolute symlinks in `$HOME`.
    `bash sdk/fleet/build.sh` (copies a binary to `~/opt/bin/fleet`) is fine from `<WT>`; the
    live gates that exercise `./install.sh` do so **on the remote host** via `fleet update`.
11. **Evidence before assertions.** No `PASS`/`done` in TRACKING for a command you did not run
    and read. Live gates G1–G9 are human-in-the-loop: capture the real transcript or leave the
    box unticked.
12. **Interactive gates in a real terminal** — never backgrounded or piped (`fleet update`
    interactive steps, `gh auth login --web`, the TUI). The plan file adversarial cases
    (plan §7: `run: "; rm -rf ~"` shown verbatim by `--dry-run`, `main;id` rejected) are
    exercised with `--dry-run` only, never sent to a host.

## 6. Command cheat-sheet

Go commands from `<WT>/sdk/fleet/`; `gff lint` from `<WT>/sdk/gff/`; `gss` from inside `<WT>`.

```sh
# every task (plan §4 preamble)
cd <WT>/sdk/fleet && gofmt -l . && go vet ./... && go test -race -cover ./...

# per-package coverage bars
go test -race -cover ./internal/updplan                      # >= 90 %  (leaf A gate)
go test -race -cover ./internal/updexec ./internal/runner    # updexec >= 90 %  (leaf B gate)
go test -race -cover ./internal/featflag                     # >= 90 %  (leaf C gate)
go test -race -cover ./cmd                                   # not below the §1.6 baseline (leaf D/E)
go test ./internal/updexec -coverprofile=cover.out && go tool cover -func=cover.out | tail -1

# one task's tests, verbose (RUN-RED / RUN-GREEN)
go test ./internal/updplan -run 'TestDefaultPlanIsTodaysUpdate' -v

# gff feature file (leaf C task 18; preflight 7)
cd <WT>/sdk/gff && go run . lint ../../.github/gff/features.yaml
cd <WT> && gff get fleet.update.config          # -> home   (live layer is discovered from cwd)

# deps (leaf A task 1: yaml; leaf C task 18: gff)
go get gopkg.in/yaml.v3 && go mod tidy && git diff --stat -- go.mod go.sum
go mod edit -require=github.com/sfc-gh-eraigosa/dotfiles/sdk/gff@v0.0.0 -replace=github.com/sfc-gh-eraigosa/dotfiles/sdk/gff=../gff && go mod tidy

# build + install the binary (safe from a worktree — it copies, it does not symlink)
bash <WT>/sdk/fleet/build.sh && ~/opt/bin/fleet version

# repo-wide floor (unchanged: fleet=60)
cd <WT> && ./scripts/test.sh

# evidence capture (append-only, dated header)
{ echo "# $(date -u +%F) task NN — <command>"; <command>; } 2>&1 | tee -a <WT>/docs/mbo/plans/fleet-update/evidence/<leaf>/task-NN.txt

# allowlist check for a new path
git status --short -- <path> && git check-ignore -v <path>

# worker lifecycle
gss feature checkpoint                                                    # inside <WT>; after every task
mkdir -p ~/.config/gss && git rev-parse HEAD > ~/.config/gss/approval.token   # call 1
gss feature pr --ready --worker <worker_ref>                              # call 2 (token-gated; ASK first)
gss feature merged <worker_ref>                                           # after merge (positional ref; token-gated)
gss feature conflicts                                                     # cross-worker overlap report
```

## 7. Resuming, recovery, and corrections

- **Resume:** read TRACKING (what is proven; §4 blockers) then TODO (first unchecked box).
  Re-run that task's last verification command before continuing.
- **Ledger drift:** TRACKING says `done` but the test fails → trust the test; reset the row to
  `in-progress`, note the drift in §5, redo.
- **Rebase / conflicts:** `gss feature conflicts`; if a checkpoint or rebase aborts on conflict,
  use the `git-machete` skill to restack — gss stays the single writer for PRs and the registry.
- **Stale base:** after any parent merge (`A→B`, `B→D`, `C→main`), checkpoint the children so
  they rebase; verify `git -C <WT> log --oneline -1` against the parent tip.
- **Checkpoint regenerates the PR body:** re-apply any custom body (proof matrix, evidence links)
  after the final checkpoint via `gh pr edit --body-file`, preserving the `gss:stack` markers.
- **A frozen contract looks wrong:** stop; write the mechanism (not speculation) into TRACKING §4;
  escalate to the objective owner. Never edit `../fleet-update.md`, the spec, or the design as
  part of the build.
- **Known plan tensions (recorded here, not blockers):** (a) `go.mod` is touched by both A and
  C (§2 reconcile note); (b) "moved" tests keep two copies between leaves B and D (§5.3).
- **Correcting this file:** IMPLEMENTATION.md is procedure, not contract — fix a wrong command
  here freely and note it in the session log.

## 8. Kickoff prompt (always CURRENT — history lives in git)

> **Maintenance rule:** exactly ONE prompt here — the one that starts the NEXT session.
> Replace it at session end (past prompts are in `git log -- docs/mbo/plans/fleet-update/IMPLEMENTATION.md`).

### CURRENT prompt — start leaf A (`updplan`), task 1

> **Open the fleet-update build: register the gss feature, create the leaf A worker, and land
> plan tasks 1–5 (`internal/updplan`) TDD, ending with the leaf A gate at ≥ 90 % coverage.**
>
> Read first, completely: `docs/mbo/plans/fleet-update/IMPLEMENTATION.md` (§1 preconditions,
> §2 worker map + reconcile note, §3 loop, §5 hard rules), `docs/mbo/plans/fleet-update/TRACKING.md`
> (§0 registry to fill, §1 rows 1–5, §3 stop condition), `docs/mbo/plans/fleet-update/TODO.md`
> (the cursor — Preflight, then `# Leaf A`), then plan `docs/mbo/plans/fleet-update.md` §3.2
> (the frozen `updplan` contract — types, functions, validation rules), §4 tasks 1–5 (test names
> and exact commit messages), §6.1, and spec `docs/mbo/specs/fleet-update.md` §4 F1 (schema,
> built-in default) and F7 (backoff schedule).
>
> Orchestration: you are the orchestrator — delegate implementation to subagents when it
> helps (the go team: `go-godev` to implement, `go-goqa` to review coverage), but run every
> RUN-RED / RUN-GREEN / gate command YOURSELF and read the output before ticking a box
> (evidence before assertions). Keep git and gss in your own hands: stage by explicit name,
> one commit per task with the plan's exact message, `gss feature checkpoint` after every task,
> ledger single-writer on the leaf A branch. TDD strictly: the FAIL is observed and recorded
> before any production code.
>
> Scope for this session, in order:
> 1. Preflight (TODO "Preflight" boxes; IMPLEMENTATION §1 rows 1–10). If row 1 fails (plan not
>    on `origin/main`) STOP and report. If row 5 shows no `fleet-update` feature, run
>    `gss feature start` as written there.
> 2. Leaf A setup: `gss feature worker add … --purpose updplan …` (§2.2); copy `worker_ref`,
>    `branch`, `worktree_path`, `base_branch` VERBATIM into TRACKING §0 and IMPLEMENTATION §2.1;
>    confirm `<WT>` is on the worker branch at `origin/main`'s tip.
> 3. Tasks 1 → 5 per TODO, each: RED → RUN-RED (fail observed) → GREEN → RUN-GREEN → VERIFY
>    (`gofmt -l .`, `go vet ./...`, `go test -race -cover ./...`) → evidence file
>    `evidence/updplan/task-0N.txt` → TRACKING row → COMMIT (plan message) → CHECKPOINT.
> 4. Leaf A gate: `go test -race -cover ./internal/updplan` ≥ 90 % teed to
>    `evidence/updplan/leaf-gate.txt`; TRACKING §0 state `in-review`; `docs/mbo/index.md`
>    state `building`; refresh this §8 prompt for the next session (leaf B task 6 stacked on A,
>    and leaf C in parallel).
>
> Human-in-the-loop stops (never fake, always `AskUserQuestion`): the first `git commit` of the
> session (grants commit + checkpoint for the rest of the run), any `gss feature pr --ready`,
> any change to a plan §3 contract (do not — escalate instead), any `go.mod` edit beyond
> `gopkg.in/yaml.v3`. Blocked → TRACKING §4 with the real failing command and output, then
> continue with the next independent task (leaf C task 17 may start in parallel if leaf A is
> blocked). Done when: TRACKING §1 rows 1–5 are `done` with SHAs and evidence, the leaf A gate
> line in TRACKING §3 is ticked with the observed percentage, the draft PR exists, and this
> prompt has been replaced by the leaf B/C kickoff.

> Companion to plan `../fleet-update.md`. Update `../../index.md` state as each leaf moves
> (`planning → building → in-review → merged`).
