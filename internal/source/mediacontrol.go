package source

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ErrNotInstalled is returned when the media-control binary is not on PATH.
var ErrNotInstalled = errors.New("media-control not found")

// MediaControl is a Source backed by the media-control CLI.
type MediaControl struct {
	Bin        string          // default "media-control"
	StreamArgs []string        // default {"stream", "--micros"}
	Backoff    []time.Duration // restart delays; the last one repeats
}

func NewMediaControl() *MediaControl { return &MediaControl{} }

func (m *MediaControl) bin() string {
	if m.Bin != "" {
		return m.Bin
	}
	return "media-control"
}

func (m *MediaControl) streamArgs() []string {
	if m.StreamArgs != nil {
		return m.StreamArgs
	}
	return []string{"stream", "--micros"}
}

func (m *MediaControl) backoff(attempt int) time.Duration {
	b := m.Backoff
	if b == nil {
		b = []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second}
	}
	return b[min(attempt, len(b)-1)]
}

// Installed reports whether the binary can be found.
func (m *MediaControl) Installed() bool {
	_, err := exec.LookPath(m.bin())
	return err == nil
}

func (m *MediaControl) Run(ctx context.Context, out chan<- State) {
	send := func(s State) bool {
		select {
		case out <- s:
			return true
		case <-ctx.Done():
			return false
		}
	}
	if !m.Installed() {
		send(State{Status: StatusMissing})
		return
	}
	var last State
	for attempt := 0; ; {
		got := m.runOnce(ctx, &last, send)
		if ctx.Err() != nil {
			return
		}
		if got {
			attempt = 0
		}
		reconnecting := last
		reconnecting.Status = StatusReconnecting
		if !send(reconnecting) {
			return
		}
		select {
		case <-time.After(m.backoff(attempt)):
			attempt++
		case <-ctx.Done():
			return
		}
	}
}

// runOnce runs one stream process to exit and reports whether it produced any state.
func (m *MediaControl) runOnce(ctx context.Context, last *State, send func(State) bool) (got bool) {
	cmd := exec.CommandContext(ctx, m.bin(), m.streamArgs()...)
	stdout, err := cmd.StdoutPipe()
	if err != nil || cmd.Start() != nil {
		return false
	}
	defer cmd.Wait()

	var merger Merger
	r := bufio.NewReaderSize(stdout, 1<<20)
	for {
		line, err := r.ReadBytes('\n')
		if len(strings.TrimSpace(string(line))) > 0 {
			if st, perr := merger.Apply(line); perr == nil {
				got = true
				*last = st
				if !send(st) {
					return got
				}
			}
		}
		if err != nil {
			if err != io.EOF {
				return got
			}
			return got
		}
	}
}

func (m *MediaControl) Control(c Cmd) error {
	var args []string
	switch c.Kind {
	case TogglePlay:
		args = []string{"toggle-play-pause"}
	case Next:
		args = []string{"next-track"}
	case Previous:
		args = []string{"previous-track"}
	case Seek:
		args = []string{"seek", strconv.FormatFloat(c.Seconds, 'f', 3, 64)}
	default:
		return fmt.Errorf("unknown command %d", c.Kind)
	}
	if out, err := exec.Command(m.bin(), args...).CombinedOutput(); err != nil {
		return fmt.Errorf("media-control %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return nil
}
