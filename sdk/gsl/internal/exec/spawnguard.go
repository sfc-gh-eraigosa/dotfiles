package exec

import (
	"path/filepath"
	"strings"
)

// CanSpawn is the recursion guard every detached refresher (the video and MCP
// refreshers) re-execs behind: never from a Go test binary (inTest —
// testing.Testing() — is authoritative; the ".test" name check is a second
// layer, since `go test -c -o <name>` gives nothing away), and never from a
// refresher (refreshEnv already set in getenv). Re-exec'ing a test binary
// runs the whole suite again, detached, with nothing to reap it: the
// 2026-10-17 fork bomb (~2,800 cmd.test processes took the WSL VM down).
func CanSpawn(self string, getenv func(string) string, inTest bool, refreshEnv string) bool {
	if inTest || getenv(refreshEnv) != "" {
		return false
	}
	base := strings.TrimSuffix(strings.ToLower(filepath.Base(self)), ".exe")
	return !strings.HasSuffix(base, ".test")
}
