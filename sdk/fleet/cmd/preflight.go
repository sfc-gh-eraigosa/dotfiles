package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/agent"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/keystore"
)

// lockedHint is what a vault user is told when the key is encrypted but not
// loaded in the agent.
const lockedHint = "key locked: run fleet keys unlock"

// Test seams (the agent one, agentLoaded, lives in vault.go).
var (
	keyEncrypted  = keystore.Encrypted
	preflightGOOS = runtime.GOOS
)

// keyLocked reports a POSITIVE "encrypted and not loaded in the agent" (or no
// agent at all). Any doubt (unreadable key, failed agent query) is not a lock:
// the command then behaves exactly as it did before the pre-flight existed.
// hint is the plain-ssh remedy; preflight swaps in lockedHint for vault users.
func keyLocked(keyPath, fp string, enc func(string) (bool, error), loaded func(string) (bool, error)) (locked bool, hint string) {
	if e, err := enc(keyPath); err != nil || !e {
		return false, ""
	}
	l, err := loaded(fp)
	switch {
	case errors.Is(err, agent.ErrNoAgent):
		h := `key locked and no ssh-agent is running: start one with eval "$(ssh-agent -s)", then ssh-add ` + keyPath
		if preflightGOOS == "darwin" {
			h += " (launchd normally provides one: check SSH_AUTH_SOCK in this shell)"
		}
		return true, h
	case err != nil, l:
		return false, ""
	}
	return true, "key locked: run ssh-add " + keyPath
}

// preflight stops a fleet command before any dial when its SSH key is
// passphrase-encrypted but not loaded, which would otherwise fail every host
// with an auth error. It applies with or without a vault.
func preflight(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	keyPath, vaulted := preflightKeyPath()
	fp := ""
	if pub, err := os.ReadFile(keyPath + ".pub"); err == nil {
		fp, _ = keystore.Fingerprint(pub)
	}
	if fp == "" {
		return nil
	}
	locked, hint := keyLocked(keyPath, fp, keyEncrypted, func(fp string) (bool, error) { return agentLoaded(ctx, fp) })
	if !locked {
		return nil
	}
	if vaulted && strings.HasPrefix(hint, "key locked: run ssh-add") {
		hint = lockedHint
	}
	return errors.New(hint)
}

// preflightKeyPath is the configured vault key, else the default key.
// vaulted is true only when the gff gate is on AND a vault is configured.
func preflightKeyPath() (path string, vaulted bool) {
	if cfg, ok, err := loadVaultConfig(vaultConfigPath()); err == nil && ok {
		return cfg.Key, vaultActive(vaultSettings(), ok)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ssh", "id_ed25519"), false
}
