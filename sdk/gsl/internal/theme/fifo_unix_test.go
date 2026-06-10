//go:build !windows

package theme_test

import "golang.org/x/sys/unix"

func createFIFO(path string) error {
	return unix.Mkfifo(path, 0o600)
}
