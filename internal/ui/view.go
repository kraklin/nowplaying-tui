package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/kraklin/nowplaying-tui/internal/source"
)

const (
	gap        = 4
	minMarginX = 6
	maxInfoW   = 64
	marginY    = 2
	minInfoW   = 36
	infoLines  = 8 // title … "recently played" header, without history rows

	// Rows inside the info block; the mouse handler relies on them.
	rowProgress  = 3
	rowTransport = 5

	// Below this the card drops to its compact form: title, artist, controls + time.
	compactBelowRows = 10
	compactBelowCols = 30

	volTrackW = 10 // cells in the volume bar, after the icon and a space
	iconGap   = "   "

	defaultAccent = "#c9a7ff" // until a cover has been read, and for the install hint

	helpText = "space play · n/p skip · ←/→ seek · +/- vol · m mute · a art · s src · r hist · c clear · q quit"
)

// View draws the card, or the install hint when media-control is missing.
func (m Model) View() string {
	if m.w == 0 || m.h == 0 {
		return ""
	}
	if m.state.Status == source.StatusMissing {
		return m.missingView()
	}
	rows, _ := m.compose()
	return strings.Join(rows, "\n")
}

// coverBlock returns the cover's lines and the accent colour to draw with: the
// configured one, else the one picked from the artwork, else the default.
func (m Model) coverBlock(l layout) (cover []string, accent string) {
	accent = defaultAccent
	if r, cached := m.cache[m.wantArt()]; cached {
		accent = r.Accent
		if l.coverPx > 0 {
			cover = strings.Split(r.Rendered, "\n")
		}
	} else if l.coverPx > 0 { // not rendered yet: keep the space
		cover = make([]string, l.coverPx/2)
		for i := range cover {
			cover[i] = strings.Repeat(" ", l.coverPx)
		}
	}
	if m.cfg.Accent != "" {
		accent = m.cfg.Accent
	}
	return cover, accent
}

// compose lays the card out on a w×h grid. It places everything with explicit
// coordinates (no Place/Join rounding), so View and the mouse handler agree.
func (m Model) compose() ([]string, geom) {
	l := m.layout()
	cover, accent := m.coverBlock(l)
	ac := accentStyle(accent)
	info := strings.Split(m.info(l, accent), "\n")

	var coverX, coverY, infoX, infoY int
	switch l.mode {
	case sideBySide:
		bw := l.coverPx + gap + l.infoW
		bh := max(len(cover), len(info))
		x0, y0 := (m.w-bw)/2, (m.h-bh)/2
		coverX, coverY = x0, y0+(bh-len(cover))/2
		infoX, infoY = x0+l.coverPx+gap, y0+(bh-len(info))/2
	case stacked:
		bh := len(cover) + 1 + len(info)
		y0 := (m.h - bh) / 2
		coverX, coverY = (m.w-l.coverPx)/2, y0
		infoX, infoY = (m.w-l.infoW)/2, y0+len(cover)+1
	default:
		infoX, infoY = (m.w-l.infoW)/2, (m.h-len(info))/2
	}
	coverX, coverY, infoX, infoY = max(coverX, 0), max(coverY, 0), max(infoX, 0), max(infoY, 0)

	rows := make([]string, m.h)
	for r := range rows {
		var segs []segment
		if i := r - coverY; i >= 0 && i < len(cover) {
			segs = append(segs, segment{coverX, cover[i]})
		}
		if i := r - infoY; i >= 0 && i < len(info) {
			segs = append(segs, segment{infoX, info[i]})
		}
		rows[r] = paint(segs)
	}
	g := geom{infoX: infoX, infoY: infoY, infoW: l.infoW, progRow: rowProgress, ctrlRow: rowTransport, volRow: -1}
	if m.h > 0 && rows[m.h-1] == "" {
		var vol [2]int
		rows[m.h-1], vol = m.bottomRow(ac)
		if vol[0] >= 0 {
			g.volRow, g.volX0, g.volX1 = m.h-1, vol[0], vol[1]
		}
	}
	if l.compact {
		g.progRow, g.ctrlRow = -1, len(info)-1
	}
	return rows, g
}

// missingView is shown when media-control is not installed.
func (m Model) missingView() string {
	bold := lipgloss.NewStyle().Bold(true)
	accent := accentStyle(defaultAccent)
	if m.cfg.Accent != "" {
		accent = accentStyle(m.cfg.Accent)
	}
	lines := []string{
		bold.Render("media-control not found"),
		"",
		m.th.dim.Render("nowplaying reads what is playing through media-control."),
		m.th.dim.Render("Install it with:"),
		"",
		accent.Render("  brew tap ungive/media-control"),
		accent.Render("  brew install media-control"),
		"",
		m.th.dim.Render("Then start nowplaying again. Press q to quit."),
	}
	w := 0
	for _, ln := range lines {
		w = max(w, lipgloss.Width(ln))
	}
	w = min(w, max(m.w-2, 1))
	x, y := max((m.w-w)/2, 0), max((m.h-len(lines))/2, 0)
	rows := make([]string, m.h)
	for r := range rows {
		if i := r - y; i >= 0 && i < len(lines) {
			ln := lines[i]
			if lipgloss.Width(ln) > w { // narrow pane: cut rather than wrap
				ln = m.th.dim.Render(fit(stripANSI(ln), w))
			}
			rows[r] = paint([]segment{{x, ln}})
		}
	}
	return strings.Join(rows, "\n")
}

