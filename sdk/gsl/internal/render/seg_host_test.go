package render

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/term"
)

func fixedHost(name string) func() (string, error) {
	return func() (string, error) { return name, nil }
}

func fixedProbe(p hostProbe) func() hostProbe {
	return func() hostProbe { return p }
}

// hostName returns the text after the glyph: "<glyph> <name>" → "<name>".
func hostName(t *testing.T, text string) string {
	t.Helper()
	i := strings.LastIndex(text, " ")
	if i < 0 {
		t.Fatalf("host text %q has no glyph separator", text)
	}
	return text[i+1:]
}

func TestHost_RendersGlyphAndShortName(t *testing.T) {
	seg := NewHostSegment(nil)
	seg.Hostname = fixedHost("spark1.local")
	seg.Probe = fixedProbe(hostProbe{GOOS: "linux"})

	got, colorKey, ok := seg.Render(context.Background(), asciiStyle(), 0)
	if !ok {
		t.Fatal("host: want ok=true")
	}
	if colorKey != "host" {
		t.Errorf("host: want colorKey=host, got %q", colorKey)
	}
	if got != "[host] spark1" {
		t.Errorf("host: want %q, got %q", "[host] spark1", got)
	}
}

func TestHost_TruncatesTo11Columns(t *testing.T) {
	seg := NewHostSegment(nil)
	seg.Hostname = fixedHost("averyveryverylonghostname")
	seg.Probe = fixedProbe(hostProbe{GOOS: "linux"})

	got, _, ok := seg.Render(context.Background(), asciiStyle(), 0)
	if !ok {
		t.Fatal("host: want ok=true")
	}
	name := hostName(t, got)
	if w := term.DisplayWidth(name); w != 11 {
		t.Errorf("host name %q is %d columns; want exactly 11", name, w)
	}
	if !strings.HasSuffix(name, ellipsis) {
		t.Errorf("host name %q should end with an ellipsis", name)
	}
	if !strings.HasPrefix(name, "averyveryv") {
		t.Errorf("host name %q should keep the first 10 characters", name)
	}
}

func TestHost_ExactlyElevenIsNotCut(t *testing.T) {
	seg := NewHostSegment(nil)
	seg.Hostname = fixedHost("elevenchars")
	seg.Probe = fixedProbe(hostProbe{GOOS: "linux"})

	got, _, _ := seg.Render(context.Background(), asciiStyle(), 0)
	if name := hostName(t, got); name != "elevenchars" {
		t.Errorf("11-char name must render untouched, got %q", name)
	}
}

func TestHost_MaxLenOption(t *testing.T) {
	seg := NewHostSegment(map[string]any{"max_len": float64(5)})
	seg.Hostname = fixedHost("longhostname")
	seg.Probe = fixedProbe(hostProbe{GOOS: "linux"})

	got, _, _ := seg.Render(context.Background(), asciiStyle(), 0)
	name := hostName(t, got)
	if w := term.DisplayWidth(name); w != 5 {
		t.Errorf("max_len=5: name %q is %d columns; want 5", name, w)
	}
}

func TestHost_HostnameErrorSelfOmits(t *testing.T) {
	seg := NewHostSegment(nil)
	seg.Hostname = func() (string, error) { return "", errors.New("boom") }
	seg.Probe = fixedProbe(hostProbe{GOOS: "linux"})

	if _, _, ok := seg.Render(context.Background(), asciiStyle(), 0); ok {
		t.Error("host: want ok=false when the hostname lookup fails")
	}
}

func TestHost_EmptyHostnameSelfOmits(t *testing.T) {
	seg := NewHostSegment(nil)
	seg.Hostname = fixedHost("")
	seg.Probe = fixedProbe(hostProbe{GOOS: "linux"})

	if _, _, ok := seg.Render(context.Background(), asciiStyle(), 0); ok {
		t.Error("host: want ok=false for an empty hostname")
	}
}

func TestHost_KindPicksIcon(t *testing.T) {
	seg := NewHostSegment(nil)
	seg.Hostname = fixedHost("spark1")
	seg.Probe = fixedProbe(hostProbe{GOOS: "linux", DMIVendor: "NVIDIA", DMIProduct: "DGX Spark"})

	got, _, _ := seg.Render(context.Background(), emojiStyleFixture(), 0)
	if got != "🧠 spark1" {
		t.Errorf("dgx host: want %q, got %q", "🧠 spark1", got)
	}
}

