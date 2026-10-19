// Package version is the single source of truth for gdash build metadata,
// stamped at link time by sdk/gdash/build.sh (-X …/internal/version.<Var>=…).
// A binary built without those flags (go test, go run) reports dev/none.
package version

// Build-time metadata. Empty unless set by the linker.
var (
	Version   string
	Commit    string
	BuildDate string
	Dirty     string
)

// Info is the metadata with display fallbacks applied.
type Info struct {
	Version   string
	Commit    string
	BuildDate string
	Dirty     string
}

// Get returns the build metadata with "dev"/"none"/"unknown"/"false" for
// anything the linker left empty.
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
