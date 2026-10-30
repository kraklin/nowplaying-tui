package ui

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func accentStyle(hex string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(hex))
}

type icons struct {
	prev, back, play, pause, fwd, next, volume string
}

// Icon sets, chosen with `icons` in the config. Back and forward are plain
// ASCII < and >: any glyph a terminal draws at a different width than we
// measure (emoji-capable symbols, variation selectors) makes the pane border
// drift, and ASCII cannot. Play and pause match in width so the transport row
// never shifts.
var (
	unicodeIcons = icons{"⏮", "<", "⏵", "⏸", ">", "⏭", "♪"}
	// basicIcons avoids the ⏮ ⏵ ⏸ ⏭ block, for fonts or terminals that draw it badly.
	basicIcons = icons{"|◂", "◂◂", "▶ ", "▮▮", "▸▸", "▸|", "♪"}
	// nerdIcons needs a Nerd Font.
	nerdIcons = icons{"\uf048", "\uf049", "\uf04b", "\uf04c", "\uf050", "\uf051", "\uf028"}
)

func iconSet(name string) icons {
	switch name {
	case "nerd":
		return nerdIcons
	case "basic":
		return basicIcons
	}
	return unicodeIcons
}

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
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// fade is the visibility of history row i: 1 for the newest, decaying
// exponentially so older rows drift into the background.
func fade(i int) float64 { return math.Max(0.2, math.Exp(-0.22*float64(i))) }

// fadedStyle blends base towards the terminal background by 1-f.
func fadedStyle(base, bg [3]uint8, f float64) lipgloss.Style {
	return colorStyle(mix(bg, base, f))
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }
