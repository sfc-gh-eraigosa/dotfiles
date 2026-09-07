//go:build !linux

package runner

import "os/exec"

// setDeathSignal is a no-op where the kernel offers no parent-death signal
// (macOS); a SIGKILLed fleet can orphan a bridge there — a named residual.
// `fleet bridge` prints the child's pid so the operator can find it.
func setDeathSignal(*exec.Cmd) {}
