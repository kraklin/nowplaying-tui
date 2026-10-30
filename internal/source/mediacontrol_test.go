package source

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const recording = "../../testdata/stream.jsonl"

func recordedLines(t *testing.T) [][]byte {
	t.Helper()
	f, err := os.Open(recording)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var lines [][]byte
	r := bufio.NewReaderSize(f, 1<<20)
	for {
		line, err := r.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			lines = append(lines, line)
		}
		if err != nil {
			return lines
		}
	}
}

func replay(t *testing.T) []State {
	t.Helper()
	var m Merger
	var states []State
	for i, l := range recordedLines(t) {
		s, err := m.Apply(l)
		if err != nil {
			t.Fatalf("line %d: %v", i, err)
		}
		states = append(states, s)
	}
	return states
}

func TestRecordedStream(t *testing.T) {
	s := replay(t)
	if len(s) != 11 {
		t.Fatalf("got %d states, want 11", len(s))
	}
	if !s[0].Empty() {
		t.Errorf("line 0 (empty payload) should mean nothing playing: %+v", s[0])
	}
	first := s[1]
	if first.Title != "Idontgivea***K" || first.Artist != "Rabbit Junk" || first.Album != "Pop That Pretty Thirty" {
		t.Errorf("first track wrong: %q / %q / %q", first.Title, first.Artist, first.Album)
	}
	if first.Duration != 249040*time.Millisecond || first.Elapsed != 0 || !first.Playing || first.Rate != 1 {
		t.Errorf("first track timing wrong: %+v", first)
	}
	if first.App != "com.spotify.client" {
		t.Errorf("app = %q", first.App)
	}
	if len(first.Art) < 1000 || first.Art[0] != 0xFF || first.Art[1] != 0xD8 {
		t.Errorf("artwork should be decoded JPEG bytes, got %d bytes", len(first.Art))
	}

	paused := s[3] // playing:false, then playbackRate:null and a fresh elapsed time
	if paused.Playing || paused.Rate != 0 || paused.Elapsed != 39996*time.Millisecond {
		t.Errorf("paused state wrong: playing=%v rate=%v elapsed=%v", paused.Playing, paused.Rate, paused.Elapsed)
	}
	if paused.Title != first.Title || len(paused.Art) != len(first.Art) {
		t.Errorf("diff must keep title and artwork")
	}
	if !s[2].Same(first) || s[2].Rate != 0 {
		t.Errorf("playing:false alone must stop the bar even though playbackRate is still 1")
	}

	resumed := s[4] // playing:true while playbackRate is still null
	if !resumed.Playing || resumed.Rate != 1 {
		t.Errorf("resume must not freeze the bar: playing=%v rate=%v", resumed.Playing, resumed.Rate)
	}

	next := s[7]
	if next.Title != "Iridium" || next.Artist != "The Sidh" || next.Elapsed != 0 {
		t.Errorf("next track wrong: %+v", next)
	}
	if bytes.Equal(next.Art, first.Art) || len(next.Art) == 0 {
		t.Errorf("next track should carry its own artwork")
	}
	if s[9].Elapsed != 20*time.Second || !s[9].Same(next) {
		t.Errorf("seek: elapsed=%v", s[9].Elapsed)
	}
}

func TestInterpolationFromRecording(t *testing.T) {
	s := replay(t)[7]
	if got := s.Position(s.Timestamp.Add(10 * time.Second)); got != 10*time.Second {
		t.Errorf("position after 10s = %v", got)
	}
	paused := replay(t)[3]
	if got := paused.Position(paused.Timestamp.Add(time.Minute)); got != paused.Elapsed {
		t.Errorf("paused bar moved: %v", got)
	}
}

