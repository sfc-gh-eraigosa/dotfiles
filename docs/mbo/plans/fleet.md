# fleet — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: use `superpowers:subagent-driven-development`
> (recommended) or `superpowers:executing-plans` to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax; the live cursor is [`fleet/TODO.md`](./fleet/TODO.md).

- **Slug:** fleet
- **Date:** 2026-08-09
- **Status:** Draft
- **Relates to:** spec [`../specs/fleet.md`](../specs/fleet.md) · design [`../designs/fleet.md`](../designs/fleet.md) · issue [#222](https://github.com/sfc-gh-eraigosa/dotfiles/issues/222) · PR [#223](https://github.com/sfc-gh-eraigosa/dotfiles/pull/223)

**Goal:** ship `fleet`, a Go CLI that reports which hosts are out of sync with the latest
dotfiles install, updates the stale ones interactively, and manages fleet membership and
access keys.

**Architecture:** a cobra CLI at `sdk/fleet` whose classification, config-editing and
key-diff logic lives in pure `internal/` packages (no SSH, no clock, no git), so the entire
decision surface is unit-testable; plus a ~10-line stamp block appended to `install.sh`.

**Tech Stack:** Go (pinned by `.go-version`), cobra v1.10.2, Bubble Tea (TUI), `ssh`/`scp`
shelled out via an injected runner interface.

---

## 1. Summary & verdict

Builds the four capability groups in spec §4 (13 features): **status** (F1–F5), **TUI +
update** (F6–F8), **membership** (F9–F10), **keys** (F11–F13). Key logic is pure and tested;
everything that touches a remote machine goes through one injected `Runner` seam so tests
never open a socket.

Two decisions from the design carry directly into this plan and must not be quietly relaxed
during the build:

1. **Public-key-only distribution.** No task copies a private key to a host. If a step seems
   to need it, that is a contract defect — escalate via `TRACKING.md` blockers.
2. **Diff-first destructive key ops.** `prune`/`delete` compute a diff and stop for
   confirmation. No task may blanket-overwrite `authorized_keys`.

`src/ssh-key-sync/` is retired **only** in Task 15, and only after the parity capture.

## 2. File inventory

| Path | Purpose | Implements |
| :-- | :-- | :-- |
| `sdk/fleet/go.mod` · `go.sum` | module `github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet` | scaffold |
| `sdk/fleet/main.go` | `main()` → `cmd.Execute()` | scaffold |
| `sdk/fleet/VERSION` | semver string read by `build.sh` ldflags | scaffold |
| `sdk/fleet/build.sh` | goenv-aware build → `~/opt/bin/fleet` (copy of `sdk/wol/build.sh`) | scaffold |
| `sdk/fleet/AGENTS.md` + `CLAUDE.md`→symlink | per-dir context node | repo convention |
| `sdk/fleet/cmd/root.go` | root cobra cmd, global flags (`--config`, `--marker`, `--repo`, `--json`) | scaffold |
| `sdk/fleet/cmd/version.go` | `fleet version` (ldflags) | scaffold |
| `sdk/fleet/internal/sshconf/sshconf.go` | parse `~/.ssh/config` → `[]Host`; **writer**: add/update/unmark/purge + render | F2, F9, F10 |
| `sdk/fleet/internal/sshconf/sshconf_test.go` | table + golden-file round-trip tests | F2, F9, F10 |
| `sdk/fleet/internal/stamp/stamp.go` | parse stamp text → `Stamp` | F1 read side |
| `sdk/fleet/internal/stamp/stamp_test.go` | well-formed / truncated / empty | F1 |
| `sdk/fleet/internal/drift/drift.go` | `Classify` + `FormatAge(now, then)` | F4 |
| `sdk/fleet/internal/drift/drift_test.go` | all five classes + age table | F4 |
| `sdk/fleet/internal/runner/runner.go` | `Runner` iface (`Run`, `RunInteractive`) + `Exec` impl + `Fake` for tests | test seam |
| `sdk/fleet/internal/keys/keys.go` | managed-key discovery + `Compute(local, remote) Diff` | F11–F13 |
| `sdk/fleet/internal/keys/keys_test.go` | diff table incl. the defect-2 regression | F13 |
| `sdk/fleet/cmd/status.go` | fan-out, classify, render table/JSON, exit code | F3, F4, F5 |
| `sdk/fleet/cmd/status_test.go` | golden table + JSON, exit-code cases | F3, F5 |
| `sdk/fleet/cmd/add.go` · `remove.go` | membership verbs (`--dry-run`, `--purge`) | F9, F10 |
| `sdk/fleet/cmd/keys.go` | `keys list\|sync\|prune\|delete` | F11–F13 |
| `sdk/fleet/cmd/tui.go` | Bubble Tea model, rows, `u` → `tea.Exec` handoff | F6, F7 |
| `sdk/fleet/cmd/update.go` | headless `fleet update <host...>`; dirty-clone policy | F7, F8 |
| **Outside the module** | | |
| `install.sh` (end of file) | stamp block, `INSTALL_PHASE=all`-gated | F1 |
| `install.sh` (sdk section, near the `wol` block ~line 578) | `gff_on install.sdk.fleet` build block | rollout |
| `.github/gff/features.yaml` (~line 99) | `install.sdk.fleet` flag, `boolDefault: true`; bump `# --- sdk (5) ---` → `(6)` | rollout |
| `Makefile` (~line 108) | add `fleet` to the `for t in gss gsl wol tmux-mgr` loop | rollout |
| `scripts/test.sh` `coverage_min()` | add `fleet) echo 60 ;;` — without it the module is silently exempt | quality gate |
| `install_test.sh` | shell test for the stamp block (phase gating) | F1 |
| `docs/mbo/index.md` | state `specifying` → `building` → `in-review` | MBO |
| `src/ssh-key-sync/` (Task 15 only) | retired after parity evidence | migration |

## 3. Interface contracts

Frozen once Task 1–3 land; downstream leaves compile against these.

```go
// internal/sshconf
type Host struct {
    Alias, HostName, User, Port, Identity string
    Fleet bool // block carries the marker
}
func Parse(cfg, marker string) ([]Host, error)     // read
func Add(cfg string, h Host, marker string) (string, error)   // idempotent upsert
func Unmark(cfg, alias, marker string) (string, error)        // remove marker, keep block
func Purge(cfg, alias string) (string, error)                 // remove whole block

// internal/stamp
type Stamp struct {
    Commit, Branch, Hostname string
    InstalledAt time.Time
}
func Parse(s string) (Stamp, error)

// internal/drift
type Class string
const (
    UpToDate  Class = "up-to-date"
    Behind    Class = "behind"
    Divergent Class = "ahead/divergent"
    Unknown   Class = "unknown"
    Unreachable Class = "unreachable"
)
type Input struct {
    Reachable, HaveStamp, IsAncestor bool
    Commit, Baseline string
    BehindCount int
}
type Result struct { Class Class; Behind int }
func Classify(in Input) Result
func FormatAge(now, then time.Time) string

// internal/runner  — the only seam that touches the network
type Runner interface {
    Run(host string, argv ...string) (stdout string, err error)
    RunInteractive(host string, argv ...string) error   // inherits the terminal
}

// internal/keys
type Diff struct { ToAdd, ToRemove []string }
func Compute(local, remote []string) Diff
```

**CLI surface** (stdout contracts; `--json` everywhere it makes sense):

```
fleet status [host...] [--json]        # exit 1 if any host != up-to-date
fleet tui                              # default on a TTY
fleet update <host...> [--force]
fleet add <alias> --hostname H [--user U] [--port P] [--identity F] [--dry-run]
fleet remove <alias> [--purge] [--dry-run]
fleet keys list|sync [name...]|prune|delete <name> [--yes]
fleet version
```

## 4. TDD build order

Every task: **tests first**, run RED, implement, run GREEN, gate, capture evidence, commit.
Evidence path: `docs/mbo/plans/fleet/evidence/<task>/`.

---

### Task 1: Module scaffold + repo wiring  *(leaf: `scaffold`, BLOCKING)*

**Files:** Create `sdk/fleet/{go.mod,main.go,VERSION,build.sh,AGENTS.md}`,
`sdk/fleet/cmd/{root.go,version.go,version_test.go}`;
Modify `Makefile:108`, `scripts/test.sh` `coverage_min()`, `.github/gff/features.yaml`,
`install.sh` (sdk block).

- [ ] **Step 1: Write the failing test**

```go
// sdk/fleet/cmd/version_test.go
package cmd

import "testing"

func TestVersionStringIncludesVersionAndCommit(t *testing.T) {
    version, commit = "9.9.9", "abc1234"
    got := versionString()
    want := "fleet 9.9.9 (abc1234)"
    if got != want {
        t.Fatalf("versionString() = %q, want %q", got, want)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd sdk/fleet && go test ./cmd/ -run TestVersionString -v`
Expected: FAIL — `undefined: versionString`.

- [ ] **Step 3: Write minimal implementation**

```go
// sdk/fleet/cmd/version.go
package cmd

import (
    "fmt"
    "github.com/spf13/cobra"
)

var (
    version = "dev"
    commit  = "none"
    date    = "unknown"
)

func versionString() string { return fmt.Sprintf("fleet %s (%s)", version, commit) }

var versionCmd = &cobra.Command{
    Use:   "version",
    Short: "Print the fleet version",
    Run:   func(cmd *cobra.Command, _ []string) { fmt.Fprintln(cmd.OutOrStdout(), versionString()) },
}

func init() { rootCmd.AddCommand(versionCmd) }
```

```go
// sdk/fleet/cmd/root.go
package cmd

import (
    "os"
    "path/filepath"
    "github.com/spf13/cobra"
)

var (
    flagConfig string
    flagMarker string
    flagRepo   string
    flagJSON   bool
)

var rootCmd = &cobra.Command{
    Use:   "fleet",
    Short: "Report and manage dotfiles install status across your hosts",
}

func Execute() {
    if err := rootCmd.Execute(); err != nil {
        os.Exit(1)
    }
}

func init() {
    home, _ := os.UserHomeDir()
    rootCmd.PersistentFlags().StringVar(&flagConfig, "config", filepath.Join(home, ".ssh", "config"), "ssh config path")
    rootCmd.PersistentFlags().StringVar(&flagMarker, "marker", "#fleet", "comment marking a host as in-fleet")
    rootCmd.PersistentFlags().StringVar(&flagRepo, "repo", filepath.Join(home, "git", "dotfiles"), "local dotfiles repo for the baseline")
    rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "machine-readable output")
}
```

```go
// sdk/fleet/main.go
package main

import "github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/cmd"

func main() { cmd.Execute() }
```

`VERSION` contains `0.1.0`. `build.sh` is `sdk/wol/build.sh` with `wol` → `fleet`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd sdk/fleet && go test ./... -v`
Expected: PASS.

- [ ] **Step 5: Wire the repo**

`Makefile:108` → `@for t in gss gsl wol tmux-mgr fleet; do \`
`scripts/test.sh` `coverage_min()` → add `fleet)    echo 60 ;;` above the `*)` arm.
`.github/gff/features.yaml` after the `install.sdk.gsl` entry:

```yaml
      - path: install.sdk.fleet
        description: Runs the fleet build-and-install block.
        boolDefault: true
