package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/pins"
)

// fakeResolver answers by EXACT upstream string, so no test depends on map
// order or substring matching.
type fakeResolver struct {
	rel     map[string]pins.Release
	err     map[string]error
	calls   atomic.Int32
	cur     atomic.Int32
	max     atomic.Int32
	hold    time.Duration
	mu      sync.Mutex
	queried []string
}

func (f *fakeResolver) Latest(_ context.Context, up string) (pins.Release, error) {
	f.calls.Add(1)
	n := f.cur.Add(1)
	for {
		m := f.max.Load()
		if n <= m || f.max.CompareAndSwap(m, n) {
			break
		}
	}
	if f.hold > 0 {
		time.Sleep(f.hold)
	}
	f.cur.Add(-1)
	f.mu.Lock()
	f.queried = append(f.queried, up)
	f.mu.Unlock()
	if up == "-" {
		return pins.Release{}, pins.ErrNoUpstream
	}
	if e, ok := f.err[up]; ok {
		return pins.Release{}, e
	}
	if r, ok := f.rel[up]; ok {
		return r, nil
	}
	return pins.Release{}, errors.New("no fake release for " + up)
}

const pinsFixture = `# manifest
go      pin     @.go-version  go:                 -  go version
uv      pin     0.12.16       gh:astral-sh/uv     opt/scripts/system/install_uv.sh   uv --version

glow    pin     3.0.0         gh:charmbracelet/glow  opt/scripts/system/install_glow.sh glow --version
rust    floor   1.96.0        gh:rust-lang/rust   opt/scripts/system/install_rust.sh rustc --version
sops    pin     3.13.1        gh:getsops/sops     opt/scripts/system/install_sops.sh sops --version
yq      pin     4.44.6        gh:mikefarah/yq     opt/scripts/system/install_yq.sh   yq --version
herdr   latest  -             -                   opt/scripts/system/install_herdr.sh herdr --version
vault   pin     1.21.4        gh:hashicorp/vault  -  vault version
`

func writePinsRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "opt", "profiles")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "versions.tsv"), []byte(pinsFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".go-version"), []byte("1.26.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func loadPinsRepo(t *testing.T) (*pins.Manifest, string) {
	t.Helper()
	root := writePinsRepo(t)
	m, err := pins.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return m, root
}

func fixtureResolver() *fakeResolver {
	d := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	return &fakeResolver{
		rel: map[string]pins.Release{
			"go:":                   {Version: "1.27.1"},                // minor
			"gh:astral-sh/uv":       {Version: "0.12.17", Published: d}, // patch
			"gh:charmbracelet/glow": {Version: "3.0.0", Published: d},   // current
			"gh:rust-lang/rust":     {Version: "1.98.1", Published: d},  // minor
			"gh:getsops/sops":       {Version: "3.12.0", Published: d},  // ahead
			"gh:hashicorp/vault":    {Version: "2.0.0", Published: d},   // major
		},
		err: map[string]error{"gh:mikefarah/yq": errors.New("HTTP 403 rate limited")},
	}
}

func TestPinsStatusRows(t *testing.T) {
	m, _ := loadPinsRepo(t)
	rows := pinsStatusRows(context.Background(), m, fixtureResolver(), 4)
	want := []struct{ tool, pin, latest, gap string }{
		{"go", "1.26.1", "1.27.1", "minor"},
		{"uv", "0.12.16", "0.12.17", "patch"},
		{"glow", "3.0.0", "3.0.0", "current"},
		{"rust", "1.96.0", "1.98.1", "minor"},
		{"sops", "3.13.1", "3.12.0", "ahead"},
		{"yq", "4.44.6", "", "error"},
		{"herdr", "latest", "n/a", "-"},
		{"vault", "1.21.4", "2.0.0", "major"},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %d, want %d", len(rows), len(want))
	}
	for i, w := range want {
		r := rows[i]
		if r.Tool != w.tool || r.Pin != w.pin || r.Latest != w.latest || r.Gap != w.gap {
			t.Errorf("row %d = %+v, want %+v", i, r, w)
		}
	}
	if !strings.Contains(rows[5].Error, "rate limited") {
		t.Errorf("yq error not kept: %+v", rows[5])
	}
	if rows[1].Released != "2026-09-30" || rows[0].Released != "" {
		t.Errorf("released = %q / %q", rows[1].Released, rows[0].Released)
	}
}

