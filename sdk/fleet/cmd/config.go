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

// isLoopbackHostName reports whether a resolved HostName points back at this
// machine. This is not hypothetical: a fleet member's own alias commonly
// resolves to 127.0.0.1 via /etc/hosts, and transferring with yourself is a
// confusing no-op worth refusing before a connection is opened.
func isLoopbackHostName(h string) bool {
	ip := net.ParseIP(strings.TrimSpace(h))
	return ip != nil && ip.IsLoopback()
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