func TestMergeSemantics(t *testing.T) {
	var m Merger
	apply := func(line string) State {
		t.Helper()
		s, err := m.Apply([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	apply(`{"type":"data","diff":false,"payload":{"title":"A","artist":"X","playing":true,"artworkData":"AQID","durationMicros":5000000}}`)
	if s := apply(`{"type":"data","diff":true,"payload":{"elapsedTimeMicros":1000000}}`); s.Title != "A" || len(s.Art) != 3 || s.Elapsed != time.Second {
		t.Errorf("diff should merge: %+v", s)
	}
	if s := apply(`{"type":"data","diff":true,"payload":{"artworkData":null}}`); s.Art != nil {
		t.Errorf("null artwork should clear it")
	}
	if s := apply(`{"type":"data","diff":true,"payload":{"artworkData":"BAUG"}}`); len(s.Art) != 3 || s.Title != "A" {
		t.Errorf("late artwork diff: %+v", s)
	}
	if s := apply(`{"type":"data","diff":false,"payload":{"title":"B","playing":true}}`); s.Title != "B" || s.Artist != "" || s.Art != nil || s.Duration != 0 {
		t.Errorf("non-diff must replace everything, incl. artwork: %+v", s)
	}
	if s := apply(`{"type":"data","diff":false,"payload":{}}`); !s.Empty() {
		t.Errorf("empty payload = nothing playing")
	}
	if _, err := m.Apply([]byte(`not json`)); err == nil {
		t.Errorf("garbage should error")
	}
}

func TestRunRestartsWithBackoff(t *testing.T) {
	abs, _ := filepath.Abs(recording)
	mc := &MediaControl{Bin: "cat", StreamArgs: []string{abs}, Backoff: []time.Duration{20 * time.Millisecond}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out := make(chan State)
	go mc.Run(ctx, out)

	var sawReconnect, sawSecondRun bool
	titles := 0
	for s := range out {
		switch {
		case s.Status == StatusReconnecting:
			if s.Title != "Iridium" {
				t.Errorf("reconnecting note should keep the last track, got %q", s.Title)
			}
			sawReconnect = true
		case sawReconnect && s.Title == "Idontgivea***K":
			sawSecondRun = true
		case s.Title != "":
			titles++
		}
		if sawSecondRun {
			cancel()
			break
		}
	}
	if !sawReconnect || !sawSecondRun {
		t.Errorf("reconnect=%v secondRun=%v", sawReconnect, sawSecondRun)
	}
}

func TestMissingBinary(t *testing.T) {
	mc := &MediaControl{Bin: "definitely-not-a-real-binary"}
	out := make(chan State, 1)
	mc.Run(context.Background(), out)
	if s := <-out; s.Status != StatusMissing {
		t.Errorf("status = %v, want missing", s.Status)
	}
	if err := mc.Control(Cmd{Kind: Next}); err == nil {
		t.Errorf("control with missing binary should error")
	}
}

func countProcs(t *testing.T, marker string) int {
	t.Helper()
	out, _ := exec.Command("pgrep", "-f", marker).Output()
	return len(strings.Fields(string(out)))
}

// media-control is a wrapper that starts the real adapter as a child; killing
// only the wrapper would leave the adapter running.
func TestCancelKillsGrandchildren(t *testing.T) {
	const marker = "31.337"
	mc := &MediaControl{Bin: "sh", StreamArgs: []string{"-c", "sleep " + marker + " & wait"}}
	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan State, 4)
	done := make(chan struct{})
	go func() { mc.Run(ctx, out); close(done) }()

	deadline := time.Now().Add(5 * time.Second)
	for countProcs(t, marker) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if countProcs(t, marker) == 0 {
		t.Fatal("fake grandchild never started")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
	for end := time.Now().Add(3 * time.Second); countProcs(t, marker) > 0 && time.Now().Before(end); {
		time.Sleep(50 * time.Millisecond)
	}
	if n := countProcs(t, marker); n > 0 {
		exec.Command("pkill", "-f", marker).Run()
		t.Errorf("%d grandchild process(es) survived cancel", n)
	}
}
