package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/config"
)

type deckEnv struct{}

func (deckEnv) LookPath(b string) (string, error) {
	if b == "htop" || b == "gsl" || b == "gdash" {
		return "/bin/" + b, nil
	}
	return "", errors.New("missing")
}
func (deckEnv) Run(_ context.Context, c string) bool { return c == "gdash doctor --quiet" }
func (deckEnv) DRMDrivers() []string                 { return nil }

func deckConfig() config.Config {
	f := false
	c := config.Default()
	c.Middle = config.MiddleSplit
	c.Panes = []config.Pane{
		{Name: "htop", Size: "60%", Cmd: "htop"},
		{Name: "mid", Size: "30%", Tools: []config.Tool{
			{Name: "gpu", Views: []config.View{{Name: "nvtop", Cmd: "nvtop"}}},
			{Name: "gdash", Probe: "gdash doctor --quiet", Zoom: true, Views: []config.View{{Name: "conductor", Cmd: "gdash conductor"}, {Name: "ci", Cmd: "gdash ci"}}},
		}},
		{Name: "gsl", Size: "5%", Cmd: "gsl status", Every: "1s", Zoomable: &f},
	}
	return c
}

// E17 (doctor): a row per view — what runs and why each was kept or dropped.
func TestDoctorRowPerView(t *testing.T) {
	p := withConfig(t)
	if err := config.Save(p, deckConfig()); err != nil {
		t.Fatal(err)
	}
	prev := doctorEnv
	doctorEnv = deckEnv{}
	defer func() { doctorEnv = prev }()
	out, err := run(t, "doctor")
	if err != nil {
		t.Fatal(err)
	}
	t.Log("\n" + out)
	for _, want := range []string{"mid", "2 of 3 views usable", "gdash/conductor", "gdash conductor", "gdash/ci", "gpu/nvtop", "nvtop not installed", "zoom"} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor output missing %q:\n%s", want, out)
		}
	}
}

func TestDoctorMiddleSingle(t *testing.T) {
	p := withConfig(t)
	c := deckConfig()
	c.Middle = "single"
	if err := config.Save(p, c); err != nil {
		t.Fatal(err)
	}
	prev := doctorEnv
	doctorEnv = deckEnv{}
	defer func() { doctorEnv = prev }()
	out, err := run(t, "doctor")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "middle: single") || !strings.Contains(out, "htop/htop") || strings.Contains(out, "\nhtop ") {
		t.Fatalf("single middle should show one mid deck holding htop:\n%s", out)
	}
}
