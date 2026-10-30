// Package art turns cover bytes into half-block pixel art.
package art

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"strings"

	xdraw "golang.org/x/image/draw"
)

// Result is a rendered cover plus its accent colour.
type Result struct {
	Rendered string
	Accent   string // "#rrggbb"
}

const (
	placeholderAccent = "#c9a7ff"
	accentSample      = 32 // px the cover is downscaled to for picking the accent
)

// Hash is a short, stable identifier of an image, used as a cache key.
func Hash(data []byte) string {
	sum := sha1.Sum(data)
	return hex.EncodeToString(sum[:8])
}

// Options controls the pixel-art look.
type Options struct {
	Quantize  int        // k-means palette size, 0 = off
	Scanlines bool       // darken every other pixel row
	BG        color.RGBA // terminal background the accent must contrast with
	Accent    string     // "#rrggbb" tint of the empty cover; "" = the default
}

// DarkBG is the background assumed when the terminal cannot tell us.
var DarkBG = color.RGBA{18, 18, 18, 255}

// DefaultOptions are the stylised defaults for a terminal with background bg.
func DefaultOptions(bg color.RGBA) Options { return Options{Quantize: 12, Scanlines: true, BG: bg} }

// Build decodes data, centre-crops, downscales to px×px, restyles and renders it.
func Build(data []byte, px int, o Options) (Result, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Result{}, err
	}
	square := cropSquare(src)
	// The accent is sampled at a fixed size so it depends on the artwork, not on how
	// big the cover happens to be drawn (or whether it is drawn at all).
	ac := hexColor(ensureContrast(pickAccent(resize(square, accentSample)), o.BG))
	small := resize(square, px)
	quantize(small, o.Quantize)
	if o.Scanlines {
		scanlines(small)
	}
	roundCorners(small)
	return Result{Rendered: render(small, nil), Accent: ac}, nil
}

// Placeholder is a gradient from the terminal background towards the accent,
// with a ♫ in the middle. An empty or invalid accent uses the default one.
func Placeholder(px int, bg color.RGBA, accent string) Result {
	ac := color.RGBA{0xc9, 0xa7, 0xff, 255}
	if _, err := fmt.Sscanf(accent, "#%02x%02x%02x", &ac.R, &ac.G, &ac.B); err != nil {
		ac = color.RGBA{0xc9, 0xa7, 0xff, 255}
	}
	img := image.NewRGBA(image.Rect(0, 0, px, px))
	for y := 0; y < px; y++ {
		for x := 0; x < px; x++ {
			// brighter towards the bottom right, like a glow under the horizon
			t := 0.10 + 0.30*float64(x+2*y)/float64(3*px)
			img.Set(x, y, color.RGBA{
				mixU8(bg.R, ac.R, t), mixU8(bg.G, ac.G, t), mixU8(bg.B, ac.B, t), 255})
		}
	}
	roundCorners(img)
	mid := px / 4
	glyph := func(col, row int) (rune, bool) { return '♫', col == px/2 && row == mid }
	return Result{Rendered: render(img, glyph), Accent: placeholderAccent}
}

func mixU8(a, b uint8, t float64) uint8 {
	return uint8(float64(a) + (float64(b)-float64(a))*t + 0.5)
}

func cropSquare(src image.Image) image.Image {
	b := src.Bounds()
	side := min(b.Dx(), b.Dy())
	x0 := b.Min.X + (b.Dx()-side)/2
	y0 := b.Min.Y + (b.Dy()-side)/2
	return subImage(src, image.Rect(x0, y0, x0+side, y0+side))
}

func subImage(src image.Image, r image.Rectangle) image.Image {
	if s, ok := src.(interface {
		SubImage(image.Rectangle) image.Image
	}); ok {
		return s.SubImage(r)
	}
	dst := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(dst, dst.Bounds(), src, r.Min, draw.Src)
	return dst
}

func resize(src image.Image, px int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, px, px))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Src, nil)
	return dst
}

