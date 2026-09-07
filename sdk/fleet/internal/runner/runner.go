// Package runner is the single seam through which fleet touches a remote
// machine. Everything else in the tool is pure, so tests never open a socket.
package runner

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// waitDelay bounds Exec.RunStreamCtx's Wait() after a kill (see its
// comment). One second is generous for real ssh/install output to flush,
// while still keeping a controller-enforced timeout tight.
const waitDelay = 1 * time.Second

// Runner executes a command on a remote host.
type Runner interface {
	// Run executes non-interactively and returns stdout.
	Run(host string, argv ...string) (string, error)
	// RunInteractive hands the terminal to the remote command, so prompts
	// (notably install.sh's sudo prompt) reach the operator.
	RunInteractive(host string, argv ...string) error
	// RunStdin executes non-interactively with stdin piped to the remote
	// command. It exists so a sudo password can reach `sudo -S` WITHOUT ever
	// appearing in argv: /proc/<pid>/cmdline is world-readable on both ends,
	// so a secret passed as an argument leaks to every user on the box.
	RunStdin(host, stdin string, argv ...string) (string, error)
	// RunVia executes on host by relaying through peer (ssh -J). It exists for
	// the wake ladder: when the workstation cannot resolve a sleeping host at
	// layer 2, a peer that CAN reach it becomes the transport. The peer is a
	// hop only — authentication stays end-to-end workstation -> host, so the
	// peer never needs the workstation's keys.
	RunVia(peer, host string, argv ...string) (string, error)
	// RunStream is RunStdin that reports progress as it happens: every output
	// line is delivered on `lines` (closed when the command ends) and the final
	// error on `done`. It exists because output captured only at completion
	// tells the operator nothing while a ten-minute install is in flight.
	RunStream(host, stdin string, argv ...string) (lines <-chan string, done <-chan error)
	// RunStreamCtx is RunStream with a controller-enforced deadline: when ctx
	// is done before the remote command finishes, the LOCAL ssh child is
	// killed (not merely abandoned), so a per-attempt timeout in the
	// executor actually bounds wall-clock time instead of leaking a process
	// that outlives the caller's patience for it.
	RunStreamCtx(ctx context.Context, host, stdin string, argv ...string) (lines <-chan string, done <-chan error)
}

// Line is one line of remote output plus which stream produced it.
type Line struct {
	Text   string
	Stderr bool
}

// SplitStreamer is an OPTIONAL capability of a Runner: RunStreamCtx with the
// two streams kept APART, so a consumer can tell a warning from progress.
//
// It is deliberately NOT part of Runner: adding a method there would ripple
// into every other package's Runner test double (the same reasoning as
// updexec's interactiveCtxRunner). A consumer type-asserts for it and falls
// back to the merged stream, where every line is stdout.
type SplitStreamer interface {
	RunSplitStreamCtx(ctx context.Context, host, stdin string, argv ...string) (lines <-chan Line, done <-chan error)
}

// controlPersist is how long a master connection outlives the command that
// opened it. Long enough that a poll, then an update, then a config transfer
// all ride one authentication; short enough that a forgotten session does not
// linger for the rest of the day.
const controlPersist = "10m"

// controlPath uses %C — a fixed-length hash of (local host, remote host, port,
// user) — rather than the conventional %r@%h:%p. A literal path grows with the
// user and host name and can exceed the ~104-byte limit on a unix socket path,
// at which point multiplexing fails SILENTLY and every command starts
// prompting again: the exact symptom this exists to remove.
const controlPath = "~/.ssh/fleet-mux-%C"

// muxArgs enables SSH connection multiplexing.
//
// This is the answer to "stop asking me for credentials on every command". The
// first connection to a host authenticates normally — interactively, on a real
// terminal, with fleet never seeing the secret — and every later command rides
// that same socket and skips authentication ENTIRELY, including under
// BatchMode. It is strictly better than holding a password in memory: nothing
// is stored, it works for key and password auth alike, and it removes a full
// handshake per command.
//
// FLEET_NO_MUX is the escape hatch. Multiplexing interacts badly with some
// jump-host setups and with stale sockets, and an operator who hits that needs
// a way out that is not "edit the binary".
// MuxArgs exposes the multiplexing options to callers that shell out to ssh
// themselves — notably the TUI's interactive session, which must open the very
// socket the unattended commands will reuse.
func MuxArgs() []string { return muxArgs() }

