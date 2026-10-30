package source

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// Merger folds `media-control stream --micros` lines into a full State.
//
// Each line is {"type":"data","diff":bool,"payload":{...}}. A non-diff line
// replaces everything; a diff line merges keys, and null clears a key.
type Merger struct {
	raw map[string]json.RawMessage
	art []byte
}

type envelope struct {
	Type    string                     `json:"type"`
	Diff    bool                       `json:"diff"`
	Payload map[string]json.RawMessage `json:"payload"`
}

// Apply merges one stream line and returns the resulting state.
func (m *Merger) Apply(line []byte) (State, error) {
	var env envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return State{}, fmt.Errorf("media-control line: %w", err)
	}
	if env.Type != "data" {
		return State{}, fmt.Errorf("media-control line: unknown type %q", env.Type)
	}
	if !env.Diff || m.raw == nil {
		m.raw = map[string]json.RawMessage{}
		m.art = nil
	}
	for k, v := range env.Payload {
		isNull := bytes.Equal(bytes.TrimSpace(v), []byte("null"))
		if k == "artworkData" {
			// Decoded once here; keeping it out of raw avoids re-decoding on every diff.
			m.art = nil
			if !isNull {
				var b64 string
				if err := json.Unmarshal(v, &b64); err != nil {
					return State{}, fmt.Errorf("artworkData: %w", err)
				}
				art, err := base64.StdEncoding.DecodeString(b64)
				if err != nil {
					return State{}, fmt.Errorf("artworkData: %w", err)
				}
				m.art = art
			}
			continue
		}
		if isNull {
			delete(m.raw, k)
		} else {
			m.raw[k] = v
		}
	}
	return m.state()
}

func (m *Merger) state() (State, error) {
	var (
		title, artist, album, bundle string
		playing                      bool
		rate                         *float64
		durUS, elapsedUS, tsUS       float64
	)
	fields := map[string]any{
		"title": &title, "artist": &artist, "album": &album,
		"bundleIdentifier": &bundle, "playing": &playing, "playbackRate": &rate,
		"durationMicros": &durUS, "elapsedTimeMicros": &elapsedUS, "timestampEpochMicros": &tsUS,
	}
	for k, dst := range fields {
		if v, ok := m.raw[k]; ok {
			if err := json.Unmarshal(v, dst); err != nil {
				return State{}, fmt.Errorf("%s: %w", k, err)
			}
		}
	}
	if title == "" {
		return State{}, nil
	}
	// playbackRate lags behind `playing` on resume and is null while paused.
	r := 0.0
	if playing {
		r = 1
		if rate != nil {
			r = *rate
		}
	}
	return State{
		Title: title, Artist: artist, Album: album,
		Duration:  time.Duration(durUS) * time.Microsecond,
		Elapsed:   time.Duration(elapsedUS) * time.Microsecond,
		Timestamp: time.UnixMicro(int64(tsUS)),
		Rate:      r, Playing: playing, App: bundle, Art: m.art,
	}, nil
}
