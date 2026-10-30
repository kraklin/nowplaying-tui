package history

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kraklin/nowplaying-tui/internal/source"
)

var t0 = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

func at(sec float64) time.Time { return t0.Add(time.Duration(sec * float64(time.Second))) }

func entry(title, artist string) Entry { return Entry{Title: title, Artist: artist} }

func titles(es []Entry) string {
	var out []string
	for _, e := range es {
		out = append(out, e.Title)
	}
	return strings.Join(out, ",")
}

func TestPushNewestFirstAndCap(t *testing.T) {
	var l []Entry
	for _, n := range []string{"a", "b", "c", "d"} {
		l = Push(l, entry(n, "x"), 3)
	}
	if got := titles(l); got != "d,c,b" {
		t.Errorf("got %s, want d,c,b (newest first, oldest dropped)", got)
	}
}

func TestPushSkipsSameTrackInARow(t *testing.T) {
	l := Push(nil, Entry{Title: "Song", Artist: "Band", Album: "One"}, 10)
	l = Push(l, Entry{Title: "  song ", Artist: "BAND", Album: "Deluxe"}, 10)
	if len(l) != 1 {
		t.Errorf("same track (case, spacing, album aside) twice in a row should be skipped: %v", titles(l))
	}
	l = Push(l, entry("Other", "Band"), 10)
	l = Push(l, entry("Song", "Band"), 10)
	if got := titles(l); got != "Song,Other,Song" {
		t.Errorf("a track may return after another one: %s", got)
	}
}

func TestPushDoesNotModifyInputAndHandlesBadMax(t *testing.T) {
	in := []Entry{entry("a", "x")}
	out := Push(in, entry("b", "x"), 10)
	if len(in) != 1 || in[0].Title != "a" || len(out) != 2 {
		t.Errorf("input must stay untouched: in=%v out=%v", titles(in), titles(out))
	}
	if got := Push(in, entry("c", "x"), -1); len(got) != 2 {
		t.Errorf("non-positive max means no cap, got %d", len(got))
	}
}

func st(title, artist, album string, playing bool, elapsed time.Duration, ts time.Time) source.State {
	rate := 0.0
	if playing {
		rate = 1
	}
	return source.State{Title: title, Artist: artist, Album: album, Duration: 200 * time.Second,
		Elapsed: elapsed, Timestamp: ts, Rate: rate, Playing: playing, App: "com.spotify.client"}
}

func TestTrackerPushesTrackThatPlayedLongEnough(t *testing.T) {
	tr := NewTracker(10 * time.Second)
	if e := tr.Observe(st("A", "x", "Alb", true, 0, at(0)), at(0)); e != nil {
		t.Fatalf("first track must not produce an entry")
	}
	e := tr.Observe(st("B", "x", "Alb", true, 0, at(30)), at(30))
	if e == nil || e.Title != "A" || e.Album != "Alb" || e.Duration != 200*time.Second || e.App != "com.spotify.client" {
		t.Fatalf("A played 30 s and should be reported with its details, got %+v", e)
	}
	if !e.PlayedAt.Equal(at(0)) {
		t.Errorf("PlayedAt should be when the track started, got %v", e.PlayedAt)
	}
}

func TestTrackerIgnoresRapidSkipping(t *testing.T) {
	tr := NewTracker(10 * time.Second)
	tr.Observe(st("A", "x", "", true, 0, at(0)), at(0))
	for i, n := range []string{"B", "C", "D", "E"} {
		sec := float64(i+1) * 3
		if e := tr.Observe(st(n, "x", "", true, 0, at(sec)), at(sec)); e != nil {
			t.Errorf("skipped after 3 s, %s must not be pushed: %+v", n, e)
		}
	}
}

func TestTrackerPausedTimeDoesNotCount(t *testing.T) {
	run := func(secondPlay float64) *Entry {
		tr := NewTracker(10 * time.Second)
		tr.Observe(st("A", "x", "", true, 0, at(0)), at(0))
		tr.Observe(st("A", "x", "", false, 6*time.Second, at(6)), at(6))  // pause after 6 s
		tr.Observe(st("A", "x", "", true, 6*time.Second, at(66)), at(66)) // a minute later resume
		end := 66 + secondPlay
		return tr.Observe(st("B", "x", "", true, 0, at(end)), at(end))
	}
	if e := run(3); e != nil {
		t.Errorf("6 s + 3 s = 9 s of real play is under the threshold: %+v", e)
	}
	if e := run(4); e == nil {
		t.Errorf("6 s + 4 s = 10 s should count")
	}
}

