// Package volume reads and sets the macOS output volume through osascript.
//
// osascript takes 50-100 ms per call, so callers must never use it on the UI
// goroutine.
package volume

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrUnavailable means the volume cannot be read or set: osascript is
// missing, or the output device has no software volume (HDMI, some DACs).
var ErrUnavailable = errors.New("system volume unavailable")

// Runner executes one AppleScript line and returns its output.
type Runner func(ctx context.Context, script string) ([]byte, error)

// Osascript reads and sets the macOS output volume and mute switch with osascript.
type Osascript struct{ run Runner }

// New returns an Osascript backed by the real osascript command.
func New() *Osascript { return &Osascript{run: osascript} }

// Available reports whether osascript is on PATH.
func Available() bool {
	_, err := exec.LookPath("osascript")
	return err == nil
}

func osascript(ctx context.Context, script string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "osascript", "-e", script).Output()
	if errors.Is(err, exec.ErrNotFound) {
		return nil, ErrUnavailable
	}
	return out, err
}

// Read returns the output volume (0-100) and whether the output is muted, in a
// single osascript call. Mute is a separate switch from the level: muting leaves
// the level alone.
func (o *Osascript) Read() (level int, muted bool, err error) {
	out, err := o.run(context.Background(),
		`(output volume of (get volume settings) as text) & "," & (output muted of (get volume settings) as text)`)
	if err != nil {
		return 0, false, err
	}
	s := strings.TrimSpace(string(out))
	lvl, mute, _ := strings.Cut(s, ",")
	if lvl == "missing value" {
		return 0, false, ErrUnavailable
	}
	v, err := strconv.Atoi(strings.TrimSpace(lvl))
	if err != nil {
		return 0, false, fmt.Errorf("volume: unexpected osascript output %q", s)
	}
	// A device with a level but no mute switch reports "missing value": not muted.
	return max(0, min(v, 100)), strings.TrimSpace(mute) == "true", nil
}

// Get returns the output volume, 0-100.
func (o *Osascript) Get() (int, error) {
	level, _, err := o.Read()
	return level, err
}

// Set sets the output volume, clamped to 0-100.
func (o *Osascript) Set(v int) error {
	_, err := o.run(context.Background(), fmt.Sprintf("set volume output volume %d", max(0, min(v, 100))))
	return err
}

// SetMuted mutes or unmutes the output without changing its level.
func (o *Osascript) SetMuted(muted bool) error {
	_, err := o.run(context.Background(), fmt.Sprintf("set volume output muted %t", muted))
	return err
}

// Fake is an in-memory volume for --demo and tests.
type Fake struct {
	mu    sync.Mutex
	Level int
	Muted bool
}

// NewFake returns a Fake at the given level.
func NewFake(level int) *Fake { return &Fake{Level: level} }

// Get returns the level.
func (f *Fake) Get() (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Level, nil
}

// Read returns the level and the mute state.
func (f *Fake) Read() (int, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Level, f.Muted, nil
}

// Set sets the level.
func (f *Fake) Set(v int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Level = max(0, min(v, 100))
	return nil
}

// SetMuted sets the mute state.
func (f *Fake) SetMuted(m bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Muted = m
	return nil
}
