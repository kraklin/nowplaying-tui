package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/tomaslatall/nowplaying-tui/internal/config"
	"github.com/tomaslatall/nowplaying-tui/internal/source"
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

	helpText = "space play · n/p skip · ←/→ seek · +/- vol · m mute · a art · s src · r hist · c clear · q quit"
)

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
}

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

// compose lays the card out on a w×h grid. It places everything with explicit
// coordinates (no Place/Join rounding), so View and the mouse handler agree.
func (m Model) compose() ([]string, geom) {
	l := m.layout()
	accent := "#c9a7ff"
	var cover []string
	if key, ok := m.wantArt(); ok {
		r, cached := m.cache[key]
		switch {
		case cached:
			accent = r.Accent
			if l.coverPx > 0 {
				cover = strings.Split(r.Rendered, "\n")
			}
		case l.coverPx > 0: // not rendered yet: keep the space
			cover = make([]string, l.coverPx/2)
			for i := range cover {
				cover[i] = strings.Repeat(" ", l.coverPx)
			}
		}
	}
	if m.cfg.Accent != "" {
		accent = m.cfg.Accent
	}
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
	if m.h > 0 && rows[m.h-1] == "" {
		rows[m.h-1] = m.bottomRow()
	}
	g := geom{infoX: infoX, infoY: infoY, infoW: l.infoW, progRow: rowProgress, ctrlRow: rowTransport}
	if l.compact {
		g.progRow, g.ctrlRow = -1, len(info)-1
	}
	return rows, g
}

// sourceTag is the name of the player for the bottom right corner: Spotify,
// Firefox… It is hidden by show_source / the s key, and when nothing is playing.
// "reconnecting…" is shown regardless, because it matters.
func (m Model) sourceTag() string {
	s := m.state
	if s.Status == source.StatusReconnecting {
		return "reconnecting…"
	}
	if !m.showSource || s.Empty() {
		return ""
	}
	return appName(s.App)
}

// bottomRow is the pane's last row, which the card leaves blank: the source name
// sits in the right corner, and a note or the key help is centred. The centred
// text stays centred while it fits clear of the source; if it would touch it, it
// slides left just far enough (and is cut only if it still does not fit).
func (m Model) bottomRow() string {
	src := m.sourceTag()
	if m.layout().compact {
		src = "" // the compact card has no source; help and notes still use a free row
	}
	// The key help is something you asked for: if it cannot fit beside the source
	// name, the name steps aside until the help is closed.
	if m.help && m.note == "" && src != "" && lipgloss.Width(helpText) > m.w-lipgloss.Width(src)-2-2-1 {
		src = ""
	}
	srcW := lipgloss.Width(src)
	limit := m.w - 1 // the centred text must end before this column
	if src != "" {
		limit = m.w - srcW - 2 - 2 // two cells clear of the source
	}
	var segs []segment
	centre, style, tail := "", m.th.dim2, ""
	switch {
	case m.note != "":
		centre, style, tail = m.note, m.th.dim, m.noteTail
	case m.help:
		centre = helpText
	}
	if avail := limit - 1; centre != "" && avail >= 1 {
		head := strings.TrimRight(fit(centre, min(avail-lipgloss.Width(tail), lipgloss.Width(centre))), " ")
		if text := head + tail; strings.TrimSpace(text) != "" {
			cw := lipgloss.Width(text)
			x := (m.w - cw) / 2
			if x+cw > limit {
				x = max(limit-cw, 1)
			}
			segs = append(segs, segment{x, style.Render(text)})
		}
	}
	if src != "" && m.w >= srcW+2 {
		segs = append(segs, segment{m.w - srcW - 2, m.th.dim2.Render(src)})
	}
	return paint(segs)
}

type segment struct {
	x    int
	text string
}

// paint joins segments (in left-to-right order) with plain spaces between them.
func paint(segs []segment) string {
	var sb strings.Builder
	cur := 0
	for _, s := range segs {
		sb.WriteString(strings.Repeat(" ", max(s.x-cur, 0)))
		sb.WriteString(s.text)
		cur = s.x + lipgloss.Width(s.text)
	}
	return sb.String()
}

