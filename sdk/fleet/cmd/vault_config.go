package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/featflag"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault"
)

// vaultFile is the on-disk shape of vault.yaml.
type vaultFile struct {
	Provider    string `yaml:"provider"`
	Vault       string `yaml:"vault,omitempty"`
	Workstation string `yaml:"workstation,omitempty"`
	Key         string `yaml:"key,omitempty"`
}

// vaultConfigPath is $XDG_CONFIG_HOME/fleet/vault.yaml, else
// ~/.config/fleet/vault.yaml, on every OS. (fleetConfigFile would send macOS to
// ~/Library/Application Support; the vault contract pins the XDG location.)
func vaultConfigPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "fleet", "vault.yaml")
}

// vaultActive is true only when the gff gate is on AND a vault is configured.
func vaultActive(s featflag.Settings, configured bool) bool {
	return s.VaultEnabled && configured
}

// loadVaultConfig reads vault.yaml. A missing file is (zero, false, nil). The
// file must not be group/world-writable nor owned by someone else (the
// readPlanFile rule). Defaults are applied after load.
func loadVaultConfig(path string) (vault.Config, bool, error) {
	fi, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return vault.Config{}, false, nil
	}
	if err != nil {
		return vault.Config{}, false, fmt.Errorf("vault config: %s: %w", path, err)
	}
	switch trust, mode := fileTrust(fi); trust {
	case trustWritable:
		return vault.Config{}, false, fmt.Errorf("vault config: %s: refusing a group/world-writable file (mode %04o) - it chooses where your keys are stored", path, mode)
	case trustUnverifiable:
		return vault.Config{}, false, fmt.Errorf("vault config: %s: could not verify file ownership on this platform, refusing", path)
	case trustForeign:
		return vault.Config{}, false, fmt.Errorf("vault config: %s: refusing a file not owned by the current user", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return vault.Config{}, false, fmt.Errorf("vault config: %s: %w", path, err)
	}
	var f vaultFile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)                                             // a typo like `vualt:` must not silently fall back to the default vault
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) { // io.EOF = empty file

		return vault.Config{}, false, fmt.Errorf("vault config: %s: %w", path, err)
	}
	c := vault.Config{Provider: f.Provider, Vault: f.Vault, Workstation: f.Workstation, Key: f.Key}
	switch c.Provider {
	case "op":
		if c.Vault == "" {
			c.Vault = "Private"
		}
	case "lpass":
		if c.Vault == "" {
			c.Vault = "fleet"
		}
	default:
		return vault.Config{}, false, fmt.Errorf("vault config: %s: unknown provider %q (want \"op\" or \"lpass\")", path, c.Provider)
	}
	if c.Workstation == "" {
		h, _ := os.Hostname()
		c.Workstation, _, _ = strings.Cut(h, ".")
	}
	if c.Key == "" {
		c.Key = "~/.ssh/id_ed25519"
	}
	if c.Key == "~" || strings.HasPrefix(c.Key, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return vault.Config{}, false, fmt.Errorf("vault config: expanding %q: %w", c.Key, err)
		}
		c.Key = filepath.Join(home, strings.TrimPrefix(c.Key, "~"))
	}
	return c, true, nil
}

// saveVaultConfig writes vault.yaml 0600, creating parent dirs 0700.
func saveVaultConfig(path string, c vault.Config) error {
	data, err := yaml.Marshal(vaultFile{Provider: c.Provider, Vault: c.Vault, Workstation: c.Workstation, Key: c.Key})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600) // WriteFile keeps an existing file's mode
}
