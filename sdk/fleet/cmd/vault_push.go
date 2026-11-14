package cmd

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" // #nosec G505 -- known_hosts "|1|" hashing is HMAC-SHA1 by format; used only to match, never to protect
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/keystore"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault"
)

// pushNeverRemoves is said in push's help and diff: removals and cleared
// fields need roster-aware semantics (PR 3, T14), so push only merges.
const pushNeverRemoves = "push adds and updates hosts; it never removes them from the vault"

// maxPushRounds bounds the re-read/re-ask loop when another workstation keeps
// writing the same items.
const maxPushRounds = 3

// verifyLocalEncrypted re-reads the key file written by --encrypt-local and
// proves it is encrypted, opens with pass, and is the key fp names. It is a
// seam so a test can drive the "verification failed" path.
var verifyLocalEncrypted = func(path string, pass []byte, fp string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if _, err := ssh.ParseRawPrivateKey(b); err == nil {
		return errors.New("the file is still unencrypted")
	}
	raw, err := ssh.ParseRawPrivateKeyWithPassphrase(b, pass)
	if err != nil {
		return errors.New("the file does not open with the vault passphrase")
	}
	return matchesPub(raw, localKey{Path: path, Fingerprint: fp})
}

type vaultPushOpts struct {
	AllowPartial bool // upload known_hosts even when some fleet hosts are unresolved
	EncryptLocal bool // also encrypt a plain local key file (after the vault holds its passphrase)
}

func newVaultPushCmd() *cobra.Command {
	var o vaultPushOpts
	c := &cobra.Command{
		Use:   "push",
		Short: "Capture this workstation's key, the roster, inventory and known_hosts into the vault (diff first, then confirm)",
		Long: "push is one-way: it never reads anything back onto this machine. It shows what would change in\n" +
			"each of key/<workstation>, roster, inventory and known_hosts, asks once, then writes with\n" +
			"compare-and-set; if another workstation wrote in between it re-reads, re-diffs and asks again.\n" +
			"The existing local key is captured as-is; nothing new is generated.\n" +
			"Inventory: " + pushNeverRemoves + ".",
		Args: cobra.NoArgs,
		RunE: gated(func(cmd *cobra.Command) error { return vaultPush(cmd, o) }),
	}
	c.Flags().BoolVar(&o.AllowPartial, "allow-partial", false, "upload known_hosts even if some fleet hosts have no usable entry")
	c.Flags().BoolVar(&o.EncryptLocal, "encrypt-local", false, "if the local key is not passphrase-protected, encrypt it too (with the passphrase stored in the vault; backup kept)")
	return c
}

// localKey is this workstation's key as found on disk. Exported fields: it
// holds a Secret (an unexported Secret field would print raw under %v).
type localKey struct {
	Path        string
	Private     vault.Secret // file bytes as-is
	PubFile     []byte       // the .pub file as-is
	Public      string       // authorized_keys line, trimmed
	Fingerprint string
	Encrypted   bool
}

func readLocalKey(path string) (localKey, error) {
	priv, err := os.ReadFile(path)
	if err != nil {
		return localKey{}, fmt.Errorf("no readable local key at %s (set `key:` in %s): %w", path, vaultConfigPath(), err)
	}
	pub, err := os.ReadFile(path + ".pub")
	if err != nil {
		return localKey{}, fmt.Errorf("no public key at %s.pub; recreate it with `ssh-keygen -y -f %s > %s.pub`: %w", path, path, path, err)
	}
	fp, err := keystore.Fingerprint(pub)
	if err != nil {
		return localKey{}, fmt.Errorf("%s.pub: %w", path, err)
	}
	enc, err := keystore.Encrypted(path)
	if err != nil {
		return localKey{}, fmt.Errorf("%s is not a private key ssh can read", path)
	}
	return localKey{Path: path, Private: vault.Secret(priv), PubFile: pub, Public: strings.TrimSpace(string(pub)), Fingerprint: fp, Encrypted: enc}, nil
}