func muxArgs() []string {
	if os.Getenv("FLEET_NO_MUX") != "" {
		return nil
	}
	return []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=" + controlPath,
		"-o", "ControlPersist=" + controlPersist,
	}
}

// Exec is the real SSH-backed runner.
//
// User and Identities exist for probes addressed by RAW IP. No per-alias Host
// block in ~/.ssh/config can match an address, so ssh offers neither the fleet
// user nor the fleet key and the probe fails on credentials rather than on
// reachability. Both are empty for every alias-addressed call, which keeps
// ssh_config the single source of truth there. ssh expands a leading ~ in -i
// itself, so paths are passed through as written in the config.
type Exec struct {
	ConnectTimeout string
	User           string
	Identities     []string
}

// identityArgs presents the credentials explicitly. It yields nothing when
// neither field is set, so the alias-addressed paths keep their exact argv.
func (e Exec) identityArgs() []string {
	var args []string
	if e.User != "" {
		args = append(args, "-l", e.User)
	}
	for _, id := range e.Identities {
		if id != "" {
			args = append(args, "-i", id)
		}
	}
	return args
}

func (e Exec) timeout() string {
	if e.ConnectTimeout == "" {
		return "6"
	}
	return e.ConnectTimeout
}

// baseArgs is the one place the unattended ssh options are spelled out, so no
// remote path can drift into a different set — a path missing the mux options
// would authenticate separately and prompt again.
func (e Exec) baseArgs(host string) []string {
	args := []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=" + e.timeout()}
	args = append(args, e.identityArgs()...)
	args = append(args, muxArgs()...)
	return append(args, host)
}

// interactiveArgs is baseArgs for a session that OWNS the terminal: no
// BatchMode, because the whole point is to let ssh prompt. This is the
// connection that establishes the master socket every later command reuses.
func (e Exec) interactiveArgs(host string) []string {
	args := append([]string{"-t"}, muxArgs()...)
	return append(args, host)
}

func (e Exec) Run(host string, argv ...string) (string, error) {
	out, err := exec.Command("ssh", append(e.baseArgs(host), argv...)...).Output()
	return strings.TrimSpace(string(out)), err
}

