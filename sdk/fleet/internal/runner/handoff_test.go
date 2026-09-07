package runner

import (
	"reflect"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/pkg/provider"
)

func remote(cmd string) provider.Handoff {
	return provider.Handoff{Kind: provider.HandoffRemote, Command: cmd}
}

func local(argv ...string) provider.Handoff {
	return provider.Handoff{Kind: provider.HandoffLocal, Argv: argv}
}

// F2a: a remote handoff owns the terminal, so it is `ssh -t` with the SAME
// multiplexing options every batch lane uses (the socket it authenticates is
// the one later probes ride) and NO BatchMode (the whole point is to let ssh
// prompt). Extends TestEveryRemotePathCarriesTheMuxOptions to the new lane.
func TestEveryHandoffCarriesTheMuxOptions(t *testing.T) {
	got, err := HandoffArgv("spark", remote("~/.local/bin/herdr attach"))
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != "ssh" {
		t.Fatalf("argv[0] = %q, want ssh", got[0])
	}
	if !contains(got, "-t") {
		t.Fatalf("argv %q lacks -t", got)
	}
	mux := MuxArgs()
	for i := 0; i+1 < len(mux); i += 2 {
		if !hasPair(got, mux[i], mux[i+1]) {
			t.Fatalf("argv %q lacks mux option %q %q", got, mux[i], mux[i+1])
		}
	}
	if hasPair(got, "-o", "BatchMode=yes") {
		t.Fatalf("a handoff must not be BatchMode (it must be allowed to prompt): %q", got)
	}
	// The remote command is the last element, verbatim, after the host.
	if got[len(got)-1] != "~/.local/bin/herdr attach" || got[len(got)-2] != "spark" {
		t.Fatalf("argv should end with host then command, got %q", got)
	}
	// And it is exactly ssh + InteractiveArgs(alias) + command: one spelling.
	want := append(append([]string{"ssh"}, InteractiveArgs("spark")...), "~/.local/bin/herdr attach")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("remote handoff argv = %q, want %q", got, want)
	}
}

// F2b: a local handoff execs argv[0] directly. A hostile element — a
// substitution, a semicolon, a quote — survives verbatim as ONE argv element
// because there is no shell anywhere to interpret it.
func TestLocalHandoffNeverInvokesAShell(t *testing.T) {
	hostile := `$(rm -rf ~); echo 'x'`
	got, err := HandoffArgv("spark", local("herdr", "--remote", "spark", "--session", hostile))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"herdr", "--remote", "spark", "--session", hostile}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("local handoff argv = %q, want the argv verbatim %q", got, want)
	}
	for _, el := range got {
		if el == "sh" || el == "-c" || el == "ssh" {
			t.Fatalf("a local handoff must never route through a shell or ssh: %q", got)
		}
	}
	c, err := Command("spark", local("herdr", "--remote", "spark", "--session", hostile))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.Args, want) {
		t.Fatalf("Command args = %q, want %q", c.Args, want)
	}
}

// F2c: Quote is the one quoting path for a provider value entering a remote
// command string. The cases are the ones that break naive quoting.
func TestRemoteHandoffQuotesEveryProviderSuppliedValue(t *testing.T) {
	cases := map[string]string{
		"plain":       "'plain'",
		"it's":        `'it'\''s'`,
		"$(evil)":     "'$(evil)'",
		"a b":         "'a b'",
		"":            "''",
		"line\nbreak": "'line\nbreak'",
		"`tick`":      "'`tick`'",
		`back\slash`:  `'back\slash'`,
		"two''quotes": `'two'\'''\''quotes'`,
	}
	for in, want := range cases {
		if got := Quote(in); got != want {
			t.Fatalf("Quote(%q) = %s, want %s", in, got, want)
		}
	}
	// A quoted value is inert inside a remote command: the substitution text
	// is present but wrapped, so the remote shell sees a literal.
	cmd := "herdr --session " + Quote("$(evil)") + " api snapshot"
	got, err := HandoffArgv("spark", remote(cmd))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got[len(got)-1], "'$(evil)'") {
		t.Fatalf("quoted value did not survive into the command: %q", got[len(got)-1])
	}
}

// F2d: the machine an action runs against is the alias FLEET passes, and
// nothing inside the Handoff can move it — the payload has no host field, so
// the same handoff dispatched to two aliases differs only at the host slot.
func TestTheAliasComesFromFleetNotTheProvider(t *testing.T) {
	h := remote("herdr attach")
	a, err := HandoffArgv("spark", h)
	if err != nil {
		t.Fatal(err)
	}
	b, err := HandoffArgv("nano", h)
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != len(b) {
		t.Fatalf("argv lengths differ: %q vs %q", a, b)
	}
	diffs := 0
	for i := range a {
		if a[i] != b[i] {
			diffs++
			if a[i] != "spark" || b[i] != "nano" {
				t.Fatalf("argv differs at a non-host slot: %q vs %q", a[i], b[i])
			}
		}
	}
	if diffs != 1 {
		t.Fatalf("expected exactly one differing element (the alias), got %d: %q vs %q", diffs, a, b)
	}
	// A command that mentions another machine is just text after the host.
	sneaky, err := HandoffArgv("spark", remote("ssh nano herdr attach"))
	if err != nil {
		t.Fatal(err)
	}
	if sneaky[len(sneaky)-2] != "spark" {
		t.Fatalf("host slot must be the dispatched alias, got %q", sneaky)
	}
}

// Empty inputs are refused before anything could become a process.
func TestHandoffArgvRefusesEmptyInputs(t *testing.T) {
	bad := []struct {
		alias string
		h     provider.Handoff
	}{
		{"", remote("x")},
		{"spark", remote("")},
		{"spark", remote("   ")},
		{"spark", local()},
		{"spark", local("")},
		{"spark", provider.Handoff{Kind: "weird", Command: "x"}},
	}
	for i, c := range bad {
		if _, err := HandoffArgv(c.alias, c.h); err == nil {
			t.Fatalf("case %d: expected an error for alias=%q handoff=%#v", i, c.alias, c.h)
		}
		if _, err := Command(c.alias, c.h); err == nil {
			t.Fatalf("case %d: Command should refuse what HandoffArgv refuses", i)
		}
	}
}

// InteractiveArgs is the promoted interactiveArgs: -t, the mux options, the
// host, and NO BatchMode or ConnectTimeout (an interactive session may wait
// for a human).
func TestInteractiveArgsIsTheTerminalOwningLane(t *testing.T) {
	got := InteractiveArgs("spark")
	if got[0] != "-t" || got[len(got)-1] != "spark" {
		t.Fatalf("InteractiveArgs = %q, want -t … spark", got)
	}
	if hasPair(got, "-o", "BatchMode=yes") {
		t.Fatalf("interactive lane must not be BatchMode: %q", got)
	}
	if !reflect.DeepEqual(got, Exec{}.interactiveArgs("spark")) {
		t.Fatalf("InteractiveArgs must be the same spelling Exec uses: %q vs %q", got, Exec{}.interactiveArgs("spark"))
	}
}

func contains(argv []string, s string) bool {
	for _, a := range argv {
		if a == s {
			return true
		}
	}
	return false
}
