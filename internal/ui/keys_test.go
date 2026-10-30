package ui

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tomaslatall/nowplay/internal/art"
	"github.com/tomaslatall/nowplay/internal/source"
)

type recorder struct {
	mu   sync.Mutex
	cmds []source.Cmd
	err  error
}

func (r *recorder) control(c source.Cmd) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cmds = append(r.cmds, c)
	return r.err
}

type fakeVol struct {
	mu   sync.Mutex
	sets []int
}

func (f *fakeVol) Get() (int, error) { return 50, nil }
func (f *fakeVol) Set(v int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sets = append(f.sets, v)
	return nil
}

var t0 = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

func playing(elapsed, dur time.Duration) source.State {
	return source.State{
		Title: "Low Orbit", Artist: "Neon Ferns", Album: "Night Signals",
		Duration: dur, Elapsed: elapsed, Timestamp: t0, Rate: 1, Playing: true, App: "com.spotify.client",
	}
}

type harness struct {
	m   Model
	now *time.Time
}

func newHarness(st source.State, deps Deps) *harness {
	now := t0
	m := New(make(chan source.State), nil, art.DarkBG, deps)
	m.clock = func() time.Time { return now }
	h := &harness{m: m, now: &now}
	h.send(tea.WindowSizeMsg{Width: 80, Height: 20})
	h.send(StateMsg(st))
	return h
}

func (h *harness) send(msg tea.Msg) tea.Cmd {
	tm, cmd := h.m.Update(msg)
	h.m = tm.(Model)
	return cmd
}

func (h *harness) press(k string) tea.Cmd {
	var msg tea.KeyMsg
	switch k {
	case "left":
		msg = tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		msg = tea.KeyMsg{Type: tea.KeyRight}
	case " ":
		msg = tea.KeyMsg{Type: tea.KeySpace}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
	}
	return h.send(msg)
}

func run(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

func TestKeysSendTransportCommands(t *testing.T) {
	for key, want := range map[string]source.CmdKind{
		" ": source.TogglePlay, "n": source.Next, "l": source.Next, "p": source.Previous, "h": source.Previous,
	} {
		rec := &recorder{}
		h := newHarness(playing(0, 200*time.Second), Deps{Control: rec.control})
		if msg := run(h.press(key)); msg != nil {
			t.Errorf("%q: unexpected message %#v", key, msg)
		}
		if len(rec.cmds) != 1 || rec.cmds[0].Kind != want {
			t.Errorf("%q sent %+v, want kind %v", key, rec.cmds, want)
		}
	}
}

func TestQuitKeys(t *testing.T) {
	for _, k := range []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune("q")}, {Type: tea.KeyCtrlC}} {
		h := newHarness(playing(0, time.Minute), Deps{})
		cmd := h.send(k)
		if cmd == nil {
			t.Fatalf("%v should quit", k)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("%v: expected QuitMsg", k)
		}
	}
}

func TestKeysWithoutControlAreHarmless(t *testing.T) {
	h := newHarness(playing(0, time.Minute), Deps{})
	for _, k := range []string{" ", "n", "p", "right", "+"} {
		if cmd := h.press(k); cmd != nil {
			t.Errorf("%q with no deps should do nothing", k)
		}
	}
}

func seekTargets(rec *recorder) []float64 {
	var out []float64
	for _, c := range rec.cmds {
		if c.Kind == source.Seek {
			out = append(out, c.Seconds)
		}
	}
	return out
}

func TestSeekStepsAndClamps(t *testing.T) {
	cases := []struct {
		name         string
		elapsed, dur time.Duration
		key          string
		want         float64
	}{
		{"forward", 60 * time.Second, 200 * time.Second, "right", 65},
		{"back", 60 * time.Second, 200 * time.Second, "left", 55},
		{"clamp at start", 2 * time.Second, 200 * time.Second, "left", 0},
		{"clamp at end", 198 * time.Second, 200 * time.Second, "right", 200},
	}
	for _, c := range cases {
		rec := &recorder{}
		h := newHarness(playing(c.elapsed, c.dur), Deps{Control: rec.control})
		run(h.press(c.key))
		if got := seekTargets(rec); len(got) != 1 || got[0] != c.want {
			t.Errorf("%s: seek targets %v, want [%v]", c.name, got, c.want)
		}
	}
}

