package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/grafana/grafanatest"
)

// fakeLab writes a config pointing at a fake Grafana and returns the server.
func fakeLab(t *testing.T, views map[string]config.View) *grafanatest.Server {
	t.Helper()
	s := grafanatest.New(t)
	p := withConfig(t)
	t.Setenv("GDASH_TEST_CRED", "fixture")
	c := config.Config{
		Profiles:       map[string]config.Profile{"lab": {URL: s.URL, Auth: config.Auth{Env: "GDASH_TEST_CRED"}}},
		DefaultProfile: "lab",
		Views:          views,
	}
	if err := config.Save(p, c); err != nil {
		t.Fatal(err)
	}
	return s
}

func resetQuiet(t *testing.T) {
	t.Cleanup(func() { rootCmd.SilenceErrors = false; doctorQuiet = false; doctorProfile = "" })
}

// E14: every check passes → one ok row each, exit 0.
func TestDoctorAllPass(t *testing.T) {
	resetQuiet(t)
	fakeLab(t, map[string]config.View{"ci": {Dashboard: "ci-lab"}, "conductor": {Dashboard: "Conductor — metrics against goals"}})
	out, err := run(t, "doctor")
	if err != nil {
		t.Fatalf("doctor failed:\n%s", out)
	}
	for _, want := range []string{"ok   reachable", "Grafana 13.1.3", "ok   authenticated", "ok   view ci", "ok   view conductor", "ok   datasource Prometheus"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "fixture") {
		t.Fatal("doctor printed the credential")
	}
}

func TestDoctorMissingDashboard(t *testing.T) {
	resetQuiet(t)
	fakeLab(t, map[string]config.View{"ci": {Dashboard: "no such dashboard"}})
	out, err := run(t, "doctor")
	if err == nil || !strings.Contains(out, "FAIL view ci") || !strings.Contains(out, "gdash list") {
		t.Fatalf("missing dashboard: %v\n%s", err, out)
	}
}

func TestDoctorRefusedCredential(t *testing.T) {
	resetQuiet(t)
	s := fakeLab(t, nil)
	s.Status = 401
	out, err := run(t, "doctor")
	if err == nil || !strings.Contains(out, "FAIL") || !strings.Contains(out, "401") {
		t.Fatalf("401: %v\n%s", err, out)
	}
}

func TestDoctorGrafanaDown(t *testing.T) {
	resetQuiet(t)
	s := fakeLab(t, nil)
	s.Close()
	out, err := run(t, "doctor")
	if err == nil || !strings.Contains(out, "FAIL reachable") {
		t.Fatalf("down: %v\n%s", err, out)
	}
}

// --quiet: reachability and auth only, no output, exit status only.
func TestDoctorQuiet(t *testing.T) {
	resetQuiet(t)
	fakeLab(t, map[string]config.View{"ci": {Dashboard: "no such dashboard"}})
	out, err := run(t, "doctor", "--quiet")
	if err != nil || out != "" {
		t.Fatalf("--quiet ignores dashboards and prints nothing: %v %q", err, out)
	}
	resetQuiet(t)
	s := fakeLab(t, nil)
	s.Status = 401
	out, err = run(t, "doctor", "--quiet")
	if err == nil || out != "" {
		t.Fatalf("--quiet with a refused credential: %v %q", err, out)
	}
}

// E15: a Grafana that hangs fails the probe inside mono's 3 s budget.
func TestDoctorQuietTimeout(t *testing.T) {
	resetQuiet(t)
	s := fakeLab(t, nil)
	s.Delay = 10 * time.Second
	start := time.Now()
	_, err := run(t, "doctor", "--quiet")
	if err == nil {
		t.Fatal("a hanging Grafana passed the probe")
	}
	if d := time.Since(start); d > 2500*time.Millisecond {
		t.Fatalf("--quiet took %v, over 2.5 s", d)
	}
}

func TestDoctorNoProfile(t *testing.T) {
	resetQuiet(t)
	withConfig(t)
	if out, err := run(t, "doctor"); err == nil || !strings.Contains(out+err.Error(), "gdash config add profile") {
		t.Fatalf("no profile: %v %s", err, out)
	}
}

func TestList(t *testing.T) {
	resetQuiet(t)
	fakeLab(t, nil)
	out, err := run(t, "list", "nodes")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "058020e04168bfdea0c52269cb699df2\tKubernetes / Compute Resources / Nodes Overview\t") {
		t.Fatalf("list nodes:\n%s", out)
	}
	if !strings.HasPrefix(lines[1], "7d57716318ee0dddbac5a7f451fb7753\tNode Exporter / Nodes\t") {
		t.Fatalf("sorted by title:\n%s", out)
	}
}
