package art

import (
	"image"
	"image/color"
	"strings"
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

func TestRoundCorners(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 28, 28))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	roundCorners(img)
	for _, p := range [][2]int{{0, 0}, {27, 0}, {0, 27}, {27, 27}} {
		if img.RGBAAt(p[0], p[1]).A != 0 {
			t.Errorf("corner %v should be transparent", p)
		}
	}
	if img.RGBAAt(14, 0).A == 0 || img.RGBAAt(0, 14).A == 0 {
		t.Errorf("edge midpoints must stay opaque")
	}
	out := render(img, nil)
	if !strings.Contains(out, "▄") || !strings.Contains(out, "▀") {
		t.Errorf("rounded render should use both half blocks")
	}
}
