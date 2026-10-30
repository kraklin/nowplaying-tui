package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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

// --- the first row is the source, not "now playing" -----------------------------------

func TestFirstRowIsTheSourceName(t *testing.T) {
	for _, tc := range []struct{ bundle, want string }{
		{"com.spotify.client", "Spotify"}, {"org.mozilla.firefox", "Firefox"}, {"com.google.Chrome", "Chrome"}, {"com.example.CoolPlayer", "CoolPlayer"},
	} {
		st := playing(0, 200*time.Second)
		st.App = tc.bundle
		h, g := stackedHarness(st, 100, 30, Deps{})
		all := strings.Join(h.rows(), "\n")
		if strings.Contains(all, "now playing") || strings.Contains(all, "♫  ") {
			t.Errorf("%s: the old label is gone:\n%s", tc.bundle, all)
		}
		row := strings.TrimSpace(h.rows()[g.infoY])
		if row != tc.want {
			t.Errorf("%s: first row %q, want %q", tc.bundle, row, tc.want)
		}
		if l, r := margins(t, h, g, 0); !near(l, r) {
			t.Errorf("%s: the source name should be centred: %d / %d", tc.bundle, l, r)
		}
	}
}

func TestFirstRowIsBlankWhenNothingIsPlaying(t *testing.T) {
	h, g := stackedHarness(source.State{}, 100, 30, Deps{})
	if row := strings.TrimSpace(h.rows()[g.infoY]); row != "" {
		t.Errorf("idle has no source: %q", row)
	}
	if title := strings.TrimSpace(h.rows()[g.infoY+2]); title != "nothing playing" {
		t.Errorf("title row %q", title)
	}
}
