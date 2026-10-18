// Package compositor keeps body, prop and shadow independent. Repaired body
// assets are a prerequisite; it cannot undo pixels occluded in V10 composites.
package compositor

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io/fs"
	"math"
	"runtime"
	"sync"
)

type frameKey struct {
	state string
	index int
}
type Cache struct {
	mu      sync.Mutex
	frames  map[string][][]byte
	decoded map[frameKey]*image.NRGBA
	order   []frameKey
	bounds  image.Rectangle
}

// Load decodes every numbered resource and validates a shared canvas. Cache
// ownership is private; no caller can mutate a shared frame or state alias.
func Load(files fs.FS, counts map[string]int, canvas image.Rectangle) (*Cache, error) {
	if canvas.Empty() || canvas.Min != (image.Point{}) {
		return nil, errors.New("canvas must have positive size and zero origin")
	}
	c := &Cache{frames: make(map[string][][]byte), decoded: make(map[frameKey]*image.NRGBA), bounds: canvas}
	for state, count := range counts {
		if count < 1 {
			return nil, fmt.Errorf("empty clip %s", state)
		}
		for i := 0; i < count; i++ {
			path := fmt.Sprintf("%s_%03d.png", state, i)
			encoded, err := fs.ReadFile(files, path)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			decoded, err := png.Decode(bytes.NewReader(encoded))
			if err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			if decoded.Bounds() != canvas {
				return nil, fmt.Errorf("%s: canvas %v != %v", path, decoded.Bounds(), canvas)
			}
			if _, ok := decoded.(*image.NRGBA); !ok {
				return nil, fmt.Errorf("%s: expected 8-bit straight RGBA", path)
			}
			// Keep compressed bytes, not hundreds of megabytes of decoded canvases.
			c.frames[state] = append(c.frames[state], encoded)
		}
	}
	return c, nil
}

// BodyCopy is a private-buffer snapshot. Overlay drawing never writes to cache.
func (c *Cache) BodyCopy(state string, index int) (*image.NRGBA, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	frames, ok := c.frames[state]
	if !ok || index < 0 || index >= len(frames) {
		return nil, errors.New("unknown state/frame")
	}
	key := frameKey{state, index}
	frame, ok := c.decoded[key]
	if !ok {
		decoded, err := png.Decode(bytes.NewReader(frames[index]))
		if err != nil {
			return nil, err
		}
		frame = decoded.(*image.NRGBA)
		if len(c.order) == 8 {
			delete(c.decoded, c.order[0])
			c.order = c.order[1:]
		}
		c.decoded[key] = frame
		c.order = append(c.order, key)
	}
	result := image.NewNRGBA(c.bounds)
	copy(result.Pix, frame.Pix)
	return result, nil
}

type Layer struct {
	Image image.Image
	At    image.Point
}

// Compose always allocates a cleared canvas and draws one body only. At the
// start of dropping, prop occlusion is naturally handled by drawing it behind
// the opaque body. A caller must pass nil prop before BeginDrop.
func Compose(canvas image.Rectangle, body Layer, prop, shadow *Layer) *image.RGBA {
	out := image.NewRGBA(canvas)
	paint := func(l *Layer) {
		if l != nil && l.Image != nil {
			r := l.Image.Bounds().Add(l.At.Sub(l.Image.Bounds().Min))
			draw.Draw(out, r, l.Image, l.Image.Bounds().Min, draw.Over)
		}
	}
	paint(shadow)
	paint(prop)
	paint(&body)
	return out
}

// BGRA consumes Go's already-premultiplied RGBA values exactly once. Returned
// scanlines are top-down, four bytes per pixel, with zero RGB at alpha zero.
func BGRA(src image.Image) []byte {
	b := src.Bounds()
	out := make([]byte, b.Dx()*b.Dy()*4)
	i := 0
	if rgba, ok := src.(*image.RGBA); ok {
		// The composited canvas is always this type; swap its channels in
		// place of four interface calls per pixel.
		for y := b.Min.Y; y < b.Max.Y; y++ {
			row := rgba.Pix[rgba.PixOffset(b.Min.X, y):]
			for x := 0; x < b.Dx(); x, i = x+1, i+4 {
				if p := row[x*4 : x*4+4 : x*4+4]; p[3] != 0 {
					out[i], out[i+1], out[i+2], out[i+3] = p[2], p[1], p[0], p[3]
				}
			}
		}
		return out
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, blue, a := src.At(x, y).RGBA()
			if a == 0 {
				i += 4
				continue
			}
			out[i] = byte(blue >> 8)
			out[i+1] = byte(g >> 8)
			out[i+2] = byte(r >> 8)
			out[i+3] = byte(a >> 8)
			i += 4
		}
	}
	return out
}

// cubicWeight is Catmull-Rom interpolation (a = -0.5). Sampling is done on
// premultiplied channels so transparent edges do not grow dark or colored halos.
func cubicWeight(x float64) float64 {
	x = math.Abs(x)
	switch {
	case x <= 1:
		return 1.5*x*x*x - 2.5*x*x + 1
	case x < 2:
		return -0.5*x*x*x + 2.5*x*x - 4*x + 2
	default:
		return 0
	}
}

