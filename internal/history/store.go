package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// DefaultSize is how many entries the history keeps.
const DefaultSize = 50

// DefaultPath is $XDG_STATE_HOME/nowplay/history.json, falling back to
// ~/Library/Application Support/nowplay/history.json.
func DefaultPath() (string, error) {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "nowplay", "history.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Application Support", "nowplay", "history.json"), nil
}

// ErrCorrupt is returned by Load when the file could not be parsed. The file
// is moved aside to history.json.corrupt and Load returns an empty list.
var ErrCorrupt = errors.New("history file unreadable")

type file struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

// Store persists the list as JSON. Saves are tagged with a sequence number so
// that concurrent saves issued from tea.Cmds can never leave an older list on
// disk: a save older than one already written is dropped.
type Store struct {
	Path string
	Max  int

	mu   sync.Mutex
	last uint64
}

func NewStore(path string, max int) *Store {
	if max <= 0 {
		max = DefaultSize
	}
	return &Store{Path: path, Max: max}
}

// Load reads the list, newest first. A missing file is an empty list.
func (s *Store) Load() ([]Entry, error) {
	b, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		_ = os.Rename(s.Path, s.Path+".corrupt")
		return nil, fmt.Errorf("%w: moved to %s.corrupt: %v", ErrCorrupt, s.Path, err)
	}
	if len(f.Entries) > s.Max {
		f.Entries = f.Entries[:s.Max]
	}
	return f.Entries, nil
}

// Save atomically replaces the file (temp file + rename).
func (s *Store) Save(seq uint64, entries []Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if seq < s.last {
		return nil
	}
	s.last = seq
	if len(entries) > s.Max {
		entries = entries[:s.Max]
	}
	if entries == nil {
		entries = []Entry{} // "entries": [] rather than null
	}
	b, err := json.MarshalIndent(file{Version: 1, Entries: entries}, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".history-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.Path)
}
