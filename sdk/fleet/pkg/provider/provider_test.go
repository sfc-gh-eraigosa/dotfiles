package provider

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// fullNode is a Node exercising every field and every action kind, so a
// round-trip test cannot pass by accident of an omitted field.
func fullNode() Node {
	return Node{
		ID:     "default",
		Kind:   "herdr-session",
		Cells:  []string{"default", "running", "2", "~/.config/herdr"},
		Detail: "default session",
		Leaf:   false,
		Attrs:  map[string]string{"binary": "/home/u/.local/bin/herdr"},
		Actions: []Action{
			{Key: "c", Label: "attach", Handoff: &Handoff{Kind: HandoffLocal, Argv: []string{"herdr", "--remote", "spark"}}},
			{Key: "x", Label: "shell", Unavailable: "local client speaks 20, host serves 19", Handoff: &Handoff{Kind: HandoffRemote, Command: "herdr attach"}},
			{Key: "l", Label: "logs", Stream: &Stream{Command: "herdr logs -f", Follow: true}},
			{Key: "t", Label: "bridge", Tunnel: &Tunnel{RemotePort: 3080, LocalPort: 0, Scheme: "http", Keeper: "kubectl port-forward svc/x 3080:3080"}},
		},
	}
}

// F1c: every contract type survives JSON unchanged, including nil maps and
// slices, and the wire spelling is the one the TUI, the CLI and the plugin
// protocol all read — no adapter between them.
func TestEveryContractTypeRoundTripsThroughJSON(t *testing.T) {
	for name, in := range map[string]Node{
		"full":                  fullNode(),
		"nil attrs and actions": {ID: "x", Kind: "k"},
		"zero cells":            {ID: "x", Kind: "k", Cells: []string{}},
	} {
		t.Run(name, func(t *testing.T) {
			b, err := json.Marshal(in)
			if err != nil {
				t.Fatal(err)
			}
			var out Node
			if err := json.Unmarshal(b, &out); err != nil {
				t.Fatal(err)
			}
			// A nil slice and an empty slice are the same row on screen and
			// on the wire; normalise before comparing so "zero cells" does
			// not fail on a distinction no consumer can see.
			if len(in.Cells) == 0 && len(out.Cells) == 0 {
				in.Cells, out.Cells = nil, nil
			}
			if !reflect.DeepEqual(in, out) {
				t.Fatalf("round trip changed the node:\n in=%#v\nout=%#v", in, out)
			}
		})
	}

	b, _ := json.Marshal(fullNode())
	s := string(b)
	for _, want := range []string{`"key":"c"`, `"unavailable":""`, `"handoff":`, `"stream":null`, `"tunnel":null`, `"remotePort":3080`, `"localPort":0`, `"keeper":"kubectl port-forward svc/x 3080:3080"`, `"exitCode"`} {
		if want == `"exitCode"` {
			eb, _ := json.Marshal(ExecResult{Stdout: "o", Stderr: "e", ExitCode: 3})
			if !strings.Contains(string(eb), `"exitCode":3`) {
				t.Fatalf("ExecResult wire shape wrong: %s", eb)
			}
			continue
		}
		if !strings.Contains(s, want) {
			t.Fatalf("wire JSON lacks %s:\n%s", want, s)
		}
	}
}

// F1b: an Action is exactly one of Handoff | Stream | Tunnel. Two, three or
// none is refused at validation so nothing downstream ever has to pick.
func TestAnActionMustCarryExactlyOneOfHandoffStreamOrTunnel(t *testing.T) {
	h := &Handoff{Kind: HandoffLocal, Argv: []string{"herdr"}}
	s := &Stream{Command: "tail -f x"}
	u := &Tunnel{RemotePort: 80}
	cases := []struct {
		name string
		a    Action
		ok   bool
	}{
		{"handoff only", Action{Key: "c", Label: "l", Handoff: h}, true},
		{"stream only", Action{Key: "c", Label: "l", Stream: s}, true},
		{"tunnel only", Action{Key: "t", Label: "l", Tunnel: u}, true},
		{"none", Action{Key: "c", Label: "l"}, false},
		{"handoff+stream", Action{Key: "c", Label: "l", Handoff: h, Stream: s}, false},
		{"stream+tunnel", Action{Key: "c", Label: "l", Stream: s, Tunnel: u}, false},
		{"handoff+tunnel", Action{Key: "c", Label: "l", Handoff: h, Tunnel: u}, false},
		{"all three", Action{Key: "c", Label: "l", Handoff: h, Stream: s, Tunnel: u}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.a.Validate()
			if c.ok && err != nil {
				t.Fatalf("expected valid, got %v", err)
			}
			if !c.ok && err == nil {
				t.Fatal("expected a validation error, got nil")
			}
		})
	}
}

