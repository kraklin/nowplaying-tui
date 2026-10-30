package ui

type layoutMode int

const (
	sideBySide layoutMode = iota
	stacked
	textOnly
)

type layout struct {
	compact  bool // title, artist and controls + time only
	mode     layoutMode
	coverPx  int // 0 hides the cover
	infoW    int
	histRows int
}

// marginX grows with the pane so wide terminals keep breathing room.
func marginX(w int) int { return minMarginX + max(0, w-80)/5 }

func (m Model) layout() layout {
	if m.h < compactBelowRows || m.w < compactBelowCols {
		return layout{mode: textOnly, compact: true, infoW: min(max(m.w-2, 1), 60)}
	}
	w, h := max(m.w-2*marginX(m.w), 0), max(m.h-2*marginY, 0)
	hist := func(rows int) int {
		if !m.showHistory {
			return 0
		}
		return max(0, min(rows, len(m.recent)))
	}
	text := func() layout {
		return layout{mode: textOnly, infoW: min(max(w, 10), 60), histRows: hist(h - infoLines)}
	}
	if !m.showCover || m.w < 40 || m.h < 12 {
		return text()
	}
	even := func(n int) int { return n &^ 1 }
	// Cells are about twice as tall as they are wide, so a pane is portrait when
	// it is taller than wide in pixels: rows*2 > columns.
	portrait := m.h*2 > m.w
	if m.w >= 70 && !portrait {
		px := even(min(h*2, m.cfg.ArtMaxPx, w*45/100, w-gap-minInfoW))
		if px < 8 {
			return text()
		}
		return layout{mode: sideBySide, coverPx: px, infoW: min(w-px-gap, maxInfoW), histRows: hist(h - infoLines)}
	}
	// Stacked: reserve a comfortable cover first, give history what is left, and
	// let the cover grow into any space still unused. The cover is sized for the
	// history's capacity, not for how many entries exist right now, so it does not
	// shrink step by step as the first tracks are recorded.
	want := even(min(m.cfg.ArtMaxPx, 24, w))
	capRows := 0
	if m.showHistory {
		capRows = max(0, min(h-1-want/2-infoLines, m.cfg.HistorySize))
	}
	infoH := infoLines - 2 // no history block
	if capRows > 0 {
		infoH = infoLines + capRows
	}
	px := even(min((h-1-infoH)*2, m.cfg.ArtMaxPx, w))
	if px < 8 {
		return text()
	}
	hr := min(capRows, len(m.recent))
	return layout{mode: stacked, coverPx: px, infoW: min(w, 60), histRows: hr}
}

// geom says where the info block sits on screen, for mouse hit-testing.
type geom struct {
	infoX, infoY, infoW int
	// Rows inside the info block holding the progress bar and the controls, for
	// mouse hit-testing; progRow is -1 when there is no bar (compact card).
	progRow, ctrlRow int
	// The volume bar sits in the bottom left corner: its row and cell range in
	// pane coordinates. volRow is -1 when it is not drawn.
	volRow       int
	volX0, volX1 int
}
