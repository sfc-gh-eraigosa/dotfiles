package runner

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"
)

var loopbackForward = regexp.MustCompile(`^127\.0\.0\.1:\d+:127\.0\.0\.1:\d+$`)

// F22a: every -L binds the workstation's loopback and targets the HOST's
// loopback — no other address on either side, so a plugin cannot use a
// fleet machine as a jump host or expose a port to the LAN. The alias is
// last and nothing follows it: there is no remote command, and no -t.
func TestBridgeArgvTargetsOnlyTheHostsLoopback(t *testing.T) {
	got, err := (Exec{}).BridgeArgv("spark", []Forward{{Local: 8080, Remote: 80}, {Local: 41234, Remote: 11434}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != "ssh" || got[len(got)-1] != "spark" {
		t.Fatalf("argv must be ssh … alias, got %q", got)
	}
	var forwards []string
	for i := 0; i+1 < len(got); i++ {
		if got[i] == "-L" {
			forwards = append(forwards, got[i+1])
		}
	}
	if len(forwards) != 2 {
		t.Fatalf("expected two -L forwards, got %q in %q", forwards, got)
	}
	for _, f := range forwards {
		if !loopbackForward.MatchString(f) {
			t.Fatalf("forward %q is not loopback:port:loopback:port", f)
		}
	}
	if forwards[0] != "127.0.0.1:8080:127.0.0.1:80" || forwards[1] != "127.0.0.1:41234:127.0.0.1:11434" {
		t.Fatalf("forwards = %q", forwards)
	}
	if contains(got, "-t") {
		t.Fatalf("a bridge must not request a tty: %q", got)
	}
	for _, el := range got {
		if strings.Contains(el, "0.0.0.0") || strings.Contains(el, "*:") {
			t.Fatalf("a bridge must never bind a non-loopback address: %q", got)
		}
	}
}

// F22a: the bridge is a batch lane — the SAME base options every unattended
// command uses (BatchMode, ConnectTimeout, identities, the mux socket) plus
// -N (no remote command) and ExitOnForwardFailure, so a busy port makes ssh
// exit with the reason instead of running a half-working set.
func TestBridgeArgvCarriesTheMuxOptionsAndExitOnForwardFailure(t *testing.T) {
	e := Exec{ConnectTimeout: "3"}
	got, err := e.BridgeArgv("spark", []Forward{{Local: 3080, Remote: 3080}})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(got, "-N") {
		t.Fatalf("bridge argv lacks -N: %q", got)
	}
	for _, pair := range [][2]string{{"-o", "ExitOnForwardFailure=yes"}, {"-o", "BatchMode=yes"}, {"-o", "ConnectTimeout=3"}} {
		if !hasPair(got, pair[0], pair[1]) {
			t.Fatalf("bridge argv lacks %q %q: %q", pair[0], pair[1], got)
		}
	}
	mux := MuxArgs()
	for i := 0; i+1 < len(mux); i += 2 {
		if !hasPair(got, mux[i], mux[i+1]) {
			t.Fatalf("bridge argv lacks mux option %q %q: %q", mux[i], mux[i+1], got)
		}
	}
	// The base options are the ones baseArgs spells, in the same order, so
	// the bridge lane cannot drift from the other batch lanes.
	base := e.baseArgs("spark")
	if !strings.Contains(strings.Join(got, " "), strings.Join(base[:len(base)-1], " ")) {
		t.Fatalf("bridge argv %q does not carry baseArgs %q verbatim", got, base)
	}
}

// Zero forwards, or a port the manager should have resolved or validated
// already, is refused before a process exists.
func TestBridgeArgvRefusesBadInput(t *testing.T) {
	bad := []struct {
		alias string
		fw    []Forward
	}{
		{"spark", nil},
		{"spark", []Forward{}},
		{"", []Forward{{Local: 80, Remote: 80}}},
		{"spark", []Forward{{Local: 0, Remote: 80}}}, // the manager allocates BEFORE building argv
		{"spark", []Forward{{Local: 80, Remote: 0}}},
		{"spark", []Forward{{Local: 65536, Remote: 80}}},
		{"spark", []Forward{{Local: 80, Remote: 70000}}},
	}
	for i, c := range bad {
		if _, err := (Exec{}).BridgeArgv(c.alias, c.fw); err == nil {
			t.Fatalf("case %d: expected an error for alias=%q forwards=%v", i, c.alias, c.fw)
		}
	}
}

// F22b: cancelling the bridge's context kills the local ssh child and closes
// done within WaitDelay — also when cancelled before it ever came up.
func TestACancelledBridgeIsKilledWithinWaitDelay(t *testing.T) {
	stubSSH(t, "sleep 30")
	for _, pre := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		if pre {
			cancel()
		} else {
			time.AfterFunc(100*time.Millisecond, cancel)
		}
		lines, done := (Exec{}).RunBridgeCtx(ctx, "spark", []Forward{{Local: 8080, Remote: 80}})
		for range lines {
		}
		select {
		case err := <-done:
			if err == nil {
				t.Fatalf("pre-cancelled=%v: expected an error from a killed bridge, got nil", pre)
			}
		case <-time.After(waitDelay + 2*time.Second):
			t.Fatalf("pre-cancelled=%v: bridge did not stop within WaitDelay", pre)
		}
		cancel()
	}
}

// Fake mirrors the bridge lane: Block simulates an ssh -N that runs until
// cancelled; Err simulates one that fails at once (a busy port).
func TestFakeBridgeHonoursBlockAndErr(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := Fake{Block: map[string]bool{"spark": true}, Err: map[string]error{"nano": ErrFake}}
	lines, done := f.RunBridgeCtx(ctx, "spark", []Forward{{Local: 1, Remote: 1}})
	select {
	case <-done:
		t.Fatal("a blocked bridge must not finish before cancellation")
	case <-time.After(50 * time.Millisecond):
	}
	cancel()
	for range lines {
	}
	if err := <-done; err == nil {
		t.Fatal("a cancelled blocked bridge must report ctx.Err()")
	}
	_, done = f.RunBridgeCtx(context.Background(), "nano", []Forward{{Local: 1, Remote: 1}})
	if err := <-done; err != ErrFake {
		t.Fatalf("Err[host] must surface as the bridge's failure, got %v", err)
	}
}

// F6c's runner half: a non-zero exit is a RESULT — stdout, stderr and the
// exit code all come back and err is nil — and stdin reaches the command.
func TestRunCtxReturnsStderrAndExitCodeWithoutAnError(t *testing.T) {
	stubSSH(t, "printf out; printf err >&2; exit 3")
	res, err := (Exec{}).RunCtx(context.Background(), "spark", "", "true")
	if err != nil {
		t.Fatalf("a non-zero exit must not be an error: %v", err)
	}
	if res.Stdout != "out" || res.Stderr != "err" || res.ExitCode != 3 {
		t.Fatalf("got %+v, want out/err/3", res)
	}

	stubSSH(t, "cat")
	res, err = (Exec{}).RunCtx(context.Background(), "spark", "hello\n", "cat")
	if err != nil {
		t.Fatal(err)
	}
	if res.Stdout != "hello\n" || res.ExitCode != 0 {
		t.Fatalf("stdin was not delivered: %+v", res)
	}
}

// F7d's runner half: a command that blocks is cancelled by its context, so a
// hung built-in probe never hangs the level load.
func TestRunCtxIsCancelledByItsContext(t *testing.T) {
	stubSSH(t, "sleep 30")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := (Exec{}).RunCtx(ctx, "spark", "", "sleep")
	if err == nil {
		t.Fatal("expected an error from a cancelled command")
	}
	if time.Since(start) > waitDelay+2*time.Second {
		t.Fatal("RunCtx did not honour its context in time")
	}
}

// Fake.RunCtx honours Out, Err, Stdin and Block, and records the argv so a
// provider test can assert what it ran.
func TestFakeRunCtxHonoursOutErrStdinAndBlock(t *testing.T) {
	f := Fake{Out: map[string]string{"spark": "hi"}, Err: map[string]error{"nano": ErrFake}, Stdin: map[string]string{}, Block: map[string]bool{"pi": true}}
	res, err := f.RunCtx(context.Background(), "spark", "in", "sh", "-c", "x")
	if err != nil || res.Stdout != "hi" || res.ExitCode != 0 || f.Stdin["spark"] != "in" {
		t.Fatalf("got %+v %v stdin=%q", res, err, f.Stdin["spark"])
	}
	if _, err := f.RunCtx(context.Background(), "nano", "", "x"); err != ErrFake {
		t.Fatalf("Err[host] must surface, got %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := f.RunCtx(ctx, "pi", "", "x"); err == nil {
		t.Fatal("a blocked host must return ctx.Err()")
	}
}

// The two new lanes are OPTIONAL capabilities of a Runner — the module's
// precedent (updexec's interactiveCtxRunner): adding them to Runner itself
// would ripple into every other package's test double. Exec and Fake carry
// both; a consumer type-asserts and reports a runner that cannot.
func TestExecAndFakeCarryTheCtxAndBridgeCapabilities(t *testing.T) {
	var _ CtxRunner = Exec{}
	var _ CtxRunner = Fake{}
	var _ BridgeRunner = Exec{}
	var _ BridgeRunner = Fake{}
}
