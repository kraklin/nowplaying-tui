package ui

import (
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/tomaslatall/nowplaying-tui/internal/art"
	"github.com/tomaslatall/nowplaying-tui/internal/config"
	"github.com/tomaslatall/nowplaying-tui/internal/history"
	"github.com/tomaslatall/nowplaying-tui/internal/source"
)

func (h *harness) withDemoHistory() *harness {
	h.m.recent = history.Demo()
	return h
}

const placeholderAccentHex = "#c9a7ff"

func plain(m Model) []string { return strings.Split(stripANSI(m.View()), "\n") }

var rgbRe = regexp.MustCompile(`\x1b\[(?:[0-9;]*;)?38;2;(\d+);(\d+);(\d+)m`)

// colourBefore is the foreground colour in effect where word starts in view.
func colourBefore(t *testing.T, view, word string) [3]int {
	t.Helper()
	i := strings.Index(view, word)
	if i < 0 {
		t.Fatalf("%q not in view", word)
	}
	ms := rgbRe.FindAllStringSubmatch(view[:i], -1)
	if len(ms) == 0 {
		t.Fatalf("no colour before %q", word)
	}
	var c [3]int
	for k := range c {
		c[k] = atoi(ms[len(ms)-1][k+1])
	}
	return c
}

func atoi(s string) (n int) {
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}

func sum(c [3]int) int { return c[0] + c[1] + c[2] }

// --- cover off ---------------------------------------------------------------

func TestShowCoverFalseShowsOnlyText(t *testing.T) {
	m := settleFull(t, 100, 30, false, art.DarkBG, cfgWith(func(c *config.Config) { c.ShowCover = false }))
	if m.layout().mode != textOnly || m.layout().coverPx != 0 {
		t.Fatalf("layout = %+v, want text only", m.layout())
	}
	v := m.View()
	if strings.ContainsAny(v, "▀▄") {
		t.Errorf("no cover cells should be drawn")
	}
	for _, want := range []string{"now playing", "Low Orbit", "Neon Ferns", "/ 3:58", "recently played", "Paper Sa"} {
		if !strings.Contains(stripANSI(v), want) {
			t.Errorf("text-only card is missing %q:\n%s", want, stripANSI(v))
		}
	}
	for k := range m.cache {
		if k.px != accentPx {
			t.Errorf("only small accent-sized builds are allowed when no cover is shown, found %dpx", k.px)
		}
	}
}

// Hiding the cover must not freeze the colour scheme: the accent still comes from
// the artwork, it just is not drawn.
func TestAccentStillFollowsTheCoverWhenItIsHidden(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	hidden := settleFull(t, 100, 30, false, art.DarkBG, cfgWith(func(c *config.Config) { c.ShowCover = false }))
	key, _ := hidden.wantArt()
	res, ok := hidden.cache[key]
	if !ok {
		t.Fatal("the small cover for the accent was never built")
	}
	if res.Accent == placeholderAccentHex {
		t.Fatalf("accent is still the default %s; it should come from the track", res.Accent)
	}
	c, _ := config.ParseHex(res.Accent)
	if want := sprintf("38;2;%d;%d;%d", c.R, c.G, c.B); !strings.Contains(hidden.View(), want) {
		t.Errorf("the card should be drawn in the picked accent %s", res.Accent)
	}
	shown := settleFull(t, 100, 30, false, art.DarkBG, nil)
	if k, _ := shown.wantArt(); shown.cache[k].Accent != res.Accent {
		t.Errorf("the accent should not depend on whether the cover is visible: %s vs %s", shown.cache[k].Accent, res.Accent)
	}
}

func TestAccentStillFollowsTheCoverInATinyPane(t *testing.T) {
	m := settleFull(t, 36, 10, false, art.DarkBG, nil) // too small for a cover
	if m.layout().coverPx != 0 {
		t.Fatal("setup: this pane should have no cover")
	}
	key, _ := m.wantArt()
	if res, ok := m.cache[key]; !ok || res.Accent == placeholderAccentHex {
		t.Errorf("a small pane keeps the track's accent too: %+v ok=%v", res.Accent, ok)
	}
}

