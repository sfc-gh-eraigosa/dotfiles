// Package provider is the contract between fleet and a provider — the thing
// that knows what lives on a host, level by level, and what an operator can do
// at each row. It is public API: plugin authors import it, and its JSON is the
// wire shape the plugin protocol, `fleet ls --json` and the TUI all read.
//
// Everything here is data. A provider declares a Handoff, a Stream or a Tunnel;
// only fleet turns one into a process, on the host the operator drilled into.
// No type in this package can name a machine or an address — that is what
// keeps a plugin from reaching a fleet member it was not asked about.
//
// The package depends on the standard library only.
package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Node is one row of a level: the TUI's row, the `fleet ls --json` element
// and the RPC result element — one shape, four consumers.
type Node struct {
	ID      string            `json:"id"`      // one path segment; the row's identity across a refresh
	Kind    string            `json:"kind"`    // the provider's namespace ("herdr-session"); fleet never switches on it
	Cells   []string          `json:"cells"`   // positional against Columns(Kind); a short slice renders blanks
	Detail  string            `json:"detail"`  // one-line qualifier for the level status bar
	Leaf    bool              `json:"leaf"`    // enter is a no-op; actions still apply
	Attrs   map[string]string `json:"attrs"`   // opaque provider state carried back on the next call
	Actions []Action          `json:"actions"` // what the operator can do on this row
}

// Action is something an operator can do on a row. Exactly one of Handoff,
// Stream or Tunnel is set; Validate enforces it so nothing downstream has to
// pick. An Unavailable action is LISTED but refused, with the reason.
type Action struct {
	Key         string   `json:"key"`         // exactly one printable rune, carried as a string ("c")
	Label       string   `json:"label"`       // shown in the header strip beside the key
	Unavailable string   `json:"unavailable"` // non-empty: listed but refused, with this reason
	Handoff     *Handoff `json:"handoff"`     // takes the terminal
	Stream      *Stream  `json:"stream"`      // lines into the log pane
	Tunnel      *Tunnel  `json:"tunnel"`      // a host port bridged to 127.0.0.1 on the workstation
}

// HandoffKind says how a Handoff becomes a process.
type HandoffKind string

const (
	// HandoffRemote runs Command on the host over `ssh -t` with the mux options.
	HandoffRemote HandoffKind = "remote"
	// HandoffLocal execs Argv on the workstation with no shell, so a hostile
	// value is an inert argv element.
	HandoffLocal HandoffKind = "local"
)

// Handoff is a process that takes the operator's terminal. It carries no
// host: fleet stamps the level's alias when it runs the handoff.
type Handoff struct {
	Kind    HandoffKind `json:"kind"`    // "remote" | "local"
	Command string      `json:"command"` // remote: a shell command the provider has already quoted
	Argv    []string    `json:"argv"`    // local: argv, no shell
}

// Stream is a remote command whose lines flow into the log pane. Follow marks
// a stream that does not end on its own (logs -f) and must be cancellable.
type Stream struct {
	Command string `json:"command"`
	Follow  bool   `json:"follow"`
}

// Tunnel bridges a port on the host's loopback to the workstation's loopback.
// It carries no address: a forward always targets 127.0.0.1 on the dispatched
// host, so a plugin cannot turn a fleet machine into a jump host.
type Tunnel struct {
	RemotePort int    `json:"remotePort"` // 1–65535, on the host's loopback
	LocalPort  int    `json:"localPort"`  // 0: prefer RemotePort locally, else allocate
	Scheme     string `json:"scheme"`     // "http" | "https" | "" — printed before 127.0.0.1:<local>
	Keeper     string `json:"keeper"`     // optional provider-quoted host command that must be running for RemotePort to listen; fleet runs it for the life of the bridge
}

// TunnelKey is the one reserved key a provider does declare: every Tunnel
// action carries it, so the TUI's `t` (toggle the row's bridge) and the CLI
// agree on which action a row's bridge is — the way enter is the drill key.
const TunnelKey = "t"

// ReservedKeys are the printable runes the HOST TOOL owns, so a provider that
// declares one would be silently shadowed. Validate rejects them — except a
// Tunnel, which must use TunnelKey — so a plugin author learns the collision at
// construction rather than from a key that never fires. Enter and esc are not
// printable and cannot be declared.
//
// Three groups, and the reason each is here:
//
//   - Navigation and search, common to every sdk TUI: j k h l g G / n N ? : q
//     and space. `h` and `:` are reserved AHEAD of use — `h` is navigation real
//     estate (the shared keymap's page-left; never help, which is `?`), and `:`
//     is the command line. Reserving a key is not spending it.
//   - Fleet's dashboard verbs, every one live today: r s u w v a p P A F e J K l.
//     `l` toggles the log pane and `s` opens an ssh session — the two a provider
//     would most plausibly have tried to take.
//   - This objective's own: t and T, the bridge keys.
//
// THIS IS A MIRROR, NOT THE SOURCE. The host tool's keymap is the source of
// truth; this package is stdlib-only by contract and cannot import it. The
// agreement is mechanical instead of clerical: fleet's own
// TestEveryFleetKeyIsReservedAgainstProviders fails if fleet binds a key that is
// missing here. Keep that test passing rather than editing this list by eye — a
// hand-maintained version of it missed six keys fleet already bound.
var ReservedKeys = map[rune]bool{
	// navigation, search, help, command line, quit, select
	'j': true, 'k': true, 'h': true, 'l': true, 'g': true, 'G': true,
	'/': true, 'n': true, 'N': true, '?': true, ':': true, 'q': true, ' ': true,
	// fleet's dashboard verbs
	'r': true, 's': true, 'u': true, 'w': true, 'v': true, 'a': true,
	'p': true, 'P': true, 'A': true, 'F': true, 'e': true, 'J': true, 'K': true,
	// this objective: the bridge keys
	't': true, 'T': true,
}

