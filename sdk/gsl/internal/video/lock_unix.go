//go:build !windows

package video

import (
	"os"
	"syscall"
)

// tryLockFile opens path and takes a non-blocking exclusive flock on it.
func tryLockFile(path string) (*os.File, bool) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false
	}
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		_ = f.Close()
		return nil, false
	}
	return f, true
}
