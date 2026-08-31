package sshfail

import (
	"errors"
	"os/exec"
	"testing"
)

// realExitError produces the EXACT error shape runner.Exec.Run returns: a
// *exec.ExitError whose Stderr was captured by (*exec.Cmd).Output(). Building
// one by hand would leave ProcessState nil and test a shape the runner never
// actually emits.
func realExitError(t *testing.T, stderr string) error {
	t.Helper()
	_, err := exec.Command("sh", "-c", `printf '%s' "$1" >&2; exit 255`, "sh", stderr).Output()
	if err == nil {
		t.Fatal("setup: wanted a failing command, got success")
	}
	return err
}

// The whole point of the package: ssh answering and then REFUSING us is not
// the same fact as ssh never getting through, and an operator sent to the
// network layer for a trust problem debugs the wrong machine.
func TestClassifySeparatesRefusedTrustFromNoConnection(t *testing.T) {
	for _, tc := range []struct {
		name, stderr string
		want         Kind
	}{
		{"unknown host key under BatchMode", "Host key verification failed.", Auth},
		{"host key changed", "@@@@@@ WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED! @@@@@@", Auth},
		{"no usable credential", "wenlock@host: Permission denied (publickey).", Auth},
		{"nothing listening", "ssh: connect to host h port 22: Connection refused", Network},
		{"host down", "ssh: connect to host h port 22: Connection timed out", Network},
		{"no route", "ssh: connect to host h port 22: No route to host", Network},
		{"name does not resolve", "ssh: Could not resolve hostname h: Name or service not known", Network},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(realExitError(t, tc.stderr)); got != tc.want {
				t.Fatalf("Classify(%q) = %q, want %q", tc.stderr, got, tc.want)
			}
		})
	}
}

// A fake runner in a unit test returns a plain error with no stderr at all.
// Guessing Auth there would invent a diagnosis from nothing; the honest answer
// is Unknown, which callers treat exactly as they treated every failure before.
func TestClassifyDoesNotInventADiagnosis(t *testing.T) {
	if got := Classify(errors.New("boom")); got != Unknown {
		t.Fatalf("plain error = %q, want %q", got, Unknown)
	}
	if got := Classify(nil); got != Unknown {
		t.Fatalf("nil error = %q, want %q", got, Unknown)
	}
}

// The class says "auth-failed"; the note says WHICH kind, because the two have
// completely different fixes: one is ssh-keygen -R, the other is a key.
func TestNoteNamesTheActualFault(t *testing.T) {
	for _, tc := range []struct{ stderr, want string }{
		{"Host key verification failed.", "host key unverified"},
		{"@@@@@@ WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED! @@@@@@", "host key CHANGED"},
		{"wenlock@host: Permission denied (publickey).", "permission denied"},
	} {
		if got := Note(realExitError(t, tc.stderr)); got != tc.want {
			t.Fatalf("Note(%q) = %q, want %q", tc.stderr, got, tc.want)
		}
	}
	if got := Note(errors.New("boom")); got != "" {
		t.Fatalf("Note(plain error) = %q, want empty", got)
	}
}
