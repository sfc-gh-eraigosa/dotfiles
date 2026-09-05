package updplan

import (
	"testing"
	"time"
)

// TestStepInheritsDefaultsFieldByField: a step overriding only
// retry.attempts must keep the plan's default retry.on and backoff.
func TestStepInheritsDefaultsFieldByField(t *testing.T) {
	yaml := `version: 1
update:
  repos: {}
  steps:
    - id: a
      kind: run
      run: echo hi
      retry: { attempts: 3 }
`
	p, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	st, ok := p.Step("a")
	if !ok {
		t.Fatal("missing step a")
	}
	if st.Retry.Attempts != 3 {
		t.Errorf("Retry.Attempts = %d, want 3", st.Retry.Attempts)
	}
	if len(st.Retry.On) != 1 || st.Retry.On[0] != RetryOnTransport {
		t.Errorf("Retry.On = %v, want [transport] (inherited)", st.Retry.On)
	}
	wantBackoff := builtinDefaults().Retry.Backoff
	if st.Retry.Backoff != wantBackoff {
		t.Errorf("Retry.Backoff = %+v, want inherited %+v", st.Retry.Backoff, wantBackoff)
	}
}

func TestInteractiveStepsDefaultToNoTimeout(t *testing.T) {
	yaml := `version: 1
update:
  repos: {}
  steps:
    - id: a
      kind: run
      run: echo hi
      interactive: true
`
	p, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	st, _ := p.Step("a")
	if st.Timeout != 0 {
		t.Errorf("Timeout = %v, want 0", st.Timeout)
	}
}

func TestExplicitTimeoutOnInteractiveStepIsKept(t *testing.T) {
	yaml := `version: 1
update:
  repos: {}
  steps:
    - id: a
      kind: run
      run: echo hi
      interactive: true
      timeout: 10m
`
	p, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	st, _ := p.Step("a")
	if st.Timeout != 10*time.Minute {
		t.Errorf("Timeout = %v, want 10m", st.Timeout)
	}
}

// TestBackoffScheduleIsExponentialAndCapped pins the exact sequence spec F7
// requires: Initial 5s, Factor 2, Max 2m, with rnd fixed at 0.5 (no jitter
// movement) so the schedule is exactly 5s,10s,20s,40s,80s,2m,2m.
func TestBackoffScheduleIsExponentialAndCapped(t *testing.T) {
	b := Backoff{Initial: 5 * time.Second, Max: 2 * time.Minute, Factor: 2, Jitter: true}
	rnd := func() float64 { return 0.5 }

	want := []time.Duration{
		5 * time.Second,
		10 * time.Second,
		20 * time.Second,
		40 * time.Second,
		80 * time.Second,
		2 * time.Minute,
		2 * time.Minute,
	}
	for n, w := range want {
		got := b.Wait(n+1, rnd)
		if got != w {
			t.Errorf("Wait(%d) = %v, want %v", n+1, got, w)
		}
	}
}

// TestJitterStaysWithinHalfOfTheWait: rnd()=0 -> 0.5x, rnd()=1 -> 1.5x.
func TestJitterStaysWithinHalfOfTheWait(t *testing.T) {
	b := Backoff{Initial: 10 * time.Second, Max: time.Hour, Factor: 2, Jitter: true}

	if got, want := b.Wait(1, func() float64 { return 0 }), 5*time.Second; got != want {
		t.Errorf("Wait(1, rnd=0) = %v, want %v", got, want)
	}
	if got, want := b.Wait(1, func() float64 { return 1 }), 15*time.Second; got != want {
		t.Errorf("Wait(1, rnd=1) = %v, want %v", got, want)
	}
}

func TestRetryOnParsesExitCodes(t *testing.T) {
	yaml := `version: 1
update:
  repos: {}
  steps:
    - id: a
      kind: run
      run: echo hi
      retry: { on: [75, transport] }
`
	p, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	st, _ := p.Step("a")
	want := []RetryOn{"exit:75", RetryOnTransport}
	if len(st.Retry.On) != len(want) {
		t.Fatalf("Retry.On = %v, want %v", st.Retry.On, want)
	}
	for i := range want {
		if st.Retry.On[i] != want[i] {
			t.Errorf("Retry.On[%d] = %q, want %q", i, st.Retry.On[i], want[i])
		}
	}
}
