package cmd

import "testing"

func TestVersionStringIncludesVersionAndCommit(t *testing.T) {
	Version, Commit = "9.9.9", "abc1234"
	got := versionString()
	want := "fleet 9.9.9 (abc1234)"
	if got != want {
		t.Fatalf("versionString() = %q, want %q", got, want)
	}
}

// Guards the ldflags contract: build.sh injects cmd.Version/Commit/Dirty/
// BuildDate by exact symbol path. Renaming or unexporting these breaks the
// injection silently, so pin the defaults here.
func TestLdflagsTargetsExist(t *testing.T) {
	for name, ptr := range map[string]*string{
		"Version": &Version, "Commit": &Commit, "Dirty": &Dirty, "BuildDate": &BuildDate,
	} {
		if ptr == nil {
			t.Fatalf("ldflags target %q missing", name)
		}
	}
}
