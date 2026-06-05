package render

import "testing"

func TestOptBool(t *testing.T) {
	opts := map[string]any{
		"a": true,
		"b": false,
		"c": "true",
		"d": "false",
		"e": "notbool",
		"f": 123, // wrong type
	}
	cases := []struct {
		key  string
		def  bool
		want bool
	}{
		{"a", false, true},
		{"b", true, false},
		{"c", false, true},
		{"d", true, false},
		{"e", true, true}, // unparseable string → default
		{"f", true, true}, // wrong type → default
		{"missing", true, true},
	}
	for _, tc := range cases {
		if got := optBool(opts, tc.key, tc.def); got != tc.want {
			t.Errorf("optBool(%q, def=%v) = %v, want %v", tc.key, tc.def, got, tc.want)
		}
	}
	// nil map → default.
	if got := optBool(nil, "x", true); !got {
		t.Error("optBool(nil) should return default")
	}
}

func TestOptString(t *testing.T) {
	opts := map[string]any{
		"name":  "feature",
		"empty": "",
		"num":   42,
	}
	if got := optString(opts, "name", "def"); got != "feature" {
		t.Errorf("optString name = %q, want feature", got)
	}
	if got := optString(opts, "empty", "def"); got != "def" {
		t.Errorf("optString empty → default, got %q", got)
	}
	if got := optString(opts, "num", "def"); got != "def" {
		t.Errorf("optString wrong-type → default, got %q", got)
	}
	if got := optString(opts, "missing", "def"); got != "def" {
		t.Errorf("optString missing → default, got %q", got)
	}
	if got := optString(nil, "x", "def"); got != "def" {
		t.Errorf("optString nil → default, got %q", got)
	}
}

func TestTokenAbbrev(t *testing.T) {
	cases := []struct {
		v    float64
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1k"},
		{84000, "84k"},
		{200000, "200k"},
		{1_000_000, "1m"},
		{2_500_000, "2m"},
	}
	for _, tc := range cases {
		if got := tokenAbbrev(tc.v); got != tc.want {
			t.Errorf("tokenAbbrev(%v) = %q, want %q", tc.v, got, tc.want)
		}
	}
}

func TestPct(t *testing.T) {
	if got := pct(42.4); got != "42%" {
		t.Errorf("pct(42.4) = %q, want 42%%", got)
	}
	if got := pct(42.6); got != "43%" {
		t.Errorf("pct(42.6) = %q, want 43%%", got)
	}
}
