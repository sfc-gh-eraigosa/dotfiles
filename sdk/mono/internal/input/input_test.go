package input

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/keymap"
)

func describe(evs []Event) string {
	var parts []string
	for _, e := range evs {
		if e.Key != nil {
			parts = append(parts, "key:"+keymap.KeyName(*e.Key))
		} else {
			parts = append(parts, fmt.Sprintf("fwd:%q", e.Forward))
		}
	}
	return strings.Join(parts, " ")
}

// feedAll routes everything, the way the app does when no mode changes.
func feedAll(r *Router, b []byte) []Event {
	var all []Event
	for {
		evs, rest := r.Feed(b)
		all = append(all, evs...)
		if len(rest) == 0 {
			return all
		}
		b = rest
	}
}

func TestRouterStopsAfterAMonoKey(t *testing.T) {
	r := &Router{Prefix: 0x02}
	evs, rest := r.Feed([]byte("ab\x02:config\r"))
	if describe(evs) != `fwd:"ab" key::` || string(rest) != "config\r" {
		t.Fatalf("Feed = %s rest %q", describe(evs), rest)
	}
}

func TestRouterForwardsEverythingButThePrefix(t *testing.T) {
	r := &Router{Prefix: 0x02}
	cases := []struct {
		in   string
		want string
	}{
		{"q/\x1b[A\x1bOP", `fwd:"q/\x1b[A\x1bOP"`}, // htop's own q, /, arrows, F1 untouched
		{"ab\x02jcd", `fwd:"ab" key:j fwd:"cd"`},
		{"\x02\x02", `key:ctrl+b`}, // literal prefix for the program
		{"\x02\x1b[B", `key:down`}, // arrow after the prefix
		{"\x02\x1b", `key:esc`},    // cancel
		{"\x02?", `key:?`},         //
		{"\x02", ``},               // prefix split across reads...
		{"q", `key:q`},             // ...finishes on the next read
		{"\x02\x1bOP", `key:f1`},   //
		{"\x02:", `key:":"`},       //
	}
	for _, tc := range cases {
		got := describe(feedAll(r, []byte(tc.in)))
		want := strings.ReplaceAll(tc.want, `key:":"`, "key::")
		if got != want {
			t.Errorf("Feed(%q) = %s, want %s", tc.in, got, want)
		}
	}
	if r.Pending() {
		t.Fatal("router left pending")
	}
}

func TestKeysDecodesAllKinds(t *testing.T) {
	var got []string
	for _, k := range Keys([]byte("aé \r\t\x7f\x1b[A\x1b[3~\x1b[5~\x03\x1bx\x1b[99Z")) {
		got = append(got, keymap.KeyName(k))
	}
	want := "a é space enter tab backspace up delete pgup ctrl+c alt+x esc"
	if strings.Join(got, " ") != want {
		t.Fatalf("Keys = %q\nwant   %q", strings.Join(got, " "), want)
	}
}

func TestRouterHoldsEscapeSplitAfterPrefix(t *testing.T) {
	r := &Router{Prefix: 0x02}
	got := describe(feedAll(r, []byte("\x02\x1b[")))
	if got != "" || !r.Pending() {
		t.Fatalf("first half: %s pending=%v", got, r.Pending())
	}
	if got := describe(feedAll(r, []byte("Bxy"))); got != `key:down fwd:"xy"` {
		t.Fatalf("second half: %s", got)
	}
	// A lone Esc after the prefix is still the Esc key at once.
	if got := describe(feedAll(r, []byte("\x02\x1b"))); got != "key:esc" {
		t.Fatalf("lone esc: %s", got)
	}
}
