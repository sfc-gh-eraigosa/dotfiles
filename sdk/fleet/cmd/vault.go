package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/agent"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/featflag"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/hostkey"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/keystore"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault/providers"
)

// Test seams: the real implementations talk to a password manager, gff and
// ssh-agent; tests swap them.
var (
	openVault = func(cfg vault.Config) (vault.Store, error) {
		return providers.Open(cfg, providers.SystemExec)
	}
	vaultSettings = func() featflag.Settings {
		return featflag.Resolve(&featflag.GFF{Repo: flagRepo}, "", flagRepo)
	}
	agentLoaded = func(ctx context.Context, fp string) (bool, error) {
		return agent.Loaded(ctx, agent.SystemExec, fp)
	}
	// fleetInventory and fleetKnownHosts gather the local half of the
	// vault's inventory and known_hosts items (status compares; push uploads).
	fleetInventory  = inventoryFromConfig
	fleetKnownHosts = func() (vault.KnownHosts, error) {
		hosts, err := fleetHosts()
		if err != nil {
			return vault.KnownHosts{}, err
		}
		aliases := make([]string, 0, len(hosts))
		for _, h := range hosts {
			aliases = append(aliases, h.Alias)
		}
		return knownHostsFor(hostkey.Run, aliases)
	}
)

var vaultCmd = newVaultCmd()

func init() { rootCmd.AddCommand(vaultCmd) }

func newVaultCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "vault",
		Short: "Keep this workstation's SSH key in a password manager (gated by gff fleet.vault.enabled)",
	}
	c.AddCommand(newVaultSetupCmd(), newVaultStatusCmd())
	return c
}

// vaultGate refuses every vault verb unless the gff flag is on. It runs before
// any store is opened.
func vaultGate() error {
	if !vaultSettings().VaultEnabled {
		return errors.New("fleet vault is off: enable it with `gff set fleet.vault.enabled true`")
	}
	return nil
}

// gated is the only way a vault RunE is built: the gff gate runs before the
// verb, so a subcommand cannot forget it (TestEveryVaultSubcommandIsGated).
func gated(run func(*cobra.Command) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		if err := vaultGate(); err != nil {
			return err
		}
		return run(cmd)
	}
}

func newVaultSetupCmd() *cobra.Command {
	var provider, vaultName, workstation, key string
	c := &cobra.Command{
		Use:   "setup",
		Short: "Choose a password manager, check it is usable, and save vault.yaml",
		Args:  cobra.NoArgs,
		RunE: gated(func(cmd *cobra.Command) error {
			in, out := bufio.NewReader(cmd.InOrStdin()), cmd.OutOrStdout()
			if provider == "" {
				fmt.Fprint(out, "Password manager [lpass/op]: ")
				line, _ := in.ReadString('\n')
				provider = strings.TrimSpace(line)
			}
			cfg, err := applyVaultDefaults(vault.Config{Provider: provider, Vault: vaultName, Workstation: workstation, Key: key})
			if err != nil {
				return err
			}
			store, err := openVault(cfg)
			if errors.Is(err, vault.ErrNotInstalled) && cfg.Provider == "op" {
				return errors.New("1Password support is not in this build yet; use lpass")
			}
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			err = store.Ready(ctx)
			if errors.Is(err, vault.ErrSignedOut) {
				if serr := store.SignIn(ctx, in, out, cmd.ErrOrStderr()); serr != nil {
					return fmt.Errorf("sign in to %s: %w", store.Name(), serr)
				}
				err = store.Ready(ctx)
			}
			if errors.Is(err, vault.ErrNotInstalled) {
				fmt.Fprintf(out, "%s is not installed: install its CLI (e.g. `brew install lastpass-cli`) and re-run fleet vault setup\n", cfg.Provider)
				return err
			}
			if err != nil {
				return fmt.Errorf("%s is not ready: %w", store.Name(), err)
			}
			if err := saveVaultConfig(vaultConfigPath(), cfg); err != nil {
				return err
			}
			fmt.Fprintf(out, "saved %s (provider %s, vault %s, workstation %s)\nnext: fleet vault push\n", vaultConfigPath(), cfg.Provider, cfg.Vault, cfg.Workstation)
			return nil
		}),
	}
	c.Flags().StringVar(&provider, "provider", "", "password manager: lpass or op (skips the prompt)")
	c.Flags().StringVar(&vaultName, "vault", "", "vault name (op) or folder (lpass)")
	c.Flags().StringVar(&workstation, "workstation", "", "this workstation's name (default: short hostname)")
	c.Flags().StringVar(&key, "key", "", "private key path (default ~/.ssh/id_ed25519)")
	return c
}

func newVaultStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show provider, key, inventory and known_hosts state against the vault",
		Args:  cobra.NoArgs,
		RunE: gated(func(cmd *cobra.Command) error {
			out := cmd.OutOrStdout()
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
			vaultStatus(ctx, out, cfg)
			return nil
		}),
	}
}

