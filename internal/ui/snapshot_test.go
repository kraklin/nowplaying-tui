package ui

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/tomaslatall/nowplaying-tui/internal/config"
	"github.com/tomaslatall/nowplaying-tui/internal/source"
)

// Not a real test: with NOWPLAYING_SNAPSHOT_DIR set it writes true-colour frames
// that scripts/ansi2svg.py turns into the README screenshots (see README).
func TestWriteSnapshots(t *testing.T) {
	dir := os.Getenv("NOWPLAYING_SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("set NOWPLAYING_SNAPSHOT_DIR to regenerate the README screenshots")
	}
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	bg := color.RGBA{30, 30, 46, 255}

	write := func(name string, m Model) {
		if err := os.WriteFile(filepath.Join(dir, name+".ansi"), []byte(m.View()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	withVolume := func(m Model) Model {
		tm, _ := m.Update(VolumeMsg{Level: 55})
		return tm.(Model)
	}
	write("wide", withVolume(settleBG(t, 90, 22, false, bg)))
	write("stacked", withVolume(settleBG(t, 56, 36, false, bg)))
	write("idle", withVolume(settleBG(t, 90, 22, true, bg)))
	write("text", withVolume(settleFull(t, 80, 17, false, bg, cfgWith(func(c *config.Config) { c.ShowCover = false }))))

	missing := settleBG(t, 90, 16, true, bg)
	tm, _ := missing.Update(StateMsg(source.State{Status: source.StatusMissing}))
	write("missing", tm.(Model))
}
