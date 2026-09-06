package version

import (
	"strings"
	"testing"
)

func TestString(t *testing.T) {
	got := String()
	for _, want := range []string{
		"gcfg v" + Version,
		"Commit:", Commit,
		"Dirty:", Dirty,
		"Build Date:", BuildDate,
		"Description: gcfg —",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("String() missing %q, got:\n%s", want, got)
		}
	}
}

func TestDefaults(t *testing.T) {
	if Version != "dev" || Commit != "none" || BuildDate != "unknown" || Dirty != "false" {
		t.Fatalf("unexpected defaults: %q %q %q %q", Version, Commit, BuildDate, Dirty)
	}
}
