package history

import (
	"time"

	"github.com/tomaslatall/nowplaying-tui/internal/source"
)

// DefaultMinPlay is how long a track must have played to enter the history.
const DefaultMinPlay = 10 * time.Second

// Tracker watches source states and reports tracks that have finished.
//
// A track is identified by title and artist. Time counts only while playing,
// so pausing does not help a skipped track over the threshold, and rapid
// skipping does not flood the list.
type Tracker struct {
	MinPlay time.Duration
	cur     *current
	seen    bool
}

type current struct {
	entry    Entry
	key      string
	start    time.Time
	played   time.Duration
	resumeAt time.Time // zero while paused
}

// NewTracker returns a tracker that needs minPlay of real play time; zero
// records every track.
func NewTracker(minPlay time.Duration) *Tracker {
	return &Tracker{MinPlay: max(minPlay, 0)}
}

// Observe feeds the tracker one state. It returns the entry of the track that
// just ended, if it played long enough.
func (t *Tracker) Observe(s source.State, now time.Time) *Entry {
	// While the source reconnects the state is a stale copy; ignore it.
	if s.Status != source.StatusOK {
		return nil
	}
	if s.Empty() {
		done := t.finish(now)
		t.cur = nil
		return done
	}
	k := key(s.Title, s.Artist)
	if t.cur == nil || t.cur.key != k {
		done := t.finish(now)
		t.begin(s, k, now)
		return done
	}

	c := t.cur
	c.entry.Album = s.Album
	if s.Duration > 0 {
		c.entry.Duration = s.Duration
	}
	c.entry.App = s.App
	switch {
	case s.Playing && c.resumeAt.IsZero():
		c.resumeAt = now
	case !s.Playing && !c.resumeAt.IsZero():
		c.played += now.Sub(c.resumeAt)
		c.resumeAt = time.Time{}
	}
	return nil
}

func (t *Tracker) begin(s source.State, k string, now time.Time) {
	c := &current{
		entry: Entry{Title: s.Title, Artist: s.Artist, Album: s.Album, Duration: s.Duration, App: s.App},
		key:   k,
		start: now,
	}
	// The first track we ever see may already be well under way.
	if !t.seen {
		c.played = s.Position(now)
		c.start = now.Add(-c.played)
	}
	if s.Playing {
		c.resumeAt = now
	}
	t.cur, t.seen = c, true
}

func (t *Tracker) finish(now time.Time) *Entry {
	c := t.cur
	if c == nil {
		return nil
	}
	played := c.played
	if !c.resumeAt.IsZero() {
		played += now.Sub(c.resumeAt)
	}
	if played < t.MinPlay {
		return nil
	}
	e := c.entry
	e.PlayedAt = c.start
	return &e
}
