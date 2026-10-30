package ui

import (
	"fmt"
	"strings"

	"github.com/tomaslatall/nowplaying-tui/internal/config"
)

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
		row := softSt.Render(fmt.Sprintf("%-2d", i+1)) + " " +
			brightSt.Render(fit(e.Title, titleW)) + " " +
			softSt.Render(fit(e.Artist, artistW)) + " " +
			softSt.Render(strings.Repeat(" ", durW-len(dur))+dur)
		rows = append(rows, row)
	}
	return rows
}
