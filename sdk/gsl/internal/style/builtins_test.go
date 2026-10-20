package style_test

import (
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
	sdktheme "github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/theme"
)

// approxLuminance returns an approximate luminance (0–255) for an ANSI-256
// color index. It is used by the palette-bounds test to enforce that every
// segment-key color is mid-luminance — legible as bare foreground on both
// light and dark terminal backgrounds (required because the emoji style renders
// color as fg-tint only, with no background block for contrast — EMOJI-F2-02).
//
// Luminance formula: 0.2126R + 0.7152G + 0.0722B
// For the 6×6×6 cube: levels map [0..5] to approximate sRGB [0,95,135,175,215,255].
// System colors 0–7 and 8–15 are not in the cube; their luminance is
// estimated from conventional dark/bright terminal palette ranges.
// Grayscale ramp 232–255: evenly spaced 8–238.
func approxLuminance(idx int) float64 {
	// Grayscale ramp: 232=dark (≈8) to 255=bright (≈238).
	if idx >= 232 && idx <= 255 {
		v := float64(8 + (idx-232)*10)
		return 0.2126*v + 0.7152*v + 0.0722*v // all channels equal
	}
	// 6×6×6 color cube: 16–231.
	if idx >= 16 && idx <= 231 {
		n := idx - 16
		b := n % 6
		g := (n / 6) % 6
		r := n / 36
		levels := [6]float64{0, 95, 135, 175, 215, 255}
		R, G, B := levels[r], levels[g], levels[b]
		return 0.2126*R + 0.7152*G + 0.0722*B
	}
	// System colors 0–7 (dark) and 8–15 (bright): rough estimates.
	if idx < 8 {
		return float64(idx * 30) // very rough: 0=0, 7=210
	}
	return float64((idx-8)*20 + 50) // bright: 8≈50 to 15≈190
}

// isColorMidLuminance returns true if the color value (a named color string or
// decimal ANSI-256 index string) is mid-luminance by the approxLuminance metric.
// The acceptable range is [30, 220] out of 255.
func isColorMidLuminance(value string) (bool, string) {
	value = strings.TrimSpace(value)

	// Named colors → use their ANSI index approximation.
	namedIndexes := map[string]int{
		"black": 0, "red": 1, "green": 2, "yellow": 3,
		"blue": 4, "magenta": 5, "cyan": 6, "white": 7,
	}
	if idx, ok := namedIndexes[value]; ok {
		lum := approxLuminance(idx)
		if lum < 30 || lum > 220 {
			return false, "named color lum=" + strconv.FormatFloat(lum, 'f', 1, 64)
		}
		return true, ""
	}

	// Decimal ANSI-256 index.
	n, err := strconv.Atoi(value)
	if err != nil {
		return false, "not a decimal index or named color: " + value
	}
	lum := approxLuminance(n)
	if lum < 30 || lum > 220 {
		return false, "lum=" + strconv.FormatFloat(lum, 'f', 1, 64) + " out of [30,220]"
	}
	return true, ""
}

// TestPalette_AllPalettesExist verifies every expected palette name is registered.
func TestPalette_AllPalettesExist(t *testing.T) {
	for _, name := range []string{"dark", "light", "dark-daltonism", "dark8"} {
		p, ok := style.Palette(name)
		if !ok {
			t.Errorf("Palette(%q) not found", name)
			continue
		}
		if len(p) == 0 {
			t.Errorf("Palette(%q) returned empty map", name)
		}
	}
}

// TestPalette_UnknownPalette verifies Palette returns ok=false for unknown names.
func TestPalette_UnknownPalette(t *testing.T) {
	_, ok := style.Palette("nonexistent")
	if ok {
		t.Error("Palette(\"nonexistent\") returned ok=true, want false")
	}
}

// TestPalette_ReturnsCopy verifies that mutating the returned map does not
// affect subsequent calls.
func TestPalette_ReturnsCopy(t *testing.T) {
	p1, _ := style.Palette("dark")
	p1["repo_root"] = "MUTATED"

	p2, _ := style.Palette("dark")
	if p2["repo_root"] == "MUTATED" {
		t.Error("Palette(\"dark\") shares map reference; expected defensive copy")
	}
}

// TestPalette_AllSegmentKeysPresent verifies every palette defines all five
// segment color keys (EMOJI-F2-01): repo_root, repo_worktree, ai, dirgit, time.
func TestPalette_AllSegmentKeysPresent(t *testing.T) {
	segKeys := style.SegmentColorKeys()
	for _, palName := range []string{"dark", "light", "dark-daltonism", "dark8"} {
		p, ok := style.Palette(palName)
		if !ok {
			t.Errorf("palette %q not found; skipping key check", palName)
			continue
		}
		for _, key := range segKeys {
			if v, present := p[key]; !present || v == "" {
				t.Errorf("palette %q missing or empty key %q", palName, key)
			}
		}
	}
}

// TestPalette_MidLuminance is the emoji fg-legibility guarantee (EMOJI-F2-02):
// every palette's five segment-key color indices must be mid-luminance
// (approx luminance 30–220/255) so they are readable as bare fg on both
// light and dark terminal backgrounds.
//
// The emoji style renders color as fg-tint only (Fill:false) — there is no
// background block for contrast. Detection yields a palette NAME, not the
// terminal's background luminance, so all palette indices must be safe on
// both background colors.
func TestPalette_MidLuminance(t *testing.T) {
	segKeys := style.SegmentColorKeys()
	for _, palName := range []string{"dark", "light", "dark-daltonism", "dark8"} {
		p, ok := style.Palette(palName)
		if !ok {
			t.Errorf("palette %q not found", palName)
			continue
		}
		for _, key := range segKeys {
			v, present := p[key]
			if !present {
				continue // TestPalette_AllSegmentKeysPresent catches missing keys
			}
			ok, reason := isColorMidLuminance(v)
			if !ok {
				t.Errorf("palette %q key %q value %q fails mid-luminance: %s",
					palName, key, v, reason)
			}
		}
	}
}

// TestSegmentColorKeys verifies the canonical key list.
func TestSegmentColorKeys(t *testing.T) {
	keys := style.SegmentColorKeys()
	want := []string{"repo_root", "repo_worktree", "ai", "dirgit", "time", "video", "host"}
	if len(keys) != len(want) {
		t.Fatalf("SegmentColorKeys(): len=%d, want %d", len(keys), len(want))
	}
	for i, k := range want {
		if keys[i] != k {
			t.Errorf("SegmentColorKeys()[%d]: got %q, want %q", i, keys[i], k)
		}
	}
}

// TestHostIcons pins the host segment glyphs: the generic "host" icon in every
// style (including the ascii fallback) and the per-kind emoji in both built-in
// styles. The per-kind icons are emoji on purpose — the user asked for the
// computer emoji even under the Nerd Font style.
func TestHostIcons(t *testing.T) {
	wantKinds := map[string]string{
		"host":           "💻",
		"host_macos":     "🍎",
		"host_wsl":       "🪟",
		"host_rpi":       "🍓",
		"host_jetson":    "🤖",
		"host_dgx":       "🧠",
		"host_container": "📦",
	}
	for _, name := range []string{"powerline", "emoji"} {
		st := style.Resolve(io.Discard, name, nil, false)
		for key, want := range wantKinds {
			if got := st.Icons[key]; got != want {
				t.Errorf("style %q icon %q = %q; want %q", name, key, got, want)
			}
		}
	}
	ascii := style.Resolve(io.Discard, "powerline", nil, true)
	if got := ascii.Icons["host"]; got != "[host]" {
		t.Errorf("ascii host icon = %q; want [host]", got)
	}
	if _, has := ascii.Icons["host_dgx"]; has {
		t.Error("ascii mode must not carry per-kind emoji icons")
	}
}

// gsl's segment colors are the sdk's shared roles (libs/tui/theme), so a
// palette name looks the same on the status line as in gff, fleet, mono and
// gdash. dark8 is spelled with color names, so no 256-color escape reaches
// an 8-color terminal.
func TestPalette_SegmentsAreSharedRoles(t *testing.T) {
	names := map[string]string{"0": "black", "1": "red", "2": "green", "3": "yellow", "4": "blue", "5": "magenta", "6": "cyan", "7": "white"}
	for _, palName := range sdktheme.Names() {
		p, ok := style.Palette(palName)
		if !ok {
			t.Fatalf("palette %q missing", palName)
		}
		r, _ := sdktheme.Get(palName)
		want := map[string]string{
			"repo_root": r.Info, "repo_worktree": r.Accent, "ai": r.SeriesColor(3),
			"dirgit": r.OK, "time": r.Warn, "video": r.Crit, "host": r.SeriesColor(2),
		}
		for key, w := range want {
			if palName == sdktheme.Dark8 {
				w = names[w]
			}
			if p[key] != w {
				t.Errorf("%s.%s = %q, want shared %q", palName, key, p[key], w)
			}
		}
		if palName == sdktheme.Dark8 {
			continue // eight colors: worktree and host share magenta, as before
		}
		seen := map[string]string{}
		for _, key := range style.SegmentColorKeys() {
			if other, dup := seen[p[key]]; dup {
				t.Errorf("%s: %s and %s are both %q", palName, other, key, p[key])
			}
			seen[p[key]] = key
		}
	}
}