// missingView is shown when media-control is not installed.
func (m Model) missingView() string {
	bold := lipgloss.NewStyle().Bold(true)
	accent := accentStyle("#c9a7ff")
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

	var elapsed int
	if !s.Empty() {
		elapsed = int(s.Position(m.now).Seconds())
	}
	progress := m.progress(ac, w, elapsed)
	transport, _, _ := m.transportRow(ac, w)

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
	elapsed := 0
	if !s.Empty() {
		elapsed = int(s.Position(m.now).Seconds())
	}
	return max(w-lipgloss.Width(timeLabel(elapsed, int(s.Duration.Seconds())))-2, 0)
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

type action int

const (
	actPrev action = iota
	actBack
	actPlay
	actFwd
	actNext
)

// hit is a clickable span of the transport row, in cells from its left edge.
type hit struct {
	x0, x1 int // [x0, x1)
	act    action
}

// transportRow renders the icons and volume bar and reports where they are, so
// clicks and scrolls land on exactly what was drawn. volX is -1 when the volume
// bar is not shown.
// iconRow lays out the given buttons left to right and returns where each can be
// clicked (one cell of slack either side; the 3-cell gaps keep spans apart).
func (m Model) iconRow(ac lipgloss.Style, acts []action) (left string, hits []hit, width int) {
	ic := m.icons
	playIcon := ic.pause
	if !m.state.Playing {
		playIcon = ic.play
	}
	all := map[action][2]string{
		actPrev: {ic.prev, ic.prev}, actBack: {ic.back, ic.back}, actPlay: {ac.Render(playIcon), playIcon},
		actFwd: {ic.fwd, ic.fwd}, actNext: {ic.next, ic.next},
	}
	parts := make([]string, len(acts))
	x := 0
	for i, a := range acts {
		parts[i] = all[a][0]
		w := lipgloss.Width(all[a][1])
		hits = append(hits, hit{x - 1, x + w + 1, a})
		x += w + len(iconGap)
	}
	left = strings.Join(parts, iconGap)
	return left, hits, lipgloss.Width(left)
}

// elapsedSeconds is the interpolated position in whole seconds.
func (m Model) elapsedSeconds() int {
	if m.state.Empty() {
		return 0
	}
	return int(m.state.Position(m.now).Seconds())
}

// compactControls is the controls row of the compact card: the buttons followed
// by the time, centred. In a very narrow card it drops the time, then the outer
// buttons, down to play/pause alone. There is no volume bar.
func (m Model) compactControls(ac lipgloss.Style, w int) (string, []hit, [2]int) {
	var left string
	var hits []hit
	var lw int
	for _, set := range [][]action{{actPrev, actBack, actPlay, actFwd, actNext}, {actPrev, actPlay, actNext}, {actPlay}} {
		if left, hits, lw = m.iconRow(ac, set); lw <= w {
			break
		}
	}
	timeText := timeLabel(m.elapsedSeconds(), int(m.state.Duration.Seconds()))
	if !m.state.Empty() && m.state.Duration <= 0 {
		timeText = fmtDur(m.elapsedSeconds())
	}
	group, tail := lw, ""
	if lw+2+lipgloss.Width(timeText) <= w {
		tail = "  " + m.th.dim.Render(timeText)
		group += 2 + lipgloss.Width(timeText)
	}
	lead := max((w-group)/2, 0)
	for i := range hits {
		hits[i].x0 += lead
		hits[i].x1 += lead
	}
	return strings.Repeat(" ", lead) + left + tail, hits, [2]int{-1, -1}
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
	controls, _, _ := m.compactControls(ac, w)
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

func (m Model) transportRow(ac lipgloss.Style, w int) (row string, hits []hit, vol [2]int) {
	if m.layout().compact {
		return m.compactControls(ac, w)
	}
	ic := m.icons
	left, hits, _ := m.iconRow(ac, []action{actPrev, actBack, actPlay, actFwd, actNext})
	lw := lipgloss.Width(left)

	// The icons are centred (left-aligned beside a cover); the volume bar stays
	// at the right edge.
	lead := max((w-lw)/2, 0)
	if m.layout().mode == sideBySide {
		lead = 0
	}

	// While muted the word "mute" replaces the bar, padded to the same width so
	// nothing in the row moves and the scroll region stays where it was.
	track := ac.Render("mute") + strings.Repeat(" ", volTrackW-len("mute"))
	if !(m.muted && m.volume >= 0) {
		filled := max(m.volume, 0) * volTrackW / 100
		track = ac.Render(strings.Repeat("━", filled)) + m.th.dim2.Render(strings.Repeat("─", volTrackW-filled))
	}
	volume := ic.volume + " " + track
	vw := lipgloss.Width(volume)
	pad := w - lw - vw // free cells when the icons start at the left edge
	volShown := pad >= 2
	if volShown {
		lead = min(lead, pad-2) // never closer than two cells to the volume bar
	}
	for i := range hits {
		hits[i].x0 += lead
		hits[i].x1 += lead
	}
	if !volShown {
		return strings.Repeat(" ", lead) + left, hits, [2]int{-1, -1}
	}
	gapMid := w - lead - lw - vw
	return strings.Repeat(" ", lead) + left + strings.Repeat(" ", gapMid) + volume, hits, [2]int{w - vw, w}
}

func (m Model) historyRows(n, w int, accentHex string) []string {
	const durW = 5
	rows := make([]string, 0, n)
	rem := w - 3 - 2 - durW - 2
	titleW := rem * 11 / 20
	artistW := rem - titleW
	bg, bright, soft := m.th.bg, m.th.bright, m.th.soft
	if ac, err := config.ParseHex(accentHex); err == nil { // lean towards the accent
		a := [3]uint8{ac.R, ac.G, ac.B}
		bright, soft = mix(bright, a, 0.25), mix(soft, a, 0.40)
	}
	for i, e := range m.recent[:n] {
		f := fade(i)
		if !m.cfg.HistoryFade {
			f = 1
		}
		brightSt, softSt := fadedStyle(bright, bg, f), fadedStyle(soft, bg, f)
		dur := fmtDur(int(e.Duration.Seconds()))
		row := softSt.Render(sprintf("%-2d", i+1)) + " " +
			brightSt.Render(fit(e.Title, titleW)) + " " +
			softSt.Render(fit(e.Artist, artistW)) + " " +
			softSt.Render(strings.Repeat(" ", durW-len(dur))+dur)
		rows = append(rows, row)
	}
	return rows
}

func appName(bundle string) string {
	names := map[string]string{
		"com.spotify.client":         "Spotify",
		"com.apple.Music":            "Music",
		"com.apple.Safari":           "Safari",
		"com.google.Chrome":          "Chrome",
		"org.mozilla.firefox":        "Firefox",
		"company.thebrowser.Browser": "Arc",
		"com.brave.Browser":          "Brave",
	}
	if n, ok := names[bundle]; ok {
		return n
	}
	if i := strings.LastIndex(bundle, "."); i >= 0 {
		return bundle[i+1:]
	}
	return bundle
}
