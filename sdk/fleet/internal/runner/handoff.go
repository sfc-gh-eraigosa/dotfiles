package runner

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/pkg/provider"
)

// Quote makes s safe as a single POSIX shell word: it single-quotes s and
// escapes any embedded single quote with the close-backslash-quote-reopen
// idiom. It is THE quoting path for a provider value entering a remote
// command string — updexec.ShQuote and cmd.shQuote are aliases of it, so one
// implementation serves every provider and every update script.
func Quote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// InteractiveArgs is the ssh argv (after "ssh") for a session that OWNS the
// terminal: -t, the multiplexing options, the host. No BatchMode and no
// ConnectTimeout, because the point is to let ssh prompt a human — and the
// socket this session authenticates is the master every later batch command
// rides. It is the one spelling; Exec.interactiveArgs delegates to it.
func InteractiveArgs(host string) []string {
	args := append([]string{"-t"}, muxArgs()...)
	return append(args, host)
}

// HandoffArgv turns a declared provider.Handoff into the argv fleet will run
// against alias. It is pure so a test can assert the argv without a process.
//
// The alias is a parameter and only fleet supplies it: the payload has no
// host field, so a provider cannot move a handoff to a machine the operator
// did not drill into. A remote handoff is `ssh` + InteractiveArgs(alias) +
// the provider's already-quoted command; a local one is the argv verbatim,
// with no shell anywhere to interpret a hostile element.
func HandoffArgv(alias string, h provider.Handoff) ([]string, error) {
	if alias == "" {
		return nil, errors.New("runner: handoff needs an alias")
	}
	switch h.Kind {
	case provider.HandoffRemote:
		if strings.TrimSpace(h.Command) == "" {
			return nil, errors.New("runner: remote handoff has no command")
		}
		return append(append([]string{"ssh"}, InteractiveArgs(alias)...), h.Command), nil
	case provider.HandoffLocal:
		if len(h.Argv) == 0 || h.Argv[0] == "" {
			return nil, errors.New("runner: local handoff has no argv")
		}
		return append([]string(nil), h.Argv...), nil
	default:
		return nil, fmt.Errorf("runner: unknown handoff kind %q", h.Kind)
	}
}

// Command is HandoffArgv as an *exec.Cmd wired to the operator's terminal,
// which is what tea.ExecProcess and `fleet connect` need.
func Command(alias string, h provider.Handoff) (*exec.Cmd, error) {
	argv, err := HandoffArgv(alias, h)
	if err != nil {
		return nil, err
	}
	c := exec.Command(argv[0], argv[1:]...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c, nil
}