// pushItem is one vault item's decision. Data is filled at write time for the
// key (so the passphrase is only asked for once the operator said yes).
type pushItem struct {
	Name    string
	Version string // vault version read; "" = absent (create)
	Data    vault.Secret
	Diff    []string
	Changed bool
}

type vaultPushPlan struct {
	Items    []pushItem
	Withheld []string // items not pushed this run, with why (exit non-zero)
}

func (p vaultPushPlan) changed() []string {
	var n []string
	for _, it := range p.Items {
		if it.Changed {
			n = append(n, it.Name)
		}
	}
	return n
}

func vaultPush(cmd *cobra.Command, o vaultPushOpts) error {
	out, errw := cmd.OutOrStdout(), cmd.ErrOrStderr()
	stdin := cmd.InOrStdin() // the real *os.File in production: the sign-in child gets it as-is
	cfg, ok, err := loadVaultConfig(vaultConfigPath())
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(out, "not configured: run fleet vault setup")
		return exitError{code: 1}
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	store, err := openVault(cfg)
	if err != nil {
		return err
	}
	if err := ensureSignedIn(ctx, store, cfg.Provider, stdin, out, errw); err != nil {
		return err
	}
	// Built only AFTER sign-in, and never handed to a child process.
	in := bufio.NewReader(stdin)

	lk, err := readLocalKey(cfg.Key)
	if err != nil {
		return err
	}
	inv, invErr := fleetInventory()
	kh, unresolved, khErr := fleetKnownHosts()

	var postErr error     // a problem after a successful write (exit non-zero)
	var kp *vault.KeyPair // materialized once, reused across conflict rounds
	var vaultCopy []byte  // the encrypted bytes stored in the vault (for --encrypt-local)
	for round := 1; round <= maxPushRounds; round++ {
		plan, err := planVaultPush(ctx, store, cfg, lk, inv, invErr, kh, unresolved, khErr, o)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "fleet vault push → %s (%s), workstation %s:\n", cfg.Provider, cfg.Vault, cfg.Workstation)
		for _, it := range plan.Items {
			for _, l := range it.Diff {
				fmt.Fprintf(out, "  %s\n", l)
			}
		}
		for _, w := range plan.Withheld {
			fmt.Fprintf(out, "  %s\n", w)
		}
		withheld := withheldError(plan)
		names := plan.changed()
		if len(names) == 0 {
			fmt.Fprintln(out, "nothing to push: the vault is current")
			return withheld
		}
		fmt.Fprintf(out, "write %d item(s) (%s) to %s? [y/N] ", len(names), strings.Join(names, ", "), cfg.Provider)
		line, _ := in.ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
			fmt.Fprintln(out, "nothing written")
			return withheld
		}
		if kp == nil && slices.Contains(names, "key/"+cfg.Workstation) {
			k, enc, err := materializeKey(lk, cfg, o, stdin, in, errw)
			if err != nil {
				return err
			}
			kp, vaultCopy = &k, enc
		}

		var wrote []string
		conflict := ""
		for i, it := range plan.Items {
			if !it.Changed {
				continue
			}
			var pending []string // this item and every later one we meant to write
			for _, rest := range plan.Items[i:] {
				if rest.Changed {
					pending = append(pending, rest.Name)
				}
			}
			data := it.Data
			if it.Name == "key/"+cfg.Workstation {
				if data, err = vault.Encode(*kp); err != nil {
					return err
				}
			}
			_, perr := store.Put(ctx, it.Name, data, it.Version)
			switch {
			case errors.Is(perr, vault.ErrConflict):
				conflict = it.Name
			case errors.Is(perr, vault.ErrSignedOut):
				return fmt.Errorf("signed out of %s midway (wrote: %s; not written: %s); nothing more was written. %s, then re-run `fleet vault push`: %w",
					cfg.Provider, listOrNone(wrote), strings.Join(pending, ", "), signInHint(cfg.Provider), perr)
			case perr != nil:
				return fmt.Errorf("writing %s (wrote: %s; not written: %s); nothing more was written: %w", it.Name, listOrNone(wrote), strings.Join(pending, ", "), perr)
			}
			if conflict != "" {
				break
			}
			wrote = append(wrote, it.Name)
			if it.Name == "key/"+cfg.Workstation && o.EncryptLocal && !lk.Encrypted {
				// Only now: the passphrase that opens the re-encrypted file is
				// safely in the vault.
				pubBefore, _ := filepath.Glob(lk.Path + ".pub.bak-*")
				backup, werr := keystore.Write(lk.Path, vaultCopy, lk.PubFile, time.Now())
				if werr != nil {
					return fmt.Errorf("vault has the key, but encrypting %s failed (backup: %s): %w", lk.Path, listOrNone([]string{backup}), werr)
				}
				// The file is now encrypted: a retry round must neither
				// re-store the key nor rewrite the file again.
				lk.Encrypted, lk.Private = true, vault.Secret(vaultCopy)
				fmt.Fprintf(out, "encrypted %s with the passphrase now stored in the vault; your agent will now need `fleet keys unlock` to load it\n", lk.Path)
				keep := ""
				if verr := verifyLocalEncrypted(lk.Path, []byte(kp.Passphrase), lk.Fingerprint); verr != nil {
					keep = "could not verify the re-encrypted key (" + verr.Error() + ")"
				} else if rerr := os.Remove(backup); rerr != nil {
					keep = "could not remove the backup (" + rerr.Error() + ")"
				} else {
					removeNewPubBackups(lk.Path, pubBefore, lk.PubFile)
				}
				if keep != "" {
					postErr = fmt.Errorf("%s: %s is kept — plaintext — delete it after `fleet keys unlock` works", keep, backup)
					fmt.Fprintf(out, "WARNING: %v\n", postErr)
				} else {
					fmt.Fprintln(out, "removed the plaintext backup and its .pub copy; the vault holds the recovery copy")
				}
			}
		}
		if conflict != "" {
			fmt.Fprintf(out, "%s changed in the vault since it was read (another workstation wrote it); wrote so far: %s. Re-reading and showing the new diff.\n", conflict, listOrNone(wrote))
			continue
		}
		fmt.Fprintf(out, "wrote: %s\n", strings.Join(wrote, ", "))
		return errors.Join(postErr, withheld)
	}
	return fmt.Errorf("the vault kept changing under us (%d rounds); nothing more was written — re-run `fleet vault push`", maxPushRounds)
}

