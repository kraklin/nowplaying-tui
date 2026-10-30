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

func compactHarness(st source.State, w, hgt int, deps Deps) (*harness, geom) {
	h := newHarness(st, deps).withDemoHistory()
	h.send(VolumeMsg{Level: 55})
	h.resize(w, hgt)
	_, g := h.m.compose()
	return h, g
}

func TestCompactKicksInOnlyForReallySmallPanes(t *testing.T) {
	for _, tc := range []struct {
		w, h    int
		compact bool
	}{
		{29, 24, true}, {30, 24, false}, {100, 9, true}, {100, 10, false}, {80, 20, false}, {10, 3, true}, {28, 8, true}, {60, 12, false},
	} {
		h := newHarness(playing(0, 200*time.Second), Deps{})
		h.resize(tc.w, tc.h)
		if got := h.m.layout().compact; got != tc.compact {
			t.Errorf("%dx%d: compact = %v, want %v", tc.w, tc.h, got, tc.compact)
		}
	}
}

func TestCompactCardIsTitleArtistControlsAndTime(t *testing.T) {
	h, g := compactHarness(playing(0, 200*time.Second), 40, 6, Deps{Volume: &fakeVol{}})
	rows := h.rows()
	if len(rows) != 6 {
		t.Fatalf("%d rows", len(rows))
	}
	all := strings.Join(rows, "\n")
	for _, want := range []string{"Low Orbit", "Neon Ferns", "0:00 / 3:20", h.m.icons.pause, h.m.icons.prev, h.m.icons.next} {
		if !strings.Contains(all, want) {
			t.Errorf("compact card is missing %q:\n%s", want, all)
		}
	}
	for _, gone := range []string{"Spotify", "recently played", h.m.icons.volume, "━", "●", "▀", "▄", "Night Signals"[:5] + "x"} {
		if strings.Contains(all, gone) {
			t.Errorf("compact card must not show %q:\n%s", gone, all)
		}
	}
	for name, r := range map[string]int{"title": 0, "artist": 1, "controls": 2} {
		if l, rr := margins(t, h, g, r); !near(l, rr) {
			t.Errorf("%s is not centred: %d / %d", name, l, rr)
		}
	}
	if first, last := blockRows(rows); !(near(first, len(rows)-1-last)) {
		t.Errorf("the three rows should be vertically centred: %d above, %d below", first, len(rows)-1-last)
	}
	if g.progRow != -1 || g.ctrlRow != 2 {
		t.Errorf("geometry: progRow %d ctrlRow %d, want -1 and 2", g.progRow, g.ctrlRow)
	}
}

func TestCompactWithFewerRows(t *testing.T) {
	h2, _ := compactHarness(playing(0, 200*time.Second), 40, 2, Deps{})
	r := h2.rows()
	if !strings.Contains(r[0], "Low Orbit · Neon Ferns") || !strings.Contains(r[1], h2.m.icons.pause) {
		t.Errorf("two rows: title · artist, then controls:\n%s", strings.Join(r, "\n"))
	}
	h1, g1 := compactHarness(playing(0, 200*time.Second), 40, 1, Deps{})
	r = h1.rows()
	if len(r) != 1 || !strings.Contains(r[0], h1.m.icons.pause) || strings.Contains(r[0], "Low Orbit") || g1.ctrlRow != 0 {
		t.Errorf("one row: controls only: %q (ctrlRow %d)", r, g1.ctrlRow)
	}
}

func TestCompactControlsDegradeWithWidthAndNeverOverflow(t *testing.T) {
	for w := 3; w <= 60; w++ {
		h, g := compactHarness(playing(0, 200*time.Second), w, 6, Deps{})
		rows := h.rows()
		for i, r := range rows {
			if lipgloss.Width(r) > w {
				t.Errorf("w=%d: row %d is %d wide", w, i, lipgloss.Width(r))
			}
		}
		ctrl := rows[g.infoY+g.ctrlRow]
		if !strings.Contains(ctrl, h.m.icons.pause) {
			t.Errorf("w=%d: play/pause must always survive: %q", w, ctrl)
		}
	}
	// the order things are dropped in
	for _, tc := range []struct {
		w    int
		time bool
		all5 bool
	}{{40, true, true}, {29, false, true}, {18, false, false}} {
		h, g := compactHarness(playing(0, 200*time.Second), tc.w, 6, Deps{})
		ctrl := h.rows()[g.infoY+g.ctrlRow]
		if got := strings.Contains(ctrl, "/ 3:20"); got != tc.time {
			t.Errorf("w=%d: time shown = %v, want %v: %q", tc.w, got, tc.time, ctrl)
		}
		if got := strings.Contains(ctrl, h.m.icons.back); got != tc.all5 {
			t.Errorf("w=%d: back/forward shown = %v, want %v: %q", tc.w, got, tc.all5, ctrl)
		}
	}
	h, g := compactHarness(playing(0, 200*time.Second), 8, 6, Deps{})
	if ctrl := h.rows()[g.infoY+g.ctrlRow]; strings.Contains(ctrl, h.m.icons.prev) {
		t.Errorf("a very narrow card keeps only play/pause: %q", ctrl)
	}
}

