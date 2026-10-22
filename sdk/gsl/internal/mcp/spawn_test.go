package mcp

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
	for _, c := range []struct {
		name   string
		self   string
		getenv func(string) string
		inTest bool
		want   bool
	}{
		{"real binary", "/usr/local/bin/gsl", none, false, true},
		{"go test binary", "/tmp/go-build1/b2/cmd.test", none, false, false},
		{"already a refresher", "/usr/local/bin/gsl", marked, false, false},
		{"renamed test binary", "/tmp/gsltest", none, true, false},
	} {
		if got := canSpawn(c.self, c.getenv, c.inTest); got != c.want {
			t.Errorf("%s: canSpawn = %v, want %v", c.name, got, c.want)
		}
	}
}

// Under go test, SpawnRefresh must refuse rather than re-exec the suite.
func TestSpawnRefresh_RefusesUnderGoTest(t *testing.T) {
	if err := SpawnRefresh(t.TempDir()); !errors.Is(err, ErrNoSpawn) {
		t.Fatalf("SpawnRefresh in a test binary = %v, want ErrNoSpawn", err)
	}
}
