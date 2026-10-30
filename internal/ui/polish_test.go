package ui

import (
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/tomaslatall/nowplaying-tui/internal/config"
	"github.com/tomaslatall/nowplaying-tui/internal/source"
)

func (h *harness) resize(w, hgt int) { h.send(tea.WindowSizeMsg{Width: w, Height: hgt}) }

func (h *harness) rows() []string { return strings.Split(stripANSI(h.m.View()), "\n") }

// cellX is the screen column where needle starts in row, or -1.
func cellX(row, needle string) int {
	i := strings.Index(row, needle)
	if i < 0 {
		return -1
	}
	return lipgloss.Width(row[:i])
}

func (h *harness) find(needle string) (x, y int) {
	for y, r := range h.rows() {
		if x := cellX(r, needle); x >= 0 {
			return x, y
		}
	}
	return -1, -1
}

func (h *harness) click(x, y int) tea.Cmd {
	return h.send(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
}

func (h *harness) wheel(x, y int, b tea.MouseButton) tea.Cmd {
	return h.send(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: b})
}

var timeRe = regexp.MustCompile(`\d+:\d\d / \d+:\d\d`)

func (h *harness) progressRow() (row int, text string) {
	for y, r := range h.rows() {
		if timeRe.MatchString(r) {
			return y, r
		}
	}
	return -1, ""
}

// --- layout geometry ---------------------------------------------------

// The mouse code locates things from compose()'s geometry. This checks that
// geometry against where the characters are actually drawn.
func TestGeometryMatchesWhatIsDrawn(t *testing.T) {
	for _, sz := range [][2]int{{80, 20}, {120, 30}, {200, 50}, {60, 30}, {50, 26}, {45, 14}, {38, 20}, {81, 21}, {99, 33}} {
		h := newHarness(playing(0, 200*time.Second), Deps{Volume: &fakeVol{}})
		h.resize(sz[0], sz[1])
		h.send(VolumeMsg{Level: 50})
		_, g := h.m.compose()

		py, _ := h.progressRow()
		if py != g.infoY+rowProgress {
			t.Errorf("%v: progress is drawn on row %d, geometry says %d", sz, py, g.infoY+rowProgress)
		}
		kx, ky := h.find("●")
		if ky != py || kx < g.infoX || kx >= g.infoX+h.m.barWidth(g.infoW) {
			t.Errorf("%v: knob at (%d,%d) should sit on the progress row inside the bar [%d,%d)", sz, kx, ky, g.infoX, g.infoX+h.m.barWidth(g.infoW))
		}
		_, hits, vol := h.m.transportRow(accentStyle("#000000"), g.infoW)
		ic := h.m.icons
		for name, glyph := range map[string]string{"previous": ic.prev, "backward": ic.back, "play/pause": ic.pause, "forward": ic.fwd, "next": ic.next} {
			x, y := h.find(glyph)
			if y != g.infoY+rowTransport {
				t.Errorf("%v: %s drawn on row %d, geometry says %d", sz, name, y, g.infoY+rowTransport)
			}
			inside := false
			for _, ht := range hits {
				inside = inside || (x-g.infoX >= ht.x0 && x-g.infoX < ht.x1)
			}
			if !inside {
				t.Errorf("%v: %s drawn at column %d is not inside any hit span %v", sz, name, x-g.infoX, hits)
			}
		}
		if vx, _ := h.find(ic.volume); vol[0] >= 0 && vx != g.infoX+vol[0] {
			t.Errorf("%v: volume drawn at %d, geometry says %d", sz, vx, g.infoX+vol[0])
		}
	}
}

func TestLayoutInvariantsAcrossSizes(t *testing.T) {
	for _, w := range []int{20, 30, 39, 40, 45, 60, 69, 70, 71, 80, 100, 120, 160, 200} {
		for _, hgt := range []int{4, 8, 11, 12, 14, 16, 20, 24, 30, 40, 60} {
			h := newHarness(playing(0, 200*time.Second), Deps{})
			h.m.recent = append(h.m.recent, h.m.recent...)
			h.resize(w, hgt)
			l := h.m.layout()

			switch {
			case w < 40 || hgt < 12:
				if l.mode != textOnly {
					t.Errorf("%dx%d: under 40 cols or 12 rows the cover is hidden", w, hgt)
				}
			case l.mode == sideBySide && (w < 70 || hgt*2 > w):
				t.Errorf("%dx%d: side by side needs 70+ columns and a landscape pane", w, hgt)
			case l.mode == stacked && w >= 70 && hgt*2 <= w:
				t.Errorf("%dx%d: a wide landscape pane should not stack", w, hgt)
			}
			if l.coverPx != 0 {
				if l.coverPx%2 != 0 || l.coverPx < 8 || l.coverPx > h.m.cfg.ArtMaxPx {
					t.Errorf("%dx%d: cover %dpx must be even, 8..%d", w, hgt, l.coverPx, h.m.cfg.ArtMaxPx)
				}
				if l.mode == sideBySide {
					if l.coverPx > w*45/100 {
						t.Errorf("%dx%d: cover %dpx is over 45%% of the width", w, hgt, l.coverPx)
					}
					if l.infoW < minInfoW {
						t.Errorf("%dx%d: info column %d is under the minimum %d", w, hgt, l.infoW, minInfoW)
					}
				}
			}

			rows := h.rows()
			if len(rows) != hgt {
				t.Errorf("%dx%d: %d rows", w, hgt, len(rows))
			}
			for i, r := range rows {
				if lipgloss.Width(r) > w {
					t.Errorf("%dx%d: row %d is %d wide", w, hgt, i, lipgloss.Width(r))
				}
			}
		}
	}
}