func TestCoverOffStillShowsAPlaceholderFreeIdleCard(t *testing.T) {
	m := settleFull(t, 100, 30, true, art.DarkBG, cfgWith(func(c *config.Config) { c.ShowCover = false }))
	v := stripANSI(m.View())
	if strings.ContainsAny(v, "▀▄") || strings.Count(v, "♫") != 1 {
		t.Errorf("the idle card should have no placeholder cover either (only the label's ♫):\n%s", v)
	}
	if !strings.Contains(v, "nothing playing") {
		t.Errorf("idle text missing")
	}
}

func TestCoverOffIsCentredAtEverySize(t *testing.T) {
	for _, sz := range [][2]int{{40, 12}, {60, 20}, {80, 20}, {120, 30}, {200, 60}, {100, 70}} {
		h := newHarness(playing(0, 200*time.Second), Deps{Config: cfgWith(func(c *config.Config) { c.ShowCover = false })}).withDemoHistory()
		h.resize(sz[0], sz[1])
		rows := h.rows()
		if len(rows) != sz[1] {
			t.Errorf("%v: %d rows", sz, len(rows))
		}
		first, last := blockRows(rows)
		if above, below := first, len(rows)-1-last; above-below > 1 || below-above > 1 {
			if last < len(rows)-1 { // a card taller than the pane is clipped, not centred
				t.Errorf("%v: should be vertically centred (%d above, %d below)", sz, above, below)
			}
		}
		for i, r := range rows {
			if lipgloss.Width(r) > sz[0] {
				t.Errorf("%v: row %d is %d wide", sz, i, lipgloss.Width(r))
			}
		}
		_, g := h.m.compose()
		if want := (sz[0] - g.infoW) / 2; g.infoX != want {
			t.Errorf("%v: info column at %d, want centred at %d", sz, g.infoX, want)
		}
	}
}

func TestMouseStillWorksWithoutACover(t *testing.T) {
	rec := &recorder{}
	h := newHarness(playing(0, 200*time.Second), Deps{Control: rec.control, Config: cfgWith(func(c *config.Config) { c.ShowCover = false })})
	h.resize(100, 30)
	x, y := h.find(h.m.icons.next)
	run(h.click(x, y))
	_, g := h.m.compose()
	run(h.click(g.infoX+10, g.infoY+rowProgress))
	if len(rec.cmds) != 2 || rec.cmds[0].Kind != source.Next || rec.cmds[1].Kind != source.Seek {
		t.Errorf("clicks should work in text-only mode: %+v", rec.cmds)
	}
}

// --- portrait ----------------------------------------------------------------

func TestPortraitPanesStackEvenWhenWide(t *testing.T) {
	for _, tc := range []struct {
		w, h int
		want layoutMode
		why  string
	}{
		{80, 20, sideBySide, "the usual landscape pane"},
		{120, 30, sideBySide, "wide landscape"},
		{200, 50, sideBySide, "very wide landscape"},
		{80, 40, sideBySide, "exactly square in pixels (40 rows × 2 = 80 columns): not portrait"},
		{80, 41, stacked, "one row taller than square: portrait"},
		{70, 35, sideBySide, "70 columns is still side by side when not portrait"},
		{70, 36, stacked, "70 columns, portrait"},
		{100, 60, stacked, "tall and wide"},
		{120, 70, stacked, "tall and wide"},
		{69, 20, stacked, "under 70 columns always stacks"},
		{50, 30, stacked, "narrow"},
	} {
		h := newHarness(playing(0, 200*time.Second), Deps{}).withDemoHistory()
		h.resize(tc.w, tc.h)
		if got := h.m.layout().mode; got != tc.want {
			t.Errorf("%dx%d (%s): mode %v, want %v", tc.w, tc.h, tc.why, got, tc.want)
		}
	}
}

