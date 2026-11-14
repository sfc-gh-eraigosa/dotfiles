package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/agent"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/cfgplan"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/keystore"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault"
)

// Test seams for `vault pull`: the platform, the clock and the agent loader
// (the real one runs ssh-add with the askpass socket).
var (
	vaultGOOS = func() string { return runtime.GOOS }
	vaultNow  = time.Now
	agentLoad = func(ctx context.Context, keyPath string, pass vault.Secret) error {
		self, err := os.Executable()
		if err != nil {
			return err
		}
		return agent.Load(ctx, agent.SystemExec, self, keyPath, pass, runtime.GOOS)
	}
)

// keychainBlock is the macOS-only Host * stanza offered after a pull.
const keychainBlock = "Host *\n    UseKeychain yes\n    AddKeysToAgent yes\n"

func newVaultPullCmd() *cobra.Command {
	var as string
	var replaceKey bool
	c := &cobra.Command{
		Use:   "pull",
		Short: "Restore this workstation's SSH key, the fleet inventory and known_hosts from the vault",
		Args:  cobra.NoArgs,
		RunE: gated(func(cmd *cobra.Command) error {
			return vaultPull(cmd, as, replaceKey)
		}),
	}
	c.Flags().StringVar(&as, "as", "", "restore the key stored for this workstation name (default: this workstation)")
	c.Flags().BoolVar(&replaceKey, "replace-key", false, "replace an existing local key that differs from the vault's (the old one is backed up)")
	return c
}

