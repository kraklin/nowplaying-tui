package ui

import (
	"fmt"
	"image/color"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tomaslatall/nowplay/internal/art"
	"github.com/tomaslatall/nowplay/internal/history"
	"github.com/tomaslatall/nowplay/internal/source"
)

var sprintf = fmt.Sprintf

// StateMsg carries a new source state.
type StateMsg source.State

// TickMsg drives the progress bar.
type TickMsg time.Time

// VolumeMsg is the result of polling the system volume.
type VolumeMsg struct {
	Level int
	Err   error
}

type volPollMsg struct{}

type volSetMsg struct {
	sent int
	err  error
}

// HistorySavedMsg is the result of writing the history file.
type HistorySavedMsg struct{ Err error }

// ControlErrMsg reports a failed transport command.
type ControlErrMsg struct {
	Op  string
	Err error
}

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

	tickEvery    = 250 * time.Millisecond
	volPollEvery = 2 * time.Second
	seekStep     = 5 * time.Second
	volStep      = 5
	// A second seek press within seekChain builds on the previous target, because
	// the stream has not reported the first one yet.
	seekChain = 800 * time.Millisecond
	// After a volume key, polled values are ignored for volHold so the bar
	// does not jump back.
	volHold     = 1500 * time.Millisecond
	noteFor     = 3 * time.Second
	confirmNote = "clear history? press c again"
	confirmFor  = 5 * time.Second
)

// Volume is the system volume. Calls are slow and must run inside a tea.Cmd.
type Volume interface {
	Get() (int, error)
	Set(int) error
}

// Deps are the side-effecting collaborators; all are optional.
type Deps struct {
	Control func(source.Cmd) error
	Volume  Volume
	History *history.Store // nil keeps the history in memory only
	// MinPlay and HistorySize default to history.DefaultMinPlay and DefaultSize.
	MinPlay     time.Duration
	HistorySize int
}

type Model struct {
	src     <-chan source.State
	deps    Deps
	clock   func() time.Time
	state   source.State
	recent  []history.Entry
	now     time.Time
	w, h    int
	cache   map[artKey]art.Result
	pending map[artKey]bool
	icons   icons
	opts    art.Options
	th      theme

	volume    int // 0-100, or -1 when unavailable
	volBusy   bool
	volHold   time.Time
	seekAt    time.Time
	seekTo    time.Duration
	note      string
	noteUntil time.Time

	tracker      *history.Tracker
	histMax      int
	histSeq      uint64
	confirmUntil time.Time
}

// New builds the model; bg is the terminal background colour.
func New(states <-chan source.State, recent []history.Entry, bg color.RGBA, deps Deps) Model {
	histMax := deps.HistorySize
	if histMax <= 0 {
		histMax = history.DefaultSize
	}
	return Model{
		tracker: history.NewTracker(deps.MinPlay),
		histMax: histMax,
		src:     states,
		deps:    deps,
		clock:   time.Now,
		recent:  recent,
		volume:  -1,
		now:     time.Now(),
		cache:   map[artKey]art.Result{},
		pending: map[artKey]bool{},
		icons:   unicodeIcons,
		opts:    art.DefaultOptions(bg),
		th:      newTheme(bg),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(waitState(m.src), tick(), m.pollVolume())
}

func tick() tea.Cmd {
	return tea.Tick(tickEvery, func(t time.Time) tea.Msg { return TickMsg(t) })
}

func (m Model) pollVolume() tea.Cmd {
	v := m.deps.Volume
	if v == nil {
		return nil
	}
	return func() tea.Msg {
		level, err := v.Get()
		return VolumeMsg{Level: level, Err: err}
	}
}

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
		m.now = m.clock()
		cmds := []tea.Cmd{waitState(m.src), m.ensureArt()}
		if e := m.tracker.Observe(m.state, m.now); e != nil {
			m.recent = history.Push(m.recent, *e, m.histMax)
			cmds = append(cmds, m.saveHistory())
		}
		return m, tea.Batch(cmds...)
	case HistorySavedMsg:
		if msg.Err != nil {
			m.setNote("history not saved")
		}
		return m, nil
	case ArtMsg:
		key := artKey{msg.Hash, msg.Px}
		delete(m.pending, key)
		if len(m.cache) >= maxCached {
			m.cache = map[artKey]art.Result{} // drag-resizing makes many sizes; start over
		}
		m.cache[key] = msg.Result
		return m, nil
	case TickMsg:
		m.now = time.Time(msg)
		if m.note != "" && m.now.After(m.noteUntil) {
			m.note = ""
		}
		return m, tick()
	case VolumeMsg:
		next := tea.Tick(volPollEvery, func(time.Time) tea.Msg { return volPollMsg{} })
		switch {
		case msg.Err != nil:
			m.volume = -1
		case !m.volBusy && !m.clock().Before(m.volHold):
			m.volume = msg.Level
		}
		return m, next
	case volPollMsg:
		return m, m.pollVolume()
	case volSetMsg:
		m.volBusy = false
		if msg.err != nil {
			m.setNote("volume failed")
			return m, m.pollVolume()
		}
		if m.volume != msg.sent { // keys were pressed while the last write ran
			m.volBusy = true
			return m, setVolume(m.deps.Volume, m.volume)
		}
		return m, nil
	case ControlErrMsg:
		m.setNote(msg.Op + " failed")
		return m, nil
	case tea.KeyMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m *Model) setNote(s string) { m.setNoteFor(s, noteFor) }

