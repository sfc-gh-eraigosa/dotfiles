package cmd

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/reach"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
)

// sleeperRunner models the actual failure this feature exists for: a host that
// is powered ON but asleep at layer 2. Every command fails until something
// rouses it, after which it answers normally. A plain runner.Fake cannot
// express that, because its verdict per host never changes.
type sleeperRunner struct {
	mu    sync.Mutex
	awake map[string]bool
	stamp string
}

func newSleeper(stamp string, hosts ...string) *sleeperRunner {
	s := &sleeperRunner{awake: map[string]bool{}, stamp: stamp}
	for _, h := range hosts {
		s.awake[h] = false
	}
	return s
}

func (s *sleeperRunner) rouse(host string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.awake[host] = true
}

func (s *sleeperRunner) Run(host string, _ ...string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.awake[host] {
		return "", runner.ErrFake
	}
	return s.stamp, nil
}

func (s *sleeperRunner) RunInteractive(string, ...string) error { return nil }
func (s *sleeperRunner) RunStdin(h, _ string, a ...string) (string, error) {
	return s.Run(h, a...)
}
func (s *sleeperRunner) RunVia(_, h string, a ...string) (string, error) { return s.Run(h, a...) }

func stampFor(sha string) string {
	return "commit=" + sha + "\ninstalled_at=1754700000\nbranch=main\nhostname=h\n"
}

// Wake tests never stream; an already-closed stream satisfies the seam.
func (s *sleeperRunner) RunStream(string, string, ...string) (<-chan string, <-chan error) {
	lines := make(chan string)
	done := make(chan error, 1)
	close(lines)
	done <- nil
	return lines, done
}

// F15a — waking a host that answered on the first try would burn the budget
// for nothing. The ladder must fire for unreachable hosts and only those.
func TestWakeFiresOnlyForUnreachableHosts(t *testing.T) {
	cur := strings.Repeat("a", 40)
	r := runner.Fake{
		Out: map[string]string{"up": stampFor(cur)},
		Err: map[string]error{"down": runner.ErrFake},
	}
	base := fakeBaseline{head: cur, ancestor: map[string]bool{cur: true}}

	var mu sync.Mutex
	var woken []string
	w := func(h sshconf.Host, _ []reach.Peer) reach.Result {
		mu.Lock()
		defer mu.Unlock()
		woken = append(woken, h.Alias)
		return reach.Result{}
	}

	collectWake([]sshconf.Host{{Alias: "up"}, {Alias: "down"}}, r, base, testNow, w)

	if len(woken) != 1 || woken[0] != "down" {
		t.Fatalf("ladder must fire for the unreachable host only, fired for %v", woken)
	}
}

// F15b — a woken host reports its REAL drift class, plus provenance so the
// operator learns the fleet holds a power-saving machine rather than a flaky
// one. The note is searchable in the TUI, which is the point.
func TestWokenHostReportsItsRealClassAndHowItWoke(t *testing.T) {
	cur := strings.Repeat("a", 40)
	s := newSleeper(stampFor(cur), "sleeper")
	base := fakeBaseline{head: cur, ancestor: map[string]bool{cur: true}}

	w := func(h sshconf.Host, _ []reach.Peer) reach.Result {
		s.rouse(h.Alias)
		return reach.Result{Woke: true, Via: "peer-x"}
	}

	rows := collectWake([]sshconf.Host{{Alias: "sleeper"}}, s, base, testNow, w)
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if rows[0].Class != "up-to-date" {
		t.Fatalf("class = %q, want the re-probed class, not unreachable", rows[0].Class)
	}
	if rows[0].Note != "woke via peer-x" {
		t.Fatalf("Note = %q, want provenance naming the rescuer", rows[0].Note)
	}
}

// A ladder that runs but fails to wake the host must leave the verdict alone:
// unreachable, and no misleading note.
func TestFailedWakeStillReportsUnreachable(t *testing.T) {
	r := runner.Fake{Err: map[string]error{"dead": runner.ErrFake}}
	base := fakeBaseline{head: strings.Repeat("a", 40)}
	w := func(sshconf.Host, []reach.Peer) reach.Result {
		return reach.Result{Woke: false}
	}

	rows := collectWake([]sshconf.Host{{Alias: "dead"}}, r, base, testNow, w)
	if rows[0].Class != "unreachable" {
		t.Fatalf("class = %q, want unreachable", rows[0].Class)
	}
	if rows[0].Note != "" {
		t.Fatalf("Note = %q, want none for a host that never woke", rows[0].Note)
	}
}

