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

func TestOpenKnownProvidersNotYetInstalled(t *testing.T) {
	for _, p := range []string{"op", "lpass"} {
		if _, err := Open(vault.Config{Provider: p}, nil); err != vault.ErrNotInstalled {
			t.Errorf("%s: %v", p, err)
		}
	}
}
