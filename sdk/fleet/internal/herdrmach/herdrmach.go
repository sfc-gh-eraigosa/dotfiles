// Package herdrmach reads and writes herdr's saved machines: the remote herdr
// servers the local herdr client lists in its sidebar and switches to with
// prefix+w (docs/mbo/specs/fleet-herdr-machines.md §3).
//
// It is pure but for List, which goes through a one-method Runner so tests
// never need a herdr binary. Every argv it builds starts with the literal
// "herdr"; a caller that resolved the local binary to a path swaps argv[0].
package herdrmach

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Machine is one row of `herdr machine list --json`.
type Machine struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Target   string `json:"target"`
	Session  string `json:"session"`
	Enabled  bool   `json:"enabled"`
	Selected bool   `json:"selected"`
}

// Runner runs a LOCAL command and returns its stdout.
type Runner interface {
	Output(ctx context.Context, argv []string) ([]byte, error)
}

// listArgv is what List runs.
var listArgv = []string{"herdr", "machine", "list", "--json"}

// Parse decodes `herdr machine list --json`. Unknown fields are ignored so a
// newer herdr still parses. Anything whose top level is not an array is an
// error: an error message or `null` must never read as "nothing is saved",
// because the caller would then try to add every host again.
func Parse(b []byte) ([]Machine, error) {
	if t := bytes.TrimSpace(b); len(t) == 0 || t[0] != '[' {
		return nil, errors.New("herdr machine list: output is not a JSON array")
	}
	ms := []Machine{}
	if err := json.Unmarshal(b, &ms); err != nil {
		return nil, fmt.Errorf("herdr machine list: %w", err)
	}
	return ms, nil
}

// List asks the local herdr for its saved machines.
func List(ctx context.Context, r Runner) ([]Machine, error) {
	out, err := r.Output(ctx, append([]string(nil), listArgv...))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.Join(listArgv, " "), err)
	}
	return Parse(out)
}

// Saved reports whether a machine targets the ssh alias. It matches on
// Target, not Label: the operator may rename a machine in herdr, and the
// target is the name fleet knows the host by.
func Saved(ms []Machine, alias string) (Machine, bool) {
	for _, m := range ms {
		if m.Target == alias {
			return m, true
		}
	}
	return Machine{}, false
}

// AddArgv saves alias as a herdr machine labelled by the alias. The alias is
// one argv element; nothing here goes through a shell.
func AddArgv(alias string) []string {
	return []string{"herdr", "machine", "add", alias, "--label", alias}
}