func TestPortraitStackIsActuallyDrawnStacked(t *testing.T) {
	for _, sz := range [][2]int{{80, 50}, {100, 60}, {120, 70}} {
		m := settle(t, sz[0], sz[1], false)
		if m.layout().mode != stacked {
			t.Fatalf("%v should stack", sz)
		}
		rows := plain(m)
		lastCover, label := -1, -1
		for i, r := range rows {
			if strings.ContainsAny(r, "▀▄") {
				lastCover = i
			}
			if label < 0 && strings.Contains(r, "now playing") {
				label = i
			}
		}
		if lastCover < 0 || label <= lastCover {
			t.Errorf("%v: the cover should sit above the info:\n%s", sz, strings.Join(rows, "\n"))
		}
		first, last := blockRows(rows)
		if above, below := first, len(rows)-1-last; above-below > 1 || below-above > 1 {
			t.Errorf("%v: the stack should be vertically centred (%d above, %d below)", sz, above, below)
		}
	}
}

func TestStackedShowsHistoryWhenThereIsRoom(t *testing.T) {
	tall := settle(t, 80, 50, false)
	v := strings.Join(plain(tall), "\n")
	if !strings.Contains(v, "recently played") || !strings.Contains(v, "Hollow Light") {
		t.Errorf("a tall portrait pane has room for the history:\n%s", v)
	}
	if l := tall.layout(); l.histRows != 5 {
		t.Errorf("all 5 demo entries should fit, got %d rows", l.histRows)
	}
	tight := settle(t, 60, 24, false)
	if l := tight.layout(); l.histRows != 0 || l.coverPx < 16 {
		t.Errorf("when space is tight the cover wins: %+v", l)
	}
}

func TestStackedNeverSqueezesTheCoverForHistory(t *testing.T) {
	for _, w := range []int{40, 50, 60, 69, 80, 100, 140} {
		for _, hgt := range []int{12, 16, 20, 24, 28, 32, 36, 40, 50, 60, 80} {
			h := newHarness(playing(0, 200*time.Second), Deps{}).withDemoHistory()
			h.resize(w, hgt)
			l := h.m.layout()
			if l.mode != stacked {
				continue
			}
			usableW := w - 2*marginX(w)
			comfortable := min(24, usableW) &^ 1
			if l.histRows > 0 && l.coverPx < comfortable {
				t.Errorf("%dx%d: %d history rows left the cover at %dpx (want at least %d)", w, hgt, l.histRows, l.coverPx, comfortable)
			}
			// everything must fit inside the margins
			rows := h.rows()
			first, last := blockRows(rows)
			if first < marginY || last > hgt-1-marginY {
				t.Errorf("%dx%d: card spans rows %d..%d but margins are %d", w, hgt, first, last, marginY)
			}
		}
	}
}

// --- history size, fade and visibility -------------------------------------------

func TestDefaultHistoryKeepsTen(t *testing.T) {
	h := newHarness(track("T0"), Deps{})
	for i := 1; i <= 15; i++ {
		h.advance(30 * time.Second)
		h.next(track(string(rune('A' + i))))
	}
	if len(h.m.recent) != 10 {
		t.Errorf("history_size defaults to 10, got %d entries", len(h.m.recent))
	}
}

func fadeColours(t *testing.T, cfg *config.Config) []int {
	t.Helper()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	h := newHarness(playing(0, 200*time.Second), Deps{Config: cfg}).withDemoHistory()
	h.resize(100, 30)
	v := h.m.View()
	var out []int
	for _, w := range []string{"Paper Sa", "Glass Ha", "Slow Sta", "Salt & S", "Hollow L"} {
		out = append(out, sum(colourBefore(t, v, w)))
	}
	return out
}

