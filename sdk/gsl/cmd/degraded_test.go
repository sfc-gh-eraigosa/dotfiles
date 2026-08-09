package cmd

// degraded_test.go — E24 / UC-7.
//
// gsl is invoked on EVERY assistant turn. A panic or a stack trace on stdout
// would be pasted straight into the user's status line, and a non-zero exit
// would make the host disable the status line entirely. So the four hostile
// environments below must each yield: exit 0, no panic, no stack trace.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/observe"
)

// withStdin points os.Stdin at a file containing s for the duration of f.
func withStdin(t *testing.T, s string, f func()) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
		t.Fatalf("write stdin fixture: %v", err)
	}
	fh, err := os.Open(path)
	if err != nil {
		t.Fatalf("open stdin fixture: %v", err)
	}
	orig := os.Stdin
	os.Stdin = fh
	defer func() { os.Stdin = orig; _ = fh.Close() }()
	f()
}

// assertNoPanicOutput fails when out looks like a Go panic / stack trace.
func assertNoPanicOutput(t *testing.T, label, out string) {
	t.Helper()
	for _, needle := range []string{"panic:", "goroutine ", "runtime error", ".go:"} {
		if strings.Contains(out, needle) {
			t.Errorf("%s: stdout contains %q — gsl leaked a panic/stack trace into the "+
				"status line (E24/UC-7). stdout:\n%s", label, needle, out)
		}
	}
}

// TestDegradedPaths is E24: each hostile environment, exercised independently.
func TestDegradedPaths(t *testing.T) {
	// A corrupt ~/.claude.json is the MCP-detect input; garbage must not
	// propagate.  "$HOME unset" is its own subtest, so each case configures the
	// environment it needs from scratch.
	cases := []struct {
		name  string
		stdin string
		// setup runs inside the subtest, after the shared temp-HOME wiring.
		setup func(t *testing.T, home string)
	}{
		{
			name:  "corrupt ~/.claude.json",
			stdin: `{"cwd":"/tmp"}`,
			setup: func(t *testing.T, home string) {
				t.Helper()
				corrupt := "{ this is not json ]]] \x00\xff"
				if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(corrupt), 0o600); err != nil {
					t.Fatalf("write corrupt .claude.json: %v", err)
				}
			},
		},
		{
			name:  "not a git repo",
			stdin: `{"cwd":"/tmp"}`,
			setup: func(t *testing.T, _ string) {
				t.Helper()
				// A bare temp dir: no .git anywhere up the tree we control.
				t.Chdir(t.TempDir())
			},
		},
		{
			name:  "malformed stdin payload",
			stdin: `{"model": 12345, "cwd": [1,2,3], "truncated`,
			setup: func(t *testing.T, _ string) { t.Helper() },
		},
		{
			name:  "$HOME unset",
			stdin: `{"cwd":"/tmp"}`,
			setup: func(t *testing.T, _ string) {
				t.Helper()
				t.Setenv("HOME", "")
				t.Setenv("XDG_CONFIG_HOME", "")
				t.Setenv("XDG_CACHE_HOME", "")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			// A segment panic is RECOVERED by Detect, so it never reaches stdout
			// — which is precisely how a test stays green on a panicking path.
			// Watch the log for it instead.
			logPath := filepath.Join(home, "gsl.log")
			t.Setenv("GSL_LOG_FILE", logPath)
			t.Setenv("GSL_LOG_LEVEL", "error")
			observe.ResetDefaultForTest()
			t.Cleanup(observe.ResetDefaultForTest)
			// Keep the render hermetic-ish and off the network: a scratch cache
			// dir and a scratch config dir, and run from a non-repo cwd.
			t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
			t.Chdir(t.TempDir())

			if err := config.Save(config.DefaultPath(), config.Default()); err != nil {
				t.Fatalf("setup: config.Save: %v", err)
			}

			tc.setup(t, home)

			var out string
			var runErr error
			withStdin(t, tc.stdin, func() {
				out = captureStdout(t, func() {
					// A panic here fails the test outright — which is the point.
					runErr = runRender(renderCmd, nil)
				})
			})

			// Exit 0: RunE returning nil is what makes the process exit 0.
			if runErr != nil {
				t.Errorf("runRender returned %v; want nil (gsl must exit 0 on every "+
					"degraded path — E24/UC-7)", runErr)
			}
			assertNoPanicOutput(t, tc.name, out)

			// No segment may have panicked — even though the recover() would have
			// hidden it from stdout.
			if data, err := os.ReadFile(logPath); err == nil {
				if strings.Contains(string(data), `"event":"segment.panic"`) {
					t.Errorf("%s: a segment PANICKED (recovered, so invisible on stdout). log:\n%s",
						tc.name, data)
				}
			}
			if n := strings.Count(strings.TrimRight(out, "\n"), "\n"); n != 0 {
				t.Errorf("%s: stdout has %d extra newlines; the status line must be a "+
					"single line (or empty). stdout:\n%q", tc.name, n, out)
			}
		})
	}
}