// Resize uses one user-selected scale on the entire stable canvas. Bicubic
// interpolation improves laugh-frame edge quality versus simple bilinear
// filtering while still operating on premultiplied channels.
func Resize(src image.Image, width, height int) (*image.RGBA, error) {
	if width < 1 || height < 1 || width > 8192 || height > 8192 {
		return nil, errors.New("invalid render dimensions")
	}
	b := src.Bounds()
	if b.Empty() {
		return nil, errors.New("empty source")
	}
	out := image.NewRGBA(image.Rect(0, 0, width, height))
	clamp := func(v, lo, hi int) int {
		if v < lo {
			return lo
		}
		if v > hi {
			return hi
		}
		return v
	}
	if rgba, ok := src.(*image.RGBA); ok {
		resizeRGBA(out, rgba)
		return out, nil
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			sx := (float64(x)+.5)*float64(b.Dx())/float64(width) - .5
			sy := (float64(y)+.5)*float64(b.Dy())/float64(height) - .5
			ix, iy := int(math.Floor(sx)), int(math.Floor(sy))
			var channels [4]float64
			var total float64
			for my := -1; my <= 2; my++ {
				wy := cubicWeight(sy - float64(iy+my))
				py := clamp(iy+my, 0, b.Dy()-1)
				for mx := -1; mx <= 2; mx++ {
					wx := cubicWeight(sx - float64(ix+mx))
					px := clamp(ix+mx, 0, b.Dx()-1)
					w := wx * wy
					if w == 0 {
						continue
					}
					r, g, bl, a := src.At(px+b.Min.X, py+b.Min.Y).RGBA()
					channels[0] += float64(r) * w
					channels[1] += float64(g) * w
					channels[2] += float64(bl) * w
					channels[3] += float64(a) * w
					total += w
				}
			}
			if total == 0 {
				continue
			}
			for i := 0; i < 4; i++ {
				channels[i] /= total
				if channels[i] < 0 {
					channels[i] = 0
				}
				if channels[i] > 65535 {
					channels[i] = 65535
				}
			}
			// Catmull-Rom has negative lobes. Independent channel clamping to
			// 0..65535 is insufficient: RGB can overshoot the interpolated alpha.
			// UpdateLayeredWindow requires premultiplied BGRA with every RGB <= A.
			// Clamp in the premultiplied domain *before* 8-bit quantization.
			a := channels[3]
			for i := 0; i < 3; i++ {
				if channels[i] > a {
					channels[i] = a
				}
			}
			out.SetRGBA(x, y, color.RGBA{uint8(math.Round(channels[0] / 257)), uint8(math.Round(channels[1] / 257)), uint8(math.Round(channels[2] / 257)), uint8(math.Round(a / 257))})
		}
	}
	return out, nil
}

// resizeRGBA is Resize for the canvas type the host actually composites. It
// performs the same arithmetic in the same order as the general loop, so the
// two produce identical pixels; it reads the buffer directly, computes each
// axis's taps once, and spreads the rows of a large frame over the processors.
func resizeRGBA(out, src *image.RGBA) {
	b := src.Bounds()
	width, height := out.Rect.Dx(), out.Rect.Dy()
	type taps struct {
		at     [4]int
		weight [4]float64
	}
	axis := func(n, size int) []taps {
		t := make([]taps, n)
		for i := range t {
			s := (float64(i)+.5)*float64(size)/float64(n) - .5
			base := int(math.Floor(s))
			for m := -1; m <= 2; m++ {
				t[i].weight[m+1] = cubicWeight(s - float64(base+m))
				t[i].at[m+1] = min(max(base+m, 0), size-1)
			}
		}
		return t
	}
	xs, ys := axis(width, b.Dx()), axis(height, b.Dy())
	rows := func(from, to int) {
		for y := from; y < to; y++ {
			for x := 0; x < width; x++ {
				var channels [4]float64
				var total float64
				for my := 0; my < 4; my++ {
					wy := ys[y].weight[my]
					line := src.Pix[src.PixOffset(b.Min.X, b.Min.Y+ys[y].at[my]):]
					for mx := 0; mx < 4; mx++ {
						w := xs[x].weight[mx] * wy
						if w == 0 {
							continue
						}
						p := line[xs[x].at[mx]*4 : xs[x].at[mx]*4+4 : xs[x].at[mx]*4+4]
						// A 16-bit channel is the byte repeated, as RGBA() reports it.
						channels[0] += float64(uint32(p[0])*257) * w
						channels[1] += float64(uint32(p[1])*257) * w
						channels[2] += float64(uint32(p[2])*257) * w
						channels[3] += float64(uint32(p[3])*257) * w
						total += w
					}
				}
				if total == 0 {
					continue
				}
				for i := 0; i < 4; i++ {
					channels[i] /= total
					if channels[i] < 0 {
						channels[i] = 0
					}
					if channels[i] > 65535 {
						channels[i] = 65535
					}
				}
				a := channels[3]
				for i := 0; i < 3; i++ {
					if channels[i] > a {
						channels[i] = a
					}
				}
				o := out.PixOffset(x, y)
				out.Pix[o], out.Pix[o+1], out.Pix[o+2], out.Pix[o+3] = uint8(math.Round(channels[0]/257)), uint8(math.Round(channels[1]/257)), uint8(math.Round(channels[2]/257)), uint8(math.Round(a/257))
			}
		}
	}
	workers := min(runtime.GOMAXPROCS(0), 8, height/32)
	if workers < 2 || width*height < 20000 {
		rows(0, height)
		return
	}
	var wait sync.WaitGroup
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			rows(height*i/workers, height*(i+1)/workers)
		}()
	}
	wait.Wait()
}
