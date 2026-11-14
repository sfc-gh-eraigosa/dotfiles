package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/agent"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/cfgplan"
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
	fleetKnownHosts = func() (vault.KnownHosts, []unresolvedHost, error) {
		hosts, err := fleetHosts()
		if err != nil {
			return vault.KnownHosts{}, nil, err
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
	c.AddCommand(newVaultSetupCmd(), newVaultStatusCmd(), newVaultPushCmd())
	c.AddCommand(newVaultPullCmd())
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
			// stdin stays the command's own reader (the real *os.File in
			// production): the vendor sign-in child must get that, never a
			// bufio.Reader. The prompt reads one line only, and only when
			// there is no --provider.
			stdin, out := cmd.InOrStdin(), cmd.OutOrStdout()
			if provider == "" {
				fmt.Fprint(out, "Password manager [lpass/op]: ")
				line, _ := bufio.NewReader(stdin).ReadString('\n')
				provider = strings.TrimSpace(line)
			}
			cfg, err := applyVaultDefaults(vault.Config{Provider: provider, Vault: vaultName, Workstation: workstation, Key: key})
			if err != nil {
				return err
			}
			if cfg.Provider == "op" {
				fmt.Fprintln(out, "note: 1Password support is unconfirmed live (no account tested)")
			}
			store, err := openVault(cfg)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			if err := ensureSignedIn(ctx, store, cfg.Provider, stdin, out, cmd.ErrOrStderr()); err != nil {
				if errors.Is(err, vault.ErrNotInstalled) {
					fmt.Fprintln(out, err.Error())
				}
				return err
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
			if !vaultStatus(ctx, out, cfg) {
				return exitError{code: 1}
			}
			return nil
		}),
	}
}

// vaultStatus prints one line per facet. A signed-out provider is reported and
// the local-only facets still print; status never signs in. It returns false
// when a vault item exists but cannot be read (status then exits non-zero).
func vaultStatus(ctx context.Context, out io.Writer, cfg vault.Config) (ok bool) {
	ok = true
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
		if slices.Contains(names, own) {
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
		return ok
	}
	vi, hasInv, ierr := readVaultItem[vault.Inventory](ctx, store, names, "inventory")
	li, lerr := fleetInventory()
	switch {
	case ierr != nil:
		ok = false
		fmt.Fprintf(out, "inventory: unreadable: %v\n", ierr)
	case lerr != nil:
		fmt.Fprintf(out, "inventory: local fleet hosts unreadable: %v\n", lerr)
	case !hasInv:
		fmt.Fprintln(out, "inventory: not in vault")
	default:
		// The same planner push uses, so "status: in sync" ⇔ "push: unchanged".
		if p, perr := planInventory(vi.Config, li); perr != nil {
			fmt.Fprintf(out, "inventory: cannot compare: %v\n", perr)
		} else if n := hostsToPush(p); n == 0 {
			fmt.Fprintln(out, "inventory: in vault, in sync")
		} else {
			fmt.Fprintf(out, "inventory: in vault, %d host(s) to push\n", n)
		}
	}
	vk, hasKH, kherr := readVaultItem[vault.KnownHosts](ctx, store, names, "known_hosts")
	lk, unresolved, kerr := fleetKnownHosts()
	defer func() {
		if kerr == nil && len(unresolved) > 0 {
			fmt.Fprintf(out, "known_hosts: %d fleet host(s) unresolved: %s\n", len(unresolved), joinUnresolved(unresolved, ", "))
		}
	}()
	switch {
	case kherr != nil:
		ok = false
		fmt.Fprintf(out, "known_hosts: unreadable: %v\n", kherr)
	case kerr != nil:
		fmt.Fprintf(out, "known_hosts: local fleet lines unreadable: %v\n", kerr)
	case !hasKH:
		fmt.Fprintln(out, "known_hosts: not in vault")
	default:
		add, del := lineSetDiff(lk.Lines, vk.Lines)
		fmt.Fprintf(out, "known_hosts: in vault, %d lines differ\n", add+del)
	}
	return ok
}

// readVaultItem reads and decodes one item for status. Absent (not listed, or
// ErrNotFound) is (zero, false, nil); any other failure is an error, so an
// item that exists but cannot be read is never reported as "not in vault".
func readVaultItem[T vault.KeyPair | vault.Roster | vault.Inventory | vault.KnownHosts](ctx context.Context, st vault.Store, names []string, name string) (T, bool, error) {
	var zero T
	if !slices.Contains(names, name) {
		return zero, false, nil
	}
	b, err := st.Get(ctx, name)
	if errors.Is(err, vault.ErrNotFound) {
		return zero, false, nil
	}
	if err != nil {
		return zero, false, err
	}
	v, err := vault.Decode[T](b.Data)
	if err != nil {
		return zero, false, err
	}
	return v, true, nil
}

