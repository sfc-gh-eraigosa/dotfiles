package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/ghapp/cmd"
)

func TestExitCodeNil(t *testing.T) {
	if got := exitCode(nil); got != 0 {
		t.Fatalf("exitCode(nil) = %d, want 0", got)
	}
}

func TestExitCodeUsage(t *testing.T) {
	if got := exitCode(cmd.ErrUsage); got != 2 {
		t.Fatalf("exitCode(cmd.ErrUsage) = %d, want 2", got)
	}
	if got := exitCode(errors.Join(errors.New("ctx"), cmd.ErrUsage)); got != 2 {
		t.Fatalf("wrapped ErrUsage: got %d, want 2", got)
	}
}

func TestExitCodeGeneric(t *testing.T) {
	if got := exitCode(errors.New("boom")); got != 1 {
		t.Fatalf("exitCode(generic) = %d, want 1", got)
	}
}

func TestRunVersionExitZero(t *testing.T) {
	var stderr bytes.Buffer
	if got := run([]string{"version"}, &stderr); got != 0 {
		t.Fatalf("run(version) = %d, want 0; stderr=%q", got, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("run(version) wrote to stderr: %q", stderr.String())
	}
}

func TestRunUnknownVerbExitOneWithMessage(t *testing.T) {
	var stderr bytes.Buffer
	if got := run([]string{"no-such-verb"}, &stderr); got != 1 {
		t.Fatalf("run(no-such-verb) = %d, want 1", got)
	}
	if !strings.HasPrefix(stderr.String(), "ghapp: ") {
		t.Fatalf("want 'ghapp: ' prefixed error on stderr, got %q", stderr.String())
	}
}
