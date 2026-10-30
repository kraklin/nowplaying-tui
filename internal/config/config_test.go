package config

import (
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "nowplay", "config.toml")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMissingFileGivesDefaults(t *testing.T) {
	cfg, warns := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if cfg != Default() || len(warns) != 0 {
		t.Errorf("got %+v %v", cfg, warns)
	}
}

func TestDefaultsMatchTheBrief(t *testing.T) {
	d := Default()
	if d.QuantizeColors != 12 || !d.Scanlines || d.Icons != "unicode" || d.HistorySize != 50 || d.MinPlaySeconds != 10 || d.Accent != "" {
		t.Errorf("defaults drifted from the brief: %+v", d)
	}
}

func TestAllIconSetsAreAccepted(t *testing.T) {
	for _, set := range []string{"unicode", "nerd", "basic"} {
		if cfg, warns := Load(write(t, "icons = \""+set+"\"\n")); cfg.Icons != set || len(warns) != 0 {
			t.Errorf("%s: %+v %v", set, cfg, warns)
		}
	}
}

func TestEveryKeyIsOptional(t *testing.T) {
	cfg, warns := Load(write(t, "scanlines = false\n"))
	want := Default()
	want.Scanlines = false
	if cfg != want || len(warns) != 0 {
		t.Errorf("only scanlines should change: %+v %v", cfg, warns)
	}
	if cfg, warns := Load(write(t, "")); cfg != Default() || len(warns) != 0 {
		t.Errorf("an empty file is fine: %+v %v", cfg, warns)
	}
}

func TestAllKeys(t *testing.T) {
	cfg, warns := Load(write(t, `
art_max_px       = 24
quantize_colors  = 0
scanlines        = false
accent           = "#c9a7ff"
icons            = "nerd"
history_size     = 100
min_play_seconds = 0
background       = "#3b3d4e"
`))
	want := Config{24, 0, false, "#c9a7ff", "nerd", 100, 0, "#3b3d4e"}
	if cfg != want || len(warns) != 0 {
		t.Errorf("got %+v %v", cfg, warns)
	}
}

func TestInvalidValuesFallBackIndividually(t *testing.T) {
	cfg, warns := Load(write(t, `
art_max_px = 1000
quantize_colors = -2
icons = "emoji"
history_size = 0
min_play_seconds = -5
accent = "purple"
background = "#12"
scanlines = false
`))
	want := Default()
	want.Scanlines = false // the one valid key survives
	if cfg != want {
		t.Errorf("bad keys should revert and good ones stay: %+v", cfg)
	}
	if len(warns) != 7 {
		t.Errorf("one warning per bad key, got %d: %v", len(warns), warns)
	}
	for _, k := range []string{"art_max_px", "quantize_colors", "icons", "history_size", "min_play_seconds", "accent", "background"} {
		found := false
		for _, w := range warns {
			found = found || strings.Contains(w, k)
		}
		if !found {
			t.Errorf("no warning names %s: %v", k, warns)
		}
	}
}

func TestUnknownKeysAreReported(t *testing.T) {
	cfg, warns := Load(write(t, "colour = \"red\"\nscanlines = false\n"))
	if len(warns) != 1 || !strings.Contains(warns[0], "colour") {
		t.Errorf("a typo should be called out: %v", warns)
	}
	if cfg.Scanlines {
		t.Errorf("the rest of the file still applies")
	}
}

func TestBrokenFileNeverStopsTheApp(t *testing.T) {
	for name, body := range map[string]string{
		"syntax error":   "this is = = not toml",
		"wrong type":     `art_max_px = "big"`,
		"unclosed":       "accent = \"#fff",
		"binary garbage": "\x00\x01\x02",
	} {
		cfg, warns := Load(write(t, body))
		if cfg != Default() {
			t.Errorf("%s: should fall back to defaults, got %+v", name, cfg)
		}
		if len(warns) != 1 || !strings.Contains(warns[0], "config ignored") {
			t.Errorf("%s: should explain why, got %v", name, warns)
		}
	}
}

func TestUnreadableFileIsReported(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores permissions")
	}
	p := write(t, "scanlines = false\n")
	os.Chmod(p, 0)
	defer os.Chmod(p, 0o644)
	if cfg, warns := Load(p); cfg != Default() || len(warns) != 1 {
		t.Errorf("unreadable: %+v %v", cfg, warns)
	}
}

func TestParseHex(t *testing.T) {
	for in, want := range map[string]color.RGBA{"#3b3d4e": {59, 61, 78, 255}, "3b3d4e": {59, 61, 78, 255}, "#FFFFFF": {255, 255, 255, 255}} {
		if got, err := ParseHex(in); err != nil || got != want {
			t.Errorf("ParseHex(%q) = %v, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "#fff", "#12345", "#gggggg", "blue", "#1234567", "#-12345"} {
		if _, err := ParseHex(bad); err == nil {
			t.Errorf("ParseHex(%q) should fail", bad)
		}
	}
}

func TestPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-config")
	if p, _ := Path(); p != "/tmp/xdg-config/nowplay/config.toml" {
		t.Errorf("XDG: %s", p)
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/Users/someone")
	if p, _ := Path(); p != "/Users/someone/.config/nowplay/config.toml" {
		t.Errorf("fallback: %s", p)
	}
}