func TestPinsStatusPoolIsBoundedAndOrdered(t *testing.T) {
	m, _ := loadPinsRepo(t)
	res := fixtureResolver()
	res.hold = 20 * time.Millisecond
	rows := pinsStatusRows(context.Background(), m, res, 3)
	if got := res.max.Load(); got > 3 || got < 2 {
		t.Errorf("max concurrency = %d, want 2..3", got)
	}
	for i, e := range m.Entries {
		if rows[i].Tool != e.Tool {
			t.Fatalf("row %d = %s, want manifest order %s", i, rows[i].Tool, e.Tool)
		}
	}
}

func TestPinsStatusTableAndJSON(t *testing.T) {
	m, _ := loadPinsRepo(t)
	rows := pinsStatusRows(context.Background(), m, fixtureResolver(), 4)

	var tb bytes.Buffer
	renderPinsStatus(&tb, rows)
	out := tb.String()
	for _, want := range []string{"TOOL", "TRACK", "PIN", "LATEST", "GAP", "RELEASED", "error", "n/a", "2026-09-30"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q:\n%s", want, out)
		}
	}

	var jb bytes.Buffer
	if err := renderPinsStatusJSON(&jb, rows); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Rows []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal(jb.Bytes(), &got); err != nil {
		t.Fatalf("bad json: %v\n%s", err, jb.String())
	}
	if len(got.Rows) != 8 || got.Rows[0]["tool"] != "go" || got.Rows[5]["gap"] != "error" {
		t.Errorf("json rows = %v", got.Rows)
	}
}

