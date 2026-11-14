package cmd

import (
	"bytes"
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
	"golang.org/x/crypto/ssh"

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
const keychainBlock = "Host *\n    IgnoreUnknown UseKeychain\n    UseKeychain yes\n    AddKeysToAgent yes\n"

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
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	sshDir := filepath.Join(home, ".ssh")

	// PLAN: fetch, decode and check every item, and ask every question, before
	// the first write, so a bad item cannot leave a half-applied pull.
	var kp vault.KeyPair
	haveKey, restore := false, false
	var vfp string
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
	if haveKey {
		if vfp, err = keystore.Fingerprint([]byte(kp.Public)); err != nil {
			return fmt.Errorf("key/%s: public key: %w", ws, err)
		}
		if pfp, perr := privateFingerprint(kp.Private, kp.Passphrase); perr != nil || pfp != vfp {
			return fmt.Errorf("key/%s: the vault's private key does not match its public key or its passphrase (%v): refusing to restore it", ws, orErr(perr))
		}
		if _, kerr := os.Stat(cfg.Key); errors.Is(kerr, fs.ErrNotExist) {
			restore = true
		} else if kerr != nil {
			return kerr
		} else if !localKeyIs(cfg.Key, kp, vfp) {
			if !replaceKey {
				lfp := ""
				if pub, rerr := os.ReadFile(cfg.Key + ".pub"); rerr == nil {
					lfp, _ = keystore.Fingerprint(pub)
				}
				return fmt.Errorf("%s already holds a different key (%s; the vault's key/%s is %s): pass --replace-key to replace it (the old key is backed up first)", cfg.Key, orNone(lfp), ws, vfp)
			}
			restore = true
		}
	}
	invPlan, err := planInventory(ctx, cmd.InOrStdin(), out, store, names)
	if err != nil {
		return err
	}
	khPlan, err := planKnownHosts(ctx, store, names, filepath.Join(sshDir, "known_hosts"))
	if err != nil {
		return err
	}

	// APPLY
	if !haveKey {
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
	} else if restore {
		secureDir(out, filepath.Dir(cfg.Key))
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
	} else {
		fmt.Fprintf(out, "key: unchanged (%s)\n", vfp)
	}
	out.Write([]byte(invPlan.msg))
	if invPlan.write {
		secureDir(out, filepath.Dir(flagConfig))
		_, serr := os.Stat(flagConfig)
		if err := os.MkdirAll(filepath.Dir(flagConfig), 0o700); err != nil {
			return err
		}
		if err := writeConfig(flagConfig, invPlan.next); err != nil {
			return err
		}
		if serr == nil {
			fmt.Fprintf(out, "updated %s (a timestamped backup was written alongside it)\n", flagConfig)
		} else {
			fmt.Fprintf(out, "created %s\n", flagConfig)
		}
	}
	if khPlan.write {
		secureDir(out, sshDir)
	}
	if err := khPlan.apply(out); err != nil {
		return err
	}
	if as != "" && as != cfg.Workstation {
		cfg.Workstation = as
		if err := saveVaultConfig(vaultConfigPath(), cfg); err != nil {
			return err
		}
		fmt.Fprintf(out, "workstation: this machine is now %q in %s\n", as, vaultConfigPath())
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

func orErr(err error) string {
	if err == nil {
		return "fingerprints differ"
	}
	return err.Error()
}

// privateFingerprint decrypts an OpenSSH private key and returns its public
// key's fingerprint.
func privateFingerprint(priv, pass []byte) (string, error) {
	var k any
	var err error
	if len(pass) > 0 {
		k, err = ssh.ParseRawPrivateKeyWithPassphrase(priv, pass)
	} else {
		k, err = ssh.ParseRawPrivateKey(priv)
	}
	if err != nil {
		return "", err
	}
	s, err := ssh.NewSignerFromKey(k)
	if err != nil {
		return "", err
	}
	return ssh.FingerprintSHA256(s.PublicKey()), nil
}

// localKeyIs reports whether the local private key IS the vault's: identical
// bytes, or one that decrypts with the vault passphrase to the same key. The
// .pub file alone is never trusted.
func localKeyIs(path string, kp vault.KeyPair, vfp string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	if bytes.Equal(b, kp.Private) {
		return true
	}
	fp, err := privateFingerprint(b, kp.Passphrase)
	return err == nil && fp == vfp
}

// secureDir makes a directory this pull writes into 0700. A symlinked
// directory is never chmodded (that would change its target): warn instead
// when the target is group/world-accessible.
func secureDir(out io.Writer, dir string) {
	fi, err := os.Lstat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		_ = os.MkdirAll(dir, 0o700)
	case err != nil:
	case fi.Mode()&fs.ModeSymlink != 0:
		if ti, terr := os.Stat(dir); terr == nil && ti.Mode().Perm()&0o077 != 0 {
			fmt.Fprintf(out, "warning: %s is a symlink to a group/world-accessible directory (mode %04o); not changing it\n", dir, ti.Mode().Perm())
		}
	case fi.Mode().Perm() != 0o700:
		_ = os.Chmod(dir, 0o700)
	}
}

type invPlanT struct {
	msg   string
	next  string
	write bool
}

// planInventory computes the merged ssh config (never replacing) and, on
// macOS, asks whether to add the Host * keychain block. It writes nothing.
func planInventory(ctx context.Context, in io.Reader, out io.Writer, store vault.Store, names []string) (invPlanT, error) {
	var p invPlanT
	local, err := readConfig(flagConfig)
	if err != nil {
		return p, err
	}
	next := local
	var msg strings.Builder
	if !contains(names, "inventory") {
		msg.WriteString("inventory: not in vault\n")
	} else {
		b, gerr := store.Get(ctx, "inventory")
		if gerr != nil {
			return p, fmt.Errorf("reading inventory: %w", gerr)
		}
		inv, derr := vault.Decode[vault.Inventory](b.Data)
		if derr != nil {
			return p, fmt.Errorf("inventory: %w", derr)
		}
		cp, perr := cfgplan.Build(local, inv.Config, cfgplan.Opts{Marker: flagMarker, Source: "vault"})
		if perr != nil {
			return p, perr
		}
		if cp.Empty() {
			msg.WriteString("inventory: unchanged\n")
		} else {
			if next, err = cp.Apply(local); err != nil {
				return p, err
			}
			add, upd := 0, 0
			for _, c := range cp.Changes {
				switch c.Kind {
				case cfgplan.Add:
					add++
				case cfgplan.Update:
					upd++
				}
			}
			fmt.Fprintf(&msg, "inventory: %d host(s) added, %d updated\n", add, upd)
		}
		if len(cp.NotImported) > 0 {
			fmt.Fprintf(&msg, "inventory: not carried (no model for them): %s\n", strings.Join(cp.NotImported, ", "))
		}
	}
	if vaultGOOS() == "darwin" && !hasUseKeychain(next) {
		fmt.Fprintf(out, "macOS: add a `Host *` block with UseKeychain yes and AddKeysToAgent yes to %s?\n", flagConfig)
		if confirm(in, out, "add it?") {
			if next != "" && !strings.HasSuffix(next, "\n") {
				next += "\n"
			}
			if next != "" {
				next += "\n"
			}
			next += keychainBlock
		}
	}
	return invPlanT{msg: msg.String(), next: next, write: next != local}, nil
}

func confirm(in io.Reader, out io.Writer, q string) bool {
	fmt.Fprintf(out, "%s [y/N] ", q)
	var ans string
	if _, err := fmt.Fscanln(in, &ans); err != nil {
		return false
	}
	ans = strings.ToLower(strings.TrimSpace(ans))
	return ans == "y" || ans == "yes"
}

func hasUseKeychain(cfg string) bool {
	for _, l := range strings.Split(cfg, "\n") {
		if f := strings.Fields(l); len(f) > 0 && strings.EqualFold(f[0], "usekeychain") {
			return true
		}
	}
	return false
}

type khPlanT struct {
	path  string
	old   []byte
	mode  fs.FileMode
	add   []string
	write bool
}

// planKnownHosts decides which vault lines to add. A host that already has ANY
// line locally (plain, [host]:port or hashed) gets nothing from the vault: its
// local key may have rotated, and trusting both would be wrong. Only hosts
// absent locally take vault lines; @revoked lines are added when missing.
func planKnownHosts(ctx context.Context, store vault.Store, names []string, path string) (khPlanT, error) {
	p := khPlanT{path: path, mode: 0o644}
	if !contains(names, "known_hosts") {
		return p, nil
	}
	b, err := store.Get(ctx, "known_hosts")
	if err != nil {
		return p, fmt.Errorf("reading known_hosts: %w", err)
	}
	kh, err := vault.Decode[vault.KnownHosts](b.Data)
	if err != nil {
		return p, fmt.Errorf("known_hosts: %w", err)
	}
	if fi, serr := os.Stat(path); serr == nil {
		p.mode = fi.Mode().Perm()
	}
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return p, err
	}
	p.old = old
	locals := strings.Split(string(old), "\n")
	have := map[string]bool{}
	for _, l := range locals {
		have[strings.TrimSpace(l)] = true
	}
	for _, l := range kh.Lines {
		l = strings.TrimSpace(l)
		if l == "" || have[l] {
			continue
		}
		if !strings.HasPrefix(l, "@revoked") {
			if hn := knownHostNames(l); len(hn) > 0 {
				seen := false
				for _, ll := range locals {
					if knownHostsLineMatches(ll, hn) {
						seen = true
						break
					}
				}
				if seen {
					continue
				}
			}
		}
		have[l] = true
		p.add = append(p.add, l)
	}
	p.write = len(p.add) > 0
	return p, nil
}

// knownHostNames lists the concrete (unhashed, non-pattern) names of a line.
func knownHostNames(line string) []string {
	f := strings.Fields(line)
	if len(f) > 0 && strings.HasPrefix(f[0], "@") {
		f = f[1:]
	}
	if len(f) == 0 || strings.HasPrefix(f[0], "#") {
		return nil
	}
	var out []string
	for _, h := range strings.Split(f[0], ",") {
		if strings.HasPrefix(h, "|1|") || strings.ContainsAny(h, "*?!") {
			continue
		}
		out = append(out, h)
	}
	return out
}

func (p khPlanT) apply(out io.Writer) error {
	if !p.write {
		fmt.Fprintln(out, "known_hosts: unchanged")
		return nil
	}
	if len(p.old) > 0 {
		if _, err := keystore.Backup(p.path, p.old, vaultNow()); err != nil {
			return fmt.Errorf("backing up %s: %w", p.path, err)
		}
	}
	body := string(p.old)
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	body += strings.Join(p.add, "\n") + "\n"
	if err := keystore.AtomicWrite(p.path, []byte(body), p.mode); err != nil {
		return err
	}
	fmt.Fprintf(out, "known_hosts: %d line(s) added to %s\n", len(p.add), p.path)
	return nil
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
