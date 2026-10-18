package screen

import (
	"bytes"
	"strings"
	"testing"

	"github.com/hinshun/vt10x"
)

func term(t *testing.T, cols, rows int, input string) vt10x.Terminal {
	t.Helper()
	vt := vt10x.New(vt10x.WithSize(cols, rows))
	if _, err := vt.Write([]byte(input)); err != nil {
		t.Fatal(err)
	}
	return vt
}

func strip(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' && s[i] != 'H' && s[i] != 'l' && s[i] != 'h' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func TestRowTextAndWidth(t *testing.T) {
	vt := term(t, 10, 2, "hello\r\nworld")
	if got := strip(Row(vt, 0, 10)); got != "hello     " {
		t.Fatalf("row 0 = %q", got)
	}
	// Wider than the emulator: padded with blanks, never truncated short.
	if got := strip(Row(vt, 1, 14)); got != "world         " {
		t.Fatalf("row 1 = %q", got)
	}
	// Outside the emulator entirely.
	if got := strip(Row(vt, 5, 3)); got != "   " {
		t.Fatalf("row 5 = %q", got)
	}
}

func TestRowColors(t *testing.T) {
	vt := term(t, 20, 1, "\x1b[1;31mR\x1b[0m\x1b[38;5;200mP\x1b[48;2;1;2;3mT\x1b[7mV")
	r := Row(vt, 0, 4)
	// vt10x brightens bold base colors (31 -> 91), the xterm convention.
	for _, want := range []string{"\x1b[0;1;91;49mR", "38;5;200", "48;2;1;2;3", "\x1b[0;7;38;5;200;48;2;1;2;3mV"} {
		if !strings.Contains(r, want) {
			t.Errorf("row %q missing %q", r, want)
		}
	}
	if !strings.HasSuffix(r, "\x1b[0m") {
		t.Errorf("row does not end with a reset: %q", r)
	}
}

func TestWriterRedrawsOnlyChangedRows(t *testing.T) {
	var out bytes.Buffer
	w := NewWriter(&out)
	_ = w.Draw([]string{"a", "b", "c"}, Cursor{})
	out.Reset()
	_ = w.Draw([]string{"a", "B", "c"}, Cursor{X: 2, Y: 1, Show: true})
	got := out.String()
	if strings.Contains(got, "\x1b[1;1H") || strings.Contains(got, "\x1b[3;1H") {
		t.Fatalf("unchanged rows redrawn: %q", got)
	}
	if !strings.Contains(got, "\x1b[2;1HB") || !strings.Contains(got, "\x1b[2;3H\x1b[?25h") {
		t.Fatalf("changed row or cursor missing: %q", got)
	}
	out.Reset()
	w.Invalidate()
	_ = w.Draw([]string{"a", "B", "c"}, Cursor{})
	if strings.Count(out.String(), ";1H") != 3 {
		t.Fatalf("Invalidate did not repaint all rows: %q", out.String())
	}
}

func TestRowReverseOfDefaultColors(t *testing.T) {
	r := Row(term(t, 5, 1, "\x1b[7mX"), 0, 1)
	if !strings.Contains(r, "\x1b[0;7;39;49mX") {
		t.Fatalf("reverse video on default colors = %q, want 7 with default fg/bg", r)
	}
}