// render draws two pixel rows per terminal row with ▀ (fg = top, bg = bottom).
func render(img *image.RGBA, glyph func(col, row int) (rune, bool)) string {
	b := img.Bounds()
	var sb strings.Builder
	for y := 0; y < b.Dy(); y += 2 {
		var fg, bg color.RGBA
		have := false
		for x := 0; x < b.Dx(); x++ {
			top := img.RGBAAt(b.Min.X+x, b.Min.Y+y)
			bot := top
			if y+1 < b.Dy() {
				bot = img.RGBAAt(b.Min.X+x, b.Min.Y+y+1)
			}
			if top.A == 0 || bot.A == 0 {
				sb.WriteString("\x1b[0m")
				have = false
				switch {
				case top.A == 0 && bot.A == 0:
					sb.WriteString(" ")
				case top.A == 0:
					fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm▄", bot.R, bot.G, bot.B)
				default:
					fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm▀", top.R, top.G, top.B)
				}
				sb.WriteString("\x1b[0m")
				continue
			}
			if !have || top != fg {
				fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm", top.R, top.G, top.B)
				fg = top
			}
			if !have || bot != bg {
				fmt.Fprintf(&sb, "\x1b[48;2;%d;%d;%dm", bot.R, bot.G, bot.B)
				bg = bot
			}
			have = true
			if r, ok := overlay(glyph, x, y/2); ok {
				fmt.Fprintf(&sb, "\x1b[38;2;150;150;165;48;2;%d;%d;%dm%c", top.R, top.G, top.B, r)
				have = false
				continue
			}
			sb.WriteString("▀")
		}
		sb.WriteString("\x1b[0m")
		if y+2 < b.Dy() {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

func overlay(g func(col, row int) (rune, bool), col, row int) (rune, bool) {
	if g == nil {
		return 0, false
	}
	return g(col, row)
}

func hexColor(c color.RGBA) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

// ensureContrast moves c's HSL lightness until it reaches 3:1 against bg,
// heading towards whichever of white and black contrasts more with bg.
func ensureContrast(c, bg color.RGBA) color.RGBA {
	white, black := color.RGBA{255, 255, 255, 255}, color.RGBA{0, 0, 0, 255}
	dir := 0.02
	if contrast(black, bg) > contrast(white, bg) {
		dir = -0.02
	}
	h, s, l := toHSL(c)
	for i := 0; i < 50 && contrast(c, bg) < 3; i++ {
		l = math.Max(0, math.Min(1, l+dir))
		c = fromHSL(h, s, l)
	}
	return c
}

// Contrast is the WCAG contrast ratio of a and b.
func Contrast(a, b color.RGBA) float64 { return contrast(a, b) }

func luminance(c color.RGBA) float64 {
	lin := func(v uint8) float64 {
		f := float64(v) / 255
		if f <= 0.03928 {
			return f / 12.92
		}
		return math.Pow((f+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

func contrast(a, b color.RGBA) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func toHSL(c color.RGBA) (h, s, l float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	mx, mn := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	l = (mx + mn) / 2
	d := mx - mn
	if d == 0 {
		return 0, 0, l
	}
	s = d / (1 - math.Abs(2*l-1))
	switch mx {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return math.Mod(h*60+360, 360), s, l
}

func fromHSL(h, s, l float64) color.RGBA {
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	var r, g, b float64
	switch int(h / 60) {
	case 0:
		r, g, b = c, x, 0
	case 1:
		r, g, b = x, c, 0
	case 2:
		r, g, b = 0, c, x
	case 3:
		r, g, b = 0, x, c
	case 4:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	m := l - c/2
	return color.RGBA{uint8((r + m) * 255), uint8((g + m) * 255), uint8((b + m) * 255), 255}
}

// roundCorners makes the corner pixels transparent, so render lets the
// terminal background show through them.
func roundCorners(img *image.RGBA) {
	n := img.Bounds().Dx()
	r := max(2, n/10)
	for y := 0; y < r; y++ {
		for x := 0; x < r; x++ {
			dx, dy := float64(r-x)-0.5, float64(r-y)-0.5
			if math.Hypot(dx, dy) <= float64(r) {
				continue
			}
			for _, p := range [][2]int{{x, y}, {n - 1 - x, y}, {x, n - 1 - y}, {n - 1 - x, n - 1 - y}} {
				img.SetRGBA(p[0], p[1], color.RGBA{})
			}
		}
	}
}