// removeNewPubBackups deletes the .pub backup keystore.Write just made (the
// pre-encrypt pair's public half), identified as a .pub.bak-* that did not
// exist before and still holds the same public key.
func removeNewPubBackups(keyPath string, before []string, pub []byte) {
	after, _ := filepath.Glob(keyPath + ".pub.bak-*")
	for _, p := range after {
		if slices.Contains(before, p) {
			continue
		}
		if b, err := os.ReadFile(p); err == nil && string(b) == string(pub) {
			_ = os.Remove(p)
		}
	}
}

// ensureSignedIn is the ONE Ready → SignIn → Ready path (setup and push). in
// must be the command's own stdin (an *os.File in production), never a
// bufio.Reader: it is handed to the vendor's interactive child.
func ensureSignedIn(ctx context.Context, store vault.Store, provider string, in io.Reader, out, errw io.Writer) error {
	err := store.Ready(ctx)
	if errors.Is(err, vault.ErrSignedOut) {
		if serr := store.SignIn(ctx, in, out, errw); serr != nil {
			return fmt.Errorf("sign in to %s: %w", store.Name(), serr)
		}
		err = store.Ready(ctx)
	}
	if errors.Is(err, vault.ErrNotInstalled) {
		return fmt.Errorf("%s: %w", installHint(provider), err)
	}
	if err != nil {
		return fmt.Errorf("%s is not ready: %w", store.Name(), err)
	}
	return nil
}

