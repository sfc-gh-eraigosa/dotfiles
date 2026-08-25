// Package fleetsrc answers "which names should wlink probe?".
//
// sdk/fleet already OWNS the `#fleet` marker — fleet add/discover/remove manage
// those ssh-config blocks — so wlink consumes `fleet discover --json` rather
// than maintaining a second parser that would drift from it. The ssh-config
// scan here is a read-only fallback for machines without fleet installed;
// wlink never writes those blocks.
package fleetsrc

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"strings"
)

// Origin records where the probe list came from, so `status` can say why it is
// probing what it is probing.
type Origin string

const (
	OriginFleet     Origin = "fleet"
	OriginSSHConfig Origin = "ssh-config"
	OriginOverride  Origin = "override"
)

// FleetMarker is the comment sdk/fleet writes on adopted Host blocks.
const FleetMarker = "#fleet"

// Cmd runs an external command; swapped in tests so `fleet` need not exist.
type Cmd interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecCmd is the real implementation.
type ExecCmd struct{}

func (ExecCmd) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// Source describes where to look.
type Source struct {
	Cmd       Cmd
	FleetBin  string // defaults to "fleet"
	SSHConfig string
	HostsFile string
	// Override skips discovery entirely — the escape hatch for a fleet that is
	// not represented in ssh config at all.
	Override []string
}

// Hosts is the resolved probe set.
type Hosts struct {
	// Probe are names worth asking a resolver about.
	Probe []string
	// Excluded are fleet names deliberately NOT probed, with the reason implied
	// by their being here: served by /etc/hosts, or already an address. They are
	// reported rather than silently dropped so a capped score is never a mystery.
	Excluded []string
	Origin   Origin
}

type discoverRow struct {
	Alias    string `json:"alias"`
	Hostname string `json:"hostname"`
	InFleet  bool   `json:"in_fleet"`
}

func (s Source) fleetBin() string {
	if s.FleetBin == "" {
		return "fleet"
	}
	return s.FleetBin
}

// Resolve produces the probe set.
//
// Order: an explicit override, then fleet's own contract, then a read-only scan
// of the ssh config. fleet failing is not an error — it may simply not be
// installed — so the fallback runs quietly.
func (s Source) Resolve(ctx context.Context) (Hosts, error) {
	if len(s.Override) > 0 {
		return s.partition(namePairs(s.Override), OriginOverride), nil
	}

	if s.Cmd != nil {
		if out, err := s.Cmd.Run(ctx, s.fleetBin(), "discover", "--json"); err == nil {
			var rows []discoverRow
			if json.Unmarshal(out, &rows) == nil {
				pairs := make([][2]string, 0, len(rows))
				for _, r := range rows {
					if !r.InFleet {
						continue // in ssh config but not adopted: not ours to probe
					}
					// ssh resolves Hostname, not the alias.
					name := r.Hostname
					if name == "" {
						name = r.Alias
					}
					pairs = append(pairs, [2]string{r.Alias, name})
				}
				return s.partition(pairs, OriginFleet), nil
			}
		}
	}

	content, err := os.ReadFile(s.SSHConfig)
	if err != nil {
		// No ssh config is a legitimately empty fleet, not a failure.
		if os.IsNotExist(err) {
			return Hosts{Origin: OriginSSHConfig}, nil
		}
		return Hosts{}, err
	}
	return s.partition(sshConfigFleetHosts(string(content)), OriginSSHConfig), nil
}

func namePairs(names []string) [][2]string {
	out := make([][2]string, 0, len(names))
	for _, n := range names {
		out = append(out, [2]string{n, n})
	}
	return out
}

// partition splits candidate names into those worth probing and those that are
// deliberately excluded.
func (s Source) partition(pairs [][2]string, origin Origin) Hosts {
	var served map[string]bool
	if s.HostsFile != "" {
		if b, err := os.ReadFile(s.HostsFile); err == nil {
			served = hostsFileNames(string(b))
		}
	}

	h := Hosts{Origin: origin}
	for _, p := range pairs {
		alias, name := p[0], p[1]
		switch {
		case name == "":
			continue
		case served[name]:
			// nsswitch is "files dns": /etc/hosts answers before any resolver is
			// consulted, so no resolver is ever asked for this name. Probing it
			// would cap the score below 100% forever, and would let verify count
			// a /etc/hosts hit as evidence the RESOLVER works.
			h.Excluded = append(h.Excluded, alias)
		case net.ParseIP(name) != nil:
			// Already an address: nothing for DNS to resolve, and pinning a
			// resolver would not change it either way.
			h.Excluded = append(h.Excluded, alias)
		default:
			h.Probe = append(h.Probe, name)
		}
	}
	return h
}

// hostsFileNames collects every name /etc/hosts answers for.
func hostsFileNames(content string) map[string]bool {
	out := map[string]bool{}
	for line := range strings.SplitSeq(content, "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue // an address with no names, or a blank line
		}
		for _, name := range fields[1:] {
			out[name] = true
		}
	}
	return out
}

// sshConfigFleetHosts scans (never writes) the ssh config for adopted hosts.
//
// Returns alias/hostname pairs: the alias is what a user types and what an
// exclusion is reported under, the hostname is what ssh actually resolves.
func sshConfigFleetHosts(content string) [][2]string {
	var out [][2]string
	var pending []string // aliases of the current #fleet block

	flush := func(hostname string) {
		for _, a := range pending {
			name := hostname
			if name == "" {
				name = a
			}
			out = append(out, [2]string{a, name})
		}
		pending = nil
	}

	hostname := ""
	for line := range strings.SplitSeq(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch strings.ToLower(fields[0]) {
		case "host":
			flush(hostname)
			hostname = ""
			if !strings.Contains(line, FleetMarker) {
				continue
			}
			for _, f := range fields[1:] {
				if strings.HasPrefix(f, "#") {
					break // the trailing comment
				}
				// Patterns cannot be resolved, so they are never probed.
				if strings.ContainsAny(f, "*?!") {
					continue
				}
				pending = append(pending, f)
			}
		case "hostname":
			if len(fields) > 1 && len(pending) > 0 {
				hostname = fields[1]
			}
		}
	}
	flush(hostname)
	return out
}