func TestHistoryFadeCanBeTurnedOff(t *testing.T) {
	on := fadeColours(t, nil)
	for i := 1; i < len(on); i++ {
		if on[i] >= on[i-1] {
			t.Errorf("with the fade on each row is dimmer than the last: %v", on)
			break
		}
	}
	off := fadeColours(t, cfgWith(func(c *config.Config) { c.HistoryFade = false }))
	for i := 1; i < len(off); i++ {
		if off[i] != off[0] {
			t.Errorf("with the fade off every row has the same strength: %v", off)
			break
		}
	}
	if off[0] != on[0] {
		t.Errorf("the first row is full strength either way: on %v off %v", on[0], off[0])
	}
}

func TestHistoryKeyTogglesTheList(t *testing.T) {
	h := newHarness(playing(0, 200*time.Second), Deps{}).withDemoHistory()
	h.resize(100, 30)
	saved := append([]history.Entry(nil), h.m.recent...)
	if !strings.Contains(h.m.View(), "recently played") {
		t.Fatal("history starts visible")
	}
	if cmd := h.press("r"); cmd != nil {
		t.Errorf("hiding the list is a view change, not a command")
	}
	v := stripANSI(h.m.View())
	if strings.Contains(v, "recently played") || strings.Contains(v, "Glass Harbour") {
		t.Errorf("r should hide the list:\n%s", v)
	}
	if len(h.rows()) != 30 {
		t.Errorf("the card must still fill the pane")
	}
	h.press("r")
	if !strings.Contains(h.m.View(), "recently played") {
		t.Errorf("a second r brings it back")
	}
	if len(h.m.recent) != len(saved) {
		t.Errorf("hiding must not delete anything: %d vs %d", len(h.m.recent), len(saved))
	}
}

func TestShowHistoryFalseStartsHiddenAndRShowsIt(t *testing.T) {
	h := newHarness(playing(0, 200*time.Second), Deps{Config: cfgWith(func(c *config.Config) { c.ShowHistory = false })}).withDemoHistory()
	h.resize(100, 30)
	if strings.Contains(h.m.View(), "recently played") {
		t.Fatal("show_history = false starts with the list hidden")
	}
	h.press("r")
	if !strings.Contains(h.m.View(), "recently played") {
		t.Errorf("r should show it")
	}
}

func TestHistoryStillRecordsWhileHidden(t *testing.T) {
	h := newHarness(track("First"), Deps{})
	h.press("r")
	h.advance(45 * time.Second)
	h.next(track("Second"))
	if len(h.m.recent) != 1 || h.m.recent[0].Title != "First" {
		t.Errorf("hidden is only a view setting; the track should still be recorded: %+v", h.m.recent)
	}
	h.press("r")
	if !strings.Contains(h.m.View(), "First") {
		t.Errorf("and it shows when unhidden")
	}
}

func TestHiddenHistoryInEveryLayout(t *testing.T) {
	for _, sz := range [][2]int{{100, 30}, {80, 20}, {60, 40}, {80, 50}, {45, 24}, {38, 20}} {
		h := newHarness(playing(0, 200*time.Second), Deps{}).withDemoHistory()
		h.resize(sz[0], sz[1])
		h.press("r")
		if l := h.m.layout(); l.histRows != 0 {
			t.Errorf("%v: hidden history must take no rows, got %d", sz, l.histRows)
		}
		if strings.Contains(strings.Join(h.rows(), "\n"), "recently played") {
			t.Errorf("%v: header still drawn", sz)
		}
		first, last := blockRows(h.rows())
		if first < 0 || last < first {
			t.Errorf("%v: nothing drawn", sz)
		}
	}
}