func TestHistoryNeverRunsPastThePane(t *testing.T) {
	h := newHarness(playing(0, 200*time.Second), Deps{})
	for i := 0; i < 30; i++ {
		h.m.recent = append(h.m.recent, h.m.recent...)
		if len(h.m.recent) > 60 {
			break
		}
	}
	for _, hgt := range []int{12, 16, 20, 30, 45} {
		h.resize(100, hgt)
		if got := len(h.rows()); got != hgt {
			t.Errorf("height %d: drew %d rows", hgt, got)
		}
	}
}

// --- mouse -------------------------------------------------------------

func TestClickTransportIcons(t *testing.T) {
	for name, want := range map[string]source.CmdKind{"previous": source.Previous, "play/pause": source.TogglePlay, "next": source.Next} {
		rec := &recorder{}
		h := newHarness(playing(0, 200*time.Second), Deps{Control: rec.control})
		h.resize(80, 20)
		glyph := map[string]string{"previous": h.m.icons.prev, "play/pause": h.m.icons.pause, "next": h.m.icons.next}[name]
		x, y := h.find(glyph)
		if x < 0 {
			t.Fatalf("%s icon not found", name)
		}
		run(h.click(x, y))
		if len(rec.cmds) != 1 || rec.cmds[0].Kind != want {
			t.Errorf("clicking %s sent %+v, want kind %v", name, rec.cmds, want)
		}
	}
}

func TestClickPlayIconWhenPaused(t *testing.T) {
	rec := &recorder{}
	st := playing(30*time.Second, 200*time.Second)
	st.Playing, st.Rate = false, 0
	h := newHarness(st, Deps{Control: rec.control})
	x, y := h.find(h.m.icons.play)
	run(h.click(x, y))
	if len(rec.cmds) != 1 || rec.cmds[0].Kind != source.TogglePlay {
		t.Errorf("the play icon should toggle: %+v", rec.cmds)
	}
}

func TestClicksElsewhereDoNothing(t *testing.T) {
	rec := &recorder{}
	h := newHarness(playing(0, 200*time.Second), Deps{Control: rec.control, Volume: &fakeVol{}})
	h.send(VolumeMsg{Level: 50})
	_, g := h.m.compose()
	px, py := h.find(h.m.icons.prev)
	for name, pos := range map[string][2]int{
		"title":              {g.infoX + 2, g.infoY + 2},
		"artist":             {g.infoX + 2, g.infoY + 3},
		"label":              {g.infoX + 2, g.infoY},
		"past the last icon": {g.infoX + 22, py},
		"between icons":      {px + 2, py},
		"cover":              {2, g.infoY + 3},
		"left of the pane":   {0, 0},
		"below everything":   {g.infoX + 3, 19},
		"time text":          {g.infoX + g.infoW - 3, g.infoY + rowProgress},
		"blank row":          {g.infoX + 3, g.infoY + 4},
		"far outside":        {500, 500},
		"negative":           {-1, -1},
	} {
		if cmd := h.click(pos[0], pos[1]); cmd != nil {
			t.Errorf("click on %s at %v should do nothing", name, pos)
		}
	}
	if len(rec.cmds) != 0 {
		t.Errorf("no commands expected, got %+v", rec.cmds)
	}
}

func TestOnlyLeftPressActs(t *testing.T) {
	rec := &recorder{}
	h := newHarness(playing(0, 200*time.Second), Deps{Control: rec.control})
	x, y := h.find(h.m.icons.next)
	for name, msg := range map[string]tea.MouseMsg{
		"release":     {X: x, Y: y, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft},
		"motion":      {X: x, Y: y, Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft},
		"right click": {X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonRight},
		"middle":      {X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonMiddle},
	} {
		if cmd := h.send(msg); cmd != nil {
			t.Errorf("%s should not trigger anything", name)
		}
	}
}

