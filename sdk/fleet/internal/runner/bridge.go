package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/pkg/provider"
)

// Forward is one port bridge: the workstation's loopback Local to the host's
// loopback Remote. Both sides are loopback by construction — there is no
// address field to fill — so a bridge can neither expose a port to the LAN
// nor turn a fleet machine into a jump host.
type Forward struct {
	Local  int
	Remote int
}

// CtxRunner is an OPTIONAL capability of a Runner (the module's precedent is
// updexec's interactiveCtxRunner: adding a method to Runner itself would
// ripple into every other package's test double). RunCtx is the batch lane
// with a context and a full result: stdin in; stdout, stderr and exit code
// out. A non-zero exit is a result, not an error — err means the command
// could not be run at all, or ctx expired. It is what provider.Host wraps.
type CtxRunner interface {
	RunCtx(ctx context.Context, host, stdin string, argv ...string) (provider.ExecResult, error)
}

// BridgeRunner is an OPTIONAL capability of a Runner: one background `ssh -N`
// carrying every forward for a host, alive exactly as long as ctx.
type BridgeRunner interface {
	RunBridgeCtx(ctx context.Context, host string, forwards []Forward) (lines <-chan string, done <-chan error)
}

// BridgeArgv is the argv (including "ssh") for one background bridge to host
// carrying every forward. It is pure so a test can assert it without a
// process. The bridge is a BATCH lane: the same base options every unattended
// command uses (BatchMode, ConnectTimeout, identities, the mux socket) plus
// -N (no remote command) and ExitOnForwardFailure=yes, so a busy port makes
// ssh exit with the reason instead of running a half-working set. Each -L is
// 127.0.0.1:<local>:127.0.0.1:<remote>; the alias is last and nothing follows.
//
// Local must already be resolved (the manager allocates a 0 before it gets
// here) and both ports must be in range; anything else is refused.
func (e Exec) BridgeArgv(host string, forwards []Forward) ([]string, error) {
	if host == "" {
		return nil, errors.New("runner: bridge needs an alias")
	}
	if len(forwards) == 0 {
		return nil, errors.New("runner: bridge needs at least one forward")
	}
	base := e.baseArgs(host)
	args := []string{"ssh", "-N", "-o", "ExitOnForwardFailure=yes"}
	args = append(args, base[:len(base)-1]...) // every base option, host deferred to last
	for _, f := range forwards {
		if f.Local < 1 || f.Local > 65535 {
			return nil, fmt.Errorf("runner: bridge local port %d is out of range 1–65535 (allocate before building argv)", f.Local)
		}
		if f.Remote < 1 || f.Remote > 65535 {
			return nil, fmt.Errorf("runner: bridge remote port %d is out of range 1–65535", f.Remote)
		}
		args = append(args, "-L", fmt.Sprintf("127.0.0.1:%d:127.0.0.1:%d", f.Local, f.Remote))
	}
	return append(args, host), nil
}

// RunBridgeCtx starts the bridge BridgeArgv describes and keeps it alive until
// ctx is done, when the local ssh child is killed (not abandoned) with the
// same WaitDelay discipline as RunStreamCtx. ssh's own output — the reason a
// forward failed — arrives on lines; done reports the exit. On Linux the
// child also gets a death signal, so a fleet that is killed outright takes
// its bridges with it.
func (e Exec) RunBridgeCtx(ctx context.Context, host string, forwards []Forward) (<-chan string, <-chan error) {
	argv, err := e.BridgeArgv(host, forwards)
	if err != nil {
		lines := make(chan string)
		close(lines)
		done := make(chan error, 1)
		done <- err
		return lines, done
	}
	c := exec.CommandContext(ctx, argv[0], argv[1:]...)
	c.Stdin = strings.NewReader("")
	setDeathSignal(c)
	return streamCombined(c)
}

// RunCtx is the batch lane with a context and a full result. A non-zero exit
// is returned in ExitCode with a nil error; a context that expires returns
// ctx.Err() so a hung command is distinguishable from one that failed.
func (e Exec) RunCtx(ctx context.Context, host, stdin string, argv ...string) (provider.ExecResult, error) {
	c := exec.CommandContext(ctx, "ssh", append(e.baseArgs(host), argv...)...)
	c.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	c.Stdout, c.Stderr = &out, &errb
	c.WaitDelay = waitDelay
	err := c.Run()
	res := provider.ExecResult{Stdout: out.String(), Stderr: errb.String()}
	if ctx.Err() != nil {
		return res, ctx.Err()
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	}
	return res, err
}

// RunBridgeCtx mirrors the bridge lane: Block[host] simulates an ssh -N that
// runs until cancelled (done carries ctx.Err()); Err[host] one that fails at
// once, as a busy port would; otherwise the bridge "comes up" and Out[host]
// is replayed as its lines before done closes cleanly on cancellation.
func (f Fake) RunBridgeCtx(ctx context.Context, host string, _ []Forward) (<-chan string, <-chan error) {
	lines := make(chan string, 64)
	done := make(chan error, 1)
	if err, ok := f.Err[host]; ok {
		close(lines)
		done <- err
		return lines, done
	}
	go func() {
		defer close(lines)
		for _, l := range strings.Split(f.Out[host], "\n") {
			if strings.TrimSpace(l) != "" {
				lines <- l
			}
		}
		<-ctx.Done()
		done <- ctx.Err()
	}()
	return lines, done
}

// RunCtx honours Out, Err, Stdin and Block, and records the argv in Argv so a
// provider test can assert what it ran on which host.
func (f Fake) RunCtx(ctx context.Context, host, stdin string, argv ...string) (provider.ExecResult, error) {
	if f.Stdin != nil {
		f.Stdin[host] = stdin
	}
	if f.Argv != nil {
		f.Argv[host] = append(f.Argv[host], argv)
	}
	if f.Block[host] {
		<-ctx.Done()
		return provider.ExecResult{}, ctx.Err()
	}
	if err, ok := f.Err[host]; ok {
		return provider.ExecResult{}, err
	}
	return provider.ExecResult{Stdout: f.Out[host]}, nil
}