// installHint names the provider's own CLI package.
func installHint(provider string) string {
	switch provider {
	case "lpass":
		return "lpass is not installed: install lastpass-cli (e.g. `brew install lastpass-cli` or `sudo apt install lastpass-cli`) and re-run"
	case "op":
		return "op is not installed: install 1password-cli (e.g. `brew install 1password-cli`; see https://developer.1password.com/docs/cli/get-started/) and re-run"
	}
	return provider + " is not installed: install its CLI and re-run"
}

func signInHint(provider string) string {
	switch provider {
	case "lpass":
		return "Sign in with `lpass login <your-email>`"
	case "op":
		return "Sign in with `op signin`"
	}
	return "Sign in with `fleet vault setup`"
}

func listOrNone(l []string) string {
	var keep []string
	for _, s := range l {
		if s != "" {
			keep = append(keep, s)
		}
	}
	if len(keep) == 0 {
		return "none"
	}
	return strings.Join(keep, ", ")
}

func withheldError(p vaultPushPlan) error {
	if len(p.Withheld) == 0 {
		return nil
	}
	return errors.New("not everything was pushed (see the NOT pushed lines above)")
}

// getVaultItem reads one item; absent is (zero, false, nil).
func getVaultItem(ctx context.Context, st vault.Store, name string) (vault.Blob, bool, error) {
	b, err := st.Get(ctx, name)
	if errors.Is(err, vault.ErrNotFound) {
		return vault.Blob{}, false, nil
	}
	if err != nil {
		return vault.Blob{}, false, fmt.Errorf("reading %s from the vault: %w", name, err)
	}
	return b, true, nil
}

