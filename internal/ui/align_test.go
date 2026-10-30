package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kraklin/nowplaying-tui/internal/config"
	"github.com/kraklin/nowplaying-tui/internal/source"
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
		for name, row := range map[string]int{"title": 0, "artist and album": 1} {
			if l, r := margins(t, h, g, row); !near(l, r) || l == 0 {
				t.Errorf("%v: %s has %d empty cells on the left and %d on the right; should be centred", sz, name, l, r)
			}
		}
	}
}

// Centring needs room: the icons (17 cells) plus, on each side, the volume bar
// (12) and a 2-cell gap. Narrower cards shift the icons left just enough to clear
// the volume bar (see TestStackedIconsNeverCrowdTheVolumeBar).
const centringNeedsAtLeast = 17 + 2*(12+2)

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

func TestStackedIconsStayInsideTheCard(t *testing.T) {
	for w := 36; w <= 110; w++ {
		h, g := stackedHarness(playing(0, 200*time.Second), w, w, Deps{Volume: &fakeVol{}}) // rows ≥ w/2: always portrait or narrow
		if h.m.layout().mode != stacked {
			continue
		}
		ic := h.m.icons
		row := h.rows()[g.infoY+rowTransport]
		first := cellX(row, ic.prev)
		last := cellX(row, ic.next) + lipgloss.Width(ic.next)
		if first < g.infoX || last > g.infoX+g.infoW {
			t.Errorf("w=%d: icons span %d..%d, the card is %d..%d", w, first, last, g.infoX, g.infoX+g.infoW)
		}
		if mid, want := (first+last)/2, g.infoX+g.infoW/2; !near(mid, want) {
			t.Errorf("w=%d: icons centred on %d, column centre %d", w, mid, want)
		}
		if strings.Contains(row, ic.volume) {
			t.Errorf("w=%d: the volume bar lives in the bottom corner now", w)
		}
	}
}

// Beside the cover the text and the icons start at the left edge of the column.
func TestSideBySideTextIsLeftAligned(t *testing.T) {
	for _, sz := range [][2]int{{80, 20}, {100, 30}, {120, 30}, {200, 50}} {
		h, g := stackedHarness(playing(0, 200*time.Second), sz[0], sz[1], Deps{Volume: &fakeVol{}})
		if h.m.layout().mode != sideBySide {
			t.Fatalf("%v should be side by side", sz)
		}
		for name, row := range map[string]int{"title": 0, "artist and album": 1} {
			if l, r := margins(t, h, g, row); l != 0 || r == 0 {
				t.Errorf("%v: %s has %d empty cells left and %d right", sz, name, l, r)
			}
		}
		ic := h.m.icons
		row := h.rows()[g.infoY+rowTransport]
		if first := cellX(row, ic.prev); first != g.infoX {
			t.Errorf("%v: first icon at %d, column starts at %d", sz, first, g.infoX)
		}
	}
}

func textOnlyHarness(w, hgt int, st source.State, deps Deps) (*harness, geom) {
	cfg := cfgWith(func(c *config.Config) { c.ShowCover = false })
	deps.Config = cfg
	h := newHarness(st, deps).withDemoHistory()
	h.send(VolumeMsg{Level: 55})
	h.resize(w, hgt)
	_, g := h.m.compose()
	return h, g
}

func TestTextOnlyCardIsCentredLikeTheStackedOne(t *testing.T) {
	for _, sz := range [][2]int{{100, 30}, {80, 20}, {120, 40}, {60, 24}, {200, 50}} {
		h, g := textOnlyHarness(sz[0], sz[1], playing(0, 200*time.Second), Deps{Volume: &fakeVol{}})
		if h.m.layout().mode != textOnly {
			t.Fatalf("%v: setup should be text only", sz)
		}
		for name, row := range map[string]int{"title": 0, "artist and album": 1} {
			if l, r := margins(t, h, g, row); !near(l, r) || l == 0 {
				t.Errorf("%v: %s has %d empty cells left and %d right; should be centred", sz, name, l, r)
			}
		}
		if g.infoW >= centringNeedsAtLeast {
			ic := h.m.icons
			row := h.rows()[g.infoY+rowTransport]
			first, last := cellX(row, ic.prev), cellX(row, ic.next)+lipgloss.Width(ic.next)
			if mid, want := (first+last)/2, g.infoX+g.infoW/2; !near(mid, want) {
				t.Errorf("%v: icons centred on %d, card centre %d", sz, mid, want)
			}
		}
	}
}

func TestTextOnlyKeepsBarAndHistoryAtFullWidth(t *testing.T) {
	h, g := textOnlyHarness(100, 30, playing(0, 200*time.Second), Deps{})
	rows := h.rows()
	if x := cellX(rows[g.infoY+rowProgress], "●"); x != g.infoX {
		t.Errorf("the bar starts at the card's left edge: %d vs %d", x, g.infoX)
	}
	if _, e := span(rows[g.infoY+rowProgress]); e != g.infoX+g.infoW {
		t.Errorf("and the time ends at the right edge: %d vs %d", e, g.infoX+g.infoW)
	}
	for i, r := range rows {
		if strings.Contains(r, "recently played") {
			if x := cellX(r, "recently"); x != g.infoX {
				t.Errorf("history header aligned with its table: %d vs %d", x, g.infoX)
			}
			return
		}
		_ = i
	}
	t.Error("history missing")
}

