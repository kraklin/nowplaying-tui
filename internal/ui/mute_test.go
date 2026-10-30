package ui

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tomaslatall/nowplaying-tui/internal/source"
)

// muteVol is a volume backend that can mute and records every call, in order.
type muteVol struct {
	mu    sync.Mutex
	level int
	muted bool
	calls []string
}

func (v *muteVol) Get() (int, error) { v.mu.Lock(); defer v.mu.Unlock(); return v.level, nil }
func (v *muteVol) Read() (int, bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.level, v.muted, nil
}
func (v *muteVol) Set(l int) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.level = l
	v.calls = append(v.calls, fmt.Sprintf("set:%d", l))
	return nil
}
func (v *muteVol) SetMuted(m bool) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.muted = m
	v.calls = append(v.calls, fmt.Sprintf("mute:%t", m))
	return nil
}
func (v *muteVol) log() string { v.mu.Lock(); defer v.mu.Unlock(); return strings.Join(v.calls, " ") }

func muteHarness(t *testing.T, w, hgt int) (*harness, *muteVol) {
	t.Helper()
	mv := &muteVol{level: 50}
	h := newHarness(playing(0, 200*time.Second), Deps{Volume: mv}).withDemoHistory()
	h.resize(w, hgt)
	h.send(VolumeMsg{Level: 50})
	return h, mv
}

// volumeRow is the plain text of the transport row from the volume icon onwards.
func volumeRow(h *harness) string {
	for _, r := range h.rows() {
		if i := strings.Index(r, h.m.icons.volume); i >= 0 {
			return r[i:]
		}
	}
	return ""
}

func TestMuteKeyMutesWithoutTouchingTheLevel(t *testing.T) {
	h, mv := muteHarness(t, 100, 30)
	cmd := h.press("m")
	if cmd == nil || !h.m.muted {
		t.Fatalf("m should mute at once and start a write (muted=%v)", h.m.muted)
	}
	h.send(run(cmd))
	if got := mv.log(); got != "mute:true" {
		t.Errorf("muting must not change the level; calls: %q", got)
	}
	if h.m.volume != 50 {
		t.Errorf("the level is remembered while muted: %d", h.m.volume)
	}
	cmd = h.press("m")
	if cmd == nil || h.m.muted {
		t.Fatalf("a second m should unmute")
	}
	h.send(run(cmd))
	if got := mv.log(); got != "mute:true set:50 mute:false" {
		t.Errorf("unmuting restores the level then the sound: %q", got)
	}
}

func TestMutedBarShowsTheWordMute(t *testing.T) {
	h, _ := muteHarness(t, 100, 30)
	unmuted := volumeRow(h)
	if strings.Contains(unmuted, "mute") || !strings.Contains(unmuted, "━") {
		t.Fatalf("an unmuted volume shows the bar: %q", unmuted)
	}
	ux, _ := h.find(h.m.icons.volume)
	h.send(run(h.press("m")))
	muted := volumeRow(h)
	if !strings.HasPrefix(muted, h.m.icons.volume+" mute") {
		t.Errorf("the bar should be replaced by the word mute: %q", muted)
	}
	if strings.ContainsAny(muted, "━─") {
		t.Errorf("no bar characters while muted: %q", muted)
	}
	if lipgloss.Width(muted) != lipgloss.Width(unmuted) {
		t.Errorf("the volume block must keep its width (%d vs %d) so the row does not shift", lipgloss.Width(muted), lipgloss.Width(unmuted))
	}
	if mx, _ := h.find(h.m.icons.volume); mx != ux {
		t.Errorf("the volume icon moved from %d to %d", ux, mx)
	}
}

func TestMutedDisplayDoesNotMoveAnythingElse(t *testing.T) {
	for _, sz := range [][2]int{{100, 30}, {80, 20}, {60, 40}, {50, 30}, {120, 70}} {
		h, _ := muteHarness(t, sz[0], sz[1])
		before := h.rows()
		_, g := h.m.compose()
		_, hitsBefore, volBefore := h.m.transportRow(accentStyle("#000"), g.infoW)
		h.send(run(h.press("m")))
		after := h.rows()
		_, hitsAfter, volAfter := h.m.transportRow(accentStyle("#000"), g.infoW)
		if volBefore != volAfter || fmt.Sprint(hitsBefore) != fmt.Sprint(hitsAfter) {
			t.Errorf("%v: muting moved click regions: %v %v -> %v %v", sz, hitsBefore, volBefore, hitsAfter, volAfter)
		}
		for i := range before {
			if i != g.infoY+rowVolume && before[i] != after[i] {
				t.Errorf("%v: row %d changed when muting", sz, i)
			}
		}
		if row := after[g.infoY+rowVolume]; lipgloss.Width(row) > sz[0] {
			t.Errorf("%v: the volume row is %d wide", sz, lipgloss.Width(row))
		}
	}
}

