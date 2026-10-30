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

// icon is the glyph of a button; the play button shows pause while playing.
func (m Model) icon(a action) string {
	switch a {
	case actPrev:
		return m.icons.prev
	case actBack:
		return m.icons.back
	case actFwd:
		return m.icons.fwd
	case actNext:
		return m.icons.next
	}
	if m.state.Playing {
		return m.icons.pause
	}
	return m.icons.play
}

// iconRow lays out the given buttons left to right and returns where each can be
// clicked (one cell of slack either side; the 3-cell gaps keep spans apart).
// Only the play button is drawn in the accent colour.
func (m Model) iconRow(ac lipgloss.Style, acts []action) (row string, hits []hit, width int) {
	parts := make([]string, len(acts))
	x := 0
	for i, a := range acts {
		glyph := m.icon(a)
		parts[i] = glyph
		if a == actPlay {
			parts[i] = ac.Render(glyph)
		}
		w := lipgloss.Width(glyph)
		hits = append(hits, hit{x - 1, x + w + 1, a})
		x += w + len(iconGap)
	}
	row = strings.Join(parts, iconGap)
	return row, hits, lipgloss.Width(row)
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
func (m Model) compactControls(ac lipgloss.Style, w int) (string, []hit) {
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
	return strings.Repeat(" ", lead) + left + tail, hits
}

func (m Model) transportRow(ac lipgloss.Style, w int) (row string, hits []hit) {
	if m.layout().compact {
		return m.compactControls(ac, w)
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

// transportHits are the clickable spans of the transport row. They do not depend
// on colours, so no style is needed.
func (m Model) transportHits(w int) []hit {
	_, hits := m.transportRow(lipgloss.NewStyle(), w)
	return hits
}
