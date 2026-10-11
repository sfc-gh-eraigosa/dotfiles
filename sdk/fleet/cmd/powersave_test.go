package cmd

import (
	"strings"
	"testing"
)

// parsePowerSave reads `iw dev <d> get power_save`. "on"/"off" are facts; any
// other answer is "don't know" (ok=false) and must never raise a false alarm.
func TestParsePowerSave(t *testing.T) {
	cases := []struct {
		name    string
		out     string
		wantOn  bool
		wantsOK bool
	}{
		{"on", "Power save: on\n", true, true},
		{"off", "\tPower save: off\n", false, true},
		{"tabbed-on", "\tPower save: on", true, true},
		{"mixed-case", "POWER SAVE: ON", true, true},
		{"unexpected", "command not found", false, false},
		{"empty", "", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			on, ok := parsePowerSave(c.out)
			if on != c.wantOn || ok != c.wantsOK {
				t.Fatalf("parsePowerSave(%q) = (%v,%v), want (%v,%v)", c.out, on, ok, c.wantOn, c.wantsOK)
			}
		})
	}
}

// powerSaveNote speaks only when a named interface is on. Off, or no interface,
// is silence — the common case must add nothing to the output.
func TestPowerSaveNoteOnlyFiresWhenARadioIsAsleep(t *testing.T) {
	if got := powerSaveNote("wlan0", false); got != "" {
		t.Fatalf("power save off must be silent, got:\n%s", got)
	}
	if got := powerSaveNote("", true); got != "" {
		t.Fatalf("no interface must be silent, got:\n%s", got)
	}
	got := powerSaveNote("wlan0", true)
	for _, want := range []string{"wlan0", "power_save off", "wifi.powersave = 2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("note missing %q:\n%s", want, got)
		}
	}
}