func TestTrackerSkippedWhilePausedStillCountsPlayedTime(t *testing.T) {
	tr := NewTracker(10 * time.Second)
	tr.Observe(st("A", "x", "", true, 0, at(0)), at(0))
	tr.Observe(st("A", "x", "", false, 40*time.Second, at(40)), at(40))
	if e := tr.Observe(st("B", "x", "", false, 0, at(100)), at(100)); e == nil || e.Title != "A" {
		t.Errorf("A played 40 s before the pause: %+v", e)
	}
}

func TestTrackerAlbumFlickerIsOneTrack(t *testing.T) {
	// Seen in the recording: the album flips between "Brave Enough (Deluxe Edition)"
	// and "Brave Enough" while the same song plays on.
	tr := NewTracker(10 * time.Second)
	tr.Observe(st("The Arena", "Lindsey", "Brave Enough (Deluxe Edition)", true, 0, at(0)), at(0))
	tr.Observe(st("The Arena", "Lindsey", "Brave Enough", true, 2*time.Second, at(2)), at(2))
	flick := st("The Arena", "Lindsey", "Brave Enough (Deluxe Edition)", true, 20*time.Second, at(20))
	flick.Duration = 199 * time.Second // and Spotify also revises the duration
	if e := tr.Observe(flick, at(20)); e != nil {
		t.Fatalf("an album change is not a new track: %+v", e)
	}
	e := tr.Observe(st("Next", "Lindsey", "Other", true, 0, at(45)), at(45))
	if e == nil || e.Title != "The Arena" || e.Album != "Brave Enough (Deluxe Edition)" || e.Duration != 199*time.Second {
		t.Errorf("one entry with the latest metadata expected, got %+v", e)
	}
}

func TestTrackerIgnoresReconnectingAndMissing(t *testing.T) {
	tr := NewTracker(10 * time.Second)
	tr.Observe(st("A", "x", "", true, 0, at(0)), at(0))
	stale := st("A", "x", "", true, 0, at(0))
	stale.Status = source.StatusReconnecting
	if e := tr.Observe(stale, at(5)); e != nil {
		t.Errorf("reconnecting must not end the track")
	}
	if e := tr.Observe(source.State{Status: source.StatusMissing}, at(6)); e != nil {
		t.Errorf("missing must not end the track")
	}
	if e := tr.Observe(st("B", "x", "", true, 0, at(30)), at(30)); e == nil || e.Title != "A" {
		t.Errorf("A kept playing through the reconnect: %+v", e)
	}
}

func TestTrackerNothingPlayingEndsTheTrack(t *testing.T) {
	tr := NewTracker(10 * time.Second)
	tr.Observe(st("A", "x", "", true, 0, at(0)), at(0))
	if e := tr.Observe(source.State{}, at(20)); e == nil || e.Title != "A" {
		t.Fatalf("player closed after 20 s: %+v", e)
	}
	if e := tr.Observe(source.State{}, at(25)); e != nil {
		t.Errorf("nothing is playing, nothing to report")
	}
	if e := tr.Observe(st("A", "x", "", true, 0, at(30)), at(30)); e != nil {
		t.Errorf("starting a track never reports")
	}
}

func TestTrackerCreditsTheFirstTrackForTimeBeforeWeStarted(t *testing.T) {
	tr := NewTracker(10 * time.Second)
	tr.Observe(st("A", "x", "", true, 120*time.Second, at(0)), at(0)) // app started mid-song
	e := tr.Observe(st("B", "x", "", true, 0, at(1)), at(1))
	if e == nil || e.Title != "A" {
		t.Fatalf("A was already 2 minutes in; skipping after 1 s should still record it: %+v", e)
	}
	if !e.PlayedAt.Before(at(-100)) {
		t.Errorf("PlayedAt should be back-dated to the song start, got %v", e.PlayedAt)
	}
	// ...but a later track that merely starts at an offset gets no such credit.
	tr.Observe(st("C", "x", "", true, 90*time.Second, at(2)), at(2))
	if e := tr.Observe(st("D", "x", "", true, 0, at(3)), at(3)); e != nil {
		t.Errorf("only the very first track is credited: %+v", e)
	}
}

