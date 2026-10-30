package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/kraklin/nowplaying-tui/internal/source"
)

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

// volumeBlock is the volume bar: the icon, a space and the bar (or the word
// "mute"), always volBlockW cells wide so nothing moves when muting.
func (m Model) volumeBlock(ac lipgloss.Style) string {
	track := ac.Render("mute") + strings.Repeat(" ", volTrackW-len("mute"))
	if !(m.muted && m.volume >= 0) {
		filled := max(m.volume, 0) * volTrackW / 100
		track = ac.Render(strings.Repeat("━", filled)) + m.th.dim2.Render(strings.Repeat("─", volTrackW-filled))
	}
	return m.icons.volume + " " + track
}

// bottomRow is the pane's last row, which the card leaves blank: the volume bar
// sits in the left corner, the source name in the right one, and a note or the
// key help is centred between them. The centred text stays centred while it
// fits clear of both; if it would touch one, it slides just far enough (and is
// cut only if it still does not fit). It also returns the volume bar's cell
// range, or {-1, -1} when it is not drawn.
func (m Model) bottomRow(ac lipgloss.Style) (string, [2]int) {
	src := m.sourceTag()
	vol := m.volumeBlock(ac)
	vw := lipgloss.Width(vol)
	if m.layout().compact {
		src, vol = "", "" // the compact card has neither; help and notes still use a free row
	}
	const edge, clear = 2, 2 // cells from the pane edge, and from the centred text
	// room is the width between the corners that the centred text can use.
	room := func() (lo, limit int) {
		lo, limit = 1, m.w-1
		if vol != "" {
			lo = edge + vw + clear
		}
		if src != "" {
			limit = m.w - lipgloss.Width(src) - edge - clear
		}
		return lo, limit
	}
	// The key help is something you asked for: if it cannot fit between the
	// corners, the volume bar and then the source name step aside until the help
	// is closed.
	if m.help && m.note == "" {
		for _, drop := range []*string{&vol, &src} {
			if lo, limit := room(); *drop != "" && lipgloss.Width(helpText) > limit-lo-1 {
				*drop = ""
			}
		}
	}
	lo, limit := room()
	var segs []segment
	volRange := [2]int{-1, -1}
	if vol != "" && m.w >= edge+vw {
		segs = append(segs, segment{edge, vol})
		volRange = [2]int{edge, edge + vw}
	}
	centre, style, tail := "", m.th.dim2, ""
	switch {
	case m.note != "":
		centre, style, tail = m.note, m.th.dim, m.noteTail
	case m.help:
		centre = helpText
	}
	if avail := limit - lo; centre != "" && avail >= 1 {
		head := strings.TrimRight(fit(centre, min(avail-lipgloss.Width(tail), lipgloss.Width(centre))), " ")
		if text := head + tail; strings.TrimSpace(text) != "" {
			cw := lipgloss.Width(text)
			x := (m.w - cw) / 2
			if x+cw > limit {
				x = limit - cw
			}
			x = max(x, lo)
			segs = append(segs, segment{x, style.Render(text)})
		}
	}
	if src != "" && m.w >= lipgloss.Width(src)+edge {
		segs = append(segs, segment{m.w - lipgloss.Width(src) - edge, m.th.dim2.Render(src)})
	}
	return paint(segs), volRange
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
