package render

import (
	"context"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/term"
)

const flexHost = "averyveryverylonghostname" // 25 columns

// flexDatas is a host segment (full name flexHost) next to a static block, so
// the line has decoration the host must not be allowed to overrun.
func flexDatas(t *testing.T, opts map[string]any) []segmentData {
	t.Helper()
	seg := NewHostSegment(opts)
	seg.Hostname = fixedHost(flexHost)
	seg.Probe = fixedProbe(hostProbe{GOOS: "linux"})
	d, ok := seg.detect(context.Background())
	if !ok {
		t.Fatal("host detect: want ok")
	}
	return []segmentData{d, &staticSegmentData{text: "[dir] project", colorKey: "dirgit"}}
}

// floorWidth is the line width when the host sits at its 11-column floor.
func floorWidth(t *testing.T, datas []segmentData) int {
	t.Helper()
	return term.DisplayWidth(Format(datas, asciiStyle(), 0))
}

func TestFit_HostGrowsToFullNameWhenRoom(t *testing.T) {
	datas := flexDatas(t, nil)
	out := Fit(datas, asciiStyle(), 500)
	if !strings.Contains(out, flexHost) {
		t.Errorf("wide terminal: want the full hostname, got %q", term.StripANSI(out))
	}
}

func TestFit_HostSpendsExactlyTheSlack(t *testing.T) {
	datas := flexDatas(t, nil)
	st := asciiStyle()
	cols := floorWidth(t, datas) + 5 // room for 5 more host columns
	out := Fit(datas, st, cols)
	plain := term.StripANSI(out)
	// floor 11 + 5 slack = 16 columns: 15 characters + ellipsis.
	want := flexHost[:15] + ellipsis
	if !strings.Contains(plain, want) {
		t.Errorf("cols=%d: want host %q, got %q", cols, want, plain)
	}
	if w := term.DisplayWidth(out); w != cols {
		t.Errorf("slack should be fully used: width %d, cols %d", w, cols)
	}
}

func TestFit_HostStaysAtFloorWhenTight(t *testing.T) {
	datas := flexDatas(t, nil)
	cols := floorWidth(t, datas)
	out := Fit(datas, asciiStyle(), cols)
	plain := term.StripANSI(out)
	if want := flexHost[:10] + ellipsis; !strings.Contains(plain, want) {
		t.Errorf("exact floor width: want host %q, got %q", want, plain)
	}
	if w := term.DisplayWidth(out); w > cols {
		t.Errorf("width invariant broken: %d > %d", w, cols)
	}
}

func TestFit_HostWidthInvariantAcrossAllColumns(t *testing.T) {
	datas := flexDatas(t, nil)
	st := asciiStyle()
	for cols := 1; cols <= 80; cols++ {
		out := Fit(datas, st, cols)
		if w := term.DisplayWidth(out); w > cols {
			t.Fatalf("cols=%d: width %d > cols (%q)", cols, w, term.StripANSI(out))
		}
	}
}

func TestFit_HostMaxLenCapsGrowth(t *testing.T) {
	datas := flexDatas(t, map[string]any{"max_len": float64(14)})
	out := term.StripANSI(Fit(datas, asciiStyle(), 500))
	if want := flexHost[:13] + ellipsis; !strings.Contains(out, want) {
		t.Errorf("max_len=14 on a wide terminal: want %q, got %q", want, out)
	}
}

func TestFit_HostMinLenSetsFloor(t *testing.T) {
	datas := flexDatas(t, map[string]any{"min_len": float64(6)})
	cols := floorWidth(t, datas) // floor is now 6 columns
	out := term.StripANSI(Fit(datas, asciiStyle(), cols))
	if want := flexHost[:5] + ellipsis; !strings.Contains(out, want) {
		t.Errorf("min_len=6 at the floor width: want %q, got %q", want, out)
	}
}

// TestFit_HostFloorIsElevenByDefault pins the crowded-line appearance: the
// floor is what Render (no fit loop) shows, and what the goldens contain.
func TestFit_HostFloorIsElevenByDefault(t *testing.T) {
	datas := flexDatas(t, nil)
	text, _ := datas[0].format(asciiStyle(), 0)
	if want := "[host] " + flexHost[:10] + ellipsis; text != want {
		t.Errorf("floor format: want %q, got %q", want, text)
	}
}

// TestFit_HostGrowsAfterSegmentDrop: slack must be spent on the Phase-2 path
// too — when a lower-priority segment is dropped to make the line fit, the
// columns it freed go to the host.
func TestFit_HostGrowsAfterSegmentDrop(t *testing.T) {
	st := asciiStyle()
	host := flexDatas(t, nil)[0]
	// Width of the host alone at its full name.
	full := term.DisplayWidth(Fit([]segmentData{host}, st, 500))
	// A wide static block with no priority: dropped first by the fit loop.
	datas := []segmentData{host, &staticSegmentData{text: strings.Repeat("x", 40), colorKey: "dirgit"}}
	cols := full + 2
	out := Fit(datas, st, cols)
	plain := term.StripANSI(out)
	if strings.Contains(plain, "xxxx") {
		t.Fatalf("cols=%d should force the static block out, got %q", cols, plain)
	}
	if !strings.Contains(plain, flexHost) {
		t.Errorf("after dropping a segment the host should grow to its full name; got %q", plain)
	}
	if w := term.DisplayWidth(out); w > cols {
		t.Errorf("width invariant broken: %d > %d", w, cols)
	}
}