// Beside the cover the card is as tall as the cover, so hiding the list does not
// shrink it; the info column re-centres against the cover instead.
func TestHidingHistoryRecentresTheInfoColumnAgainstTheCover(t *testing.T) {
	m := settle(t, 100, 30, false)
	_, before := m.compose()
	tm, _ := m.Update(keyRunes("r"))
	hidden := tm.(Model)
	_, after := hidden.compose()
	if after.infoY <= before.infoY {
		t.Errorf("a shorter info column should sit lower, centred on the cover: infoY %d -> %d", before.infoY, after.infoY)
	}
	rows := plain(hidden)
	coverTop, coverBottom, label := -1, -1, -1
	for i, r := range rows {
		if strings.ContainsAny(r, "▀▄") {
			if coverTop < 0 {
				coverTop = i
			}
			coverBottom = i
		}
		if label < 0 && strings.Contains(r, "now playing") {
			label = i
		}
	}
	infoRows := 8 // label … transport, no history block
	coverRows := coverBottom - coverTop + 1
	if want := coverTop + (coverRows-infoRows)/2; label != want {
		t.Errorf("info should start at row %d to centre on the cover (rows %d..%d), got %d", want, coverTop, coverBottom, label)
	}
	if first, last := blockRows(rows); first != coverTop || last != coverBottom {
		t.Errorf("the whole card is still exactly as tall as the cover: %d..%d vs %d..%d", first, last, coverTop, coverBottom)
	}
}

func TestMouseStillWorksWhenHistoryIsHidden(t *testing.T) {
	rec := &recorder{}
	h := newHarness(playing(0, 200*time.Second), Deps{Control: rec.control}).withDemoHistory()
	h.resize(100, 30)
	h.press("r")
	x, y := h.find(h.m.icons.next)
	run(h.click(x, y))
	if len(rec.cmds) != 1 || rec.cmds[0].Kind != source.Next {
		t.Errorf("hit-testing must follow the re-centred card: %+v", rec.cmds)
	}
}

func TestHelpMentionsTheHistoryKey(t *testing.T) {
	h := newHarness(playing(0, 200*time.Second), Deps{})
	h.resize(120, 30)
	h.press("?")
	rows := h.rows()
	if got := rows[len(rows)-1]; !strings.Contains(got, "r history") || !strings.Contains(got, "q quit") {
		t.Errorf("help should list the new key: %q", got)
	}
}

func keyRunes(k string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)} }

// On a fresh install the history starts empty and fills up. In a tight stacked
// pane the cover must not shrink step by step while that happens.
func TestStackedCoverDoesNotResizeAsHistoryFills(t *testing.T) {
	for _, sz := range [][2]int{{60, 30}, {50, 28}, {80, 46}, {70, 40}, {66, 36}} {
		h := newHarness(playing(0, 200*time.Second), Deps{})
		h.resize(sz[0], sz[1])
		if h.m.layout().mode != stacked {
			t.Fatalf("%v should stack", sz)
		}
		demo := history.Demo()
		want := h.m.layout().coverPx
		prevRows := -1
		for n := 0; n <= len(demo); n++ {
			h.m.recent = demo[:n]
			l := h.m.layout()
			if l.coverPx != want {
				t.Errorf("%v: the cover is %dpx with %d entries but %dpx with none", sz, l.coverPx, n, want)
			}
			if l.histRows < prevRows {
				t.Errorf("%v: visible rows went down as entries were added (%d -> %d)", sz, prevRows, l.histRows)
			}
			if l.histRows > n {
				t.Errorf("%v: %d rows shown for %d entries", sz, l.histRows, n)
			}
			prevRows = l.histRows
		}
	}
}

func TestHidingHistoryNeverShrinksTheStackedCover(t *testing.T) {
	h := newHarness(playing(0, 200*time.Second), Deps{}).withDemoHistory()
	h.resize(60, 30)
	shown := h.m.layout().coverPx
	h.press("r")
	if hidden := h.m.layout().coverPx; hidden < shown {
		t.Errorf("hiding the list gives its space back to the cover: %dpx -> %dpx", shown, hidden)
	}
}