// Provider is what fleet asks about a host. Probe answers "is your tool here,
// and what is its state"; Children lists a deeper level; Columns names the
// header for a kind. Every call is bounded by ctx.
type Provider interface {
	Name() string
	// Probe costs a bounded number of round trips. ErrAbsent still yields a
	// Node, so an absent capability is a row with a reason, never an omission.
	Probe(ctx context.Context, h Host) (Node, error)
	// Children lists the level at path, with the attrs the parent carried.
	Children(ctx context.Context, h Host, path []string, attrs map[string]string) ([]Node, error)
	// Columns names the header for kind; unknown kind → nil → IDs only.
	Columns(kind string) []string
}

// Host is the ONLY capability a provider has over a machine: run one command
// on it. In-process it wraps the runner seam; over the wire it is the
// host/exec callback. Both paths see the same data — stdin in; stdout,
// stderr and exit code out. A non-zero exit is a result, not an error; err
// means the command could not be run at all, or ctx expired.
type Host interface {
	Alias() string
	Exec(ctx context.Context, stdin string, argv ...string) (ExecResult, error)
}

// ExecResult is what a command produced on the host.
type ExecResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exitCode"`
}

// ErrAbsent means the provider's tool is not on the host. Probe returns it
// together with a Node whose detail says what was tried.
var ErrAbsent = errors.New("provider: capability absent on host")

// ErrNoSuchPath means a path segment names nothing at that level.
var ErrNoSuchPath = errors.New("provider: no such path")

// Row renders the node's cells against columns: a short slice is padded with
// blanks, a long one is cut, and zero cells never panic.
func (n Node) Row(columns []string) []string {
	row := make([]string, len(columns))
	copy(row, n.Cells)
	return row
}

// Validate checks that the node is one path segment with valid actions.
func (n Node) Validate() error {
	if n.ID == "" {
		return errors.New("provider: node has an empty id")
	}
	if strings.Contains(n.ID, "/") {
		return fmt.Errorf("provider: node id %q is not one path segment", n.ID)
	}
	for _, a := range n.Actions {
		if err := a.Validate(); err != nil {
			return fmt.Errorf("provider: node %q: %w", n.ID, err)
		}
	}
	return nil
}

// Validate enforces the action contract: one printable, non-reserved key; a
// label; exactly one of Handoff, Stream or Tunnel; and a valid payload.
func (a Action) Validate() error {
	r, size := utf8.DecodeRuneInString(a.Key)
	if a.Key == "" || r == utf8.RuneError || size != len(a.Key) || !unicode.IsPrint(r) {
		return fmt.Errorf("provider: action key %q is not exactly one printable rune", a.Key)
	}
	if a.Tunnel != nil && a.Key != TunnelKey {
		return fmt.Errorf("provider: tunnel action key %q must be %q, fleet's tunnel key", a.Key, TunnelKey)
	}
	if ReservedKeys[r] && (a.Tunnel == nil || a.Key != TunnelKey) {
		return fmt.Errorf("provider: action key %q is reserved by fleet", a.Key)
	}
	if a.Label == "" {
		return fmt.Errorf("provider: action %q has no label", a.Key)
	}
	set := 0
	if a.Handoff != nil {
		set++
	}
	if a.Stream != nil {
		set++
	}
	if a.Tunnel != nil {
		set++
	}
	if set != 1 {
		return fmt.Errorf("provider: action %q must carry exactly one of handoff, stream or tunnel (has %d)", a.Key, set)
	}
	switch {
	case a.Handoff != nil:
		return a.Handoff.validate()
	case a.Stream != nil:
		return a.Stream.validate()
	default:
		return a.Tunnel.validate()
	}
}

func (h Handoff) validate() error {
	switch h.Kind {
	case HandoffRemote:
		if strings.TrimSpace(h.Command) == "" {
			return errors.New("provider: remote handoff has no command")
		}
	case HandoffLocal:
		if len(h.Argv) == 0 || h.Argv[0] == "" {
			return errors.New("provider: local handoff has no argv")
		}
	default:
		return fmt.Errorf("provider: unknown handoff kind %q", h.Kind)
	}
	return nil
}

func (s Stream) validate() error {
	if strings.TrimSpace(s.Command) == "" {
		return errors.New("provider: stream has no command")
	}
	return nil
}

func (t Tunnel) validate() error {
	if t.RemotePort < 1 || t.RemotePort > 65535 {
		return fmt.Errorf("provider: tunnel remote port %d is out of range 1–65535", t.RemotePort)
	}
	if t.LocalPort < 0 || t.LocalPort > 65535 {
		return fmt.Errorf("provider: tunnel local port %d is out of range 0–65535", t.LocalPort)
	}
	return nil
}
