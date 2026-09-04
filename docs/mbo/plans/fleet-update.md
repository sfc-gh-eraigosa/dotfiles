# fleet-update — config-driven, multi-repo update DAG for the fleet CLI — implementation plan

- **Slug:** fleet-update
- **Date:** 2026-09-02
- **Status:** Draft
- **Relates to:** spec [`../specs/fleet-update.md`](../specs/fleet-update.md) · design [`../designs/fleet-update.md`](../designs/fleet-update.md) · issue [#265](https://github.com/sfc-gh-eraigosa/dotfiles/issues/265) · PR [#270](https://github.com/sfc-gh-eraigosa/dotfiles/pull/270)
- **Execution trio:** [`fleet-update/IMPLEMENTATION.md`](./fleet-update/IMPLEMENTATION.md) · [`TRACKING.md`](./fleet-update/TRACKING.md) · [`TODO.md`](./fleet-update/TODO.md)

## 1. Summary & verdict

`fleet update <host>...` becomes a DAG runner driven by `~/.config/fleet/fleet.yaml`. The
controller walks the plan; each step is one SSH command per host through `runner.Runner`; hosts
stay dumb. Step kinds: `sync` (fetch/checkout/ff a repo, multi-branch, `default` branch,
auto-clone from `url`, per-repo local-state policy with branch restore), `run` (user-authored
shell in a repo dir, optionally interactive), `gh-auth` (batch check first, interactive web
login only on a miss, never a token). Failed steps skip transitive dependents; independent
branches continue; retries with backoff under per-attempt controller-enforced timeouts. With no
YAML the built-in plan is byte-for-byte today's behaviour and `--ref` / `--force` keep working.
gff gates the feature (`fleet.update.enabled`, fail-open) and picks the plan location
(`fleet.update.config`: `home` | `repo`). The TUI lands last, on the same executor.

Verified while planning (shape the plan):
1. `rescueWorktree`, `updateScript`, `resetToFetched` (`cmd/update.go`) and `status.go`'s
   `remoteRepo` all hardcode `~/git/dotfiles` — they move into `updexec`, parameterised by path;
   `cmd` keeps thin wrappers only while the TUI still needs them (between leaves D and E).
2. `runner.Fake` returns one canned `Out` per host regardless of argv; a multi-step run needs a
   scripted double (substring-keyed, per-step sequences) defined in `updexec` tests — `runner`
   is not changed for that. `runner` *is* extended with `RunStreamCtx` for timeouts.
3. The gff live layer is discovered from `os.Getwd()`, so the adapter scopes every lookup to
   `gff.WithSource(<--repo path>)` (the checkout's live feature file, cwd-independent) and falls
   back to unscoped only on `ErrUnknownSource`; every other error is fail-open. (Planned as a
   namespace scope; changed after the leaf C review showed that read a stale snapshot.)
4. gff choice flags are closed option sets, so the "config path" flag is a two-option location
   selector; arbitrary paths use `--file`.

Global constraints (every task): `gofmt -l .` empty, `go vet ./...` clean, `go test -race
./...` green; every impure edge injected (`runner.Runner`, `StepIO`, `Output`, `Now`, `Sleep`,
`Rand`, `featflag.Source`, filesystem via `XDG_CONFIG_HOME`/`t.TempDir()`, `os.Executable` for
the handoff); no test opens a socket, reads `$HOME`, or touches `~/.config/gff`; no existing
test deleted — moved tests keep their names and assertions and are listed in the commit body.

## 2. File inventory

| Path | Purpose | Implements |
| :-- | :-- | :-- |
| `sdk/fleet/internal/updplan/plan.go` (create) | types, `Parse` (`yaml.Decoder.KnownFields(true)` → defaults merge → validate → resolve paths), `Default`, `DefaultYAML`, `WithRef`/`WithRefs`, `Backoff.Wait` | spec F1, F7 schedule |
| `sdk/fleet/internal/updplan/validate.go` (create) | `ValidRef` (moved from `cmd/update.go`), `ValidPath`, `ValidRepoName`, `ValidID`, `ValidURL`, `ValidHostname`, duration/retry/local checks, `errors.Join` aggregation | F1 |
| `sdk/fleet/internal/updplan/graph.go` (create) | `Order()` stable Kahn, `Dependents()`, cycle detection | F7 |
| `sdk/fleet/internal/updplan/*_test.go` (create) | tasks 1–5; ≥90 % | §5 |
| `sdk/fleet/internal/updexec/script.go` (create) | `PrecheckScript`, `RescueScript` (generalised `rescueWorktree`), `ResetScript` (`resetToFetched`), `SyncScript` (prologue/body/epilogue; single, multi, default), `CloneScript`, `RestoreScript`, `RunScript`, `GhAuthCheck`, `GhAuthLogin`, `ShQuote` (moved from `tui_cmds.go`); every builder re-validates | F3–F6 |
| `sdk/fleet/internal/updexec/exec.go` (create) | `Executor`, `StepIO`, `Console`, `Background`, `Output`/`LineWriter`/`Discard`, `Result`, `HostReport`, attempt loop, cascade, synthesized restore, `exitCode` | F4, F7 |
| `sdk/fleet/internal/updexec/*_test.go` (create) | scripted `fakeIO`, stepping clock, recorded sleep; tasks 6–13b; ≥90 % | §5 |
| `sdk/fleet/internal/runner/runner.go` (modify) | `RunStreamCtx` on `Runner`, `Exec` (`exec.CommandContext`), `Fake` (`ctx.Done()`); `RunStream` delegates | F7 timeout |
| `sdk/fleet/internal/runner/*_test.go` (modify) | task 13c; mux invariant extended to the ctx path | §5 |
| `sdk/fleet/internal/featflag/featflag.go` (create) | `Source`, `Settings`, `Resolve` (fail-open; `home`→"", `repo`→`<repoDir>/opt/etc/fleet/fleet.yaml`), `Static` | F2, F9 |
| `sdk/fleet/internal/featflag/gff.go` (create) | `GFF` adapter — the **only** import of `sdk/gff/pkg/gff` | F9 |
| `sdk/fleet/internal/featflag/*_test.go` (create) | tasks 14–15 | §5 |
| `sdk/fleet/go.mod`, `go.sum` (modify) | `gopkg.in/yaml.v3`, `github.com/sfc-gh-eraigosa/dotfiles/sdk/gff v0.0.0` + `replace … => ../gff` | F1, F9 |
| `.github/gff/features.yaml` (modify) | new `area: fleet` — `fleet.update.enabled`, `fleet.update.config` | F9 |
| `sdk/fleet/cmd/update.go` (rewrite) | flags `--local --force --no-restore --reset --timeout --no-retry --ref[] --file --dry-run`; `loadPlan`; `runUpdate`; report; exit code; `--json` | F2, F8 |
| `sdk/fleet/cmd/update_init.go` (create) | `fleet update init [--file] [--overwrite] [--print]` | F8 |
| `sdk/fleet/cmd/update_output.go` (create) | `captureOutput`: `updexec.Output` over `applog.NewCapture` (same options as `teeToRunLog`) | F10 |
| `sdk/fleet/cmd/answers_store.go` (modify) | extract `fleetConfigDir()`; add `defaultPlanPath()` | F2 |
| `sdk/fleet/cmd/status.go` (modify, small) | `remoteRepo` from the plan's `dotfiles` repo path when present | consistency |
| `sdk/fleet/cmd/update_test.go` (rewrite) | 10 existing tests migrated onto `runUpdate`/`updexec`, names kept; tasks 17–21 | §5 |
| `sdk/fleet/cmd/tui_cmds.go`, `tui_model.go`, `tui.go`, `tui_*_test.go`, `runlog_test.go` (modify, leaf E) | `beginStream` over `Background`; handoff self-execs `fleet update`; `ErrNoTerminal` → interactive queue; `--update-ref`→`WithRef`; `--file` | F11 |
| `sdk/fleet/AGENTS.md`, `README.md` (modify) | command row, layout rows, invariants, `fleet.yaml` reference, local-changes table | docs |
| `opt/etc/fleet/fleet.yaml` (create, optional) + `.gitignore` `!opt/etc/fleet/**` | sample tracked plan for the `repo` location | F2 |
| `docs/mbo/{designs,specs,plans}/fleet-update.md`, `plans/fleet-update/{IMPLEMENTATION,TRACKING,TODO}.md`, `evidence/`, `index.md` row | MBO trail | — |
| `scripts/test.sh` (no change) | fleet floor stays 60 (warn-only); new packages hold ≥90 by their own gate | — |

Reused by name: `validRef`, `rescueWorktree`, `resetToFetched`, `updateScript` (thin wrapper
until E), `teeToRunLog` / `fleetLogDir` / `nowFn` / `logTool`, `answersPath` config-dir logic,
`runner.Fake` + `recordingRunner` (`keys_test.go`), `shQuote`, `handoffWrapper`, the
`os.Executable()` self-exec pattern (`configShell`), `sudoGate` / `answers.envPrefix()`.

## 3. Interface contracts (frozen)

### 3.1 YAML schema — see spec §4 F1 (normative). Built-in default = spec F1 last paragraph.

### 3.2 `internal/updplan`

```go
type Kind string      // KindSync="sync", KindRun="run", KindGhAuth="gh-auth"
type OnFailure string // Stop="stop", Continue="continue"
type Local string     // LocalSkip="skip", LocalRescue="rescue", LocalCarry="carry"
type RetryOn string   // "transport" | "timeout" | "any" | "exit:<n>"

type Backoff struct { Initial, Max time.Duration; Factor float64; Jitter bool }
func (b Backoff) Wait(n int, rnd func() float64) time.Duration   // n = 1-based failed attempt; min(Max, Initial×Factor^(n-1)) ±50% when Jitter
type Retry   struct { Attempts int; On []RetryOn; Backoff Backoff }
type Defaults struct { Timeout time.Duration; Retry Retry }
type Expect  struct { Exit []int }

type Step struct {
    ID string; Kind Kind; Repo, Run string; Interactive bool; Needs []string
    Expect Expect; OnFailure OnFailure; Hostname string
    Timeout time.Duration; Retry Retry              // resolved (defaults merged) after Parse
}
type Repo struct { Name, Path, URL string; Branches []string; Local Local; Restore bool }   // Path is the REMOTE path: absolute or ~/…, never relative
type Plan struct { Root string; Defaults Defaults; Repos map[string]Repo; Steps []Step; Source string }

func Default() Plan
func Parse(data []byte) (Plan, error)
func (p Plan) Order() []Step                          // topological; ties by declaration index
func (p Plan) Dependents(id string) []string          // transitive, in Order() order
func (p Plan) Step(id string) (Step, bool)
func (p Plan) RepoOf(st Step) (Repo, bool)
func (p Plan) LastStepUsing(repo string) (string, bool)   // for the synthesized restore
func (p Plan) WithRef(spec string) (Plan, error)      // "b" → repo "dotfiles" else sole repo else error; "repo=b"; ValidRef(b); replaces Branches[0], drops a duplicate extra
func (p Plan) WithRefs(specs []string) (Plan, error)

func ValidRef(s string) bool        // letters digits . _ / - (today's validRef) PLUS git check-ref-format rules: no leading '-', no '..', no '@{', no '.lock' suffix (review finding: a leading '-' is a git option once interpolated bare)
func ValidRepoName(s string) bool   // ^[a-z0-9][a-z0-9._-]*$
func ValidID(s string) bool         // same charset
func ValidPath(s string) bool       // [A-Za-z0-9._/-]+, optional leading "~/", no ".." segment, no leading "-"
func ValidURL(s string) bool        // ^(https://|ssh://|git@)[A-Za-z0-9._:/@~-]+(\.git)?$
func ValidHostname(s string) bool   // ^[A-Za-z0-9.-]+$
func ValidSHA(s string) bool        // exactly 40 hex
const DefaultYAML string            // the commented starter `fleet update init` writes
```

Validation rules (aggregated with `errors.Join`, each error names step/repo + field): unknown
keys; `version` ≠ 1; duplicate ids; unknown kind; `sync` without repo / unknown repo; `gh-auth`
with repo; `run` without `run:`; `run:` containing NUL or newline; `interactive` on non-run;
`retry` on interactive; `needs` unknown / self / cycle; `default` not first; duplicate branches (a tag cannot be
told from a branch syntactically, so "tag only as the sole entry" is documented, not validated —
review finding, PR #270); `hostname` on a non-gh-auth step; `update.root` not absolute/`~`-relative
or outside the path charset; `expect.exit` outside 0..255; bad `on_failure` / `local`; `attempts < 1`;
unknown `retry.on` token; `factor < 1`; unparsable duration; negative timeout.

### 3.3 `internal/updexec`

```go
type Status string // OK="ok", Failed="failed", Skipped="skipped", DepFailed="dependency-failed"
type Result struct {
    Step string; Kind updplan.Kind; Status Status
    Exit int                    // 0 ok; -1 unknown; 255 = ssh transport failure
    Duration time.Duration; Reason string; Notes []string   // Notes = remote lines prefixed "fleet: "
    Attempts int; TimedOut bool
}
type HostReport struct { Host, Plan string; Started time.Time; Results []Result; Output string }
func (h HostReport) Failed() bool     // any Result.Status != OK
func (h HostReport) Err() error       // nil or "step <id>: <reason>" for the first non-ok

type StepIO interface {
    Batch(ctx context.Context, host string, st updplan.Step, script string) (out string, err error)
    Interactive(ctx context.Context, host string, st updplan.Step, script string) error
}
var ErrNoTerminal = errors.New("updexec: step needs a terminal this lane cannot provide")
var ErrTransport  = errors.New("updexec: ssh transport failure")

type Console struct {                  // CLI lane: Batch → R.RunStreamCtx drained to Line; Interactive → R.RunInteractive
    R runner.Runner
    Line     func(host, line string)        // may be nil
    Stdin    func(st updplan.Step) string   // nil → ""; TUI supplies the sudo secret for run steps only
    Preamble func(st updplan.Step) string   // nil → ""; prepended to run-step scripts ONLY
}
type Background struct{ Console }      // Interactive → ErrNoTerminal

type Output interface{ Open(host, header string) (LineWriter, string) }
type LineWriter interface{ Line(string); Close(footer string) }
type Discard struct{}

type Executor struct {
    IO StepIO; Out Output
    Now   func() time.Time             // nil → time.Now
    Sleep func(time.Duration)          // nil → time.Sleep (backoff)
    Rand  func() float64               // nil → math/rand (jitter)
    Local updplan.Local                // "" = per-repo policy; else overrides every repo (--local / --force)
    NoRestore, Reset, NoRetry bool
    Timeout time.Duration              // >0 overrides every batch step (--timeout)
}
func (e Executor) RunHost(host string, p updplan.Plan) HostReport

// script builders (pure; each re-validates and returns error on any invalid field)
func PrecheckScript(r updplan.Repo) (string, error)
func SyncScript(r updplan.Repo, local updplan.Local, reset bool) (string, error)   // prologue + body + epilogue
func CloneScript(r updplan.Repo) (string, error)
func RescueScript(r updplan.Repo) (string, error)
func ResetScript(ref string) string                                              // today's resetToFetched text
func RestoreScript(r updplan.Repo, orig, sha string) (string, error)             // ValidRef(orig)||ValidSHA(orig); sha "" or ValidSHA
func RunScript(st updplan.Step, r *updplan.Repo) (string, error)
func GhAuthCheck(host string) (string, error)
func GhAuthLogin(host string) (string, error)
func ShQuote(s string) string
```

**RunHost algorithm**

```
w, path := Out.Open(host, header)        // "fleet update — host=H plan=<Source> mode=fast-forward|FORCE RESET started=<RFC3339>"
status := map[id]Status{}; pending := map[repo]restoreInfo{}
for st in p.Order():
    if blocker := firstStopBlocker(st): record(DepFailed, "blocked by "+blocker); continue
    res := attempt loop:
        for n := 1; ; n++:
            w.Line("=== step ID (KIND) attempt n/N ===")   // "attempt" suffix only when N>1
            ctx := WithTimeout(effectiveTimeout(st))       // 0 → no deadline; interactive: only explicit
            out, err := runStep(ctx, st)                   // sync: precheck→clone|skip|rescue+sync|carry-sync; run; gh-auth: check→login→check
            classify → ok | failed(exit, timedOut, transport)
            if ok || !matches(st.Retry.On, class) || n == effectiveAttempts(st): break
            Sleep(st.Retry.Backoff.Wait(n, Rand))
    record(res); parse notes → if sync switched/carried: pending[repo] = {orig, sha}
    if sync failed after switching/stashing: runRestore(repo) now
    if id == p.LastStepUsing(repo) && pending[repo] && restoreEnabled(repo): runRestore(repo)   // synthesized "<repo>.restore", fixed policy 3×transport, 5m
w.Close("finished")
```

### 3.4 Exact remote shell strings — spec F3/F4/F5/F6 plus the approved plan's script block are
normative; reproduced here for the builders' tests (`P` path, `Bn` branches, `U` url quoted,
`H` hostname, `RUN` verbatim, `N` repo name):

```sh
# PrecheckScript — read-only
if [ ! -e P/.git ]; then echo "state=missing"; else cd P && g=$(git rev-parse --git-dir) && if [ -e "$g/MERGE_HEAD" ] || [ -d "$g/rebase-merge" ] || [ -d "$g/rebase-apply" ]; then s=in-progress; elif [ -n "$(git status --porcelain)" ]; then s=dirty; else s=clean; fi; b=$(git symbolic-ref -q --short HEAD || echo detached); echo "state=$s branch=$b"; fi
# SyncScript = PROLOGUE ; BODY ; EPILOGUE
#   PROLOGUE (all): cd P && orig=$(git symbolic-ref -q --short HEAD || git rev-parse HEAD) && echo "fleet: orig=$orig" &&
#   PROLOGUE (carry adds): ts=$(date -u +%Y%m%dT%H%M%SZ) && { [ -z "$(git status --porcelain)" ] || { git stash push -q -u -m "fleet-carry $ts" && echo "fleet: carried stash=$(git rev-parse stash@{0}) from=$orig"; }; } &&
#   BODY single (today's text): git fetch origin B1 && git checkout B1 && git merge --ff-only FETCH_HEAD          # reset=true: merge → ResetScript(B1, "FETCH_HEAD")
#   BODY multi:  git fetch origin B1 B2 && b1=B1 && git checkout -q "$b1" && git merge --ff-only "origin/$b1" && EXTRAS          # reset=true: merge → ResetScript("$b1", "origin/$b1")
#   BODY default: git fetch origin && { b1=$(git symbolic-ref -q --short refs/remotes/origin/HEAD); b1=${b1#origin/}; [ -n "$b1" ] || { b1=$(git ls-remote --symref origin HEAD | sed -n 's|^ref: refs/heads/\(.*\)[[:space:]]HEAD$|\1|p') && [ -n "$b1" ] && git remote set-head origin "$b1"; }; [ -n "$b1" ] || { echo 'fleet: cannot resolve the default branch' >&2; exit 3; }; } && git checkout -q "$b1" && git merge --ff-only "origin/$b1" && EXTRAS          # the fetch stays inside the && chain — a bare `;` there let a failed fetch fall through to resolving/checking out/merging stale refs and exit 0; reset=true: merge → ResetScript("$b1", "origin/$b1") (NOT FETCH_HEAD — after this fetch it names whichever ref was last advertised, not necessarily origin/$b1)
#   EXTRAS: fail=0; for b in B2 B3; do [ "$b" = "$b1" ] && continue; if git show-ref -q --verify "refs/heads/$b"; then if git merge-base --is-ancestor "$b" "origin/$b"; then git branch -q -f "$b" "origin/$b" && echo "fleet: ff $b" || fail=1; else echo "fleet: skipped(diverged) $b"; fi; else git branch -q --track "$b" "origin/$b" && echo "fleet: created $b" || fail=1; fi; done; [ "$fail" = 0 ]          # POSIX `for`'s exit status is only its LAST iteration's; the flag keeps an earlier failing extra from being swallowed by a later successful one (skipped(diverged) is not itself a failure)
#   EPILOGUE (all): ; rc=$?; now=$(git symbolic-ref -q --short HEAD || git rev-parse HEAD); [ "$orig" = "$now" ] || echo "fleet: switched $orig -> $now"; exit $rc
# CloneScript: mkdir -p "$(dirname P)" && git clone -q --branch B1 'U' P && cd P && b1=B1 && EXTRAS      # default: no --branch, b1 from symbolic-ref
# RescueScript: cd P && ts=$(date -u +%Y%m%dT%H%M%SZ) && orig=$(git symbolic-ref -q --short HEAD || git rev-parse HEAD) && git checkout -q -b "fleet-rescue/$ts" && git add -A && { git -c user.email=fleet@local -c user.name=fleet commit -q -m "fleet rescue $ts" || true; } && git checkout -q "$orig" && mkdir -p ~/.local/state/fleet/rescue/N && git worktree add ~/.local/state/fleet/rescue/N/$ts "fleet-rescue/$ts"          # orig via symbolic-ref (falls back to the SHA) — `rev-parse --abbrev-ref HEAD` prints the literal string "HEAD" when detached, which made the later checkout a no-op and stranded the clone on fleet-rescue/$ts; commit is `|| true`, mirroring ResetScript — submodule-only dirt stages nothing under `git add -A` and would otherwise abort the chain
# RestoreScript: cd P && git checkout -q ORIG && { [ -z "SHA" ] || { git stash apply -q SHA && git stash drop -q SHA && echo "fleet: restored stash=SHA"; }; } || { echo "fleet: restore-failed stash=SHA branch=ORIG" >&2; exit 4; }
# RunScript: cd P && RUN        |  RUN
# GhAuthCheck: command -v gh >/dev/null 2>&1 || exit 127; gh auth status -h H >/dev/null 2>&1
# GhAuthLogin: gh auth login -h H --web --git-protocol https && gh auth setup-git -h H
```

Interpolation policy: branch/repo name/id/path/hostname bare after allowlist validation (path
bare so `~/` expands remotely); url single-quoted; `run:` verbatim after `cd P && `; `ORIG`/`SHA`
validated (`ValidRef`|`ValidSHA` / `ValidSHA`). Nothing else from a host is ever interpolated.

### 3.5 `internal/featflag`

```go
type Source interface { Bool(key string) (bool, error); Strings(key string) ([]string, error) }
type Settings struct { Enabled bool; ConfigPath string; Note string }   // Enabled=true on ANY error; ConfigPath "" → caller's default
const ( KeyEnabled = "fleet.update.enabled"; KeyConfig = "fleet.update.config" )
func Resolve(src Source, home, repoDir string) Settings   // "home"→"", "repo"→repoDir+"/opt/etc/fleet/fleet.yaml" (repoDir must be absolute, else Note + ""); >1 selection → Note + ""; typed-nil src is fail-open
type Static struct { Bools map[string]bool; Strs map[string][]string; Err error }
type GFF struct{ Repo string; Opts []gff.Option }          // gff.go only; nil-receiver safe. Scoped to gff.WithSource(Repo) — the --repo checkout's LIVE feature file — retrying unscoped only on ErrUnknownSource; Strings = gff.Selected (option IDs). (Review finding: namespace scoping read a stale snapshot / bound the live layer to cwd.)
```

`.github/gff/features.yaml` addition:

```yaml
  - area: fleet
    features:
      - path: fleet.update.enabled
        description: Lets `fleet update` read its step plan from fleet.yaml. Set false to pin the built-in dotfiles-only plan (fetch → ff → install.sh) regardless of any file. Fail-open - if gff is unavailable fleet behaves as enabled.
        boolDefault: true
      - path: fleet.update.config
        description: Which fleet.yaml `fleet update` reads when --file is not given. `home` = $XDG_CONFIG_HOME/fleet/fleet.yaml; `repo` = opt/etc/fleet/fleet.yaml inside the local dotfiles clone (a tracked team plan).
        choiceDefault:
          mode: CHOICE_MODE_SINGLE
          options:
            - {id: home, description: "~/.config/fleet/fleet.yaml (XDG config dir)", stringValue: home, selected: true}
            - {id: repo, description: "<dotfiles clone>/opt/etc/fleet/fleet.yaml", stringValue: repo}
```

### 3.6 CLI — spec F8 (normative). `loadPlan(file string, src featflag.Source, repoDir string, fsys) (updplan.Plan, error)`.

## 4. TDD build order

Evidence: each task's done-when command output is `tee`'d to
`docs/mbo/plans/fleet-update/evidence/<leaf>/task-NN.txt` (dated header, append-only) and
committed with the task. Every task: `cd sdk/fleet && gofmt -l . && go vet ./... && go test -race ./...`.

### Leaf A — `internal/updplan` (pure; blocking base)

1. **Default plan is today's behaviour.** `TestDefaultPlanIsTodaysUpdate` (root `~/git`; one repo
   `dotfiles` path `~/git/dotfiles` branches `[main]` local skip restore true; steps exactly
   `dotfiles.sync` → `dotfiles.install` run `./install.sh` interactive), `TestDefaultYAMLRoundTripsToDefault`.
   Commit: `feat(fleet/updplan): built-in default plan equals today's update`.
2. **Validation table.** `TestParseRejects` (every rule in §3.2), `TestParseAggregatesEveryError`,
   `TestLocalDefaultsToSkipAndRestoreToTrue`. Commit: `feat(fleet/updplan): parse + aggregated validation`.
3. **Defaults merge + backoff schedule.** `TestStepInheritsDefaultsFieldByField`,
   `TestInteractiveStepsDefaultToNoTimeout`, `TestExplicitTimeoutOnInteractiveStepIsKept`,
   `TestBackoffScheduleIsExponentialAndCapped` (`5s,10s,20s,40s,80s,2m,2m` with rnd 0.5),
   `TestJitterStaysWithinHalfOfTheWait`, `TestRetryOnParsesExitCodes`.
   Commit: `feat(fleet/updplan): retry/timeout defaults merge and backoff schedule`.
4. **Topological order and cascade helpers.** `TestOrderIsTopologicalAndStable`,
   `TestDependentsIsTransitive`, `TestLastStepUsingRepo`. Commit: `feat(fleet/updplan): stable topological order`.
5. **`--ref` shim + path resolution.** `TestWithRefTargetsDotfilesByName`, `…TheSoleRepo`,
   `…IsAmbiguousWithManyRepos`, `…RepoEqualsBranch`, `…RejectsShellInjection` (vectors from
   `cmd/update_test.go` `TestValidRefRejectsShellInjection`), `…DropsADuplicateExtra`,
   `TestRepoPathResolvesUnderRoot`. Commit: `feat(fleet/updplan): --ref compatibility shim`.
   **Leaf gate:** `go test -race -cover ./internal/updplan` ≥ 90 % → `evidence/updplan/`.

### Leaf B — `internal/updexec` + `runner.RunStreamCtx` (consumes A; blocking for D/E)

6. **Byte-compat + one network call.** `TestSyncScriptSingleBranchMatchesTodaysForm`; **move**
   `TestUpdateMakesExactlyOneNetworkCall` (four assertions verbatim); add
   `TestEverySyncFormMakesAtMostOneUnconditionalNetworkCall`. Commit: `feat(fleet/updexec): sync script builders (byte-compatible, one fetch)`.
7. **Remaining builders.** `TestMultiBranchFetchesAllInOneCall`, `TestExtrasOnlyForceMoveAnAncestor`,
   `TestDefaultBranchPrefersLocalSymbolicRef`, `TestCloneNeverFetches`, `TestPrecheckUsesDashEForWorktrees`,
   `TestPrecheckReportsStateAndBranchReadOnly`, **move** `TestRescuePreservesUntrackedWork`,
   `TestResetScriptUnchanged`, `TestRunScriptIsVerbatimAfterCd`, `TestGhAuthNeverCarriesAToken`,
   `TestGhAuthCheckReserves127`, `TestRestoreUsesApplyBySHANeverPop`, `TestRestoreRejectsUnvalidatedOrigOrSHA`.
   Commit: `feat(fleet/updexec): precheck, clone, rescue, restore, run, gh-auth builders`.
8. **Builders re-validate.** `TestBuildersRejectUnvalidatedInput`. Commit: `test(fleet/updexec): builders refuse unvalidated input`.
9. **Runner ctx path.** `TestRunStreamCtxKillsTheChildOnDeadline` (stub `ssh` on `PATH` running
   `sleep 30`, 100 ms deadline), `TestRunStreamDelegatesToCtx`, `TestFakeHonoursContextCancellation`,
   `TestEveryRemotePathCarriesTheMuxOptions` extended. Commit: `feat(fleet/runner): RunStreamCtx for controller-enforced timeouts`.
10. **Executor happy path.** scripted `fakeIO` + stepping clock; `TestRunHostRunsStepsInOrderWithDurations`,
    `TestNotesAreParsedFromFleetLines`, `TestAttemptHeaderIsWrittenToTheCapture`.
    Commit: `feat(fleet/updexec): executor walks the plan per host`.
11. **Cascade.** `TestFailedStepSkipsTransitiveDependents`, `TestOnFailureContinueLetsDependentsRunButStillFailsTheHost`,
    `TestExpectExitAcceptsNonZero`, `TestDependencyFailedAlsoBlocks`. Commit: `feat(fleet/updexec): failure cascade`.
12. **Sync decisions.** **Migrate** `TestUpdateSkipsDirtyCloneByDefault`, `TestUpdateProceedsOnCleanClone`,
    `TestForceRescuesDirtyWorkBeforePulling`, `TestUpdateSurfacesProbeFailure`; add
    `TestMissingCloneWithURLClones`, `TestMissingCloneWithoutURLFails`, `TestInProgressMergeIsSkippedUnderEveryPolicy`,
    `TestResetModeUsesResetScript`, `TestUnexpectedPrecheckOutputFails`, `TestCLILocalOverridesEveryRepoPolicy`,
    `TestResetIsIncompatibleWithCarry`. Commit: `feat(fleet/updexec): local-state policies`.
13. **Carry & restore.** `TestCarryStashesWithUntrackedAndCapturesTheSHA`, `TestCarryRestoreRunsAfterTheLastStepUsingTheRepo`,
    `TestRestoreRunsEvenWhenAnIntermediateStepFailed`, `TestRestoreRunsImmediatelyWhenSyncFailsAfterStash`,
    `TestRestoreConflictKeepsTheStash`, `TestCleanOffBranchIsRestoredUnderEveryPolicy`, `TestOnTargetNeverSynthesizesARestore`,
    `TestRescueOffBranchRestoresTheBranchWithoutAStash`, `TestDetachedHeadRestoresToTheSHA`,
    `TestRestoreFalseLeavesHostOnTarget`, `TestRestoreCheckoutFailureKeepsEverything`, `TestRestoreStepHasFixedRetryPolicy`,
    `TestCarryPrologueIsIdempotentAcrossAttempts`. Commit: `feat(fleet/updexec): carry and branch restore`.
14. **gh-auth.** `TestGhAuthSkipsLoginWhenStatusPasses`, `TestGhAuthLogsInInteractivelyThenReverifies`,
    `TestGhAuthReports127AsNotInstalled`, `TestGhAuthWithoutATerminalFailsCleanly`, `TestGhAuthNeverUsesStdin`,
    `TestGhAuthLoginIsNeverRetriedButCheckIs`. Commit: `feat(fleet/updexec): gh-auth step`.
15. **Retry / backoff / timeout.** `TestTransportFailureIsRetriedWithBackoff`, `TestNonMatchingFailureIsNotRetried`,
    `TestRetryOnAnyRetriesEveryUnexpectedExit`, `TestRetryOnExitCodeMatchesOnlyThatCode`, `TestExpectedExitIsNeverRetried`,
    `TestAttemptsAreExhaustedThenOnFailureApplies`, `TestTimeoutCancelsTheAttempt`, `TestTimeoutIsRetriedOnlyWhenListed`,
    `TestInteractiveStepsAreNeverRetried`, `TestInteractiveHasNoDeadlineUnlessSet`, `TestExecutorTimeoutOverridesBatchSteps`,
    `TestNoRetryForcesOneAttempt`. Commit: `feat(fleet/updexec): retries with backoff under per-attempt timeouts`.
16. **Lanes + exit codes.** `TestConsoleStreamsBatchAndHandsOffInteractive`, `TestBackgroundRefusesInteractive`,
    `TestPreambleAndStdinApplyToRunStepsOnly`, `TestExitCodeMapsExitErrorAndSSH255`.
    Commit: `feat(fleet/updexec): Console and Background lanes`.
    **Leaf gate:** `go test -race -cover ./internal/updexec ./internal/runner` ≥ 90 % (updexec) → `evidence/updexec/`.

### Leaf C — `internal/featflag` + deps (independent; blocking for D)

17. **Fail-open resolution.** `TestResolveDefaultsWhenSourceErrors`, `TestResolveHonoursDisabled`,
    `TestResolveMapsHomeToEmptyPath`, `TestResolveMapsRepoUnderRepoDir`, `TestResolveUnknownKeyIsFailOpen`.
    Commit: `feat(fleet/featflag): fail-open gff resolution`.
18. **gff adapter + deps + flags.** `TestGFFFallsBackToUnscopedOnUnknownSource`; `go.mod`/`go.sum`
    (`go mod tidy` no diff; binary size before/after recorded); `.github/gff/features.yaml` area
    `fleet`; `cd sdk/gff && go run . lint ../../.github/gff/features.yaml` clean; `gff get fleet.update.config` → `home`.
    Commit: `feat(fleet): gff flags fleet.update.{enabled,config} + SDK adapter`.
    **Leaf gate:** ≥ 90 % featflag; `go build ./...` → `evidence/featflag/`.

### Leaf D — `cmd` rewire (consumes A, B, C)

19. **Plan loading.** `TestLoadPlanPrefersFileFlag`, `…UsesBuiltInWhenDisabled`, `…UsesBuiltInWhenNoFile`,
    `…ReadsTheConfiguredPath` (`XDG_CONFIG_HOME`), `…ReadsTheRepoLocation`, `…RefusesAWorldWritableFile`;
    `TestSavedAnswersNeverContainTheCredential` stays green. Commit: `feat(fleet/cmd): loadPlan (flag → gff → built-in)`.
20. **`runUpdate`.** `TestUpdateHostUsesTheRequestedRef`, `TestUpdateDefaultPlanSendsExactlyOneFetchPerSyncStep`,
    `TestValidRefRejectsShellInjection` (alias), `TestForceIsAnAliasForLocalRescue`, `TestTimeoutAndNoRetryFlagsReachTheExecutor`.
    Commit: `feat(fleet/cmd): fleet update runs the plan through the executor`.
21. **Report / exit / json / dry-run.** `TestReportNamesEveryStepAndTheLog`, `TestExitCodeReflectsAnyFailedHost`,
    `TestDryRunSendsNothing`, `TestJSONReportIsMachineReadable`. Commit: `feat(fleet/cmd): per-step report, --json, --dry-run`.
22. **`fleet update init`.** `TestInitWritesTheDefaultPlanOnce`, `TestInitPrintToStdout`, `TestInitOutputParsesToDefault`.
    Commit: `feat(fleet/cmd): fleet update init`.
23. **Headless run log + answers env.** `TestHeadlessUpdateIsCaptured`, `TestLocalAnswerEnvIsExportedForRunStepsOnly`,
    `TestSudoSecretNeverAppearsInTheRemoteCommand` extended. Commit: `feat(fleet/cmd): headless run log`.
    **Leaf gate:** `go test -race ./...`; `bash sdk/fleet/build.sh`; live `fleet update <host> --dry-run`
    (no file, then two-repo plan) and one live host run → `evidence/cmd/`, `evidence/e2e/`.

### Leaf E — TUI (consumes D)

24. **Background lane.** `TestSudoPreambleIsPerRunStepSession`; existing capture tests pass with the
    `plan=` header. Commit: `feat(fleet/tui): background updates run the plan executor`.
25. **Handoff delegates to the CLI verb.** `TestHandoffDelegatesToFleetUpdate`, `TestHandoffEnvNeverCarriesTheSecret`,
    `TestNeedsTerminalRoutesToInteractiveQueue`. Delete the `updateScript` wrapper. Commit: `feat(fleet/tui): interactive handoff is fleet update`.
26. **Flags + status text.** `--update-ref` → `WithRef`, `--file`; `TestTUIDemoWidthGuard` green.
    Commit: `feat(fleet/tui): --file and plan-aware status line`.
    **Leaf gate:** every `tui_*` test green; live TUI transcript → `evidence/tui/`.

### Leaf F — docs (consumes D; parallel with E)

27. AGENTS.md (command row, layout rows for `updplan`/`updexec`/`featflag`, invariants in §6),
    README (`fleet update` rewrite, `fleet.yaml` reference, local-changes table, retry/timeout
    table, manual restore one-liner), optional `opt/etc/fleet/fleet.yaml` sample + `.gitignore`
    `!`-rule, MBO index state. Commit: `docs(fleet): fleet.yaml update plans`.

## 5. Verification mapping

| Spec rule | Test(s) |
| :-- | :-- |
| F1 defaults / validation / merge | tasks 1, 2, 3 |
| F2 plan resolution | task 19; 17 |
| F3 one fetch, byte-compat, multi-branch, local policy | tasks 6, 7, 12 |
| F4 restore | task 13 |
| F5 run step, preamble scoping | tasks 7, 16, 23 |
| F6 gh-auth | tasks 7, 14 |
| F7 cascade / retry / timeout | tasks 11, 15, 9 |
| F8 CLI | tasks 20, 21, 22 |
| F9 gff | tasks 17, 18 |
| F10 run log | tasks 10, 23 |
| F11 TUI | tasks 24–26 |
| G1–G9 human gates | leaf D/E gates, `evidence/e2e/` |

## 6. Integration & rollout

- Build/test discovery is by directory under `sdk/`; nothing to register. `install.sh` already
  builds `gff` before `fleet`; the `replace ../gff` only needs the source tree.
- Rollout: A + B + C (no user-visible change) → D (CLI; default plan identical; `fleet update
  init` available) → E (TUI) → F (docs). Between D and E the TUI compiles against the thin
  `updateScript` wrapper.
- New invariants to pin in `sdk/fleet/AGENTS.md` (each naming its test): one unconditional
  network call per sync step; a failed step blocks dependents, never siblings; gh-auth checks
  before it prompts and never forwards a token; `run:` is verbatim so the plan file is
  executable config (ownership/mode check, `--dry-run` shows the wire); the built-in plan is
  today's update byte for byte; gff is fail-open here; the TUI's interactive lane is the CLI
  verb; a dirty clone is skipped by default, `rescue` commits work aside, `carry` stashes and
  re-applies, nothing is ever dropped; a host on another branch is put back by default under
  every policy; restore is cleanup, not a dependent; interactive steps never auto-retry; a
  timeout kills the local ssh, not the remote job. Amend the "never `stash@{0}`" text to say why
  `push -u` + `apply <sha>` is safe where `branch <n> stash@{0}` was not (licensed by G7).

### 6.1 Build leaves / DAG (authoritative — mirrored to the design issue and gss bases)

| Leaf | Owns (paths) | Consumes (in-edges) | `done-when` gate | Blocking? |
| :-- | :-- | :-- | :-- | :-- |
| A `updplan` | `sdk/fleet/internal/updplan/**` | — | tasks 1–5; `go test -race -cover ./internal/updplan` ≥ 90 % | yes (base) |
| B `updexec` (+ runner ctx) | `sdk/fleet/internal/updexec/**`, `sdk/fleet/internal/runner/**` | A (§3.2 types) | tasks 6–16; ≥ 90 % updexec; moved invariant tests green; mux invariant green | yes (D, E) |
| C `featflag` + deps | `sdk/fleet/internal/featflag/**`, `sdk/fleet/go.mod`, `go.sum`, `.github/gff/features.yaml` | — | tasks 17–18; `gff lint` clean; `go build ./...` | yes (D) |
| D `cmd` CLI | `sdk/fleet/cmd/update*.go`, `answers_store.go`, `status.go` | A, B, C (§3.2–3.5) | tasks 19–23; `go test -race ./...`; live `--dry-run` + one live host run | yes (E, F) |
| E TUI | `sdk/fleet/cmd/tui*.go`, `runlog_test.go` | D | tasks 24–26; all `tui_*` tests green; live TUI transcript | no |
| F docs | `sdk/fleet/{AGENTS,README}.md`, `opt/etc/fleet/**`, `docs/mbo/**fleet-update**`, `index.md` | D | task 27; links resolve | no |

```
A ──► B ──┐
          ├──► D ──► E
C ────────┘     └──► F
```

## 7. Validation & evidence (show the work)

Coverage: `updplan`, `updexec`, `featflag` ≥ 90 % each (asserted in each leaf gate and
recorded); `cmd` not below its pre-change figure (record before/after); repo floor `fleet=60`
unchanged. Evidence tree `docs/mbo/plans/fleet-update/evidence/{updplan,updexec,featflag,cmd,tui,docs,e2e,demo}/`,
one file per task (`task-NN.txt`, dated header, append-only), plus `e2e/G1..G9` transcripts and
`demo/` (the README console demo must be real output). Adversarial scenarios: plan file with
`run: "; rm -rf ~"` (verbatim — shown by `--dry-run`, refused only by the ownership/mode
check when the file is not the user's), branch `main;id` (rejected), a host whose `origin/HEAD`
is unset (default fallback path exercised once), an `ssh` stub exiting 255 twice (retry), a
stub that never exits (timeout), and a carry whose restore conflicts (stash kept).

> Produced via `superpowers:writing-plans` (mbo-plan). Execute with the trio in
> `./fleet-update/`, TDD throughout. Update `../index.md` state as it moves.