func TestSeekUsesInterpolatedPosition(t *testing.T) {
	rec := &recorder{}
	h := newHarness(playing(60*time.Second, 200*time.Second), Deps{Control: rec.control})
	*h.now = t0.Add(10 * time.Second) // 10 s of playback since the state arrived
	run(h.press("right"))
	if got := seekTargets(rec); len(got) != 1 || got[0] != 75 {
		t.Errorf("seek from interpolated 70 s should target 75, got %v", got)
	}
}

func TestSeekDoesNothingWithoutDurationOrTrack(t *testing.T) {
	rec := &recorder{}
	live := newHarness(playing(30*time.Second, 0), Deps{Control: rec.control})
	if cmd := live.press("right"); cmd != nil {
		t.Errorf("live streams cannot seek")
	}
	empty := newHarness(source.State{}, Deps{Control: rec.control})
	if cmd := empty.press("left"); cmd != nil {
		t.Errorf("nothing playing cannot seek")
	}
	if len(rec.cmds) != 0 {
		t.Errorf("no commands expected, got %+v", rec.cmds)
	}
}

func TestRapidSeekPressesChain(t *testing.T) {
	rec := &recorder{}
	h := newHarness(playing(60*time.Second, 200*time.Second), Deps{Control: rec.control})
	run(h.press("right"))
	*h.now = t0.Add(100 * time.Millisecond) // stream has not reported the first seek yet
	run(h.press("right"))
	run(h.press("right"))
	*h.now = t0.Add(5 * time.Second) // paused pressing: the real position is 60 + 5 s played
	run(h.press("right"))
	got := seekTargets(rec)
	want := []float64{65, 70, 75, 70}
	if len(got) != 4 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] || got[3] != want[3] {
		t.Errorf("seek targets %v, want %v", got, want)
	}
}

func TestControlErrorShowsNoteThenClears(t *testing.T) {
	rec := &recorder{err: errors.New("boom")}
	h := newHarness(playing(0, time.Minute), Deps{Control: rec.control})
	h.send(run(h.press(" ")))
	if !strings.Contains(h.m.View(), "play/pause failed") {
		t.Errorf("failure should be visible:\n%s", h.m.View())
	}
	h.send(TickMsg(t0.Add(noteFor + time.Second)))
	if strings.Contains(h.m.View(), "failed") {
		t.Errorf("note should clear after a few seconds")
	}
}

func TestVolumeKeysCoalesceWrites(t *testing.T) {
	fv := &fakeVol{}
	h := newHarness(playing(0, time.Minute), Deps{Volume: fv})
	h.send(VolumeMsg{Level: 50})

	first := h.press("+")
	if first == nil || h.m.volume != 55 {
		t.Fatalf("first + should start a write and show 55, got volume %d", h.m.volume)
	}
	if h.press("+") != nil || h.press("=") != nil {
		t.Errorf("presses during a write must not start another one")
	}
	if h.m.volume != 65 {
		t.Fatalf("display should follow the keys immediately, got %d", h.m.volume)
	}
	followUp := h.send(run(first)) // the first write finishes
	if followUp == nil {
		t.Fatal("completion must send the latest value")
	}
	if h.send(run(followUp)) != nil {
		t.Errorf("nothing more to write once caught up")
	}
	if got := fv.sets; len(got) != 2 || got[0] != 55 || got[1] != 65 {
		t.Errorf("writes %v, want [55 65] (in order, no races)", got)
	}
}

func TestVolumeClampsAndNeedsAReading(t *testing.T) {
	fv := &fakeVol{}
	h := newHarness(playing(0, time.Minute), Deps{Volume: fv})
	if h.press("+") != nil {
		t.Errorf("before the first reading the volume is unknown; keys do nothing")
	}
	h.send(VolumeMsg{Level: 98})
	h.send(run(h.press("+")))
	if h.m.volume != 100 {
		t.Errorf("should clamp at 100, got %d", h.m.volume)
	}
	h.send(VolumeMsg{Level: 2})
	*h.now = t0.Add(time.Minute)
	h.send(VolumeMsg{Level: 2})
	h.send(run(h.press("-")))
	if h.m.volume != 0 {
		t.Errorf("should clamp at 0, got %d", h.m.volume)
	}
}

