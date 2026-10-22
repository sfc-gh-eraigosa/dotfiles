//go:build windows

package exec

import "os/exec"

// Detach is a no-op on Windows: Start already returns without waiting, and
// the refreshers refuse to spawn there anyway.
func Detach(*exec.Cmd) {}
