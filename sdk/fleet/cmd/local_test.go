package cmd

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// modelOf builds a model from FULL host blocks, which the alias-only `hosts`
// helper cannot express — HostName is half of what decides "is this me".
func modelOf(hs ...sshconf.Host) tuiModel {
	return newTUIModel(hs, runner.Fake{}, fakeBaseline{head: "abc"}, testNow, "main", 2, updplan.Default())
}

// --- who am I -------------------------------------------------------------

// The common case: the ssh alias IS the machine's hostname. Alias matching is
// what keeps the badge right when DHCP has moved the box and the config's
// HostName is stale.
func TestLocalHostMatchesByAlias(t *testing.T) {
	me := localHost{Name: "host-spark", IPs: []string{"10.0.0.9"}}
	h := sshconf.Host{Alias: "host-spark", HostName: "192.168.0.201"}
	if !isLocalHost(h, me) {
		t.Fatal("a host whose alias is this machine's hostname must be recognised as local")
	}
}

// The other half of the same question: the alias is a nickname, but the
// configured address is one this machine actually answers on.
func TestLocalHostMatchesByInterfaceIP(t *testing.T) {
	me := localHost{Name: "somethingelse", IPs: []string{"192.168.0.201", "fd00::1"}}
	for _, hostname := range []string{"192.168.0.201", "fd00::1"} {
		if !isLocalHost(sshconf.Host{Alias: "gpu", HostName: hostname}, me) {
			t.Fatalf("HostName %s is one of this machine's own addresses — must be local", hostname)
		}
	}
}

// The loopback case, asked for by name: an entry pointed back at this machine
// is local even when nothing about the NAME says so.
func TestLocalHostMatchesLoopback(t *testing.T) {
	me := localHost{Name: "box", IPs: []string{"192.168.0.5"}}
	for _, hostname := range []string{"127.0.0.1", "127.0.1.1", "::1", "localhost", "localhost.localdomain"} {
		if !isLocalHost(sshconf.Host{Alias: "tunnel", HostName: hostname}, me) {
			t.Fatalf("HostName %s points back at this machine — must be local", hostname)
		}
	}
}

// A short hostname and its FQDN are the same machine.
func TestLocalHostMatchesTheShortFormOfAnFQDN(t *testing.T) {
	me := localHost{Name: "host-spark.lan"}
	if !isLocalHost(sshconf.Host{Alias: "host-spark"}, me) {
		t.Fatal("short alias must match this machine's FQDN")
	}
	if !isLocalHost(sshconf.Host{Alias: "x", HostName: "HOST-SPARK.example.com"}, me) {
		t.Fatal("hostname comparison must be case-insensitive and domain-insensitive")
	}
}

// An ssh block with no HostName connects to the alias — so the alias IS the
// address, and must be matched as one.
func TestLocalHostFallsBackToTheAliasWhenNoHostNameIsSet(t *testing.T) {
	me := localHost{Name: "box", IPs: []string{"192.168.0.5"}}
	if !isLocalHost(sshconf.Host{Alias: "192.168.0.5"}, me) {
		t.Fatal("with no HostName, ssh dials the alias — it must be matched as an address")
	}
}

// The whole point is that exactly ONE row lights up. A peer must never match.
func TestLocalHostRejectsEveryOtherMachine(t *testing.T) {
	me := localHost{Name: "host-spark", IPs: []string{"192.168.0.201"}}
	for _, h := range []sshconf.Host{
		{Alias: "host-nano", HostName: "192.168.0.63"},
		{Alias: "host-pi", HostName: "192.168.0.128"},
		{Alias: "host-sparkle", HostName: "192.168.0.7"}, // near-miss name
		{Alias: "host-spark-old", HostName: "192.168.0.8"},
	} {
		if isLocalHost(h, me) {
			t.Fatalf("%s is a different machine — must not be local", h.Alias)
		}
	}
}

// An unresolvable identity must light up nothing rather than guess.
func TestLocalHostWithNoIdentityMatchesNothing(t *testing.T) {
	if isLocalHost(sshconf.Host{Alias: "a", HostName: "b"}, localHost{}) {
		t.Fatal("with no hostname and no addresses, nothing can be claimed as local")
	}
}

// --- the model's pick -----------------------------------------------------

func TestModelResolvesTheLocalAliasFromTheFleet(t *testing.T) {
	m := testModel("host-nano", "host-spark", "host-pi")
	m.setLocal(localHost{Name: "host-spark"})
	if m.localAlias != "host-spark" {
		t.Fatalf("localAlias = %q, want host-spark", m.localAlias)
	}
}

// Running the dashboard from a machine that is not in the fleet is normal
// (it is how a host gets adopted); it must resolve to no row, not to one.
func TestModelLeavesTheLocalAliasEmptyWhenWeAreNotInTheFleet(t *testing.T) {
	m := testModel("host-nano", "host-pi")
	m.setLocal(localHost{Name: "laptop"})
	if m.localAlias != "" {
		t.Fatalf("localAlias = %q, want empty", m.localAlias)
	}
}

