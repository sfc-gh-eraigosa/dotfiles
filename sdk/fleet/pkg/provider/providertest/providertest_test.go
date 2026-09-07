package providertest_test

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/pkg/provider"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/pkg/provider/providertest"
)

// runnerHost is the adapter leaf C will own in internal/providers: a
// provider.Host backed by the runner seam for one alias. It lives here, in a
// test, so leaf A can prove the seam closes end to end without depending on
// the registry it has no in-edge to.
type runnerHost struct {
	alias string
	r     runner.CtxRunner
}

func (h runnerHost) Alias() string { return h.alias }
func (h runnerHost) Exec(ctx context.Context, stdin string, argv ...string) (provider.ExecResult, error) {
	return h.r.RunCtx(ctx, h.alias, stdin, argv...)
}

// The smoke test of leaf A: FakeProvider drives every level through the
// Provider and Host interfaces ALONE — no registry, no protocol, no herdr —
// and every node it returns satisfies the contract's own Validate.
func TestFakeProviderDrivesEveryLevelThroughTheContractAlone(t *testing.T) {
	host := runnerHost{
		alias: "spark",
		r: runner.Fake{
			Out:   map[string]string{"spark": "widget-a\nwidget-b"},
			Stdin: map[string]string{},
			Argv:  map[string][][]string{},
		},
	}
	var p provider.Provider = providertest.NewFakeProvider()
	ctx := context.Background()

	if p.Name() == "" {
		t.Fatal("a provider must name itself")
	}

	// Level 0: the capability row.
	cap, err := p.Probe(ctx, host)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if err := cap.Validate(); err != nil {
		t.Fatalf("the probe node must satisfy the contract: %v", err)
	}
	if cap.Attrs["alias"] != "spark" {
		t.Fatalf("the provider sees the dispatched alias, got attrs=%v", cap.Attrs)
	}
	if len(p.Columns(cap.Kind)) != 5 {
		t.Fatalf("the fake kind has five columns, got %v", p.Columns(cap.Kind))
	}

	// Level 1: children of the capability, with attrs round-tripped.
	mid, err := p.Children(ctx, host, nil, cap.Attrs)
	if err != nil {
		t.Fatalf("children(nil): %v", err)
	}
	if len(mid) == 0 {
		t.Fatal("level 1 is empty")
	}
	for _, n := range mid {
		if err := n.Validate(); err != nil {
			t.Fatalf("level-1 node %q: %v", n.ID, err)
		}
		if n.Attrs["alias"] != "spark" || n.Attrs["parent"] != cap.ID {
			t.Fatalf("attrs did not round-trip into level 1: %v", n.Attrs)
		}
		if got := len(p.Columns(n.Kind)); got != 5 {
			t.Fatalf("kind %q has %d columns, want 5", n.Kind, got)
		}
	}

	// Level 2: leaves, one action of each of the three kinds.
	leaves, err := p.Children(ctx, host, []string{mid[0].ID}, mid[0].Attrs)
	if err != nil {
		t.Fatalf("children(%q): %v", mid[0].ID, err)
	}
	if len(leaves) == 0 {
		t.Fatal("level 2 is empty")
	}
	kinds := map[string]bool{}
	for _, n := range leaves {
		if err := n.Validate(); err != nil {
			t.Fatalf("leaf %q: %v", n.ID, err)
		}
		if !n.Leaf {
			t.Fatalf("level 2 must be leaves, %q is not", n.ID)
		}
		for _, a := range n.Actions {
			switch {
			case a.Handoff != nil:
				kinds["handoff"] = true
			case a.Stream != nil:
				kinds["stream"] = true
			case a.Tunnel != nil:
				kinds["tunnel"] = true
			}
		}
	}
	for _, want := range []string{"handoff", "stream", "tunnel"} {
		if !kinds[want] {
			t.Fatalf("the fake tree must carry one action of each kind; missing %q (got %v)", want, kinds)
		}
	}

	// An unknown segment is ErrNoSuchPath, and an unknown kind has no columns.
	if _, err := p.Children(ctx, host, []string{"nope"}, nil); err == nil {
		t.Fatal("an unknown path segment must error")
	}
	if cols := p.Columns("not-a-kind"); cols != nil {
		t.Fatalf("an unknown kind renders IDs only, got %v", cols)
	}
}

