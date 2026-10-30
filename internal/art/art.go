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
func Build(data []byte, px int) (Result, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Result{}, err
	}
	small := resize(cropSquare(src), px)
	return Result{Rendered: render(small, nil), Accent: accent(small)}, nil
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
func accent(img *image.RGBA) string {
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
	return fmt.Sprintf("#%02x%02x%02x", best.R, best.G, best.B)
}