func (m Model) info(l layout, accentHex string) string {
	if l.compact {
		return m.compactInfo(l, accentStyle(accentHex))
	}
	w := l.infoW
	ac := accentStyle(accentHex)
	s := m.state
	line := func(x string) string { return lipgloss.NewStyle().Width(w).Render(x) }
	// Beside a cover the text is left-aligned; under it, or alone, it is centred.
	// The bar and the history always keep the full width.
	align := func(x string) string { return centerIn(x, w) }
	if l.mode == sideBySide {
		align = func(x string) string { return x }
	}

	title, sub := "nothing playing", ""
	if !s.Empty() {
		title = s.Title
		sub = lipgloss.NewStyle().Bold(true).Render(s.Artist) + m.th.dim.Render(" · "+s.Album)
	}

	progress := m.progress(ac, w, m.elapsedSeconds())
	transport, _ := m.transportRow(ac, w)

	lines := []string{
		line(align(ac.Render(strings.TrimRight(fit(title, w), " ")))),
		line(align(m.truncStyled(sub, s.Artist+" · "+s.Album, w))), "",
		line(progress), "",
		line(transport), "",
	}
	if l.histRows > 0 {
		lines = append(lines, m.th.dim.Render("recently played"))
		lines = append(lines, m.historyRows(l.histRows, w, accentHex)...)
	} else {
		lines = lines[:len(lines)-1]
	}
	return lipgloss.NewStyle().Width(w).Render(strings.Join(lines, "\n"))
}

func timeLabel(elapsed, total int) string { return fmtDur(elapsed) + " / " + fmtDur(total) }

// barWidth is the progress bar's width in cells, or 0 when there is no bar.
func (m Model) barWidth(w int) int {
	s := m.state
	if !s.Empty() && s.Duration <= 0 {
		return 0
	}
	return max(w-lipgloss.Width(timeLabel(m.elapsedSeconds(), int(s.Duration.Seconds())))-2, 0)
}

// progress is the bar with m:ss / m:ss, or only the elapsed time for streams
// with no duration.
func (m Model) progress(ac lipgloss.Style, w, elapsed int) string {
	s := m.state
	if !s.Empty() && s.Duration <= 0 {
		return m.th.dim.Render(fmtDur(elapsed))
	}
	total := int(s.Duration.Seconds())
	frac := 0.0
	if total > 0 {
		frac = float64(elapsed) / float64(total)
	}
	return m.bar(ac, frac, m.barWidth(w)) + "  " + m.th.dim.Render(timeLabel(elapsed, total))
}

// truncStyled returns styled when plain fits in w, otherwise a truncated dim version.
func (m Model) truncStyled(styled, plain string, w int) string {
	if lipgloss.Width(plain) <= w {
		return styled
	}
	return m.th.dim.Render(strings.TrimRight(fit(plain, w), " "))
}

// centerIn pads s on both sides to width w (the extra cell, if any, goes right).
func centerIn(s string, w int) string {
	pad := w - lipgloss.Width(s)
	if pad <= 0 {
		return s
	}
	left := pad / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", pad-left)
}

func (m Model) bar(ac lipgloss.Style, frac float64, w int) string {
	if w < 3 {
		return ""
	}
	knob := min(int(frac*float64(w-1)), w-1)
	filled := ac.Render(strings.Repeat("━", knob) + "●")
	if !m.state.Playing {
		filled = m.th.dim.Render(strings.Repeat("━", knob) + "●")
	}
	return filled + m.th.dim2.Render(strings.Repeat("─", w-knob-1))
}

// compactInfo is the card for very small panes: title, artist, then controls and
// time. With fewer rows the artist joins the title, then only the controls remain.
func (m Model) compactInfo(l layout, ac lipgloss.Style) string {
	w, s := l.infoW, m.state
	line := func(x string) string { return lipgloss.NewStyle().Width(w).Render(x) }
	title, artist, album := "nothing playing", "", ""
	if !s.Empty() {
		title, artist, album = s.Title, s.Artist, s.Album
	}
	titleRow := centerIn(ac.Render(strings.TrimRight(fit(title, w), " ")), w)
	subRow := ""
	if !s.Empty() {
		subRow = centerIn(m.truncStyled(lipgloss.NewStyle().Bold(true).Render(artist)+m.th.dim.Render(" · "+album), artist+" · "+album, w), w)
	}
	if m.note != "" {
		subRow = centerIn(m.th.dim.Render(strings.TrimRight(fit(m.note, w), " ")), w)
	}
	controls, _ := m.compactControls(ac, w)
	switch {
	case m.h >= 3:
		return strings.Join([]string{line(titleRow), line(subRow), line(controls)}, "\n")
	case m.h == 2:
		one := titleRow
		if artist != "" {
			joined := title + " · " + artist
			one = centerIn(ac.Render(strings.TrimRight(fit(joined, w), " ")), w)
		}
		return strings.Join([]string{line(one), line(controls)}, "\n")
	}
	return line(controls)
}
