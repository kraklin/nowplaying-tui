package history

import "time"

// Entry is one recently played track.
type Entry struct {
	Title    string        `json:"title"`
	Artist   string        `json:"artist"`
	Album    string        `json:"album"`
	Duration time.Duration `json:"duration"`
	App      string        `json:"app"`
	PlayedAt time.Time     `json:"played_at"`
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
