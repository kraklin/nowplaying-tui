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

type Osascript struct{ run Runner }

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

// Get returns the output volume, 0-100.
func (o *Osascript) Get() (int, error) {
	out, err := o.run(context.Background(), "output volume of (get volume settings)")
	if err != nil {
		return 0, err
	}
	s := strings.TrimSpace(string(out))
	if s == "missing value" {
		return 0, ErrUnavailable
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("volume: unexpected osascript output %q", s)
	}
	return max(0, min(v, 100)), nil
}

// Set sets the output volume, clamped to 0-100.
func (o *Osascript) Set(v int) error {
	_, err := o.run(context.Background(), fmt.Sprintf("set volume output volume %d", max(0, min(v, 100))))
	return err
}

// Fake is an in-memory volume for --demo and tests.
type Fake struct {
	mu    sync.Mutex
	Level int
}

func NewFake(level int) *Fake { return &Fake{Level: level} }

func (f *Fake) Get() (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Level, nil
}

func (f *Fake) Set(v int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Level = max(0, min(v, 100))
	return nil
}
