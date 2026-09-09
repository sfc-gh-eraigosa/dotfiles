package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// stubSSH installs a fake `ssh` executable on PATH that just sleeps, so a
// test can prove RunStreamCtx actually kills the CHILD process on a
// deadline, without ever opening a real socket.
func stubSSH(t *testing.T, body string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub ssh script requires a POSIX shell")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "ssh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestRunStreamCtxKillsTheChildOnDeadline proves the context deadline
// actually terminates the local ssh child — not merely that the call
// returns quickly while a `sleep 30` lingers as an orphan.
func TestRunStreamCtxKillsTheChildOnDeadline(t *testing.T) {
	stubSSH(t, "sleep 30")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	e := Exec{}
	lines, done := e.RunStreamCtx(ctx, "host", "", "echo hi")
	for range lines {
	}

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error from a killed child, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunStreamCtx did not honour the deadline within 2s")
	}
}

// TestRunStreamDelegatesToCtx proves RunStream is RunStreamCtx with a
// background (never-cancelled) context — same behaviour, no deadline.
func TestRunStreamDelegatesToCtx(t *testing.T) {
	stubSSH(t, "echo hello")

	e := Exec{}
	lines, done := e.RunStream("host", "", "echo hi")
	var got []string
	for l := range lines {
		got = append(got, l)
	}
	if err := <-done; err != nil {
		t.Fatalf("RunStream: %v", err)
	}
	if len(got) != 1 || got[0] != "hello" {
		t.Fatalf("RunStream lines = %v, want [hello]", got)
	}
}

// TestFakeHonoursContextCancellation proves Fake.RunStreamCtx can simulate a
// blocking remote command and stops when the context is cancelled, sending
// ctx.Err() on done rather than hanging forever.
func TestFakeHonoursContextCancellation(t *testing.T) {
	f := Fake{Block: map[string]bool{"host": true}}
	ctx, cancel := context.WithCancel(context.Background())

	lines, done := f.RunStreamCtx(ctx, "host", "", "cmd")
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("done = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Fake.RunStreamCtx did not honour cancellation within 2s")
	}
	for range lines {
	}
}

// TestEveryRemotePathCarriesTheMuxOptions extends the mux-options invariant
// (see TestDirectLanesStillMultiplex) to RunStreamCtx: it must build its
// ssh argv from the SAME baseArgs every other unattended lane uses, or a
// controller-timeout path would silently authenticate separately and start
// prompting again.
func TestEveryRemotePathCarriesTheMuxOptions(t *testing.T) {
	e := Exec{}
	got := e.baseArgs("h") // the argv RunStreamCtx builds on, per RunStream/RunStdin
	if !hasPair(got, "-o", "ControlMaster=auto") {
		t.Fatalf("RunStreamCtx's base args = %q, want ControlMaster=auto", got)
	}
	if !hasPair(got, "-o", "BatchMode=yes") {
		t.Fatalf("RunStreamCtx's base args = %q, want BatchMode=yes", got)
	}
}
