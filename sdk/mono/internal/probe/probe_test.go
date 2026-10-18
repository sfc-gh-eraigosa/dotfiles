package probe

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/config"
)

type fakeEnv struct {
	path    map[string]bool
	ok      map[string]bool // command line -> exit 0
	drivers []string
	ran     []string
}

func (f *fakeEnv) LookPath(b string) (string, error) {
	if f.path[b] {
		return "/usr/bin/" + b, nil
	}
	return "", errors.New("not found")
}

func (f *fakeEnv) Run(_ context.Context, c string) bool {
	f.ran = append(f.ran, c)
	return f.ok[c]
}

func (f *fakeEnv) DRMDrivers() []string { return f.drivers }

func names(p ...string) map[string]bool {
	m := map[string]bool{}
	for _, s := range p {
		m[s] = true
	}
	return m
}

func TestChooseDefaultLayoutPerHost(t *testing.T) {
	nvsmi := map[string]bool{"nvidia-smi -L >/dev/null 2>&1": true}
	cases := []struct {
		name string
		env  *fakeEnv
		want string // chosen cmd per pane, "-" = dropped
	}{
		{"wsl intel: nvtop installed but blind", &fakeEnv{path: names("htop", "nvtop", "gsl")}, "htop - gsl status"},
		{"dotfiles host: htop-fit preferred", &fakeEnv{path: names("htop-fit", "htop", "gsl")}, "htop-fit - gsl status"},
		{"nvidia host", &fakeEnv{path: names("htop", "nvtop", "gsl", "nvidia-smi"), ok: nvsmi}, "htop nvtop gsl status"},
		{"nvidia host, only nvitop", &fakeEnv{path: names("htop", "nvitop", "gsl", "nvidia-smi"), ok: nvsmi}, "htop nvitop gsl status"},
		{"amd linux", &fakeEnv{path: names("htop", "nvtop", "gsl"), drivers: []string{"amdgpu"}}, "htop nvtop gsl status"},
		{"amd linux, nvitop needs nvidia", &fakeEnv{path: names("htop", "nvitop", "gsl"), drivers: []string{"amdgpu"}}, "htop - gsl status"},
		{"nvidia-smi present but no device", &fakeEnv{path: names("htop", "nvtop", "nvidia-smi")}, "htop - -"},
		{"bare host", &fakeEnv{path: names()}, "- - -"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, c := range Choose(context.Background(), tc.env, config.Default().Panes) {
				if c.Usable() {
					got = append(got, c.Cmd)
				} else {
					got = append(got, "-")
				}
			}
			if strings.Join(got, " ") != tc.want {
				t.Fatalf("got %q, want %q", strings.Join(got, " "), tc.want)
			}
		})
	}
}

func TestChooseReasons(t *testing.T) {
	env := &fakeEnv{path: names("nvtop")}
	c := Choose(context.Background(), env, config.Default().Panes)[1]
	if c.Usable() || !strings.Contains(c.Reason, "nvtop: no GPU it can read") || !strings.Contains(c.Reason, "nvitop not installed") {
		t.Fatalf("reason = %q", c.Reason)
	}
}

func TestProbeOverrides(t *testing.T) {
	env := &fakeEnv{path: names("nvtop", "mytool"), ok: map[string]bool{"test -e /dev/x": true}}
	p := config.Pane{Name: "g", Size: "10%", Cmd: "nvtop", Probe: "none"}
	if c := Choose(context.Background(), env, []config.Pane{p})[0]; c.Cmd != "nvtop" {
		t.Fatalf("probe none: %+v", c)
	}
	p = config.Pane{Name: "g", Size: "10%", Cmd: "mytool", Probe: "test -e /dev/x"}
	if c := Choose(context.Background(), env, []config.Pane{p})[0]; c.Cmd != "mytool" {
		t.Fatalf("custom probe ok: %+v", c)
	}
	p.Probe = "false"
	if c := Choose(context.Background(), env, []config.Pane{p})[0]; c.Usable() || !strings.Contains(c.Reason, "probe `false` failed") {
		t.Fatalf("custom probe fail: %+v", c)
	}
}

func TestBinary(t *testing.T) {
	for in, want := range map[string]string{
		"gsl status":               "gsl",
		"  htop -d 10":             "htop",
		"TERM=xterm-256color htop": "htop",
		"/usr/bin/top":             "/usr/bin/top",
		"":                         "",
	} {
		if got := Binary(in); got != want {
			t.Errorf("Binary(%q) = %q, want %q", in, got, want)
		}
	}
}
