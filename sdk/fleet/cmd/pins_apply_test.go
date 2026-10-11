package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/featflag"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/pins"
)

const applyFixture = `
uv       pin     0.12.16  -  opt/scripts/system/install_uv.sh        uv --version
kubectl  pin     1.37.0   -  opt/scripts/system/install_k8s_tools.sh kubectl version --client
helm     pin     4.2.4    -  opt/scripts/system/install_k8s_tools.sh helm version --short
kind     pin     0.33.0   -  opt/scripts/system/install_k8s_tools.sh kind version
yq       pin     4.44.6   -  opt/scripts/system/install_yq.sh        yq --version
sops     pin     3.13.1   -  opt/scripts/system/install_sops.sh      sops --version
keyd     pin     2.6.0    -  -                                        keyd --version
glow     pin     3.0.0    -  opt/scripts/system/install_glow.sh      glow --version
fonts    pin     3.4.0    -  -                                        -
`

// probeOut keys by exact probe command.
var applyProbes = map[string]string{
	"uv --version":             "uv 0.12.10",                        // drift
	"kubectl version --client": "Client Version: v1.36.0",           // drift
	"helm version --short":     "v4.2.0",                            // drift
	"kind version":             "kind v0.33.0 go1.26.7 linux/arm64", // ok
	"yq --version":             "yq version v4.44.6",                // ok
	// sops: absent -> missing
	"keyd --version": "keyd v2.5.0",        // drift, report-only
	"glow --version": "glow version 3.0.0", // ok
}

func applyExec(_ context.Context, cmd string) (string, error) {
	if out, ok := applyProbes[cmd]; ok {
		return out, nil
	}
	return "sh: 1: " + cmd + ": not found", errors.New("exit status 127")
}

type installerLog struct {
	calls []string
	fail  map[string]bool
}

func (l *installerLog) run(_ context.Context, repo, installer string, out io.Writer) error {
	l.calls = append(l.calls, installer)
	_, _ = io.WriteString(out, "ran "+installer+"\n")
	if l.fail[installer] {
		return errors.New("exit status 1")
	}
	return nil
}

func applyDeps(l *installerLog) *pinsDeps {
	return &pinsDeps{
		resolver:     func(string) pins.Resolver { return fixtureResolver() },
		credential:   func(context.Context) string { return "" },
		exec:         applyExec,
		trackMode:    func(string) string { return "manifest" },
		runInstaller: l.run,
	}
}

func writeApplyRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "opt", "profiles")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "versions.tsv"), []byte(applyFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestPinsStatusInstalledJSONShape(t *testing.T) {
	root := writeApplyRepo(t)
	withPinsGlobals(t, root, true)
	out, err := runPinsCmd(t, applyDeps(&installerLog{}), "status", "--installed")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Rows    []map[string]string `json:"rows"`
		Summary map[string]int      `json:"summary"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("bad json: %v\n%s", err, out)
	}
	if len(got.Rows) != 9 {
		t.Fatalf("rows = %d", len(got.Rows))
	}
	r0 := got.Rows[0]
	if r0["tool"] != "uv" || r0["track"] != "pin" || r0["want"] != "0.12.16" || r0["installed"] != "0.12.10" || r0["state"] != "drift" {
		t.Errorf("row0 = %v", r0)
	}
	// ok: kind, yq, glow; drift: uv, kubectl, helm, keyd; missing: sops; unknown: fonts
	want := map[string]int{"ok": 3, "drift": 4, "missing": 1, "unknown": 1}
	for k, v := range want {
		if got.Summary[k] != v {
			t.Errorf("summary[%s] = %d, want %d (%v)", k, got.Summary[k], v, got.Summary)
		}
	}
	for _, k := range []string{"tool", "track", "want", "installed", "state"} {
		if _, ok := r0[k]; !ok {
			t.Errorf("row lacks key %q", k)
		}
	}
}

func TestPinsStatusInstalledTable(t *testing.T) {
	root := writeApplyRepo(t)
	withPinsGlobals(t, root, false)
	out, err := runPinsCmd(t, applyDeps(&installerLog{}), "status", "--installed")
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"TOOL", "WANT", "INSTALLED", "STATE", "drift", "missing", "unknown", "0.12.10"} {
		if !strings.Contains(out, w) {
			t.Errorf("table missing %q:\n%s", w, out)
		}
	}
}

func TestPinsApplyRunsEachInstallerOnceForDrift(t *testing.T) {
	root := writeApplyRepo(t)
	withPinsGlobals(t, root, false)
	l := &installerLog{}
	out, err := runPinsCmd(t, applyDeps(l), "apply")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
	// drift rows: uv, kubectl, helm (shared installer), keyd (report-only). sops is
	// missing and not named, so it is NOT installed.
	want := []string{"opt/scripts/system/install_uv.sh", "opt/scripts/system/install_k8s_tools.sh"}
	if strings.Join(l.calls, ",") != strings.Join(want, ",") {
		t.Errorf("installers run = %v, want %v", l.calls, want)
	}
	if !strings.Contains(out, "keyd") || !strings.Contains(out, "install.sh") {
		t.Errorf("report-only row not listed:\n%s", out)
	}
	if strings.Contains(out, "sops") {
		t.Errorf("missing-but-unnamed tool must not be touched:\n%s", out)
	}
	if !strings.Contains(out, "ran opt/scripts/system/install_uv.sh") {
		t.Errorf("installer output not streamed:\n%s", out)
	}
}

func TestPinsApplyNamedMissingAndOk(t *testing.T) {
	root := writeApplyRepo(t)
	withPinsGlobals(t, root, false)
	l := &installerLog{}
	out, err := runPinsCmd(t, applyDeps(l), "apply", "sops", "yq", "kind")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
	if strings.Join(l.calls, ",") != "opt/scripts/system/install_sops.sh" {
		t.Errorf("installers run = %v", l.calls)
	}
	if !strings.Contains(out, "yq") || !strings.Contains(out, "ok") {
		t.Errorf("ok rows should be reported as nothing to do:\n%s", out)
	}
}

func TestPinsApplyDryRunRunsNothing(t *testing.T) {
	root := writeApplyRepo(t)
	withPinsGlobals(t, root, false)
	l := &installerLog{}
	out, err := runPinsCmd(t, applyDeps(l), "apply", "--dry-run")
	if err != nil || len(l.calls) != 0 {
		t.Fatalf("err=%v calls=%v", err, l.calls)
	}
	if !strings.Contains(out, "would run") || !strings.Contains(out, "install_k8s_tools.sh") || !strings.Contains(out, "kubectl, helm") {
		t.Errorf("dry-run plan:\n%s", out)
	}
}

func TestPinsApplyFailureIsNonZeroButOthersStillRun(t *testing.T) {
	root := writeApplyRepo(t)
	withPinsGlobals(t, root, false)
	l := &installerLog{fail: map[string]bool{"opt/scripts/system/install_uv.sh": true}}
	out, err := runPinsCmd(t, applyDeps(l), "apply")
	if err == nil || !strings.Contains(err.Error(), "install_uv.sh") {
		t.Fatalf("want error naming the failed installer, got %v", err)
	}
	if len(l.calls) != 2 {
		t.Errorf("later installers must still run, calls = %v", l.calls)
	}
	_ = out
}

func TestPinsApplyNothingToDoAndErrors(t *testing.T) {
	root := writeApplyRepo(t)
	withPinsGlobals(t, root, false)
	l := &installerLog{}
	d := applyDeps(l)
	d.exec = func(_ context.Context, cmd string) (string, error) { return "tool 99.0.0", nil }
	d.exec = func(_ context.Context, cmd string) (string, error) {
		switch cmd {
		case "uv --version":
			return "0.12.16", nil
		case "kubectl version --client":
			return "1.37.0", nil
		case "helm version --short":
			return "4.2.4", nil
		case "kind version":
			return "0.33.0", nil
		case "yq --version":
			return "4.44.6", nil
		case "sops --version":
			return "3.13.1", nil
		case "keyd --version":
			return "2.6.0", nil
		case "glow --version":
			return "3.0.0", nil
		}
		return "", errors.New("nope")
	}
	out, err := runPinsCmd(t, d, "apply")
	if err != nil || len(l.calls) != 0 || !strings.Contains(out, "nothing to apply") {
		t.Errorf("err=%v calls=%v out=%s", err, l.calls, out)
	}
	if _, err := runPinsCmd(t, applyDeps(l), "apply", "ghost"); err == nil {
		t.Error("unknown tool must be an error")
	}
}

func TestPinsApplyRefusesInstallerOutsideRepo(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "opt", "profiles")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "versions.tsv"), []byte("evil pin 1.0.0 - ../../outside.sh evil --version\n"), 0o644)
	withPinsGlobals(t, root, false)
	l := &installerLog{}
	d := applyDeps(l)
	d.exec = func(context.Context, string) (string, error) { return "evil 0.1.0", nil }
	if _, err := runPinsCmd(t, d, "apply"); err == nil || len(l.calls) != 0 {
		t.Errorf("err=%v calls=%v", err, l.calls)
	}
}

func TestPinsTrackModeFromFlag(t *testing.T) {
	cases := []struct {
		name string
		src  featflag.Source
		want string
	}{
		{"latest", featflag.Static{Strs: map[string][]string{"pins.track.mode": {"latest"}}}, "latest"},
		{"pin", featflag.Static{Strs: map[string][]string{"pins.track.mode": {"pin"}}}, "pin"},
		{"manifest", featflag.Static{Strs: map[string][]string{"pins.track.mode": {"manifest"}}}, "manifest"},
		{"error fails open", featflag.Static{Err: errors.New("gff down")}, "manifest"},
		{"unknown key fails open", featflag.Static{}, "manifest"},
		{"junk fails open", featflag.Static{Strs: map[string][]string{"pins.track.mode": {"weird"}}}, "manifest"},
		{"empty fails open", featflag.Static{Strs: map[string][]string{"pins.track.mode": {}}}, "manifest"},
		{"nil source", nil, "manifest"},
	}
	for _, c := range cases {
		if got := pinsTrackModeFrom(c.src); got != c.want {
			t.Errorf("%s: mode = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestRunInstallerScriptRunsBashInRepo(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "inst.sh"), []byte("echo installing from $(basename \"$PWD\" >/dev/null; echo ok)\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runInstallerScript(context.Background(), root, "inst.sh", &out); err != nil {
		t.Fatalf("err=%v out=%s", err, out.String())
	}
	if !strings.Contains(out.String(), "installing") {
		t.Errorf("output = %q", out.String())
	}
	if err := os.WriteFile(filepath.Join(root, "bad.sh"), []byte("exit 4\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runInstallerScript(context.Background(), root, "bad.sh", &out); err == nil {
		t.Error("non-zero exit must be an error")
	}
}
