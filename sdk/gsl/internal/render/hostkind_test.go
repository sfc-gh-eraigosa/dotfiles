package render

import "testing"

func TestClassifyHost(t *testing.T) {
	cases := []struct {
		name string
		p    hostProbe
		want string
	}{
		{"darwin is macos", hostProbe{GOOS: "darwin"}, hostKindMacOS},
		{"wsl kernel", hostProbe{GOOS: "linux", ProcVersion: "Linux version 5.15.167.4-microsoft-standard-WSL2 (root@...) #1 SMP"}, hostKindWSL},
		{"raspberry pi device tree", hostProbe{GOOS: "linux", DeviceTreeModel: "Raspberry Pi 4 Model B Rev 1.4\x00"}, hostKindRPi},
		{"jetson device tree", hostProbe{GOOS: "linux", DeviceTreeModel: "NVIDIA Jetson Nano Developer Kit\x00"}, hostKindJetson},
		{"dgx by dmi vendor", hostProbe{GOOS: "linux", DMIVendor: "NVIDIA", DMIProduct: "DGX Spark"}, hostKindDGX},
		{"dgx by dmi product", hostProbe{GOOS: "linux", DMIVendor: "Some OEM", DMIProduct: "NVIDIA DGX Station"}, hostKindDGX},
		{"container wins over host hardware", hostProbe{GOOS: "linux", InContainer: true, DMIVendor: "NVIDIA", DMIProduct: "DGX Spark"}, hostKindContainer},
		{"container wins over wsl", hostProbe{GOOS: "linux", InContainer: true, ProcVersion: "Linux version 5.15-microsoft-standard-WSL2"}, hostKindContainer},
		{"plain linux", hostProbe{GOOS: "linux", ProcVersion: "Linux version 6.8.0-generic", DMIVendor: "GIGABYTE", DMIProduct: "B650 AORUS"}, hostKindLinux},
		{"empty probe", hostProbe{}, hostKindLinux},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyHost(tc.p); got != tc.want {
				t.Errorf("classifyHost(%+v) = %q; want %q", tc.p, got, tc.want)
			}
		})
	}
}

func TestValidHostKind(t *testing.T) {
	for _, k := range []string{hostKindMacOS, hostKindWSL, hostKindRPi, hostKindJetson, hostKindDGX, hostKindContainer, hostKindLinux} {
		if !validHostKind(k) {
			t.Errorf("validHostKind(%q) = false; want true", k)
		}
	}
	for _, k := range []string{"", "windows", "MACOS", "pi"} {
		if validHostKind(k) {
			t.Errorf("validHostKind(%q) = true; want false", k)
		}
	}
}

// TestHostGlyph pins the icon resolution order: an explicit override beats the
// per-kind icon, which beats the generic host icon.
func TestHostGlyph(t *testing.T) {
	st := emojiStyleFixture()
	if got := hostGlyph(st, hostKindDGX, ""); got != "🧠" {
		t.Errorf("dgx glyph = %q; want 🧠", got)
	}
	if got := hostGlyph(st, hostKindLinux, ""); got != "💻" {
		t.Errorf("linux glyph = %q; want 💻", got)
	}
	if got := hostGlyph(st, "no-such-kind", ""); got != "💻" {
		t.Errorf("unknown kind should fall back to the generic host glyph, got %q", got)
	}
	if got := hostGlyph(st, hostKindDGX, "🎮"); got != "🎮" {
		t.Errorf("override glyph = %q; want 🎮", got)
	}
}
