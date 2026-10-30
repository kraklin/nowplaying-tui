package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	dim  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6b6b6b", Dark: "#8a8a8a"})
	dim2 = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#9a9a9a", Dark: "#5c5c5c"})
)

func accentStyle(hex string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(hex))
}

type icons struct {
	shuffle, prev, play, pause, next, repeat, volume string
}

// Geometric glyphs only: ⏮ ⏸ ⏭ 🔈 render as double-width emoji in many terminals.
var unicodeIcons = icons{"⇄", "|◂", "▶ ", "▮▮", "▸|", "↻", "♪"}

// fit truncates plain text to w cells with … and pads it with spaces to w.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) > w {
		var b strings.Builder
		used := 0
		for _, r := range s {
			rw := lipgloss.Width(string(r))
			if used+rw > w-1 {
				break
			}
			b.WriteRune(r)
			used += rw
		}
		s = b.String() + "…"
	}
	return s + strings.Repeat(" ", max(0, w-lipgloss.Width(s)))
}

func fmtDur(secs int) string {
	if secs < 0 {
		secs = 0
	}
	h, m, s := secs/3600, secs/60%60, secs%60
	if h > 0 {
		return sprintf("%d:%02d:%02d", h, m, s)
	}
	return sprintf("%d:%02d", m, s)
}