```

and change `# --- sdk (5) ---` to `# --- sdk (6) ---`.
`install.sh`, immediately after the `wol` block:

```bash
# build and install fleet
if gff_on install.sdk.fleet; then
  if [ -f "${BASE_DIR}/sdk/fleet/build.sh" ]; then
    echo "Installing fleet (dotfiles install-status checker)..."
    bash "${BASE_DIR}/sdk/fleet/build.sh"
    if [ -f "${HOME}/opt/bin/fleet" ]; then
      "${HOME}/opt/bin/fleet" version
    fi
  fi
else gff_skip_msg install.sdk.fleet; fi
```

- [ ] **Step 6: Verify the wiring**

Run: `bash -n install.sh && ./scripts/test.sh 2>&1 | grep -i fleet | tee docs/mbo/plans/fleet/evidence/task01/wiring.txt`
Expected: `bash -n` silent; test.sh discovers the `fleet` module and prints its coverage line.

- [ ] **Step 7: Commit**

```bash
git add sdk/fleet Makefile scripts/test.sh .github/gff/features.yaml install.sh docs/mbo/plans/fleet/evidence/task01
git commit -m "feat(fleet): module scaffold + build/test/install wiring"
```

**Done when:** `go test ./...` passes in `sdk/fleet`, `./scripts/test.sh` lists `fleet`, and
`bash -n install.sh` is clean.

---

### Task 2: `install.sh` stamp  *(leaf: `stamp-sh`, independent)*

**Files:** Modify `install.sh` (end of file); Test: `install_test.sh`.

- [ ] **Step 1: Write the failing test**

```bash
# install_test.sh — append
test_stamp_written_only_for_phase_all() {
  tmp="$(mktemp -d)"; export HOME="$tmp"; mkdir -p "$tmp/.local/state/dotfiles"
  BASE_DIR="$PWD" INSTALL_PHASE=deps bash -c 'source ./install.sh.stampblock' 2>/dev/null || true
  [ ! -f "$tmp/.local/state/dotfiles/install-stamp" ] || { echo "FAIL: stamped during deps phase"; return 1; }
  BASE_DIR="$PWD" INSTALL_PHASE=all bash -c 'source ./install.sh.stampblock'
  grep -q '^commit=[0-9a-f]\{40\}$' "$tmp/.local/state/dotfiles/install-stamp" || { echo "FAIL: no 40-char commit"; return 1; }
  grep -q '^installed_at=[0-9]\{10\}' "$tmp/.local/state/dotfiles/install-stamp" || { echo "FAIL: no epoch"; return 1; }
  echo "PASS: stamp phase gating"
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bash install_test.sh 2>&1 | grep stamp`
Expected: FAIL — the stamp block does not exist yet.

- [ ] **Step 3: Write minimal implementation**

Appended as the **last** executable block of `install.sh` (extracted to
`install.sh.stampblock` for the test to source, and sourced back by `install.sh`):

```bash
# --- install stamp (fleet) -------------------------------------------------
# Recorded LAST so an aborted run leaves no false success marker, and only for
# a full run — a Docker `--phase deps|config` layer must never stamp.
if [ "${INSTALL_PHASE:-all}" = "all" ]; then
  _stamp_dir="${HOME}/.local/state/dotfiles"
  _stamp_commit="$(git -C "${BASE_DIR}" rev-parse HEAD 2>/dev/null || true)"
  if [ -n "${_stamp_commit}" ]; then
    mkdir -p "${_stamp_dir}"
    {
      echo "commit=${_stamp_commit}"
      echo "installed_at=$(date -u +%s)"
      echo "branch=$(git -C "${BASE_DIR}" rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)"
      echo "hostname=$(uname -n)"
    } > "${_stamp_dir}/install-stamp"
  fi
  unset _stamp_dir _stamp_commit
fi
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bash install_test.sh 2>&1 | tee docs/mbo/plans/fleet/evidence/task02/stamp.txt | grep stamp`
Expected: `PASS: stamp phase gating`.

- [ ] **Step 5: Commit**

```bash
git add install.sh install.sh.stampblock install_test.sh docs/mbo/plans/fleet/evidence/task02
git commit -m "feat(install): record an install stamp for fleet (phase-gated)"
```

**Done when:** the test passes and `bash -n install.sh` is clean.

---

### Task 3: `sshconf` reader  *(leaf: `sshconf`, BLOCKING for 4/8/9/10/11)*

**Files:** Create `sdk/fleet/internal/sshconf/{sshconf.go,sshconf_test.go}`,
`sdk/fleet/internal/sshconf/testdata/basic.cfg`.

- [ ] **Step 1: Write the failing test**

```go
// sdk/fleet/internal/sshconf/sshconf_test.go
package sshconf

import "testing"

const cfg = `Host alpha  # fleet
    HostName 10.0.0.1
    User ops

Host beta
    HostName 10.0.0.2

Host *
    ServerAliveInterval 60

Host gamma
    # fleet
    HostName 10.0.0.3
    Port 2222
`

func TestParseReturnsOnlyMarkedConcreteHosts(t *testing.T) {
    hosts, err := Parse(cfg, "#fleet")
    if err != nil { t.Fatalf("Parse: %v", err) }
    var got []string
    for _, h := range hosts { if h.Fleet { got = append(got, h.Alias) } }
    want := []string{"alpha", "gamma"}
    if len(got) != len(want) { t.Fatalf("marked hosts = %v, want %v", got, want) }
    for i := range want { if got[i] != want[i] { t.Fatalf("marked hosts = %v, want %v", got, want) } }
}

func TestParseSkipsPatternHostsEntirely(t *testing.T) {
    hosts, _ := Parse(cfg, "#fleet")
    for _, h := range hosts { if h.Alias == "*" { t.Fatal("pattern host must not be returned") } }
}

func TestParseCapturesFields(t *testing.T) {
    hosts, _ := Parse(cfg, "#fleet")
    var g Host
    for _, h := range hosts { if h.Alias == "gamma" { g = h } }
    if g.HostName != "10.0.0.3" || g.Port != "2222" {
        t.Fatalf("gamma = %+v, want HostName 10.0.0.3 Port 2222", g)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd sdk/fleet && go test ./internal/sshconf/ -v`
Expected: FAIL — `undefined: Parse`.

- [ ] **Step 3: Write minimal implementation**

