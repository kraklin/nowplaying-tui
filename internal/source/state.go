package source

import "time"

// State is the full now-playing snapshot. A zero Title means nothing is playing.
type State struct {
	Title     string
	Artist    string
	Album     string
	Duration  time.Duration
	Elapsed   time.Duration
	Timestamp time.Time
	Rate      float64
	Playing   bool
	App       string
	Art       []byte
}

// Empty reports whether nothing is playing.
func (s State) Empty() bool { return s.Title == "" }

// Position is the interpolated playback position at now, clamped to [0, Duration].
func (s State) Position(now time.Time) time.Duration {
	pos := s.Elapsed + time.Duration(float64(now.Sub(s.Timestamp))*s.Rate)
	if pos < 0 {
		return 0
	}
	if s.Duration > 0 && pos > s.Duration {
		return s.Duration
	}
	return pos
}

// Same reports whether two states describe the same track.
func (s State) Same(o State) bool {
	return s.Title == o.Title && s.Artist == o.Artist && s.Album == o.Album
}
