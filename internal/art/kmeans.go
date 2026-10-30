package art

import (
	"image"
	"image/color"
	"math"
	"math/rand"
)

type rgb [3]float64

func dist2(a, b rgb) float64 {
	d0, d1, d2 := a[0]-b[0], a[1]-b[1], a[2]-b[2]
	return d0*d0 + d1*d1 + d2*d2
}

func nearest(cent []rgb, p rgb) (int, float64) {
	best, bd := 0, math.Inf(1)
	for i, c := range cent {
		if d := dist2(c, p); d < bd {
			best, bd = i, d
		}
	}
	return best, bd
}

// kmeans clusters pts into at most k groups. It is deterministic for a given
// seed: k-means++ seeding from a fixed RNG, then Lloyd iterations. Clusters
// that end up empty keep their centroid; callers should check the sizes.
func kmeans(pts []rgb, k int, seed int64) (cent []rgb, assign []int) {
	if len(pts) == 0 || k <= 0 {
		return nil, nil
	}
	k = min(k, len(pts))
	rng := rand.New(rand.NewSource(seed))
	cent = []rgb{pts[rng.Intn(len(pts))]}
	d2 := make([]float64, len(pts))
	for len(cent) < k {
		total := 0.0
		for i, p := range pts {
			_, d2[i] = nearest(cent, p)
			total += d2[i]
		}
		if total == 0 { // fewer distinct colours than k
			break
		}
		r, pick := rng.Float64()*total, len(pts)-1
		for i, d := range d2 {
			if r -= d; r <= 0 {
				pick = i
				break
			}
		}
		cent = append(cent, pts[pick])
	}

	assign = make([]int, len(pts))
	for i := range assign {
		assign[i] = -1
	}
	for iter := 0; iter < 25; iter++ {
		changed := false
		for i, p := range pts {
			if c, _ := nearest(cent, p); c != assign[i] {
				assign[i], changed = c, true
			}
		}
		if !changed {
			break
		}
		sums := make([]rgb, len(cent))
		counts := make([]int, len(cent))
		for i, p := range pts {
			c := assign[i]
			counts[c]++
			for j := range p {
				sums[c][j] += p[j]
			}
		}
		for c := range cent {
			if counts[c] > 0 {
				for j := range sums[c] {
					cent[c][j] = sums[c][j] / float64(counts[c])
				}
			}
		}
	}
	return cent, assign
}

func pixels(img *image.RGBA) []rgb {
	b := img.Bounds()
	pts := make([]rgb, 0, b.Dx()*b.Dy())
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.RGBAAt(x, y)
			pts = append(pts, rgb{float64(c.R), float64(c.G), float64(c.B)})
		}
	}
	return pts
}

func toRGBA(c rgb) color.RGBA {
	return color.RGBA{uint8(math.Round(c[0])), uint8(math.Round(c[1])), uint8(math.Round(c[2])), 255}
}

// quantize snaps every pixel to one of k k-means colours, for the flat,
// stylised look. k <= 0 leaves the image untouched.
func quantize(img *image.RGBA, k int) {
	if k <= 0 {
		return
	}
	cent, assign := kmeans(pixels(img), k, 1)
	b := img.Bounds()
	i := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			img.SetRGBA(x, y, toRGBA(cent[assign[i]]))
			i++
		}
	}
}

// scanlines darkens every other pixel row by 12%.
func scanlines(img *image.RGBA) {
	b := img.Bounds()
	for y := b.Min.Y + 1; y < b.Max.Y; y += 2 {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.RGBAAt(x, y)
			img.SetRGBA(x, y, color.RGBA{uint8(float64(c.R) * 0.88), uint8(float64(c.G) * 0.88), uint8(float64(c.B) * 0.88), c.A})
		}
	}
}

// pickAccent clusters the cover into five colours and returns the cluster with
// the best saturation × share of the image, ignoring near-black and near-white.
func pickAccent(img *image.RGBA) color.RGBA {
	pts := pixels(img)
	cent, assign := kmeans(pts, 5, 1)
	sizes := make([]int, len(cent))
	for _, c := range assign {
		sizes[c]++
	}
	var best, biggest color.RGBA
	bestScore, bigN := -1.0, 0
	for i, c := range cent {
		if sizes[i] == 0 {
			continue
		}
		col := toRGBA(c)
		if sizes[i] > bigN {
			biggest, bigN = col, sizes[i]
		}
		_, s, l := toHSL(col)
		if l < 0.15 || l > 0.9 {
			continue
		}
		if score := s * float64(sizes[i]) / float64(len(pts)); score > bestScore {
			best, bestScore = col, score
		}
	}
	if bestScore < 0 {
		return biggest
	}
	return best
}
