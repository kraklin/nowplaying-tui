package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tomaslatall/nowplaying-tui/internal/config"
	"github.com/tomaslatall/nowplaying-tui/internal/history"
	"github.com/tomaslatall/nowplaying-tui/internal/source"
)

func track(title string) source.State {
	s := playing(0, 200*time.Second)
	s.Title = title
	return s
}

func (h *harness) advance(d time.Duration) { *h.now = h.now.Add(d) }

// next delivers a new state at the harness clock, as the stream would.
func (h *harness) next(st source.State) tea.Cmd {
	st.Timestamp = *h.now
	return h.send(StateMsg(st))
}

// settle runs commands that finish on their own (saves), ignoring blocking ones.
func (h *harness) settle(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		switch msg := msg.(type) {
		case tea.BatchMsg:
			for _, c := range msg {
				h.settle(c)
			}
		case HistorySavedMsg:
			h.send(msg)
		}
	case <-time.After(100 * time.Millisecond): // waitState blocks by design
	}
}

func newStoreHarness(t *testing.T) (*harness, *history.Store) {
	t.Helper()
	store := history.NewStore(filepath.Join(t.TempDir(), "nowplaying", "history.json"), 50)
	h := newHarness(track("First"), Deps{History: store})
	return h, store
}

func TestSkippedTrackEntersHistoryAndIsSaved(t *testing.T) {
	h, store := newStoreHarness(t)
	h.advance(45 * time.Second)
	h.settle(h.next(track("Second")))

	if len(h.m.recent) != 1 || h.m.recent[0].Title != "First" {
		t.Fatalf("recent = %+v", h.m.recent)
	}
	saved, err := store.Load()
	if err != nil || len(saved) != 1 || saved[0].Title != "First" {
		t.Errorf("history file should hold the track: %v %v", saved, err)
	}
	if !strings.Contains(h.m.View(), "First") {
		t.Errorf("the list on screen should show it:\n%s", h.m.View())
	}
}

func TestQuickSkipsAreNotRecorded(t *testing.T) {
	h, store := newStoreHarness(t)
	for _, n := range []string{"B", "C", "D"} {
		h.advance(3 * time.Second)
		if cmd := h.next(track(n)); cmd == nil {
			t.Fatal("state updates always re-arm the stream wait")
		}
	}
	if len(h.m.recent) != 0 {
		t.Errorf("tracks skipped after 3 s must not enter the list: %+v", h.m.recent)
	}
	if _, err := os.Stat(store.Path); err == nil {
		t.Errorf("nothing was recorded, so nothing should be written")
	}
}

func TestSurvivesRestart(t *testing.T) {
	h, store := newStoreHarness(t)
	h.advance(30 * time.Second)
	h.settle(h.next(track("Second")))
	h.advance(30 * time.Second)
	h.settle(h.next(track("Third")))

	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	h2 := newHarness(track("Third"), Deps{History: store})
	h2.m.recent = loaded
	if got := h2.m.View(); !strings.Contains(got, "First") || !strings.Contains(got, "Second") {
		t.Errorf("a restarted app should show the saved list:\n%s", got)
	}
	if loaded[0].Title != "Second" || loaded[1].Title != "First" {
		t.Errorf("newest first: %+v", loaded)
	}
}

func TestSameTrackTwiceInARowIsNotDuplicated(t *testing.T) {
	h, _ := newStoreHarness(t)
	h.advance(30 * time.Second)
	h.settle(h.next(track("Second")))
	h.advance(30 * time.Second)
	h.settle(h.next(track("First"))) // back to the first one; Second goes in
	h.advance(30 * time.Second)
	h.settle(h.next(track("First"))) // same track again (e.g. a reconnect): no change
	if len(h.m.recent) != 2 || h.m.recent[0].Title != "Second" || h.m.recent[1].Title != "First" {
		t.Errorf("recent = %+v", h.m.recent)
	}
}

func TestClearAsksForConfirmation(t *testing.T) {
	h, store := newStoreHarness(t)
	h.advance(30 * time.Second)
	h.settle(h.next(track("Second")))
	if len(h.m.recent) != 1 {
		t.Fatal("setup")
	}

	if cmd := h.press("c"); cmd != nil || len(h.m.recent) != 1 {
		t.Fatalf("the first c must only ask")
	}
	if !strings.Contains(h.m.View(), "clear history? press c again") {
		t.Errorf("prompt missing:\n%s", h.m.View())
	}
	h.settle(h.press("c"))
	if len(h.m.recent) != 0 {
		t.Errorf("second c should clear the list")
	}
	if saved, _ := store.Load(); len(saved) != 0 {
		t.Errorf("the clear must be persisted, file has %v", saved)
	}
	if strings.Contains(h.m.View(), "clear history?") {
		t.Errorf("prompt should be gone after clearing")
	}
}