func TestCompactIdleAndLiveStream(t *testing.T) {
	idle, _ := compactHarness(source.State{}, 40, 6, Deps{})
	if v := strings.Join(idle.rows(), "\n"); !strings.Contains(v, "nothing playing") || !strings.Contains(v, "0:00 / 0:00") {
		t.Errorf("idle compact card:\n%s", v)
	}
	live, _ := compactHarness(playing(7*time.Second, 0), 40, 6, Deps{})
	v := strings.Join(live.rows(), "\n")
	if !strings.Contains(v, "0:07") || strings.Contains(v, "/ 0:00") {
		t.Errorf("a stream with no duration shows elapsed only:\n%s", v)
	}
}

func TestCompactNoteTakesTheArtistRow(t *testing.T) {
	h, g := compactHarness(playing(0, 200*time.Second), 40, 6, Deps{})
	h.m.setNote("next failed")
	if row := h.rows()[g.infoY+1]; !strings.Contains(row, "next failed") {
		t.Errorf("a note should replace the artist row: %q", row)
	}
}

func TestCompactMouse(t *testing.T) {
	rec := &recorder{}
	h, g := compactHarness(playing(60*time.Second, 200*time.Second), 40, 6, Deps{Control: rec.control, Volume: &fakeVol{}})
	ic := h.m.icons
	for _, glyph := range []string{ic.prev, ic.back, ic.pause, ic.fwd, ic.next} {
		h.m.seekAt = time.Time{}
		x, y := h.find(glyph)
		if y != g.infoY+g.ctrlRow {
			t.Fatalf("%q drawn on row %d, geometry says %d", glyph, y, g.infoY+g.ctrlRow)
		}
		run(h.click(x, y))
	}
	count := map[source.CmdKind]int{}
	for _, c := range rec.cmds {
		count[c.Kind]++
	}
	if count[source.Previous] != 1 || count[source.Next] != 1 || count[source.TogglePlay] != 1 || count[source.Seek] != 2 {
		t.Errorf("clicking the five compact icons sent %+v", rec.cmds)
	}
	before := len(rec.cmds)
	for name, pos := range map[string][2]int{"title": {g.infoX + 5, g.infoY}, "artist": {g.infoX + 5, g.infoY + 1}, "left edge of the controls": {g.infoX, g.infoY + g.ctrlRow}, "above": {g.infoX + 5, g.infoY - 1}} {
		if cmd := h.click(pos[0], pos[1]); cmd != nil {
			t.Errorf("click on %s should do nothing", name)
		}
	}
	if cmd := h.wheel(g.infoX+10, g.infoY+g.ctrlRow, tea.MouseButtonWheelUp); cmd != nil {
		t.Errorf("there is no volume bar to scroll")
	}
	if len(rec.cmds) != before {
		t.Errorf("stray commands: %+v", rec.cmds[before:])
	}
}

func TestLeavingCompactRestoresTheFullCard(t *testing.T) {
	h := newHarness(playing(0, 200*time.Second), Deps{}).withDemoHistory()
	h.resize(40, 6)
	if strings.Contains(strings.Join(h.rows(), "\n"), "recently played") {
		t.Fatal("compact shows no history")
	}
	h.resize(100, 30)
	v := strings.Join(h.rows(), "\n")
	if !strings.Contains(v, "Spotify") || !strings.Contains(v, "recently played") || !strings.Contains(v, "●") {
		t.Errorf("the full card should be back:\n%s", v)
	}
	h.resize(40, 6)
	if h.m.layout().compact == false {
		t.Errorf("and compact again")
	}
}

// --- the source name sits in the bottom right corner ------------------------------------

// withoutVolume blanks the volume bar out of a bottom row, leaving the rest.
func withoutVolume(h *harness, row string) string {
	return strings.Replace(row, stripANSI(h.m.volumeBlock(accentStyle("#000"))), "", 1)
}

