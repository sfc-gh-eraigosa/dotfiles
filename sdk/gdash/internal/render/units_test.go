package render

import (
	"math"
	"testing"
)

func ip(i int) *int { return &i }

// E6: units per spec F3; NaN prints as an em dash; unknown units fall back
// to three significant figures.
func TestFormatValue(t *testing.T) {
	cases := []struct {
		v    float64
		unit string
		dec  *int
		want string
	}{
		{42.123, "percent", nil, "42.1%"},
		{0.421, "percentunit", nil, "42.1%"},
		{0.6, "percentunit", ip(2), "60.00%"},
		{2040109465, "bytes", nil, "1.9 GiB"},
		{1536, "bytes", nil, "1.5 KiB"},
		{512, "bytes", nil, "512 B"},
		{1900000000, "decbytes", nil, "1.9 GB"},
		{3.2, "s", nil, "3.2 s"},
		{289, "s", nil, "4.8 min"},
		{7200, "s", nil, "2.0 h"},
		{0.012, "s", nil, "12 ms"},
		{12.5, "ms", nil, "12.5 ms"},
		{1500, "ms", nil, "1.5 s"},
		{12345, "short", nil, "12.3k"},
		{2500000, "short", nil, "2.5M"},
		{7, "short", nil, "7"},
		{7, "none", nil, "7"},
		{0.25, "none", nil, "0.25"},
		{-3.5, "none", nil, "-3.5"},
		{1234.5678, "bps", nil, "1.23k"},
		{3.14159, "h", nil, "3.14"},
		{math.NaN(), "percent", nil, "—"},
		{0, "short", nil, "0"},
		{-0.00001, "percentunit", nil, "0.0%"}, // no negative zero
		{-0.00001, "percent", nil, "0.0%"},
		{0.0123, "currencyUSD", nil, "$0.01"},
		{0, "currencyUSD", nil, "$0.00"},
		{-2.5, "currencyUSD", nil, "-$2.50"},
		{12345, "currencyUSD", nil, "$12.3k"},
	}
	for _, tc := range cases {
		if got := FormatValue(tc.v, tc.unit, tc.dec); got != tc.want {
			t.Errorf("FormatValue(%v, %q) = %q, want %q", tc.v, tc.unit, got, tc.want)
		}
	}
}