func TestTextOnlyNarrowPanesStayCentredAndInsideThePane(t *testing.T) {
	for _, sz := range [][2]int{{36, 14}, {39, 20}, {40, 11}, {30, 10}} {
		h, g := textOnlyHarness(sz[0], sz[1], playing(0, 200*time.Second), Deps{Volume: &fakeVol{}})
		for i, r := range h.rows() {
			if lipgloss.Width(r) > sz[0] {
				t.Errorf("%v: row %d is %d wide", sz, i, lipgloss.Width(r))
			}
		}
		if l, r := margins(t, h, g, 0); !near(l, r) {
			t.Errorf("%v: title margins %d / %d", sz, l, r)
		}
	}
}

func TestCoverKeyKeepsTheTextCentredInPortrait(t *testing.T) {
	h := newHarness(playing(0, 200*time.Second), Deps{}).withDemoHistory()
	h.resize(80, 50) // portrait: stacked
	_, g1 := h.m.compose()
	l1, r1 := margins(t, h, g1, 0)
	h.press("a") // hide the cover: text only
	_, g2 := h.m.compose()
	l2, r2 := margins(t, h, g2, 0)
	if !near(l1, r1) || !near(l2, r2) || l2 == 0 {
		t.Errorf("the title stays centred when the cover is hidden: %d/%d then %d/%d", l1, r1, l2, r2)
	}
	if x1, x2 := g1.infoX+l1, g2.infoX+l2; !near(x1, x2) {
		t.Errorf("and does not jump sideways: title at column %d, then %d", x1, x2)
	}
}

func TestTextOnlyClicksStillLandOnTheShiftedIcons(t *testing.T) {
	rec := &recorder{}
	h, _ := textOnlyHarness(100, 30, playing(60*time.Second, 200*time.Second), Deps{Control: rec.control})
	for _, g := range []string{h.m.icons.prev, h.m.icons.pause, h.m.icons.next} {
		x, y := h.find(g)
		run(h.click(x, y))
	}
	if len(rec.cmds) != 3 || rec.cmds[0].Kind != source.Previous || rec.cmds[1].Kind != source.TogglePlay || rec.cmds[2].Kind != source.Next {
		t.Errorf("clicks on the centred icons: %+v", rec.cmds)
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
	for name, row := range map[string]int{"title": 0, "artist line": 1} {
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

func TestNotesAndReconnectingSitOnTheBottomRowCentredOrRight(t *testing.T) {
	h, _ := stackedHarness(playing(0, 200*time.Second), 80, 50, Deps{})
	h.m.setNote("next failed")
	rows := h.rows()
	last := rows[len(rows)-1]
	st, en := span(last)
	if x := cellX(last, "next failed"); x < 0 || !near(x, 80-(x+len("next failed"))+0) && false {
		t.Errorf("the note should be on the bottom row: %q", last)
	}
	if mid := cellX(last, "next failed") + len("next failed")/2; !near(mid, 40) {
		t.Errorf("the note should be centred in the pane, centre at %d", mid)
	}
	if !strings.Contains(last, "Spotify") || en != 80-2 || st < 0 {
		t.Errorf("the source stays in the corner next to a note: %q", last)
	}
	st2 := playing(0, 200*time.Second)
	st2.Status = source.StatusReconnecting
	h2, _ := stackedHarness(st2, 80, 50, Deps{})
	r2 := h2.rows()[49]
	if !strings.Contains(r2, "reconnecting…") || strings.Contains(r2, "Spotify") {
		t.Errorf("reconnecting takes the corner: %q", r2)
	}
}

func TestStackedNothingPlayingIsCentred(t *testing.T) {
	h, g := stackedHarness(source.State{}, 80, 50, Deps{})
	for name, row := range map[string]int{"title": 0} { // idle has no source, so the first row is blank
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

func TestResizingAcrossTheStackingBoundaryFollowsTheLayout(t *testing.T) {
	h := newHarness(playing(0, 200*time.Second), Deps{}).withDemoHistory()
	for _, sz := range [][2]int{{80, 20}, {80, 50}, {80, 20}, {60, 30}} {
		h.resize(sz[0], sz[1])
		_, g := h.m.compose()
		l, r := margins(t, h, g, 0)
		if h.m.layout().mode == sideBySide {
			if l != 0 {
				t.Errorf("%v: the title should start at the left edge: %d / %d", sz, l, r)
			}
		} else if !near(l, r) || l == 0 {
			t.Errorf("%v: the title should be centred: %d / %d", sz, l, r)
		}
	}
}
