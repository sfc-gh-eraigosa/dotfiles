// Package layout turns percentage sizes into whole terminal rows. It is pure:
// no terminal, no processes.
package layout

// Rows splits height rows between the title bar and the panes. pct[0] is the
// title's percentage, pct[1:] the panes' (only the panes that will run; a
// dropped pane is simply absent, and the others grow into its share).
//
// Rules:
//   - The title keeps its configured share of the whole screen.
//   - The panes split the rest in proportion to their percentages, so a
//     dropped pane's rows go to the others pro rata.
//   - Every region gets at least one row while there is room, so a 5% pane
//     never rounds away to nothing on a short terminal.
//   - Rounding uses largest remainders; leftover rows go to the largest pane.
//   - When height is smaller than the number of regions, regions at the end
//     get zero rows.
//
// The result has len(pct) entries and sums to height (or less only when
// height < 0, which yields all zeros).
func Rows(height int, pct []int) []int {
	out := make([]int, len(pct))
	if height <= 0 || len(pct) == 0 {
		return out
	}
	if height < len(pct) {
		for i := 0; i < height; i++ {
			out[i] = 1
		}
		return out
	}

	// Ideal rows in hundredths: the title's share of the screen, then the
	// panes' shares of what is left.
	ideal := make([]int, len(pct))
	title := min(max(pct[0], 0), 100)
	ideal[0] = height * title
	paneTotal := 0
	for _, p := range pct[1:] {
		paneTotal += max(p, 0)
	}
	rest := height*100 - ideal[0]
	for i, p := range pct[1:] {
		if paneTotal > 0 {
			ideal[i+1] = rest * max(p, 0) / paneTotal
		}
	}

	// Floor, with the one-row minimum.
	used := 0
	for i, v := range ideal {
		out[i] = max(v/100, 1)
		used += out[i]
	}

	// Too many rows (minimums pushed us over): take from the largest region.
	for used > height {
		big := largest(out, 0)
		if out[big] <= 1 {
			break
		}
		out[big]--
		used--
	}
	// Rows left over: largest remainder first, ties to the earlier region.
	for used < height {
		best, bestRem := -1, -1
		for i, v := range ideal {
			rem := v - out[i]*100
			if rem > bestRem {
				best, bestRem = i, rem
			}
		}
		if bestRem <= 0 {
			best = largest(out, 1)
		}
		out[best]++
		ideal[best] -= 100 // one remainder consumed
		used++
	}
	return out
}

// largest returns the index of the biggest entry at or after from (falling
// back to 0 when from is out of range).
func largest(v []int, from int) int {
	if from >= len(v) {
		from = 0
	}
	idx := from
	for i := from; i < len(v); i++ {
		if v[i] > v[idx] {
			idx = i
		}
	}
	return idx
}
