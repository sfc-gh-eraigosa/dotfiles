package runner

import (
	"context"
	"testing"
	"time"
)

// stubSSH (runner_ctx_test.go) installs a fake `ssh` on PATH, so these run
// against a REAL process without opening a socket.

func TestRunSplitStreamCtxSeparatesTheStreams(t *testing.T) {
	stubSSH(t, "echo out1; echo err1 >&2; echo out2")

	e := Exec{}
	lines, done := e.RunSplitStreamCtx(context.Background(), "host", "", "whatever")

	var stdout, stderr []string
	for l := range lines {
		if l.Stderr {
			stderr = append(stderr, l.Text)
		} else {
			stdout = append(stdout, l.Text)
		}
	}
	if err := <-done; err != nil {
		t.Fatalf("stub should exit 0, got %v", err)
	}
	if len(stdout) != 2 || stdout[0] != "out1" || stdout[1] != "out2" {
		t.Fatalf("stdout order lost: %q", stdout)
	}
	if len(stderr) != 1 || stderr[0] != "err1" {
		t.Fatalf("stderr not separated: %q", stderr)
	}
}

// Both pipes must drain concurrently: a reader that services one stream at a
// time lets the other pipe's buffer fill and wedges the remote command. This
// is the deadlock the split introduces if it is written naively.
func TestSplitStreamDoesNotDeadlockUnderBackpressure(t *testing.T) {
	stubSSH(t, "i=0; while [ $i -lt 2000 ]; do echo out$i; echo err$i >&2; i=$((i+1)); done")

	e := Exec{}
	lines, done := e.RunSplitStreamCtx(context.Background(), "host", "", "whatever")

	var n, nerr int
	for l := range lines {
		n++
		if l.Stderr {
			nerr++
		}
	}
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("RunSplitStreamCtx deadlocked under a 4000-line burst")
	}
	if n != 4000 || nerr != 2000 {
		t.Fatalf("lost lines: total=%d stderr=%d, want 4000/2000", n, nerr)
	}
}

// RunStreamCtx keeps its signature AND its content: it is the split stream
// with the tag dropped, so the two implementations cannot drift.
func TestRunStreamMatchesSplitStreamMerged(t *testing.T) {
	stubSSH(t, "echo a; echo b >&2; echo c")

	e := Exec{}
	merged, mdone := e.RunStreamCtx(context.Background(), "host", "", "x")
	var got []string
	for l := range merged {
		got = append(got, l)
	}
	<-mdone
	want := map[string]bool{"a": true, "b": true, "c": true}
	if len(got) != 3 {
		t.Fatalf("merged stream lost lines: %q", got)
	}
	for _, g := range got {
		if !want[g] {
			t.Fatalf("unexpected line %q in %q", g, got)
		}
	}
}

// The deadline guarantee must survive the rewrite.
func TestRunSplitStreamCtxKillsTheChildOnDeadline(t *testing.T) {
	stubSSH(t, "sleep 30")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	e := Exec{}
	lines, done := e.RunSplitStreamCtx(ctx, "host", "", "x")
	for range lines {
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error from a killed child")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("deadline not honoured within 2s")
	}
}

func TestFakeReplaysErrOutAsStderr(t *testing.T) {
	f := Fake{
		Out:    map[string]string{"h": "one\ntwo"},
		ErrOut: map[string]string{"h": "boom"},
	}
	lines, done := f.RunSplitStreamCtx(context.Background(), "h", "", "x")

	var out, errs []string
	for l := range lines {
		if l.Stderr {
			errs = append(errs, l.Text)
		} else {
			out = append(out, l.Text)
		}
	}
	if err := <-done; err != nil {
		t.Fatalf("no Err scripted, want nil, got %v", err)
	}
	if len(out) != 2 || out[0] != "one" || out[1] != "two" {
		t.Fatalf("stdout replay wrong: %q", out)
	}
	if len(errs) != 1 || errs[0] != "boom" {
		t.Fatalf("stderr replay wrong: %q", errs)
	}
}

// The merged path must still see BOTH streams, or every existing test that
// asserts on Fake output would silently start missing lines.
func TestFakeMergedStreamStillCarriesErrOut(t *testing.T) {
	f := Fake{Out: map[string]string{"h": "one"}, ErrOut: map[string]string{"h": "boom"}}
	lines, _ := f.RunStreamCtx(context.Background(), "h", "", "x")
	var got []string
	for l := range lines {
		got = append(got, l)
	}
	if len(got) != 2 {
		t.Fatalf("merged Fake stream lost a line: %q", got)
	}
}

// Fake must still record what was piped to it: several tests assert the sudo
// secret travelled over stdin and NOT argv.
func TestFakeSplitStreamStillRecordsStdin(t *testing.T) {
	f := Fake{Out: map[string]string{"h": "x"}, Stdin: map[string]string{}}
	lines, _ := f.RunSplitStreamCtx(context.Background(), "h", "hunter2\n", "x")
	for range lines {
	}
	if f.Stdin["h"] != "hunter2\n" {
		t.Fatalf("stdin not recorded: %q", f.Stdin["h"])
	}
}

var (
	_ SplitStreamer = Fake{}
	_ SplitStreamer = Exec{}
)