// The provider's ONLY reach is Host.Exec, and what it runs lands on the
// runner seam for the alias fleet dispatched — nowhere else.
func TestTheFakeProviderReachesTheHostOnlyThroughExec(t *testing.T) {
	f := runner.Fake{
		Out:   map[string]string{"spark": "widget-a"},
		Stdin: map[string]string{},
		Argv:  map[string][][]string{},
	}
	p := providertest.NewFakeProvider()
	if _, err := p.Probe(context.Background(), runnerHost{alias: "spark", r: f}); err != nil {
		t.Fatal(err)
	}
	if len(f.Argv["spark"]) == 0 {
		t.Fatal("the provider never reached the host through Exec")
	}
	if len(f.Argv["nano"]) != 0 {
		t.Fatal("the provider reached a host it was not dispatched to")
	}
}

// FakeProvider is scriptable: the failure modes a consumer must render as a
// row — absent, an exec that fails, a hang — are what the later leaves test
// against, so they belong to the harness, not to each leaf.
func TestFakeProviderCanBeScriptedIntoItsFailureModes(t *testing.T) {
	host := runnerHost{alias: "spark", r: runner.Fake{Out: map[string]string{"spark": "widget-a"}}}

	absent := providertest.NewFakeProvider(providertest.Absent("nothing installed here"))
	n, err := absent.Probe(context.Background(), host)
	if err == nil {
		t.Fatal("an absent capability must report ErrAbsent")
	}
	if !strings.Contains(err.Error(), "nothing installed here") {
		t.Fatalf("the absent error must carry the reason: %v", err)
	}
	if n.ID == "" || len(n.Actions) != 0 {
		t.Fatalf("absent is still a ROW, with no actions: %#v", n)
	}

	boom := providertest.NewFakeProvider(providertest.ProbeError("cannot look"))
	if _, err := boom.Probe(context.Background(), host); err == nil {
		t.Fatal("a scripted probe error must surface")
	}

	// A hanging provider is cancelled by its caller's context, never by itself.
	hang := providertest.NewFakeProvider(providertest.Hang())
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := hang.Probe(ctx, host); err == nil {
		t.Fatal("a hanging probe must return the context's error")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("the hang did not honour the context")
	}
}

// BuildStub compiles the scriptable stub process once per test binary. The
// stub is protocol-AGNOSTIC on purpose: leaf A owns no wire format, so the
// stub is a line-oriented process whose replies are canned by the caller.
// Leaf B's protocol tests supply the JSON; the failure modes — sleeping past
// a deadline, exiting at once, writing half a line, stderr — are here.
func TestBuildStubYieldsAScriptableProcess(t *testing.T) {
	stub := providertest.BuildStub(t)

	// Canned reply: the stub answers each line it reads.
	out := runStub(t, stub, `{"id":1,"method":"initialize"}`+"\n", "-reply", `{"id":1,"result":{"protocol":1}}`)
	var got map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &got); err != nil {
		t.Fatalf("stub reply is not one JSON object per line: %q (%v)", out, err)
	}
	if got["id"] != float64(1) {
		t.Fatalf("stub did not answer the request: %v", got)
	}

	// Exits at once: no reply, non-zero status, stderr preserved.
	c := exec.Command(stub, "-exit-at-once", "-stderr", "boom")
	c.Stdin = strings.NewReader(`{"id":1}` + "\n")
	var stderr strings.Builder
	c.Stderr = &stderr
	o, err := c.Output()
	if err == nil {
		t.Fatal("-exit-at-once must exit non-zero")
	}
	if len(o) != 0 {
		t.Fatalf("-exit-at-once must not reply, got %q", o)
	}
	if !strings.Contains(stderr.String(), "boom") {
		t.Fatalf("stub stderr not captured: %q", stderr.String())
	}

	// Half a line: framing must be testable by leaf B.
	if out := runStub(t, stub, `{"id":1}`+"\n", "-half-line", `{"id":1,"resu`); !strings.HasSuffix(out, "resu") || strings.Contains(out, "\n") {
		t.Fatalf("-half-line must write an unterminated fragment, got %q", out)
	}

	// Sleeps past a deadline: the caller kills it, the stub never returns.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	cc := exec.CommandContext(ctx, stub, "-sleep", "30s")
	cc.Stdin = strings.NewReader(`{"id":1}` + "\n")
	start := time.Now()
	_ = cc.Run()
	if time.Since(start) > 3*time.Second {
		t.Fatal("a sleeping stub must be killable by its caller's context")
	}

	// Built once: a second call returns the same path without rebuilding.
	if providertest.BuildStub(t) != stub {
		t.Fatal("BuildStub must build once per test binary")
	}
}

