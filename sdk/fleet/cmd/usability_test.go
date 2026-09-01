package cmd

import (
	"strings"
	"testing"
	"time"
)

// Running `fleet` bare should open the dashboard, not print help. Help stays
// reachable explicitly.
func TestBareFleetRunsTheTUI(t *testing.T) {
	if rootCmd.RunE == nil && rootCmd.Run == nil {
		t.Fatal("root command has no action — bare `fleet` would print help")
	}
	if rootCmd.Args == nil {
		t.Fatal("root command must constrain its args so a typo'd subcommand still errors")
	}
}

// A mistyped subcommand must NOT silently open the dashboard — that would hide
// the typo behind a working-looking UI.
func TestUnknownSubcommandIsStillAnError(t *testing.T) {
	if err := rootCmd.Args(rootCmd, []string{"nosuchthing"}); err == nil {
		t.Fatal("an unknown subcommand must be rejected, not treated as a TUI launch")
	}
}

// The TUI list must be ordered by host name and STAY there. Ordering by
// severity means a row jumps position the moment its class changes while
// streaming — the cursor survives (it is alias-keyed) but the list moves under
// the operator's eyes, which is the bug being fixed.
func TestTUIOrderIsStableByAlias(t *testing.T) {
	rows := []Row{
		{Alias: "charlie", Class: "up-to-date"},
		{Alias: "alpha", Class: "unreachable"},
		{Alias: "bravo", Class: "behind"},
	}
	sortByAlias(rows)
	got := []string{rows[0].Alias, rows[1].Alias, rows[2].Alias}
	want := []string{"alpha", "bravo", "charlie"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// The ordering must not depend on class at all, or it will still shuffle as
// rows resolve.
func TestTUIOrderDoesNotMoveWhenAClassChanges(t *testing.T) {
	m := newTUIModel(nil, nil, nil, time.Time{}, "", 1)
	for _, a := range []string{"alpha", "bravo", "charlie"} {
		m.setRow(Row{Alias: a, Class: "polling"})
	}
	m.resort()
	before := []string{m.rows[0].Alias, m.rows[1].Alias, m.rows[2].Alias}

	// bravo resolves to the worst possible class — under severity ordering it
	// would jump to the top.
	m.setRow(Row{Alias: "bravo", Class: "unreachable"})
	m.resort()
	after := []string{m.rows[0].Alias, m.rows[1].Alias, m.rows[2].Alias}

	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("order moved when a class changed: %v -> %v", before, after)
		}
	}
}

// `fleet status` is a one-shot report with no re-sorting problem, so it keeps
// worst-first: there, surfacing the broken hosts at the top is the whole point.
func TestStatusTableStillLeadsWithTheWorstHost(t *testing.T) {
	out := renderTable([]Row{
		{Alias: "zulu", Class: "up-to-date"},
		{Alias: "alpha", Class: "unreachable"},
	}, testNow)
	if strings.Index(out, "alpha") > strings.Index(out, "zulu") {
		t.Fatalf("status table must stay worst-first:\n%s", out)
	}
}
