// Command glyphcheck verifies that a font file contains a glyph for every
// Private-Use-Area codepoint the gsl powerline style emits. Exit 0 = all
// covered; exit 1 = one or more missing (printed to stderr).
package main

import (
	"fmt"
	"os"

	"golang.org/x/image/font/sfnt"
)

// codepoints are the exact runes from internal/style/builtins.go (powerlineStyle).
var codepoints = []rune{
	0xE0A0, 0xE0B0, 0xE0B1, 0xF017, 0xF055, 0xF071, 0xF07B, 0xF126,
	0xF128, 0xF135, 0xF175, 0xF176, 0xF1C0, 0xF1E6, 0xF408, 0xF47E, 0xF48E,
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: glyphcheck <font.ttf>")
		os.Exit(2)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", os.Args[1], err)
		os.Exit(2)
	}
	f, err := sfnt.Parse(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse %s: %v\n", os.Args[1], err)
		os.Exit(2)
	}
	var b sfnt.Buffer
	var missing []rune
	for _, r := range codepoints {
		g, err := f.GlyphIndex(&b, r)
		if err != nil || g == 0 {
			missing = append(missing, r)
		}
	}
	if len(missing) > 0 {
		for _, r := range missing {
			fmt.Fprintf(os.Stderr, "MISSING U+%04X\n", r)
		}
		fmt.Fprintf(os.Stderr, "FAIL: %d/%d gsl codepoints missing in %s\n",
			len(missing), len(codepoints), os.Args[1])
		os.Exit(1)
	}
	fmt.Printf("OK: all %d gsl codepoints present in %s\n", len(codepoints), os.Args[1])
}
