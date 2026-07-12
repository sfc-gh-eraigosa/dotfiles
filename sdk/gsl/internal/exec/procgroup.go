// Package exec hardens *os/exec.Cmd against the two ways a subprocess defeats
// a deadline. It is the shared helper behind the three seams (internal/git,
// internal/gh, internal/mcp); it never RUNS a command itself — it only
// configures one — so the seam gate in scripts/check-deps.sh still holds.
//
// # The bug this exists to fix (spec F12 / rule E18)
//
// The seams point cmd.Stdout at a *bytes.Buffer. os/exec cannot hand a
// non-*os.File to the child, so it allocates an os.Pipe and copies from it in a
// goroutine. Wait() then blocks until EVERY writer closes the write end — and
// the child's own children INHERIT that write end.
//
// So a command that exits instantly but leaves a grandchild behind (the exact
// shape of `claude mcp list`, which dials every server, and of `gh`, which forks
// git) pins Wait() open for as long as the GRANDCHILD lives. Measured against an
// 800 ms deadline, a 5 s grandchild made git.Run take 5004 ms and a 30 s
// grandchild made it take 30006 ms: the context deadline was decorative.
//
// Cancelling the context did not help either, because os/exec's default Cancel
// kills only the DIRECT child. The orphan lives on, still holding the pipe.
//
// # The fix
//
// Harden applies all three parts, which only work together:
//
//   - Setpgid — the child leads its own process group, so its descendants are
//     addressable as a unit.
//   - Cancel — on ctx cancellation, SIGKILL the whole GROUP (a negative pid),
//     not just the direct child.
//   - WaitDelay — bound the time Wait() will block on I/O pipes that an orphan
//     we could not reach may still hold open. This is the backstop that makes
//     the deadline unconditional.
package exec

import (
	"os/exec"
	"time"
)

// DefaultWaitDelay bounds how long Wait() may block on the child's I/O pipes
// after the process has exited (or after the context was cancelled).
//
// 200 ms is deliberately far below the render budget: gsl runs on every
// assistant turn, so the worst case a user can experience from an orphaned
// grandchild is deadline + 200 ms, not deadline + <however long the orphan
// feels like living>.
const DefaultWaitDelay = 200 * time.Millisecond

// Harden configures cmd for containment with DefaultWaitDelay.
//
// cmd MUST have been created with exec.CommandContext: os/exec rejects a Cancel
// func on a command with no context ("exec: command with Cancel but no Context").
// All three seams do exactly that.
//
// Harden is a no-op on a nil cmd.
func Harden(cmd *exec.Cmd) { HardenWithDelay(cmd, DefaultWaitDelay) }

// HardenWithDelay is Harden with an explicit WaitDelay. A zero or negative delay
// restores the os/exec default (block on the pipes until EOF), which is exactly
// the behaviour E18 exists to prevent — pass a positive delay.
func HardenWithDelay(cmd *exec.Cmd, delay time.Duration) {
	if cmd == nil {
		return
	}
	setPgid(cmd)
	if delay > 0 {
		cmd.WaitDelay = delay
	}
	cmd.Cancel = func() error { return KillGroup(cmd) }
}

// KillGroup SIGKILLs the whole process group led by cmd's process — the child
// AND every grandchild it spawned.
//
// It returns an error wrapping os.ErrProcessDone when there is nothing left to
// kill; os/exec treats that as "the command already finished" and preserves the
// command's real exit status instead of reporting a cancellation error.
func KillGroup(cmd *exec.Cmd) error { return killGroup(cmd) }
