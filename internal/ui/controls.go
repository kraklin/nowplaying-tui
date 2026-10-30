package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

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

func (m Model) transportRow(ac lipgloss.Style, w int) (row string, hits []hit) {
	if m.layout().compact {
		row, hits, _ := m.compactControls(ac, w)
		return row, hits
	}
	left, hits, _ := m.iconRow(ac, []action{actPrev, actBack, actPlay, actFwd, actNext})
	// The icons are centred, or left-aligned beside a cover.
	lead := 0
	if m.layout().mode != sideBySide {
		lead = max((w-lipgloss.Width(left))/2, 0)
	}
	for i := range hits {
		hits[i].x0 += lead
		hits[i].x1 += lead
	}
	return strings.Repeat(" ", lead) + left, hits
}
