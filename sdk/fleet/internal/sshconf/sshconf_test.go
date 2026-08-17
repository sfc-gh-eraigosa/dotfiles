package sshconf

import "testing"

const cfg = `Host alpha  # fleet
    HostName 10.0.0.1
    User ops

Host beta
    HostName 10.0.0.2

Host *
    ServerAliveInterval 60

Host gamma
    # fleet
    HostName 10.0.0.3
    Port 2222
    IdentityFile ~/.ssh/id_gamma
`

func marked(hosts []Host) []string {
	var out []string
	for _, h := range hosts {
		if h.Fleet {
			out = append(out, h.Alias)
		}
	}
	return out
}

func TestParseReturnsOnlyMarkedConcreteHosts(t *testing.T) {
	hosts, err := Parse(cfg, "#fleet")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := marked(hosts)
	want := []string{"alpha", "gamma"}
	if len(got) != len(want) {
		t.Fatalf("marked = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("marked = %v, want %v", got, want)
		}
	}
}

func TestParseSkipsPatternHostsEntirely(t *testing.T) {
	hosts, _ := Parse(cfg, "#fleet")
	for _, h := range hosts {
		if h.Alias == "*" || h.Alias == "?" {
			t.Fatalf("pattern host %q must not be returned", h.Alias)
		}
	}
}

func TestParseCapturesFields(t *testing.T) {
	hosts, _ := Parse(cfg, "#fleet")
	var g Host
	for _, h := range hosts {
		if h.Alias == "gamma" {
			g = h
		}
	}
	if g.HostName != "10.0.0.3" || g.Port != "2222" || g.Identity != "~/.ssh/id_gamma" {
		t.Fatalf("gamma = %+v", g)
	}
}

func TestParseUnmarkedHostIsNotInFleet(t *testing.T) {
	hosts, _ := Parse(cfg, "#fleet")
	for _, h := range hosts {
		if h.Alias == "beta" && h.Fleet {
			t.Fatal("unmarked host must not be in fleet scope")
		}
	}
}

// A marker that merely appears as a substring of another word must not count.
func TestParseMarkerIsNotMatchedLoosely(t *testing.T) {
	hosts, _ := Parse("Host solo  # fleetwood\n    HostName 10.0.0.9\n", "#fleet")
	for _, h := range hosts {
		if h.Fleet {
			t.Fatalf("%q matched the marker too loosely", h.Alias)
		}
	}
}

// The docs write the marker "# fleet"; the --marker default is "#fleet".
// Both spellings must be recognised.
func TestParseAcceptsBothMarkerSpellings(t *testing.T) {
	for _, in := range []string{"Host a  # fleet\n", "Host a  #fleet\n"} {
		hosts, _ := Parse(in, "#fleet")
		if len(hosts) != 1 || !hosts[0].Fleet {
			t.Fatalf("%q was not recognised as in-fleet", in)
		}
	}
}

func TestParseEmptyConfigIsNotAnError(t *testing.T) {
	hosts, err := Parse("", "#fleet")
	if err != nil {
		t.Fatalf("Parse(\"\"): %v", err)
	}
	if len(hosts) != 0 {
		t.Fatalf("expected no hosts, got %v", hosts)
	}
}
