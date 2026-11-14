package providers

import (
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault"
)

func TestOpenRejectsAnUnknownProvider(t *testing.T) {
	_, err := Open(vault.Config{Provider: "keepass"}, nil)
	if err == nil || !strings.Contains(err.Error(), `"op"`) || !strings.Contains(err.Error(), `"lpass"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestOpenOpReturnsTheAdapter(t *testing.T) {
	s, err := Open(vault.Config{Provider: "op"}, SystemExec)
	if err != nil || s.Name() != "op" {
		t.Fatalf("s=%v err=%v", s, err)
	}
}

func TestOpenLpassReturnsTheAdapter(t *testing.T) {
	s, err := Open(vault.Config{Provider: "lpass"}, SystemExec)
	if err != nil || s.Name() != "lpass" {
		t.Fatalf("s=%v err=%v", s, err)
	}
}
