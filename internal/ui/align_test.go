package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tomaslatall/nowplaying-tui/internal/config"
	"github.com/tomaslatall/nowplaying-tui/internal/source"
)

// span is the first and one-past-last screen column holding non-space text.
func span(row string) (start, end int) {
	start, end = -1, -1
	col := 0
	for _, r := range row {
		w := lipgloss.Width(string(r))
		if r != ' ' {
			if start < 0 {
				start = col
			}
			end = col + w
		}
		col += w
	}
	return start, end
}

func stackedHarness(st source.State, w, hgt int, deps Deps) (*harness, geom) {
	h := newHarness(st, deps).withDemoHistory()
	h.send(VolumeMsg{Level: 55})
	h.resize(w, hgt)
	_, g := h.m.compose()
	return h, g
}

// margins returns the empty cells left and right of the text in an info row.
func margins(t *testing.T, h *harness, g geom, row int) (left, right int) {
	t.Helper()
	rows := h.rows()
	s, e := span(rows[g.infoY+row])
	if s < 0 {
		t.Fatalf("info row %d is empty", row)
	}
	return s - g.infoX, g.infoX + g.infoW - e
}

func near(a, b int) bool { return a-b <= 1 && b-a <= 1 }

var stackedSizes = [][2]int{{60, 40}, {50, 30}, {100, 60}, {80, 50}, {120, 70}, {66, 36}}

func TestStackedLabelTitleAndArtistAreCentred(t *testing.T) {
	for _, sz := range stackedSizes {
		h, g := stackedHarness(playing(0, 200*time.Second), sz[0], sz[1], Deps{})
		if h.m.layout().mode != stacked {
			t.Fatalf("%v should stack", sz)
		}
		for name, row := range map[string]int{"label": 0, "title": 2, "artist and album": 3} {
			if l, r := margins(t, h, g, row); !near(l, r) || l == 0 {
				t.Errorf("%v: %s has %d empty cells on the left and %d on the right; should be centred", sz, name, l, r)
			}
		}
	}
}

// Centring needs room: the icons (19 cells) plus, on each side, the volume bar
// (12) and a 2-cell gap. Narrower cards shift the icons left just enough to clear
// the volume bar (see TestStackedIconsNeverCrowdTheVolumeBar).
const centringNeedsAtLeast = 19 + 2*(12+2)

func TestStackedControlsAreCentred(t *testing.T) {
	checked := 0
	for _, sz := range stackedSizes {
		h, g := stackedHarness(playing(0, 200*time.Second), sz[0], sz[1], Deps{Volume: &fakeVol{}})
		if g.infoW < centringNeedsAtLeast {
			continue
		}
		checked++
		ic := h.m.icons
		row := h.rows()[g.infoY+rowTransport]
		first := cellX(row, ic.prev)
		last := cellX(row, ic.next) + lipgloss.Width(ic.next)
		if mid, want := (first+last)/2, g.infoX+g.infoW/2; !near(mid, want) {
			t.Errorf("%v (card %d wide): the icons are centred on column %d, the card's centre is %d", sz, g.infoW, mid, want)
		}
	}
	if checked < 4 {
		t.Errorf("only %d sizes were wide enough to check; widen stackedSizes", checked)
	}
}

func TestNarrowStackedCardsShiftIconsOnlyAsMuchAsNeeded(t *testing.T) {
	h, g := stackedHarness(playing(0, 200*time.Second), 50, 30, Deps{Volume: &fakeVol{}})
	if g.infoW >= centringNeedsAtLeast {
		t.Fatalf("setup: a %d-wide card should be too narrow to centre", g.infoW)
	}
	ic := h.m.icons
	row := h.rows()[g.infoY+rowTransport]
	last := cellX(row, ic.next) + lipgloss.Width(ic.next)
	if gap := cellX(row, ic.volume) - last; gap != 2 {
		t.Errorf("icons should sit exactly 2 cells from the volume bar when squeezed, got %d", gap)
	}
	if first := cellX(row, ic.prev); first <= g.infoX {
		t.Errorf("they should still be moved off the left edge (centred as far as possible): first icon at %d, edge %d", first, g.infoX)
	}
}