// vaultStatus prints one line per facet. A signed-out provider is reported and
// the local-only facets still print; status never signs in.
func vaultStatus(ctx context.Context, out io.Writer, cfg vault.Config) {
	var store vault.Store
	live := false
	st, err := openVault(cfg)
	switch {
	case err != nil:
		fmt.Fprintf(out, "provider: %s unavailable (%v)\n", cfg.Provider, err)
	default:
		store = st
		switch rerr := st.Ready(ctx); {
		case rerr == nil:
			live = true
			fmt.Fprintf(out, "provider: %s signed in\n", cfg.Provider)
		case errors.Is(rerr, vault.ErrSignedOut):
			fmt.Fprintf(out, "provider: %s signed out\n", cfg.Provider)
		case errors.Is(rerr, vault.ErrNotInstalled):
			fmt.Fprintf(out, "provider: %s not installed\n", cfg.Provider)
		default:
			fmt.Fprintf(out, "provider: %s not ready (%v)\n", cfg.Provider, rerr)
		}
	}

	// key
	fp := ""
	if _, err := os.Stat(cfg.Key); err != nil {
		fmt.Fprintf(out, "key: no local key file at %s\n", cfg.Key)
	} else {
		enc, eerr := keystore.Encrypted(cfg.Key)
		encStr := "encrypted"
		if eerr != nil {
			encStr = "unreadable"
		} else if !enc {
			encStr = "NOT encrypted"
		}
		fpStr := "no public key"
		if pub, perr := os.ReadFile(cfg.Key + ".pub"); perr == nil {
			if f, ferr := keystore.Fingerprint(pub); ferr == nil {
				fp, fpStr = f, f
			}
		}
		fmt.Fprintf(out, "key: local file exists, %s, %s\n", encStr, fpStr)
	}
	var names []string
	if live {
		names, err = store.List(ctx)
		if err != nil {
			fmt.Fprintf(out, "key: listing the vault failed: %v\n", err)
			live = false
		}
	}
	if live {
		own := "key/" + cfg.Workstation
		if contains(names, own) {
			fmt.Fprintf(out, "key: vault has %s\n", own)
		} else {
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
			fmt.Fprintf(out, "key: no vault key for %s; existing: %s\n", cfg.Workstation, ex)
		}
		fmt.Fprintf(out, "key: %s\n", rosterFacet(ctx, store, names, fp))
	}
	if fp != "" {
		switch loaded, aerr := agentLoaded(ctx, fp); {
		case errors.Is(aerr, agent.ErrNoAgent):
			fmt.Fprintln(out, "key: no agent running")
		case aerr != nil:
			fmt.Fprintf(out, "key: agent check failed: %v\n", aerr)
		case loaded:
			fmt.Fprintln(out, "key: loaded in agent")
		default:
			fmt.Fprintln(out, "key: not loaded in agent")
		}
	}

	// inventory and known_hosts
	if !live {
		return
	}
	var vi vault.Inventory
	hasInv := false
	if contains(names, "inventory") {
		if b, gerr := store.Get(ctx, "inventory"); gerr == nil {
			if vi, gerr = vault.Decode[vault.Inventory](b.Data); gerr == nil {
				hasInv = true
			}
		}
	}
	li, lerr := fleetInventory()
	switch {
	case lerr != nil:
		fmt.Fprintf(out, "inventory: local fleet hosts unreadable: %v\n", lerr)
	case !hasInv:
		fmt.Fprintln(out, "inventory: not in vault")
	default:
		fmt.Fprintf(out, "inventory: in vault, %d hosts differ\n", inventoryDrift(li, vi))
	}
	var vk vault.KnownHosts
	hasKH := false
	if contains(names, "known_hosts") {
		if b, gerr := store.Get(ctx, "known_hosts"); gerr == nil {
			if vk, gerr = vault.Decode[vault.KnownHosts](b.Data); gerr == nil {
				hasKH = true
			}
		}
	}
	lk, kerr := fleetKnownHosts()
	switch {
	case kerr != nil:
		fmt.Fprintf(out, "known_hosts: local fleet lines unreadable: %v\n", kerr)
	case !hasKH:
		fmt.Fprintln(out, "known_hosts: not in vault")
	default:
		fmt.Fprintf(out, "known_hosts: in vault, %d lines differ\n", linesDrift(lk.Lines, vk.Lines))
	}
}