func TestVolumeKeysUnmute(t *testing.T) {
	for _, key := range []string{"+", "-"} {
		h, mv := muteHarness(t, 100, 30)
		h.send(run(h.press("m")))
		cmd := h.press(key)
		if cmd == nil || h.m.muted {
			t.Fatalf("%s while muted should unmute", key)
		}
		h.send(run(cmd))
		want := map[string]string{"+": "mute:true set:55 mute:false", "-": "mute:true set:45 mute:false"}[key]
		if got := mv.log(); got != want {
			t.Errorf("%s: calls %q, want %q", key, got, want)
		}
		if strings.Contains(volumeRow(h), "mute") {
			t.Errorf("%s: the bar should be back", key)
		}
	}
}

func TestScrollingOverTheBarUnmutes(t *testing.T) {
	h, mv := muteHarness(t, 100, 30)
	h.send(run(h.press("m")))
	x, y := h.find(h.m.icons.volume)
	cmd := h.wheel(x+3, y, tea.MouseButtonWheelUp)
	if cmd == nil || h.m.muted || h.m.volume != 55 {
		t.Fatalf("scrolling up over the muted bar should unmute and raise it: muted=%v volume=%d", h.m.muted, h.m.volume)
	}
	h.send(run(cmd))
	if got := mv.log(); got != "mute:true set:55 mute:false" {
		t.Errorf("calls %q", got)
	}
}

func TestMuteNeedsABackendThatCanMute(t *testing.T) {
	plainVol := newHarness(playing(0, time.Minute), Deps{Volume: &fakeVol{}})
	plainVol.send(VolumeMsg{Level: 50})
	if cmd := plainVol.press("m"); cmd != nil || plainVol.m.muted {
		t.Errorf("a backend without mute: m does nothing")
	}
	none := newHarness(playing(0, time.Minute), Deps{})
	if cmd := none.press("m"); cmd != nil || none.m.muted {
		t.Errorf("no volume at all: m does nothing")
	}
	unknown := newHarness(playing(0, time.Minute), Deps{Volume: &muteVol{}})
	if cmd := unknown.press("m"); cmd != nil || unknown.m.muted {
		t.Errorf("before the first reading the state is unknown: m does nothing")
	}
	broken := newHarness(playing(0, time.Minute), Deps{Volume: &muteVol{}})
	broken.send(VolumeMsg{Err: errors.New("no device")})
	if cmd := broken.press("m"); cmd != nil {
		t.Errorf("an unavailable volume cannot be muted")
	}
}

func TestPollShowsMuteChangedElsewhere(t *testing.T) {
	h, _ := muteHarness(t, 100, 30)
	h.send(VolumeMsg{Level: 50, Muted: true}) // muted from the menu bar
	if !strings.Contains(volumeRow(h), "mute") {
		t.Errorf("a mute done elsewhere should show:\n%s", volumeRow(h))
	}
	*h.now = t0.Add(10 * time.Second)
	h.send(VolumeMsg{Level: 50, Muted: false})
	if strings.Contains(volumeRow(h), "mute") {
		t.Errorf("and clear when it is undone elsewhere")
	}
	h.send(VolumeMsg{Level: 50, Muted: true})
	h.send(VolumeMsg{Err: errors.New("device gone")})
	if h.m.muted || h.m.volume != -1 {
		t.Errorf("an unavailable volume clears the mute state: muted=%v volume=%d", h.m.muted, h.m.volume)
	}
}