func TestClickProgressBarSeeks(t *testing.T) {
	rec := &recorder{}
	h := newHarness(playing(0, 200*time.Second), Deps{Control: rec.control})
	h.resize(100, 24)
	_, g := h.m.compose()
	bw := h.m.barWidth(g.infoW)
	y := g.infoY + rowProgress

	run(h.click(g.infoX, y))          // left end
	run(h.click(g.infoX+bw-1, y))     // right end
	run(h.click(g.infoX+(bw-1)/2, y)) // middle
	got := seekTargets(rec)
	if len(got) != 3 {
		t.Fatalf("seek targets %v", got)
	}
	if got[0] != 0 || got[1] != 200 {
		t.Errorf("ends of the bar should be 0 and the duration, got %v", got[:2])
	}
	if got[2] < 98 || got[2] > 102 {
		t.Errorf("the middle should be about 100 s, got %v", got[2])
	}
	// every column maps to a non-decreasing position
	rec2 := &recorder{}
	h2 := newHarness(playing(0, 200*time.Second), Deps{Control: rec2.control})
	h2.resize(100, 24)
	for c := 0; c < bw; c++ {
		h2.m.seekAt = time.Time{}
		run(h2.click(g.infoX+c, y))
	}
	prev := -1.0
	for _, s := range seekTargets(rec2) {
		if s < prev {
			t.Fatalf("bar positions must not go backwards: %v", seekTargets(rec2))
		}
		prev = s
	}
}

func TestClickSeekLandsUnderTheKnob(t *testing.T) {
	// Click a column, apply the seek the way the stream would, and the knob should be there.
	rec := &recorder{}
	h := newHarness(playing(0, 200*time.Second), Deps{Control: rec.control})
	h.resize(100, 24)
	_, g := h.m.compose()
	bw := h.m.barWidth(g.infoW)
	for _, col := range []int{0, 7, bw / 2, bw - 2} {
		rec.cmds = nil
		h.m.seekAt = time.Time{}
		run(h.click(g.infoX+col, g.infoY+rowProgress))
		target := time.Duration(seekTargets(rec)[0] * float64(time.Second))
		h.send(StateMsg(playing(target, 200*time.Second)))
		kx, _ := h.find("●")
		if d := kx - (g.infoX + col); d < -1 || d > 1 {
			t.Errorf("clicked column %d but the knob ended at %d (off by %d)", col, kx-g.infoX, d)
		}
	}
}

func TestClickProgressIgnoredWithoutDuration(t *testing.T) {
	rec := &recorder{}
	live := newHarness(playing(5*time.Second, 0), Deps{Control: rec.control})
	_, g := live.m.compose()
	if cmd := live.click(g.infoX+3, g.infoY+rowProgress); cmd != nil {
		t.Errorf("a stream with no duration cannot be seeked")
	}
	empty := newHarness(source.State{}, Deps{Control: rec.control})
	_, g = empty.m.compose()
	if cmd := empty.click(g.infoX+3, g.infoY+rowProgress); cmd != nil {
		t.Errorf("nothing playing, nothing to seek")
	}
}

func TestScrollOverVolumeBar(t *testing.T) {
	fv := &fakeVol{}
	h := newHarness(playing(0, 200*time.Second), Deps{Volume: fv})
	h.send(VolumeMsg{Level: 50})
	x, y := h.find(h.m.icons.volume)
	if x < 0 {
		t.Fatal("volume bar not drawn")
	}
	_, g := h.m.compose()
	_, _, vol := h.m.transportRow(accentStyle("#000000"), g.infoW)

	cmd := h.wheel(x+3, y, tea.MouseButtonWheelUp)
	if cmd == nil || h.m.volume != 55 {
		t.Fatalf("scrolling up over the bar adds 5, got volume %d", h.m.volume)
	}
	h.send(run(cmd))
	h.wheel(g.infoX+vol[1]-1, y, tea.MouseButtonWheelDown) // last cell of the bar
	if h.m.volume != 50 {
		t.Errorf("scrolling down over the bar's last cell subtracts 5, got %d", h.m.volume)
	}
	before := h.m.volume
	h.wheel(x-2, y, tea.MouseButtonWheelUp)   // just left of the icon
	h.wheel(x+3, y-1, tea.MouseButtonWheelUp) // row above
	h.wheel(g.infoX, g.infoY+2, tea.MouseButtonWheelUp)
	if h.m.volume != before {
		t.Errorf("scrolling off the bar must not change the volume")
	}
}

