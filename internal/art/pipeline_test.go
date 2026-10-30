package art

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomaslatall/nowplaying-tui/internal/source"
)

var update = flag.Bool("update", false, "rewrite golden files")

func fill(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	fill(img, img.Bounds(), c)
	return img
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func hueOf(c color.RGBA) float64 { h, _, _ := toHSL(c); return h }

func distinct(img *image.RGBA) int {
	seen := map[color.RGBA]bool{}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			seen[img.RGBAAt(x, y)] = true
		}
	}
	return len(seen)
}

func TestKmeansSeparatesColours(t *testing.T) {
	pts := []rgb{}
	for i := 0; i < 30; i++ {
		pts = append(pts, rgb{250, 10, 10}, rgb{10, 10, 250})
	}
	cent, assign := kmeans(pts, 2, 1)
	if len(cent) != 2 {
		t.Fatalf("got %d centroids", len(cent))
	}
	for i := 0; i < len(pts); i += 2 {
		if assign[i] != assign[0] || assign[i+1] != assign[1] {
			t.Fatalf("same-colour points split across clusters")
		}
	}
	if assign[0] == assign[1] {
		t.Errorf("red and blue must land in different clusters")
	}
}

func TestKmeansEdgeCases(t *testing.T) {
	if c, a := kmeans(nil, 5, 1); c != nil || a != nil {
		t.Errorf("empty input should give nothing")
	}
	cent, _ := kmeans([]rgb{{5, 5, 5}, {5, 5, 5}, {5, 5, 5}}, 5, 1)
	if len(cent) != 1 {
		t.Errorf("one distinct colour should give one centroid, got %d", len(cent))
	}
	pts := pixels(solid(8, 8, color.RGBA{1, 2, 3, 255}))
	for i := range pts[:20] {
		pts[i] = rgb{200, 100, 50}
	}
	a1, b1 := kmeans(pts, 5, 7)
	a2, b2 := kmeans(pts, 5, 7)
	if len(a1) != len(a2) || len(b1) != len(b2) {
		t.Fatalf("not deterministic")
	}
	for i := range a1 {
		if a1[i] != a2[i] {
			t.Errorf("centroid %d differs between runs", i)
		}
	}
}

func TestQuantizeLimitsPalette(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(x * 8), uint8(y * 8), uint8((x + y) * 4), 255})
		}
	}
	before := distinct(img)
	quantize(img, 0)
	if distinct(img) != before {
		t.Errorf("k=0 must be a no-op")
	}
	quantize(img, 12)
	if n := distinct(img); n > 12 || n < 6 {
		t.Errorf("quantize(12) left %d colours", n)
	}
}

func TestScanlines(t *testing.T) {
	img := solid(4, 6, color.RGBA{200, 100, 50, 255})
	scanlines(img)
	for y := 0; y < 6; y++ {
		got := img.RGBAAt(1, y)
		want := color.RGBA{200, 100, 50, 255}
		if y%2 == 1 {
			want = color.RGBA{176, 88, 44, 255}
		}
		if got != want {
			t.Errorf("row %d = %v, want %v", y, got, want)
		}
	}
}

func TestAccentPrefersSaturatedOverGrey(t *testing.T) {
	img := solid(32, 32, color.RGBA{128, 128, 128, 255})
	fill(img, image.Rect(0, 0, 32, 10), color.RGBA{230, 30, 30, 255})
	c := pickAccent(img)
	if h := hueOf(c); h > 10 && h < 350 {
		t.Errorf("expected a red accent, got %v (hue %.0f)", c, h)
	}
}

func TestAccentSaturationTimesSize(t *testing.T) {
	img := solid(32, 32, color.RGBA{20, 120, 220, 255}) // big, saturated blue
	fill(img, image.Rect(0, 0, 4, 4), color.RGBA{255, 0, 0, 255})
	if h := hueOf(pickAccent(img)); h < 190 || h > 230 {
		t.Errorf("large saturated area should beat a tiny one, hue %.0f", h)
	}
}

func TestAccentSkipsBlackAndWhite(t *testing.T) {
	img := solid(32, 32, color.RGBA{4, 4, 4, 255})
	fill(img, image.Rect(0, 0, 32, 8), color.RGBA{250, 250, 250, 255})
	fill(img, image.Rect(0, 8, 32, 12), color.RGBA{40, 200, 90, 255})
	if h := hueOf(pickAccent(img)); h < 120 || h > 160 {
		t.Errorf("black and white should be skipped, got hue %.0f", h)
	}
}

func TestAccentFallbackAndContrast(t *testing.T) {
	bg := color.RGBA{18, 18, 18, 255}
	for name, img := range map[string]*image.RGBA{
		"all black": solid(16, 16, color.RGBA{0, 0, 0, 255}),
		"all white": solid(16, 16, color.RGBA{255, 255, 255, 255}),
		"dark navy": solid(16, 16, color.RGBA{10, 10, 40, 255}),
	} {
		res, err := Build(encodePNG(t, img), 16, DefaultOptions(DarkBG))
		if err != nil {
			t.Fatal(err)
		}
		var r, g, b int
		if _, err := sscanf(res.Accent, &r, &g, &b); err != nil {
			t.Fatalf("%s: bad accent %q", name, res.Accent)
		}
		if got := contrast(color.RGBA{uint8(r), uint8(g), uint8(b), 255}, bg); got < 3 {
			t.Errorf("%s: accent %s only %.2f:1 on a dark terminal", name, res.Accent, got)
		}
	}
}

