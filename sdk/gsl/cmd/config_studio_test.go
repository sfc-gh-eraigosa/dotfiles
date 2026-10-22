package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/term"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/tui"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/video"
)

// E23: bare `gsl config` opens the studio (only on a terminal); subcommands
// keep their CLI behaviour and never open it.
func TestConfigCmd_Routing(t *testing.T) {
	oldTTY, oldRun := studioTTY, runStudio
	t.Cleanup(func() { studioTTY, runStudio = oldTTY, oldRun; rootCmd.SetArgs(nil) })
	opened := 0
	var got tui.Model
	runStudio = func(m tui.Model) error { opened++; got = m; return nil }

	withTempConfig(t, config.Default(), func() {
		studioTTY = func() bool { return false }
		rootCmd.SetArgs([]string{"config"})
		if err := rootCmd.Execute(); !errors.Is(err, errNoTerminal) || opened != 0 {
			t.Fatalf("no terminal: err=%v opened=%d", err, opened)
		}
		studioTTY = func() bool { return true }
		rootCmd.SetArgs([]string{"config"})
		if err := rootCmd.Execute(); err != nil || opened != 1 {
			t.Fatalf("bare config: err=%v opened=%d", err, opened)
		}
		if !strings.Contains(got.View(), "gsl config") {
			t.Errorf("studio view: %q", got.View())
		}
		for _, args := range [][]string{{"config", "get", "style"}, {"config", "set", "links", "plain"}, {"config", "validate"}} {
			rootCmd.SetArgs(args)
			_ = captureStdout(t, func() {
				if err := rootCmd.Execute(); err != nil {
					t.Errorf("%v: %v", args, err)
				}
			})
		}
		if opened != 1 {
			t.Errorf("a subcommand opened the studio (%d)", opened)
		}
		rootCmd.SetArgs([]string{"config", "bogus"})
		if err := rootCmd.Execute(); err == nil {
			t.Error("an unknown subcommand must fail, not open the studio")
		}
	})
}

// The studio's preview draws the video segment from the GPU cache, so its
// row toggles something you can see (it used to be missing from the preview).
func TestStudioPreviewShowsVideo(t *testing.T) {
	dir, err := video.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := (video.Cache{Dir: dir}).Store(video.Stats{Util: 37, UsedBytes: 3 << 30, TotalBytes: 24 << 30, Source: "nvidia"}, nil); err != nil {
		t.Fatal(err)
	}
	// The state dir is shared by the package (TestMain): leave no reading
	// behind for the segment-count tests.
	t.Cleanup(func() { _ = os.Remove(filepath.Join(dir, "video.json")) })
	cfg := config.Default()
	env, err := studioEnv(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if line := term.StripANSI(env.Line(cfg, 200)); !strings.Contains(line, "37%") {
		t.Fatalf("video on, preview lacks the GPU reading: %q", line)
	}
	off := cfg
	off.Segments = append([]config.Segment(nil), cfg.Segments...)
	for i := range off.Segments {
		if off.Segments[i].Type == "video" {
			off.Segments[i].Enabled = false
		}
	}
	if line := term.StripANSI(env.Line(off, 200)); strings.Contains(line, "37%") {
		t.Fatalf("video off, preview still shows it: %q", line)
	}
}
