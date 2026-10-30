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

const (
	appDir = "nowplaying"
	// legacyDir is what the app was called before it was renamed; data from
	// there is still picked up (never modified or deleted).
	legacyDir = "nowplay"
)

func pathIn(dir string) (string, error) {
	if state := os.Getenv("XDG_STATE_HOME"); state != "" {
		return filepath.Join(state, dir, "history.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Application Support", dir, "history.json"), nil
}

// DefaultPath is $XDG_STATE_HOME/nowplaying/history.json, falling back to
// ~/Library/Application Support/nowplaying/history.json.
func DefaultPath() (string, error) { return pathIn(appDir) }

// LegacyPath is where the history lived before the app was renamed from nowplay.
func LegacyPath() (string, error) { return pathIn(legacyDir) }

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
	// Legacy, if set, is read when Path does not exist yet. It is never written.
	Legacy string
	Max    int

	mu   sync.Mutex
	last uint64
}

func NewStore(path string, max int) *Store {
	if max <= 0 {
		max = DefaultSize
	}
	return &Store{Path: path, Max: max}
}

// Load reads the list, newest first. A missing file is an empty list, unless
// Legacy exists: then that is read, and the next Save writes Path.
func (s *Store) Load() ([]Entry, error) {
	entries, found, err := read(s.Path, true)
	if !found && err == nil && s.Legacy != "" {
		// An unreadable legacy file is ignored rather than moved aside: it is not ours to touch.
		if old, ok, lerr := read(s.Legacy, false); ok && lerr == nil {
			entries = old
		}
	}
	if len(entries) > s.Max {
		entries = entries[:s.Max]
	}
	return entries, err
}

// read parses path. moveAside moves an unparseable file to path+".corrupt".
func read(path string, moveAside bool) (entries []Entry, found bool, err error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		if moveAside {
			_ = os.Rename(path, path+".corrupt")
			return nil, true, fmt.Errorf("%w: moved to %s.corrupt: %v", ErrCorrupt, path, err)
		}
		return nil, true, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	return f.Entries, true, nil
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
