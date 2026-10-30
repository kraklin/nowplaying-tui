package main

import (
	"image/color"
	"testing"
)

func TestParseHex(t *testing.T) {
	for in, want := range map[string]color.RGBA{
		"#3b3d4e": {59, 61, 78, 255}, "3b3d4e": {59, 61, 78, 255}, "#000000": {0, 0, 0, 255}, "#FFFFFF": {255, 255, 255, 255},
	} {
		if got, err := parseHex(in); err != nil || got != want {
			t.Errorf("parseHex(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "#fff", "#12345", "#gggggg", "blue", "#1234567"} {
		if _, err := parseHex(bad); err == nil {
			t.Errorf("parseHex(%q) should fail", bad)
		}
	}
}

func TestBackgroundFlagWins(t *testing.T) {
	got, err := background("#3b3d4e")
	if err != nil || got != (color.RGBA{59, 61, 78, 255}) {
		t.Errorf("flag should skip the terminal query: %v, %v", got, err)
	}
}
