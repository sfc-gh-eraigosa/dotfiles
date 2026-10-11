package pins

import (
	"regexp"
	"strconv"
	"strings"
)

var extractRe = regexp.MustCompile(`\d+\.\d+(\.\d+)?`)

// ExtractVersion returns the first dotted version in s ("" if none). Tool
// banners vary wildly ("kind v0.33.0 go1.26.7", "rustc 1.98.1 (… 2026-09-01)"),
// so the rule is just the first \d+.\d+(.\d+)?.
func ExtractVersion(s string) string { return extractRe.FindString(s) }

// parts splits a dotted version into numbers; ok is false when nothing numeric
// leads it. A trailing suffix on a part ("3-rc1") is ignored.
func parts(v string) (p [3]int, ok bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	for i, s := range strings.SplitN(v, ".", 3) {
		end := 0
		for end < len(s) && s[end] >= '0' && s[end] <= '9' {
			end++
		}
		if end == 0 {
			return p, i > 0
		}
		n, err := strconv.Atoi(s[:end])
		if err != nil {
			return p, false
		}
		p[i] = n
		ok = true
	}
	return p, ok
}

// Compare orders two dotted versions numerically (missing parts are 0):
// -1 if a<b, 0 if equal, 1 if a>b.
func Compare(a, b string) int {
	pa, _ := parts(a)
	pb, _ := parts(b)
	for i := range pa {
		switch {
		case pa[i] < pb[i]:
			return -1
		case pa[i] > pb[i]:
			return 1
		}
	}
	return 0
}

// Gap classifies how far pin is behind latest: current, patch, minor, major,
// ahead (pin newer than upstream) or unknown (either side unparsable).
func Gap(pin, latest string) string {
	pp, ok1 := parts(pin)
	lp, ok2 := parts(latest)
	if !ok1 || !ok2 {
		return "unknown"
	}
	switch c := Compare(pin, latest); {
	case c == 0:
		return "current"
	case c > 0:
		return "ahead"
	}
	switch {
	case pp[0] != lp[0]:
		return "major"
	case pp[1] != lp[1]:
		return "minor"
	}
	return "patch"
}
