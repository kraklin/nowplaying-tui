package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tomaslatall/nowplaying-tui/internal/config"
)

func volumeHarness(w, hgt int, cover bool) (*harness, geom, *muteVol) {
	mv := &muteVol{level: 50}
	cfg := cfgWith(func(c *config.Config) { c.ShowCover = cover })
	h := newHarness(playing(0, 200*time.Second), Deps{Volume: mv, Config: cfg}).withDemoHistory()
	h.send(VolumeMsg{Level: 50})
	h.resize(w, hgt)
	_, g := h.m.compose()
	return h, g, mv
}

func TestVolumeHasItsOwnRowUnderTheControls(t *testing.T) {
	for _, sz := range [][2]int{{100, 30}, {80, 20}, {60, 40}, {80, 50}, {120, 30}} {
		for _, cover := range []bool{true, false} {
			h, g, _ := volumeHarness(sz[0], sz[1], cover)
			rows := h.rows()
			ctrl, vol := rows[g.infoY+rowTransport], rows[g.infoY+rowVolume]
			if strings.Contains(ctrl, h.m.icons.volume) {
				t.Errorf("%v cover=%v: the controls row must not hold the volume bar: %q", sz, cover, ctrl)
			}
			if !strings.Contains(vol, h.m.icons.volume) || !strings.Contains(vol, "━") {
				t.Errorf("%v cover=%v: the row under the controls should be the volume bar: %q", sz, cover, vol)
			}
			if l, r := margins(t, h, g, rowVolume); !near(l, r) {
				t.Errorf("%v cover=%v: the volume bar should be centred: %d / %d", sz, cover, l, r)
			}
		}
	}
}

// With the volume off the controls row, nothing competes with the icons any more.
func TestIconsStayCentredInNarrowCards(t *testing.T) {
	for _, sz := range [][2]int{{50, 30}, {45, 28}, {40, 20}, {36, 14}} {
		h, g, _ := volumeHarness(sz[0], sz[1], true)
		ic := h.m.icons
		row := h.rows()[g.infoY+rowTransport]
		first, last := cellX(row, ic.prev), cellX(row, ic.next)+lipgloss.Width(ic.next)
		if mid, want := (first+last)/2, g.infoX+g.infoW/2; !near(mid, want) {
			t.Errorf("%v: icons centred on %d, card centre %d", sz, mid, want)
		}
	}
}

func TestScrollWorksOnTheVolumeRowOnly(t *testing.T) {
	h, g, _ := volumeHarness(100, 30, true)
	x, y := h.find(h.m.icons.volume)
	if y != g.infoY+rowVolume {
		t.Fatalf("the bar is on row %d, geometry says %d", y, g.infoY+rowVolume)
	}
	if cmd := h.wheel(x+3, g.infoY+rowTransport, tea.MouseButtonWheelUp); cmd != nil || h.m.volume != 50 {
		t.Errorf("scrolling over the controls row, at the bar's column, must not change the volume")
	}
	if cmd := h.wheel(x+3, y, tea.MouseButtonWheelUp); cmd == nil || h.m.volume != 55 {
		t.Errorf("scrolling over the bar's own row changes it: volume %d", h.m.volume)
	}
}

func TestVolumeRowIsHiddenNotWrappedInATinyCard(t *testing.T) {
	for w := 30; w <= 40; w++ {
		h, _, _ := volumeHarness(w, 14, false)
		for i, r := range h.rows() {
			if lipgloss.Width(r) > w {
				t.Errorf("w=%d: row %d is %d wide", w, i, lipgloss.Width(r))
			}
		}
	}
}

func TestCompactStillHasNoVolumeRow(t *testing.T) {
	h, g, _ := volumeHarness(40, 6, true)
	if g.volRow != -1 || strings.Contains(strings.Join(h.rows(), "\n"), h.m.icons.volume) {
		t.Errorf("the compact card has no volume row (volRow=%d)", g.volRow)
	}
}