func TestScrollWithoutVolumeDoesNothing(t *testing.T) {
	h := newHarness(playing(0, 200*time.Second), Deps{})
	x, y := h.find(h.m.icons.volume)
	if cmd := h.wheel(x+3, y, tea.MouseButtonWheelUp); cmd != nil {
		t.Errorf("no volume control, nothing to scroll")
	}
}

// --- help line ----------------------------------------------------------

func TestHelpLineToggles(t *testing.T) {
	h := newHarness(playing(0, 200*time.Second), Deps{})
	h.resize(140, 24)
	before := h.rows()
	if strings.Contains(strings.Join(before, "\n"), "space play/pause") {
		t.Fatal("help starts hidden")
	}
	h.press("?")
	after := h.rows()
	if !strings.Contains(after[len(after)-1], "space play/pause") || !strings.Contains(after[len(after)-1], "q quit") {
		t.Errorf("help should fill the bottom row, got %q", after[len(after)-1])
	}
	for i := 0; i < len(after)-1; i++ {
		if after[i] != before[i] {
			t.Errorf("the help line must not move the card (row %d changed)", i)
		}
	}
	if st, en := span(after[len(after)-1]); st < 5 || !near(st, 140-en) {
		t.Errorf("help should be centred in the pane: %d empty cells left, %d right", st, 140-en)
	}
	h.press("?")
	if strings.Contains(h.m.View(), "space play/pause") {
		t.Errorf("a second ? hides it")
	}
}

func TestHelpLineFitsNarrowPanes(t *testing.T) {
	for _, w := range []int{20, 30, 40, 60, 90} {
		h := newHarness(playing(0, 200*time.Second), Deps{})
		h.resize(w, 24)
		h.press("?")
		rows := h.rows()
		if got := lipgloss.Width(rows[len(rows)-1]); got > w || got == 0 {
			t.Errorf("width %d: help is %d wide", w, got)
		}
	}
}

func TestHelpDoesNotCoverTheCardInShortPanes(t *testing.T) {
	h := newHarness(playing(0, 200*time.Second), Deps{})
	h.resize(80, 10)
	h.press("?")
	if !strings.Contains(h.m.View(), "now playing") || !strings.Contains(h.m.View(), "Low Orbit") {
		t.Errorf("the card must stay visible:\n%s", h.m.View())
	}
}

// --- states ---------------------------------------------------------------

func TestMissingMediaControlShowsInstallHint(t *testing.T) {
	for _, sz := range [][2]int{{80, 20}, {40, 12}, {120, 40}, {30, 8}} {
		h := newHarness(source.State{Status: source.StatusMissing}, Deps{})
		h.resize(sz[0], sz[1])
		v := stripANSI(h.m.View())
		if sz[0] >= 50 {
			for _, want := range []string{"media-control not found", "brew tap ungive/media-control", "brew install media-control", "q to quit"} {
				if !strings.Contains(v, want) {
					t.Errorf("%v: missing %q:\n%s", sz, want, v)
				}
			}
		}
		rows := strings.Split(v, "\n")
		if len(rows) != sz[1] {
			t.Errorf("%v: %d rows", sz, len(rows))
		}
		for i, r := range rows {
			if lipgloss.Width(r) > sz[0] {
				t.Errorf("%v: row %d is %d wide", sz, i, lipgloss.Width(r))
			}
		}
		if strings.Contains(v, "now playing") || strings.Contains(v, "▀") {
			t.Errorf("%v: the card should not show when media-control is missing", sz)
		}
	}
}

