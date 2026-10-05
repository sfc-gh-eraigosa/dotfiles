package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/observe"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/usage"
)

// runUsageCmd runs `gsl usage` with the given flags and returns stdout.
func runUsageCmd(t *testing.T, host string, asJSON bool) (string, error) {
	t.Helper()
	usageHost, usageJSON = host, asJSON
	t.Cleanup(func() { usageHost, usageJSON = "", false })
	var buf bytes.Buffer
	usageCmd.SetOut(&buf)
	t.Cleanup(func() { usageCmd.SetOut(nil) })
	err := runUsage(usageCmd, nil)
	return buf.String(), err
}

// TestRenderRecordsUsage is the end-to-end promise: a render that carries rate
// limits leaves them where `gsl usage` reads them, for each host separately.
func TestRenderRecordsUsage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("GSL_LOG_FILE", filepath.Join(home, "gsl.log"))
	observe.ResetDefaultForTest()
	t.Cleanup(observe.ResetDefaultForTest)
	t.Chdir(t.TempDir())
	if err := config.Save(config.DefaultPath(), config.Default()); err != nil {
		t.Fatalf("config.Save: %v", err)
	}

	out, err := runUsageCmd(t, "", false)
	if err != nil || !strings.Contains(out, "no usage recorded yet") {
		t.Errorf("before any render: %q, %v", out, err)
	}
	if _, err := runUsageCmd(t, usage.HostClaude, false); !errors.Is(err, usage.ErrNone) {
		t.Errorf("--host claude before any render = %v; want ErrNone (exit 1)", err)
	}

	resets := time.Now().Add(3*24*time.Hour + 4*time.Hour + 30*time.Minute).UTC().Format(time.RFC3339)
	render := func(stdin string) {
		withStdin(t, stdin, func() {
			captureStdout(t, func() {
				if err := runRender(renderCmd, nil); err != nil {
					t.Fatalf("runRender: %v", err)
				}
			})
		})
	}
	render(`{"model":{"id":"claude-opus-5-5"},"rate_limits":{
		"five_hour":{"used_percentage":12,"resets_at":"` + resets + `"},
		"seven_day":{"used_percentage":34.4,"resets_at":"` + resets + `"}}}`)
	render(`{"product":"antigravity","quota":{"gemini-weekly":{"remaining_fraction":0.8}}}`)
	render(`{"cwd":"/tmp"}`) // no rate limits: must not erase what was recorded

	out, err = runUsageCmd(t, usage.HostClaude, true)
	if err != nil {
		t.Fatalf("usage --host claude --json: %v", err)
	}
	var s usage.Snapshot
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if s.SevenDay == nil || s.SevenDay.UsedPercentage != 34.4 || s.SevenDay.ResetsAt != resets || s.Model != "claude-opus-5-5" {
		t.Errorf("claude snapshot = %+v", s)
	}

	out, err = runUsageCmd(t, "", false)
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	for _, want := range []string{"antigravity", "7d 20%", "claude", "7d 34%", "5h 12%", "(in 3d 4h)", "seen 0s ago"} {
		if !strings.Contains(out, want) {
			t.Errorf("usage output lacks %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "antigravity") > strings.Index(out, "claude") {
		t.Errorf("hosts not sorted:\n%s", out)
	}

	out, _ = runUsageCmd(t, "", true)
	var all []usage.Snapshot
	if err := json.Unmarshal([]byte(out), &all); err != nil || len(all) != 2 {
		t.Errorf("usage --json = %v, %v; want an array of 2", all, err)
	}
}

func TestRoughDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		-time.Second:     "0s",
		40 * time.Second: "40s",
		2 * time.Minute:  "2m",
		65 * time.Minute: "1h 5m",
		76 * time.Hour:   "3d 4h",
	} {
		if got := roughDuration(d); got != want {
			t.Errorf("roughDuration(%v) = %q; want %q", d, got, want)
		}
	}
}
