package ui

import (
	"fmt"
	"image/color"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tomaslatall/nowplaying-tui/internal/art"
	"github.com/tomaslatall/nowplaying-tui/internal/config"
	"github.com/tomaslatall/nowplaying-tui/internal/history"
	"github.com/tomaslatall/nowplaying-tui/internal/source"
)

var sprintf = fmt.Sprintf

// StateMsg carries a new source state.
type StateMsg source.State

// TickMsg drives the progress bar.
type TickMsg time.Time

// VolumeMsg is the result of polling the system volume.
type VolumeMsg struct {
	Level int
	Muted bool
	Err   error
}

type volPollMsg struct{}

type volSetMsg struct {
	level int
	muted bool
	err   error
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
	warnFor     = 8 * time.Second
)

// Volume is the system volume. Calls are slow and must run inside a tea.Cmd.
type Volume interface {
	Get() (int, error)
	Set(int) error
}

// Muter is implemented by volume backends that can also mute. Read returns the
// level and the mute state in one round trip, which is what the 2 s poll uses.
type Muter interface {
	Read() (level int, muted bool, err error)
	SetMuted(bool) error
}

// Deps are the collaborators and settings the model is built from; all are
// optional.
type Deps struct {
	Control func(source.Cmd) error
	Volume  Volume
	History *history.Store // nil keeps the history in memory only
	Config  *config.Config // nil means config.Default()
	// Warnings (from loading the config) are shown briefly on the label row.
	Warnings []string
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
	muted     bool
	volBusy   bool
	volHold   time.Time
	seekAt    time.Time
	seekTo    time.Duration
	note      string
	noteTail  string // always kept visible when the note is truncated
	noteUntil time.Time

	cfg          config.Config
	help         bool
	showHistory  bool
	showCover    bool
	showSource   bool
	tracker      *history.Tracker
	histMax      int
	histSeq      uint64
	confirmUntil time.Time
}

// New builds the model; bg is the terminal background colour.
func New(states <-chan source.State, recent []history.Entry, bg color.RGBA, deps Deps) Model {
	cfg := config.Default()
	if deps.Config != nil {
		cfg = *deps.Config
	}
	opts := art.DefaultOptions(bg)
	opts.Quantize, opts.Scanlines, opts.Accent = cfg.QuantizeColors, cfg.Scanlines, cfg.Accent
	m := Model{
		cfg:         cfg,
		showHistory: cfg.ShowHistory,
		showCover:   cfg.ShowCover,
		showSource:  cfg.ShowSource,
		tracker:     history.NewTracker(time.Duration(cfg.MinPlaySeconds) * time.Second),
		histMax:     cfg.HistorySize,
		src:         states,
		deps:        deps,
		clock:       time.Now,
		recent:      recent,
		volume:      -1,
		now:         time.Now(),
		cache:       map[artKey]art.Result{},
		pending:     map[artKey]bool{},
		icons:       iconSet(cfg.Icons),
		opts:        opts,
		th:          newTheme(bg),
	}
	if len(deps.Warnings) > 0 {
		m.setNoteFor(deps.Warnings[0], warnFor)
		if n := len(deps.Warnings) - 1; n > 0 {
			m.noteTail = sprintf(" (+%d more)", n)
		}
	}
	return m
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
		if mv, ok := v.(Muter); ok {
			level, muted, err := mv.Read()
			return VolumeMsg{Level: level, Muted: muted, Err: err}
		}
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
			res = art.Placeholder(key.px, opts.BG, opts.Accent)
		} else if r, err := art.Build(data, key.px, opts); err == nil {
			res = r
		} else {
			res = art.Placeholder(key.px, opts.BG, opts.Accent)
		}
		return ArtMsg{Hash: key.hash, Px: key.px, Result: res}
	}
}

// accentPx is the size cover that is built just to pick the accent colour when no
// cover is shown (text-only mode, or a pane too small for one).
const accentPx = 16

// wantArt returns the cache key for the cover the current state and layout need.
// Without a visible cover it is a small one, so the accent still follows the music.
func (m Model) wantArt() (artKey, bool) {
	px := m.layout().coverPx
	if px == 0 {
		px = accentPx
	}
	hash := placeholderHash
	if !m.state.Empty() && len(m.state.Art) > 0 {
		hash = art.Hash(m.state.Art)
	}
	return artKey{hash, px}, true
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
			m.note, m.noteTail = "", ""
		}
		return m, tick()
	case VolumeMsg:
		next := tea.Tick(volPollEvery, func(time.Time) tea.Msg { return volPollMsg{} })
		switch {
		case msg.Err != nil:
			m.volume, m.muted = -1, false
		case !m.volBusy && !m.clock().Before(m.volHold):
			m.volume, m.muted = msg.Level, msg.Muted
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
		if m.volume != msg.level || m.muted != msg.muted { // keys were pressed while the last write ran
			m.volBusy = true
			return m, setVolume(m.deps.Volume, m.volume, m.muted)
		}
		return m, nil
	case ControlErrMsg:
		m.setNote(msg.Op + " failed")
		return m, nil
	case tea.KeyMsg:
		return m.key(msg)
	case tea.MouseMsg:
		return m.mouse(msg)
	}
	return m, nil
}

