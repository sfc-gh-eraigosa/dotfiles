// Package version is the single source of truth for gcfg build metadata.
//
// The four vars are injected at link time by sdk/gcfg/build.sh via
// `-X github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/version.<Var>=<value>`.
// No other package declares build vars. In any binary built without those
// ldflags — `go test`, `go run`, a plain `go install` — the defaults below
// render (dev / none / unknown / false).
package version

import "fmt"

// Build-time metadata, overridden by the linker (see build.sh).
var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
	Dirty     = "false"
)

// String renders the gss-style version block.
func String() string {
	return fmt.Sprintf("gcfg v%s\n  Commit:      %s\n  Dirty:       %s\n  Build Date:  %s\n  Description: gcfg — GitHub settings as code: .github/gcfg.yaml exported, verified, applied",
		Version, Commit, Dirty, BuildDate)
}
