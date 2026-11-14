// Package providers maps a vault.Config to a concrete vault.Store.
package providers

import (
	"fmt"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault"
)

// Open returns the Store for cfg.Provider. The "op" and "lpass" adapters land
// in later tasks; until then they report ErrNotInstalled.
func Open(cfg vault.Config, x vault.Exec) (vault.Store, error) {
	switch cfg.Provider {
	case "op", "lpass":
		return nil, vault.ErrNotInstalled
	default:
		return nil, fmt.Errorf("unknown vault provider %q (supported: %q, %q)", cfg.Provider, "op", "lpass")
	}
}