// planVaultPush reads the four items and decides each one. It writes nothing
// and needs no secret: the key is compared by fingerprint.
func planVaultPush(ctx context.Context, st vault.Store, cfg vault.Config, lk localKey,
	inv vault.Inventory, invErr error, kh vault.KnownHosts, unresolved []unresolvedHost, khErr error, o vaultPushOpts) (vaultPushPlan, error) {
	var p vaultPushPlan
	now := time.Now()

	// key/<workstation>
	keyName := "key/" + cfg.Workstation
	b, found, err := getVaultItem(ctx, st, keyName)
	if err != nil {
		return p, err
	}
	key := pushItem{Name: keyName, Version: b.Version}
	switch {
	case !found:
		key.Changed = true
		key.Diff = []string{keyName + ": new (" + lk.Fingerprint + ")"}
	default:
		cur, derr := vault.Decode[vault.KeyPair](b.Data)
		switch {
		case derr != nil:
			key.Changed = true
			key.Diff = []string{keyName + ": vault copy unreadable → " + lk.Fingerprint}
		case cur.Fingerprint != lk.Fingerprint:
			key.Changed = true
			key.Diff = []string{fmt.Sprintf("%s: fingerprint %s → %s", keyName, cur.Fingerprint, lk.Fingerprint)}
		case o.EncryptLocal && !lk.Encrypted:
			key.Changed = true
			key.Diff = []string{keyName + ": unchanged key, re-stored under a new passphrase so the local file can be encrypted (--encrypt-local)"}
		default:
			key.Diff = []string{keyName + ": unchanged"}
		}
	}
	if key.Changed && !lk.Encrypted {
		if o.EncryptLocal {
			key.Diff = append(key.Diff, "  the local key is not passphrase-protected: it will be encrypted too (backup kept)")
		} else {
			key.Diff = append(key.Diff, "  the local key is not passphrase-protected: the vault copy is encrypted with a generated passphrase; the local file is left as-is (add --encrypt-local to encrypt it too)")
		}
	}
	p.Items = append(p.Items, key)

	// roster
	b, found, err = getVaultItem(ctx, st, "roster")
	if err != nil {
		return p, err
	}
	var r vault.Roster
	if found {
		if r, err = vault.Decode[vault.Roster](b.Data); err != nil {
			return p, fmt.Errorf("the vault roster is unreadable (%v); refusing to overwrite it", err)
		}
	}
	ros := pushItem{Name: "roster", Version: b.Version}
	idx := -1
	for i, e := range r.Entries {
		if e.Fingerprint == lk.Fingerprint {
			idx = i
			break
		}
	}
	switch {
	case idx < 0:
		r.Add(cfg.Workstation, lk.Public, lk.Fingerprint, now)
		ros.Diff = append(ros.Diff, fmt.Sprintf("roster: add %s %s as active", cfg.Workstation, lk.Fingerprint))
		ros.Changed = true
	case r.Entries[idx].State == vault.KeyRetired:
		return p, fmt.Errorf("this key (%s) is retired in the vault roster and will not be re-activated; rotate it with `fleet keys rotate`", lk.Fingerprint)
	case r.Entries[idx].State == vault.KeyPending:
		ros.Diff = append(ros.Diff, fmt.Sprintf("roster: activate %s %s (pending → active)", r.Entries[idx].Workstation, lk.Fingerprint))
		ros.Changed = true
	default:
		ros.Diff = append(ros.Diff, fmt.Sprintf("roster: unchanged (%s active as %s)", lk.Fingerprint, r.Entries[idx].Workstation))
	}
	if ros.Changed {
		for _, e := range r.Activate(lk.Fingerprint, now) {
			ros.Diff = append(ros.Diff, fmt.Sprintf("roster: retire %s %s (replaced by this key)", e.Workstation, e.Fingerprint))
		}
		if ros.Data, err = vault.Encode(r); err != nil {
			return p, err
		}
	}
	p.Items = append(p.Items, ros)

	// inventory: merged into the vault copy through cfgplan, so only the
	// modelled (inert) fields travel and every other directive is named.
	b, found, err = getVaultItem(ctx, st, "inventory")
	if err != nil {
		return p, err
	}
	cur := ""
	if found {
		vi, derr := vault.Decode[vault.Inventory](b.Data)
		if derr != nil {
			return p, fmt.Errorf("the vault inventory is unreadable (%v); refusing to overwrite it", derr)
		}
		cur = vi.Config
	}
	if invErr != nil {
		p.Withheld = append(p.Withheld, fmt.Sprintf("inventory: NOT pushed — local fleet hosts unreadable: %v", invErr))
	} else {
		plan, perr := planInventory(cur, inv)
		if perr != nil {
			p.Withheld = append(p.Withheld, fmt.Sprintf("inventory: NOT pushed — %v", perr))
		} else {
			it := pushItem{Name: "inventory", Version: b.Version, Diff: []string{"inventory:"}}
			it.Diff = append(it.Diff, strings.Split(strings.TrimRight(renderPlan(plan), "\n"), "\n")...)
			it.Diff = append(it.Diff, "  ("+pushNeverRemoves+")")
			if !plan.Empty() {
				next, aerr := plan.Apply(cur)
				if aerr != nil {
					return p, aerr
				}
				if it.Data, err = vault.Encode(vault.Inventory{Config: next}); err != nil {
					return p, err
				}
				it.Changed = true
			}
			p.Items = append(p.Items, it)
		}
	}

	// known_hosts: the local view replaces the vault copy, so a partial view
	// must never be uploaded silently.
	b, found, err = getVaultItem(ctx, st, "known_hosts")
	if err != nil {
		return p, err
	}
	var vk vault.KnownHosts
	if found {
		if vk, err = vault.Decode[vault.KnownHosts](b.Data); err != nil {
			return p, fmt.Errorf("the vault known_hosts is unreadable (%v); refusing to overwrite it", err)
		}
	}
	noneResolved := errors.Is(khErr, errNoAliasResolved) // a fresh machine: unresolved is the whole fleet
	switch {
	case khErr != nil && !noneResolved:
		p.Withheld = append(p.Withheld, fmt.Sprintf("known_hosts: NOT pushed — %v", khErr))
	case len(unresolved) > 0 && !o.AllowPartial:
		w := fmt.Sprintf("known_hosts: NOT pushed — %d fleet host(s) have no usable known_hosts entry, and a partial copy must not replace the vault's:", len(unresolved))
		for _, u := range unresolved {
			w += "\n      " + u.String()
		}
		w += "\n    fix: connect to each host once (or `ssh-keyscan <host> >> ~/.ssh/known_hosts`), then re-run `fleet vault push`" +
			"\n    or:  re-run with --allow-partial to upload only the hosts that resolved"
		p.Withheld = append(p.Withheld, w)
	case noneResolved:
		// --allow-partial with nothing resolved here: the merge keeps
		// every vault line and adds none.
		p.Items = append(p.Items, pushItem{Name: "known_hosts", Version: b.Version, Diff: []string{
			fmt.Sprintf("known_hosts: unchanged — no fleet host resolved here (%d unresolved); --allow-partial keeps all %d vault line(s)", len(unresolved), len(vk.Lines)),
		}})
	default:
		lines := append([]string(nil), kh.Lines...)
		var keptFor, noneFor []string
		kept := 0
		if len(unresolved) > 0 {
			// --allow-partial: the vault's lines for hosts we could not
			// resolve here are the only verified copy; keep them.
			have := map[string]bool{}
			for _, l := range lines {
				have[l] = true
			}
			for _, u := range unresolved {
				n := 0
				for _, l := range vk.Lines {
					if knownHostsLineMatches(l, u.Names) {
						n++
						if !have[l] {
							have[l] = true
							lines = append(lines, l)
							kept++
						}
					}
				}
				if n > 0 {
					keptFor = append(keptFor, u.Alias)
				} else {
					noneFor = append(noneFor, u.Alias)
				}
			}
		}
		sort.Strings(lines)
		add, del := lineSetDiff(lines, vk.Lines)
		it := pushItem{Name: "known_hosts", Version: b.Version}
		if add == 0 && del == 0 {
			it.Diff = []string{"known_hosts: unchanged"}
		} else {
			it.Changed = true
			it.Diff = []string{fmt.Sprintf("known_hosts: +%d -%d lines", add, del)}
			if it.Data, err = vault.Encode(vault.KnownHosts{Lines: lines}); err != nil {
				return p, err
			}
		}
		if len(unresolved) > 0 {
			it.Diff = append(it.Diff, fmt.Sprintf("  partial (--allow-partial): %d host(s) unresolved here: %s", len(unresolved), joinUnresolved(unresolved, "; ")))
			if len(keptFor) > 0 {
				it.Diff = append(it.Diff, fmt.Sprintf("  kept %d existing vault line(s) for: %s", kept, strings.Join(keptFor, ", ")))
			}
			if len(noneFor) > 0 {
				it.Diff = append(it.Diff, fmt.Sprintf("  the vault has no lines for: %s (they stay missing)", strings.Join(noneFor, ", ")))
			}
		}
		p.Items = append(p.Items, it)
	}
	return p, nil
}