func TestPollDoesNotUndoARecentMute(t *testing.T) {
	h, _ := muteHarness(t, 100, 30)
	h.send(run(h.press("m")))
	*h.now = t0.Add(300 * time.Millisecond)
	h.send(VolumeMsg{Level: 50, Muted: false}) // a poll that started before the write
	if !h.m.muted {
		t.Errorf("a stale poll inside the hold window must not flip the display back")
	}
	*h.now = t0.Add(volHold + time.Second)
	h.send(VolumeMsg{Level: 50, Muted: false})
	if h.m.muted {
		t.Errorf("later polls are believed again")
	}
}

func TestMuteWritesAreSerialisedAndCoalesced(t *testing.T) {
	h, mv := muteHarness(t, 100, 30)
	first := h.press("m") // mute: starts the only write
	if first == nil {
		t.Fatal("first press must start a write")
	}
	if h.press("m") != nil || h.press("m") != nil {
		t.Errorf("presses during a write must not start another one")
	}
	if !h.m.muted {
		t.Errorf("three presses end muted")
	}
	if follow := h.send(run(first)); follow != nil {
		t.Errorf("already in the wanted state: no second write needed")
	}
	if got := mv.log(); got != "mute:true" {
		t.Errorf("three quick presses should cost one write, got %q", got)
	}

	h2, mv2 := muteHarness(t, 100, 30)
	w1 := h2.press("m")
	h2.press("m") // back to unmuted while the first write runs
	follow := h2.send(run(w1))
	if follow == nil {
		t.Fatal("the wanted state changed during the write, so a follow-up is needed")
	}
	h2.send(run(follow))
	if got := mv2.log(); got != "mute:true set:50 mute:false" {
		t.Errorf("writes must happen in order, got %q", got)
	}
}

func TestMuteThenVolumeKeyEndsUnmutedAtTheNewLevel(t *testing.T) {
	h, mv := muteHarness(t, 100, 30)
	w1 := h.press("m")
	if h.press("+") != nil {
		t.Errorf("the + press must wait for the running write")
	}
	if h.m.muted || h.m.volume != 55 {
		t.Fatalf("wanted state: unmuted at 55, got muted=%v volume=%d", h.m.muted, h.m.volume)
	}
	h.send(run(h.send(run(w1))))
	if got := mv.log(); got != "mute:true set:55 mute:false" {
		t.Errorf("calls %q", got)
	}
}

func TestMuteKeyIsOnlyAViewAndVolumeChange(t *testing.T) {
	rec := &recorder{}
	mv := &muteVol{level: 50}
	h := newHarness(playing(0, 200*time.Second), Deps{Volume: mv, Control: rec.control})
	h.send(VolumeMsg{Level: 50})
	h.send(run(h.press("m")))
	if len(rec.cmds) != 0 {
		t.Errorf("muting must not send transport commands: %+v", rec.cmds)
	}
}

func TestMuteSurvivesLayoutToggles(t *testing.T) {
	h, _ := muteHarness(t, 100, 30)
	h.send(run(h.press("m")))
	for _, k := range []string{"a", "r", "?", "a", "r", "?"} {
		h.press(k)
	}
	if !h.m.muted || !strings.Contains(volumeRow(h), "mute") {
		t.Errorf("toggling the cover, history or help must not change the mute state")
	}
}

func TestHelpMentionsMute(t *testing.T) {
	h, _ := muteHarness(t, 140, 30)
	h.press("?")
	rows := h.rows()
	if got := rows[len(rows)-1]; !strings.Contains(got, "m mute") || !strings.Contains(got, "q quit") {
		t.Errorf("help should list the mute key: %q", got)
	}
}

func TestMuteWordFitsInTheVolumeBlockAtEverySize(t *testing.T) {
	for w := 44; w <= 130; w += 3 {
		for _, hgt := range []int{20, 40, 60} {
			h, _ := muteHarness(t, w, hgt)
			h.send(run(h.press("m")))
			_, g := h.m.compose()
			row := h.rows()[g.infoY+rowVolume]
			x := cellX(row, "mute")
			_, _, vol := h.m.transportRow(accentStyle("#000"), g.infoW)
			if vol[0] < 0 {
				continue // the volume block is not shown in a very narrow card
			}
			if x < g.infoX+vol[0] || x+4 > g.infoX+vol[1] {
				t.Errorf("%dx%d: the word sits at %d..%d, outside the volume block %d..%d", w, hgt, x, x+4, g.infoX+vol[0], g.infoX+vol[1])
			}
		}
	}
}

var _ = source.Cmd{}