func TestClearIsCancelledByAnyOtherKey(t *testing.T) {
	h, _ := newStoreHarness(t)
	h.advance(30 * time.Second)
	h.settle(h.next(track("Second")))
	h.press("c")
	h.press("x")
	if strings.Contains(h.m.View(), "clear history?") {
		t.Errorf("another key should dismiss the prompt")
	}
	h.press("c") // this starts a new confirmation instead of clearing
	if len(h.m.recent) != 1 {
		t.Errorf("c after a cancel must ask again, not clear")
	}
}

func TestClearConfirmationExpires(t *testing.T) {
	h, _ := newStoreHarness(t)
	h.advance(30 * time.Second)
	h.settle(h.next(track("Second")))
	h.press("c")
	h.advance(confirmFor + time.Second)
	h.press("c")
	if len(h.m.recent) != 1 {
		t.Errorf("a late second c must not clear")
	}
	h.send(TickMsg(*h.now))
	h.advance(time.Second)
}

func TestClearWithEmptyHistoryDoesNothing(t *testing.T) {
	h, _ := newStoreHarness(t)
	if cmd := h.press("c"); cmd != nil || strings.Contains(h.m.View(), "clear history?") {
		t.Errorf("nothing to clear, nothing to ask")
	}
}

func TestSaveFailureIsShown(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores permissions")
	}
	dir := t.TempDir()
	os.Chmod(dir, 0o500)
	defer os.Chmod(dir, 0o700)
	store := history.NewStore(filepath.Join(dir, "sub", "history.json"), 50)
	h := newHarness(track("First"), Deps{History: store})
	h.advance(30 * time.Second)
	h.settle(h.next(track("Second")))
	if !strings.Contains(h.m.View(), "history not saved") {
		t.Errorf("a failed write should be visible:\n%s", h.m.View())
	}
	if len(h.m.recent) != 1 {
		t.Errorf("the list still works in memory")
	}
}

func TestHistoryWorksWithoutAStore(t *testing.T) {
	h := newHarness(track("First"), Deps{}) // --demo has no store
	h.advance(30 * time.Second)
	h.next(track("Second"))
	if len(h.m.recent) != 1 {
		t.Errorf("in-memory history should still update")
	}
	if cmd := h.press("c"); cmd != nil {
		t.Errorf("no store, no save command")
	}
}

func TestHistorySizeIsCapped(t *testing.T) {
	cfg := config.Default()
	cfg.HistorySize = 3
	h := newHarness(track("T0"), Deps{Config: &cfg})
	for i := 1; i <= 6; i++ {
		h.advance(30 * time.Second)
		h.next(track(string(rune('A' + i))))
	}
	if len(h.m.recent) != 3 {
		t.Errorf("history_size 3, got %d entries", len(h.m.recent))
	}
}

func TestSavesCarryIncreasingSequence(t *testing.T) {
	h, _ := newStoreHarness(t)
	h.advance(30 * time.Second)
	h.next(track("Second"))
	first := h.m.histSeq
	h.advance(30 * time.Second)
	h.next(track("Third"))
	if h.m.histSeq <= first {
		t.Errorf("each save needs a newer sequence number: %d then %d", first, h.m.histSeq)
	}
}

// A note replaces the label on its row. It must never wrap, or the whole card
// shifts down a line.
func TestNotesNeverWrapTheLabelRow(t *testing.T) {
	for _, w := range []int{50, 60, 70, 80, 100} {
		for _, note := range []string{"history not saved", "play/pause failed", confirmNote, "Zulu " + strings.Repeat("x", 80)} {
			h := newHarness(track("First"), Deps{})
			h.send(tea.WindowSizeMsg{Width: w, Height: 24})
			before := strings.Count(h.m.View(), "\n")
			h.m.setNote(note)
			after := h.m.View()
			if strings.Count(after, "\n") != before {
				t.Errorf("w=%d note %q changed the card height", w, note)
			}
			start := note[:min(6, len(note))]
			if n := strings.Count(after, start); n != 1 {
				t.Errorf("w=%d note %q: its text appears %d times, want once on a single row:\n%s", w, note, n, after)
			}
		}
	}
}
