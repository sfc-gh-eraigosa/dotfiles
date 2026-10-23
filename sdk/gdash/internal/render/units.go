package render

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// FormatValue formats a value in a Grafana unit (spec F3). NaN is "—";
// units gdash does not know print with three significant figures.
func FormatValue(v float64, unit string, decimals *int) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "—"
	}
	fixed := func(x float64, def int) string {
		d := def
		if decimals != nil {
			d = *decimals
		}
		s := strconv.FormatFloat(x, 'f', d, 64)
		if strings.TrimLeft(s, "-0.") == "" { // -0.0 after rounding is 0.0
			s = strings.TrimPrefix(s, "-")
		}
		return s
	}
	switch unit {
	case "percent":
		return fixed(v, 1) + "%"
	case "percentunit":
		return fixed(v*100, 1) + "%"
	case "bytes":
		return scaled(v, 1024, []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"})
	case "decbytes":
		return scaled(v, 1000, []string{"B", "kB", "MB", "GB", "TB", "PB"})
	case "s":
		return seconds(v)
	case "ms":
		if math.Abs(v) >= 1000 {
			return seconds(v / 1000)
		}
		return sig3(v) + " ms"
	case "currencyUSD":
		sign := ""
		if v < 0 && fixed(v, 2) != "0.00" {
			sign, v = "-", -v
		}
		if v >= 1000 {
			return sign + "$" + short(v)
		}
		return sign + "$" + fixed(v, 2)
	case "none":
		if decimals != nil {
			return fixed(v, 0)
		}
		if math.Abs(v) >= 1000 {
			return strconv.FormatFloat(v, 'f', 0, 64)
		}
		return sig3(v)
	default: // short, and every unit gdash does not know
		if decimals != nil && math.Abs(v) < 1000 {
			return fixed(v, 0)
		}
		return short(v)
	}
}

// sig3 is three significant figures with trailing zeros dropped.
func sig3(v float64) string {
	if v == 0 {
		return "0"
	}
	return strconv.FormatFloat(v, 'g', 3, 64)
}

func short(v float64) string {
	a := math.Abs(v)
	for _, s := range []struct {
		div    float64
		suffix string
	}{{1e12, "T"}, {1e9, "G"}, {1e6, "M"}, {1e3, "k"}} {
		if a >= s.div {
			return sig3(v/s.div) + s.suffix
		}
	}
	return sig3(v)
}

func scaled(v, base float64, units []string) string {
	a, i := math.Abs(v), 0
	for a >= base && i < len(units)-1 {
		a /= base
		v /= base
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%.0f %s", v, units[0])
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}

func seconds(v float64) string {
	a := math.Abs(v)
	switch {
	case a < 1:
		return sig3(v*1000) + " ms"
	case a < 60:
		return fmt.Sprintf("%.1f s", v)
	case a < 3600:
		return fmt.Sprintf("%.1f min", v/60)
	case a < 86400:
		return fmt.Sprintf("%.1f h", v/3600)
	}
	return fmt.Sprintf("%.1f d", v/86400)
}

// level is a threshold verdict.
type level int

const (
	levelNone level = iota
	levelOK
	levelWarn
	levelCrit
)

// colorLevel reads a Grafana threshold color as ok, warn or critical.
func colorLevel(c string) level {
	c = strings.ToLower(strings.TrimSpace(c))
	switch {
	case strings.Contains(c, "red"):
		return levelCrit
	case strings.Contains(c, "orange"), strings.Contains(c, "yellow"):
		return levelWarn
	case strings.Contains(c, "green"), strings.Contains(c, "blue"), strings.Contains(c, "transparent"):
		return levelOK
	case strings.HasPrefix(c, "rgb"):
		var r, g, b float64
		inner := c[strings.Index(c, "(")+1:]
		_, _ = fmt.Sscanf(strings.ReplaceAll(inner, " ", ""), "%f,%f,%f", &r, &g, &b)
		switch {
		case r > 200 && g < 100:
			return levelCrit
		case r > 200:
			return levelWarn
		}
		return levelOK
	case strings.HasPrefix(c, "#") && len(c) == 7:
		r, _ := strconv.ParseInt(c[1:3], 16, 0)
		g, _ := strconv.ParseInt(c[3:5], 16, 0)
		switch {
		case r > 200 && g < 100:
			return levelCrit
		case r > 200:
			return levelWarn
		}
		return levelOK
	}
	return levelNone
}
