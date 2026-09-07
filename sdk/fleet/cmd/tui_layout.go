package cmd

// The pane model: `fleet tui` is three stacked panels the operator composes.
// The host table on top, the log stream and the error stream sharing the
// bottom. Every height in the frame comes from layout() and nowhere else —
// the arithmetic used to be spread over listHeight/logHeight with inline magic
// numbers, which is how the frame came to overflow the terminal by up to 12
// rows (docs/mbo/designs/fleet-error-view.md §1.3).

type pane int

const (
	paneHost pane = iota
	paneLog
	paneErr
)

// panes is which panels are open; heights is how many BODY rows each gets.
type panes struct{ host, log, err bool }

func (p panes) count() int {
	n := 0
	for _, on := range []bool{p.host, p.log, p.err} {
		if on {
			n++
		}
	}
	return n
}

type heights struct{ host, log, err int }

const (
	// panelBorderRows is a framed panel's top and bottom border.
	panelBorderRows = 2
	// panelLeadRows is the ONE line every open panel writes inside its frame
	// before any body row: the host panel's column header, and a stream pane's
	// title line (logView writes the title into the body before the log lines)
	// — or, when that pane has nothing to show, its collapsed hint, which
	// occupies exactly the same single line.
	panelLeadRows = 1
	// panelFixedRows is therefore what ANY open panel costs before a single
	// body row. Measured against the real render: at 100x40 with host+log,
	// banner 5 + (1+6+2) + (1+24+2) + separator 1 + status 1 = 43.
	panelFixedRows = panelBorderRows + panelLeadRows
	// minPaneRows is the body height below which a pane is useless. It is a
	// PREFERENCE, not a guarantee — see minFrameRows.
	minPaneRows = 3
	// hostShareDenom gives the host table the top fifth once a stream pane is
	// flowing — the ratio the dashboard shipped with.
	hostShareDenom = 5
)

// minFrameRows is what the frame costs with ZERO body rows: the measured
// chrome plus every open panel's fixed cost. When it exceeds the terminal, no
// pane arithmetic can make the frame fit — the panes' job is then to add
// nothing at all. The frame guard uses it as its budget, so the guard and the
// layout cannot disagree about what "fits" means.
func minFrameRows(chromeRows int, p panes) int {
	return chromeRows + panelFixedRows*p.count()
}

// layout splits vpHeight between the visible panes. chromeRows is MEASURED by
// the caller (banner + separator + status), never assumed: the banner's
// key-hint strip wraps at narrow widths, and the "status line" is a framed
// panel of 8-12 rows in modeAnswers/modeConfirm.
//
// A pane that is open but has nothing to show (logActive/errActive false) gets
// zero body rows and renders its one-line hint — an empty box must not cost
// the fleet view a fifth of the screen to say nothing. It costs the same
// panelFixedRows either way, which is why the deduction does not care.
func layout(vpHeight, chromeRows int, p panes, logActive, errActive bool) heights {
	avail := vpHeight - minFrameRows(chromeRows, p)
	if avail < 0 {
		// Chrome + panel frames already exceed the terminal (a dialog on a
		// short screen, or three panels at 60x16). The panes must never ADD to
		// a frame that already does not fit.
		avail = 0
	}

	bottom := (p.log && logActive) || (p.err && errActive)
	switch {
	case !bottom && !p.host:
		return heights{}
	case !bottom:
		return heights{host: avail}
	case !p.host:
		return splitBottom(avail, p.log && logActive, p.err && errActive)
	}

	host := vpHeight / hostShareDenom
	if host < minPaneRows {
		host = minPaneRows
	}
	if host > avail-minPaneRows {
		host = avail - minPaneRows
	}
	if host < 0 {
		host = 0
	}
	h := splitBottom(avail-host, p.log && logActive, p.err && errActive)
	h.host = host
	return h
}

// splitBottom shares the bottom region. With both stream panes active it is
// halved, the odd row going to the log (the primary). Only a region with fewer
// than two rows goes wholly to the log — splitting one row is not a split.
//
// It deliberately does NOT consider which pane has focus: a focus-dependent
// split would resize the frame every time the operator pressed tab, and a
// layout you cannot predict is worse than one that is merely unequal. An error
// pane left with zero body rows still renders its frame and one line, which
// minFrameRows already budgeted — that is why yielding is safe here.
func splitBottom(avail int, log, err bool) heights {
	if avail < 0 {
		avail = 0
	}
	switch {
	case log && err:
		// Split as evenly as the region allows. Insisting both reach
		// minPaneRows before splitting at all collapsed the error pane
		// entirely on a 24-row terminal — which is precisely the screen the
		// operator opened it on. The floor is an aspiration for a roomy
		// terminal, not a precondition for splitting at all. Only a region too
		// small to give each pane a single row goes wholly to the log.
		if avail < 2 {
			return heights{log: avail}
		}
		e := avail / 2
		return heights{log: avail - e, err: e}
	case log:
		return heights{log: avail}
	case err:
		return heights{err: avail}
	}
	return heights{}
}
