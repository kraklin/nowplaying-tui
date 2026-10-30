package art

import (
	"image/color"
	"testing"
)

func TestEnsureContrast(t *testing.T) {
	darkBg, white := color.RGBA{18, 18, 18, 255}, color.RGBA{255, 255, 255, 255}
	for _, c := range []color.RGBA{{0, 0, 0, 255}, {20, 10, 40, 255}, {40, 0, 90, 255}} {
		if got := contrast(ensureContrast(c, true), darkBg); got < 3 {
			t.Errorf("dark: %v only reaches %.2f:1", c, got)
		}
	}
	for _, c := range []color.RGBA{{255, 255, 255, 255}, {250, 240, 160, 255}} {
		if got := contrast(ensureContrast(c, false), white); got < 3 {
			t.Errorf("light: %v only reaches %.2f:1", c, got)
		}
	}
	ok := color.RGBA{201, 167, 255, 255}
	if ensureContrast(ok, true) != ok {
		t.Errorf("already-legible colour should be untouched")
	}
}
