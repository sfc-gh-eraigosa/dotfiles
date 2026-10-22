//go:build windows

package mcp

import "os"

// tryLock always succeeds on Windows: there is no flock, and nothing races —
// the render path never starts a refresher there (SpawnRefresh refuses), so
// only the manual `gsl mcp refresh` runs.
func tryLock(string) (*os.File, bool) { return nil, true }