// F15c — the ladder runs INSIDE the existing per-host fan-out, so N sleeping
// hosts cost about one budget of wall clock rather than N. This is the whole
// reason auto-wake can be on by default; if it ever serialises, a fleet with
// several dead hosts becomes unusable.
func TestSleepingHostsWakeConcurrentlyNotSerially(t *testing.T) {
	const n, dwell = 4, 150 * time.Millisecond

	r := runner.Fake{Err: map[string]error{}}
	var hosts []sshconf.Host
	for _, a := range []string{"s1", "s2", "s3", "s4"} {
		r.Err[a] = runner.ErrFake
		hosts = append(hosts, sshconf.Host{Alias: a})
	}
	base := fakeBaseline{head: strings.Repeat("a", 40)}

	var mu sync.Mutex
	inFlight, peak := 0, 0
	w := func(sshconf.Host, []reach.Peer) reach.Result {
		mu.Lock()
		inFlight++
		if inFlight > peak {
			peak = inFlight
		}
		mu.Unlock()

		time.Sleep(dwell)

		mu.Lock()
		inFlight--
		mu.Unlock()
		return reach.Result{}
	}

	start := time.Now()
	collectWake(hosts, r, base, testNow, w)
	elapsed := time.Since(start)

	if peak < 2 {
		t.Fatalf("peak concurrency %d: ladders ran serially", peak)
	}
	if elapsed >= n*dwell {
		t.Fatalf("elapsed %v ~= %d x budget: ladders did not overlap", elapsed, n)
	}
}

// F15d — --no-wake is a hard off switch. A nil waker is the representation, so
// every pre-existing call site keeps its exact old behaviour by construction.
func TestNoWakeSuppressesTheLadderEntirely(t *testing.T) {
	s := newSleeper(stampFor(strings.Repeat("a", 40)), "sleeper")
	base := fakeBaseline{head: strings.Repeat("a", 40)}

	rows := collectWake([]sshconf.Host{{Alias: "sleeper"}}, s, base, testNow, nil)
	if rows[0].Class != "unreachable" {
		t.Fatalf("class = %q, want unreachable with wake disabled", rows[0].Class)
	}
	if rows[0].Note != "" {
		t.Fatalf("Note = %q, want none", rows[0].Note)
	}
}

// The peer list handed to the ladder must exclude the target and prefer hosts
// already known to answer in this run (reach.rankPeers does the ordering; this
// asserts collect actually supplies the knowledge it needs).
func TestWakeReceivesTheOtherFleetHostsAsPeers(t *testing.T) {
	cur := strings.Repeat("a", 40)
	r := runner.Fake{
		Out: map[string]string{"up": stampFor(cur)},
		Err: map[string]error{"down": runner.ErrFake},
	}
	base := fakeBaseline{head: cur, ancestor: map[string]bool{cur: true}}

	var mu sync.Mutex
	var gotPeers []string
	w := func(_ sshconf.Host, peers []reach.Peer) reach.Result {
		mu.Lock()
		defer mu.Unlock()
		for _, p := range peers {
			gotPeers = append(gotPeers, p.Alias)
		}
		return reach.Result{}
	}

	collectWake([]sshconf.Host{{Alias: "up"}, {Alias: "down"}}, r, base, testNow, w)

	for _, p := range gotPeers {
		if p == "down" {
			t.Fatalf("the target must not be offered as its own peer: %v", gotPeers)
		}
	}
	if len(gotPeers) != 1 || gotPeers[0] != "up" {
		t.Fatalf("peers = %v, want the other fleet host", gotPeers)
	}
}

// F18a — a non-positive budget would make every ladder a no-op that still
// costs a round of probes; reject it at the flag rather than silently.
func TestWakeTimeoutMustBePositive(t *testing.T) {
	if err := validateWakeTimeout(0); err == nil {
		t.Fatal("--wake-timeout 0 must be rejected")
	}
	if err := validateWakeTimeout(-time.Second); err == nil {
		t.Fatal("a negative --wake-timeout must be rejected")
	}
	if err := validateWakeTimeout(8 * time.Second); err != nil {
		t.Fatalf("a positive --wake-timeout must be accepted: %v", err)
	}
}