// The payloads themselves are validated too: a remote handoff needs a
// command, a local one an argv, a stream a command.
func TestActionPayloadsAreValidated(t *testing.T) {
	bad := []Action{
		{Key: "c", Label: "l", Handoff: &Handoff{Kind: HandoffRemote}},                        // no command
		{Key: "c", Label: "l", Handoff: &Handoff{Kind: HandoffLocal}},                         // no argv
		{Key: "c", Label: "l", Handoff: &Handoff{Kind: "weird", Command: "x"}},                // unknown kind
		{Key: "c", Label: "l", Handoff: &Handoff{Kind: HandoffLocal, Argv: []string{""}}},     // empty argv[0]
		{Key: "c", Label: "l", Stream: &Stream{}},                                             // no command
		{Key: "c", Label: "", Handoff: &Handoff{Kind: HandoffLocal, Argv: []string{"herdr"}}}, // no label
	}
	for i, a := range bad {
		if err := a.Validate(); err == nil {
			t.Fatalf("case %d: expected a validation error for %#v", i, a)
		}
	}
}

// F1d: ports are validated at the boundary so BridgeArgv never sees garbage.
func TestATunnelWithAnOutOfRangePortIsRejected(t *testing.T) {
	cases := []struct {
		name string
		tn   Tunnel
		ok   bool
	}{
		{"remote 0", Tunnel{RemotePort: 0}, false},
		{"remote 65536", Tunnel{RemotePort: 65536}, false},
		{"local 65536", Tunnel{RemotePort: 80, LocalPort: 65536}, false},
		{"local negative", Tunnel{RemotePort: 80, LocalPort: -1}, false},
		{"local 0 allocates", Tunnel{RemotePort: 80, LocalPort: 0}, true},
		{"explicit local", Tunnel{RemotePort: 80, LocalPort: 8080}, true},
		{"with keeper", Tunnel{RemotePort: 80, Keeper: "kubectl port-forward svc/g 80:80"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Action{Key: "t", Label: "bridge", Tunnel: &c.tn}.Validate()
			if c.ok && err != nil {
				t.Fatalf("expected valid, got %v", err)
			}
			if !c.ok && err == nil {
				t.Fatal("expected a validation error, got nil")
			}
		})
	}
}

// The escape the callId rule closes for exec must be unrepresentable on the
// action path too: no payload can name a machine or an address. Reflection,
// so a field added later fails this test rather than a review.
func TestNoActionPayloadCarriesAHostOrAddress(t *testing.T) {
	for _, v := range []any{Handoff{}, Stream{}, Tunnel{}} {
		rt := reflect.TypeOf(v)
		for i := 0; i < rt.NumField(); i++ {
			n := strings.ToLower(rt.Field(i).Name)
			for _, bad := range []string{"host", "addr", "alias", "target", "peer"} {
				if strings.Contains(n, bad) {
					t.Fatalf("%s has field %q — an action payload must not name a machine", rt.Name(), rt.Field(i).Name)
				}
			}
		}
	}
}

// F1e: a provider cannot claim a key fleet binds inside a level, or one the
// dashboard deliberately leaves unbound there; the collision is reported at
// validation, naming the key, instead of as a key that never fires.
func TestAReservedKeyIsRejectedAtValidation(t *testing.T) {
	ok := &Handoff{Kind: HandoffLocal, Argv: []string{"herdr"}}
	for _, k := range []string{"r", "t", "T", "q", "/", "n", "N", "j", "k", "g", "G", "u", "w", "v", "a", "p", "P", "A", "F", " "} {
		err := Action{Key: k, Label: "l", Handoff: ok}.Validate()
		if err == nil {
			t.Fatalf("key %q is reserved but was accepted", k)
		}
		if !strings.Contains(err.Error(), k) {
			t.Fatalf("error for reserved key %q does not name it: %v", k, err)
		}
		if !ReservedKeys[[]rune(k)[0]] {
			t.Fatalf("ReservedKeys does not list %q", k)
		}
	}
	for _, k := range []string{"c", "l", "d", "e", "x", "b"} {
		if err := (Action{Key: k, Label: "l", Handoff: ok}).Validate(); err != nil {
			t.Fatalf("key %q should be free: %v", k, err)
		}
	}
	for _, k := range []string{"", "ab", "\x01", "é"} {
		// empty, two runes, non-printable, non-ASCII: not a single printable key
		if k == "é" {
			// a printable non-ASCII rune IS one printable rune; allowed
			if err := (Action{Key: k, Label: "l", Handoff: ok}).Validate(); err != nil {
				t.Fatalf("key %q is one printable rune and should be accepted: %v", k, err)
			}
			continue
		}
		if err := (Action{Key: k, Label: "l", Handoff: ok}).Validate(); err == nil {
			t.Fatalf("key %q is not one printable rune but was accepted", k)
		}
	}
}