func TestSourceNameIsInTheBottomRightCorner(t *testing.T) {
	for _, tc := range []struct{ bundle, want string }{
		{"com.spotify.client", "Spotify"}, {"org.mozilla.firefox", "Firefox"}, {"com.google.Chrome", "Chrome"}, {"com.example.CoolPlayer", "CoolPlayer"},
	} {
		for _, sz := range [][2]int{{100, 30}, {80, 50}, {60, 24}} {
			st := playing(0, 200*time.Second)
			st.App = tc.bundle
			h, g := stackedHarness(st, sz[0], sz[1], Deps{})
			rows := h.rows()
			raw := rows[len(rows)-1]
			last := withoutVolume(h, raw)
			if strings.TrimSpace(last) != tc.want {
				t.Errorf("%s %v: bottom row %q, want just %q", tc.bundle, sz, last, tc.want)
			}
			if _, end := span(raw); end != sz[0]-2 {
				t.Errorf("%s %v: the name should end 2 cells from the right edge, ends at %d of %d", tc.bundle, sz, end, sz[0])
			}
			if strings.Contains(strings.Join(rows[:len(rows)-1], "\n"), tc.want) {
				t.Errorf("%s %v: the name must appear only in the corner", tc.bundle, sz)
			}
			if title := strings.TrimSpace(rows[g.infoY]); title != "Low Orbit" {
				t.Errorf("%v: the card now starts with the title, got %q", sz, title)
			}
			if strings.Contains(strings.Join(rows, "\n"), "now playing") {
				t.Errorf("no 'now playing' anywhere")
			}
		}
	}
}

func TestNoSourceNameWhenNothingIsPlaying(t *testing.T) {
	h, g := stackedHarness(source.State{}, 100, 30, Deps{})
	rows := h.rows()
	if last := strings.TrimSpace(withoutVolume(h, rows[len(rows)-1])); last != "" {
		t.Errorf("idle has no source: %q", last)
	}
	if title := strings.TrimSpace(rows[g.infoY]); title != "nothing playing" {
		t.Errorf("title row %q", title)
	}
}

func TestSourceKeyAndConfigToggleTheName(t *testing.T) {
	h, _ := stackedHarness(playing(0, 200*time.Second), 100, 30, Deps{})
	if !strings.Contains(h.rows()[29], "Spotify") {
		t.Fatal("shown by default")
	}
	if cmd := h.press("s"); cmd != nil {
		t.Errorf("hiding the source is only a view change")
	}
	if strings.Contains(strings.Join(h.rows(), "\n"), "Spotify") {
		t.Errorf("s should hide it")
	}
	h.press("s")
	if !strings.Contains(h.rows()[29], "Spotify") {
		t.Errorf("s again shows it")
	}
	off := newHarness(playing(0, 200*time.Second), Deps{Config: cfgWith(func(c *config.Config) { c.ShowSource = false })})
	off.resize(100, 30)
	if strings.Contains(strings.Join(off.rows(), "\n"), "Spotify") {
		t.Errorf("show_source = false starts hidden")
	}
	off.press("s")
	if !strings.Contains(off.rows()[29], "Spotify") {
		t.Errorf("and s shows it")
	}
}

func TestReconnectingShowsEvenWhenTheSourceIsHidden(t *testing.T) {
	st := playing(0, 200*time.Second)
	st.Status = source.StatusReconnecting
	h := newHarness(st, Deps{Config: cfgWith(func(c *config.Config) { c.ShowSource = false })})
	h.resize(100, 30)
	if !strings.Contains(h.rows()[29], "reconnecting…") {
		t.Errorf("a stream problem is not something to hide: %q", h.rows()[29])
	}
}

func TestSourceNeverCollidesWithHelpOrNotes(t *testing.T) {
	for w := 30; w <= 140; w += 5 {
		h, _ := stackedHarness(playing(0, 200*time.Second), w, 30, Deps{})
		h.press("?")
		h.m.setNote("history not saved")
		last := h.rows()[29]
		if lipgloss.Width(last) > w {
			t.Errorf("w=%d: the bottom row is %d wide", w, lipgloss.Width(last))
		}
		if !strings.Contains(last, "Spotify") {
			t.Errorf("w=%d: the source must stay visible next to help/notes: %q", w, last)
		}
		idx := strings.Index(last, "Spotify")
		noteEnd := lipgloss.Width(strings.TrimRight(last[:idx], " "))
		if gap := cellX(last, "Spotify") - noteEnd; gap < 2 {
			t.Errorf("w=%d: only %d cells between the note and the source: %q", w, gap, last)
		}
	}
}

func TestCompactStillGetsHelpButNoSource(t *testing.T) {
	h, _ := compactHarness(playing(0, 200*time.Second), 50, 9, Deps{})
	h.press("?")
	last := h.rows()[8]
	if !strings.Contains(last, "space play") || strings.Contains(last, "Spotify") {
		t.Errorf("a free last row shows help, but the compact card has no source: %q", last)
	}
}

func TestHelpMentionsTheSourceKey(t *testing.T) {
	h := newHarness(playing(0, 200*time.Second), Deps{})
	h.resize(160, 30)
	h.press("?")
	if !strings.Contains(h.rows()[29], "s src") {
		t.Errorf("help should list s: %q", h.rows()[29])
	}
}

