//go:build !windows

package mcp

import (
	"os"
	"syscall"
)

// tryLock takes a non-blocking exclusive flock on path. The kernel drops it
// when the holder exits, however it exits — so there is no stale lock to
// break, and no check-then-remove race between two refreshers.
func tryLock(path string) (*os.File, bool) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false
	}
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil { //nolint:gosec // G115: an fd fits an int
		_ = f.Close()
		return nil, false
	}
	return f, true
}