func (m *Model) setNoteFor(s string, d time.Duration) {
	m.note = s
	m.noteUntil = m.clock().Add(d)
}

// saveHistory writes a snapshot of the list off the UI loop. The sequence
// number lets the store drop a save that finishes after a newer one.
func (m *Model) saveHistory() tea.Cmd {
	store := m.deps.History
	if store == nil {
		return nil
	}
	m.histSeq++
	seq, snap := m.histSeq, append([]history.Entry(nil), m.recent...)
	return func() tea.Msg { return HistorySavedMsg{Err: store.Save(seq, snap)} }
}

func (m Model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Clearing needs a second "c"; any other key, or waiting, cancels it.
	armed := !m.confirmUntil.IsZero() && m.clock().Before(m.confirmUntil)
	m.confirmUntil = time.Time{}
	if msg.String() != "c" && m.note == confirmNote {
		m.note = ""
	}
	switch msg.String() {
	case "c":
		if armed {
			m.recent, m.note = nil, ""
			return m, m.saveHistory()
		}
		if len(m.recent) > 0 {
			m.confirmUntil = m.clock().Add(confirmFor)
			m.setNoteFor(confirmNote, confirmFor)
		}
		return m, nil
	case "q", "ctrl+c":
		return m, tea.Quit
	case " ":
		return m, m.send("play/pause", source.Cmd{Kind: source.TogglePlay})
	case "n", "l":
		return m, m.send("next", source.Cmd{Kind: source.Next})
	case "p", "h":
		return m, m.send("previous", source.Cmd{Kind: source.Previous})
	case "left":
		return m.seek(-seekStep)
	case "right":
		return m.seek(seekStep)
	case "+", "=":
		return m.changeVolume(volStep)
	case "-", "_":
		return m.changeVolume(-volStep)
	}
	return m, nil
}

// send runs a transport command off the UI loop. The stream reports the
// resulting state, so the model never guesses it.
func (m Model) send(op string, c source.Cmd) tea.Cmd {
	ctl := m.deps.Control
	if ctl == nil {
		return nil
	}
	return func() tea.Msg {
		if err := ctl(c); err != nil {
			return ControlErrMsg{Op: op, Err: err}
		}
		return nil
	}
}

func (m Model) seek(delta time.Duration) (tea.Model, tea.Cmd) {
	s := m.state
	if s.Empty() || s.Duration <= 0 {
		return m, nil
	}
	now := m.clock()
	base := s.Position(now)
	if !m.seekAt.IsZero() && now.Sub(m.seekAt) < seekChain {
		base = m.seekTo
	}
	target := max(0, min(base+delta, s.Duration))
	m.seekAt, m.seekTo = now, target
	return m, m.send("seek", source.Cmd{Kind: source.Seek, Seconds: target.Seconds()})
}

func (m Model) changeVolume(delta int) (tea.Model, tea.Cmd) {
	if m.deps.Volume == nil || m.volume < 0 {
		return m, nil
	}
	m.volume = max(0, min(m.volume+delta, 100))
	m.volHold = m.clock().Add(volHold)
	if m.volBusy {
		return m, nil // the in-flight write's completion sends the latest value
	}
	m.volBusy = true
	return m, setVolume(m.deps.Volume, m.volume)
}

func setVolume(v Volume, level int) tea.Cmd {
	return func() tea.Msg { return volSetMsg{sent: level, err: v.Set(level)} }
}