// At 100 columns the full help and the source name do not fit side by side. The
// help must never lose its last keys (q quit) to make room for a decoration.
func TestHelpWinsOverTheSourceNameWhenTheyDoNotFit(t *testing.T) {
	for _, w := range []int{60, 80, 100, 110} {
		h, _ := stackedHarness(playing(0, 200*time.Second), w, 30, Deps{})
		h.press("?")
		last := h.rows()[29]
		if w >= 100 && !strings.Contains(last, "q quit") {
			t.Errorf("w=%d: the help lost its last keys: %q", w, last)
		}
		if lipgloss.Width(last) > w {
			t.Errorf("w=%d: bottom row is %d wide", w, lipgloss.Width(last))
		}
		fits := lipgloss.Width(helpText) <= w-lipgloss.Width("Spotify")-5
		if both := strings.Contains(last, "q quit") && strings.Contains(last, "Spotify"); both != fits {
			t.Errorf("w=%d: help and source drawn together = %v, but fitting together = %v: %q", w, both, fits, last)
		}
		h.press("?")
		if !strings.Contains(h.rows()[29], "Spotify") {
			t.Errorf("w=%d: the source comes back when the help closes", w)
		}
	}
	wide, _ := stackedHarness(playing(0, 200*time.Second), 160, 30, Deps{})
	wide.press("?")
	if last := wide.rows()[29]; !strings.Contains(last, "q quit") || !strings.Contains(last, "Spotify") {
		t.Errorf("when there is room, both show: %q", last)
	}
}

// --- the volume bar sits in the bottom left corner ----------------------------------

func TestVolumeBarIsInTheBottomLeftCorner(t *testing.T) {
	for _, sz := range [][2]int{{100, 30}, {80, 20}, {80, 50}, {60, 24}} {
		h, g := stackedHarness(playing(0, 200*time.Second), sz[0], sz[1], Deps{Volume: &fakeVol{}})
		rows := h.rows()
		x, y := h.find(h.m.icons.volume)
		if y != sz[1]-1 || x != 2 {
			t.Errorf("%v: volume icon at (%d,%d), want (2,%d)", sz, x, y, sz[1]-1)
		}
		if g.volRow != y || g.volX0 != x {
			t.Errorf("%v: geometry says row %d col %d", sz, g.volRow, g.volX0)
		}
		if strings.Contains(strings.Join(rows[:len(rows)-1], "\n"), h.m.icons.volume) {
			t.Errorf("%v: the volume bar must appear only in the corner", sz)
		}
	}
}

func TestCompactCardHasNoVolumeBar(t *testing.T) {
	h, g := compactHarness(playing(0, 200*time.Second), 40, 6, Deps{Volume: &fakeVol{}})
	if x, _ := h.find(h.m.icons.volume); x >= 0 || g.volRow >= 0 {
		t.Errorf("the compact card has no volume bar")
	}
}

func TestVolumeStaysClearOfNotesAndStepsAsideForHelp(t *testing.T) {
	h, _ := stackedHarness(playing(0, 200*time.Second), 100, 30, Deps{Volume: &fakeVol{}})
	h.m.note = "next failed"
	_, g := h.m.compose()
	last := h.rows()[29]
	if g.volRow < 0 || cellX(last, "next failed") < g.volX1+2 {
		t.Errorf("a note must keep two cells from the volume bar: %q (bar ends %d)", last, g.volX1)
	}
	h.m.note = ""
	h.press("?")
	_, g = h.m.compose()
	if last = h.rows()[29]; !strings.Contains(last, "q quit") || g.volRow >= 0 {
		t.Errorf("the help keeps its keys and the bar steps aside: %q", last)
	}
	h.press("?")
	if _, g = h.m.compose(); g.volRow < 0 {
		t.Errorf("the bar comes back with the help closed")
	}
}

func TestScrollOverTheCornerVolumeBar(t *testing.T) {
	h, g := stackedHarness(playing(0, 200*time.Second), 100, 30, Deps{Volume: &fakeVol{}})
	h.send(VolumeMsg{Level: 50})
	if cmd := h.wheel(g.volX0+3, g.volRow, tea.MouseButtonWheelUp); cmd == nil || h.m.volume != 55 {
		t.Errorf("scrolling over the corner bar adds 5, got %d", h.m.volume)
	}
	before := h.m.volume
	h.wheel(g.volX1, g.volRow, tea.MouseButtonWheelUp)
	h.wheel(g.volX0, g.volRow-1, tea.MouseButtonWheelUp)
	if h.m.volume != before {
		t.Errorf("scrolling beside the bar must do nothing")
	}
}
