package pins

import "testing"

func TestExtractVersion(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Client Version: v1.37.0", "1.37.0"},
		{"v4.2.4+g3900f43", "4.2.4"},
		{"kind v0.33.0 go1.26.7 linux/arm64", "0.33.0"},
		{"sops 3.13.1", "3.13.1"},
		{"yq (https://github.com/mikefarah/yq/) version v4.44.6", "4.44.6"},
		{"version is set by build process", ""},
		{"uv 0.12.16 (aarch64-unknown-linux-gnu)", "0.12.16"},
		{"glow version 3.0.0 (add5e7a)", "3.0.0"},
		{"rustc 1.98.1 (48a229cea 2026-09-01)", "1.98.1"},
		{"keyd v2.6.0 (7c0aecb)", "2.6.0"},
		{"herdr 0.9.3", "0.9.3"},
		{"go version go1.26.1 linux/arm64", "1.26.1"},
		{"tool 2.5", "2.5"},
		{"", ""},
	}
	for _, c := range cases {
		if got := ExtractVersion(c.in); got != c.want {
			t.Errorf("ExtractVersion(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.2.3", "1.2.3", 0},
		{"1.2", "1.2.0", 0},
		{"1.2.4", "1.2.3", 1},
		{"1.10.0", "1.9.9", 1},
		{"0.9", "1.0.0", -1},
		{"v1.2.3", "1.2.3", 0},
		{"2", "1.99.99", 1},
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestGap(t *testing.T) {
	cases := []struct{ pin, latest, want string }{
		{"1.2.3", "1.2.3", "current"},
		{"1.2.3", "1.2.4", "patch"},
		{"1.2.3", "1.3.0", "minor"},
		{"1.2.3", "2.0.0", "major"},
		{"1.2.4", "1.2.3", "ahead"},
		{"1.2.3", "", "unknown"},
		{"", "1.2.3", "unknown"},
		{"junk", "1.2.3", "unknown"},
	}
	for _, c := range cases {
		if got := Gap(c.pin, c.latest); got != c.want {
			t.Errorf("Gap(%q,%q) = %q, want %q", c.pin, c.latest, got, c.want)
		}
	}
}
