//go:build windows

package video

import "os/exec"

// detach is a no-op on Windows: Start already returns without waiting and the
// refresher is released right after.
func detach(*exec.Cmd) {}
