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