// knownHostsLineMatches reports whether a known_hosts line names any of names,
// plain ("host", "[host]:port", comma lists, an @marker prefix) or hashed
// ("|1|salt|hash", re-hashed with the entry's own salt). Patterns and negations
// never match: only concrete names identify a host's own line.
func knownHostsLineMatches(line string, names []string) bool {
	f := strings.Fields(line)
	if len(f) > 0 && strings.HasPrefix(f[0], "@") {
		f = f[1:]
	}
	if len(f) == 0 || strings.HasPrefix(f[0], "#") {
		return false
	}
	norm := make([]string, len(names))
	for i, n := range names {
		norm[i] = knownhosts.Normalize(n)
	}
	for _, h := range strings.Split(f[0], ",") {
		if rest, ok := strings.CutPrefix(h, "|1|"); ok {
			salt64, hash64, ok := strings.Cut(rest, "|")
			salt, err := base64.StdEncoding.DecodeString(salt64)
			if !ok || err != nil {
				continue
			}
			for _, n := range norm {
				m := hmac.New(sha1.New, salt) // #nosec G401 -- OpenSSH known_hosts hash format
				m.Write([]byte(n))
				if base64.StdEncoding.EncodeToString(m.Sum(nil)) == hash64 {
					return true
				}
			}
			continue
		}
		if strings.ContainsAny(h, "*?!") {
			continue
		}
		for _, n := range norm {
			if knownhosts.Normalize(h) == n {
				return true
			}
		}
	}
	return false
}

