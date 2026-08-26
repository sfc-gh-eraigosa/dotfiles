package winhost

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Queries ask for JSON rather than the default table rendering: table output is
// column-width-truncated and locale-dependent, and parsing it is how these
// integrations usually break.
var (
	dnsServersScript = script{
		kind: kindDNSServers,
		source: `Get-DnsClientServerAddress -AddressFamily IPv4 | ` +
			`Select-Object InterfaceAlias,InterfaceIndex,ServerAddresses | ConvertTo-Json -Compress`,
	}
	adaptersScript = script{
		kind: kindAdapters,
		source: `Get-NetAdapter | ` +
			`Select-Object Name,InterfaceDescription,Status,InterfaceIndex | ConvertTo-Json -Compress`,
	}
)

type dnsRow struct {
	InterfaceAlias  string   `json:"InterfaceAlias"`
	InterfaceIndex  int      `json:"InterfaceIndex"`
	ServerAddresses []string `json:"ServerAddresses"`
}

type adapterRow struct {
	Name                 string `json:"Name"`
	InterfaceDescription string `json:"InterfaceDescription"`
	Status               string `json:"Status"`
	InterfaceIndex       int    `json:"InterfaceIndex"`
}

// decodeRows unmarshals a PowerShell ConvertTo-Json payload into a slice.
//
// ConvertTo-Json emits a BARE OBJECT when exactly one row matches and an ARRAY
// otherwise. A parser that assumes an array silently returns nothing on a
// single-interface machine, so both shapes are handled here rather than at each
// call site. Output is also CRLF-terminated and may carry a UTF-8 BOM.
func decodeRows[T any](raw []byte) ([]T, error) {
	trimmed := bytes.TrimSpace(bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf")))
	if len(trimmed) == 0 {
		return nil, nil
	}
	var many []T
	if err := json.Unmarshal(trimmed, &many); err == nil {
		return many, nil
	}
	var one T
	if err := json.Unmarshal(trimmed, &one); err != nil {
		return nil, fmt.Errorf("parsing PowerShell JSON: %w", err)
	}
	return []T{one}, nil
}

// wgAlias matches WireGuard-shaped interface names, which is how a tunnel
// presents when its adapter description is unavailable. Covers both the
// client-generated form (wg65fffd80) and user-named tunnels (wg0, wg-lab,
// wg_home). Deliberately broad: a false positive only changes the wording of a
// diagnosis, while a false negative loses the "attached but not ready"
// distinction entirely — the asymmetry favours matching.
var wgAlias = regexp.MustCompile(`^(?i)wg([-_]?[0-9a-z]+)?$`)

// tunAlias matches tun0/utun3-style names. Package-level so it is compiled
// once rather than on every classification.
var tunAlias = regexp.MustCompile(`^u?tun[0-9]*$`)

// isTunnelAdapter reports whether an adapter is a VPN/tunnel rather than a
// physical or virtual-switch interface.
//
// This matters because the resolver that knows your fleet is usually on the
// tunnel, and because "a tunnel is attached but not answering" is the signature
// of a handshake still completing — a state worth naming rather than reporting
// as a generic failure.
//
// Deliberately excluded: Hyper-V virtual switches (the WSL vEthernet adapter
// says "Virtual", but it is a bridge to the host, not a tunnel) and Bluetooth
// PAN.
func isTunnelAdapter(alias, description string) bool {
	a, d := strings.ToLower(alias), strings.ToLower(description)
	for _, marker := range []string{"wireguard", "wintun", "openvpn", "tap-windows", "tailscale", "zerotier"} {
		if strings.Contains(d, marker) || strings.Contains(a, marker) {
			return true
		}
	}
	if wgAlias.MatchString(alias) {
		return true
	}
	// "tun" as a whole word/prefix on the alias (tun0, utun3) — but not as a
	// substring of unrelated names.
	return tunAlias.MatchString(a)
}

// Interfaces enumerates every Windows interface with its resolvers.
//
// Adapter data is best-effort: tunnel classification is a refinement, so a
// machine where Get-NetAdapter fails still gets a usable resolver list rather
// than nothing. The DNS query is not optional — without it there is no answer.
func (h *Host) Interfaces(ctx context.Context) ([]Interface, error) {
	raw, err := h.r.Run(ctx, dnsServersScript)
	if err != nil {
		return nil, fmt.Errorf("querying Windows DNS servers: %w", err)
	}
	dnsRows, err := decodeRows[dnsRow](raw)
	if err != nil {
		return nil, err
	}

	tunnelByIndex, tunnelByAlias := map[int]bool{}, map[string]bool{}
	if rawAd, adErr := h.r.Run(ctx, adaptersScript); adErr == nil {
		if adapters, decErr := decodeRows[adapterRow](rawAd); decErr == nil {
			for _, a := range adapters {
				if isTunnelAdapter(a.Name, a.InterfaceDescription) {
					tunnelByIndex[a.InterfaceIndex] = true
					tunnelByAlias[a.Name] = true
				}
			}
		}
	}

	out := make([]Interface, 0, len(dnsRows))
	for _, r := range dnsRows {
		servers := r.ServerAddresses
		if servers == nil {
			servers = []string{}
		}
		out = append(out, Interface{
			Alias:      r.InterfaceAlias,
			Index:      r.InterfaceIndex,
			DNSServers: servers,
			// Fall back to the alias when adapter data was unavailable, so a
			// tunnel is still recognised on a machine where Get-NetAdapter fails.
			IsTunnel: tunnelByIndex[r.InterfaceIndex] ||
				tunnelByAlias[r.InterfaceAlias] ||
				isTunnelAdapter(r.InterfaceAlias, ""),
		})
	}
	return out, nil
}
