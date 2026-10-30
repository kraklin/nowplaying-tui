package source

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"math"
	"sync"
	"time"
)

type fakeTrack struct {
	title, artist, album string
	duration             time.Duration
	hue                  float64
	art                  []byte
}

// Fake is a scripted Source for --demo and tests.
type Fake struct {
	mu      sync.Mutex
	tracks  []fakeTrack
	idx     int
	playing bool
	elapsed time.Duration
	since   time.Time
	wake    chan struct{}
}

func NewFake() *Fake {
	f := &Fake{
		tracks: []fakeTrack{
			{title: "Low Orbit", artist: "Neon Ferns", album: "Night Signals", duration: 238 * time.Second, hue: 275},
			{title: "Paper Satellites", artist: "Neon Ferns", album: "Night Signals", duration: 201 * time.Second, hue: 190},
			{title: "Glass Harbour", artist: "Ostra", album: "Tidelines", duration: 242 * time.Second, hue: 18},
			{title: "Slow Static", artist: "Kites at Dusk", album: "Paper Weather", duration: 167 * time.Second, hue: 130},
		},
		playing: true,
		elapsed: 134 * time.Second,
		since:   time.Now(),
		wake:    make(chan struct{}, 1),
	}
	for i := range f.tracks {
		f.tracks[i].art = GenerateCover(f.tracks[i].hue, 96)
	}
	return f
}

// Snapshot returns the current state without running the source.
func (f *Fake) Snapshot() State {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snapshot(time.Now())
}

func (f *Fake) snapshot(now time.Time) State {
	t := f.tracks[f.idx]
	rate := 0.0
	if f.playing {
		rate = 1
	}
	return State{
		Title: t.title, Artist: t.artist, Album: t.album,
		Duration: t.duration, Elapsed: f.elapsed, Timestamp: now,
		Rate: rate, Playing: f.playing, App: "com.spotify.client", Art: t.art,
	}
}

// advance folds elapsed play time into f.elapsed. Caller holds f.mu.
func (f *Fake) advance(now time.Time) {
	if f.playing {
		f.elapsed += now.Sub(f.since)
	}
	f.since = now
}

func (f *Fake) goTo(i int) {
	f.idx = (i + len(f.tracks)) % len(f.tracks)
	f.elapsed = 0
}

func (f *Fake) Run(ctx context.Context, out chan<- State) {
	send := func() bool {
		f.mu.Lock()
		s := f.snapshot(time.Now())
		f.mu.Unlock()
		select {
		case out <- s:
			return true
		case <-ctx.Done():
			return false
		}
	}
	if !send() {
		return
	}
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-f.wake:
		case <-tick.C:
			f.mu.Lock()
			now := time.Now()
			f.advance(now)
			ended := f.playing && f.elapsed >= f.tracks[f.idx].duration
			if ended {
				f.goTo(f.idx + 1)
			}
			f.mu.Unlock()
			if !ended {
				continue
			}
		}
		if !send() {
			return
		}
	}
}

func (f *Fake) Control(c Cmd) error {
	f.mu.Lock()
	f.advance(time.Now())
	switch c.Kind {
	case TogglePlay:
		f.playing = !f.playing
	case Next:
		f.goTo(f.idx + 1)
	case Previous:
		f.goTo(f.idx - 1)
	case Seek:
		d := time.Duration(c.Seconds * float64(time.Second))
		f.elapsed = max(0, min(d, f.tracks[f.idx].duration))
	}
	f.mu.Unlock()
	select {
	case f.wake <- struct{}{}:
	default:
	}
	return nil
}

// GenerateCover returns a PNG of a gradient with a few soft discs, tinted by hue.
func GenerateCover(hue float64, size int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			u, v := float64(x)/float64(size), float64(y)/float64(size)
			h := hue + 50*u
			l := 0.18 + 0.45*(1-v)
			r, g, b := hsl(h, 0.65, l)
			for _, d := range [][3]float64{{0.7, 0.3, 0.22}, {0.3, 0.7, 0.14}} {
				if dist := math.Hypot(u-d[0], v-d[1]); dist < d[2] {
					r, g, b = hsl(h+150, 0.8, 0.6-0.3*dist/d[2])
				}
			}
			img.Set(x, y, color.RGBA{r, g, b, 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func hsl(h, s, l float64) (uint8, uint8, uint8) {
	h = math.Mod(h, 360) / 60
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h, 2)-1))
	var r, g, b float64
	switch int(h) {
	case 0:
		r, g, b = c, x, 0
	case 1:
		r, g, b = x, c, 0
	case 2:
		r, g, b = 0, c, x
	case 3:
		r, g, b = 0, x, c
	case 4:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	m := l - c/2
	return uint8((r + m) * 255), uint8((g + m) * 255), uint8((b + m) * 255)
}
