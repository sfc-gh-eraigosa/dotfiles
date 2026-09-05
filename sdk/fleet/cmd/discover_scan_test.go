package cmd

import (
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
)

func TestClassifyScanIdentifiesMovedNewCurrentAndUnidentified(t *testing.T) {
	hosts := []sshconf.Host{
		{Alias: "steady", HostName: "10.0.0.5", Fleet: true},
		{Alias: "wanderer", HostName: "10.0.0.9", Fleet: true},
	}
	found := []responder{
		{IP: "10.0.0.5", Hostname: "steady", Identified: true},
		{IP: "10.0.0.7", Hostname: "wanderer", Identified: true}, // DHCP moved it
		{IP: "10.0.0.8", Hostname: "newbox", Identified: true},
		{IP: "10.0.0.11", Identified: false}, // answers :22, would not authenticate
	}
	got := map[string]scanKind{}
	alias := map[string]string{}
	for _, r := range classifyScan(hosts, found, nil) {
		got[r.IP] = r.Kind
		alias[r.IP] = r.Alias
	}
	if got["10.0.0.5"] != scanCurrent {
		t.Errorf("steady = %q, want current", got["10.0.0.5"])
	}
	if got["10.0.0.7"] != scanMoved || alias["10.0.0.7"] != "wanderer" {
		t.Errorf("moved host = %q alias %q, want moved/wanderer", got["10.0.0.7"], alias["10.0.0.7"])
	}
	if got["10.0.0.8"] != scanNew {
		t.Errorf("newbox = %q, want new", got["10.0.0.8"])
	}
	if got["10.0.0.11"] != scanUnidentified {
		t.Errorf("silent responder = %q, want unidentified", got["10.0.0.11"])
	}
}

// The point of the scan: a moved host yields a HostName refresh, not a
// duplicate Host block under a second alias.
func TestScanPlanRefreshesAMovedHostRatherThanAddingIt(t *testing.T) {
	cfg := "Host wanderer  #fleet\n    HostName 10.0.0.9\n    ProxyCommand nc %h %p\n"
	hosts, _ := sshconf.Parse(cfg, "#fleet")
	rows := classifyScan(hosts, []responder{{IP: "10.0.0.7", Hostname: "wanderer", Identified: true}}, nil)
	next, changed, err := applyScan(cfg, rows, "#fleet")
	if err != nil {
		t.Fatal(err)
	}
	if changed != 1 {
		t.Fatalf("changed = %d, want 1", changed)
	}
	if strings.Count(next, "Host wanderer") != 1 {
		t.Fatalf("alias duplicated:\n%s", next)
	}
	if !strings.Contains(next, "HostName 10.0.0.7") {
		t.Fatalf("HostName not refreshed:\n%s", next)
	}
	if !strings.Contains(next, "ProxyCommand nc %h %p") {
		t.Fatalf("refresh was destructive:\n%s", next)
	}
}

// An unidentified responder must never be written into the config: we do not
// know what it is or which user it wants.
func TestApplyScanNeverWritesAnUnidentifiedResponder(t *testing.T) {
	rows := []scanRow{{IP: "10.0.0.11", Kind: scanUnidentified}}
	next, changed, err := applyScan("", rows, "#fleet")
	if err != nil {
		t.Fatal(err)
	}
	if changed != 0 || strings.Contains(next, "10.0.0.11") {
		t.Fatalf("an unidentified responder was written:\n%s", next)
	}
}

// A new host is adopted under its own reported hostname as the alias.
func TestApplyScanAdoptsANewHostUnderItsReportedName(t *testing.T) {
	rows := classifyScan(nil, []responder{{IP: "10.0.0.8", Hostname: "newbox", Identified: true}}, nil)
	next, changed, err := applyScan("", rows, "#fleet")
	if err != nil {
		t.Fatal(err)
	}
	if changed != 1 || !strings.Contains(next, "Host newbox") || !strings.Contains(next, "HostName 10.0.0.8") {
		t.Fatalf("changed=%d config:\n%s", changed, next)
	}
}

// Rescanning an unchanged network must produce no edit at all.
func TestScanIsANoOpWhenNothingMoved(t *testing.T) {
	cfg := "Host steady  #fleet\n    HostName 10.0.0.5\n"
	hosts, _ := sshconf.Parse(cfg, "#fleet")
	rows := classifyScan(hosts, []responder{{IP: "10.0.0.5", Hostname: "steady", Identified: true}}, nil)
	next, changed, err := applyScan(cfg, rows, "#fleet")
	if err != nil {
		t.Fatal(err)
	}
	if changed != 0 || next != cfg {
		t.Fatalf("changed=%d, config drifted:\n%s", changed, next)
	}
}

// Addresses must order NUMERICALLY. String ordering puts .128 before .16 and
// .201 before .61, which makes a scan of the same network read as noise — the
// operator cannot scan the eye down a column that is not in address order.
// Caught by running a live sweep, not by review.
func TestClassifyScanOrdersAddressesNumerically(t *testing.T) {
	found := []responder{
		{IP: "192.168.0.61"}, {IP: "192.168.0.128"}, {IP: "192.168.0.16"},
		{IP: "192.168.0.201"}, {IP: "192.168.0.9"},
	}
	rows := classifyScan(nil, found, nil)
	var got []string
	for _, r := range rows {
		got = append(got, r.IP)
	}
	want := []string{"192.168.0.9", "192.168.0.16", "192.168.0.61", "192.168.0.128", "192.168.0.201"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v\nwant %v", got, want)
		}
	}
}
