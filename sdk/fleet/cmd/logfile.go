package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Persistent install logs.
//
// The TUI's log pane is an in-memory ring that dies with the process, which is
// no help the morning after a host misbehaved. Every update also streams to a
// file so an install can be read back, diffed against a later run, or attached
// to a bug report.
//
// One file per host per run: a wave updating four hosts concurrently would
// interleave into an unreadable single file, and a per-run file is the unit an
// operator actually wants to look at.

const (
	logKeepRuns = 200 // files retained per host before the oldest are pruned
	logDirPerm  = 0o700
	logFilePerm = 0o600 // an install log can echo hostnames and paths
)

// logDir is $XDG_STATE_HOME/fleet/logs, falling back to ~/.local/state, which
// is where fleet's own install-stamp convention already puts machine state.
func logDir() string {
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "fleet", "logs")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "state", "fleet", "logs")
}

// logFileName is sortable-by-name on purpose: pruning and "what ran last" are
// both plain lexical operations on the directory listing.
func logFileName(alias string, at time.Time) string {
	return fmt.Sprintf("%s__%s.log", at.UTC().Format("20060102T150405Z"), safeAlias(alias))
}

// safeAlias keeps an ssh alias from escaping the log directory.
func safeAlias(a string) string {
	var b strings.Builder
	for _, r := range a {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	// "." and ".." survive the charset filter but are directory entries, not
	// names — reject them outright rather than trusting the caller to join
	// them somewhere harmless.
	if out == "" || out == "." || out == ".." {
		return "host"
	}
	return out
}

// openRunLog creates this run's file and writes a header naming what is about
// to happen — without it a bare log cannot be tied to a host, a ref, or a time.
// A failure to open is never fatal: losing the log must not cost the update.
func openRunLog(dir, alias, ref string, at time.Time, reset bool) *os.File {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, logDirPerm); err != nil {
		return nil
	}
	path := filepath.Join(dir, logFileName(alias, at))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, logFilePerm)
	if err != nil {
		return nil
	}
	mode := "fast-forward"
	if reset {
		mode = "FORCE RESET"
	}
	fmt.Fprintf(f, "# fleet update — host=%s ref=%s mode=%s started=%s\n",
		alias, ref, mode, at.UTC().Format(time.RFC3339))
	return f
}

// pruneRunLogs keeps the newest logKeepRuns files for one host. Unbounded logs
// on a workstation that updates a fleet daily would grow without limit.
func pruneRunLogs(dir, alias string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	suffix := "__" + safeAlias(alias) + ".log"
	var mine []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), suffix) {
			mine = append(mine, e.Name())
		}
	}
	if len(mine) <= keep {
		return
	}
	sort.Strings(mine) // timestamp-prefixed, so lexical == chronological
	for _, name := range mine[:len(mine)-keep] {
		_ = os.Remove(filepath.Join(dir, name))
	}
}