func TestMissingMediaControlIsCentredAndQuits(t *testing.T) {
	h := newHarness(source.State{Status: source.StatusMissing}, Deps{})
	h.resize(100, 30)
	rows := h.rows()
	first, last := -1, -1
	for i, r := range rows {
		if strings.TrimSpace(r) != "" {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if top, bottom := first, len(rows)-1-last; top-bottom > 1 || bottom-top > 1 {
		t.Errorf("message should be vertically centred: %d rows above, %d below", top, bottom)
	}
	if x := cellX(rows[first], "media-control not found"); x < 20 {
		t.Errorf("message should be horizontally centred, starts at column %d", x)
	}
	if cmd := h.press("q"); cmd == nil {
		t.Fatal("q must quit")
	} else if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("q should send QuitMsg")
	}
	if cmd := h.click(50, 15); cmd != nil {
		t.Errorf("clicks do nothing on this screen")
	}
}

func TestReconnectingIsShownWithoutWrapping(t *testing.T) {
	st := playing(30*time.Second, 200*time.Second)
	st.Status = source.StatusReconnecting
	for _, w := range []int{50, 60, 80, 120} {
		h := newHarness(st, Deps{})
		h.resize(w, 24)
		v := stripANSI(h.m.View())
		if !strings.Contains(v, "reconnecting…") {
			t.Errorf("w=%d: should say reconnecting:\n%s", w, v)
		}
		if strings.Contains(v, "Spotify") {
			t.Errorf("w=%d: the reconnecting tag replaces the app tag", w)
		}
		if !strings.Contains(v, "Low Orbit") {
			t.Errorf("w=%d: the last track stays visible", w)
		}
		if strings.Count(v, "♫") != 1 {
			t.Errorf("w=%d: label row wrapped:\n%s", w, v)
		}
	}
	ok := newHarness(playing(0, 200*time.Second), Deps{})
	if strings.Contains(ok.m.View(), "reconnecting") {
		t.Errorf("a healthy source shows no reconnecting tag")
	}
}

// --- config effects ---------------------------------------------------------

func cfgWith(f func(*config.Config)) *config.Config {
	c := config.Default()
	f(&c)
	return &c
}

func TestIconSetsFromConfig(t *testing.T) {
	for set, want := range map[string][]string{
		"unicode": {"⏮", "<", "⏸", ">", "⏭"},
		"basic":   {"|◂", "◂◂", "▮▮", "▸▸", "▸|"},
		"nerd":    {"\uf048", "\uf049", "\uf04c", "\uf050", "\uf051"},
	} {
		h := newHarness(playing(0, 200*time.Second), Deps{Config: cfgWith(func(c *config.Config) { c.Icons = set })})
		v := h.m.View()
		for _, g := range want {
			if !strings.Contains(v, g) {
				t.Errorf("icons=%s: missing %q", set, g)
			}
		}
	}
}

func TestIconSetsKeepTheRowSteady(t *testing.T) {
	for _, set := range []string{"unicode", "basic", "nerd"} {
		ic := iconSet(set)
		if lipgloss.Width(ic.play) != lipgloss.Width(ic.pause) {
			t.Errorf("%s: play (%d) and pause (%d) must be the same width or the row shifts when you pause", set, lipgloss.Width(ic.play), lipgloss.Width(ic.pause))
		}
		for _, g := range []string{ic.prev, ic.back, ic.play, ic.pause, ic.fwd, ic.next, ic.volume} {
			if w := lipgloss.Width(g); w < 1 || w > 2 {
				t.Errorf("%s: %q is %d cells wide", set, g, w)
			}
		}
	}
	if w := lipgloss.Width("⏮⏵⏸⏭"); w != 4 {
		t.Errorf("⏮ ⏵ ⏸ ⏭ must measure one cell each, got %d for four", w)
	}
}

func TestPlayPauseSwapKeepsRowWidth(t *testing.T) {
	for _, set := range []string{"unicode", "basic", "nerd"} {
		cfg := cfgWith(func(c *config.Config) { c.Icons = set })
		pl := newHarness(playing(0, 200*time.Second), Deps{Config: cfg})
		st := playing(0, 200*time.Second)
		st.Playing, st.Rate = false, 0
		pa := newHarness(st, Deps{Config: cfg})
		_, y := pl.find(pl.m.icons.next)
		_, y2 := pa.find(pa.m.icons.next)
		x1, _ := pl.find(pl.m.icons.next)
		x2, _ := pa.find(pa.m.icons.next)
		if y != y2 || x1 != x2 {
			t.Errorf("%s: the next icon moved from %d to %d when pausing", set, x1, x2)
		}
	}
}

func TestAccentOverride(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	def := newHarness(playing(0, 200*time.Second), Deps{})
	if strings.Contains(def.m.View(), "38;2;255;0;0") {
		t.Fatal("setup: red should not appear by default")
	}
	h := newHarness(playing(0, 200*time.Second), Deps{Config: cfgWith(func(c *config.Config) { c.Accent = "#ff0000" })})
	if !strings.Contains(h.m.View(), "38;2;255;0;0") {
		t.Errorf("accent = \"#ff0000\" should colour the card red")
	}
}

func TestArtMaxPxFromConfig(t *testing.T) {
	for _, tc := range []struct{ cap, w, h, want int }{{16, 120, 40, 16}, {24, 120, 40, 24}, {48, 200, 60, 48}, {32, 200, 60, 32}} {
		h := newHarness(playing(0, time.Minute), Deps{Config: cfgWith(func(c *config.Config) { c.ArtMaxPx = tc.cap })})
		h.resize(tc.w, tc.h)
		if got := h.m.layout().coverPx; got != tc.want {
			t.Errorf("art_max_px=%d at %dx%d: cover %dpx, want %d", tc.cap, tc.w, tc.h, got, tc.want)
		}
	}
}

func TestQuantizeAndScanlinesFromConfig(t *testing.T) {
	h := newHarness(playing(0, time.Minute), Deps{Config: cfgWith(func(c *config.Config) { c.QuantizeColors, c.Scanlines = 0, false })})
	if h.m.opts.Quantize != 0 || h.m.opts.Scanlines {
		t.Errorf("config should reach the art options: %+v", h.m.opts)
	}
	d := newHarness(playing(0, time.Minute), Deps{})
	if d.m.opts.Quantize != 12 || !d.m.opts.Scanlines {
		t.Errorf("defaults: %+v", d.m.opts)
	}
}

func TestMinPlayAndHistorySizeFromConfig(t *testing.T) {
	h := newHarness(track("A"), Deps{Config: cfgWith(func(c *config.Config) { c.MinPlaySeconds = 0 })})
	h.advance(time.Second)
	h.next(track("B"))
	if len(h.m.recent) != 1 {
		t.Errorf("min_play_seconds = 0 records every track, got %d entries", len(h.m.recent))
	}
	slow := newHarness(track("A"), Deps{Config: cfgWith(func(c *config.Config) { c.MinPlaySeconds = 60 })})
	slow.advance(30 * time.Second)
	slow.next(track("B"))
	if len(slow.m.recent) != 0 {
		t.Errorf("min_play_seconds = 60 should ignore a 30 s listen")
	}
}

func TestConfigWarningsAreShownThenFade(t *testing.T) {
	h := newHarness(playing(0, time.Minute), Deps{Warnings: []string{"config: icons must be ok", "config: second", "config: third"}})
	v := stripANSI(h.m.View())
	if !strings.Contains(v, "config: icons must be") || !strings.Contains(v, "+2 more") {
		t.Errorf("the first warning and a count should show:\n%s", v)
	}
	if strings.Count(v, "♫") != 1 {
		t.Errorf("the warning must not wrap the label row:\n%s", v)
	}
	h.send(TickMsg(time.Now().Add(warnFor + time.Second))) // New stamps the note with the real clock
	if strings.Contains(h.m.View(), "config:") {
		t.Errorf("warnings fade after a few seconds")
	}
}

func TestBrokenDefaultsNeverPanic(t *testing.T) {
	// A hand-built zero config (not from Load) must not crash the layout.
	var zero config.Config
	h := newHarness(playing(0, time.Minute), Deps{Config: &zero})
	h.resize(100, 30)
	_ = h.m.View()
}

// --- composition rules --------------------------------------------------------

func blockRows(rows []string) (first, last int) {
	first, last = -1, -1
	for i, r := range rows {
		if strings.TrimSpace(r) != "" {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	return
}

func TestStackedLayoutRules(t *testing.T) {
	for _, sz := range [][2]int{{60, 30}, {50, 26}, {65, 34}} {
		m := settle(t, sz[0], sz[1], false)
		if m.layout().mode != stacked {
			t.Fatalf("%v should stack", sz)
		}
		rows := strings.Split(stripANSI(m.View()), "\n")
		lastCover, label := -1, -1
		for i, r := range rows {
			if strings.ContainsAny(r, "▀▄") {
				lastCover = i
			}
			if label < 0 && strings.Contains(r, "now playing") {
				label = i
			}
		}
		if lastCover < 0 || label < 0 {
			t.Fatalf("%v: cover or label missing:\n%s", sz, strings.Join(rows, "\n"))
		}
		if label-lastCover != 2 {
			t.Errorf("%v: expected exactly one blank row between cover and info, got %d", sz, label-lastCover-1)
		}
		first, last := blockRows(rows)
		if above, below := first, len(rows)-1-last; above-below > 1 || below-above > 1 {
			t.Errorf("%v: the card should be vertically centred (%d above, %d below)", sz, above, below)
		}
		// cover and info are centred on the same axis
		cx := cellX(rows[lastCover], "▀")
		ix := cellX(rows[label], "♫")
		coverW := m.layout().coverPx
		if mid := cx + coverW/2; mid < ix || mid > ix+m.layout().infoW {
			t.Errorf("%v: cover (centre %d) and info column (%d..%d) should line up", sz, mid, ix, ix+m.layout().infoW)
		}
	}
}

func TestSideBySideLayoutRules(t *testing.T) {
	for _, sz := range [][2]int{{80, 20}, {120, 30}, {100, 24}} {
		m := settle(t, sz[0], sz[1], false)
		l := m.layout()
		if l.mode != sideBySide {
			t.Fatalf("%v should be side by side", sz)
		}
		rows := strings.Split(stripANSI(m.View()), "\n")
		firstCover, lastCover, label := -1, -1, -1
		for i, r := range rows {
			if strings.ContainsAny(r, "▀▄") {
				if firstCover < 0 {
					firstCover = i
				}
				lastCover = i
			}
			if label < 0 && strings.Contains(r, "now playing") {
				label = i
			}
		}
		if got := lastCover - firstCover + 1; got != l.coverPx/2 {
			t.Errorf("%v: cover spans %d rows, want %d (2 px per cell)", sz, got, l.coverPx/2)
		}
		first, last := blockRows(rows)
		if above, below := first, len(rows)-1-last; above-below > 1 || below-above > 1 {
			t.Errorf("%v: the card should be vertically centred (%d above, %d below)", sz, above, below)
		}
		// the gap between the columns is exactly 4 cells
		cx := cellX(rows[firstCover+1], "▀") // second row avoids the rounded corners
		lastBlock := strings.LastIndexAny(rows[firstCover+1], "▀▄")
		coverEnd := lipgloss.Width(rows[firstCover+1][:lastBlock]) + 1
		if cx < 0 || coverEnd-cx != l.coverPx {
			t.Errorf("%v: cover is %d cells wide, want %d", sz, coverEnd-cx, l.coverPx)
		}
		if lx := cellX(rows[label], "♫"); lx-coverEnd != gap {
			t.Errorf("%v: %d cells between cover and info, want %d", sz, lx-coverEnd, gap)
		}
	}
}

// --- back / forward buttons -----------------------------------------------------

func TestTransportRowIsPrevBackPlayFwdNext(t *testing.T) {
	h := newHarness(playing(0, 200*time.Second), Deps{})
	ic := h.m.icons
	var xs []int
	for _, g := range []string{ic.prev, ic.back, ic.pause, ic.fwd, ic.next} {
		x, _ := h.find(g)
		if x < 0 {
			t.Fatalf("%q not drawn", g)
		}
		xs = append(xs, x)
	}
	for i := 1; i < len(xs); i++ {
		if xs[i] <= xs[i-1] {
			t.Errorf("order should be previous, backward, play/pause, forward, next; columns %v", xs)
		}
	}
	v := h.m.View()
	for _, gone := range []string{"⇄", "↻"} {
		if strings.Contains(v, gone) {
			t.Errorf("the unused shuffle/repeat placeholders (%s) should be gone", gone)
		}
	}
}

func TestHitSpansDoNotOverlap(t *testing.T) {
	for _, set := range []string{"unicode", "basic", "nerd"} {
		h := newHarness(playing(0, 200*time.Second), Deps{Config: cfgWith(func(c *config.Config) { c.Icons = set })})
		_, g := h.m.compose()
		_, hits, _ := h.m.transportRow(accentStyle("#000000"), g.infoW)
		if len(hits) != 5 {
			t.Fatalf("%s: want 5 buttons, got %d", set, len(hits))
		}
		for i := 1; i < len(hits); i++ {
			if hits[i].x0 < hits[i-1].x1 {
				t.Errorf("%s: spans %v and %v overlap, so a click could hit the wrong button", set, hits[i-1], hits[i])
			}
		}
		// every button's own glyph cells are inside its span, whatever their width
		ic := h.m.icons
		for i, glyph := range []string{ic.prev, ic.back, ic.pause, ic.fwd, ic.next} {
			x, _ := h.find(glyph)
			w := lipgloss.Width(glyph)
			if x-g.infoX < hits[i].x0 || x-g.infoX+w > hits[i].x1 {
				t.Errorf("%s: %q at %d..%d is outside its span %v", set, glyph, x-g.infoX, x-g.infoX+w, hits[i])
			}
		}
	}
}

func TestClickBackAndForwardSeekByFiveSeconds(t *testing.T) {
	for _, set := range []string{"unicode", "basic", "nerd"} {
		rec := &recorder{}
		h := newHarness(playing(60*time.Second, 200*time.Second), Deps{Control: rec.control, Config: cfgWith(func(c *config.Config) { c.Icons = set })})
		bx, by := h.find(h.m.icons.back)
		fx, fy := h.find(h.m.icons.fwd)
		run(h.click(bx, by))
		*h.now = t0.Add(2 * time.Second) // well past the chaining window
		h.m.seekAt = time.Time{}
		run(h.click(fx, fy))
		got := seekTargets(rec)
		// 60 s at t0; two seconds later the real position is 62 s
		if len(got) != 2 || got[0] != 55 || got[1] != 67 {
			t.Errorf("%s: back then forward should seek to 55 and 67, got %v", set, got)
		}
	}
}

func TestClickingBackAndForwardIsTheSameAsTheArrowKeys(t *testing.T) {
	keys, clicks := &recorder{}, &recorder{}
	hk := newHarness(playing(60*time.Second, 200*time.Second), Deps{Control: keys.control})
	hc := newHarness(playing(60*time.Second, 200*time.Second), Deps{Control: clicks.control})
	bx, by := hc.find(hc.m.icons.back)
	fx, fy := hc.find(hc.m.icons.fwd)
	run(hk.press("left"))
	run(hk.press("right"))
	run(hc.click(bx, by))
	run(hc.click(fx, fy))
	k, c := seekTargets(keys), seekTargets(clicks)
	if len(k) != 2 || len(c) != 2 || k[0] != c[0] || k[1] != c[1] {
		t.Errorf("keys sent %v, clicks sent %v", k, c)
	}
}

func TestRapidBackClicksChain(t *testing.T) {
	rec := &recorder{}
	h := newHarness(playing(100*time.Second, 200*time.Second), Deps{Control: rec.control})
	bx, by := h.find(h.m.icons.back)
	run(h.click(bx, by))
	*h.now = t0.Add(100 * time.Millisecond)
	run(h.click(bx, by))
	run(h.click(bx, by))
	if got := seekTargets(rec); len(got) != 3 || got[0] != 95 || got[1] != 90 || got[2] != 85 {
		t.Errorf("three quick back clicks should walk back 5 s each time, got %v", got)
	}
}

func TestBackAndForwardClampAtTheEnds(t *testing.T) {
	rec := &recorder{}
	h := newHarness(playing(2*time.Second, 200*time.Second), Deps{Control: rec.control})
	bx, by := h.find(h.m.icons.back)
	run(h.click(bx, by))
	e := newHarness(playing(198*time.Second, 200*time.Second), Deps{Control: rec.control})
	fx, fy := e.find(e.m.icons.fwd)
	run(e.click(fx, fy))
	if got := seekTargets(rec); len(got) != 2 || got[0] != 0 || got[1] != 200 {
		t.Errorf("seeks should clamp to 0 and the duration, got %v", got)
	}
}

func TestBackAndForwardDoNothingWithoutADuration(t *testing.T) {
	rec := &recorder{}
	live := newHarness(playing(30*time.Second, 0), Deps{Control: rec.control})
	bx, by := live.find(live.m.icons.back)
	fx, fy := live.find(live.m.icons.fwd)
	if live.click(bx, by) != nil || live.click(fx, fy) != nil {
		t.Errorf("a live stream cannot seek")
	}
	empty := newHarness(source.State{}, Deps{Control: rec.control})
	bx, by = empty.find(empty.m.icons.back)
	if empty.click(bx, by) != nil {
		t.Errorf("nothing playing, nothing to seek")
	}
	if len(rec.cmds) != 0 {
		t.Errorf("no commands expected: %+v", rec.cmds)
	}
}

func TestGapsBetweenButtonsAreNotClickable(t *testing.T) {
	rec := &recorder{}
	h := newHarness(playing(60*time.Second, 200*time.Second), Deps{Control: rec.control})
	ic := h.m.icons
	_, y := h.find(ic.prev)
	for i, pair := range [][2]string{{ic.prev, ic.back}, {ic.back, ic.pause}, {ic.pause, ic.fwd}, {ic.fwd, ic.next}} {
		x0, _ := h.find(pair[0])
		x1, _ := h.find(pair[1])
		mid := (x0 + lipgloss.Width(pair[0]) + x1) / 2 // the middle cell of the gap
		if cmd := h.click(mid, y); cmd != nil {
			t.Errorf("gap %d at column %d should not be a button", i, mid)
		}
	}
	if len(rec.cmds) != 0 {
		t.Errorf("clicks in the gaps sent %+v", rec.cmds)
	}
}

// The pane border drifts whenever a glyph is drawn at a different width than we
// measured. Back and forward are ASCII, and nothing in the default set may rely
// on variation selectors or on emoji-capable fast-forward/rewind symbols.
func TestDefaultIconsCannotDriftTheBorder(t *testing.T) {
	ic := unicodeIcons
	if ic.back != "<" || ic.fwd != ">" {
		t.Errorf("back and forward should be plain < and >, got %q and %q", ic.back, ic.fwd)
	}
	for name, g := range map[string]string{"prev": ic.prev, "back": ic.back, "play": ic.play, "pause": ic.pause, "fwd": ic.fwd, "next": ic.next, "volume": ic.volume} {
		if got, runes := lipgloss.Width(g), len([]rune(g)); got != runes {
			t.Errorf("%s %q measures %d cells for %d rune(s): a wide glyph", name, g, got, runes)
		}
		for _, r := range g {
			if r == '\uFE0E' || r == '\uFE0F' {
				t.Errorf("%s %q uses a variation selector, which terminals handle inconsistently", name, g)
			}
			if r == '\u23E9' || r == '\u23EA' || r == '\u23EF' {
				t.Errorf("%s %q is an emoji-presentation symbol (drawn wide in many terminals)", name, g)
			}
		}
	}
}