func vaultPull(cmd *cobra.Command, as string, replaceKey bool) error {
	out := cmd.OutOrStdout()
	cfg, ok, err := loadVaultConfig(vaultConfigPath())
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("not configured: run fleet vault setup")
	}
	ws := cfg.Workstation
	if as != "" {
		ws = as
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	store, err := openVault(cfg)
	if err != nil {
		return err
	}
	if err := store.Ready(ctx); err != nil {
		if errors.Is(err, vault.ErrSignedOut) {
			return fmt.Errorf("%s is signed out: run fleet vault setup to sign in", store.Name())
		}
		return fmt.Errorf("%s is not ready: %w", store.Name(), err)
	}
	names, err := store.List(ctx)
	if err != nil {
		return fmt.Errorf("listing the vault: %w", err)
	}

	// Everything that can refuse does so before anything is written.
	var kp vault.KeyPair
	haveKey, restore := false, false
	switch b, gerr := store.Get(ctx, "key/"+ws); {
	case errors.Is(gerr, vault.ErrNotFound):
	case gerr != nil:
		return fmt.Errorf("reading key/%s: %w", ws, gerr)
	default:
		if kp, err = vault.Decode[vault.KeyPair](b.Data); err != nil {
			return fmt.Errorf("key/%s: %w", ws, err)
		}
		haveKey = true
	}
	var vfp string
	if haveKey {
		if vfp, err = keystore.Fingerprint([]byte(kp.Public)); err != nil {
			return fmt.Errorf("key/%s: public key: %w", ws, err)
		}
		lfp := ""
		if pub, rerr := os.ReadFile(cfg.Key + ".pub"); rerr == nil {
			lfp, _ = keystore.Fingerprint(pub)
		}
		_, kerr := os.Stat(cfg.Key)
		switch {
		case kerr == nil && lfp == vfp:
		case kerr == nil && !replaceKey:
			return fmt.Errorf("%s already holds a different key (%s; the vault's key/%s is %s): pass --replace-key to replace it (the old key is backed up first)", cfg.Key, orNone(lfp), ws, vfp)
		case kerr != nil && !errors.Is(kerr, fs.ErrNotExist):
			return kerr
		default:
			restore = true
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	sshDir := filepath.Join(home, ".ssh")
	if err := ensureDir0700(sshDir); err != nil {
		return err
	}

	// key
	switch {
	case !haveKey:
		var others []string
		for _, n := range names {
			if w, ok := strings.CutPrefix(n, "key/"); ok {
				others = append(others, w)
			}
		}
		sort.Strings(others)
		ex := "none"
		if len(others) > 0 {
			ex = strings.Join(others, ", ")
		}
		fmt.Fprintf(out, "key: the vault has no key for %s (existing: %s)\n", ws, ex)
		fmt.Fprintln(out, "  to make one: fleet keys rotate --new")
		fmt.Fprintln(out, "  then run `fleet keys sync` on a trusted workstation to authorize it")
	case restore:
		pub := kp.Public
		if !strings.HasSuffix(pub, "\n") {
			pub += "\n"
		}
		bak, werr := keystore.Write(cfg.Key, kp.Private, []byte(pub), vaultNow())
		if bak != "" {
			fmt.Fprintf(out, "key: backed up the previous key to %s\n", bak)
		}
		if werr != nil {
			return werr
		}
		fmt.Fprintf(out, "key: restored %s (%s)\n", cfg.Key, vfp)
	default:
		fmt.Fprintf(out, "key: unchanged (%s)\n", vfp)
	}

	// inventory
	if err := pullInventory(cmd, store, names, out); err != nil {
		return err
	}
	// known_hosts
	if err := pullKnownHosts(ctx, store, names, filepath.Join(sshDir, "known_hosts"), out); err != nil {
		return err
	}

	if haveKey {
		pullLoadAgent(ctx, out, cfg.Key, vfp, kp.Passphrase)
	}
	return nil
}

func orNone(s string) string {
	if s == "" {
		return "unreadable or no public key"
	}
	return s
}

func ensureDir0700(dir string) error {
	fi, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return os.MkdirAll(dir, 0o700)
	}
	if err != nil {
		return err
	}
	if fi.Mode().Perm() != 0o700 {
		return os.Chmod(dir, 0o700)
	}
	return nil
}

// pullInventory merges the vault's #fleet Host blocks into the ssh config the
// way `fleet config pull` does (never replacing; backup before the write), and
// on macOS offers the Host * keychain block after a y/N.
func pullInventory(cmd *cobra.Command, store vault.Store, names []string, out io.Writer) error {
	local, err := readConfig(flagConfig)
	if err != nil {
		return err
	}
	next := local
	if !contains(names, "inventory") {
		fmt.Fprintln(out, "inventory: not in vault")
	} else {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		b, gerr := store.Get(ctx, "inventory")
		if gerr != nil {
			return fmt.Errorf("reading inventory: %w", gerr)
		}
		inv, derr := vault.Decode[vault.Inventory](b.Data)
		if derr != nil {
			return fmt.Errorf("inventory: %w", derr)
		}
		p, perr := cfgplan.Build(local, inv.Config, cfgplan.Opts{Marker: flagMarker, Source: "vault"})
		if perr != nil {
			return perr
		}
		if p.Empty() {
			fmt.Fprintln(out, "inventory: unchanged")
		} else {
			if next, err = p.Apply(local); err != nil {
				return err
			}
			add, upd := 0, 0
			for _, c := range p.Changes {
				switch c.Kind {
				case cfgplan.Add:
					add++
				case cfgplan.Update:
					upd++
				}
			}
			fmt.Fprintf(out, "inventory: %d host(s) added, %d updated\n", add, upd)
		}
		if len(p.NotImported) > 0 {
			fmt.Fprintf(out, "inventory: not carried (no model for them): %s\n", strings.Join(p.NotImported, ", "))
		}
	}
	if vaultGOOS() == "darwin" && !hasUseKeychain(next) {
		fmt.Fprintf(out, "macOS: add a `Host *` block with UseKeychain yes and AddKeysToAgent yes to %s?\n", flagConfig)
		if askYesNo(cmd, "add it?") {
			if next != "" && !strings.HasSuffix(next, "\n") {
				next += "\n"
			}
			if next != "" {
				next += "\n"
			}
			next += keychainBlock
		}
	}
	if next == local {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(flagConfig), 0o700); err != nil {
		return err
	}
	if err := writeConfig(flagConfig, next); err != nil {
		return err
	}
	fmt.Fprintf(out, "updated %s (a timestamped backup was written alongside it)\n", flagConfig)
	return nil
}

func hasUseKeychain(cfg string) bool {
	for _, l := range strings.Split(cfg, "\n") {
		if f := strings.Fields(l); len(f) > 0 && strings.EqualFold(f[0], "usekeychain") {
			return true
		}
	}
	return false
}

// pullKnownHosts appends the vault's lines that known_hosts lacks. Existing
// lines are never removed; the file is backed up (O_EXCL) before it changes.
func pullKnownHosts(ctx context.Context, store vault.Store, names []string, path string, out io.Writer) error {
	if !contains(names, "known_hosts") {
		fmt.Fprintln(out, "known_hosts: not in vault")
		return nil
	}
	b, err := store.Get(ctx, "known_hosts")
	if err != nil {
		return fmt.Errorf("reading known_hosts: %w", err)
	}
	kh, err := vault.Decode[vault.KnownHosts](b.Data)
	if err != nil {
		return fmt.Errorf("known_hosts: %w", err)
	}
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	have := map[string]bool{}
	for _, l := range strings.Split(string(old), "\n") {
		have[strings.TrimSpace(l)] = true
	}
	var add []string
	for _, l := range kh.Lines {
		if l = strings.TrimSpace(l); l != "" && !have[l] {
			have[l] = true
			add = append(add, l)
		}
	}
	if len(add) == 0 {
		fmt.Fprintln(out, "known_hosts: unchanged")
		return nil
	}
	if len(old) > 0 {
		if _, err := backupFile(path, old, vaultNow()); err != nil {
			return err
		}
	}
	body := string(old)
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	body += strings.Join(add, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return err
	}
	fmt.Fprintf(out, "known_hosts: %d line(s) added to %s\n", len(add), path)
	return nil
}

// backupFile writes data to <path>.bak-<ts>[-N] exclusively (0600).
func backupFile(path string, data []byte, now time.Time) (string, error) {
	base := path + ".bak-" + now.Format("20060102T150405")
	for i := 0; i < 1000; i++ {
		p := base
		if i > 0 {
			p = fmt.Sprintf("%s-%d", base, i)
		}
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("backing up %s: %w", path, err)
		}
		_, werr := f.Write(data)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return "", fmt.Errorf("backing up %s: %w", path, werr)
		}
		return p, nil
	}
	return "", fmt.Errorf("backing up %s: too many backups", path)
}

// pullLoadAgent loads the key unless the agent already has it. Failures are
// reported, not fatal: the key is restored either way.
func pullLoadAgent(ctx context.Context, out io.Writer, keyPath, fp string, pass vault.Secret) {
	loaded, err := agentLoaded(ctx, fp)
	switch {
	case errors.Is(err, agent.ErrNoAgent):
		fmt.Fprintln(out, "agent: no agent running; start one and run fleet keys unlock")
		return
	case err != nil:
		fmt.Fprintf(out, "agent: check failed: %v\n", err)
		return
	case loaded:
		fmt.Fprintln(out, "agent: key already loaded")
		return
	}
	if err := agentLoad(ctx, keyPath, pass); err != nil {
		fmt.Fprintf(out, "agent: loading the key failed: %v\n", err)
		return
	}
	if ok, err := agentLoaded(ctx, fp); err != nil || !ok {
		fmt.Fprintln(out, "agent: ssh-add ran but the key is not listed in the agent")
		return
	}
	fmt.Fprintln(out, "agent: key loaded in agent")
}
