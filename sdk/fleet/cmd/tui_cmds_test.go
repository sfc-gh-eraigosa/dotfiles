package cmd

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// scriptSpy is a runner.Runner double that records the script argument of
// every RunStreamCtx call, in order, so a test can assert on exactly what
// beginStream/the executor sent — not just what the fake returned.
type scriptSpy struct {
	scripts []string
}

func (s *scriptSpy) Run(string, ...string) (string, error)              { return "", nil }
func (s *scriptSpy) RunInteractive(string, ...string) error             { return nil }
func (s *scriptSpy) RunStdin(string, string, ...string) (string, error) { return "", nil }
func (s *scriptSpy) RunVia(string, string, ...string) (string, error)   { return "", nil }

func (s *scriptSpy) RunStream(host, stdin string, argv ...string) (<-chan string, <-chan error) {
	return s.RunStreamCtx(context.Background(), host, stdin, argv...)
}

func (s *scriptSpy) RunStreamCtx(_ context.Context, _ string, _ string, argv ...string) (<-chan string, <-chan error) {
	if len(argv) > 0 {
		s.scripts = append(s.scripts, argv[0])
	}
	lines := make(chan string)
	done := make(chan error, 1)
	close(lines)
	done <- nil
	return lines, done
}

// twoStepPlan is a sync step and a non-interactive run step, independent of
// one another (no needs), so both always run regardless of the fake sync
// precheck's canned (non-matching) output.
func twoStepPlan() updplan.Plan {
	return updplan.Plan{
		Source: "test",
		Repos: map[string]updplan.Repo{
			"r": {Name: "r", Path: "~/git/r", Local: updplan.LocalSkip, Restore: true},
		},
		Steps: []updplan.Step{
			{ID: "sync1", Kind: updplan.KindSync, Repo: "r",
				Expect: updplan.Expect{Exit: []int{0}}, Retry: updplan.Retry{Attempts: 1}},
			{ID: "run1", Kind: updplan.KindRun, Run: "true",
				Expect: updplan.Expect{Exit: []int{0}}, Retry: updplan.Retry{Attempts: 1}},
		},
	}
}

// TestSudoPreambleIsPerRunStepSession pins that the background lane's sudo
// preamble is scoped to KindRun steps only: a run step's script is primed
// and verified when a credential was supplied, but a sync step's script
// (the precheck, here) never carries the sudo prime — Console/Background
// gate Preamble to updplan.KindRun regardless of what the callback returns.
func TestSudoPreambleIsPerRunStepSession(t *testing.T) {
	spy := &scriptSpy{}
	a := answers{}
	a.appendSecret(probeMarker)
	st := beginStream("host-a", twoStepPlan(), a, spy, "")().(streamStartedMsg).st
	for range st.lines {
	}
	<-st.done

	if len(spy.scripts) != 2 {
		t.Fatalf("expected 2 scripts sent (precheck + run), got %d: %v", len(spy.scripts), spy.scripts)
	}
	syncScript, runScript := spy.scripts[0], spy.scripts[1]

	primeCmd := "sudo -S -p ''"
	if strings.Contains(syncScript, primeCmd) {
		t.Fatalf("a sync step's script must never carry the sudo preamble:\n%s", syncScript)
	}
	if !strings.HasPrefix(runScript, primeCmd) {
		t.Fatalf("a run step's script must start with the sudo prime when a credential is set:\n%s", runScript)
	}
}

// TestHandoffDelegatesToFleetUpdate pins that the interactive handoff's
// self-exec argv is exactly `<self> update <alias> [--file F] [--ref R]
// [--reset]` — there is now exactly ONE definition of "update a host", and
// the interactive lane delegates to it rather than re-implementing a
// remote script.
func TestHandoffDelegatesToFleetUpdate(t *testing.T) {
	got := handoffArgv("/opt/bin/fleet", "host-b", "/etc/fleet/fleet.yaml", "feature/x", true)
	want := []string{"/opt/bin/fleet", "update", "host-b", "--file", "/etc/fleet/fleet.yaml", "--ref", "feature/x", "--reset"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv = %v, want %v", got, want)
	}

	// No --file/--ref/--reset when unset — nothing is appended just to be
	// appended.
	got2 := handoffArgv("fleet", "host-a", "", "", false)
	want2 := []string{"fleet", "update", "host-a"}
	if !reflect.DeepEqual(got2, want2) {
		t.Fatalf("argv = %v, want %v", got2, want2)
	}
}

// TestHandoffEnvNeverCarriesTheSecret pins that the child's environment
// carries only the non-secret prompt answers — never the sudo credential.
// The child prompts for its own credential on its own tty.
func TestHandoffEnvNeverCarriesTheSecret(t *testing.T) {
	a := answers{windows: "s", gemini: "keep"}
	a.appendSecret(probeMarker)
	for _, e := range handoffEnv(a) {
		if strings.Contains(e, probeMarker) {
			t.Fatalf("the credential leaked into the child's environment: %q", e)
		}
	}
}

// TestNeedsTerminalRoutesToInteractiveQueue pins that a Background run that
// stopped on a step needing a terminal (errNeedsTerminal, wrapping
// updexec.ErrNoTerminal) is routed to the interactive queue instead of
// being marked a failed row.
func TestNeedsTerminalRoutesToInteractiveQueue(t *testing.T) {
	// Keep a second host running so pump() cannot immediately dequeue "a"
	// into the interactive handoff — this asserts on the QUEUEING decision
	// itself, not on how soon pump happens to service it.
	m := testModel("a", "b")
	m.updating["a"] = updState{phase: updRunning}
	m.updating["b"] = updState{phase: updRunning}
	m.running = 2

	mm, _ := m.Update(bgUpdateDoneMsg{alias: "a", err: errNeedsTerminal})
	m2 := mm.(tuiModel)

	if len(m2.iaQueue) != 1 || m2.iaQueue[0] != "a" {
		t.Fatalf("expected %q queued for the interactive handoff, got %v", "a", m2.iaQueue)
	}
	if m2.updating["a"].phase == updFail {
		t.Fatal("a host needing a terminal must not be marked failed")
	}
	if m2.iaTotal != 1 {
		t.Fatalf("iaTotal must count the newly-queued host, got %d", m2.iaTotal)
	}
}
