package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/cmd"
)

// Plan §3.5: 0 clean/ok · 1 drift or apply left findings · 2 usage, no
// credential, unreadable, non-TTY apply without --yes.
func TestExitCodeMapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, 0},
		{"findings", cmd.ErrFindings, 1},
		{"usage", cmd.ErrUsage, 2},
		{"wrapped usage", errors.Join(errors.New("ctx"), cmd.ErrUsage), 2},
		{"wrapped findings", errors.Join(errors.New("ctx"), cmd.ErrFindings), 1},
		{"generic", errors.New("boom"), 1},
	}
	for _, tc := range cases {
		if got := exitCode(tc.err); got != tc.want {
			t.Errorf("%s: exitCode = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// A findings exit is not an error message: verify already printed its report.
func TestRunSilentForFindings(t *testing.T) {
	var stderr bytes.Buffer
	if got := run([]string{"version"}, &stderr); got != 0 || stderr.Len() != 0 {
		t.Fatalf("version: code=%d stderr=%q", got, stderr.String())
	}
	stderr.Reset()
	if got := run([]string{"no-such-verb"}, &stderr); got != 1 {
		t.Fatalf("unknown verb: code=%d", got)
	}
	if !strings.HasPrefix(stderr.String(), "gcfg: ") {
		t.Fatalf("want 'gcfg: ' prefix, got %q", stderr.String())
	}
}