// Two blocks can legitimately point at this machine (an alias and a loopback
// tunnel). The pick must be stable, or the highlight moves on its own between
// restarts.
func TestLocalAliasIsDeterministicWhenTwoBlocksMatch(t *testing.T) {
	build := func() string {
		m := modelOf(
			sshconf.Host{Alias: "zulu", HostName: "127.0.0.1"},
			sshconf.Host{Alias: "alpha", HostName: "localhost"},
		)
		m.setLocal(localHost{Name: "box"})
		return m.localAlias
	}
	if got := build(); got != "alpha" {
		t.Fatalf("localAlias = %q, want alpha (first in row order)", got)
	}
	if build() != build() {
		t.Fatal("the local pick must not vary between runs")
	}
}

// --- the header badge -----------------------------------------------------

func TestBannerNamesTheMachineWeAreRunningOn(t *testing.T) {
	m := testModel("host-nano", "host-spark")
	m.setLocal(localHost{Name: "host-spark"})
	if !strings.Contains(m.banner(), "host-spark") {
		t.Fatalf("banner must name this machine:\n%s", m.banner())
	}
}

// Not being in the fleet is worth SAYING — otherwise a bare hostname in the
// header reads as "and it is the highlighted row", when no row is highlighted.
func TestBannerSaysWhenThisMachineIsNotInTheFleet(t *testing.T) {
	m := testModel("host-nano")
	m.setLocal(localHost{Name: "laptop"})
	b := m.banner()
	if !strings.Contains(b, "laptop") || !strings.Contains(b, "not in fleet") {
		t.Fatalf("banner must name this machine and say it is not in the fleet:\n%s", b)
	}
}

// When the fleet knows this machine under a DIFFERENT alias, both names must
// appear: the hostname answers "where am I", the alias answers "which row".
func TestBannerNamesBothWhenTheAliasDiffersFromTheHostname(t *testing.T) {
	m := modelOf(sshconf.Host{Alias: "gpu", HostName: "192.168.0.201"})
	m.setLocal(localHost{Name: "host-spark", IPs: []string{"192.168.0.201"}})
	b := m.banner()
	if !strings.Contains(b, "host-spark") || !strings.Contains(b, "gpu") {
		t.Fatalf("banner must carry both the hostname and the fleet alias:\n%s", b)
	}
}

// A machine that cannot name itself gets no badge rather than an empty one.
func TestBannerHasNoBadgeWithoutAHostname(t *testing.T) {
	if strings.Contains(testModel("a").banner(), localGlyph) {
		t.Fatal("no hostname means no badge")
	}
}

// The banner is framed at panelWidth; a badge that overflows would push the
// border past the terminal edge.
func TestBannerStaysInsideThePanel(t *testing.T) {
	m := testModel("a")
	m.vp = viewport{height: 16, width: 100}
	m.setLocal(localHost{Name: "a-very-long-hostname-that-goes-on-and-on.example.internal"})
	for _, line := range strings.Split(m.banner(), "\n") {
		if w := lipgloss.Width(line); w > m.vp.width {
			t.Fatalf("banner line is %d wide, terminal is %d:\n%s", w, m.vp.width, line)
		}
	}
}

// --- the highlighted row --------------------------------------------------

// Colour has to actually reach the row, so this one runs with a real profile.
func TestLocalRowIsPaintedWithTheLocalColour(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := testModel("host-nano", "host-spark")
	m.setLocal(localHost{Name: "host-spark"})
	m.setRow(Row{Alias: "host-spark", Class: "up-to-date"})
	m.setRow(Row{Alias: "host-nano", Class: "up-to-date"})

	local := m.rowView(m.indexOf("host-spark"))
	if !strings.Contains(local, th.local.Render("host-spark")) {
		t.Fatalf("the local row's alias must carry the local style:\n%q", local)
	}
	if peer := m.rowView(m.indexOf("host-nano")); strings.Contains(peer, localColor) {
		t.Fatalf("a peer row must not carry the local colour:\n%q", peer)
	}
}

// Styling must not cost the row a cell: the alias column is fixed width and
// every column after it lines up on that.
func TestLocalHighlightDoesNotChangeTheRowWidth(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := testModel("host-nano", "host-spark")
	m.setRow(Row{Alias: "host-spark", Class: "up-to-date"})
	plain := lipgloss.Width(m.rowView(m.indexOf("host-spark")))
	m.setLocal(localHost{Name: "host-spark"})
	if got := lipgloss.Width(m.rowView(m.indexOf("host-spark"))); got != plain {
		t.Fatalf("highlighted row is %d wide, plain row is %d", got, plain)
	}
}

// A search hit is transient and answers "what did I just type"; "this is you"
// is always true and can be re-read from the header. The search wins.
func TestSearchHighlightWinsOverTheLocalHighlight(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := testModel("host-spark")
	m.setRow(Row{Alias: "host-spark", Class: "up-to-date"})
	m.setLocal(localHost{Name: "host-spark"})
	m, _ = send(m, "/", "s", "p", "a", "r", "k", "enter")
	row := m.rowView(m.indexOf("host-spark"))
	if !strings.Contains(row, th.match.Render("host-spark")) {
		t.Fatalf("a matched row must keep the search highlight:\n%q", row)
	}
}

// --- detectSelf -----------------------------------------------------------

// The one impure edge: it must name this machine and never invent addresses.
func TestDetectLocalNamesThisMachine(t *testing.T) {
	me := detectLocal()
	if me.Name == "" {
		t.Fatal("detectLocal must report this machine's hostname")
	}
	for _, ip := range me.IPs {
		if ip == "" {
			t.Fatal("detectLocal must not report empty addresses")
		}
	}
}