// A stream line's own timestamp is the position anchor, not its arrival time
// (a "playing":false line still carries the old anchor), so arrival times are
// rebuilt: a line with a fresh timestamp is stamped with it, others inherit the
// previous arrival, and the first line arrives 3 s before the pause because the
// recording script slept 3 s first.
func TestTrackerOnRecording(t *testing.T) {
	f, err := os.ReadFile("../../testdata/stream.jsonl")
	if err != nil {
		t.Skip("no recording")
	}
	var lines [][]byte
	for _, l := range bytes.Split(f, []byte("\n")) {
		if len(bytes.TrimSpace(l)) > 0 {
			lines = append(lines, l)
		}
	}
	freshTS := func(line []byte) (time.Time, bool) {
		var env struct {
			Payload map[string]json.RawMessage `json:"payload"`
		}
		if json.Unmarshal(line, &env) != nil {
			return time.Time{}, false
		}
		raw, ok := env.Payload["timestampEpochMicros"]
		if !ok {
			return time.Time{}, false
		}
		var us int64
		json.Unmarshal(raw, &us)
		return time.UnixMicro(us), true
	}
	pause, _ := freshTS(lines[3]) // the first diff that moves the anchor: the pause
	arrival := pause.Add(-3 * time.Second)

	var m source.Merger
	tr := NewTracker(DefaultMinPlay)
	var pushed []string
	for i, line := range lines {
		s, err := m.Apply(line)
		if err != nil {
			t.Fatal(err)
		}
		if ts, ok := freshTS(line); ok && i > 1 {
			arrival = ts
		}
		if e := tr.Observe(s, arrival); e != nil {
			pushed = append(pushed, e.Title)
		}
	}
	if len(pushed) != 1 || pushed[0] != "Idontgivea***K" {
		t.Errorf("one track finished after playing well over 10 s (Iridium only just started): got %v", pushed)
	}
}

func newStore(t *testing.T, max int) *Store {
	t.Helper()
	return NewStore(filepath.Join(t.TempDir(), "state", "nowplaying", "history.json"), max)
}

func TestStoreRoundTrip(t *testing.T) {
	s := newStore(t, 50)
	in := []Entry{
		{Title: "A", Artist: "x", Album: "Alb", Duration: 201*time.Second + 500*time.Millisecond, App: "com.spotify.client", PlayedAt: t0},
		{Title: "B", Artist: "y", PlayedAt: t0.Add(-time.Hour)},
	}
	if err := s.Save(1, in); err != nil {
		t.Fatal(err)
	}
	out, err := s.Load()
	if err != nil || len(out) != 2 {
		t.Fatalf("load: %v %v", out, err)
	}
	for i := range in {
		if out[i].Title != in[i].Title || out[i].Duration != in[i].Duration || !out[i].PlayedAt.Equal(in[i].PlayedAt) || out[i].App != in[i].App {
			t.Errorf("entry %d changed: %+v vs %+v", i, out[i], in[i])
		}
	}
	raw, _ := os.ReadFile(s.Path)
	if !strings.Contains(string(raw), `"duration_seconds": 201.5`) {
		t.Errorf("durations should be readable seconds in the file:\n%s", raw)
	}
}

func TestStoreMissingFileIsEmpty(t *testing.T) {
	if out, err := newStore(t, 50).Load(); err != nil || out != nil {
		t.Errorf("missing file: %v, %v", out, err)
	}
}

func TestStoreSaveIsAtomicAndTidy(t *testing.T) {
	s := newStore(t, 50)
	for i := uint64(1); i <= 5; i++ {
		if err := s.Save(i, []Entry{entry("A", "x")}); err != nil {
			t.Fatal(err)
		}
	}
	files, _ := os.ReadDir(filepath.Dir(s.Path))
	if len(files) != 1 || files[0].Name() != "history.json" {
		var names []string
		for _, f := range files {
			names = append(names, f.Name())
		}
		t.Errorf("only history.json should remain (no temp files): %v", names)
	}
	var f struct{ Version int }
	raw, _ := os.ReadFile(s.Path)
	if err := json.Unmarshal(raw, &f); err != nil || f.Version != 1 {
		t.Errorf("file must be valid JSON with a version: %v %s", err, raw)
	}
}

