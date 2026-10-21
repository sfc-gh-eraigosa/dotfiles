//go:build windows

package mcp

import "os/exec"

// detach is a no-op on Windows; SpawnRefresh refuses there anyway.
func detach(*exec.Cmd) {}