```go
// sdk/fleet/internal/sshconf/sshconf.go
package sshconf

import (
    "strings"
)

type Host struct {
    Alias, HostName, User, Port, Identity string
    Fleet bool
}

// Parse returns every concrete (non-pattern) Host block. Fleet is true when the
// marker appears on the Host line or on any line inside the block.
func Parse(cfg, marker string) ([]Host, error) {
    var out []Host
    var cur *Host
    flush := func() {
        if cur != nil && !strings.ContainsAny(cur.Alias, "*?") {
            out = append(out, *cur)
        }
        cur = nil
    }
    for _, line := range strings.Split(cfg, "\n") {
        t := strings.TrimSpace(line)
        if strings.HasPrefix(strings.ToLower(t), "host ") {
            flush()
            rest := strings.TrimSpace(t[5:])
            marked := false
            if i := strings.Index(rest, "#"); i >= 0 {
                marked = strings.Contains(strings.ReplaceAll(rest[i:], " ", ""), strings.ReplaceAll(marker, " ", ""))
                rest = strings.TrimSpace(rest[:i])
            }
            alias := rest
            if f := strings.Fields(rest); len(f) > 0 { alias = f[0] }
            cur = &Host{Alias: alias, Fleet: marked}
            continue
        }
        if cur == nil { continue }
        if strings.HasPrefix(t, "#") {
            if strings.Contains(strings.ReplaceAll(t, " ", ""), strings.ReplaceAll(marker, " ", "")) {
                cur.Fleet = true
            }
            continue
        }
        f := strings.Fields(t)
        if len(f) < 2 { continue }
        switch strings.ToLower(f[0]) {
        case "hostname":     cur.HostName = f[1]
        case "user":         cur.User = f[1]
        case "port":         cur.Port = f[1]
        case "identityfile": cur.Identity = f[1]
        }
    }
    flush()
    return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd sdk/fleet && go test ./internal/sshconf/ -v -cover | tee ../../docs/mbo/plans/fleet/evidence/task03/parse.txt`
Expected: PASS, 3 tests.

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/internal/sshconf docs/mbo/plans/fleet/evidence/task03
git commit -m "feat(fleet): parse ~/.ssh/config for marked fleet hosts"
```

**Done when:** all three tests pass; pattern hosts and unmarked hosts provably excluded.

---

### Task 4: `sshconf` writer (add / unmark / purge)  *(leaf: `sshconf`)*

**Files:** Modify `sdk/fleet/internal/sshconf/sshconf.go`, `sshconf_test.go`.

- [ ] **Step 1: Write the failing test**

```go
func TestAddIsIdempotentAndPreservesOtherBlocks(t *testing.T) {
    base := "Host beta\n    HostName 10.0.0.2\n"
    once, err := Add(base, Host{Alias: "alpha", HostName: "10.0.0.1"}, "#fleet")
    if err != nil { t.Fatalf("Add: %v", err) }
    twice, err := Add(once, Host{Alias: "alpha", HostName: "10.0.0.1"}, "#fleet")
    if err != nil { t.Fatalf("Add twice: %v", err) }
    if once != twice { t.Fatalf("Add not idempotent:\n--- once ---\n%s\n--- twice ---\n%s", once, twice) }
    if !strings.Contains(once, "Host beta") || !strings.Contains(once, "HostName 10.0.0.2") {
        t.Fatal("unrelated block was modified")
    }
    hosts, _ := Parse(once, "#fleet")
    var found bool
    for _, h := range hosts { if h.Alias == "alpha" && h.Fleet { found = true } }
    if !found { t.Fatal("added host is not marked in-fleet") }
}

func TestUnmarkKeepsBlockButLeavesFleet(t *testing.T) {
    cfg, _ := Add("", Host{Alias: "alpha", HostName: "10.0.0.1"}, "#fleet")
    out, err := Unmark(cfg, "alpha", "#fleet")
    if err != nil { t.Fatalf("Unmark: %v", err) }
    if !strings.Contains(out, "Host alpha") { t.Fatal("block was deleted; Unmark must keep it") }
    hosts, _ := Parse(out, "#fleet")
    for _, h := range hosts { if h.Alias == "alpha" && h.Fleet { t.Fatal("still marked after Unmark") } }
}

func TestPurgeRemovesOnlyTheTargetBlock(t *testing.T) {
    cfg := "Host alpha  # fleet\n    HostName 10.0.0.1\n\nHost beta\n    HostName 10.0.0.2\n"
    out, err := Purge(cfg, "alpha")
    if err != nil { t.Fatalf("Purge: %v", err) }
    if strings.Contains(out, "Host alpha") { t.Fatal("target block survived Purge") }
    if !strings.Contains(out, "Host beta") || !strings.Contains(out, "HostName 10.0.0.2") {
        t.Fatal("Purge removed an unrelated block")
    }
}

func TestUnknownAliasIsAnError(t *testing.T) {
    if _, err := Unmark("Host beta\n", "nope", "#fleet"); err == nil {
        t.Fatal("expected an error for an unknown alias")
    }
}
```

Add `import "strings"` to the test file.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd sdk/fleet && go test ./internal/sshconf/ -run 'TestAdd|TestUnmark|TestPurge|TestUnknown' -v`
Expected: FAIL — `undefined: Add`.

- [ ] **Step 3: Write minimal implementation**

```go
// appended to sdk/fleet/internal/sshconf/sshconf.go
import "fmt"

func blockRange(cfg, alias string) (start, end int, lines []string, ok bool) {
    lines = strings.Split(cfg, "\n")
    start = -1
    for i, l := range lines {
        t := strings.TrimSpace(l)
        if !strings.HasPrefix(strings.ToLower(t), "host ") { continue }
        rest := strings.TrimSpace(t[5:])
        if i := strings.Index(rest, "#"); i >= 0 { rest = strings.TrimSpace(rest[:i]) }
        name := rest
        if f := strings.Fields(rest); len(f) > 0 { name = f[0] }
        if start >= 0 { return start, i, lines, true }   // next Host ends the block
        if name == alias { start = i }
    }
    if start >= 0 { return start, len(lines), lines, true }
    return 0, 0, lines, false
}

func Add(cfg string, h Host, marker string) (string, error) {
    if h.Alias == "" { return "", fmt.Errorf("alias is required") }
    if _, _, _, exists := blockRange(cfg, h.Alias); exists {
        out, err := Purge(cfg, h.Alias)
        if err != nil { return "", err }
        cfg = out
    }
    var b strings.Builder
    b.WriteString(strings.TrimRight(cfg, "\n"))
    if strings.TrimSpace(cfg) != "" { b.WriteString("\n\n") }
    fmt.Fprintf(&b, "Host %s  %s\n", h.Alias, marker)
    if h.HostName != "" { fmt.Fprintf(&b, "    HostName %s\n", h.HostName) }
    if h.User != ""     { fmt.Fprintf(&b, "    User %s\n", h.User) }
    if h.Port != ""     { fmt.Fprintf(&b, "    Port %s\n", h.Port) }
    if h.Identity != "" { fmt.Fprintf(&b, "    IdentityFile %s\n", h.Identity) }
    return b.String(), nil
}

func Unmark(cfg, alias, marker string) (string, error) {
    start, end, lines, ok := blockRange(cfg, alias)
    if !ok { return "", fmt.Errorf("host %q not found in ssh config", alias) }
    strip := strings.ReplaceAll(marker, " ", "")
    for i := start; i < end && i < len(lines); i++ {
        t := strings.TrimSpace(lines[i])
        if i == start {
            if j := strings.Index(lines[i], "#"); j >= 0 &&
                strings.Contains(strings.ReplaceAll(lines[i][j:], " ", ""), strip) {
                lines[i] = strings.TrimRight(lines[i][:j], " \t")
            }
            continue
        }
        if strings.HasPrefix(t, "#") && strings.Contains(strings.ReplaceAll(t, " ", ""), strip) {
            lines[i] = ""
        }
    }
    return strings.Join(lines, "\n"), nil
}

func Purge(cfg, alias string) (string, error) {
    start, end, lines, ok := blockRange(cfg, alias)
    if !ok { return "", fmt.Errorf("host %q not found in ssh config", alias) }
    out := append([]string{}, lines[:start]...)
    out = append(out, lines[end:]...)
    return strings.Join(out, "\n"), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd sdk/fleet && go test ./internal/sshconf/ -v -cover | tee ../../docs/mbo/plans/fleet/evidence/task04/writer.txt`
Expected: PASS, all tests.

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/internal/sshconf docs/mbo/plans/fleet/evidence/task04
git commit -m "feat(fleet): idempotent ssh-config add/unmark/purge"
```

**Done when:** idempotency, unmark-keeps-block, purge-only-target, and unknown-alias-errors
all pass.

---

### Task 5: `stamp` parser  *(leaf: `drift`)*

**Files:** Create `sdk/fleet/internal/stamp/{stamp.go,stamp_test.go}`.

- [ ] **Step 1: Write the failing test**

```go
package stamp

import "testing"

func TestParseWellFormed(t *testing.T) {
    in := "commit=" + repeat("a", 40) + "\ninstalled_at=1754700000\nbranch=main\nhostname=box\n"
    s, err := Parse(in)
    if err != nil { t.Fatalf("Parse: %v", err) }
    if s.Branch != "main" || s.Hostname != "box" { t.Fatalf("got %+v", s) }
    if s.InstalledAt.Unix() != 1754700000 { t.Fatalf("InstalledAt = %v", s.InstalledAt) }
}

func TestParseEmptyIsError(t *testing.T) {
    if _, err := Parse(""); err == nil { t.Fatal("expected error for empty stamp") }
}

func TestParseTruncatedIsError(t *testing.T) {
    if _, err := Parse("commit=abc\n"); err == nil {
        t.Fatal("expected error for a short commit / missing installed_at")
    }
}