func TestStackedIconsNeverCrowdTheVolumeBar(t *testing.T) {
	for w := 36; w <= 110; w++ {
		h, g := stackedHarness(playing(0, 200*time.Second), w, w, Deps{Volume: &fakeVol{}}) // rows ≥ w/2: always portrait or narrow
		if h.m.layout().mode != stacked {
			continue
		}
		ic := h.m.icons
		row := h.rows()[g.infoY+rowTransport]
		first := cellX(row, ic.prev)
		last := cellX(row, ic.next) + lipgloss.Width(ic.next)
		if first < g.infoX {
			t.Errorf("w=%d: icons start at %d, left of the card at %d", w, first, g.infoX)
		}
		if vx := cellX(row, ic.volume); vx >= 0 && vx-last < 2 {
			t.Errorf("w=%d: only %d cells between the icons and the volume bar", w, vx-last)
		}
		if last > g.infoX+g.infoW {
			t.Errorf("w=%d: icons run past the card", w)
		}
	}
}

func TestOtherLayoutsStayLeftAligned(t *testing.T) {
	cases := map[string]struct {
		w, h int
		cfg  *config.Config
	}{
		"side by side": {80, 20, nil},
		"text only":    {100, 30, cfgWith(func(c *config.Config) { c.ShowCover = false })},
		"tiny pane":    {36, 14, nil},
	}
	for name, tc := range cases {
		h := newHarness(playing(0, 200*time.Second), Deps{Config: tc.cfg}).withDemoHistory()
		h.resize(tc.w, tc.h)
		if h.m.layout().mode == stacked {
			t.Fatalf("%s: setup should not be stacked", name)
		}
		_, g := h.m.compose()
		rows := h.rows()
		for label, tr := range map[string]struct {
			row  int
			text string
		}{"title": {2, "Low Orbit"}, "label": {0, "now playing"}} {
			if x := cellX(rows[g.infoY+tr.row], tr.text); label == "title" && x != g.infoX {
				t.Errorf("%s: the title starts at %d, want the left edge %d", name, x, g.infoX)
			}
		}
		if x := cellX(rows[g.infoY+rowTransport], h.m.icons.prev); x != g.infoX {
			t.Errorf("%s: icons start at %d, want the left edge %d", name, x, g.infoX)
		}
	}
}

func TestStackedProgressBarAndHistoryKeepTheirFullWidthLayout(t *testing.T) {
	h, g := stackedHarness(playing(0, 200*time.Second), 100, 60, Deps{})
	rows := h.rows()
	if x := cellX(rows[g.infoY+rowProgress], "●"); x != g.infoX {
		t.Errorf("the bar still starts at the card's left edge: knob at %d, edge %d", x, g.infoX)
	}
	_, e := span(rows[g.infoY+rowProgress])
	if e != g.infoX+g.infoW {
		t.Errorf("and the time label ends at the right edge: %d vs %d", e, g.infoX+g.infoW)
	}
	y := -1
	for i, r := range rows {
		if strings.Contains(r, "recently played") {
			y = i
		}
	}
	if y < 0 {
		t.Fatal("history missing in a tall pane")
	}
	if x := cellX(rows[y], "recently"); x != g.infoX {
		t.Errorf("the history header stays aligned with its table: %d vs %d", x, g.infoX)
	}
	if x := cellX(rows[y+1], "1"); x != g.infoX {
		t.Errorf("the table starts at the left edge: %d vs %d", x, g.infoX)
	}
}

func TestStackedLongTitlesStillFit(t *testing.T) {
	st := playing(0, 200*time.Second)
	st.Title = strings.Repeat("Very Long Title ", 12)
	st.Artist = strings.Repeat("An Artist ", 12)
	st.Album = strings.Repeat("Some Album ", 12)
	h, g := stackedHarness(st, 60, 40, Deps{})
	rows := h.rows()
	for name, row := range map[string]int{"title": 2, "artist line": 3} {
		s, e := span(rows[g.infoY+row])
		if s < g.infoX || e > g.infoX+g.infoW {
			t.Errorf("%s spans %d..%d, outside the card %d..%d", name, s, e, g.infoX, g.infoX+g.infoW)
		}
		if !strings.Contains(rows[g.infoY+row], "…") {
			t.Errorf("%s should be cut with …", name)
		}
	}
	if len(rows) != 40 {
		t.Errorf("height changed: %d", len(rows))
	}
}

