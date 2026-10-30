package ui

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
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

// fade is the visibility of history row i: 1 for the newest, decaying
// exponentially so older rows drift into the background.
func fade(i int) float64 { return math.Max(0.2, math.Exp(-0.22*float64(i))) }

// fadedStyle blends base towards the terminal background by 1-f.
func fadedStyle(base, bg [3]uint8, f float64) lipgloss.Style {
	return colorStyle(mix(bg, base, f))
}
