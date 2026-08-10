package sshconf

import (
	"strings"
	"testing"
)

func TestAddIsIdempotentAndPreservesOtherBlocks(t *testing.T) {
	base := "Host beta\n    HostName 10.0.0.2\n"
	once, err := Add(base, Host{Alias: "alpha", HostName: "10.0.0.1"}, "#fleet")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	twice, err := Add(once, Host{Alias: "alpha", HostName: "10.0.0.1"}, "#fleet")
	if err != nil {
		t.Fatalf("Add twice: %v", err)
	}
	if once != twice {
		t.Fatalf("Add not idempotent:\n--- once ---\n%s\n--- twice ---\n%s", once, twice)
	}
	if !strings.Contains(once, "Host beta") || !strings.Contains(once, "HostName 10.0.0.2") {
		t.Fatalf("unrelated block was modified:\n%s", once)
	}
	hosts, _ := Parse(once, "#fleet")
	var found bool
	for _, h := range hosts {
		if h.Alias == "alpha" && h.Fleet && h.HostName == "10.0.0.1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("added host missing or unmarked:\n%s", once)
	}
}

func TestAddUpdatesOnlySuppliedFields(t *testing.T) {
	cfg, _ := Add("", Host{Alias: "alpha", HostName: "10.0.0.1", User: "ops"}, "#fleet")
	out, err := Add(cfg, Host{Alias: "alpha", HostName: "10.0.0.9", User: "ops"}, "#fleet")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	hosts, _ := Parse(out, "#fleet")
	if len(hosts) != 1 {
		t.Fatalf("expected exactly one block, got %d:\n%s", len(hosts), out)
	}
	if hosts[0].HostName != "10.0.0.9" || hosts[0].User != "ops" {
		t.Fatalf("update lost fields: %+v", hosts[0])
	}
}

func TestAddRequiresAlias(t *testing.T) {
	if _, err := Add("", Host{HostName: "10.0.0.1"}, "#fleet"); err == nil {
		t.Fatal("expected an error when alias is empty")
	}
}

func TestUnmarkKeepsBlockButLeavesFleet(t *testing.T) {
	cfg, _ := Add("", Host{Alias: "alpha", HostName: "10.0.0.1"}, "#fleet")
	out, err := Unmark(cfg, "alpha", "#fleet")
	if err != nil {
		t.Fatalf("Unmark: %v", err)
	}
	if !strings.Contains(out, "Host alpha") || !strings.Contains(out, "HostName 10.0.0.1") {
		t.Fatalf("Unmark must keep the block and its fields:\n%s", out)
	}
	hosts, _ := Parse(out, "#fleet")
	for _, h := range hosts {
		if h.Alias == "alpha" && h.Fleet {
			t.Fatalf("still marked after Unmark:\n%s", out)
		}
	}
}

func TestUnmarkHandlesOwnLineMarker(t *testing.T) {
	cfg := "Host alpha\n    # fleet\n    HostName 10.0.0.1\n"
	out, err := Unmark(cfg, "alpha", "#fleet")
	if err != nil {
		t.Fatalf("Unmark: %v", err)
	}
	hosts, _ := Parse(out, "#fleet")
	for _, h := range hosts {
		if h.Fleet {
			t.Fatalf("own-line marker not removed:\n%s", out)
		}
	}
	if !strings.Contains(out, "HostName 10.0.0.1") {
		t.Fatalf("Unmark removed a directive:\n%s", out)
	}
}

func TestPurgeRemovesOnlyTheTargetBlock(t *testing.T) {
	cfg := "Host alpha  # fleet\n    HostName 10.0.0.1\n\nHost beta\n    HostName 10.0.0.2\n"
	out, err := Purge(cfg, "alpha")
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if strings.Contains(out, "Host alpha") || strings.Contains(out, "10.0.0.1") {
		t.Fatalf("target block survived Purge:\n%s", out)
	}
	if !strings.Contains(out, "Host beta") || !strings.Contains(out, "HostName 10.0.0.2") {
		t.Fatalf("Purge removed an unrelated block:\n%s", out)
	}
}

func TestPurgeKeepsTrailingBlocksIntact(t *testing.T) {
	cfg := "Host a\n    HostName 1\n\nHost b\n    HostName 2\n\nHost c\n    HostName 3\n"
	out, err := Purge(cfg, "b")
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	hosts, _ := Parse(out, "#fleet")
	var names []string
	for _, h := range hosts {
		names = append(names, h.Alias)
	}
	if strings.Join(names, ",") != "a,c" {
		t.Fatalf("expected a,c got %v:\n%s", names, out)
	}
}

func TestUnknownAliasIsAnError(t *testing.T) {
	if _, err := Unmark("Host beta\n", "nope", "#fleet"); err == nil {
		t.Fatal("Unmark: expected an error for an unknown alias")
	}
	if _, err := Purge("Host beta\n", "nope"); err == nil {
		t.Fatal("Purge: expected an error for an unknown alias")
	}
}

// A config that ended with a newline must still end with exactly one after
// any edit. Found live: add->purge round-trip dropped the trailing newline,
// which shows up as a spurious "\ No newline at end of file" in every
// subsequent diff.
func TestEditsPreserveExactlyOneTrailingNewline(t *testing.T) {
	base := "Host beta\n    HostName 10.0.0.2\n"
	added, err := Add(base, Host{Alias: "tmp", HostName: "10.0.0.99"}, "#fleet")
	if err != nil {
		t.Fatal(err)
	}
	purged, err := Purge(added, "tmp")
	if err != nil {
		t.Fatal(err)
	}
	if purged != base {
		t.Fatalf("add->purge is not a round-trip:\nwant %q\ngot  %q", base, purged)
	}
	for name, got := range map[string]string{"Add": added, "Purge": purged} {
		if !strings.HasSuffix(got, "\n") {
			t.Errorf("%s dropped the trailing newline: %q", name, got)
		}
		if strings.HasSuffix(got, "\n\n") {
			t.Errorf("%s left a doubled trailing newline: %q", name, got)
		}
	}
}

func TestUnmarkPreservesTrailingNewline(t *testing.T) {
	cfg := "Host a  # fleet\n    HostName 1\n"
	out, err := Unmark(cfg, "a", "#fleet")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(out, "\n") || strings.HasSuffix(out, "\n\n") {
		t.Fatalf("Unmark mangled the trailing newline: %q", out)
	}
}
