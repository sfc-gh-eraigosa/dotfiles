package cfgplan

import "testing"

const localCfg = "Host keep  #fleet\n    HostName 10.0.0.1\n    User me\n"

func TestBuildClassifiesAddUpdateAndUnchanged(t *testing.T) {
	remote := "Host keep  #fleet\n    HostName 10.0.0.9\n    User me\n" +
		"Host fresh  #fleet\n    HostName 10.0.0.2\n"
	p, err := Build(localCfg, remote, Opts{Marker: "#fleet", Source: "src"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]ChangeKind{}
	for _, c := range p.Changes {
		got[c.Alias] = c.Kind
	}
	if got["fresh"] != Add {
		t.Errorf("fresh = %q, want %q", got["fresh"], Add)
	}
	if got["keep"] != Update {
		t.Errorf("keep = %q, want %q", got["keep"], Update)
	}
}

// A field that did not move must not be reported as a change: the diff is the
// operator's only safety, so noise in it is a correctness problem.
func TestBuildReportsOnlyFieldsThatMoved(t *testing.T) {
	remote := "Host keep  #fleet\n    HostName 10.0.0.9\n    User me\n"
	p, _ := Build(localCfg, remote, Opts{Marker: "#fleet", Source: "src"})
	if len(p.Changes) != 1 || len(p.Changes[0].Fields) != 1 {
		t.Fatalf("want exactly one field delta, got %+v", p.Changes)
	}
	if f := p.Changes[0].Fields[0]; f.Name != "HostName" || f.From != "10.0.0.1" || f.To != "10.0.0.9" {
		t.Fatalf("delta = %+v", f)
	}
}

// Only marked blocks travel unless All is set — the source decides what it shares.
func TestBuildHonoursTheMarkerScope(t *testing.T) {
	remote := "Host personal\n    HostName 10.0.0.3\n"
	if p, _ := Build("", remote, Opts{Marker: "#fleet"}); len(p.Changes) != 0 {
		t.Fatalf("unmarked host must not travel, got %+v", p.Changes)
	}
	if p, _ := Build("", remote, Opts{Marker: "#fleet", All: true}); len(p.Changes) != 1 {
		t.Fatalf("--all must include it, got %+v", p.Changes)
	}
}

// Omission is not an instruction to delete.
func TestBuildNeverBlanksAFieldTheSourceOmitted(t *testing.T) {
	p, _ := Build(localCfg, "Host keep  #fleet\n    HostName 10.0.0.9\n", Opts{Marker: "#fleet"})
	if len(p.Changes) != 1 {
		t.Fatalf("changes = %+v", p.Changes)
	}
	if p.Changes[0].Host.User != "me" {
		t.Fatalf("User = %q, want the local value preserved", p.Changes[0].Host.User)
	}
}