func (e Exec) RunInteractive(host string, argv ...string) error {
	c := exec.Command("ssh", append(e.interactiveArgs(host), argv...)...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// RunInteractiveCtx is RunInteractive with a controller-enforced deadline:
// when ctx is done before the interactive remote command exits, the LOCAL
// ssh child is killed (not merely abandoned) — the same guarantee
// RunStreamCtx gives the batch lane, extended to the lane that owns a real
// terminal. Without it, a caller enforcing a deadline on an interactive step
// (updexec.Console.Interactive) could only detect the timeout, never
// actually stop the child holding the terminal (and, transitively, the
// clone a later restore step needs).
func (e Exec) RunInteractiveCtx(ctx context.Context, host string, argv ...string) error {
	c := exec.CommandContext(ctx, "ssh", append(e.interactiveArgs(host), argv...)...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	c.WaitDelay = waitDelay
	return c.Run()
}

// viaArgs builds the relay argv. Split out from RunVia so the ordering — which
// decides WHICH machine the command lands on — is asserted by a unit test
// instead of by opening a socket.
func viaArgs(peer, host, timeout string, argv []string) []string {
	base := []string{
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=" + timeout,
	}
	// Deliberately NOT multiplexed. The relay lane exists because the direct
	// lane failed, so reusing a direct master here is backwards — and on a
	// client older than OpenSSH 8.4 the ControlPath token %C hashes only
	// %l%h%p%r (no %j), which makes the relayed and direct sockets the SAME
	// file: a live direct master would silently win and the peer would never
	// be used at all.
	//
	// The sharper reason survives even where %j is included: ConnectTimeout
	// does not apply to an already-established master, and the wake ladder's
	// budget is sized on the assumption that probing a dead host costs one
	// connect timeout. A hung master would blow it.
	//
	// ControlPath=none rather than merely omitting the options, so an
	// operator's own ssh_config cannot multiplex this lane behind our back.
	base = append(base, "-o", "ControlPath=none")
	base = append(base, "-J", peer, host)
	return append(base, argv...)
}

func (e Exec) RunVia(peer, host string, argv ...string) (string, error) {
	out, err := exec.Command("ssh", viaArgs(peer, host, e.timeout(), argv)...).Output()
	return strings.TrimSpace(string(out)), err
}

func (e Exec) RunStdin(host, stdin string, argv ...string) (string, error) {
	base := e.baseArgs(host)
	c := exec.Command("ssh", append(base, argv...)...)
	c.Stdin = strings.NewReader(stdin)
	// CombinedOutput: sudo writes its failure text to stderr, and that text is
	// the whole diagnosis when authentication fails.
	out, err := c.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// RunStream pipes the remote command's output back line by line. It is
// RunStreamCtx with a background (never-cancelled) context — no per-attempt
// deadline.
func (e Exec) RunStream(host, stdin string, argv ...string) (<-chan string, <-chan error) {
	return e.RunStreamCtx(context.Background(), host, stdin, argv...)
}

// RunSplitStreamCtx streams the remote command with stdout and stderr on
// SEPARATE pipes, tagging each line with the stream that produced it, so a
// consumer can tell a warning from progress. A host that writes
// "WARNING: apt-get update failed" and exits 0 is the case this exists for.
//
// The cost of the split is that ordering BETWEEN the two streams is arrival
// order rather than the remote's exact interleaving (ordering WITHIN each
// stream is exact) — accepted in docs/mbo/designs/fleet-error-view.md §3.1,
// which is also why the TUI keeps stderr in the merged log pane and stamps
// every line with its arrival time.
//
// Both pipes MUST be drained CONCURRENTLY: one goroutine scanning stdout to
// completion and then stderr would let the unread pipe's buffer fill and wedge
// the remote command. Hence one scanner per pipe, a WaitGroup, and a single
// closer.
func (e Exec) RunSplitStreamCtx(ctx context.Context, host, stdin string, argv ...string) (<-chan Line, <-chan error) {
	lines := make(chan Line, 256)
	done := make(chan error, 1)

	c := exec.CommandContext(ctx, "ssh", append(e.baseArgs(host), argv...)...)
	c.Stdin = strings.NewReader(stdin)
	outR, outW := io.Pipe()
	errR, errW := io.Pipe()
	c.Stdout, c.Stderr = outW, errW
	// WaitDelay bounds how long Wait() waits for output plumbing to drain
	// after a kill: without it, a killed ssh whose remote (or, in tests, a
	// stub) leaves a GRANDCHILD holding a pipe open can make Wait block for as
	// long as that orphan lives, silently defeating the whole point of a
	// controller-enforced deadline. After the delay, Wait force-closes the
	// pipes and returns instead.
	c.WaitDelay = waitDelay

	if err := c.Start(); err != nil {
		close(lines)
		done <- err
		return lines, done
	}

	go func() {
		// Closing both writers ends the scanners below; without it a reader
		// would block forever after the process exits.
		err := c.Wait()
		_ = outW.Close()
		_ = errW.Close()
		done <- err
	}()

	var wg sync.WaitGroup
	scan := func(r io.Reader, isErr bool) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024) // install logs have long lines
		for sc.Scan() {
			lines <- Line{Text: sc.Text(), Stderr: isErr}
		}
	}
	wg.Add(2)
	go scan(outR, false)
	go scan(errR, true)
	go func() {
		wg.Wait()
		close(lines)
	}()

	return lines, done
}

// RunStreamCtx is RunSplitStreamCtx with the tag dropped, so there is exactly
// ONE streaming implementation and the merged path can never drift from the
// split one. ctx is wired through exec.CommandContext: when its deadline (or
// cancellation) fires before the remote command exits, the local ssh child is
// killed, not merely abandoned — which is what lets the executor enforce a
// per-attempt timeout that actually bounds wall-clock time.
func (e Exec) RunStreamCtx(ctx context.Context, host, stdin string, argv ...string) (<-chan string, <-chan error) {
	split, done := e.RunSplitStreamCtx(ctx, host, stdin, argv...)
	lines := make(chan string, 256)
	go func() {
		defer close(lines)
		for l := range split {
			lines <- l.Text
		}
	}()
	return lines, done
}

// ErrFake is the canned failure used by Fake.
var ErrFake = errors.New("runner: fake ssh failure")

// Fake is a table-driven Runner for tests. Stdin records what was piped to
// each host so a test can assert a secret went over stdin and not argv; Via
// records which peer relayed to each host so a wake test can assert the peer
// ranking chose the peer it meant to.
type Fake struct {
	Out map[string]string
	// ErrOut is what the host writes to STDERR, replayed by
	// RunSplitStreamCtx after Out. A test scripts either or both.
	ErrOut map[string]string
	Err    map[string]error
	Stdin  map[string]string
	Via    map[string]string // host -> peer that relayed
	// Block, when true for a host, makes RunStreamCtx simulate a remote
	// command that never finishes on its own: it waits on ctx.Done() and
	// sends ctx.Err() on done, so a test can drive the controller-timeout
	// path deterministically without a real blocking process.
	Block map[string]bool
}

func (f Fake) Run(host string, _ ...string) (string, error) {
	if err, ok := f.Err[host]; ok {
		return "", err
	}
	return f.Out[host], nil
}

func (f Fake) RunInteractive(host string, _ ...string) error {
	if err, ok := f.Err[host]; ok {
		return err
	}
	return nil
}

// RunInteractiveCtx honours ctx like RunStreamCtx: when Block[host] is set
// it waits on ctx.Done() and returns ctx.Err(), simulating an interactive
// remote command a controller timeout has to kill; otherwise it behaves
// exactly like RunInteractive.
func (f Fake) RunInteractiveCtx(ctx context.Context, host string, argv ...string) error {
	if f.Block[host] {
		<-ctx.Done()
		return ctx.Err()
	}
	return f.RunInteractive(host, argv...)
}

func (f Fake) RunVia(peer, host string, _ ...string) (string, error) {
	if f.Via != nil {
		f.Via[host] = peer
	}
	// The relay is only a transport: the outcome belongs to the TARGET, so a
	// reachable peer must not make an unreachable host look like a success.
	if err, ok := f.Err[host]; ok {
		return "", err
	}
	return f.Out[host], nil
}

// RunStream replays Out[host] as lines, so a test can drive the streaming
// path deterministically with no process involved. It delegates to
// RunStreamCtx with a background context, matching Exec's RunStream/
// RunStreamCtx relationship.
func (f Fake) RunStream(host, stdin string, argv ...string) (<-chan string, <-chan error) {
	return f.RunStreamCtx(context.Background(), host, stdin, argv...)
}

// RunSplitStreamCtx replays Out[host] as stdout and ErrOut[host] as stderr,
// unless Block[host] is set — in which case it waits on ctx.Done() and reports
// ctx.Err(), simulating a remote command a controller timeout has to kill.
//
// It records stdin exactly as the merged path did: several tests assert the
// sudo secret travelled over stdin and NOT argv.
func (f Fake) RunSplitStreamCtx(ctx context.Context, host, stdin string, _ ...string) (<-chan Line, <-chan error) {
	lines := make(chan Line, 64)
	done := make(chan error, 1)
	if f.Stdin != nil {
		f.Stdin[host] = stdin
	}
	if f.Block[host] {
		go func() {
			defer close(lines)
			<-ctx.Done()
			done <- ctx.Err()
		}()
		return lines, done
	}
	go func() {
		defer close(lines)
		for _, l := range strings.Split(f.Out[host], "\n") {
			if strings.TrimSpace(l) != "" {
				lines <- Line{Text: l}
			}
		}
		for _, l := range strings.Split(f.ErrOut[host], "\n") {
			if strings.TrimSpace(l) != "" {
				lines <- Line{Text: l, Stderr: true}
			}
		}
		done <- f.Err[host]
	}()
	return lines, done
}

// RunStreamCtx is the split stream with the tag dropped — the same
// relationship Exec's two methods have, so the fake cannot drift from the
// real runner in the one way that would matter.
func (f Fake) RunStreamCtx(ctx context.Context, host, stdin string, argv ...string) (<-chan string, <-chan error) {
	split, done := f.RunSplitStreamCtx(ctx, host, stdin, argv...)
	lines := make(chan string, 64)
	go func() {
		defer close(lines)
		for l := range split {
			lines <- l.Text
		}
	}()
	return lines, done
}

func (f Fake) RunStdin(host, stdin string, _ ...string) (string, error) {
	if f.Stdin != nil {
		f.Stdin[host] = stdin
	}
	if err, ok := f.Err[host]; ok {
		return "", err
	}
	return f.Out[host], nil
}