func TestStoreDropsStaleSaves(t *testing.T) {
	s := newStore(t, 50)
	s.Save(5, []Entry{entry("new", "x")})
	s.Save(3, []Entry{entry("old", "x")}) // a slower, older save arriving late
	out, _ := s.Load()
	if len(out) != 1 || out[0].Title != "new" {
		t.Errorf("an older save must not overwrite a newer one: %v", titles(out))
	}
}

func TestStoreCapsOnSaveAndLoad(t *testing.T) {
	big := newStore(t, 100)
	var l []Entry
	for i := 0; i < 80; i++ {
		l = append(l, entry(string(rune('a'+i%26))+string(rune('A'+i/26)), "x"))
	}
	big.Save(1, l)
	small := NewStore(big.Path, 10)
	if out, _ := small.Load(); len(out) != 10 {
		t.Errorf("load should cut to the configured size, got %d", len(out))
	}
	small.Save(1, l)
	if out, _ := NewStore(big.Path, 100).Load(); len(out) != 10 {
		t.Errorf("save should cut to the configured size, got %d", len(out))
	}
}

func TestStoreCorruptFileIsMovedAside(t *testing.T) {
	s := newStore(t, 50)
	os.MkdirAll(filepath.Dir(s.Path), 0o755)
	os.WriteFile(s.Path, []byte("{ not json"), 0o644)
	out, err := s.Load()
	if err == nil || !strings.Contains(err.Error(), "corrupt") || out != nil {
		t.Errorf("corrupt file should report ErrCorrupt and load empty: %v %v", out, err)
	}
	if _, err := os.Stat(s.Path + ".corrupt"); err != nil {
		t.Errorf("the bad file should be kept as .corrupt: %v", err)
	}
	if err := s.Save(1, []Entry{entry("A", "x")}); err != nil {
		t.Errorf("saving after a corrupt load must work: %v", err)
	}
}

func TestStoreSaveErrorIsReported(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores permissions")
	}
	dir := t.TempDir()
	os.Chmod(dir, 0o500)
	defer os.Chmod(dir, 0o700)
	s := NewStore(filepath.Join(dir, "sub", "history.json"), 50)
	if err := s.Save(1, []Entry{entry("A", "x")}); err == nil {
		t.Errorf("an unwritable directory must surface as an error")
	}
}

func TestDefaultPath(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/xdg-state")
	if p, _ := DefaultPath(); p != "/tmp/xdg-state/nowplaying/history.json" {
		t.Errorf("XDG path: %s", p)
	}
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/Users/someone")
	if p, _ := DefaultPath(); p != "/Users/someone/Library/Application Support/nowplaying/history.json" {
		t.Errorf("fallback path: %s", p)
	}
}

// Atomic means: the new content arrives via a rename (a different file), and
// a reader racing the writer never sees an empty or half-written file.
func TestStoreSaveIsAtomicForConcurrentReaders(t *testing.T) {
	s := newStore(t, 5000)
	big := make([]Entry, 2000)
	for i := range big {
		big[i] = Entry{Title: strings.Repeat("t", 40), Artist: "artist", Album: "album", PlayedAt: t0}
	}
	if err := s.Save(1, big); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(s.Path)
	if err := s.Save(2, big); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(s.Path)
	if os.SameFile(before, after) {
		t.Errorf("Save rewrote the file in place; it must write a temp file and rename it")
	}

	stop := make(chan struct{})
	bad := make(chan string, 1)
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			b, err := os.ReadFile(s.Path)
			if err == nil && !json.Valid(b) {
				select {
				case bad <- fmt.Sprintf("%d bytes, starting %q", len(b), b[:min(len(b), 20)]):
				default:
				}
				return
			}
		}
	}()
	for i := uint64(3); i < 150; i++ {
		if err := s.Save(i, big); err != nil {
			t.Fatal(err)
		}
	}
	close(stop)
	select {
	case msg := <-bad:
		t.Errorf("a concurrent reader saw a partial file: %s", msg)
	default:
	}
}

func TestStoreWritesEmptyListAsArray(t *testing.T) {
	s := newStore(t, 50)
	if err := s.Save(1, nil); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(s.Path)
	if !strings.Contains(string(raw), `"entries": []`) {
		t.Errorf("a cleared history should be an empty array, got:\n%s", raw)
	}
	if out, err := s.Load(); err != nil || len(out) != 0 {
		t.Errorf("and load back as empty: %v %v", out, err)
	}
}
