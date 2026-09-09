package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/wlink/internal/resolvconf"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/wlink/internal/sshcfg"
	"github.com/spf13/cobra"
)

// Finding is one thing worth telling the user about.
//
// Every finding carries a one-line Detail and, where one exists, a Fix. A
// finding a reader cannot act on is noise, and noise is what makes people stop
// reading diagnostics.
type Finding struct {
	OK     bool
	Title  string
	Detail string
	Fix    string
}

// Doctor reports conditions that predict outages rather than describe current
// ones — the things that will bite later if left alone.
func (r *Runtime) Doctor(ctx context.Context, fix bool) (int, error) {
	if !r.WSL {
		r.sayf("not running under WSL; nothing to check.")
		return 0, nil
	}

	findings := []Finding{
		r.checkKeepalive(ctx, fix),
		r.checkSnapshot(),
		r.checkDrift(),
		r.checkFleetSource(),
	}

	problems := 0
	for _, f := range findings {
		if f.OK {
			fmt.Fprintf(r.Out, "[ok]   %s\n", f.Detail)
			continue
		}
		problems++
		fmt.Fprintf(r.Out, "[warn] %s\n", f.Title)
		fmt.Fprintf(r.Out, "       %s\n", f.Detail)
		if f.Fix != "" {
			fmt.Fprintf(r.Out, "       fix: %s\n", f.Fix)
		}
	}
	fmt.Fprintf(r.Out, "%d checks passed, %d finding(s)\n", len(findings)-problems, problems)

	if problems > 0 {
		return 1, nil
	}
	return 0, nil
}

// checkKeepalive is the one that came from a real incident: git hanging
// indefinitely over a tunnel.
func (r *Runtime) checkKeepalive(ctx context.Context, fix bool) Finding {
	host := r.GitHost
	if host == "" {
		host = "github.com"
	}
	cmd := r.SSHCmd
	if cmd == nil {
		cmd = sshcfg.ExecCmd{}
	}

	s, err := sshcfg.Effective(ctx, cmd, "ssh", host)
	if err != nil {
		return Finding{
			OK:     true, // cannot check is not the same as broken
			Detail: fmt.Sprintf("ssh keepalive for %s: could not ask ssh (%v); skipping", host, err),
		}
	}
	if !s.KeepaliveDisabled() {
		return Finding{OK: true, Detail: fmt.Sprintf("ssh keepalive for %s is set (ServerAliveInterval %d)", host, s.ServerAliveInterval)}
	}

	f := Finding{
		Title: fmt.Sprintf("ssh has no keepalive for %s (ServerAliveInterval %d, ConnectTimeout %s)", host, s.ServerAliveInterval, s.ConnectTimeout),
		Detail: "over a tunnel a connection can stall with TCP established and no bytes moving; " +
			"with keepalives off neither end notices, so git hangs forever instead of failing",
		Fix: fmt.Sprintf("wlink doctor --fix   (adds ServerAliveInterval %d / ServerAliveCountMax %d)",
			sshcfg.KeepaliveInterval, sshcfg.KeepaliveCountMax),
	}
	if !fix {
		return f
	}

	changed, err := sshcfg.ApplyKeepalive(r.SSHConfigPath, host)
	if err != nil {
		f.Detail = fmt.Sprintf("could not apply the keepalive: %v", err)
		return f
	}
	// Re-ask ssh rather than assuming: ssh takes the FIRST value it sees for a
	// keyword, so an earlier Host block that already sets the interval would
	// still win and our append would be inert. Claiming success there would be
	// worse than reporting nothing.
	if after, aerr := sshcfg.Effective(ctx, cmd, "ssh", host); aerr == nil && !after.KeepaliveDisabled() {
		return Finding{OK: true, Detail: fmt.Sprintf("ssh keepalive for %s applied (ServerAliveInterval %d)", host, after.ServerAliveInterval)}
	}
	if changed {
		f.Detail = "a keepalive block was appended, but ssh still reports it disabled — " +
			"an earlier Host block in your config sets ServerAliveInterval and wins (ssh takes the first value)"
		f.Fix = "move the wlink block above that one, or set ServerAliveInterval there"
	}
	return f
}

func (r *Runtime) checkSnapshot() Finding {
	content := readFileOrEmpty(r.Paths.ResolvConf)
	if !resolvconf.IsManaged(content) {
		return Finding{OK: true, Detail: "resolv.conf is not managed by wlink; nothing to undo"}
	}
	if resolvconf.HasSnapshot(r.Paths) {
		return Finding{OK: true, Detail: fmt.Sprintf("snapshot present at %s — unpin will restore", r.Paths.BackupDir)}
	}
	return Finding{
		Title:  "resolv.conf is managed by wlink but there is no snapshot",
		Detail: "unpin can still repair the stock layout, but it cannot restore exactly what was there before",
		Fix:    "wlink unpin, then wlink pin to take a fresh snapshot",
	}
}

func (r *Runtime) checkDrift() Finding {
	d, err := resolvconf.DetectDrift(r.Paths)
	switch {
	case err != nil:
		return Finding{OK: true, Detail: fmt.Sprintf("drift check skipped (%v)", err)}
	case d == nil:
		return Finding{OK: true, Detail: "resolv.conf matches what wlink wrote (no drift)"}
	}
	return Finding{
		Title:  fmt.Sprintf("%s changed since wlink wrote it", d.File),
		Detail: d.Detail,
		Fix:    "wlink pin to re-apply, or wlink unpin to hand the file back",
	}
}

func (r *Runtime) checkFleetSource() Finding {
	switch n := len(r.FleetHosts); {
	case n > 0:
		return Finding{OK: true, Detail: fmt.Sprintf("%d fleet name(s) to check", n)}
	case len(r.ExcludedHosts) > 0:
		return Finding{OK: true, Detail: "every fleet name is served by /etc/hosts; no resolver is consulted for them"}
	}
	return Finding{
		Title:  "no fleet names found",
		Detail: "wlink has nothing to probe, so it cannot tell whether name resolution works",
		Fix:    "adopt hosts with `fleet add <host>`, or set WLINK_PROBE_HOSTS",
	}
}

func newDoctorCmd() *cobra.Command {
	var fix bool
	c := &cobra.Command{
		Use:   "doctor",
		Short: "Report conditions that predict outages",
		Long: "Checks the things that bite later: ssh keepalives (a stall over a tunnel hangs\n" +
			"git forever without them), a missing undo snapshot, resolv.conf drift, and\n" +
			"whether there are any fleet names to check at all.\n\n" +
			"Exit code: 0 no findings, 1 findings present.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cc *cobra.Command, _ []string) error {
			rt := newRuntime(cc.Context())
			if rt.SSHConfigPath == "" {
				rt.SSHConfigPath = os.ExpandEnv("$HOME/.ssh/config")
			}
			code, err := rt.Doctor(cc.Context(), fix)
			if err != nil {
				return err
			}
			exitWith(code)
			return nil
		},
	}
	c.Flags().BoolVar(&fix, "fix", false, "apply the fixes that can be applied safely")
	return c
}
