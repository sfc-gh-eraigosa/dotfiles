package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDiscoverRowsSplitsFleetFromAvailable(t *testing.T) {
	cfg := "" +
		"Host web  #fleet\n    HostName 10.0.0.1\n\n" +
		"Host db\n    HostName 10.0.0.2\n\n" +
		"Host *.internal\n    User ops\n\n" + // pattern: must be omitted
		"Host cache\n    HostName 10.0.0.3\n"
	rows := discoverRows(cfg, "#fleet")

	// Pattern host is not a machine — never listed.
	for _, r := range rows {
		if strings.ContainsAny(r.Alias, "*?") {
			t.Fatalf("pattern host leaked into discovery: %v", rows)
		}
	}
	// Sorted by alias, so the order is deterministic: cache, db, web.
	var aliases []string
	byAlias := map[string]discoverRow{}
	for _, r := range rows {
		aliases = append(aliases, r.Alias)
		byAlias[r.Alias] = r
	}
	if strings.Join(aliases, ",") != "cache,db,web" {
		t.Fatalf("expected sorted cache,db,web got %v", aliases)
	}
	if byAlias["web"].InFleet != true {
		t.Fatalf("web should be in-fleet: %+v", byAlias["web"])
	}
	if byAlias["db"].InFleet != false || byAlias["cache"].InFleet != false {
		t.Fatalf("db/cache should be available, not in-fleet: %+v", rows)
	}
}

func TestRenderDiscoverJSONIsValidAndFlags(t *testing.T) {
	cfg := "Host web  #fleet\n    HostName 10.0.0.1\n\nHost db\n    HostName 10.0.0.2\n"
	out := renderDiscoverJSON(discoverRows(cfg, "#fleet"))
	var got []discoverRow
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("discover --json is not valid JSON: %v\n%s", err, out)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(got))
	}
}

func TestRenderDiscoverTableMarksAvailable(t *testing.T) {
	cfg := "Host web  #fleet\n    HostName 10.0.0.1\n\nHost db\n    HostName 10.0.0.2\n"
	out := renderDiscoverTable(discoverRows(cfg, "#fleet"))
	if !strings.Contains(out, "available") || !strings.Contains(out, "in-fleet") {
		t.Fatalf("table must show both statuses:\n%s", out)
	}
	if !strings.Contains(out, "db") || !strings.Contains(out, "web") {
		t.Fatalf("table missing hosts:\n%s", out)
	}
}

// --- bulk adopt ----------------------------------------------------------

// adoptAll marks every available host in ONE pass over the config, so the
// result is a single write (and therefore a single backup), not N.
func TestAdoptAllMarksEveryAvailableHostInOnePass(t *testing.T) {
	cfg := "" +
		"Host web  #fleet\n    HostName 10.0.0.1\n\n" +
		"Host db\n    HostName 10.0.0.2\n    User ops\n\n" +
		"Host cache\n    HostName 10.0.0.3\n"
	next, adopted, err := adoptAll(cfg, "#fleet")
	if err != nil {
		t.Fatalf("adoptAll: %v", err)
	}
	if strings.Join(adopted, ",") != "cache,db" {
		t.Fatalf("expected the available hosts in table order, got %v", adopted)
	}
	rows := discoverRows(next, "#fleet")
	for _, r := range rows {
		if !r.InFleet {
			t.Fatalf("%s left out of the fleet:\n%s", r.Alias, next)
		}
	}
	// Adoption must preserve each block's directives — that is the whole point
	// of marking in place rather than re-rendering from flags.
	var db discoverRow
	for _, r := range rows {
		if r.Alias == "db" {
			db = r
		}
	}
	if db.HostName != "10.0.0.2" {
		t.Fatalf("adoption lost a directive: %+v", db)
	}
	if !strings.Contains(next, "User ops") {
		t.Fatalf("adoption dropped a non-indexed directive:\n%s", next)
	}
}

// Nothing available must be a no-op that returns the config untouched, so the
// caller can skip the write (and the backup) entirely.
func TestAdoptAllIsANoOpWhenEverythingIsAlreadyInFleet(t *testing.T) {
	cfg := "Host web  #fleet\n    HostName 10.0.0.1\n"
	next, adopted, err := adoptAll(cfg, "#fleet")
	if err != nil {
		t.Fatalf("adoptAll: %v", err)
	}
	if len(adopted) != 0 {
		t.Fatalf("nothing should have been adopted, got %v", adopted)
	}
	if next != cfg {
		t.Fatalf("a no-op must not rewrite the config:\n%s", next)
	}
}

func TestAdoptAllSkipsPatternHosts(t *testing.T) {
	cfg := "Host *.internal\n    User ops\n\nHost real\n    HostName 10.0.0.9\n"
	_, adopted, err := adoptAll(cfg, "#fleet")
	if err != nil {
		t.Fatalf("adoptAll: %v", err)
	}
	if strings.Join(adopted, ",") != "real" {
		t.Fatalf("pattern hosts are not machines and must not be adopted, got %v", adopted)
	}
}
