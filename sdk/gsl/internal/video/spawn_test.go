package video

import (
	"errors"
	"testing"
)

func TestCanSpawn(t *testing.T) {
	none := func(string) string { return "" }
	marked := func(k string) string {
		if k == RefreshEnv {
			return "1"
		}
		return ""
	}
	cases := []struct {
		name   string
		self   string
		getenv func(string) string
		inTest bool
		want   bool
	}{
		{"real binary", "/usr/local/bin/gsl", none, false, true},
		{"real binary windows", `C:\tools\gsl.exe`, none, false, true},
		{"go test binary", "/tmp/go-build123/b237/cmd.test", none, false, false},
		{"go test binary windows", `C:\Temp\go-build123\b237\cmd.test.exe`, none, false, false},
		{"already a refresher", "/usr/local/bin/gsl", marked, false, false},
		// go test -c -o gsltest: a test binary whose name gives nothing away.
		{"renamed test binary", "/tmp/gsltest", none, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := canSpawn(c.self, c.getenv, c.inTest); got != c.want {
				t.Fatalf("canSpawn(%q) = %v, want %v", c.self, got, c.want)
			}
		})
	}
}

// TestSpawnRefresher_RefusesUnderGoTest is the regression test for the
// 2026-10-17 fork bomb: inside `go test` the executable is <pkg>.test, and
// SpawnRefresher must refuse rather than re-exec it.
func TestSpawnRefresher_RefusesUnderGoTest(t *testing.T) {
	t.Setenv(RefreshEnv, "")
	if err := SpawnRefresher(); !errors.Is(err, ErrNoSpawn) {
		t.Fatalf("SpawnRefresher() under go test = %v, want ErrNoSpawn", err)
	}
}