func runStub(t *testing.T, stub, stdin string, args ...string) string {
	t.Helper()
	c := exec.Command(stub, args...)
	c.Stdin = strings.NewReader(stdin)
	out, err := c.Output()
	if err != nil {
		t.Fatalf("stub %v: %v", args, err)
	}
	return string(out)
}

// The CLI enters a level directly — `fleet ls <host> fake widget-a` — with no
// prior probe, so Children must be able to fetch what an empty attrs map
// does not carry, and must surface a failing host rather than an empty level.
func TestALevelEnteredWithoutAProbeStillFetchesItsOwnData(t *testing.T) {
	f := runner.Fake{Out: map[string]string{"spark": "widget-a\nwidget-b"}, Argv: map[string][][]string{}}
	p := providertest.NewFakeProvider()
	got, err := p.Children(context.Background(), runnerHost{alias: "spark", r: f}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected the level to fetch its own rows, got %d", len(got))
	}
	if len(f.Argv["spark"]) != 1 {
		t.Fatalf("expected exactly one round trip, got %d", len(f.Argv["spark"]))
	}

	broken := runnerHost{alias: "spark", r: runner.Fake{Err: map[string]error{"spark": runner.ErrFake}}}
	if _, err := p.Children(context.Background(), broken, nil, nil); err == nil {
		t.Fatal("a failing host must surface as an error, not an empty level")
	}
	if _, err := p.Probe(context.Background(), broken); err == nil {
		t.Fatal("a failing host must surface from Probe too")
	}
}

// The path is the contract: a segment that names nothing, and a path deeper
// than the tree, are both ErrNoSuchPath — and a stale attrs blob naming a
// different parent cannot smuggle a consumer into the wrong level.
func TestAPathThatNamesNothingIsErrNoSuchPath(t *testing.T) {
	host := runnerHost{alias: "spark", r: runner.Fake{Out: map[string]string{"spark": "widget-a"}}}
	p := providertest.NewFakeProvider()
	ctx := context.Background()

	for _, path := range [][]string{{"nope"}, {"widget-a", "widget-a-gadget"}, {"a", "b", "c"}} {
		if _, err := p.Children(ctx, host, path, nil); !errors.Is(err, provider.ErrNoSuchPath) {
			t.Fatalf("path %v: want ErrNoSuchPath, got %v", path, err)
		}
	}
	stale := map[string]string{"alias": "spark", "widget": "widget-b"}
	if _, err := p.Children(ctx, host, []string{"widget-a"}, stale); !errors.Is(err, provider.ErrNoSuchPath) {
		t.Fatalf("attrs naming another widget must not resolve: %v", err)
	}
}

// Every kind of the fake tree has five columns; a kind fleet has never seen
// renders IDs only.
func TestEveryFakeKindHasFiveColumns(t *testing.T) {
	p := providertest.NewFakeProvider()
	for _, kind := range []string{providertest.KindCapability, providertest.KindWidget, providertest.KindGadget} {
		if got := p.Columns(kind); len(got) != 5 {
			t.Fatalf("kind %q has %d columns, want 5: %v", kind, len(got), got)
		}
	}
	if p.Columns("") != nil {
		t.Fatal("an unknown kind renders IDs only")
	}
}
