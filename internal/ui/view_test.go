package ui

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tomaslatall/nowplay/internal/history"
	"github.com/tomaslatall/nowplay/internal/source"
)

// settle feeds the fake's first state and size, then runs art commands to completion.
func settle(t *testing.T, w, h int, empty bool) Model {
	t.Helper()
	ch := make(chan source.State, 1)
	f := source.NewFake()
	st := f.Snapshot()
	if empty {
		st = source.State{}
	}
	ch <- st
	var m tea.Model = New(ch, history.Demo())
	var cmds []tea.Cmd
	for _, msg := range []tea.Msg{tea.WindowSizeMsg{Width: w, Height: h}, StateMsg(<-ch)} {
		var c tea.Cmd
		m, c = m.Update(msg)
		cmds = append(cmds, c)
	}
	for len(cmds) > 0 {
		c := cmds[0]
		cmds = cmds[1:]
		if c == nil {
			continue
		}
		done := make(chan tea.Msg, 1)
		go func() { done <- c() }()
		select {
		case msg := <-done:
			switch msg := msg.(type) {
			case tea.BatchMsg:
				cmds = append(cmds, msg...)
			case ArtMsg:
				m, _ = m.Update(msg)
			}
		case <-time.After(200 * time.Millisecond): // waitState blocks by design
		}
	}
	return m.(Model)
}

func TestViewFitsPane(t *testing.T) {
	cases := []struct {
		w, h  int
		cover bool
	}{{80, 20, true}, {120, 30, true}, {60, 30, true}, {50, 24, true}, {50, 14, false}, {80, 10, false}, {38, 20, false}}
	for _, c := range cases {
		m := settle(t, c.w, c.h, false)
		out := m.View()
		lines := strings.Split(out, "\n")
		if len(lines) != c.h {
			t.Errorf("%dx%d: got %d lines", c.w, c.h, len(lines))
		}
		for i, ln := range lines {
			if lipgloss.Width(ln) > c.w {
				t.Errorf("%dx%d: line %d is %d wide", c.w, c.h, i, lipgloss.Width(ln))
			}
		}
		if got := strings.Contains(out, "▀"); got != c.cover {
			t.Errorf("%dx%d: cover present = %v, want %v", c.w, c.h, got, c.cover)
		}
		if os.Getenv("NOWPLAY_DUMP") != "" {
			t.Logf("%dx%d\n%s", c.w, c.h, out)
		}
	}
}

func TestEmptyState(t *testing.T) {
	out := settle(t, 80, 20, true).View()
	if !strings.Contains(out, "nothing playing") || !strings.Contains(out, "♫") {
		t.Errorf("empty state missing placeholder text or glyph:\n%s", out)
	}
	if !strings.Contains(out, "recently played") {
		t.Errorf("history should stay visible when nothing is playing")
	}
}

func TestFadeDecaysExponentially(t *testing.T) {
	if fade(0) != 1 {
		t.Errorf("newest row must be fully visible, got %v", fade(0))
	}
	for i := 1; i < 12; i++ {
		if fade(i) > fade(i-1) || fade(i) < 0.2 {
			t.Errorf("fade(%d)=%v not monotonic or below floor", i, fade(i))
		}
	}
	// exponential, not linear: early steps drop more than late ones
	if fade(0)-fade(1) <= fade(3)-fade(4) {
		t.Errorf("decay should flatten out")
	}
}

func TestFadedStyleBlend(t *testing.T) {
	bg, fg := [3]uint8{18, 18, 18}, [3]uint8{218, 218, 218}
	if got := fadedStyle(fg, bg, 1).GetForeground(); got != lipgloss.Color("#dadada") {
		t.Errorf("f=1 should keep base, got %v", got)
	}
	if got := fadedStyle(fg, bg, 0).GetForeground(); got != lipgloss.Color("#121212") {
		t.Errorf("f=0 should equal background, got %v", got)
	}
	if got := fadedStyle(fg, bg, 0.5).GetForeground(); got != lipgloss.Color("#767676") {
		t.Errorf("f=0.5 midpoint wrong, got %v", got)
	}
}