// repeat — deliberately not named `strings`, which would shadow the package.
func repeat(c string, n int) string {
    out := ""
    for i := 0; i < n; i++ { out += c }
    return out
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd sdk/fleet && go test ./internal/stamp/ -v`
Expected: FAIL — `undefined: Parse`.

- [ ] **Step 3: Write minimal implementation**

```go
package stamp

import (
    "fmt"
    "strconv"
    "strings"
    "time"
)

type Stamp struct {
    Commit, Branch, Hostname string
    InstalledAt              time.Time
}

func Parse(s string) (Stamp, error) {
    var out Stamp
    kv := map[string]string{}
    for _, line := range strings.Split(s, "\n") {
        line = strings.TrimSpace(line)
        i := strings.Index(line, "=")
        if i <= 0 { continue }
        kv[line[:i]] = line[i+1:]
    }
    out.Commit = kv["commit"]
    if len(out.Commit) != 40 {
        return Stamp{}, fmt.Errorf("stamp: commit must be a 40-character sha, got %d chars", len(out.Commit))
    }
    epoch, err := strconv.ParseInt(kv["installed_at"], 10, 64)
    if err != nil { return Stamp{}, fmt.Errorf("stamp: bad installed_at: %w", err) }
    out.InstalledAt = time.Unix(epoch, 0).UTC()
    out.Branch, out.Hostname = kv["branch"], kv["hostname"]
    return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd sdk/fleet && go test ./internal/stamp/ -v -cover | tee ../../docs/mbo/plans/fleet/evidence/task05/stamp.txt`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/internal/stamp docs/mbo/plans/fleet/evidence/task05
git commit -m "feat(fleet): parse the install stamp"
```

**Done when:** well-formed parses; empty and truncated both error.

---

### Task 6: `drift` classify + age  *(leaf: `drift`)*

**Files:** Create `sdk/fleet/internal/drift/{drift.go,drift_test.go}`.

- [ ] **Step 1: Write the failing test**

```go
package drift

import (
    "testing"
    "time"
)

func TestClassifyAllFiveClasses(t *testing.T) {
    cases := []struct{
        name string
        in   Input
        want Class
    }{
        {"unreachable", Input{Reachable: false}, Unreachable},
        {"no stamp", Input{Reachable: true, HaveStamp: false}, Unknown},
        {"equal", Input{Reachable: true, HaveStamp: true, Commit: "aaa", Baseline: "aaa", IsAncestor: true}, UpToDate},
        {"behind", Input{Reachable: true, HaveStamp: true, Commit: "bbb", Baseline: "aaa", IsAncestor: true, BehindCount: 24}, Behind},
        {"divergent", Input{Reachable: true, HaveStamp: true, Commit: "ccc", Baseline: "aaa", IsAncestor: false}, Divergent},
    }
    for _, c := range cases {
        if got := Classify(c.in); got.Class != c.want {
            t.Errorf("%s: Classify = %q, want %q", c.name, got.Class, c.want)
        }
    }
}

func TestClassifyNeverReportsBehindWhenCommitsMatch(t *testing.T) {
    got := Classify(Input{Reachable: true, HaveStamp: true, Commit: "aaa", Baseline: "aaa", IsAncestor: true, BehindCount: 7})
    if got.Class != UpToDate || got.Behind != 0 {
        t.Fatalf("identical commits must be up-to-date with 0 behind, got %+v", got)
    }
}

func TestFormatAge(t *testing.T) {
    now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
    cases := []struct{ then time.Time; want string }{
        {now.Add(-30 * time.Second), "just now"},
        {now.Add(-5 * time.Minute), "5m ago"},
        {now.Add(-3 * time.Hour), "3h ago"},
        {now.Add(-72 * time.Hour), "3d ago"},
        {now.Add(-21 * 24 * time.Hour), "3w ago"},
    }
    for _, c := range cases {
        if got := FormatAge(now, c.then); got != c.want {
            t.Errorf("FormatAge(%v) = %q, want %q", c.then, got, c.want)
        }
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd sdk/fleet && go test ./internal/drift/ -v`
Expected: FAIL — `undefined: Classify`.

- [ ] **Step 3: Write minimal implementation**

```go
package drift

import (
    "fmt"
    "time"
)

type Class string

const (
    UpToDate    Class = "up-to-date"
    Behind      Class = "behind"
    Divergent   Class = "ahead/divergent"
    Unknown     Class = "unknown"
    Unreachable Class = "unreachable"
)

type Input struct {
    Reachable, HaveStamp, IsAncestor bool
    Commit, Baseline                 string
    BehindCount                      int
}

type Result struct {
    Class  Class
    Behind int
}

func Classify(in Input) Result {
    switch {
    case !in.Reachable:
        return Result{Class: Unreachable}
    case !in.HaveStamp:
        return Result{Class: Unknown}
    case in.Commit == in.Baseline:
        return Result{Class: UpToDate}
    case !in.IsAncestor:
        return Result{Class: Divergent}
    default:
        return Result{Class: Behind, Behind: in.BehindCount}
    }
}

func FormatAge(now, then time.Time) string {
    d := now.Sub(then)
    switch {
    case d < time.Minute:
        return "just now"
    case d < time.Hour:
        return fmt.Sprintf("%dm ago", int(d.Minutes()))
    case d < 24*time.Hour:
        return fmt.Sprintf("%dh ago", int(d.Hours()))
    case d < 14*24*time.Hour:
        return fmt.Sprintf("%dd ago", int(d.Hours())/24)
    default:
        return fmt.Sprintf("%dw ago", int(d.Hours())/(24*7))
    }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd sdk/fleet && go test ./internal/drift/ -v -cover | tee ../../docs/mbo/plans/fleet/evidence/task06/drift.txt`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/internal/drift docs/mbo/plans/fleet/evidence/task06
git commit -m "feat(fleet): classify install drift and format age"
```

**Done when:** all five classes and the age table pass; identical commits never report behind.

---

### Task 7: `Runner` seam + `fleet status`  *(leaf: `status`; consumes sshconf, drift)*

**Files:** Create `sdk/fleet/internal/runner/runner.go`, `sdk/fleet/cmd/{status.go,status_test.go}`.

- [ ] **Step 1: Write the failing test**

```go
// sdk/fleet/cmd/status_test.go
package cmd

import (
    "strings"
    "testing"
    "time"
)

func TestRenderTableIsWorstFirst(t *testing.T) {
    now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
    rows := []Row{
        {Alias: "good", Class: "up-to-date", Commit: "0b8726e", Age: now.Add(-time.Hour)},
        {Alias: "stale", Class: "behind", Behind: 24, Commit: "1bc1928", Age: now.Add(-14 * 24 * time.Hour)},
        {Alias: "dead", Class: "unreachable"},
    }
    out := renderTable(rows, now)
    iDead, iStale, iGood := strings.Index(out, "dead"), strings.Index(out, "stale"), strings.Index(out, "good")
    if !(iDead < iStale && iStale < iGood) {
        t.Fatalf("rows not worst-first:\n%s", out)
    }
    if !strings.Contains(out, "behind 24") { t.Fatalf("missing behind count:\n%s", out) }
}

func TestExitCodeNonZeroWhenAnyHostStale(t *testing.T) {
    if exitCode([]Row{{Class: "up-to-date"}}) != 0 { t.Fatal("all up-to-date must exit 0") }
    if exitCode([]Row{{Class: "up-to-date"}, {Class: "behind"}}) == 0 { t.Fatal("a stale host must exit non-zero") }
    if exitCode([]Row{{Class: "unreachable"}}) == 0 { t.Fatal("an unreachable host must exit non-zero") }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd sdk/fleet && go test ./cmd/ -run 'TestRenderTable|TestExitCode' -v`
Expected: FAIL — `undefined: Row`.

- [ ] **Step 3: Write minimal implementation**

```go
// sdk/fleet/internal/runner/runner.go
package runner

import (
    "os"
    "os/exec"
    "strings"
)

type Runner interface {
    Run(host string, argv ...string) (string, error)
    RunInteractive(host string, argv ...string) error
}

type Exec struct{ Timeout string }

func (e Exec) Run(host string, argv ...string) (string, error) {
    a := []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=" + e.Timeout, host}
    out, err := exec.Command("ssh", append(a, argv...)...).Output()
    return strings.TrimSpace(string(out)), err
}

func (e Exec) RunInteractive(host string, argv ...string) error {
    c := exec.Command("ssh", append([]string{"-t", host}, argv...)...)
    c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
    return c.Run()
}

type Fake struct {
    Out map[string]string
    Err map[string]error
}

func (f Fake) Run(host string, _ ...string) (string, error) { return f.Out[host], f.Err[host] }
func (f Fake) RunInteractive(string, ...string) error       { return nil }
```

```go
// sdk/fleet/cmd/status.go  (rendering + exit code; wiring omitted for brevity is NOT allowed —
// the full command is implemented here)
package cmd

import (
    "fmt"
    "sort"
    "strings"
    "time"

    "github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/drift"
)

type Row struct {
    Alias  string
    Class  string
    Behind int
    Commit string
    Age    time.Time
}

var severity = map[string]int{
    "unreachable": 0, "ahead/divergent": 1, "unknown": 2, "behind": 3, "up-to-date": 4,
}

func renderTable(rows []Row, now time.Time) string {
    sort.SliceStable(rows, func(i, j int) bool { return severity[rows[i].Class] < severity[rows[j].Class] })
    var b strings.Builder
    fmt.Fprintf(&b, "%-16s %-9s %-13s %s\n", "HOST", "COMMIT", "LAST RUN", "STATUS")
    for _, r := range rows {
        status := r.Class
        if r.Class == string(drift.Behind) { status = fmt.Sprintf("behind %d", r.Behind) }
        age, commit := "-", "-"
        if !r.Age.IsZero() { age = drift.FormatAge(now, r.Age) }
        if r.Commit != "" { commit = r.Commit }
        fmt.Fprintf(&b, "%-16s %-9s %-13s %s\n", r.Alias, commit, age, status)
    }
    return b.String()
}

func exitCode(rows []Row) int {
    for _, r := range rows {
        if r.Class != "up-to-date" { return 1 }
    }
    return 0
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd sdk/fleet && go test ./cmd/ -v -cover | tee ../../docs/mbo/plans/fleet/evidence/task07/status.txt`
Expected: PASS.

- [ ] **Step 5: Live capture (the headline proof)**

Run: `fleet status | tee docs/mbo/plans/fleet/evidence/task07/live-status.txt`
Expected: one row per marked host; the Jetson host shows `behind N`.

- [ ] **Step 6: Commit**

```bash
git add sdk/fleet docs/mbo/plans/fleet/evidence/task07
git commit -m "feat(fleet): status command with worst-first table and stale exit code"
```

**Done when:** unit tests pass **and** a real multi-host capture exists.

---

### Task 8: `fleet add` / `fleet remove`  *(leaf: `membership`; consumes sshconf writer)*

**Files:** Create `sdk/fleet/cmd/{add.go,remove.go,membership_test.go}`.

- [ ] **Step 1: Write the failing test**

```go
package cmd

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestWriteConfigTakesBackupFirst(t *testing.T) {
    dir := t.TempDir()
    p := filepath.Join(dir, "config")
    if err := os.WriteFile(p, []byte("Host beta\n    HostName 10.0.0.2\n"), 0o600); err != nil { t.Fatal(err) }
    if err := writeConfig(p, "Host beta\n    HostName 10.0.0.9\n"); err != nil { t.Fatalf("writeConfig: %v", err) }
    entries, _ := os.ReadDir(dir)
    var backups int
    for _, e := range entries { if strings.HasPrefix(e.Name(), "config.bak-") { backups++ } }
    if backups != 1 { t.Fatalf("expected exactly 1 backup, got %d", backups) }
    got, _ := os.ReadFile(p)
    if !strings.Contains(string(got), "10.0.0.9") { t.Fatal("new content not written") }
}

func TestDryRunWritesNothing(t *testing.T) {
    dir := t.TempDir()
    p := filepath.Join(dir, "config")
    orig := "Host beta\n    HostName 10.0.0.2\n"
    os.WriteFile(p, []byte(orig), 0o600)
    if err := applyConfig(p, "Host beta\n    HostName 10.0.0.9\n", true); err != nil { t.Fatal(err) }
    got, _ := os.ReadFile(p)
    if string(got) != orig { t.Fatal("--dry-run modified the file") }
    entries, _ := os.ReadDir(dir)
    if len(entries) != 1 { t.Fatalf("--dry-run created files: %v", entries) }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd sdk/fleet && go test ./cmd/ -run 'TestWriteConfig|TestDryRun' -v`
Expected: FAIL — `undefined: writeConfig`.

- [ ] **Step 3: Write minimal implementation**

```go
// sdk/fleet/cmd/add.go
package cmd

import (
    "fmt"
    "os"
    "time"

    "github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
    "github.com/spf13/cobra"
)

func writeConfig(path, content string) error {
    old, err := os.ReadFile(path)
    if err == nil {
        bak := fmt.Sprintf("%s.bak-%s", path, time.Now().UTC().Format("20060102T150405Z"))
        if err := os.WriteFile(bak, old, 0o600); err != nil { return err }
    } else if !os.IsNotExist(err) {
        return err
    }
    return os.WriteFile(path, []byte(content), 0o600)
}

func applyConfig(path, content string, dryRun bool) error {
    if dryRun {
        fmt.Printf("--- would write %s ---\n%s", path, content)
        return nil
    }
    return writeConfig(path, content)
}

var addDryRun bool
var addHost sshconf.Host

var addCmd = &cobra.Command{
    Use:   "add <alias>",
    Short: "Add a host to the fleet",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        cur, err := os.ReadFile(flagConfig)
        if err != nil && !os.IsNotExist(err) { return err }
        addHost.Alias = args[0]
        out, err := sshconf.Add(string(cur), addHost, flagMarker)
        if err != nil { return err }
        return applyConfig(flagConfig, out, addDryRun)
    },
}

func init() {
    addCmd.Flags().StringVar(&addHost.HostName, "hostname", "", "hostname or IP (required)")
    addCmd.Flags().StringVar(&addHost.User, "user", "", "ssh user")
    addCmd.Flags().StringVar(&addHost.Port, "port", "", "ssh port")
    addCmd.Flags().StringVar(&addHost.Identity, "identity", "", "identity file")
    addCmd.Flags().BoolVar(&addDryRun, "dry-run", false, "print the result without writing")
    _ = addCmd.MarkFlagRequired("hostname")
    rootCmd.AddCommand(addCmd)
}
```

```go
// sdk/fleet/cmd/remove.go
package cmd

import (
    "os"

    "github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
    "github.com/spf13/cobra"
)

var (
    rmPurge  bool
    rmDryRun bool
)

var removeCmd = &cobra.Command{
    Use:   "remove <alias>",
    Short: "Remove a host from the fleet (unmark by default; --purge deletes the block)",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        cur, err := os.ReadFile(flagConfig)
        if err != nil { return err }
        var out string
        if rmPurge {
            out, err = sshconf.Purge(string(cur), args[0])
        } else {
            out, err = sshconf.Unmark(string(cur), args[0], flagMarker)
        }
        if err != nil { return err }
        return applyConfig(flagConfig, out, rmDryRun)
    },
}

func init() {
    removeCmd.Flags().BoolVar(&rmPurge, "purge", false, "delete the Host block entirely")
    removeCmd.Flags().BoolVar(&rmDryRun, "dry-run", false, "print the result without writing")
    rootCmd.AddCommand(removeCmd)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd sdk/fleet && go test ./cmd/ -v -cover | tee ../../docs/mbo/plans/fleet/evidence/task08/membership.txt`
Expected: PASS.

- [ ] **Step 5: Live round-trip capture**

```bash
cp ~/.ssh/config /tmp/fleet-rt.cfg
fleet add tmphost --hostname 10.0.0.99 --config /tmp/fleet-rt.cfg
fleet status --config /tmp/fleet-rt.cfg | grep tmphost
fleet remove tmphost --config /tmp/fleet-rt.cfg
fleet remove tmphost --purge --config /tmp/fleet-rt.cfg
diff <(git show HEAD:/dev/null 2>/dev/null; cat ~/.ssh/config) /tmp/fleet-rt.cfg \
  | tee docs/mbo/plans/fleet/evidence/task08/roundtrip.txt
```

Expected: after `remove`, `tmphost` is gone from `status` but the block remains; after
`--purge` the block is gone; the diff shows no other block changed.

- [ ] **Step 6: Commit**

```bash
git add sdk/fleet docs/mbo/plans/fleet/evidence/task08
git commit -m "feat(fleet): add/remove fleet targets with backup and dry-run"
```

**Done when:** backup-per-write, dry-run-writes-nothing, and the live round-trip all pass.

---

### Task 9: `keys` diff core  *(leaf: `keys`; consumes sshconf reader)*

**Files:** Create `sdk/fleet/internal/keys/{keys.go,keys_test.go}`.

- [ ] **Step 1: Write the failing test**

```go
package keys

import "testing"

func TestComputeAddsMissingAndFlagsForeignForRemoval(t *testing.T) {
    local := []string{"ssh-ed25519 AAA me@box", "ssh-ed25519 BBB me@pi"}
    remote := []string{"ssh-ed25519 AAA me@box", "ssh-ed25519 ZZZ ci@runner"}
    d := Compute(local, remote)
    if len(d.ToAdd) != 1 || d.ToAdd[0] != "ssh-ed25519 BBB me@pi" {
        t.Fatalf("ToAdd = %v, want the one missing key", d.ToAdd)
    }
    if len(d.ToRemove) != 1 || d.ToRemove[0] != "ssh-ed25519 ZZZ ci@runner" {
        t.Fatalf("ToRemove = %v, want the foreign key", d.ToRemove)
    }
}

// Regression for defect 2 of the absorbed ssh-key-sync.sh: a foreign remote key
// must be REPORTED for removal, never silently dropped by the diff itself.
func TestComputeNeverReturnsAnEmptyRemoteAsWholesaleRemoval(t *testing.T) {
    d := Compute([]string{"ssh-ed25519 AAA me@box"}, nil)
    if len(d.ToRemove) != 0 {
        t.Fatalf("no remote keys must mean nothing to remove, got %v", d.ToRemove)
    }
    if len(d.ToAdd) != 1 { t.Fatalf("ToAdd = %v", d.ToAdd) }
}

func TestComputeIsNoOpWhenIdentical(t *testing.T) {
    k := []string{"ssh-ed25519 AAA me@box"}
    d := Compute(k, k)
    if len(d.ToAdd) != 0 || len(d.ToRemove) != 0 { t.Fatalf("expected no-op, got %+v", d) }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd sdk/fleet && go test ./internal/keys/ -v`
Expected: FAIL — `undefined: Compute`.

- [ ] **Step 3: Write minimal implementation**

```go
package keys

import "strings"

type Diff struct{ ToAdd, ToRemove []string }

func norm(s string) string { return strings.Join(strings.Fields(s), " ") }

// Compute reports what would change to make remote match local. ToRemove is
// REPORTED, never applied here — the caller must confirm (see spec F13).
func Compute(local, remote []string) Diff {
    var d Diff
    have := map[string]bool{}
    for _, r := range remote { have[norm(r)] = true }
    want := map[string]bool{}
    for _, l := range local { want[norm(l)] = true }
    for _, l := range local {
        if !have[norm(l)] { d.ToAdd = append(d.ToAdd, norm(l)) }
    }
    for _, r := range remote {
        if !want[norm(r)] { d.ToRemove = append(d.ToRemove, norm(r)) }
    }
    return d
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd sdk/fleet && go test ./internal/keys/ -v -cover | tee ../../docs/mbo/plans/fleet/evidence/task09/keys-diff.txt`
Expected: PASS, 3 tests.

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/internal/keys docs/mbo/plans/fleet/evidence/task09
git commit -m "feat(fleet): authorized_keys diff (report removals, never apply)"
```

**Done when:** the three diff tests pass, including the defect-2 regression.

---

### Task 10: `fleet keys list` / `sync`  *(leaf: `keys`)*

**Files:** Create `sdk/fleet/cmd/keys.go`, `sdk/fleet/cmd/keys_test.go`.

- [ ] **Step 1: Write the failing test**

```go
package cmd

import (
    "strings"
    "testing"

    "github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
)

func TestKeysSyncSendsOnlyPublicKeyMaterial(t *testing.T) {
    var sent []string
    r := recordingRunner{fake: runner.Fake{Out: map[string]string{"alpha": ""}}, log: &sent}
    if err := syncKeyToHost(r, "alpha", "ssh-ed25519 AAA me@box"); err != nil { t.Fatal(err) }
    joined := strings.Join(sent, " ")
    if strings.Contains(joined, "BEGIN OPENSSH PRIVATE KEY") || strings.Contains(joined, "scp") {
        t.Fatalf("private key material or scp used: %v", sent)
    }
    if !strings.Contains(joined, "ssh-ed25519 AAA") {
        t.Fatalf("public key was not sent: %v", sent)
    }
}

func TestKeysSyncReportsPerHostFailure(t *testing.T) {
    r := runner.Fake{Err: map[string]error{"dead": errFake}}
    if err := syncKeyToHost(r, "dead", "ssh-ed25519 AAA me@box"); err == nil {
        t.Fatal("a failing host must surface an error, not be swallowed")
    }
}
```

Add the small helpers `recordingRunner` and `errFake` to the test file:

```go
type recordingRunner struct {
    fake runner.Fake
    log  *[]string
}

func (r recordingRunner) Run(host string, argv ...string) (string, error) {
    *r.log = append(*r.log, strings.Join(argv, " "))
    return r.fake.Run(host, argv...)
}
func (r recordingRunner) RunInteractive(h string, a ...string) error { return r.fake.RunInteractive(h, a...) }

var errFake = errors.New("ssh failed")
```

(import `errors` and `strings`).

- [ ] **Step 2: Run test to verify it fails**

Run: `cd sdk/fleet && go test ./cmd/ -run TestKeysSync -v`
Expected: FAIL — `undefined: syncKeyToHost`.

- [ ] **Step 3: Write minimal implementation**

```go
// sdk/fleet/cmd/keys.go
package cmd

import (
    "fmt"
    "strings"

    "github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
    "github.com/spf13/cobra"
)

// syncKeyToHost authorizes ONE PUBLIC key on a host. It never transfers a
// private key — see design §3 defect 1.
func syncKeyToHost(r runner.Runner, host, pub string) error {
    pub = strings.Join(strings.Fields(pub), " ")
    cmdline := fmt.Sprintf(
        "mkdir -p ~/.ssh && touch ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys && "+
            "grep -qF %q ~/.ssh/authorized_keys || echo %q >> ~/.ssh/authorized_keys", pub, pub)
    if _, err := r.Run(host, cmdline); err != nil {
        return fmt.Errorf("%s: %w", host, err)
    }
    return nil
}

var keysCmd = &cobra.Command{Use: "keys", Short: "Manage fleet SSH keys"}

func init() { rootCmd.AddCommand(keysCmd) }
```

`keys list` and `keys sync` subcommands read managed `*.pub` files from `~/.ssh`, call
`keys.Compute` per host for `list`, and `syncKeyToHost` per (key, host) for `sync`,
accumulating per-host failures and returning a non-zero exit if any occurred.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd sdk/fleet && go test ./cmd/ -v -cover | tee ../../docs/mbo/plans/fleet/evidence/task10/keys.txt`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet docs/mbo/plans/fleet/evidence/task10
git commit -m "feat(fleet): keys list/sync (public-key-only, per-host results)"
```

**Done when:** the no-private-key and per-host-failure tests pass.

---

### Task 11: `fleet keys prune` / `delete` (diff-first)  *(leaf: `keys`)*

**Files:** Modify `sdk/fleet/cmd/keys.go`, `keys_test.go`.

- [ ] **Step 1: Write the failing test**

```go
func TestPruneRequiresConfirmationAndChangesNothingWhenDeclined(t *testing.T) {
    var sent []string
    r := recordingRunner{fake: runner.Fake{Out: map[string]string{
        "alpha": "ssh-ed25519 AAA me@box\nssh-ed25519 ZZZ ci@runner",
    }}, log: &sent}
    changed, err := pruneHost(r, "alpha", []string{"ssh-ed25519 AAA me@box"}, false /* confirmed */)
    if err != nil { t.Fatal(err) }
    if changed { t.Fatal("declined prune must report no change") }
    for _, c := range sent {
        if strings.Contains(c, ">") || strings.Contains(c, "sed -i") {
            t.Fatalf("declined prune still mutated the host: %q", c)
        }
    }
}

func TestPruneAppliesOnlyWhenConfirmed(t *testing.T) {
    var sent []string
    r := recordingRunner{fake: runner.Fake{Out: map[string]string{
        "alpha": "ssh-ed25519 AAA me@box\nssh-ed25519 ZZZ ci@runner",
    }}, log: &sent}
    changed, err := pruneHost(r, "alpha", []string{"ssh-ed25519 AAA me@box"}, true)
    if err != nil { t.Fatal(err) }
    if !changed { t.Fatal("confirmed prune should report a change") }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd sdk/fleet && go test ./cmd/ -run TestPrune -v`
Expected: FAIL — `undefined: pruneHost`.

- [ ] **Step 3: Write minimal implementation**

```go
// appended to sdk/fleet/cmd/keys.go
import "github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/keys"

// pruneHost computes the removal diff and applies it ONLY when confirmed.
// It never rewrites authorized_keys wholesale — see design §3 defect 2.
func pruneHost(r runner.Runner, host string, local []string, confirmed bool) (bool, error) {
    out, err := r.Run(host, "cat ~/.ssh/authorized_keys 2>/dev/null || true")
    if err != nil { return false, fmt.Errorf("%s: %w", host, err) }
    var remote []string
    for _, l := range strings.Split(out, "\n") {
        if strings.TrimSpace(l) != "" { remote = append(remote, l) }
    }
    d := keys.Compute(local, remote)
    if len(d.ToRemove) == 0 { return false, nil }
    fmt.Printf("%s: would remove %d authorized key(s):\n", host, len(d.ToRemove))
    for _, k := range d.ToRemove { fmt.Printf("  - %s\n", k) }
    if !confirmed { return false, nil }
    for _, k := range d.ToRemove {
        esc := strings.ReplaceAll(k, "/", "\\/")
        if _, err := r.Run(host, fmt.Sprintf("sed -i.bak '/%s/d' ~/.ssh/authorized_keys", esc)); err != nil {
            return false, fmt.Errorf("%s: %w", host, err)
        }
    }
    return true, nil
}
```

`keys prune` / `keys delete` gate on an interactive y/N prompt, bypassed by `--yes`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd sdk/fleet && go test ./cmd/ -v -cover | tee ../../docs/mbo/plans/fleet/evidence/task11/prune.txt`
Expected: PASS.

- [ ] **Step 5: Live defect-2 regression capture**

Add a throwaway `ssh-ed25519 ZZZ test@foreign` line to one host's `authorized_keys`, run
`fleet keys prune` and answer **no**, then show the entry still present:

```bash
fleet keys prune 2>&1 | tee docs/mbo/plans/fleet/evidence/task11/declined.txt
ssh <host> 'grep -c "test@foreign" ~/.ssh/authorized_keys' | tee -a docs/mbo/plans/fleet/evidence/task11/declined.txt
```

Expected: the count is `1` — the foreign key survived.

- [ ] **Step 6: Commit**

```bash
git add sdk/fleet docs/mbo/plans/fleet/evidence/task11
git commit -m "feat(fleet): diff-first keys prune/delete with confirmation gate"
```

**Done when:** declined prune provably changes nothing on a real host.

---

### Task 12: `fleet update` (headless) + dirty-clone policy  *(leaf: `update`; consumes status)*

**Files:** Create `sdk/fleet/cmd/{update.go,update_test.go}`.

- [ ] **Step 1: Write the failing test**

```go
func TestUpdateSkipsDirtyCloneByDefault(t *testing.T) {
    r := runner.Fake{Out: map[string]string{"alpha": " M install.sh"}} // git status --porcelain
    res, err := updateHost(r, "alpha", false /* force */)
    if err != nil { t.Fatal(err) }
    if !res.Skipped { t.Fatal("a dirty clone must be skipped without --force") }
    if res.Reason == "" { t.Fatal("skip must state a reason") }
}

func TestUpdateProceedsOnCleanClone(t *testing.T) {
    r := runner.Fake{Out: map[string]string{"alpha": ""}}
    res, err := updateHost(r, "alpha", false)
    if err != nil { t.Fatal(err) }
    if res.Skipped { t.Fatalf("a clean clone must not be skipped: %s", res.Reason) }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd sdk/fleet && go test ./cmd/ -run TestUpdate -v`
Expected: FAIL — `undefined: updateHost`.

- [ ] **Step 3: Write minimal implementation**

```go
// sdk/fleet/cmd/update.go
package cmd

import (
    "fmt"
    "strings"

    "github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
)

type UpdateResult struct {
    Skipped bool
    Reason  string
}

const remoteUpdate = `cd ~/git/dotfiles && git fetch origin && git checkout main && ` +
    `git pull --ff-only origin main && ./install.sh`

func updateHost(r runner.Runner, host string, force bool) (UpdateResult, error) {
    dirty, err := r.Run(host, "git -C ~/git/dotfiles status --porcelain")
    if err != nil { return UpdateResult{}, fmt.Errorf("%s: %w", host, err) }
    if strings.TrimSpace(dirty) != "" && !force {
        return UpdateResult{Skipped: true,
            Reason: "clone is dirty; re-run with --force to preserve local work in a rescue worktree"}, nil
    }
    if strings.TrimSpace(dirty) != "" && force {
        if _, err := r.Run(host, rescueWorktree); err != nil { return UpdateResult{}, err }
    }
    return UpdateResult{}, r.RunInteractive(host, remoteUpdate)
}

const rescueWorktree = `cd ~/git/dotfiles && ts=$(date -u +%Y%m%dT%H%M%SZ) && ` +
    `git stash push -u -m "fleet-rescue-$ts" && git branch "fleet-rescue/$ts" stash@{0} && ` +
    `mkdir -p ~/.local/state/dotfiles/rescue && ` +
    `git worktree add ~/.local/state/dotfiles/rescue/$ts "fleet-rescue/$ts"`
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd sdk/fleet && go test ./cmd/ -v -cover | tee ../../docs/mbo/plans/fleet/evidence/task12/update.txt`
Expected: PASS.

- [ ] **Step 5: Verify the rescue mechanism on a scratch clone**

The design flagged `git branch <name> stash@{0}` as needing verification against the
installed git. Run on a throwaway clone:

```bash
cd $(mktemp -d) && git init -q r && cd r && echo a > f && git add f && git commit -qm init
echo b > f && ts=test && git stash push -u -q -m "fleet-rescue-$ts" \
  && git branch "fleet-rescue/$ts" 'stash@{0}' \
  && git worktree add /tmp/rescue-$ts "fleet-rescue/$ts" \
  && cat /tmp/rescue-$ts/f
```

Expected: prints `b` (the stashed content is recoverable). **If this fails, record a blocker
in `TRACKING.md` and fall back to a rescue branch built from a temporary commit** — do not
silently change the design.

- [ ] **Step 6: Commit**

```bash
git add sdk/fleet docs/mbo/plans/fleet/evidence/task12
git commit -m "feat(fleet): headless update with skip-on-dirty and --force rescue"
```

**Done when:** both unit tests pass and the rescue mechanism is proven (or a blocker filed).

---

### Task 13: TUI  *(leaf: `tui`; consumes status + update)*

**Files:** Create `sdk/fleet/cmd/{tui.go,tui_test.go}`.

- [ ] **Step 1: Write the failing test**

```go
func TestTUIModelRendersOneLinePerHost(t *testing.T) {
    m := newModel([]Row{{Alias: "alpha", Class: "up-to-date"}, {Alias: "beta", Class: "behind", Behind: 3}})
    view := m.View()
    for _, want := range []string{"alpha", "beta", "behind 3"} {
        if !strings.Contains(view, want) { t.Fatalf("view missing %q:\n%s", want, view) }
    }
}

func TestTUISelectionMovesWithinBounds(t *testing.T) {
    m := newModel([]Row{{Alias: "a"}, {Alias: "b"}})
    m = m.moveCursor(-1)
    if m.cursor != 0 { t.Fatalf("cursor went above 0: %d", m.cursor) }
    m = m.moveCursor(1); m = m.moveCursor(1)
    if m.cursor != 1 { t.Fatalf("cursor went past the last row: %d", m.cursor) }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd sdk/fleet && go test ./cmd/ -run TestTUI -v`
Expected: FAIL — `undefined: newModel`.

- [ ] **Step 3: Write minimal implementation**

```go
// sdk/fleet/cmd/tui.go
package cmd

import (
    "fmt"
    "strings"

    tea "github.com/charmbracelet/bubbletea"
)

type model struct {
    rows   []Row
    cursor int
}

func newModel(rows []Row) model { return model{rows: rows} }

func (m model) Init() tea.Cmd { return nil }

func (m model) moveCursor(d int) model {
    m.cursor += d
    if m.cursor < 0 { m.cursor = 0 }
    if m.cursor > len(m.rows)-1 { m.cursor = len(m.rows) - 1 }
    return m
}

func (m model) View() string {
    var b strings.Builder
    b.WriteString("fleet — u: update  r: refresh  q: quit\n\n")
    for i, r := range m.rows {
        cur := "  "
        if i == m.cursor { cur = "> " }
        status := r.Class
        if r.Class == "behind" { status = fmt.Sprintf("behind %d", r.Behind) }
        fmt.Fprintf(&b, "%s%-16s %s\n", cur, r.Alias, status)
    }
    return b.String()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    if k, ok := msg.(tea.KeyMsg); ok {
        switch k.String() {
        case "q", "ctrl+c":
            return m, tea.Quit
        case "up", "k":
            return m.moveCursor(-1), nil
        case "down", "j":
            return m.moveCursor(1), nil
        case "u":
            host := m.rows[m.cursor].Alias
            // Release the terminal so install.sh can prompt for sudo (spec F7).
            return m, tea.Exec(interactiveUpdate(host), func(error) tea.Msg { return refreshMsg{} })
        }
    }
    return m, nil
}

type refreshMsg struct{}

// interactiveUpdate builds the *exec.Cmd that tea.Exec runs with the terminal
// handed over, so install.sh's sudo prompt reaches the user (spec F7). It
// reuses the same remote script as the headless path (Task 12) — one contract,
// two entry points.
func interactiveUpdate(host string) *exec.Cmd {
    return exec.Command("ssh", "-t", host, remoteUpdate)
}
```

Add `"os/exec"` to the `cmd/tui.go` imports. `remoteUpdate` comes from `cmd/update.go`
(Task 12), so **Task 12 must land before Task 13** — as the §6.1 DAG already requires.

Extend the test file to pin that contract:

```go
func TestInteractiveUpdateUsesTTYAndTheSharedRemoteScript(t *testing.T) {
    c := interactiveUpdate("alpha")
    args := strings.Join(c.Args, " ")
    if !strings.Contains(args, "-t ") { t.Fatalf("ssh must allocate a TTY: %v", c.Args) }
    if !strings.Contains(args, "install.sh") {
        t.Fatalf("must run the shared remote update script: %v", c.Args)
    }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd sdk/fleet && go test ./cmd/ -v -cover | tee ../../docs/mbo/plans/fleet/evidence/task13/tui.txt`
Expected: PASS.

- [ ] **Step 5: Capture the TUI**

Run: `fleet tui` and capture a still or asciinema into
`docs/mbo/plans/fleet/evidence/task13/tui.txt` (or `.cast`).

- [ ] **Step 6: Commit**

```bash
git add sdk/fleet docs/mbo/plans/fleet/evidence/task13
git commit -m "feat(fleet): TUI host list with update key"
```

**Done when:** model tests pass and a screen capture exists.

---

### Task 14: Live update of the stale host  *(the gate unit tests cannot fake)*

**Files:** evidence only.

- [ ] **Step 1: Capture the before state**

Run: `fleet status | tee docs/mbo/plans/fleet/evidence/task14/before.txt`
Expected: the Jetson host shows `behind N` / `unknown`.

- [ ] **Step 2: Run the interactive update**

Run: `fleet update <stale-host> 2>&1 | tee docs/mbo/plans/fleet/evidence/task14/transcript.txt`
Expected: an interactive session where `install.sh` prompts are answerable; completes.

- [ ] **Step 3: Capture the after state**

Run: `fleet status | tee docs/mbo/plans/fleet/evidence/task14/after.txt`
Expected: that host is now `up-to-date` with a fresh stamp.

- [ ] **Step 4: Commit**

```bash
git add docs/mbo/plans/fleet/evidence/task14
git commit -m "test(fleet): live update evidence for the stale host"
```

**Done when:** before shows stale, after shows up-to-date, with the transcript in between.
**This is the only proof that the end-to-end product works.** If the ARM/Jetson host
fails here, file a blocker — do not mark the objective done.

---

### Task 15: Parity, retirement, and docs

**Files:** Delete `src/ssh-key-sync/`; Modify `docs/mbo/index.md`, `sdk/fleet/AGENTS.md`,
root `AGENTS.md` (repo structure), `README.md` if it lists sdk tools.

- [ ] **Step 1: Capture parity**

```bash
bash src/ssh-key-sync/ssh-key-sync.sh --list | tee docs/mbo/plans/fleet/evidence/task15/legacy-list.txt
fleet keys list                              | tee docs/mbo/plans/fleet/evidence/task15/fleet-list.txt
```

Expected: the same key→host relationships for hosts in both scopes. **Differences caused by
the deliberate scope change (all-config-hosts → `#fleet`-marked) must be annotated in the
evidence file, not silently accepted.**

- [ ] **Step 2: Retire the script**

```bash
git rm -r src/ssh-key-sync
```

- [ ] **Step 3: Verify nothing still references it**

Run: `grep -rn "ssh-key-sync" --exclude-dir=.git . | grep -v docs/mbo | tee docs/mbo/plans/fleet/evidence/task15/refs.txt`
Expected: empty (any hit must be updated before this task is done).

- [ ] **Step 4: Update docs + index state**

`docs/mbo/index.md`: `fleet` state → `in-review`. Write `sdk/fleet/AGENTS.md` and the
`CLAUDE.md` symlink. Add `fleet` to the root `AGENTS.md` repo-structure list.

- [ ] **Step 5: Commit**

```bash
git add -u && git add docs/mbo sdk/fleet/AGENTS.md sdk/fleet/CLAUDE.md
git commit -m "refactor(fleet): retire ssh-key-sync after parity; document fleet"
```

**Done when:** parity captured, no dangling references, index state advanced.

---

## 5. Verification mapping

| Spec rule | Test / evidence |
| :-- | :-- |
| F1 stamp phase-gated | `install_test.sh::test_stamp_written_only_for_phase_all` (Task 2) |
| F2 marked concrete hosts only | `sshconf::TestParseReturnsOnlyMarkedConcreteHosts`, `TestParseSkipsPatternHostsEntirely` (Task 3) |
| F3 worst-first table | `cmd::TestRenderTableIsWorstFirst` (Task 7) |
| F4 five classes | `drift::TestClassifyAllFiveClasses`, `TestClassifyNeverReportsBehindWhenCommitsMatch` (Task 6) |
| F5 JSON + exit code | `cmd::TestExitCodeNonZeroWhenAnyHostStale` (Task 7) |
| F6 TUI rows | `cmd::TestTUIModelRendersOneLinePerHost`, `TestTUISelectionMovesWithinBounds` (Task 13) |
| F7 interactive update | `cmd::TestUpdateProceedsOnCleanClone` + **Task 14 live transcript** |
| F8 dirty-clone safety | `cmd::TestUpdateSkipsDirtyCloneByDefault` + Task 12 Step 5 rescue proof |
| F9 add idempotent + backup | `sshconf::TestAddIsIdempotentAndPreservesOtherBlocks`, `cmd::TestWriteConfigTakesBackupFirst`, `TestDryRunWritesNothing` (Tasks 4, 8) |
| F10 remove unmarks; purge deletes | `sshconf::TestUnmarkKeepsBlockButLeavesFleet`, `TestPurgeRemovesOnlyTheTargetBlock`, `TestUnknownAliasIsAnError` (Task 4) |
| F11 keys list matrix | `keys::TestComputeIsNoOpWhenIdentical` + Task 15 parity capture |
| F12 public-key-only, per-host results | `cmd::TestKeysSyncSendsOnlyPublicKeyMaterial`, `TestKeysSyncReportsPerHostFailure` (Task 10) |
| F13 diff-first prune | `cmd::TestPruneRequiresConfirmationAndChangesNothingWhenDeclined`, `TestPruneAppliesOnlyWhenConfirmed`, `keys::TestComputeAddsMissingAndFlagsForeignForRemoval` + **Task 11 live declined-prune capture** |

## 6. Integration & rollout

- **Build/test discovery** is automatic once `sdk/fleet/go.mod` exists (`scripts/test.sh`
  scans `src/` and `sdk/`), but the coverage floor is **not** — Task 1 adds
  `fleet) echo 60 ;;` to `coverage_min()`, without which the module is silently exempt.
- **Install**: the `gff_on install.sdk.fleet` block plus the `features.yaml` flag.
- **Manual acceptance checklist** (after Task 15):
  1. `fleet status` lists every marked host with a plausible age.
  2. `fleet status --json | jq .` parses; `echo $?` is 1 while any host is stale.
  3. `fleet add`/`remove` round-trip leaves `~/.ssh/config` otherwise byte-identical.
  4. `fleet keys list` matches the legacy script's view (annotated for scope).
  5. A declined `fleet keys prune` leaves a foreign key intact.
  6. `fleet tui` renders and `u` hands over the terminal.

### 6.1 Build leaves / DAG

Authoritative graph. `A → B` = B's row lists A under *Consumes*.

| Leaf | Owns (paths) | Consumes (in-edges) | `done-when` gate | Blocking? |
| :-- | :-- | :-- | :-- | :-- |
| `scaffold` | `sdk/fleet/{go.mod,main.go,VERSION,build.sh,cmd/root.go,cmd/version.go}`, `Makefile`, `scripts/test.sh`, `.github/gff/features.yaml`, `install.sh` (sdk block) | — | `go test ./...` passes; `scripts/test.sh` lists `fleet`; `bash -n install.sh` clean | **yes (base)** |
| `stamp-sh` | `install.sh` (stamp block), `install.sh.stampblock`, `install_test.sh` | — | stamp phase-gating test passes | no (independent) |
| `sshconf` | `sdk/fleet/internal/sshconf/**` | `scaffold` | Tasks 3–4 tests pass | **yes** (status, membership, keys import it) |
| `drift` | `sdk/fleet/internal/{stamp,drift}/**` | `scaffold` | Tasks 5–6 tests pass | **yes** (status imports it) |
| `status` | `sdk/fleet/internal/runner/**`, `sdk/fleet/cmd/status*.go` | `sshconf`, `drift` | Task 7 tests + live capture | no |
| `membership` | `sdk/fleet/cmd/{add,remove,membership_test}.go` | `sshconf` | Task 8 tests + round-trip capture | no |
| `keys` | `sdk/fleet/internal/keys/**`, `sdk/fleet/cmd/keys*.go` | `sshconf`, `status` (runner) | Tasks 9–11 tests + declined-prune capture | no |
| `tui` | `sdk/fleet/cmd/{tui,update}*.go` | `status` | Tasks 12–13 tests + captures | no |
| `migration` | delete `src/ssh-key-sync/`, docs, index state | `keys`, `tui` | Task 15 parity + no dangling refs | no (last) |

**Blocking-first order:** `scaffold` → (`sshconf`, `drift` in parallel) → (`status`,
`membership`, `keys` in parallel) → `tui` → `migration`. `stamp-sh` can run any time.

**Sequential vs parallel is still an open call.** Per `docs/mbo/AGENTS.md` the default is
*not* to break out; this table defines the build order either way. If fanned out, each leaf
becomes a `gss feature worker` with `--base` set to its in-edge, and blocking leaves must be
created first.

## 7. Validation & evidence (show the work)

**Coverage.** `sdk/fleet` floor is **60%** via `coverage_min()`. `COVERAGE_ENFORCE` defaults
to `0` (warn-only) repo-wide — so treat the floor as a **hard gate for this objective**:
Task 15 must show `go test ./... -cover` ≥ 60% for every package, regardless of the
warn-only default. Real test failures are always hard.

**Evidence protocol.** A tracked tree at `docs/mbo/plans/fleet/evidence/` with one folder per
task (`task01/` … `task15/`). Every `done-when` command's output is `tee`'d there with a
dated header, append-only, and committed **with the task**. A feature without captured
evidence is not done.

**Adversarial scenarios that must be exercised, not assumed:**

| Scenario | Expected | Task |
| :-- | :-- | :-- |
| host reachable, no stamp | `unknown`, not `up-to-date` | 6, 7 |
| host unreachable | row present as `unreachable`, exit ≠ 0 | 6, 7 |
| stamp commit not in local history | `ahead/divergent`, no crash | 6 |
| `add` an alias twice | byte-identical file; no duplicate block | 4 |
| `remove` then `ssh <alias>` | still resolves — access preserved | 8 |
| foreign key in remote `authorized_keys`, prune declined | key survives | 11 |
| `install.sh` fails mid-update | row keeps prior state; error surfaced | 12, 14 |
| ARM/Jetson host update | completes, or a blocker is filed | 14 |

**Demo.** After Task 15, a short capture for the repo's show-and-tell: `fleet status`
showing mixed states → `u` in the TUI updating the stale host → `fleet status` all green.

> Produced via `superpowers:writing-plans`. Execute with
> `superpowers:subagent-driven-development` / `executing-plans`, TDD throughout.
> Update [`../index.md`](../index.md) state as it moves.
