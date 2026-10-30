package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	gap        = 4
	marginX    = 3
	marginY    = 1
	minInfoW   = 36
	maxCoverPx = 48
	infoLines  = 10 // label … "recently played" header, without history rows
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

func (m Model) layout() layout {
	w, h := max(m.w-2*marginX, 0), max(m.h-2*marginY, 0)
	hist := func(rows int) int { return max(0, min(rows, len(m.recent))) }
	text := func() layout {
		return layout{mode: textOnly, infoW: min(max(w, 10), 60), histRows: hist(h - infoLines)}
	}
	if m.w < 40 || m.h < 12 {
		return text()
	}
	even := func(n int) int { return n &^ 1 }
	if m.w >= 70 {
		px := even(min(h*2, maxCoverPx, w*45/100, w-gap-minInfoW))
		if px < 8 {
			return text()
		}
		return layout{mode: sideBySide, coverPx: px, infoW: w - px - gap, histRows: hist(h - infoLines)}
	}
	px := even(min((h-infoLines-1)*2, maxCoverPx, w))
	if px < 8 {
		return text()
	}
	return layout{mode: stacked, coverPx: px, infoW: min(w, 60)}
}

func (m Model) View() string {
	if m.w == 0 || m.h == 0 {
		return ""
	}
	l := m.layout()
	accent := "#c9a7ff"
	cover := ""
	if key, ok := m.wantArt(); ok {
		if r, cached := m.cache[key]; cached {
			cover, accent = r.Rendered, r.Accent
		} else {
			cover = strings.Repeat(strings.Repeat(" ", l.coverPx)+"\n", l.coverPx/2-1) + strings.Repeat(" ", l.coverPx)
		}
	}
	info := m.info(l, accent)

	var body string
	switch l.mode {
	case sideBySide:
		body = lipgloss.JoinHorizontal(lipgloss.Center, cover, strings.Repeat(" ", gap), info)
	case stacked:
		body = lipgloss.JoinVertical(lipgloss.Center, cover, "", info)
	default:
		body = info
	}
	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, body)
}

func (m Model) info(l layout, accentHex string) string {
	w := l.infoW
	ac := accentStyle(accentHex)
	s := m.state
	line := func(x string) string { return lipgloss.NewStyle().Width(w).Render(x) }

	label := "♫  now playing"
	if app := appName(s.App); app != "" && !s.Empty() {
		label += "  " + dim2.Render(app)
	}
	title, sub := "nothing playing", ""
	if !s.Empty() {
		title = s.Title
		sub = lipgloss.NewStyle().Bold(true).Render(s.Artist) + dim.Render(" · "+s.Album)
	}

	var elapsed, total int
	frac := 0.0
	if !s.Empty() {
		elapsed = int(s.Position(m.now).Seconds())
		total = int(s.Duration.Seconds())
		if total > 0 {
			frac = float64(elapsed) / float64(total)
		}
	}
	timeStr := fmtDur(elapsed) + " / " + fmtDur(total)

	lines := []string{
		dim.Render(label), "",
		line(ac.Render(fit(title, w))),
		line(truncStyled(sub, s.Artist+" · "+s.Album, w)), "",
		line(m.bar(ac, frac, w-lipgloss.Width(timeStr)-2) + "  " + dim.Render(timeStr)), "",
		line(m.transport(ac, w)), "",
	}
	if l.histRows > 0 {
		lines = append(lines, dim.Render("recently played"))
		lines = append(lines, m.historyRows(l.histRows, w)...)
	} else {
		lines = lines[:len(lines)-1]
	}
	return lipgloss.NewStyle().Width(w).Render(strings.Join(lines, "\n"))
}

// truncStyled returns styled when plain fits in w, otherwise a truncated dim version.
func truncStyled(styled, plain string, w int) string {
	if lipgloss.Width(plain) <= w {
		return styled
	}
	return dim.Render(fit(plain, w))
}

func (m Model) bar(ac lipgloss.Style, frac float64, w int) string {
	if w < 3 {
		return ""
	}
	knob := min(int(frac*float64(w-1)), w-1)
	filled := ac.Render(strings.Repeat("━", knob) + "●")
	if !m.state.Playing {
		filled = dim.Render(strings.Repeat("━", knob) + "●")
	}
	return filled + dim2.Render(strings.Repeat("─", w-knob-1))
}

func (m Model) transport(ac lipgloss.Style, w int) string {
	ic := m.icons
	playIcon := ic.pause
	if !m.state.Playing {
		playIcon = ic.play
	}
	left := strings.Join([]string{dim2.Render(ic.shuffle), ic.prev, ac.Render(playIcon), ic.next, dim2.Render(ic.repeat)}, "   ")

	const volW = 10
	filled := m.volume * volW / 100
	vol := ic.volume + " " + ac.Render(strings.Repeat("━", filled)) + dim2.Render(strings.Repeat("─", volW-filled))
	pad := w - lipgloss.Width(left) - lipgloss.Width(vol)
	if pad < 2 {
		return left
	}
	return left + strings.Repeat(" ", pad) + vol
}

func (m Model) historyRows(n, w int) []string {
	const durW = 5
	rows := make([]string, 0, n)
	rem := w - 3 - 2 - durW - 2
	titleW := rem * 11 / 20
	artistW := rem - titleW
	for i, e := range m.recent[:n] {
		dur := fmtDur(int(e.Duration.Seconds()))
		row := dim2.Render(sprintf("%-2d", i+1)) + " " +
			fit(e.Title, titleW) + " " +
			dim.Render(fit(e.Artist, artistW)) + " " +
			dim.Render(strings.Repeat(" ", durW-len(dur))+dur)
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