func bumpOpts(m *pins.Manifest, res pins.Resolver, out *bytes.Buffer) pinsBumpOpts {
	return pinsBumpOpts{M: m, Res: res, Out: out}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestPinsBumpNamedWritesOnlyThatField(t *testing.T) {
	m, root := loadPinsRepo(t)
	var out bytes.Buffer
	o := bumpOpts(m, fixtureResolver(), &out)
	o.Tools = []string{"uv", "rust"}
	if err := runPinsBump(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, m.Path)
	want := strings.Replace(pinsFixture, "0.12.16      ", "0.12.17      ", 1)
	want = strings.Replace(want, "1.96.0        gh:rust", "1.98.1        gh:rust", 1)
	if got != want {
		t.Errorf("manifest after bump:\n%s\nwant:\n%s", got, want)
	}
	if !strings.Contains(out.String(), "uv 0.12.16 → 0.12.17") || !strings.Contains(out.String(), "rust 1.96.0 → 1.98.1") {
		t.Errorf("output = %q", out.String())
	}
	if readFile(t, filepath.Join(root, ".go-version")) != "1.26.1\n" {
		t.Error(".go-version must be untouched")
	}
}

func TestPinsBumpAllSkipsCurrentAheadLatestAndErrors(t *testing.T) {
	m, root := loadPinsRepo(t)
	var out bytes.Buffer
	o := bumpOpts(m, fixtureResolver(), &out)
	o.All = true
	if err := runPinsBump(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	// bumped: go (file), uv, rust, vault. Not: glow current, sops ahead, yq error, herdr latest.
	if got := readFile(t, filepath.Join(root, ".go-version")); got != "1.27.1\n" {
		t.Errorf(".go-version = %q", got)
	}
	re, _ := pins.Load(root)
	for tool, want := range map[string]string{"uv": "0.12.17", "rust": "1.98.1", "vault": "2.0.0", "glow": "3.0.0", "sops": "3.13.1", "yq": "4.44.6"} {
		if e, _ := re.Get(tool); e.Version != want {
			t.Errorf("%s = %s, want %s", tool, e.Version, want)
		}
	}
	for _, line := range []string{"go 1.26.1 → 1.27.1", "uv 0.12.16 → 0.12.17", "vault 1.21.4 → 2.0.0"} {
		if !strings.Contains(out.String(), line) {
			t.Errorf("output missing %q:\n%s", line, out.String())
		}
	}
	if strings.Contains(out.String(), "glow 3.0.0 →") || strings.Contains(out.String(), "herdr") {
		t.Errorf("output bumps rows it should skip:\n%s", out.String())
	}
}

func TestPinsBumpDryRunWritesNothing(t *testing.T) {
	m, root := loadPinsRepo(t)
	var out bytes.Buffer
	o := bumpOpts(m, fixtureResolver(), &out)
	o.All, o.DryRun = true, true
	if err := runPinsBump(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if readFile(t, m.Path) != pinsFixture || readFile(t, filepath.Join(root, ".go-version")) != "1.26.1\n" {
		t.Error("dry-run changed files")
	}
	if !strings.Contains(out.String(), "uv 0.12.16 → 0.12.17") {
		t.Errorf("dry-run should still print the plan: %q", out.String())
	}
}

func TestPinsBumpToSkipsNetwork(t *testing.T) {
	m, _ := loadPinsRepo(t)
	res := fixtureResolver()
	var out bytes.Buffer
	o := bumpOpts(m, res, &out)
	o.Tools, o.To = []string{"go"}, "v1.26.3"
	if err := runPinsBump(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if res.calls.Load() != 0 {
		t.Errorf("--to made %d lookups", res.calls.Load())
	}
	if !strings.Contains(out.String(), "go 1.26.1 → 1.26.3") {
		t.Errorf("output = %q", out.String())
	}
	if got := readFile(t, filepath.Join(m.Root, ".go-version")); got != "1.26.3\n" {
		t.Errorf(".go-version = %q", got)
	}
}

func TestPinsBumpArgErrors(t *testing.T) {
	cases := []struct {
		name string
		mod  func(*pinsBumpOpts)
		want string
	}{
		{"nothing", func(o *pinsBumpOpts) {}, "name a tool or pass --all"},
		{"to needs one", func(o *pinsBumpOpts) { o.Tools, o.To = []string{"uv", "yq"}, "1.0.0" }, "exactly one"},
		{"to with all", func(o *pinsBumpOpts) { o.All, o.To = true, "1.0.0" }, "exactly one"},
		{"all with tools", func(o *pinsBumpOpts) { o.All, o.Tools = true, []string{"uv"} }, "either"},
		{"unknown", func(o *pinsBumpOpts) { o.Tools = []string{"ghost"} }, "ghost"},
		{"latest row", func(o *pinsBumpOpts) { o.Tools = []string{"herdr"} }, "latest"},
		{"bad to", func(o *pinsBumpOpts) { o.Tools, o.To = []string{"uv"}, "nope" }, "version"},
		{"lookup error", func(o *pinsBumpOpts) { o.Tools = []string{"yq"} }, "rate limited"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, _ := loadPinsRepo(t)
			var out bytes.Buffer
			o := bumpOpts(m, fixtureResolver(), &out)
			c.mod(&o)
			err := runPinsBump(context.Background(), o)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want %q", err, c.want)
			}
			if readFile(t, m.Path) != pinsFixture {
				t.Error("a refused bump must not write")
			}
		})
	}
}

func TestPinsBumpNamedAlreadyCurrent(t *testing.T) {
	m, _ := loadPinsRepo(t)
	var out bytes.Buffer
	o := bumpOpts(m, fixtureResolver(), &out)
	o.Tools = []string{"glow", "sops"}
	if err := runPinsBump(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if readFile(t, m.Path) != pinsFixture {
		t.Error("nothing should change")
	}
	if !strings.Contains(out.String(), "glow") || !strings.Contains(out.String(), "up to date") {
		t.Errorf("output = %q", out.String())
	}
}

func TestPinsToken(t *testing.T) {
	env := func(v string) func(string) string {
		return func(k string) string {
			if k == "GITHUB_TOKEN" {
				return v
			}
			return ""
		}
	}
	ghOK := func(string) (string, error) { return "/usr/bin/gh", nil }
	ghMissing := func(string) (string, error) { return "", errors.New("not found") }
	run := func(out string, err error) func(context.Context, string, ...string) (string, error) {
		return func(context.Context, string, ...string) (string, error) { return out, err }
	}
	ctx := context.Background()
	if got := pinsToken(ctx, env("fromenv"), ghOK, run("fromgh\n", nil)); got != "fromenv" {
		t.Errorf("env should win, got %q", got)
	}
	if got := pinsToken(ctx, env(""), ghOK, run("fromgh\n", nil)); got != "fromgh" {
		t.Errorf("gh fallback = %q", got)
	}
	if got := pinsToken(ctx, env(""), ghMissing, run("x", nil)); got != "" {
		t.Errorf("no gh = %q", got)
	}
	if got := pinsToken(ctx, env(""), ghOK, run("", errors.New("not logged in"))); got != "" {
		t.Errorf("gh failure = %q", got)
	}
}

// withPinsGlobals points the shared --repo/--json flag variables at a test
// value and restores them.
func withPinsGlobals(t *testing.T, repo string, jsonOut bool) {
	t.Helper()
	oldRepo, oldJSON, oldExplicit, oldDefault := flagRepo, flagJSON, flagRepoExplicit, defaultDotfilesRepo
	// Hermetic: no fallback to the developer's real ~/git/dotfiles, whose
	// manifest made "missing manifest" tests pass in CI and fail locally.
	flagRepo, flagJSON, flagRepoExplicit, defaultDotfilesRepo = repo, jsonOut, false, ""
	t.Cleanup(func() {
		flagRepo, flagJSON, flagRepoExplicit, defaultDotfilesRepo = oldRepo, oldJSON, oldExplicit, oldDefault
	})
}

func runPinsCmd(t *testing.T, d *pinsDeps, args ...string) (string, error) {
	t.Helper()
	c := newPinsCmd(d)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetArgs(args)
	err := c.Execute()
	return out.String(), err
}

func TestPinsCommandStatusExitsZeroWithRowErrors(t *testing.T) {
	root := writePinsRepo(t)
	withPinsGlobals(t, root, false)
	d := &pinsDeps{
		resolver:   func(string) pins.Resolver { return fixtureResolver() },
		credential: func(context.Context) string { return "" },
	}
	out, err := runPinsCmd(t, d, "status")
	if err != nil {
		t.Fatalf("status with a failing row must exit 0, got %v", err)
	}
	if !strings.Contains(out, "error") || !strings.Contains(out, "minor") {
		t.Errorf("output:\n%s", out)
	}
	withPinsGlobals(t, root, true)
	out, err = runPinsCmd(t, d, "status")
	if err != nil || !strings.Contains(out, `"rows"`) {
		t.Errorf("json: err=%v out=%s", err, out)
	}
}

func TestPinsCommandBump(t *testing.T) {
	root := writePinsRepo(t)
	withPinsGlobals(t, root, false)
	d := &pinsDeps{
		resolver:   func(string) pins.Resolver { return fixtureResolver() },
		credential: func(context.Context) string { return "" },
	}
	out, err := runPinsCmd(t, d, "bump", "uv", "--dry-run")
	if err != nil || !strings.Contains(out, "uv 0.12.16 → 0.12.17") {
		t.Fatalf("err=%v out=%s", err, out)
	}
	if readFile(t, filepath.Join(root, "opt/profiles/versions.tsv")) != pinsFixture {
		t.Error("dry-run wrote")
	}
	if _, err := runPinsCmd(t, d, "bump", "uv", "--to", "0.12.20"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readFile(t, filepath.Join(root, "opt/profiles/versions.tsv")), "0.12.20") {
		t.Error("--to not written")
	}
	if _, err := runPinsCmd(t, d, "bump"); err == nil {
		t.Error("bare bump should fail")
	}
}

func TestPinsCommandMissingManifest(t *testing.T) {
	withPinsGlobals(t, t.TempDir(), false)
	d := &pinsDeps{resolver: func(string) pins.Resolver { return fixtureResolver() }, credential: func(context.Context) string { return "" }}
	if _, err := runPinsCmd(t, d, "status"); err == nil {
		t.Error("missing manifest should be an error")
	}
}

// Run from inside ANOTHER git repo, fleet's --repo becomes that repo; pins
// must still find the dotfiles manifest at the default clone unless --repo
// was given explicitly.
func TestLoadPinsManifestFallsBackToTheDefaultClone(t *testing.T) {
	other := t.TempDir()
	dot := writePinsRepo(t)
	oldRepo, oldExplicit, oldDefault := flagRepo, flagRepoExplicit, defaultDotfilesRepo
	t.Cleanup(func() { flagRepo, flagRepoExplicit, defaultDotfilesRepo = oldRepo, oldExplicit, oldDefault })

	flagRepo, flagRepoExplicit, defaultDotfilesRepo = other, false, dot
	m, err := loadPinsManifest()
	if err != nil || m.Root != dot {
		t.Fatalf("fallback: m=%v err=%v", m, err)
	}
	flagRepoExplicit = true
	if _, err := loadPinsManifest(); err == nil {
		t.Fatal("an explicit --repo without a manifest must be an error, not a silent fallback")
	}
}
