package updexec

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// waitForCall blocks until io has recorded a batch call whose script holds
// substr, so a test can cancel exactly while that step is in flight.
func waitForCall(t *testing.T, io *fakeIO, substr string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, c := range io.batchCalls() {
			if strings.Contains(c.script, substr) {
				return
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("no batch call containing %q", substr)
}

func runHostAsync(e Executor, ctx context.Context, host string, p updplan.Plan) <-chan HostReport {
	done := make(chan HostReport, 1)
	go func() { done <- e.RunHostCtx(ctx, host, p) }()
	return done
}

func awaitReport(t *testing.T, done <-chan HostReport) HostReport {
	t.Helper()
	select {
	case rep := <-done:
		return rep
	case <-time.After(5 * time.Second):
		t.Fatal("RunHostCtx did not return after cancel")
		return HostReport{}
	}
}

func TestCancelledBeforeStartSendsNothing(t *testing.T) {
	io := newFakeIO()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rep := Executor{IO: io}.RunHostCtx(ctx, "alpha", defaultTestPlan())

	if n := len(io.batchCalls()) + len(io.interactiveCalls()); n != 0 {
		t.Fatalf("a cancelled run sent %d call(s): %+v", n, io.calls)
	}
	if len(rep.Results) != 2 {
		t.Fatalf("got %d results, want one per step: %+v", len(rep.Results), rep.Results)
	}
	for _, r := range rep.Results {
		if r.Status != Cancelled {
			t.Fatalf("step %s = %s, want cancelled", r.Step, r.Status)
		}
	}
	if err := rep.Err(); err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("Err() = %v, want a cancelled error", err)
	}
	if !rep.Cancelled() {
		t.Fatal("Cancelled() = false for a cancelled run")
	}
}

func TestCancelMidStepStopsTheHostWithoutRetrying(t *testing.T) {
	io := newFakeIO().
		on("git rev-parse --git-dir", cleanPrecheck, nil).
		on("orig=$(git symbolic-ref", "fleet: orig=main", nil).
		blockOn("slow-step")
	slow := runStep("slow", "", "slow-step", "dotfiles.sync")
	slow.Retry.On = []updplan.RetryOn{updplan.RetryOnAny}
	p := updplan.Plan{
		Repos: map[string]updplan.Repo{"dotfiles": syncRepo("dotfiles", updplan.LocalSkip)},
		Steps: []updplan.Step{
			syncStep("dotfiles.sync", "dotfiles"),
			slow,
			runStep("after", "", "after-step"),
		},
	}
	sleeps := &sleepRecorder{}
	e := Executor{IO: io, Sleep: sleeps.Sleep}
	ctx, cancel := context.WithCancel(context.Background())
	done := runHostAsync(e, ctx, "alpha", p)
	waitForCall(t, io, "slow-step")
	cancel()
	rep := awaitReport(t, done)

	if r, _ := resultFor(rep, "dotfiles.sync"); r.Status != OK {
		t.Fatalf("dotfiles.sync finished before the cancel and must stay ok: %+v", r)
	}
	r, _ := resultFor(rep, "slow")
	if r.Status != Cancelled || r.Attempts != 1 {
		t.Fatalf("slow = %+v, want cancelled after 1 attempt (no retry)", r)
	}
	if a, _ := resultFor(rep, "after"); a.Status != Cancelled {
		t.Fatalf("after = %+v, want cancelled", a)
	}
	for _, c := range io.batchCalls() {
		if strings.Contains(c.script, "after-step") {
			t.Fatal("a step after the cancel was still sent")
		}
	}
	if s := sleeps.all(); len(s) != 0 {
		t.Fatalf("a cancelled step must not back off: slept %v", s)
	}
}

func TestRestoreStillRunsAfterCancel(t *testing.T) {
	io := newFakeIO().
		on("git rev-parse --git-dir", "state=dirty branch=main", nil).
		on("orig=$(git symbolic-ref", "fleet: orig=main\nfleet: carried stash=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa from=main", nil).
		on(restoreFailedMarker, "", nil).
		blockOn("./build.sh")
	p := updplan.Plan{
		Repos: map[string]updplan.Repo{"dotfiles": syncRepo("dotfiles", updplan.LocalCarry)},
		Steps: []updplan.Step{
			syncStep("dotfiles.sync", "dotfiles"),
			runStep("dotfiles.build", "dotfiles", "./build.sh", "dotfiles.sync"),
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := runHostAsync(Executor{IO: io}, ctx, "alpha", p)
	waitForCall(t, io, "./build.sh")
	cancel()
	rep := awaitReport(t, done)

	if b, _ := resultFor(rep, "dotfiles.build"); b.Status != Cancelled {
		t.Fatalf("dotfiles.build = %+v, want cancelled", b)
	}
	// The operator's stashed work goes back even though the run was cancelled:
	// the restore runs on a context the cancel does not reach.
	if rr, ok := resultFor(rep, "dotfiles.restore"); !ok || rr.Status != OK {
		t.Fatalf("restore must run after a cancel: %+v", rr)
	}
}
