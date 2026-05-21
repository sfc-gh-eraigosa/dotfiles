// Package version is the single source of truth for gss build metadata.
//
// The four exported vars are injected at link time by src/gss/build.sh via
// `-X github.com/wenlock/dotfiles/gss/internal/version.<Var>=<value>`. No
// other package declares build vars: design.md → "Build-time version"
// (resolution #6) requires that `build.sh` target only this package and
// that the `gss version` command read from here rather than from package
// main or package cmd.
//
// In any binary built without those ldflags — `go test`, `go run`, a plain
// `go install` — the vars are the empty string. Callers that render version
// info should go through Get(), which layers display fallbacks on top so an
// unstamped binary still shows "dev"/"none"/… instead of blanks.
package version

// Build-time metadata. Empty unless set by the linker (see build.sh).
var (
	// Version is the release version, sourced from the VERSION file.
	Version string
	// Commit is the short git SHA at build time.
	Commit string
	// BuildDate is the UTC build timestamp (RFC3339).
	BuildDate string
	// Dirty is "true" when the worktree had uncommitted changes at build
	// time, "false" otherwise.
	Dirty string
)

// Info is a snapshot of build metadata with display fallbacks applied.
type Info struct {
	Version   string
	Commit    string
	BuildDate string
	Dirty     string
}

// Get returns the build metadata, substituting a sensible default for any
// field the linker left empty. The defaults preserve the historical
// behaviour of the classic `gss version` command (dev / none / unknown /
// false) so an unstamped binary renders identically to before.
func Get() Info {
	return Info{
		Version:   orDefault(Version, "dev"),
		Commit:    orDefault(Commit, "none"),
		BuildDate: orDefault(BuildDate, "unknown"),
		Dirty:     orDefault(Dirty, "false"),
	}
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