func TestStackedNoteAndReconnectingAreCentredToo(t *testing.T) {
	h, g := stackedHarness(playing(0, 200*time.Second), 80, 50, Deps{})
	h.m.setNote("next failed")
	if l, r := margins(t, h, g, 0); !near(l, r) || l == 0 {
		t.Errorf("a note should be centred: %d / %d", l, r)
	}
	st := playing(0, 200*time.Second)
	st.Status = source.StatusReconnecting
	h2, g2 := stackedHarness(st, 80, 50, Deps{})
	if l, r := margins(t, h2, g2, 0); !near(l, r) || l == 0 {
		t.Errorf("the reconnecting label should be centred: %d / %d", l, r)
	}
}

func TestStackedNothingPlayingIsCentred(t *testing.T) {
	h, g := stackedHarness(source.State{}, 80, 50, Deps{})
	for name, row := range map[string]int{"label": 0, "title": 2} {
		if l, r := margins(t, h, g, row); !near(l, r) || l == 0 {
			t.Errorf("idle %s: %d / %d", name, l, r)
		}
	}
}

// --- the shifted icons must still be clickable ---------------------------------------

func TestClickCenteredIconsInStackedLayout(t *testing.T) {
	for _, sz := range [][2]int{{60, 40}, {100, 60}, {50, 30}} {
		rec := &recorder{}
		h, _ := stackedHarness(playing(60*time.Second, 200*time.Second), sz[0], sz[1], Deps{Control: rec.control, Volume: &fakeVol{}})
		ic := h.m.icons
		for name, glyph := range map[string]string{"prev": ic.prev, "back": ic.back, "play": ic.pause, "fwd": ic.fwd, "next": ic.next} {
			x, y := h.find(glyph)
			if x < 0 {
				t.Fatalf("%v: %s not drawn", sz, name)
			}
			h.m.seekAt = time.Time{}
			run(h.click(x, y))
		}
		kinds := make([]source.CmdKind, len(rec.cmds))
		for i, c := range rec.cmds {
			kinds[i] = c.Kind
		}
		// map iteration order is random, so compare as a multiset
		count := map[source.CmdKind]int{}
		for _, k := range kinds {
			count[k]++
		}
		if count[source.Previous] != 1 || count[source.Next] != 1 || count[source.TogglePlay] != 1 || count[source.Seek] != 2 || len(kinds) != 5 {
			t.Errorf("%v: clicking the five centred icons sent %v", sz, kinds)
		}
	}
}

func TestOldLeftEdgeIsNoLongerAButtonWhenCentred(t *testing.T) {
	rec := &recorder{}
	h, g := stackedHarness(playing(60*time.Second, 200*time.Second), 100, 60, Deps{Control: rec.control})
	if cmd := h.click(g.infoX, g.infoY+rowTransport); cmd != nil {
		t.Errorf("the left edge of the card is empty now; clicking it must do nothing")
	}
	if len(rec.cmds) != 0 {
		t.Errorf("sent %+v", rec.cmds)
	}
}

func TestScrollOverVolumeInStackedLayout(t *testing.T) {
	h, _ := stackedHarness(playing(0, 200*time.Second), 100, 60, Deps{Volume: &fakeVol{}})
	x, y := h.find(h.m.icons.volume)
	if cmd := h.wheel(x+3, y, tea.MouseButtonWheelUp); cmd == nil || h.m.volume != 60 {
		t.Errorf("the volume bar still scrolls where it is drawn; volume %d", h.m.volume)
	}
}

func TestResizingAcrossTheStackingBoundaryMovesTheAlignment(t *testing.T) {
	h := newHarness(playing(0, 200*time.Second), Deps{}).withDemoHistory()
	h.resize(80, 20) // landscape: side by side, left aligned
	_, g := h.m.compose()
	if x := cellX(h.rows()[g.infoY+2], "Low Orbit"); x != g.infoX {
		t.Fatalf("side by side starts at the left edge: %d vs %d", x, g.infoX)
	}
	h.resize(80, 50) // portrait: stacked, centred
	_, g = h.m.compose()
	if l, r := margins(t, h, g, 2); !near(l, r) || l == 0 {
		t.Errorf("after resizing to portrait the title should be centred: %d / %d", l, r)
	}
	h.resize(80, 20)
	_, g = h.m.compose()
	if x := cellX(h.rows()[g.infoY+2], "Low Orbit"); x != g.infoX {
		t.Errorf("and back to left aligned: %d vs %d", x, g.infoX)
	}
}