func rosterFacet(ctx context.Context, store vault.Store, names []string, fp string) string {
	if !contains(names, "roster") {
		return "roster: not in vault"
	}
	b, err := store.Get(ctx, "roster")
	if err != nil {
		return fmt.Sprintf("roster: unreadable (%v)", err)
	}
	r, err := vault.Decode[vault.Roster](b.Data)
	if err != nil {
		return fmt.Sprintf("roster: unreadable (%v)", err)
	}
	if fp == "" {
		return "roster: no local fingerprint to look up"
	}
	for _, e := range r.Entries {
		if e.Fingerprint == fp {
			return fmt.Sprintf("roster: %s (%s)", e.State, e.Workstation)
		}
	}
	return "roster: this key is not in the roster"
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// inventoryFromConfig is the vault Inventory for this machine: the text of
// every #fleet Host block in the ssh config, found the way `fleetHosts` finds
// them (so the marker, --config and selection rules are the same).
func inventoryFromConfig() (vault.Inventory, error) {
	hosts, err := fleetHosts()
	if err != nil {
		return vault.Inventory{}, err
	}
	raw, err := readConfig(flagConfig)
	if err != nil {
		return vault.Inventory{}, err
	}
	want := map[string]bool{}
	for _, h := range hosts {
		want[h.Alias] = true
	}
	blocks := hostBlocks(raw)
	var parts []string
	for _, b := range blocks {
		if want[b.alias] {
			parts = append(parts, b.text)
		}
	}
	text := strings.Join(parts, "\n\n")
	if text != "" {
		text += "\n"
	}
	return vault.Inventory{Config: text}, nil
}

type hostBlock struct{ alias, text string }

// hostBlocks splits ssh_config text into its single-alias Host blocks.
func hostBlocks(cfg string) []hostBlock {
	var out []hostBlock
	var cur []string
	alias := ""
	flush := func() {
		if alias != "" {
			out = append(out, hostBlock{alias, strings.TrimRight(strings.Join(cur, "\n"), " \t\n")})
		}
		cur, alias = nil, ""
	}
	for _, l := range strings.Split(cfg, "\n") {
		code, _, _ := strings.Cut(l, "#") // the marker rides on the Host line as a comment
		f := strings.Fields(code)
		if len(f) > 0 && (strings.EqualFold(f[0], "host") || strings.EqualFold(f[0], "match")) {
			flush()
			if strings.EqualFold(f[0], "host") && len(f) == 2 {
				alias = f[1]
			}
		}
		if alias != "" {
			cur = append(cur, l)
		}
	}
	flush()
	return out
}

// inventoryDrift counts aliases present in only one side or whose block text
// differs.
func inventoryDrift(local, remote vault.Inventory) int {
	l, r := map[string]string{}, map[string]string{}
	for _, b := range hostBlocks(local.Config) {
		l[b.alias] = b.text
	}
	for _, b := range hostBlocks(remote.Config) {
		r[b.alias] = b.text
	}
	n := 0
	for a, t := range l {
		if rt, ok := r[a]; !ok || rt != t {
			n++
		}
	}
	for a := range r {
		if _, ok := l[a]; !ok {
			n++
		}
	}
	return n
}

// linesDrift is the size of the symmetric difference of two line sets.
func linesDrift(a, b []string) int {
	sa, sb := map[string]bool{}, map[string]bool{}
	for _, x := range a {
		sa[x] = true
	}
	for _, x := range b {
		sb[x] = true
	}
	n := 0
	for x := range sa {
		if !sb[x] {
			n++
		}
	}
	for x := range sb {
		if !sa[x] {
			n++
		}
	}
	return n
}

// knownHostsFor collects the known_hosts lines ssh would consult for each
// alias, resolving names and files through hostkey.Resolve (ssh -G) the way
// `fleet trust` does. ssh-keygen exit 1 means "no entry" and is fine; any
// other failure (a missing binary, a bad file) is an error, as is every alias
// failing to resolve - an empty result must never mean "the tools broke". A
// partial resolve failure still returns the lines found.
func knownHostsFor(x hostkey.Exec, aliases []string) (vault.KnownHosts, error) {
	seen := map[string]bool{}
	var lines, failed []string
	for _, a := range aliases {
		t, err := hostkey.Resolve(x, a)
		if err != nil {
			failed = append(failed, err.Error())
			continue
		}
		for _, file := range t.KnownHosts {
			if _, err := os.Stat(file); err != nil {
				continue
			}
			for _, n := range t.Names() {
				out, err := x("ssh-keygen", "", "-F", n, "-f", file)
				var ec interface{ ExitCode() int }
				if err != nil && !(errors.As(err, &ec) && ec.ExitCode() == 1) {
					return vault.KnownHosts{}, fmt.Errorf("ssh-keygen -F %s: %w", n, err)
				}
				for l := range strings.SplitSeq(out, "\n") {
					if l = strings.TrimSpace(l); l == "" || strings.HasPrefix(l, "#") || seen[l] {
						continue
					}
					seen[l] = true
					lines = append(lines, l)
				}
			}
		}
	}
	if len(aliases) > 0 && len(failed) == len(aliases) {
		return vault.KnownHosts{}, fmt.Errorf("no fleet alias resolved (first: %s)", failed[0])
	}
	sort.Strings(lines)
	return vault.KnownHosts{Lines: lines}, nil
}
