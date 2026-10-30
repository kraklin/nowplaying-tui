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

const placeholderAccent = "#c9a7ff"

func Hash(data []byte) string {
	sum := sha1.Sum(data)
	return hex.EncodeToString(sum[:8])
}

// Build decodes data, centre-crops, downscales to px×px and renders it.
func Build(data []byte, px int, dark bool) (Result, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Result{}, err
	}
	small := resize(cropSquare(src), px)
	ac := hexColor(ensureContrast(accent(small), dark))
	roundCorners(small)
	return Result{Rendered: render(small, nil), Accent: ac}, nil
}

// Placeholder is a dim gradient with a ♫ in the middle.
func Placeholder(px int) Result {
	img := image.NewRGBA(image.Rect(0, 0, px, px))
	for y := 0; y < px; y++ {
		for x := 0; x < px; x++ {
			t := float64(x+y) / float64(2*px)
			v := uint8(34 + 36*t)
			img.Set(x, y, color.RGBA{v, v, v + 8, 255})
		}
	}
	roundCorners(img)
	mid := px / 4
	glyph := func(col, row int) (rune, bool) { return '♫', col == px/2 && row == mid }
	return Result{Rendered: render(img, glyph), Accent: placeholderAccent}
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

// accent is a stand-in until milestone 3: the most saturated, mid-lightness pixel.
func accent(img *image.RGBA) color.RGBA {
	best, bestScore := color.RGBA{201, 167, 255, 255}, -1.0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.RGBAAt(x, y)
			mx := float64(max(c.R, c.G, c.B)) / 255
			mn := float64(min(c.R, c.G, c.B)) / 255
			score := (mx - mn) * (1 - math.Abs(mx+mn-1))
			if score > bestScore {
				best, bestScore = c, score
			}
		}
	}
	return best
}

func hexColor(c color.RGBA) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

// ensureContrast moves c's HSL lightness until it reaches 3:1 against an
// assumed terminal background (near-black when dark, white otherwise).
func ensureContrast(c color.RGBA, dark bool) color.RGBA {
	bg, dir := color.RGBA{18, 18, 18, 255}, 0.02
	if !dark {
		bg, dir = color.RGBA{255, 255, 255, 255}, -0.02
	}
	h, s, l := toHSL(c)
	for i := 0; i < 50 && contrast(c, bg) < 3; i++ {
		l = math.Max(0, math.Min(1, l+dir))
		c = fromHSL(h, s, l)
	}
	return c
}

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