// planInventory is the ONE inventory comparison: push writes its result and
// status reports it. Only cfgplan's modelled fields count.
func planInventory(vaultText string, local vault.Inventory) (cfgplan.Plan, error) {
	return cfgplan.Build(vaultText, local.Config, cfgplan.Opts{Marker: flagMarker, All: true})
}

// hostsToPush counts the hosts a push would add or update.
func hostsToPush(p cfgplan.Plan) int {
	n := 0
	for _, c := range p.Changes {
		if c.Kind == cfgplan.Add || c.Kind == cfgplan.Update {
			n++
		}
	}
	return n
}

func rosterFacet(ctx context.Context, store vault.Store, names []string, fp string) string {
	if !slices.Contains(names, "roster") {
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

// errNoAliasResolved: knownHostsFor resolved none of the fleet aliases. The
// unresolved list is returned with it; a real ssh-keygen failure is a
// different error.
var errNoAliasResolved = errors.New("no fleet alias resolved")

// unresolvedHost is a fleet alias whose known_hosts lines are missing from a
// knownHostsFor result. Names are the known_hosts lookup names it is known by
// (the alias, plus ssh -G's HostName/IPs when that resolved), so a caller can
// find the alias's lines elsewhere (push keeps the vault's under
// --allow-partial). Reason never carries key material.
type unresolvedHost struct {
	Alias, Reason string
	Names         []string
}

func (u unresolvedHost) String() string { return u.Alias + ": " + u.Reason }

func joinUnresolved(us []unresolvedHost, sep string) string {
	s := make([]string, len(us))
	for i, u := range us {
		s[i] = u.String()
	}
	return strings.Join(s, sep)
}

// knownHostsFor collects the known_hosts lines ssh would consult for each
// alias, resolving names and files through hostkey.Resolve (ssh -G) the way
// `fleet trust` does.
//
// Contract (relied on by `fleet vault push`): the returned KnownHosts is
// COMPLETE only when the unresolved list is empty. Each unresolved entry names
// an alias whose lines are missing from the result (ssh -G
// failed, one of its known_hosts files could not be read, or no known_hosts
// entry exists for any of its names: "alias: no known_hosts entry"); the reason never
// carries key material. A caller that uploads the result as the vault copy
// must refuse while the list is non-empty unless the user opts in
// (--allow-partial), or the vault silently loses those hosts. Errors: any
// ssh-keygen failure other than exit 1 (= no entry, fine), and every alias
// being unresolved (errNoAliasResolved, returned WITH the unresolved list).
func knownHostsFor(x hostkey.Exec, aliases []string) (vault.KnownHosts, []unresolvedHost, error) {
	seen := map[string]bool{}
	var lines []string
	var unresolved []unresolvedHost
	for _, a := range aliases {
		t, err := hostkey.Resolve(x, a)
		if err != nil {
			unresolved = append(unresolved, unresolvedHost{Alias: a, Reason: err.Error(), Names: []string{a}})
			continue
		}
		bad, found := "", false
		for _, file := range t.KnownHosts {
			if _, err := os.Stat(file); err != nil {
				if !errors.Is(err, fs.ErrNotExist) && bad == "" {
					bad = file + ": " + err.Error()
				}
				continue // a missing file holds no keys
			}
			for _, n := range t.Names() {
				out, err := x("ssh-keygen", "", "-F", n, "-f", file)
				var ec interface{ ExitCode() int }
				if err != nil && (!errors.As(err, &ec) || ec.ExitCode() != 1) {
					return vault.KnownHosts{}, nil, fmt.Errorf("ssh-keygen -F %s: %w", n, err)
				}
				for l := range strings.SplitSeq(out, "\n") {
					if l = strings.TrimSpace(l); l == "" || strings.HasPrefix(l, "#") {
						continue
					}
					found = true
					if !seen[l] {
						seen[l] = true
						lines = append(lines, l)
					}
				}
			}
		}
		names := append([]string{a}, t.Names()...)
		switch {
		case bad != "":
			unresolved = append(unresolved, unresolvedHost{Alias: a, Reason: bad, Names: names})
		case !found:
			unresolved = append(unresolved, unresolvedHost{Alias: a, Reason: "no known_hosts entry", Names: names})
		}
	}
	if len(aliases) > 0 && len(unresolved) == len(aliases) {
		// The list rides along so push --allow-partial can still keep the
		// vault's lines on a fresh machine where nothing resolves yet.
		return vault.KnownHosts{}, unresolved, fmt.Errorf("%w (first: %s)", errNoAliasResolved, unresolved[0])
	}
	sort.Strings(lines)
	return vault.KnownHosts{Lines: lines}, unresolved, nil
}
