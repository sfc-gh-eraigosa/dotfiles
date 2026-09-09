package cmd

import (
	"fmt"
	"net"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/cfgplan"
	"github.com/spf13/cobra"
)

// configCmd groups the ssh-config transfer verbs.
//
// Each verb is STRICTLY ONE-WAY and names its direction at the call site. There
// is deliberately no `sync`: a combined operation would resolve conflicts by
// policy instead of by an operator reading a diff, and would make one mistake's
// blast radius the union of both directions.
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Transfer ssh-config host entries between this machine and one fleet host",
}

// isLoopbackHost reports whether a source points back at this machine, and is
// the guard the pull path actually uses.
//
// Found by live testing rather than review: `ssh -G` reports the CONFIGURED
// hostname, which is normally a NAME, so a guard that only tried ParseIP never
// fired for the very case it exists for — an alias resolving to 127.0.0.1 via
// /etc/hosts. Names must therefore be resolved before deciding.
//
// A resolution failure is NOT loopback: a pull must not be blocked because DNS
// is unavailable. `resolve` is injected so this stays testable without touching
// the network.
func isLoopbackHost(h string, resolve func(string) ([]net.IP, error)) bool {
	h = strings.TrimSpace(h)
	if h == "" {
		return false
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback()
	}
	ips, err := resolve(h)
	if err != nil || len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
		if !ip.IsLoopback() {
			return false
		}
	}
	return true
}

// renderPlan is the operator's only safety, so it names every change and
// everything that was withheld.
func renderPlan(p cfgplan.Plan) string {
	var b strings.Builder
	var adds, updates int
	for _, c := range p.Changes {
		switch c.Kind {
		case cfgplan.Add:
			adds++
			fmt.Fprintf(&b, "  + %s\n", c.Alias)
			for _, f := range fieldsOf(c) {
				fmt.Fprintf(&b, "      %s %s\n", f.Name, f.To)
			}
		case cfgplan.Update:
			updates++
			fmt.Fprintf(&b, "  ~ %s\n", c.Alias)
			for _, f := range c.Fields {
				fmt.Fprintf(&b, "      %s %s → %s\n", f.Name, f.From, f.To)
			}
		}
	}
	if adds == 0 && updates == 0 {
		b.WriteString("  (nothing to change — already current)\n")
	}
	if len(p.NotImported) > 0 {
		// Withheld and NAMED. Exec-safety here is structural, which makes the
		// exclusion invisible unless it is reported.
		fmt.Fprintf(&b, "\n  not imported: %s\n", strings.Join(p.NotImported, ", "))
	}
	if p.Includes > 0 {
		fmt.Fprintf(&b, "  source config has %d Include directive(s); those files were not read\n", p.Includes)
	}
	return b.String()
}

// fieldsOf renders an added host's modelled directives, which have no "from"
// side to show.
func fieldsOf(c cfgplan.Change) []cfgplan.FieldDelta {
	var out []cfgplan.FieldDelta
	for _, kv := range []struct{ n, v string }{
		{"HostName", c.Host.HostName}, {"User", c.Host.User},
		{"Port", c.Host.Port}, {"IdentityFile", c.Host.Identity},
	} {
		if kv.v != "" {
			out = append(out, cfgplan.FieldDelta{Name: kv.n, To: kv.v})
		}
	}
	return out
}

func init() {
	rootCmd.AddCommand(configCmd)
}