func (m *Model) setNote(s string) { m.setNoteFor(s, noteFor) }

func (m *Model) setNoteFor(s string, d time.Duration) {
	m.note, m.noteTail = s, ""
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
	case "?":
		m.help = !m.help
		return m, nil
	// These change the layout, and with it possibly the cover's size, so ask for
	// a render of any size we do not have yet.
	case "r":
		m.showHistory = !m.showHistory
		return m, m.ensureArt()
	case "s":
		m.showSource = !m.showSource
		return m, nil
	case "a":
		m.showCover = !m.showCover
		return m, m.ensureArt()
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
	case "m":
		return m.toggleMute()
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
	return m.seekTarget(base + delta)
}

// seekTarget seeks to an absolute position, clamped to the track.
func (m Model) seekTarget(target time.Duration) (tea.Model, tea.Cmd) {
	s := m.state
	if s.Empty() || s.Duration <= 0 {
		return m, nil
	}
	target = max(0, min(target, s.Duration))
	m.seekAt, m.seekTo = m.clock(), target
	return m, m.send("seek", source.Cmd{Kind: source.Seek, Seconds: target.Seconds()})
}

// mouse handles clicks on the transport icons and progress bar and scrolling
// over the volume bar. Positions come from the same layout View draws.
func (m Model) mouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.state.Status == source.StatusMissing || msg.Action != tea.MouseActionPress {
		return m, nil
	}
	_, g := m.compose()
	if (msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) &&
		g.volRow >= 0 && msg.Y == g.volRow && msg.X >= g.volX0 && msg.X < g.volX1 {
		if msg.Button == tea.MouseButtonWheelUp {
			return m.changeVolume(volStep)
		}
		return m.changeVolume(-volStep)
	}
	row, col := msg.Y-g.infoY, msg.X-g.infoX
	if col < 0 || col >= g.infoW {
		return m, nil
	}
	_, hits := m.transportRow(accentStyle("#000000"), g.infoW)
	switch msg.Button {
	case tea.MouseButtonLeft:
		switch {
		case row == g.progRow && g.progRow >= 0:
			if bw := m.barWidth(g.infoW); bw > 1 && col < bw && !m.state.Empty() {
				frac := float64(col) / float64(bw-1)
				return m.seekTarget(time.Duration(frac * float64(m.state.Duration)))
			}
		case row == g.ctrlRow:
			for _, h := range hits {
				if col < h.x0 || col >= h.x1 {
					continue
				}
				switch h.act {
				case actPrev:
					return m, m.send("previous", source.Cmd{Kind: source.Previous})
				case actNext:
					return m, m.send("next", source.Cmd{Kind: source.Next})
				case actPlay:
					return m, m.send("play/pause", source.Cmd{Kind: source.TogglePlay})
				case actBack:
					return m.seek(-seekStep) // same as the left arrow
				case actFwd:
					return m.seek(seekStep)
				}
			}
		}
	}
	return m, nil
}

func (m Model) changeVolume(delta int) (tea.Model, tea.Cmd) {
	if m.deps.Volume == nil || m.volume < 0 {
		return m, nil
	}
	m.volume = max(0, min(m.volume+delta, 100))
	m.muted = false // adjusting the volume unmutes, as in most players
	return m.writeVolume()
}

// toggleMute flips the system mute switch; the level is left alone, so unmuting
// restores it. It does nothing without a backend that can mute.
func (m Model) toggleMute() (tea.Model, tea.Cmd) {
	if _, ok := m.deps.Volume.(Muter); !ok || m.volume < 0 {
		return m, nil
	}
	m.muted = !m.muted
	return m.writeVolume()
}

// writeVolume sends the wanted level and mute state. Writes are serialised: while
// one is running, later key presses only update the wanted state, and the running
// write's completion sends the latest one.
func (m Model) writeVolume() (tea.Model, tea.Cmd) {
	m.volHold = m.clock().Add(volHold)
	if m.volBusy {
		return m, nil
	}
	m.volBusy = true
	return m, setVolume(m.deps.Volume, m.volume, m.muted)
}

func setVolume(v Volume, level int, muted bool) tea.Cmd {
	return func() tea.Msg {
		var err error
		if !muted { // muting leaves the level alone; unmuting sets it first
			err = v.Set(level)
		}
		if mv, ok := v.(Muter); ok && err == nil {
			err = mv.SetMuted(muted)
		}
		return volSetMsg{level: level, muted: muted, err: err}
	}
}
