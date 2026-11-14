// Package providers maps a vault.Config to a concrete vault.Store.
package providers

import (
	"fmt"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault/lpass"
)

// Open returns the Store for cfg.Provider. The "op" adapter lands in a later
// task; until then it reports ErrNotInstalled.
func Open(cfg vault.Config, x vault.Exec) (vault.Store, error) {
	switch cfg.Provider {
	case "lpass":
		return lpass.New(cfg, x), nil
	case "op":
		return nil, vault.ErrNotInstalled
	default:
		return nil, fmt.Errorf("unknown vault provider %q (supported: %q, %q)", cfg.Provider, "op", "lpass")
	}
}
