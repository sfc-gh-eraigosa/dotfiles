// Package resolvconf renders and edits the two files wlink manages:
// /etc/resolv.conf and /etc/wsl.conf.
//
// Rendering is kept separate from writing on purpose. Every shape here is
// exercised as a pure string transform, so the file surgery that could clobber
// someone's wsl.conf is fully covered without a privileged write in sight.
package resolvconf

import (
	"fmt"
	"strconv"
	"strings"
)

// ManagedMarker identifies a file wlink wrote. Drift detection depends on it:
// without a marker there is no way to tell our file from a hand-edited one.
const ManagedMarker = "# Managed by wlink"

// MaxNS is glibc's cap on nameservers read from resolv.conf. Emitting more is
// not more redundancy — the extras are silently ignored, which would give a
// false sense of having fallbacks.
const MaxNS = 3

// glibc's defaults when resolv.conf carries no `options` line.
const (
	glibcDefaultTimeoutSeconds = 5
	// Each nameserver is tried `attempts` times, not once. Omitting this is why
	// the budget came out at 11s on a stock config whose real worst case was
	// measured at 20–21s — so a perfectly normal miss was reported as a
	// regression, which is the exact failure the derivation exists to avoid.
	glibcDefaultAttempts = 2
)

// Render describes the resolv.conf to produce.
type Render struct {
	// Winner goes first: nameserver #1 is asked for every name.
	Winner string
	// Fallbacks are reached only when the winner TIMES OUT — never on an
	// NXDOMAIN, which is final.
	Fallbacks []string
	// Timeout and Attempts default to 1/1: the cap that turns an off-network
	// miss from ~20s into ~1s per dead server.
	Timeout  int
	Attempts int
}

// RenderResolvConf produces the managed file.
func RenderResolvConf(r Render) string {
	timeout, attempts := r.Timeout, r.Attempts
	if timeout <= 0 {
		timeout = 1
	}
	if attempts <= 0 {
		attempts = 1
	}

	servers := make([]string, 0, MaxNS)
	seen := map[string]bool{}
	for _, s := range append([]string{r.Winner}, r.Fallbacks...) {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		servers = append(servers, s)
		if len(servers) == MaxNS {
			break
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s — do not edit.\n", ManagedMarker)
	b.WriteString("# Regenerate: wlink pin   ·   Undo: wlink unpin\n")
	b.WriteString("# WSL's generated resolv.conf is disabled via /etc/wsl.conf (generateResolvConf).\n")
	fmt.Fprintf(&b, "options timeout:%d attempts:%d\n", timeout, attempts)
	for _, s := range servers {
		fmt.Fprintf(&b, "nameserver %s\n", s)
	}
	return b.String()
}

// IsManaged reports whether wlink wrote this file.
func IsManaged(content string) bool { return strings.Contains(content, ManagedMarker) }

// Nameservers lists the resolvers in a resolv.conf, in order.
func Nameservers(content string) []string {
	var out []string
	for line := range strings.SplitSeq(content, "\n") {
		// resolv.conf accepts both '#' and ';' as comment introducers.
		if i := strings.IndexAny(line, "#;"); i >= 0 {
			line = line[:i]
		}
		if f := strings.Fields(line); len(f) >= 2 && f[0] == "nameserver" {
			out = append(out, f[1])
		}
	}
	return out
}

// FailBudgetSeconds is the longest a FAILED lookup should plausibly take under
// the resolver config actually in force.
//
//	budget = nameservers × timeout × attempts × 2 families + 1s slack
//
// Derived rather than guessed, because a hardcoded limit is exactly what made
// the prototype's verify report a regression on a run that was in fact a 5×
// improvement. The ×2 is because a miss walks every nameserver for BOTH A and
// AAAA, so a dead server's timeout is paid once per family.
func FailBudgetSeconds(content string) int {
	ns := len(Nameservers(content))
	if ns == 0 {
		ns = 1
	}
	timeout, attempts := glibcDefaultTimeoutSeconds, glibcDefaultAttempts
	for line := range strings.SplitSeq(content, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "options") {
			continue
		}
		for f := range strings.FieldsSeq(line) {
			if v, ok := strings.CutPrefix(f, "timeout:"); ok {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					timeout = n
				}
			}
			if v, ok := strings.CutPrefix(f, "attempts:"); ok {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					attempts = n
				}
			}
		}
	}
	return ns*timeout*attempts*2 + 1
}
