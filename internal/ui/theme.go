package ui

import (
	"image/color"

	"github.com/charmbracelet/lipgloss"

	"github.com/tomaslatall/nowplaying-tui/internal/art"
)

// theme holds every colour that depends on the terminal background, derived
// from the real background so dim tones stay legible on any theme.
type theme struct {
	bg     [3]uint8
	bright [3]uint8 // history titles
	soft   [3]uint8 // history secondary columns
	dim    lipgloss.Style
	dim2   lipgloss.Style
}

func newTheme(bg color.RGBA) theme {
	ink := [3]uint8{255, 255, 255}
	if art.Contrast(color.RGBA{0, 0, 0, 255}, bg) > art.Contrast(color.RGBA{255, 255, 255, 255}, bg) {
		ink = [3]uint8{0, 0, 0}
	}
	base := [3]uint8{bg.R, bg.G, bg.B}
	blend := func(f float64) [3]uint8 { return mix(base, ink, f) }
	return theme{
		bg:     base,
		bright: blend(0.85),
		soft:   blend(0.55),
		dim:    colorStyle(blend(0.55)),
		dim2:   colorStyle(blend(0.40)),
	}
}

// mix returns a + (b-a)*f per channel.
func mix(a, b [3]uint8, f float64) [3]uint8 {
	var c [3]uint8
	for k := range c {
		c[k] = uint8(float64(a[k]) + (float64(b[k])-float64(a[k]))*f + 0.5)
	}
	return c
}

func colorStyle(c [3]uint8) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(sprintf("#%02x%02x%02x", c[0], c[1], c[2])))
}
