package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/tomaslatall/nowplay/internal/source"
)

const (
	gap        = 4
	minMarginX = 6
	maxInfoW   = 64
	marginY    = 2
	minInfoW   = 36
	infoLines  = 10 // label … "recently played" header, without history rows

	// Rows inside the info block; the mouse handler relies on them.
	rowProgress  = 5
	rowTransport = 7

	volTrackW = 10 // cells in the volume bar, after the icon and a space
	iconGap   = "   "

	helpText = "space play/pause · n/p skip · ←/→ seek · +/- volume · c clear · q quit"
)

type layoutMode int

const (
	sideBySide layoutMode = iota
	stacked
	textOnly
)

type layout struct {
	mode     layoutMode
	coverPx  int // 0 hides the cover
	infoW    int
	histRows int
}

// marginX grows with the pane so wide terminals keep breathing room.
func marginX(w int) int { return minMarginX + max(0, w-80)/5 }

func (m Model) layout() layout {
	w, h := max(m.w-2*marginX(m.w), 0), max(m.h-2*marginY, 0)
	hist := func(rows int) int { return max(0, min(rows, len(m.recent))) }
	text := func() layout {
		return layout{mode: textOnly, infoW: min(max(w, 10), 60), histRows: hist(h - infoLines)}
	}
	if m.w < 40 || m.h < 12 {
		return text()
	}
	even := func(n int) int { return n &^ 1 }
	if m.w >= 70 {
		px := even(min(h*2, m.cfg.ArtMaxPx, w*45/100, w-gap-minInfoW))
		if px < 8 {
			return text()
		}
		return layout{mode: sideBySide, coverPx: px, infoW: min(w-px-gap, maxInfoW), histRows: hist(h - infoLines)}
	}
	px := even(min((h-infoLines-1)*2, m.cfg.ArtMaxPx, w))
	if px < 8 {
		return text()
	}
	return layout{mode: stacked, coverPx: px, infoW: min(w, 60)}
}

// geom says where the info block sits on screen, for mouse hit-testing.
type geom struct {
	infoX, infoY, infoW int
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
		if r, cached := m.cache[key]; cached {
			cover, accent = strings.Split(r.Rendered, "\n"), r.Accent
		} else {
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
	if m.help && m.h > 0 && rows[m.h-1] == "" {
		text := fit(helpText, min(m.w-2, lipgloss.Width(helpText)))
		rows[m.h-1] = paint([]segment{{max((m.w-lipgloss.Width(text))/2, 0), m.th.dim2.Render(strings.TrimRight(text, " "))}})
	}
	return rows, geom{infoX: infoX, infoY: infoY, infoW: l.infoW}
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
		m.th.dim.Render("nowplay reads what is playing through media-control."),
		m.th.dim.Render("Install it with:"),
		"",
		accent.Render("  brew tap ungive/media-control"),
		accent.Render("  brew install media-control"),
		"",
		m.th.dim.Render("Then start nowplay again. Press q to quit."),
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
	w := l.infoW
	ac := accentStyle(accentHex)
	s := m.state
	line := func(x string) string { return lipgloss.NewStyle().Width(w).Render(x) }

	// A note (error, confirmation) takes over the label row; it must stay one row.
	labelRow := m.th.dim.Render("♫  now playing")
	if s.Status == source.StatusReconnecting {
		labelRow += "  " + m.th.dim2.Render("reconnecting…")
	} else if app := appName(s.App); app != "" && !s.Empty() {
		labelRow += "  " + m.th.dim2.Render(app)
	}
	if m.note != "" {
		tail := m.noteTail
		head := strings.TrimRight(fit("♫  "+m.note, max(w-lipgloss.Width(tail), 1)), " ")
		labelRow = m.th.dim.Render(head + tail)
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
		labelRow, "",
		line(ac.Render(fit(title, w))),
		line(m.truncStyled(sub, s.Artist+" · "+s.Album, w)), "",
		line(progress), "",
		line(transport), "",
	}
	if l.histRows > 0 {
		lines = append(lines, m.th.dim.Render("recently played"))
		lines = append(lines, m.historyRows(l.histRows, w)...)
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
	return m.th.dim.Render(fit(plain, w))
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

// hit is a clickable span of the transport row, in cells from its left edge.
type hit struct {
	x0, x1 int // [x0, x1)
	cmd    source.Cmd
}

// transportRow renders the icons and volume bar and reports where they are, so
// clicks and scrolls land on exactly what was drawn. volX is -1 when the volume
// bar is not shown.
func (m Model) transportRow(ac lipgloss.Style, w int) (row string, hits []hit, vol [2]int) {
	ic := m.icons
	playIcon, playCmd := ic.pause, source.Cmd{Kind: source.TogglePlay}
	if !m.state.Playing {
		playIcon = ic.play
	}
	type item struct {
		styled, plain string
		cmd           *source.Cmd
	}
	prev, next := source.Cmd{Kind: source.Previous}, source.Cmd{Kind: source.Next}
	items := []item{
		{m.th.dim2.Render(ic.shuffle), ic.shuffle, nil}, // not supported by the stream yet
		{ic.prev, ic.prev, &prev},
		{ac.Render(playIcon), playIcon, &playCmd},
		{ic.next, ic.next, &next},
		{m.th.dim2.Render(ic.repeat), ic.repeat, nil},
	}
	parts := make([]string, len(items))
	x := 0
	for i, it := range items {
		parts[i] = it.styled
		width := lipgloss.Width(it.plain)
		if it.cmd != nil {
			// one cell of slack either side; the 3-cell gaps keep spans apart
			hits = append(hits, hit{x - 1, x + width + 1, *it.cmd})
		}
		x += width + len(iconGap)
	}
	left := strings.Join(parts, iconGap)

	filled := max(m.volume, 0) * volTrackW / 100
	volume := ic.volume + " " + ac.Render(strings.Repeat("━", filled)) + m.th.dim2.Render(strings.Repeat("─", volTrackW-filled))
	pad := w - lipgloss.Width(left) - lipgloss.Width(volume)
	if pad < 2 {
		return left, hits, [2]int{-1, -1}
	}
	vx := w - lipgloss.Width(volume)
	return left + strings.Repeat(" ", pad) + volume, hits, [2]int{vx, w}
}

func (m Model) historyRows(n, w int) []string {
	const durW = 5
	rows := make([]string, 0, n)
	rem := w - 3 - 2 - durW - 2
	titleW := rem * 11 / 20
	artistW := rem - titleW
	bg, bright, soft := m.th.bg, m.th.bright, m.th.soft
	for i, e := range m.recent[:n] {
		f := fade(i)
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
