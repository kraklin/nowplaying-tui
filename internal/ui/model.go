package ui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tomaslatall/nowplay/internal/art"
	"github.com/tomaslatall/nowplay/internal/history"
	"github.com/tomaslatall/nowplay/internal/source"
)

var sprintf = fmt.Sprintf

// StateMsg carries a new source state.
type StateMsg source.State

// ArtMsg carries a finished cover render.
type ArtMsg struct {
	Hash string
	Px   int
	art.Result
}

type artKey struct {
	hash string
	px   int
}

const (
	placeholderHash = "placeholder"
	maxCached       = 64
)

type Model struct {
	src     <-chan source.State
	state   source.State
	recent  []history.Entry
	volume  int
	now     time.Time
	w, h    int
	cache   map[artKey]art.Result
	pending map[artKey]bool
	icons   icons
	opts    art.Options
}

func New(states <-chan source.State, recent []history.Entry) Model {
	return Model{
		src:     states,
		recent:  recent,
		volume:  55,
		now:     time.Now(),
		cache:   map[artKey]art.Result{},
		pending: map[artKey]bool{},
		icons:   unicodeIcons,
		opts:    art.DefaultOptions(lipgloss.HasDarkBackground()),
	}
}

func (m Model) Init() tea.Cmd { return waitState(m.src) }

func waitState(ch <-chan source.State) tea.Cmd {
	return func() tea.Msg { return StateMsg(<-ch) }
}

func renderArt(key artKey, data []byte, opts art.Options) tea.Cmd {
	return func() tea.Msg {
		var res art.Result
		if key.hash == placeholderHash {
			res = art.Placeholder(key.px)
		} else if r, err := art.Build(data, key.px, opts); err == nil {
			res = r
		} else {
			res = art.Placeholder(key.px)
		}
		return ArtMsg{Hash: key.hash, Px: key.px, Result: res}
	}
}

// wantArt returns the cache key for the cover the current state and layout need.
func (m Model) wantArt() (artKey, bool) {
	l := m.layout()
	if l.coverPx == 0 {
		return artKey{}, false
	}
	hash := placeholderHash
	if !m.state.Empty() && len(m.state.Art) > 0 {
		hash = art.Hash(m.state.Art)
	}
	return artKey{hash, l.coverPx}, true
}

func (m *Model) ensureArt() tea.Cmd {
	key, ok := m.wantArt()
	if !ok {
		return nil
	}
	if _, cached := m.cache[key]; cached || m.pending[key] {
		return nil
	}
	m.pending[key] = true
	return renderArt(key, m.state.Art, m.opts)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, m.ensureArt()
	case StateMsg:
		m.state = source.State(msg)
		m.now = time.Now()
		return m, tea.Batch(waitState(m.src), m.ensureArt())
	case ArtMsg:
		key := artKey{msg.Hash, msg.Px}
		delete(m.pending, key)
		if len(m.cache) >= maxCached {
			m.cache = map[artKey]art.Result{} // drag-resizing makes many sizes; start over
		}
		m.cache[key] = msg.Result
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}
