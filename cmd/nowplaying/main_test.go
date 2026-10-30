package main

import (
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackgroundPrecedence(t *testing.T) {
	flagged, err := background("#3b3d4e", "#000000")
	if err != nil || flagged != (color.RGBA{59, 61, 78, 255}) {
		t.Errorf("the flag beats the config: %v %v", flagged, err)
	}
	cfg, err := background("", "#102030")
	if err != nil || cfg != (color.RGBA{16, 32, 48, 255}) {
		t.Errorf("config beats asking the terminal: %v %v", cfg, err)
	}
}

func TestBadBackgroundFlagIsReported(t *testing.T) {
	_, err := background("nope", "")
	if err == nil || !strings.Contains(err.Error(), "--background") {
		t.Errorf("want an error that names the flag, got %v", err)
	}
}

func TestLoadConfigDefaultsAndExplicitPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if cfg, warns := loadConfig(""); cfg.Icons != "unicode" || len(warns) != 0 {
		t.Errorf("no file at the default path is fine: %+v %v", cfg, warns)
	}
	if _, warns := loadConfig(filepath.Join(dir, "missing.toml")); len(warns) != 1 {
		t.Errorf("a file named with --config must exist: %v", warns)
	}
	p := filepath.Join(dir, "mine.toml")
	os.WriteFile(p, []byte("icons = \"nerd\"\n"), 0o644)
	if cfg, warns := loadConfig(p); cfg.Icons != "nerd" || len(warns) != 0 {
		t.Errorf("explicit file: %+v %v", cfg, warns)
	}
	os.MkdirAll(filepath.Join(dir, "nowplaying"), 0o755)
	os.WriteFile(filepath.Join(dir, "nowplaying", "config.toml"), []byte("scanlines = false\n"), 0o644)
	if cfg, _ := loadConfig(""); cfg.Scanlines {
		t.Errorf("the default location should be read")
	}
}

func TestLoadConfigStillReadsTheOldFolder(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	os.MkdirAll(filepath.Join(dir, "nowplay"), 0o755)
	os.WriteFile(filepath.Join(dir, "nowplay", "config.toml"), []byte("icons = \"basic\"\n"), 0o644)
	if cfg, warns := loadConfig(""); cfg.Icons != "basic" || len(warns) != 0 {
		t.Errorf("a config from before the rename should still apply: %+v %v", cfg, warns)
	}
}
