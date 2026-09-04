package updexec

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// --- coverage-closing tests (not named in plan §4, but required by the
// leaf gate's >=90% floor) ---------------------------------------------------

func TestHostReportErrNamesTheFirstNonOkStep(t *testing.T) {
	rep := HostReport{Results: []Result{
		{Step: "a", Status: OK},
		{Step: "b", Status: Failed, Reason: "boom"},
		{Step: "c", Status: OK},
	}}
	if !rep.Failed() {
		t.Fatal("Failed() must be true")
	}
	err := rep.Err()
	if err == nil || err.Error() != "step b: boom" {
		t.Fatalf("Err() = %v, want \"step b: boom\"", err)
	}

	clean := HostReport{Results: []Result{{Step: "a", Status: OK}}}
	if clean.Failed() || clean.Err() != nil {
		t.Fatal("an all-ok report must not be Failed and Err() must be nil")
	}
}

func TestDiscardKeepsNothing(t *testing.T) {
	w, path := (Discard{}).Open("alpha", "header")
	if path != "" {
		t.Fatalf("Discard.Open path = %q, want empty", path)
	}
	w.Line("some line")
	w.Close("footer")
	// Nothing to assert beyond "did not panic" — Discard is a no-op sink.
}

func TestConsoleInteractiveRespectsDeadline(t *testing.T) {
	r := &recordingRunner{}
	c := Console{R: r}

	// No deadline: goes straight to RunInteractive.
	if err := c.Interactive(context.Background(), "alpha", updplan.Step{Kind: updplan.KindRun}, "s"); err != nil {
		t.Fatal(err)
	}

	// A deadline that has ALREADY passed must report DeadlineExceeded, not
	// hang.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(2 * time.Millisecond)
	err := c.Interactive(ctx, "alpha", updplan.Step{Kind: updplan.KindRun}, "s")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
}

func TestRunRunSurfacesAScriptBuildError(t *testing.T) {
	badRepo := updplan.Repo{Name: "x", Path: "~/x;id"}
	st := runStep("x", "x", "echo hi")
	p := updplan.Plan{
		Repos: map[string]updplan.Repo{"x": badRepo},
		Steps: []updplan.Step{st},
	}
	e := Executor{IO: newFakeIO()}
	rep := e.RunHost("alpha", p)
	r, ok := resultFor(rep, "x")
	if !ok || r.Status != Failed {
		t.Fatalf("an invalid repo path must fail the run step before any call: %+v", r)
	}
}

func TestUnknownStepKindFails(t *testing.T) {
	e := Executor{IO: newFakeIO()}
	p := updplan.Plan{Steps: []updplan.Step{{ID: "x", Kind: "bogus"}}}
	rep := e.RunHost("alpha", p)
	r, ok := resultFor(rep, "x")
	if !ok || r.Status != Failed {
		t.Fatalf("an unknown step kind must fail: %+v", r)
	}
}

func TestGhAuthLoginScriptBuildErrorFails(t *testing.T) {
	e := Executor{IO: newFakeIO().on("gh auth status -h gh;id", "", realExitError(t, 1))}
	p := updplan.Plan{Steps: []updplan.Step{
		{ID: "gh.auth", Kind: updplan.KindGhAuth, Hostname: "gh;id", Expect: stdExpect(), Retry: stdRetry(), OnFailure: updplan.OnFailureStop},
	}}
	rep := e.RunHost("alpha", p)
	r, ok := resultFor(rep, "gh.auth")
	if !ok || r.Status != Failed {
		t.Fatalf("an invalid gh-auth hostname must fail cleanly: %+v", r)
	}
}

func TestSleepNoopOnZeroAndRealDefault(t *testing.T) {
	var e Executor
	e.sleep(0) // must be a no-op, not call time.Sleep
	start := time.Now()
	e.sleep(1 * time.Millisecond) // exercises the default time.Sleep path
	if time.Since(start) <= 0 {
		t.Fatal("sleep must actually wait when d > 0 and Sleep is nil")
	}
}

func TestUnexpectedExitWithNilErrIsFailed(t *testing.T) {
	// err == nil (exit 0) but Expect.Exit excludes 0 -> the default
	// "unexpected exit %d" reason branch.
	io := newFakeIO().on("echo hi", "", nil)
	st := runStep("x", "", "echo hi")
	st.Expect = updplan.Expect{Exit: []int{3}}
	e := Executor{IO: io}
	rep := e.RunHost("alpha", updplan.Plan{Steps: []updplan.Step{st}})
	r, ok := resultFor(rep, "x")
	if !ok || r.Status != Failed || r.Reason != "unexpected exit 0" {
		t.Fatalf("r = %+v, want failed with \"unexpected exit 0\"", r)
	}
}

func TestRunSyncUnknownRepoFails(t *testing.T) {
	e := Executor{IO: newFakeIO()}
	p := updplan.Plan{Steps: []updplan.Step{syncStep("x.sync", "ghost")}}
	rep := e.RunHost("alpha", p)
	r, ok := resultFor(rep, "x.sync")
	if !ok || r.Status != Failed {
		t.Fatalf("a sync step targeting an unknown repo must fail: %+v", r)
	}
}
