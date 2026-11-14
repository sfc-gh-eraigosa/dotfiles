package providers

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault"
)

func TestSystemExecPipesStdin(t *testing.T) {
	out, err := SystemExec(context.Background(), "cat", []byte("hello"))
	if err != nil || string(out) != "hello" {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestSystemExecFailureCarriesStatusAndStderrNotStdin(t *testing.T) {
	_, err := SystemExec(context.Background(), "sh", []byte("SECRET-STDIN"), "-c", "echo  oops >&2; exit 3")
	if err == nil {
		t.Fatal("want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "3") || !strings.Contains(msg, "oops") || strings.Contains(msg, "SECRET-STDIN") {
		t.Fatalf("err = %q", msg)
	}
	if _, err := SystemExec(context.Background(), "false", nil); err == nil {
		t.Fatal("false should fail")
	}
}

func TestSystemExecMissingBinaryIsNotInstalled(t *testing.T) {
	_, err := SystemExec(context.Background(), "definitely-not-a-real-binary-xyz", nil)
	if !errors.Is(err, vault.ErrNotInstalled) {
		t.Fatalf("err = %v", err)
	}
}