func TestPollDoesNotClobberRecentChange(t *testing.T) {
	fv := &fakeVol{}
	h := newHarness(playing(0, time.Minute), Deps{Volume: fv})
	h.send(VolumeMsg{Level: 50})
	h.send(run(h.press("+"))) // 55, write completes
	*h.now = t0.Add(500 * time.Millisecond)
	h.send(VolumeMsg{Level: 50}) // stale poll that started before the write
	if h.m.volume != 55 {
		t.Errorf("poll inside the hold window must be ignored, got %d", h.m.volume)
	}
	*h.now = t0.Add(volHold + time.Second)
	h.send(VolumeMsg{Level: 61}) // changed elsewhere (menu bar, keyboard)
	if h.m.volume != 61 {
		t.Errorf("later poll should be adopted, got %d", h.m.volume)
	}
	h.send(VolumeMsg{Err: errors.New("no device")})
	if h.m.volume != -1 {
		t.Errorf("an error should mark the volume unavailable")
	}
}

func TestVolumePollReschedules(t *testing.T) {
	h := newHarness(playing(0, time.Minute), Deps{Volume: &fakeVol{}})
	if h.send(VolumeMsg{Level: 40}) == nil {
		t.Errorf("a poll result must schedule the next poll")
	}
	if h.send(volPollMsg{}) == nil {
		t.Errorf("poll tick must start a read")
	}
	if cmd := h.m.Init(); cmd == nil {
		t.Errorf("Init must start the stream, tick and volume poll")
	}
}

func TestTickAdvancesProgressAndReschedules(t *testing.T) {
	h := newHarness(playing(0, 238*time.Second), Deps{})
	if !strings.Contains(h.m.View(), "0:00 / 3:58") {
		t.Fatalf("expected 0:00 at start:\n%s", h.m.View())
	}
	if h.send(TickMsg(t0.Add(10*time.Second))) == nil {
		t.Errorf("tick must reschedule itself")
	}
	if !strings.Contains(h.m.View(), "0:10 / 3:58") {
		t.Errorf("expected 0:10 after 10 s:\n%s", h.m.View())
	}
}

func TestPausedProgressFreezes(t *testing.T) {
	st := playing(30*time.Second, 238*time.Second)
	st.Playing, st.Rate = false, 0
	h := newHarness(st, Deps{})
	h.send(TickMsg(t0.Add(5 * time.Minute)))
	if !strings.Contains(h.m.View(), "0:30 / 3:58") {
		t.Errorf("paused bar must not move:\n%s", h.m.View())
	}
	if !strings.Contains(h.m.View(), "▶") {
		t.Errorf("paused shows the play icon")
	}
}

func TestProgressStopsAtDuration(t *testing.T) {
	h := newHarness(playing(0, 60*time.Second), Deps{})
	h.send(TickMsg(t0.Add(10 * time.Minute)))
	if !strings.Contains(h.m.View(), "1:00 / 1:00") {
		t.Errorf("position must clamp to the duration:\n%s", h.m.View())
	}
}

func TestNoDurationShowsElapsedOnly(t *testing.T) {
	h := newHarness(playing(0, 0), Deps{})
	h.send(TickMsg(t0.Add(7 * time.Second)))
	v := h.m.View()
	if !strings.Contains(v, "0:07") || strings.Contains(v, "0:07 /") || strings.Contains(v, "━●") {
		t.Errorf("live streams show elapsed time and no bar:\n%s", v)
	}
}

func TestUnavailableVolumeIsDisabled(t *testing.T) {
	h := newHarness(playing(0, time.Minute), Deps{})
	if h.m.volume != -1 {
		t.Errorf("no volume dependency means unavailable, got %d", h.m.volume)
	}
	if !strings.Contains(h.m.View(), "♪ ──────────") {
		t.Errorf("unavailable volume should draw an empty, dim track:\n%s", h.m.View())
	}
}
