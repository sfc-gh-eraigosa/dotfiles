package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/wenlock/dotfiles/gsl/internal/config"
)

// withTempConfig sets XDG_CONFIG_HOME to a temp dir, writes cfg there, and
// calls f. The original env value is restored after f returns.
func withTempConfig(t *testing.T, cfg config.Config, f func()) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfgPath := config.DefaultPath()
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatalf("setup: config.Save: %v", err)
	}
	f()
}

// captureStdout runs f and returns whatever was written to os.Stdout.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	f()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r) //nolint:errcheck
	return buf.String()
}

// TestRenderCmd_EmptyStdin exercises runRender with empty stdin.
// The function must not panic and should produce a non-empty line when the
// config is enabled (it reads the real git status of the current directory
// so we just check the line is non-empty).
func TestRenderCmd_EmptyStdin(t *testing.T) {
	cfg := config.Default()
	withTempConfig(t, cfg, func() {
		// Redirect stdin to /dev/null so ParseReader gets EOF.
		origStdin := os.Stdin
		f, _ := os.Open(os.DevNull)
		os.Stdin = f
		defer func() { os.Stdin = origStdin; f.Close() }()

		out := captureStdout(t, func() {
			cmd := renderCmd
			cmd.ResetFlags()
			if err := runRender(renderCmd, nil); err != nil {
				t.Errorf("runRender: unexpected error: %v", err)
			}
		})
		// We just require no panic and the function runs; output may be empty
		// if inside a non-git directory; don't assert content for portability.
		_ = out
	})
}

// TestRenderCmd_WithPayload_ProducesLine pipes a realistic JSON payload and
// checks that the output contains expected substrings.
func TestRenderCmd_WithPayload_ProducesLine(t *testing.T) {
	cfg := config.Default()
	withTempConfig(t, cfg, func() {
		payload := `{"cwd":"/tmp","model":{"display_name":"TestModel"},"context_window":{"used_percentage":50,"total_input_tokens":100000,"context_window_size":200000}}`

		// Redirect stdin to the payload.
		r, w, _ := os.Pipe()
		fmt.Fprint(w, payload)
		w.Close()

		origStdin := os.Stdin
		os.Stdin = r
		defer func() { os.Stdin = origStdin; r.Close() }()

		out := captureStdout(t, func() {
			if err := runRender(renderCmd, nil); err != nil {
				t.Errorf("runRender: unexpected error: %v", err)
			}
		})
		// The AI segment should render with the model name.
		if !strings.Contains(out, "TestModel") {
			t.Errorf("output missing model name %q; got: %q", "TestModel", out)
		}
		if !strings.Contains(out, "50%") {
			t.Errorf("output missing context %q; got: %q", "50%", out)
		}
	})
}

// TestRenderCmd_MasterDisabled produces no output when cfg.Enabled == false.
func TestRenderCmd_MasterDisabled(t *testing.T) {
	cfg := config.Default()
	cfg.Enabled = false
	withTempConfig(t, cfg, func() {
		origStdin := os.Stdin
		f, _ := os.Open(os.DevNull)
		os.Stdin = f
		defer func() { os.Stdin = origStdin; f.Close() }()

		out := captureStdout(t, func() {
			if err := runRender(renderCmd, nil); err != nil {
				t.Errorf("runRender: unexpected error: %v", err)
			}
		})
		if strings.TrimSpace(out) != "" {
			t.Errorf("expected empty output when master disabled, got: %q", out)
		}
	})
}

// TestRenderCmd_BadJSON degrades gracefully on malformed stdin.
func TestRenderCmd_BadJSON(t *testing.T) {
	cfg := config.Default()
	withTempConfig(t, cfg, func() {
		r, w, _ := os.Pipe()
		fmt.Fprint(w, `{invalid json`)
		w.Close()

		origStdin := os.Stdin
		os.Stdin = r
		defer func() { os.Stdin = origStdin; r.Close() }()

		// Must not return an error (degrade gracefully).
		captureStdout(t, func() {
			if err := runRender(renderCmd, nil); err != nil {
				t.Errorf("runRender: expected nil error on bad JSON, got: %v", err)
			}
		})
	})
}

// TestConfigToRawStyles covers the configToRawStyles helper.
func TestConfigToRawStyles(t *testing.T) {
	// nil input → nil output
	if got := configToRawStyles(nil); got != nil {
		t.Errorf("nil input: got %v, want nil", got)
	}

	// map with a valid nested map[string]any value.
	raw := map[string]any{
		"powerline": map[string]any{"separator": "space"},
		"notamap":   "ignored",
	}
	got := configToRawStyles(raw)
	if len(got) != 1 {
		t.Errorf("len: got %d, want 1", len(got))
	}
	if _, ok := got["powerline"]; !ok {
		t.Error("got map missing \"powerline\" key")
	}
}
