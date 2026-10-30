// Package config loads ~/.config/nowplaying/config.toml. Every key is optional,
// and a bad file never stops the app: invalid values fall back to their
// defaults and Load reports what it ignored.
package config

import (
	"errors"
	"fmt"
	"image/color"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/BurntSushi/toml"
)

// Config is the user configuration. Every field has a default; see Default.
type Config struct {
	// ShowCover false hides the cover and shows only the text.
	ShowCover      bool   `toml:"show_cover"`
	ArtMaxPx       int    `toml:"art_max_px"`
	QuantizeColors int    `toml:"quantize_colors"`
	Scanlines      bool   `toml:"scanlines"`
	Accent         string `toml:"accent"`
	Icons          string `toml:"icons"`
	HistorySize    int    `toml:"history_size"`
	// PersistHistory keeps the recently played list on disk between runs. Off by
	// default: the list then lives only as long as the session.
	PersistHistory bool `toml:"persist_history"`
	// ShowSource false hides the name of the player (bottom right corner).
	ShowSource bool `toml:"show_source"`
	// ShowHistory false hides the recently played list at start (r toggles it).
	ShowHistory bool `toml:"show_history"`
	// HistoryFade false draws every history row at full strength.
	HistoryFade    bool `toml:"history_fade"`
	MinPlaySeconds int  `toml:"min_play_seconds"`
	// Background is the terminal background as "#rrggbb". When set, nowplaying
	// does not ask the terminal for it.
	Background string `toml:"background"`
}

// Default returns the configuration used when no file sets a value.
func Default() Config {
	return Config{
		ShowCover:      true,
		ShowSource:     true,
		ShowHistory:    true,
		HistoryFade:    true,
		ArtMaxPx:       32,
		QuantizeColors: 12,
		Scanlines:      true,
		Icons:          "unicode",
		HistorySize:    10,
		MinPlaySeconds: 10,
	}
}

// Path is $XDG_CONFIG_HOME/nowplaying/config.toml, or ~/.config/nowplaying/config.toml.
func Path() (string, error) {
	if cfg := os.Getenv("XDG_CONFIG_HOME"); cfg != "" {
		return filepath.Join(cfg, "nowplaying", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "nowplaying", "config.toml"), nil
}

// Load reads path. A missing file gives the defaults and no warnings. Anything
// wrong in the file is reported in warnings and replaced by its default.
func Load(path string) (cfg Config, warnings []string) {
	cfg = Default()
	md, err := toml.DecodeFile(path, &cfg)
	if errors.Is(err, fs.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Default(), []string{"config ignored: " + err.Error()}
	}
	for _, k := range md.Undecoded() {
		warnings = append(warnings, fmt.Sprintf("config: unknown key %q", k.String()))
	}
	return cfg.sanitize(warnings)
}

func (c Config) sanitize(warnings []string) (Config, []string) {
	d := Default()
	bad := func(key string, why string) { warnings = append(warnings, fmt.Sprintf("config: %s %s", key, why)) }
	if c.ArtMaxPx < 8 || c.ArtMaxPx > 128 {
		bad("art_max_px", "must be 8-128")
		c.ArtMaxPx = d.ArtMaxPx
	}
	if c.QuantizeColors < 0 || c.QuantizeColors > 64 {
		bad("quantize_colors", "must be 0-64")
		c.QuantizeColors = d.QuantizeColors
	}
	if c.Icons != "unicode" && c.Icons != "nerd" && c.Icons != "basic" {
		bad("icons", `must be "unicode", "nerd" or "basic"`)
		c.Icons = d.Icons
	}
	if c.HistorySize < 1 || c.HistorySize > 500 {
		bad("history_size", "must be 1-500")
		c.HistorySize = d.HistorySize
	}
	if c.MinPlaySeconds < 0 {
		bad("min_play_seconds", "must be 0 or more")
		c.MinPlaySeconds = d.MinPlaySeconds
	}
	if c.Accent != "" {
		if _, err := ParseHex(c.Accent); err != nil {
			bad("accent", `must be "#rrggbb"`)
			c.Accent = ""
		}
	}
	if c.Background != "" {
		if _, err := ParseHex(c.Background); err != nil {
			bad("background", `must be "#rrggbb"`)
			c.Background = ""
		}
	}
	return c, warnings
}

// ParseHex parses "#rrggbb" (the # is optional).
func ParseHex(s string) (color.RGBA, error) {
	h := s
	if len(h) > 0 && h[0] == '#' {
		h = h[1:]
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil || len(h) != 6 {
		return color.RGBA{}, fmt.Errorf("%q: want #rrggbb", s)
	}
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}, nil
}
