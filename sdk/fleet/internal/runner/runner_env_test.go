package runner

import (
	"context"
	"os/exec"
	"testing"
)

// TestFilteredEnvDropsLocaleKeepsEverythingElse pins the rule Change D exists
// for: fleet's ssh sends whatever LANG/LC_* the OPERATOR's shell has (this
// machine's /etc/ssh/ssh_config sets `SendEnv LANG LC_*`), and a remote host
// that never installed that locale logs
// `/bin/bash: warning: setlocale: LC_ALL: cannot change locale (en_US.UTF-8)`
// to stderr on every command — noise the TUI then badges as a warning even
// though nothing on the host is actually broken. Stripping the locale
// variables from the LOCAL process's env means ssh's SendEnv has nothing to
// forward, so each remote falls back to its own default locale.
//
// Everything else must survive: SSH_AUTH_SOCK (agent forwarding), PATH and
// HOME are all still required for ssh itself to work, and a variable that
// merely starts with "LC" but isn't an "LC_" locale var (LCX_NOT_LOCALE)
// must not be caught by an overly broad prefix check.
func TestFilteredEnvDropsLocaleKeepsEverythingElse(t *testing.T) {
	in := []string{
		"LANG=en_US.UTF-8",
		"LANGUAGE=en_US:en",
		"LC_ALL=en_US.UTF-8",
		"LC_CTYPE=en_US.UTF-8",
		"LC_TIME=en_US.UTF-8",
		"SSH_AUTH_SOCK=/tmp/agent.sock",
		"PATH=/usr/bin:/bin",
		"HOME=/home/op",
		"LCX_NOT_LOCALE=keep-me",
	}

	got := filteredEnv(in)

	wantDropped := []string{"LANG=", "LANGUAGE=", "LC_ALL=", "LC_CTYPE=", "LC_TIME="}
	for _, prefix := range wantDropped {
		for _, kv := range got {
			if hasPrefix(kv, prefix) {
				t.Errorf("filteredEnv kept %q, want it dropped", kv)
			}
		}
	}

	wantKept := []string{
		"SSH_AUTH_SOCK=/tmp/agent.sock",
		"PATH=/usr/bin:/bin",
		"HOME=/home/op",
		"LCX_NOT_LOCALE=keep-me",
	}
	for _, w := range wantKept {
		found := false
		for _, kv := range got {
			if kv == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("filteredEnv dropped %q, want it kept: %v", w, got)
		}
	}

	if len(got) != len(in)-5 {
		t.Errorf("filteredEnv returned %d entries, want %d (5 locale vars dropped): %v",
			len(got), len(in)-5, got)
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// TestEverySSHSiteUsesFilteredEnv pins that every ssh invocation Exec makes
// goes through the ONE choke point (sshCmd), not seven independent
// exec.Command/exec.CommandContext calls that could individually forget the
// filter. The var is swapped for a spy that records the Env each site built
// and hands back a harmless local command instead of really invoking ssh —
// no real ssh is ever run, no network touched.
func TestEverySSHSiteUsesFilteredEnv(t *testing.T) {
	var gotEnvs [][]string
	restore := sshCmd
	sshCmd = func(ctx context.Context, argv []string) *exec.Cmd {
		c := restore(ctx, argv) // exercise the real construction, including filteredEnv
		gotEnvs = append(gotEnvs, c.Env)
		// Redirect to a harmless local binary so the test never dials out.
		stub := exec.CommandContext(ctx, "true")
		stub.Env = c.Env
		return stub
	}
	t.Cleanup(func() { sshCmd = restore })

	e := Exec{}
	_, _ = e.Run("host-a", "echo", "hi")
	_ = e.RunInteractive("host-a", "echo", "hi")
	_ = e.RunInteractiveCtx(context.Background(), "host-a", "echo", "hi")
	_, _ = e.RunVia("peer-a", "host-a", "echo", "hi")
	_, _ = e.RunStdin("host-a", "secret", "echo", "hi")
	lines, done := e.RunStreamCtx(context.Background(), "host-a", "", "echo", "hi")
	for range lines {
	}
	<-done
	split, done2 := e.RunSplitStreamCtx(context.Background(), "host-a", "", "echo", "hi")
	for range split {
	}
	<-done2

	if len(gotEnvs) != 7 {
		t.Fatalf("got %d ssh invocations, want 7 (one per Runner method)", len(gotEnvs))
	}
	for i, env := range gotEnvs {
		for _, kv := range env {
			if hasPrefix(kv, "LANG=") || hasPrefix(kv, "LC_") {
				t.Errorf("invocation %d carried a locale var: %q", i, kv)
			}
		}
	}
}
