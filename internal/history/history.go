// Package history keeps the recently-played list: deciding what counts as
// played (Tracker), keeping the list tidy (Push) and persisting it (Store).
package history

import (
	"encoding/json"
	"strings"
	"time"
)

// Entry is one recently played track.
type Entry struct {
	Title    string
	Artist   string
	Album    string
	Duration time.Duration
	App      string
	PlayedAt time.Time
}

type entryJSON struct {
	Title       string    `json:"title"`
	Artist      string    `json:"artist"`
	Album       string    `json:"album"`
	DurationSec float64   `json:"duration_seconds"`
	App         string    `json:"app"`
	PlayedAt    time.Time `json:"played_at"`
}

func (e Entry) MarshalJSON() ([]byte, error) {
	return json.Marshal(entryJSON{e.Title, e.Artist, e.Album, e.Duration.Seconds(), e.App, e.PlayedAt})
}

func (e *Entry) UnmarshalJSON(b []byte) error {
	var j entryJSON
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}
	*e = Entry{j.Title, j.Artist, j.Album, time.Duration(j.DurationSec * float64(time.Second)), j.App, j.PlayedAt}
	return nil
}

// Same reports whether two entries are the same track. The album is ignored
// because players change it mid-track (e.g. "… (Deluxe Edition)").
func Same(a, b Entry) bool { return key(a.Title, a.Artist) == key(b.Title, b.Artist) }

func key(title, artist string) string {
	return strings.ToLower(strings.TrimSpace(title)) + "\x00" + strings.ToLower(strings.TrimSpace(artist))
}

// Push adds e as the newest entry (index 0). The same track twice in a row is
// ignored, and the list is cut to max entries. The input slice is not modified.
func Push(entries []Entry, e Entry, max int) []Entry {
	if len(entries) > 0 && Same(entries[0], e) {
		return entries
	}
	out := make([]Entry, 0, len(entries)+1)
	out = append(out, e)
	out = append(out, entries...)
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

// Demo returns canned entries for --demo.
func Demo() []Entry {
	return []Entry{
		{Title: "Paper Satellites", Artist: "Neon Ferns", Album: "Night Signals", Duration: 201 * time.Second},
		{Title: "Glass Harbour", Artist: "Ostra", Album: "Tidelines", Duration: 242 * time.Second},
		{Title: "Slow Static", Artist: "Kites at Dusk", Album: "Paper Weather", Duration: 167 * time.Second},
		{Title: "Salt & Signal", Artist: "Ostra", Album: "Tidelines", Duration: 215 * time.Second},
		{Title: "Hollow Light", Artist: "Kites at Dusk", Album: "Paper Weather", Duration: 189 * time.Second},
	}
}
