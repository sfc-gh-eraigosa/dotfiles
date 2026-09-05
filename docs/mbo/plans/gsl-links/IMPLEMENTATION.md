# gsl-links — implementation playbook

- **Slug:** gsl-links
- **Date:** 2026-09-05
- **Status:** Ready to execute
- **Plan (source of truth):** [`../gsl-links.md`](../gsl-links.md) · spec [`../../specs/gsl-links.md`](../../specs/gsl-links.md)
- **Objective anchors:** issue #278 · PR #279 · `docs/mbo/index.md` row `gsl-links`

> This file is the **procedure**. It does not restate the plan — it tells a fresh agent
> session how to execute the plan, task by task, resumably. The plan wins any conflict.

## 0. The three files

| File | Role | Written by |
| :-- | :-- | :-- |
| `IMPLEMENTATION.md` (this) | Procedure: preconditions, worker map, per-task loop, hard rules, kickoff prompt | Read-only during the run (except §7 corrections) |
| [`TRACKING.md`](./TRACKING.md) | Live **state ledger**: per-task status/commit/evidence, proof matrix, blockers, append-only session log | Updated after **every** task |
| [`TODO.md`](./TODO.md) | The **cursor**: plan tasks expanded into ordered micro-steps | Checkboxes ticked as you go |

**Resumption rule:** the first unchecked box in `TODO.md` is the next action; `TRACKING.md`
says what has been proven. Re-run the last verification command before continuing —
the ledger is a claim, the command is the proof.

## 1. Preconditions
| Check | Verify |
| :-- | :-- |
| On branch `worktree/gsl` in the herdr worktree (NOT `~/git/dotfiles`) | `git rev-parse --abbrev-ref HEAD` → `worktree/gsl`; `git rev-parse --show-toplevel` ends in `worktree-gsl` |
| Go toolchain ≥ 1.26 | `cd sdk/gsl && go version` |
| gsl tests green at baseline | `cd sdk/gsl && go test ./... 2>&1 \| tail -3` → all `ok` |
| gff binary + lint available | `gff version`; `gff lint` (repo root) → clean |
| Working tree clean before each task | `git status --short` → empty (evidence files from a prior task are committed) |

## 2. Worker map
Single classic-lane worker: branch `worktree/gsl`, worktree `~/.herdr/worktrees/dotfiles/worktree-gsl`, draft PR #279 (opened by `gss pr` on the first commit). No `gss feature` stack; no breakout. Task order is strict: T1 → T2 → T3 → T4 → T5 → T6.

## 3. The execution loop (every task)
1. Locate: first unchecked `TODO.md` box → its plan task; read the plan task fully.
2. RED: write the failing test first; run it; **verify it fails**; record the failure line.
3. GREEN: implement the minimum; run to pass.
4. Gates: `go test ./... -race` in `sdk/gsl`, `go vet ./...`; T5 adds `gff lint`, `make lint-shell`, `make lint-portability`; `git status --short -- <new path>` for every new file (allowlist `.gitignore`).
5. Ledgers: tick `TODO.md`; update the `TRACKING.md` task row (status, commit, evidence).
6. Commit with the plan's exact message, staging by explicit name, with the session attribution trailers. Checkpoint: mint a gss token and run `gss push` in a **separate** Bash call (confirm via the interactive prompt first).

## 4. Done-when gates
- T1: `go test ./internal/render/ -race` green; `TestJoin_SpansZeroWidth_BothPaths` and `TestTruncateToWidth_ClipsSpans` pass.
- T2: `TestNormalizeRemote` (8 forms) + `TestTreeURL`/`TestFileURL`/`TestTimeURL_Placeholders` pass.
- T3: every segment implements `LinkedSegment`; `TestDetectFormat_MatchesRender` passes with links on; golden `links` case added and reviewed.
- T4: `internal/flags` tests pass incl. the 20 ms-budget test; `go build ./...` with the gff dependency.
- T5: `go test ./... -race` green; `gff lint`, `make lint-shell`, `make lint-portability` clean; live `gsl status` shows ≥6 `]8;;` in a PR worktree and `gff set gsl.links.time false` removes the time link.
- T6: docs updated; human click check recorded with observed URLs.
- **Objective stop condition:** TRACKING §3 fully ticked; PR flipped from draft; index row `in-review`.

## 5. Hard rules
- `render` never imports `os/exec`; gff and git execs live in `internal/flags` / `internal/git`, called from `cmd`.
- Fail-open everywhere a link could be lost by an error: gff, remote URL, template.
- Links add zero width; every OSC 8 open is closed; underline SGR closed within the span.
- Never `git add -A`; never run `install.sh` from this worktree; `bash sdk/gsl/build.sh` is fine.
- privacy_guard: no literal home paths or the login name in commands/evidence — use `$HOME`/`~`, scrub evidence with `sed "s#$HOME#~#g"`.
- Confirm through the interactive prompt before `git commit`, `gss push`, `gss pr`.
- Evidence before assertions: a TRACKING row is `done` only with a SHA and observed output.

## 6. Command cheat-sheet
```bash
cd sdk/gsl && go test ./... -race -cover            # unit gate
cd sdk/gsl && go test ./internal/render/ -run Golden -update   # ONLY when a golden legitimately changes; review the diff
cd sdk/gsl && go vet ./... && go build ./...
gff lint                                             # repo root; flag schema
make lint-shell && make lint-portability             # install.sh change (T5)
bash sdk/gsl/build.sh                                # refresh ~/opt/bin/gsl for the live check
gsl status | cat -v | grep -o ']8;;[^\\]*' | sort -u # list emitted link targets
gss push --approval-token "$(gss approval-token)"     # two SEPARATE Bash calls: mint, then push
```

## 7. Resuming, recovery, and corrections
Fix wrong commands here freely and note it in the session log. Never edit the plan/spec
as part of the build — escalate contract defects via TRACKING blockers instead.
If `TestDetectFormat_MatchesRender` diverges after T3, the legacy `Render` delegation is incomplete for one segment — check that each `Render` calls `RenderLinked` → `detect` → `formatLinkedOf`.

## 8. Kickoff prompt (always CURRENT — history lives in git)
> **Maintenance rule:** exactly ONE prompt here — the one that starts the NEXT session.
> Replace it at session end (past prompts are in `git log -- <this file>`).

Mission: build `gsl-links` (spec `docs/mbo/specs/gsl-links.md`, plan `docs/mbo/plans/gsl-links.md`) on branch `worktree/gsl` in the herdr worktree, TDD, one task per commit.
Read first: this file §1–§6, `TRACKING.md`, `TODO.md`, then the plan task named by the first unchecked TODO box.
Scope in order: T1 spans/join/fit → T2 remote URL + builders → T3 segment spans + Render delegation → T4 flags package → T5 wiring (config, cmd, preview, features.yaml, install.sh) → T6 docs + human click check.
Human-in-the-loop stops: before every commit/push (interactive prompt); the T6 click check needs the user at a herdr pane.
Blocked → add a TRACKING §4 row with the failing command and its real output; do not patch contracts.
Done-when: TRACKING §3 all ticked, PR out of draft, index row `in-review`.
