package cmd

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/pins"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
)

// --- the PINS column rides the one status probe (pins plan §3.5) -----------

func TestProbePinsSectionAsksTheHostsOwnFleetAgainstThePlanRepo(t *testing.T) {
	got := probeCmdFor("~/.local/state/playground/install-stamp", "~/github/org/playground")
	want := "echo '" + pinsDelim + "'; ~/opt/bin/fleet pins status --installed --json --repo ~/github/org/playground 2>/dev/null || true"
	if !strings.HasSuffix(got, want) {
		t.Fatalf("probe %q does not end with the pins section %q", got, want)
	}
	// The pins section is appended AFTER the branch section, so an old
	// fleet reading a new reply still finds stamp and branch where it
	// always has.
	if strings.Index(got, probeDelim) > strings.Index(got, pinsDelim) {
		t.Fatalf("pins section precedes the branch section: %q", got)
	}
}

func TestSplitProbeThreeSections(t *testing.T) {
	out := "STAMP\n" + probeDelim + "\nmain\n" + pinsDelim + "\n{\"summary\":{}}\n"
	stampText, branch, pinsText := splitProbe(out)
	if strings.TrimSpace(stampText) != "STAMP" || branch != "main" || strings.TrimSpace(pinsText) != `{"summary":{}}` {
		t.Fatalf("got %q / %q / %q", stampText, branch, pinsText)
	}
}

func TestSplitProbeOlderRepliesHaveNoPins(t *testing.T) {
	// A host whose fleet predates pins prints nothing after the delimiter;
	// a reply from before the branch section has no delimiters at all.
	for _, out := range []string{
		"STAMP\n" + probeDelim + "\nmain\n" + pinsDelim + "\n",
		"STAMP\n" + probeDelim + "\nmain\n",
		"STAMP\n",
	} {
		_, _, p := splitProbe(out)
		if parsePinsSummary(p) != nil {
			t.Fatalf("reply %q produced a pins summary", out)
		}
	}
}

func TestParsePinsSummary(t *testing.T) {
	good := `{"rows":[{"tool":"yq","track":"pin","want":"4.44.6","installed":"4.40.0","state":"drift"}],` +
		`"summary":{"ok":9,"drift":2,"missing":1,"unknown":0}}`
	s := parsePinsSummary(good)
	if s == nil || s.OK != 9 || s.Drift != 2 || s.Missing != 1 {
		t.Fatalf("parsed %+v", s)
	}
	for _, bad := range []string{"", "   ", "Error: unknown command \"pins\"", `{"rows":[]}`, `{"summary":`} {
		if parsePinsSummary(bad) != nil {
			t.Fatalf("%q must not parse as a summary", bad)
		}
	}
}

func TestProbeHostCarriesThePinsSummary(t *testing.T) {
	sha := strings.Repeat("a", 40)
	reply := "commit=" + sha + "\ninstalled_at=1754700000\nbranch=main\nhostname=h\n" +
		probeDelim + "\nmain\n" + pinsDelim + "\n" + `{"summary":{"ok":3,"drift":1,"missing":0,"unknown":0}}`
	r := runner.Fake{Out: map[string]string{"h1": reply}}
	row := probeHost(sshconf.Host{Alias: "h1"}, r, fakeBaseline{head: sha})
	if row.Class != "up-to-date" || row.Branch != "main" {
		t.Fatalf("the pins section broke the stamp/branch parse: %+v", row)
	}
	if row.Pins == nil || row.Pins.Drift != 1 || row.Pins.OK != 3 {
		t.Fatalf("row.Pins = %+v", row.Pins)
	}
}

func TestPinsCell(t *testing.T) {
	cases := []struct {
		in   *pins.Summary
		want string
	}{
		{nil, "-"},
		{&pins.Summary{OK: 10}, "ok"},
		// missing is not drift: a tool can be absent on purpose (its gff
		// flag is off), so it never turns a host's cell into a warning.
		{&pins.Summary{OK: 10, Missing: 2}, "ok"},
		{&pins.Summary{OK: 8, Drift: 3}, "3 drift"},
	}
	for _, c := range cases {
		if got := pinsCell(c.in); got != c.want {
			t.Fatalf("pinsCell(%+v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRenderTableHasThePinsColumn(t *testing.T) {
	rows := []Row{{Alias: "h1", Class: "up-to-date", Pins: &pins.Summary{Drift: 2}}, {Alias: "h2", Class: "up-to-date"}}
	out := renderTable(rows, time.Now())
	lines := strings.Split(out, "\n")
	if !strings.Contains(lines[0], "PINS") {
		t.Fatalf("header lacks PINS: %q", lines[0])
	}
	if !strings.Contains(out, "2 drift") {
		t.Fatalf("table lacks the drift cell:\n%s", out)
	}
}

func TestRenderJSONCarriesPins(t *testing.T) {
	rows := []Row{{Alias: "h1", Class: "up-to-date", Pins: &pins.Summary{OK: 1, Drift: 2}}, {Alias: "h2", Class: "up-to-date"}}
	var got []map[string]any
	if err := json.Unmarshal([]byte(renderJSON(rows)), &got); err != nil {
		t.Fatal(err)
	}
	byAlias := map[string]map[string]any{}
	for _, g := range got {
		byAlias[g["alias"].(string)] = g
	}
	p, ok := byAlias["h1"]["pins"].(map[string]any)
	if !ok || p["drift"].(float64) != 2 {
		t.Fatalf("h1 pins = %v", byAlias["h1"]["pins"])
	}
	if _, has := byAlias["h2"]["pins"]; has {
		t.Fatal("a host without a pins answer must omit the field, not report zeros")
	}
}
