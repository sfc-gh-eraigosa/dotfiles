package cmd

import (
	"context"
	"os"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/wlink/internal/fleetsrc"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/wlink/internal/probe"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/wlink/internal/resolvconf"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/wlink/internal/winhost"
	"github.com/spf13/cobra"
)

// Environment overrides, mirroring the prototype's so anyone who learned that
// tool is not surprised. They also make the live paths redirectable for
// evidence capture without touching /etc.
const (
	envSSHConfig  = "WLINK_SSH_CONFIG"
	envHostsFile  = "WLINK_HOSTS_FILE"
	envResolvConf = "WLINK_RESOLV_CONF"
	envWslConf    = "WLINK_WSL_CONF"
	envBackupDir  = "WLINK_BACKUP_DIR"
	envProbeHosts = "WLINK_PROBE_HOSTS"
	envSentinel   = "WLINK_PUBLIC_PROBE"
	envFallbacks  = "WLINK_FALLBACKS"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// IsWSL reports whether this is a WSL host.
//
// Everything wlink does is WSL-shaped — Windows interop, /etc/wsl.conf, the WSL
// NAT proxy — so elsewhere it must degrade cleanly rather than pretend.
func IsWSL() bool {
	b, err := os.ReadFile("/proc/version")
	return err == nil && strings.Contains(strings.ToLower(string(b)), "microsoft")
}

// newRuntime assembles the real dependencies.
//
// A missing powershell.exe is not fatal here: Host stays nil-safe via a lister
// that reports the error, so the command declines rather than crashing on a
// machine where Windows is unreachable.
func newRuntime(ctx context.Context) *Runtime {
	rt := &Runtime{
		WSL:            IsWSL(),
		Lookup:         &probe.Resolver{},
		PublicSentinel: envOr(envSentinel, "github.com"),
		Out:            os.Stdout,
		Paths: resolvconf.Paths{
			ResolvConf: envOr(envResolvConf, "/etc/resolv.conf"),
			WslConf:    envOr(envWslConf, "/etc/wsl.conf"),
			BackupDir:  envOr(envBackupDir, "/etc/wlink.backup"),
		},
	}

	if ps, err := winhost.NewPowerShell(); err == nil {
		rt.Host = winhost.New(ps)
	} else {
		rt.Host = errLister{err}
	}

	src := fleetsrc.Source{
		Cmd:       fleetsrc.ExecCmd{},
		SSHConfig: envOr(envSSHConfig, os.ExpandEnv("$HOME/.ssh/config")),
		HostsFile: envOr(envHostsFile, "/etc/hosts"),
	}
	if v := os.Getenv(envFallbacks); v != "" {
		rt.ExtraFallbacks = strings.Fields(strings.ReplaceAll(v, ",", " "))
	}
	if v := os.Getenv(envProbeHosts); v != "" {
		src.Override = strings.Fields(strings.ReplaceAll(v, ",", " "))
	}
	if hosts, err := src.Resolve(ctx); err == nil {
		rt.FleetHosts, rt.ExcludedHosts = hosts.Probe, hosts.Excluded
	}
	return rt
}

type errLister struct{ err error }

func (e errLister) Interfaces(context.Context) ([]winhost.Interface, error) { return nil, e.err }

func newPinCmd() *cobra.Command {
	var dryRun, allowNonRecursive bool
	c := &cobra.Command{
		Use:   "pin",
		Short: "Pin the resolver that knows your fleet",
		Long: "Probes every DNS server Windows knows — not just the default route's, which is\n" +
			"frequently the wrong one — and pins the one that resolves your fleet.\n\n" +
			"Snapshots first: if no undo point can be recorded, nothing is written.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cc *cobra.Command, _ []string) error {
			rt := newRuntime(cc.Context())
			rt.DryRun, rt.AllowNonRecursive = dryRun, allowNonRecursive
			code, err := rt.Pin(cc.Context())
			if err != nil {
				return err
			}
			exitWith(code)
			return nil
		},
	}
	c.Flags().BoolVar(&dryRun, "dry-run", false, "report the decision and write nothing")
	c.Flags().BoolVar(&allowNonRecursive, "allow-nonrecursive", false,
		"pin a resolver that cannot answer public names (this WILL break public DNS)")
	return c
}

func newUnpinCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "unpin",
		Short:         "Restore the files wlink replaced",
		Long:          "Restores the pre-pin resolv.conf and wsl.conf from the snapshot, symlink target included.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cc *cobra.Command, _ []string) error {
			rt := newRuntime(cc.Context())
			code, err := rt.Unpin(cc.Context())
			if err != nil {
				return err
			}
			exitWith(code)
			return nil
		},
	}
}

// exitWith centralises the exit-code contract (spec §3): 0 success or a safe
// decline, 1 a real failure, 2 a usage error. Zero returns normally so tests
// and `--help` are unaffected.
var exitWith = func(code int) {
	if code != 0 {
		os.Exit(code)
	}
}
