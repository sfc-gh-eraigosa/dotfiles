package cmd

import (
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/cfgplan"
)

// diff is read-only and shows BOTH directions without performing either — it is
// the only place the two appear together, and it changes nothing.
func TestDiffShowsBothDirections(t *testing.T) {
	local := "Host a  #fleet\n    HostName 10.0.0.1\n"
	remote := "Host a  #fleet\n    HostName 10.0.0.9\n\nHost b  #fleet\n    HostName 10.0.0.2\n"
	in, out, err := diffBothWays(local, remote, cfgplan.Opts{Marker: "#fleet"})
	if err != nil {
		t.Fatal(err)
	}
	if in.Empty() {
		t.Fatal("inbound should see b as an add and a as an update")
	}
	if out.Empty() {
		t.Fatal("outbound should see a as an update")
	}
	var inbound []string
	for _, c := range in.Changes {
		if c.Kind == cfgplan.Add {
			inbound = append(inbound, c.Alias)
		}
	}
	if strings.Join(inbound, ",") != "b" {
		t.Fatalf("inbound adds = %v, want just b", inbound)
	}
}

func TestDiffOfIdenticalConfigsIsEmptyBothWays(t *testing.T) {
	same := "Host a  #fleet\n    HostName 10.0.0.1\n"
	in, out, err := diffBothWays(same, same, cfgplan.Opts{Marker: "#fleet"})
	if err != nil {
		t.Fatal(err)
	}
	if !in.Empty() || !out.Empty() {
		t.Fatalf("identical configs must diff empty both ways: in=%+v out=%+v", in.Changes, out.Changes)
	}
}
