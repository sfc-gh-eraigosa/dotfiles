package exec

import "testing"

func TestCanSpawn(t *testing.T) {
	const env = "GSL_X_REFRESH"
	none := func(string) string { return "" }
	marked := func(k string) string {
		if k == env {
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
		{"real binary windows", `C:\tools\gsl.exe`, none, false, true},
		{"go test binary", "/tmp/go-build1/b2/cmd.test", none, false, false},
		{"go test binary windows", `C:\Temp\b2\cmd.test.exe`, none, false, false},
		{"already a refresher", "/usr/local/bin/gsl", marked, false, false},
		{"renamed test binary", "/tmp/gsltest", none, true, false},
	} {
		if got := CanSpawn(c.self, c.getenv, c.inTest, env); got != c.want {
			t.Errorf("%s: CanSpawn = %v, want %v", c.name, got, c.want)
		}
	}
}