// lineSetDiff counts the distinct lines only in a (add) and only in b (del);
// add+del is the size of the symmetric difference. push and status share it.
func lineSetDiff(a, b []string) (add, del int) {
	sa, sb := map[string]bool{}, map[string]bool{}
	for _, x := range a {
		sa[x] = true
	}
	for _, x := range b {
		sb[x] = true
	}
	for x := range sa {
		if !sb[x] {
			add++
		}
	}
	for x := range sb {
		if !sa[x] {
			del++
		}
	}
	return add, del
}

// materializeKey builds the KeyPair to store. An encrypted local key is
// stored as-is with the passphrase the operator types (verified first); a plain
// one is encrypted in memory under a generated passphrase. It also returns the
// encrypted bytes (for --encrypt-local). Nothing is written here.
func materializeKey(lk localKey, cfg vault.Config, o vaultPushOpts, stdin io.Reader, in *bufio.Reader, prompt io.Writer) (vault.KeyPair, []byte, error) {
	kp := vault.KeyPair{Public: lk.Public, Fingerprint: lk.Fingerprint, Created: time.Now().UTC()}
	if lk.Encrypted {
		pass, err := readPassphrase(stdin, in, prompt, fmt.Sprintf("passphrase for %s (not echoed): ", lk.Path))
		if err != nil {
			return vault.KeyPair{}, nil, err
		}
		raw, err := ssh.ParseRawPrivateKeyWithPassphrase(lk.Private, pass)
		if err != nil {
			return vault.KeyPair{}, nil, fmt.Errorf("that passphrase does not open %s; nothing was written", lk.Path)
		}
		if err := matchesPub(raw, lk); err != nil {
			return vault.KeyPair{}, nil, err
		}
		kp.Private, kp.Passphrase = append(vault.Secret(nil), lk.Private...), vault.Secret(pass)
		return kp, []byte(kp.Private), nil
	}
	raw, err := ssh.ParseRawPrivateKey(lk.Private)
	if err != nil {
		return vault.KeyPair{}, nil, fmt.Errorf("%s is not a private key ssh can read", lk.Path)
	}
	if err := matchesPub(raw, lk); err != nil {
		return vault.KeyPair{}, nil, err
	}
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		return vault.KeyPair{}, nil, err
	}
	pass := []byte(base64.RawURLEncoding.EncodeToString(seed))
	comment := cfg.Workstation
	if f := strings.Fields(lk.Public); len(f) > 2 {
		comment = strings.Join(f[2:], " ")
	}
	enc, err := keystore.Encrypt(lk.Private, pass, comment)
	if err != nil {
		return vault.KeyPair{}, nil, err
	}
	kp.Private, kp.Passphrase = vault.Secret(enc), vault.Secret(pass)
	return kp, enc, nil
}

// matchesPub refuses a private key whose public half is not the .pub we
// fingerprinted: storing a mismatched pair would restore a key nodes reject.
func matchesPub(raw any, lk localKey) error {
	s, err := ssh.NewSignerFromKey(raw)
	if err != nil {
		return fmt.Errorf("%s: unsupported key type", lk.Path)
	}
	if got := ssh.FingerprintSHA256(s.PublicKey()); got != lk.Fingerprint {
		return fmt.Errorf("%s does not match %s.pub (%s vs %s); fix the pair before pushing", lk.Path, lk.Path, got, lk.Fingerprint)
	}
	return nil
}

// readPassphrase reads without echo from a terminal; from anything else
// (tests, pipes) it reads exactly one line.
func readPassphrase(stdin io.Reader, in *bufio.Reader, prompt io.Writer, msg string) ([]byte, error) {
	fmt.Fprint(prompt, msg)
	if f, ok := stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		b, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(prompt)
		if err != nil {
			return nil, fmt.Errorf("reading the passphrase: %w", err)
		}
		return b, nil
	}
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return nil, errors.New("no passphrase given; nothing was written")
	}
	return []byte(strings.TrimRight(line, "\r\n")), nil
}
