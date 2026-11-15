// Package providers maps a vault.Config to a concrete vault.Store.
package providers

import (
	"fmt"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault/lpass"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault/op"
)

// Open returns the Store for cfg.Provider.
func Open(cfg vault.Config, x vault.Exec) (vault.Store, error) {
	switch cfg.Provider {
	case "lpass":
		s := lpass.New(cfg, x)
		s.Interactive = Interactive
		return s, nil
	case "op":
		s := op.New(cfg, x)
		s.Interactive = Interactive
		return s, nil
	default:
		return nil, fmt.Errorf("unknown vault provider %q (supported: %q, %q)", cfg.Provider, "op", "lpass")
	}
}
