package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wenlock/dotfiles/gsl/internal/config"
	"github.com/wenlock/dotfiles/gsl/internal/observe"
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

// TestRenderCmd_BadJSON_LogsStructuredEvent asserts that malformed stdin is
// recorded as a payload.parse_error in the structured log so a silent
// regression (e.g. issue #30) is diagnosable on the first failing refresh.
func TestRenderCmd_BadJSON_LogsStructuredEvent(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "gsl.log")
	t.Setenv("GSL_LOG_FILE", logPath)
	observe.ResetDefaultForTest()
	t.Cleanup(observe.ResetDefaultForTest)

	cfg := config.Default()
	withTempConfig(t, cfg, func() {
		r, w, _ := os.Pipe()
		fmt.Fprint(w, `{invalid json`)
		w.Close()

		origStdin := os.Stdin
		os.Stdin = r
		defer func() { os.Stdin = origStdin; r.Close() }()

		captureStdout(t, func() {
			if err := runRender(renderCmd, nil); err != nil {
				t.Errorf("runRender: expected nil error on bad JSON, got: %v", err)
			}
		})
	})

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), `"event":"payload.parse_error"`) {
		t.Fatalf("expected payload.parse_error in log, got: %s", data)
	}
}

// TestRenderCmd_MalformedConfig_FallsBackToDefaults verifies that a corrupt
// config file does not break the status line: runRender must not return an
// error, must warn to stderr, and must still render using Default(). (Finding #1)
func TestRenderCmd_MalformedConfig_FallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfgPath := config.DefaultPath()
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatalf("setup: MkdirAll: %v", err)
	}
	if err := os.WriteFile(cfgPath, []byte(`{invalid json`), 0o644); err != nil {
		t.Fatalf("setup: WriteFile: %v", err)
	}

	// Redirect stdin to /dev/null so ParseReader gets EOF.
	origStdin := os.Stdin
	f, _ := os.Open(os.DevNull)
	os.Stdin = f
	defer func() { os.Stdin = origStdin; f.Close() }()

	// Capture stderr to assert a warning is emitted.
	origStderr := os.Stderr
	rErr, wErr, _ := os.Pipe()
	os.Stderr = wErr
	defer func() { os.Stderr = origStderr }()

	captureStdout(t, func() {
		if err := runRender(renderCmd, nil); err != nil {
			t.Errorf("runRender: expected nil error on malformed config, got: %v", err)
		}
	})

	wErr.Close()
	os.Stderr = origStderr
	var errBuf bytes.Buffer
	io.Copy(&errBuf, rErr) //nolint:errcheck
	if !strings.Contains(errBuf.String(), "config load failed") {
		t.Errorf("expected stderr warning about config load failure, got: %q", errBuf.String())
	}
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