func sscanf(s string, r, g, b *int) (int, error) {
	return fmt.Sscanf(strings.TrimPrefix(s, "#"), "%02x%02x%02x", r, g, b)
}

func TestCentreCropIsSquareFromTheMiddle(t *testing.T) {
	wide := image.NewRGBA(image.Rect(0, 0, 300, 100))
	fill(wide, image.Rect(0, 0, 100, 100), color.RGBA{255, 0, 0, 255})
	fill(wide, image.Rect(100, 0, 200, 100), color.RGBA{0, 255, 0, 255})
	fill(wide, image.Rect(200, 0, 300, 100), color.RGBA{0, 0, 255, 255})
	sq := cropSquare(wide).Bounds()
	if sq.Dx() != sq.Dy() {
		t.Fatalf("crop is %v, not square", sq)
	}
	res, err := Build(encodePNG(t, wide), 16, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Rendered, "38;2;255;0;0") || strings.Contains(res.Rendered, "38;2;0;0;255") {
		t.Errorf("edges of a wide cover leaked into the crop")
	}
	if rows := strings.Count(res.Rendered, "\n") + 1; rows != 8 {
		t.Errorf("16px cover should be 8 rows, got %d", rows)
	}
}

func TestBuildRejectsGarbage(t *testing.T) {
	if _, err := Build([]byte("not an image"), 16, DefaultOptions(DarkBG)); err == nil {
		t.Errorf("garbage must error so the UI can fall back to the placeholder")
	}
}

// sample covers are generated in code so the goldens stay reproducible.
func sampleCovers() map[string]*image.RGBA {
	sunset := image.NewRGBA(image.Rect(0, 0, 96, 96))
	for y := 0; y < 96; y++ {
		for x := 0; x < 96; x++ {
			t := float64(y) / 96
			c := color.RGBA{uint8(250 - 140*t), uint8(120 - 90*t), uint8(60 + 150*t), 255}
			if math.Hypot(float64(x-48), float64(y-40)) < 20 {
				c = color.RGBA{255, 230, 120, 255}
			}
			sunset.SetRGBA(x, y, c)
		}
	}
	stripes := image.NewRGBA(image.Rect(0, 0, 120, 80))
	for x := 0; x < 120; x++ {
		c := color.RGBA{220, 50, 70, 255}
		if (x/10)%2 == 1 {
			c = color.RGBA{30, 180, 190, 255}
		}
		fill(stripes, image.Rect(x, 0, x+1, 80), c)
	}
	fill(stripes, image.Rect(45, 25, 75, 55), color.RGBA{10, 10, 12, 255})
	dark := solid(96, 96, color.RGBA{8, 8, 14, 255})
	for y := 0; y < 96; y++ {
		for x := 0; x < 96; x++ {
			if math.Hypot(float64(x-70), float64(y-30)) < 9 {
				dark.SetRGBA(x, y, color.RGBA{40, 220, 230, 255})
			}
		}
	}
	return map[string]*image.RGBA{"sunset": sunset, "stripes-wide": stripes, "dark-with-spark": dark}
}

func TestGoldenRenders(t *testing.T) {
	for name, img := range sampleCovers() {
		for _, px := range []int{16, 28} {
			res, err := Build(encodePNG(t, img), px, DefaultOptions(DarkBG))
			if err != nil {
				t.Fatal(err)
			}
			got := res.Rendered + "\naccent " + res.Accent + "\n"
			path := filepath.Join("..", "..", "testdata", "golden", name+"-"+fmt.Sprint(px)+".ansi")
			if *update {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				continue
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run go test ./internal/art -update)", err)
			}
			if got != string(want) {
				t.Errorf("%s at %dpx differs from golden (run with -update if intended)", name, px)
			}
		}
	}
}

func TestRealCovers(t *testing.T) {
	f, err := os.ReadFile("../../testdata/stream.jsonl")
	if err != nil {
		t.Skip("no recording")
	}
	var m source.Merger
	var covers [][]byte
	for _, line := range bytes.Split(f, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		st, err := m.Apply(line)
		if err != nil {
			t.Fatal(err)
		}
		if len(st.Art) > 0 && (len(covers) == 0 || !bytes.Equal(covers[len(covers)-1], st.Art)) {
			covers = append(covers, st.Art)
		}
	}
	if len(covers) < 2 {
		t.Fatalf("recording should hold two covers, found %d", len(covers))
	}
	for name, bg := range map[string]color.RGBA{"dark": DarkBG, "light": {255, 255, 255, 255}, "slate": {59, 61, 78, 255}} {
		for i, c := range covers {
			res, err := Build(c, 28, DefaultOptions(bg))
			if err != nil {
				t.Fatalf("cover %d: %v", i, err)
			}
			if rows := strings.Count(res.Rendered, "\n") + 1; rows != 14 {
				t.Errorf("cover %d: %d rows", i, rows)
			}
			var r, g, b int
			if _, err := sscanf(res.Accent, &r, &g, &b); err != nil {
				t.Fatal(err)
			}
			if got := contrast(color.RGBA{uint8(r), uint8(g), uint8(b), 255}, bg); got < 3 {
				t.Errorf("cover %d on %s: accent %s only %.2f:1", i, name, res.Accent, got)
			}
		}
	}
	a, _ := Build(covers[0], 28, DefaultOptions(DarkBG))
	b, _ := Build(covers[1], 28, DefaultOptions(DarkBG))
	if a.Rendered == b.Rendered {
		t.Errorf("different covers rendered identically")
	}
}