func TestHost_IconOptionOverridesDetection(t *testing.T) {
	seg := NewHostSegment(map[string]any{"icon": "🎮"})
	seg.Hostname = fixedHost("spark1")
	seg.Probe = fixedProbe(hostProbe{GOOS: "linux", DMIVendor: "NVIDIA", DMIProduct: "DGX Spark"})

	got, _, _ := seg.Render(context.Background(), emojiStyleFixture(), 0)
	if got != "🎮 spark1" {
		t.Errorf("icon override: want %q, got %q", "🎮 spark1", got)
	}
}

func TestHost_KindOptionOverridesDetection(t *testing.T) {
	seg := NewHostSegment(map[string]any{"kind": "rpi"})
	seg.Hostname = fixedHost("pi4")
	seg.Probe = fixedProbe(hostProbe{GOOS: "linux"})

	got, _, _ := seg.Render(context.Background(), emojiStyleFixture(), 0)
	if got != "🍓 pi4" {
		t.Errorf("kind override: want %q, got %q", "🍓 pi4", got)
	}
}

func TestHost_UnknownKindOptionIsIgnored(t *testing.T) {
	seg := NewHostSegment(map[string]any{"kind": "toaster"})
	seg.Hostname = fixedHost("box")
	seg.Probe = fixedProbe(hostProbe{GOOS: "darwin"})

	got, _, _ := seg.Render(context.Background(), emojiStyleFixture(), 0)
	if got != "🍎 box" {
		t.Errorf("bad kind option must fall through to detection, got %q", got)
	}
}

func TestHost_IconOptionWithoutAutoDetect(t *testing.T) {
	// An explicit icon means the probe is never needed; a nil Probe must not panic.
	seg := NewHostSegment(map[string]any{"icon": "[x]"})
	seg.Hostname = fixedHost("box")
	seg.Probe = nil

	got, _, ok := seg.Render(context.Background(), asciiStyle(), 0)
	if !ok || got != "[x] box" {
		t.Errorf("want %q ok, got %q ok=%v", "[x] box", got, ok)
	}
}

func TestHost_WidthNonIncreasingAcrossLevels(t *testing.T) {
	seg := NewHostSegment(nil)
	seg.Hostname = fixedHost("averyveryverylonghostname")
	seg.Probe = fixedProbe(hostProbe{GOOS: "linux"})
	st := asciiStyle()
	prev := -1
	for level := 0; level <= finalCompactLevel; level++ {
		got, _, _ := seg.Render(context.Background(), st, level)
		w := term.DisplayWidth(got)
		if prev >= 0 && w > prev {
			t.Errorf("level %d width %d > level %d width %d", level, w, level-1, prev)
		}
		prev = w
	}
}

func TestHost_PriorityDefault(t *testing.T) {
	seg := NewHostSegment(nil)
	seg.Hostname = fixedHost("box")
	seg.Probe = fixedProbe(hostProbe{})
	d, ok := seg.detect(context.Background())
	if !ok {
		t.Fatal("detect: want ok")
	}
	if got := priorityOf(d); got != 15 {
		t.Errorf("host default priority = %d; want 15 (after time, before ai)", got)
	}
}

func TestHost_MaxLenOptionAsString(t *testing.T) {
	seg := NewHostSegment(map[string]any{"max_len": "5"})
	seg.Hostname = fixedHost("longhostname")
	seg.Probe = fixedProbe(hostProbe{GOOS: "linux"})

	got, _, _ := seg.Render(context.Background(), asciiStyle(), 0)
	if w := term.DisplayWidth(hostName(t, got)); w != 5 {
		t.Errorf(`max_len="5": name %q is %d columns; want 5`, hostName(t, got), w)
	}
}

func TestShortHostname(t *testing.T) {
	cases := map[string]string{
		"spark1.local":       "spark1",
		"  box.example.com ": "box",
		"10.0.0.5":           "10.0.0.5",
		"fe80::1":            "fe80::1",
		"plain":              "plain",
		".local":             "",
		"":                   "",
	}
	for in, want := range cases {
		if got := shortHostname(in); got != want {
			t.Errorf("shortHostname(%q) = %q; want %q", in, got, want)
		}
	}
}
