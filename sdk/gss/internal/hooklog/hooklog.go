// Package hooklog appends one line per git hook run to a local log, so a
// commit or push that left no line is visible as a hook that did not run.
//
// The privacy git hooks (ai/githooks) call `gss hooks log` from an EXIT trap
// and, just before handing off to a repo's own hook, with result "chained".
// Logging never changes what a hook allows or refuses: every error here is
// returned to the caller, which drops it, and the hook's exit status is set
// before the trap runs.
package hooklog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Results a hook run can have.
const (
	Pass    = "pass"    // the hook exited 0
	Refused = "refused" // the hook exited non-zero: a block, or a rules load failure
	Chained = "chained" // the guard passed and handed off to the repo's own hook
)

// MaxBytes bounds the log: past it, the newest half of the lines is kept.
const MaxBytes = 2 << 20

// Entry is one log line.
type Entry struct {
	Time   string `json:"time"`
	Host   string `json:"host"`
	Repo   string `json:"repo"`
	Hook   string `json:"hook"`
	Result string `json:"result"`
	Exit   *int   `json:"exit,omitempty"`
	// Skip records PRIVACY_GUARD_SKIP: the judgement was skipped on request.
	Skip bool `json:"skip,omitempty"`
}

// Env is the environment lookup (os.Getenv in production).
type Env func(string) string

// Path is where the log lives: $GSS_HOOK_LOG, else
// $XDG_STATE_HOME/githooks/runs.jsonl, else ~/.local/state/githooks/runs.jsonl.
func Path(env Env) string {
	if p := env("GSS_HOOK_LOG"); p != "" {
		return p
	}
	state := env("XDG_STATE_HOME")
	if state == "" {
		state = filepath.Join(env("HOME"), ".local", "state")
	}
	return filepath.Join(state, "githooks", "runs.jsonl")
}

// Build makes the entry for one run. status is the hook's exit code, or
// "chained". cwd is shown with the home directory as ~.
func Build(now time.Time, host, cwd, hook, status string, env Env) (Entry, error) {
	if hook == "" {
		return Entry{}, fmt.Errorf("hooks log: no hook name")
	}
	e := Entry{
		Time: now.UTC().Format(time.RFC3339),
		Host: shortHost(host),
		Repo: tildeHome(cwd, env("HOME")),
		Hook: hook,
		Skip: env("PRIVACY_GUARD_SKIP") != "",
	}
	if status == Chained {
		e.Result = Chained
		return e, nil
	}
	code, err := strconv.Atoi(status)
	if err != nil {
		return Entry{}, fmt.Errorf("hooks log: status %q is not an exit code or %q", status, Chained)
	}
	e.Exit = &code
	e.Result = Pass
	if code != 0 {
		e.Result = Refused
	}
	return e, nil
}

// Append writes e as one JSON line, creating the directory, and trims the
// file when it has grown past MaxBytes.
func Append(path string, e Entry) error {
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.Write(append(line, '\n'))
	st, serr := f.Stat()
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return werr
	}
	if serr == nil && st.Size() > MaxBytes {
		return trim(path)
	}
	return nil
}

// trim keeps the newest half of the lines.
func trim(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := bytes.SplitAfter(raw, []byte("\n"))
	keep := bytes.Join(lines[len(lines)/2:], nil)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, keep, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func shortHost(h string) string {
	if i := strings.IndexByte(h, '.'); i > 0 {
		return h[:i]
	}
	return h
}

func tildeHome(p, home string) string {
	if home == "" || home == "/" {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + p[len(home):]
	}
	return p
}