// `t` is fleet's tunnel key, the way enter is its drill key: a Tunnel action
// must declare it (so the TUI's `t` and the CLI agree on which action a row's
// bridge is), and no other kind may take it.
func TestATunnelActionUsesFleetsTunnelKey(t *testing.T) {
	if err := (Action{Key: "t", Label: "bridge", Tunnel: &Tunnel{RemotePort: 80}}).Validate(); err != nil {
		t.Fatalf("a tunnel keyed t must validate: %v", err)
	}
	if err := (Action{Key: "b", Label: "bridge", Tunnel: &Tunnel{RemotePort: 80}}).Validate(); err == nil {
		t.Fatal("a tunnel with a key other than t was accepted")
	}
	if err := (Action{Key: "t", Label: "shell", Handoff: &Handoff{Kind: HandoffLocal, Argv: []string{"sh"}}}).Validate(); err == nil {
		t.Fatal("a handoff keyed t was accepted")
	}
	if err := (Action{Key: "t", Label: "logs", Stream: &Stream{Command: "tail -f x"}}).Validate(); err == nil {
		t.Fatal("a stream keyed t was accepted")
	}
}

// F1a: a row shorter than its kind's columns renders trailing blanks; a
// longer one is cut to the columns; zero cells never panics.
func TestAShortCellSliceRendersBlanksNotAPanic(t *testing.T) {
	cols := []string{"A", "B", "C"}
	cases := []struct {
		cells []string
		want  []string
	}{
		{nil, []string{"", "", ""}},
		{[]string{}, []string{"", "", ""}},
		{[]string{"x"}, []string{"x", "", ""}},
		{[]string{"x", "y", "z"}, []string{"x", "y", "z"}},
		{[]string{"x", "y", "z", "w"}, []string{"x", "y", "z"}},
	}
	for _, c := range cases {
		got := Node{ID: "id", Kind: "k", Cells: c.cells}.Row(cols)
		if !reflect.DeepEqual(got, c.want) {
			t.Fatalf("cells %v against %v: got %v want %v", c.cells, cols, got, c.want)
		}
	}
	if got := (Node{ID: "id"}).Row(nil); len(got) != 0 {
		t.Fatalf("no columns should render no cells, got %v", got)
	}
}

// A Node's identity is one path segment, and its actions must validate.
func TestANodeIsOnePathSegmentWithValidActions(t *testing.T) {
	if err := (Node{ID: "", Kind: "k"}).Validate(); err == nil {
		t.Fatal("empty ID accepted")
	}
	if err := (Node{ID: "a/b", Kind: "k"}).Validate(); err == nil {
		t.Fatal("ID with a path separator accepted")
	}
	if err := (Node{ID: "a", Kind: "k", Actions: []Action{{Key: "c", Label: "l"}}}).Validate(); err == nil {
		t.Fatal("a node with an invalid action was accepted")
	}
	if err := fullNode().Validate(); err != nil {
		t.Fatalf("the full node should validate: %v", err)
	}
}

// The sentinels a provider returns are distinguishable with errors.Is even
// when wrapped with the reason a row will show.
func TestSentinelsSurviveWrapping(t *testing.T) {
	if !errors.Is(errors.Join(ErrAbsent, errors.New("tried ~/.local/bin/herdr")), ErrAbsent) {
		t.Fatal("ErrAbsent lost in a wrap")
	}
	if !errors.Is(errors.Join(ErrNoSuchPath, errors.New("nope")), ErrNoSuchPath) {
		t.Fatal("ErrNoSuchPath lost in a wrap")
	}
	if errors.Is(ErrAbsent, ErrNoSuchPath) {
		t.Fatal("the sentinels must be distinct")
	}
}
