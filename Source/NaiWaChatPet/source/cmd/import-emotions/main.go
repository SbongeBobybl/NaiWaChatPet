// Command import-emotions turns the key-pose emotion GIFs into animation
// folders the pet can load.
//
// The GIFs are 256-colour with a one-bit edge, drawn smaller than the pet's
// idle pose and standing on a white floor. Each kept frame is therefore
// cleaned, de-banded, colour-matched to the idle render and registered to its
// scale and ground line before it is written.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sync"

	"codexpet/reconstruction/emotion"
)

const (
	width, height = 420, 560
	groundY       = 540
)

// plate is a frame being cleaned: straight colour plus a hard coverage mask.
type plate struct {
	rgb []float32
	on  []bool
}

func newPlate(im *image.NRGBA) *plate {
	p := &plate{rgb: make([]float32, width*height*3), on: make([]bool, width*height)}
	for i := 0; i < width*height; i++ {
		p.on[i] = im.Pix[i*4+3] >= 128
		for c := 0; c < 3; c++ {
			p.rgb[i*3+c] = float32(im.Pix[i*4+c])
		}
	}
	return p
}

// strip removes the white floor and matting fringe: everything whitish that
// can be reached from outside the figure, then one more pixel of edge, which
// is where colour was blended with the white background.
func (p *plate) strip() {
	whitish := func(i int) bool {
		r, g, b := p.rgb[i*3], p.rgb[i*3+1], p.rgb[i*3+2]
		lo, hi := min(r, g, b), max(r, g, b)
		return lo >= 196 && hi-lo <= 46
	}
	var queue []int
	for i, on := range p.on {
		if !on {
			queue = append(queue, i)
		}
	}
	for len(queue) > 0 {
		i := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		x, y := i%width, i/width
		for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			nx, ny := x+d[0], y+d[1]
			if nx < 0 || ny < 0 || nx >= width || ny >= height {
				continue
			}
			if n := ny*width + nx; p.on[n] && whitish(n) {
				p.on[n] = false
				queue = append(queue, n)
			}
		}
	}
	edge := make([]bool, len(p.on))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			i := y*width + x
			edge[i] = p.on[i] && (x == 0 || y == 0 || x == width-1 || y == height-1 || !p.on[i-1] || !p.on[i+1] || !p.on[i-width] || !p.on[i+width])
		}
	}
	for i, e := range edge {
		if e {
			p.on[i] = false
		}
	}
}

// deband smooths the palette's shading steps with a bilateral filter, which
// leaves the eyes, mouth, hands and belly outline sharp.
func (p *plate) deband() {
	const radius, spread, tolerance = 7, 4.5, 14.0
	var kernel [2*radius + 1][2*radius + 1]float32
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			kernel[dy+radius][dx+radius] = float32(math.Exp(-float64(dx*dx+dy*dy) / (2 * spread * spread)))
		}
	}
	var similar [3 * 256 * 256]float32
	for i := range similar {
		similar[i] = float32(math.Exp(-float64(i) / (2 * tolerance * tolerance)))
	}
	for pass := 0; pass < 2; pass++ {
		out := make([]float32, len(p.rgb))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				i := y*width + x
				if !p.on[i] {
					continue
				}
				r, g, b := p.rgb[i*3], p.rgb[i*3+1], p.rgb[i*3+2]
				var sr, sg, sb, total float32
				for dy := max(-radius, -y); dy <= min(radius, height-1-y); dy++ {
					for dx := max(-radius, -x); dx <= min(radius, width-1-x); dx++ {
						n := i + dy*width + dx
						if !p.on[n] {
							continue
						}
						dr, dg, db := p.rgb[n*3]-r, p.rgb[n*3+1]-g, p.rgb[n*3+2]-b
						w := kernel[dy+radius][dx+radius] * similar[int(dr*dr+dg*dg+db*db)]
						sr, sg, sb, total = sr+w*p.rgb[n*3], sg+w*p.rgb[n*3+1], sb+w*p.rgb[n*3+2], total+w
					}
				}
				out[i*3], out[i*3+1], out[i*3+2] = sr/total, sg/total, sb/total
			}
		}
		p.rgb = out
	}
}

// tone is a per-channel gain and offset.
type tone [3][2]float64

func (p *plate) stats() (mean, dev [3]float64) {
	var n float64
	for i, on := range p.on {
		if on {
			n++
			for c := 0; c < 3; c++ {
				mean[c] += float64(p.rgb[i*3+c])
			}
		}
	}
	for c := range mean {
		mean[c] /= n
	}
	for i, on := range p.on {
		if on {
			for c := 0; c < 3; c++ {
				d := float64(p.rgb[i*3+c]) - mean[c]
				dev[c] += d * d
			}
		}
	}
	for c := range dev {
		dev[c] = math.Sqrt(dev[c] / n)
	}
	return mean, dev
}

// match returns the tone that carries one figure's colour statistics onto
// another's. Gains are kept close to 1 so no channel is stretched into noise.
func match(from, to *plate) tone {
	fm, fd := from.stats()
	tm, td := to.stats()
	var t tone
	for c := 0; c < 3; c++ {
		gain := math.Max(.85, math.Min(1.18, td[c]/fd[c]))
		t[c] = [2]float64{gain, tm[c] - gain*fm[c]}
	}
	return t
}

func (p *plate) grade(t tone) {
	for i := range p.on {
		for c := 0; c < 3; c++ {
			p.rgb[i*3+c] = float32(math.Max(0, math.Min(255, float64(p.rgb[i*3+c])*t[c][0]+t[c][1])))
		}
	}
}

// stand describes where a figure stands: its height and the middle of its feet.
type stand struct{ top, bottom, feet float64 }

func (p *plate) stand() stand {
	top, bottom := height, 0
	for i, on := range p.on {
		if on {
			top, bottom = min(top, i/width), max(bottom, i/width)
		}
	}
	left, right := width, 0
	for y := bottom - (bottom-top)/12; y <= bottom; y++ {
		for x := 0; x < width; x++ {
			if p.on[y*width+x] {
				left, right = min(left, x), max(right, x)
			}
		}
	}
	return stand{float64(top), float64(bottom + 1), float64(left+right+1) / 2}
}

// placement maps source pixels onto the canvas: scale about the origin, then shift.
type placement struct{ scale, dx, dy float64 }

// place resamples the plate through a placement and reports whether the
// figure was cut off by the canvas.
func (p *plate) place(m placement) (*image.NRGBA, bool) {
	out := image.NewNRGBA(image.Rect(0, 0, width, height))
	clipped := false
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			sx, sy := (float64(x)+.5-m.dx)/m.scale-.5, (float64(y)+.5-m.dy)/m.scale-.5
			x0, y0 := int(math.Floor(sx)), int(math.Floor(sy))
			fx, fy := sx-float64(x0), sy-float64(y0)
			var r, g, b, a float64
			for _, tap := range [4]struct {
				x, y int
				w    float64
			}{{x0, y0, (1 - fx) * (1 - fy)}, {x0 + 1, y0, fx * (1 - fy)}, {x0, y0 + 1, (1 - fx) * fy}, {x0 + 1, y0 + 1, fx * fy}} {
				if tap.x < 0 || tap.y < 0 || tap.x >= width || tap.y >= height || !p.on[tap.y*width+tap.x] {
					continue
				}
				i := tap.y*width + tap.x
				r, g, b, a = r+tap.w*float64(p.rgb[i*3]), g+tap.w*float64(p.rgb[i*3+1]), b+tap.w*float64(p.rgb[i*3+2]), a+tap.w
			}
			if a < .03 {
				continue
			}
			if x == 0 || y == 0 || x == width-1 {
				clipped = true
			}
			out.SetNRGBA(x, y, color.NRGBA{uint8(math.Round(r / a)), uint8(math.Round(g / a)), uint8(math.Round(b / a)), uint8(math.Round(255 * math.Min(1, a)))})
		}
	}
	return out, clipped
}

func readNRGBA(path string) (*image.NRGBA, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	im, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	n := image.NewNRGBA(im.Bounds())
	draw.Draw(n, n.Bounds(), im, im.Bounds().Min, draw.Src)
	return n, nil
}

// readGIF composes every frame the way a viewer would show it.
func readGIF(path string) (frames []*image.NRGBA, delays []int, sum string, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, "", err
	}
	g, err := gif.DecodeAll(bytes.NewReader(b))
	if err != nil {
		return nil, nil, "", err
	}
	if g.Config.Width != width || g.Config.Height != height {
		return nil, nil, "", fmt.Errorf("%dx%d, want %dx%d", g.Config.Width, g.Config.Height, width, height)
	}
	digest := sha256.Sum256(b)
	screen := image.NewNRGBA(image.Rect(0, 0, width, height))
	for i, f := range g.Image {
		before := append([]uint8(nil), screen.Pix...)
		draw.Draw(screen, f.Bounds(), f, f.Bounds().Min, draw.Over)
		shown := image.NewNRGBA(screen.Bounds())
		copy(shown.Pix, screen.Pix)
		frames, delays = append(frames, shown), append(delays, g.Delay[i])
		switch g.Disposal[i] {
		case gif.DisposalBackground:
			draw.Draw(screen, f.Bounds(), image.Transparent, image.Point{}, draw.Src)
		case gif.DisposalPrevious:
			copy(screen.Pix, before)
		}
	}
	return frames, delays, hex.EncodeToString(digest[:]), nil
}

func difference(a, b *image.NRGBA) float64 {
	var total float64
	for i := range a.Pix {
		total += math.Abs(float64(a.Pix[i]) - float64(b.Pix[i]))
	}
	return total / float64(len(a.Pix))
}

// clip is one animation folder to write.
type clip struct {
	name, emotion string
	fps           float64
	loop          bool
	frames        []*image.NRGBA
}

func (c clip) write(root, source string, priority int, mouth emotion.Anchor) error {
	dir := filepath.Join(root, c.name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	// Clear frames left by an earlier import that kept more of them.
	stale, _ := filepath.Glob(filepath.Join(dir, "[0-9][0-9][0-9][0-9].png"))
	for _, path := range stale {
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	m := emotion.Manifest{Name: c.name, Emotion: c.emotion, FPS: c.fps, FrameCount: len(c.frames), Loop: c.loop, Priority: priority, MouthAnchor: mouth,
		Canvas: emotion.Canvas{Width: width, Height: height}, GroundY: groundY, Reviewed: true, SourceSHA256: source}
	for i, frame := range c.frames {
		var b bytes.Buffer
		if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&b, frame); err != nil {
			return err
		}
		digest := sha256.Sum256(b.Bytes())
		file := fmt.Sprintf("%04d.png", i)
		if err := os.WriteFile(filepath.Join(dir, file), b.Bytes(), 0644); err != nil {
			return err
		}
		m.Frames = append(m.Frames, emotion.Frame{File: file, SHA256: hex.EncodeToString(digest[:])})
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "manifest.json"), append(b, '\n'), 0644)
}

func main() {
	gifs := flag.String("gifs", "", "directory holding the emotion GIFs")
	out := flag.String("out", "../animations", "animations directory to write into")
	idle := flag.String("idle", "cmd/codexpet/embedded_assets/body/idle_000.png", "idle frame to match")
	sheet := flag.String("sheet", "", "optional contact sheet for reviewing the result")
	flag.Parse()
	if err := run(*gifs, *out, *idle, *sheet); err != nil {
		fmt.Fprintln(os.Stderr, "import-emotions:", err)
		os.Exit(1)
	}
}

func run(gifs, out, idlePath, sheetPath string) error {
	if gifs == "" {
		return errors.New("-gifs is required")
	}
	idleImage, err := readNRGBA(idlePath)
	if err != nil {
		return err
	}
	if idleImage.Bounds() != image.Rect(0, 0, width, height) {
		return errors.New("idle frame is not on the pet's canvas")
	}
	idle := newPlate(idleImage)

	// Every GIF opens on the same neutral drawing, so one colour match and one
	// scale serve the whole set and keep the poses consistent with each other.
	var posed []emotion.Entry
	for _, e := range emotion.Catalog {
		if e.Posed {
			posed = append(posed, e)
		}
	}
	if len(posed) == 0 {
		return errors.New("the catalog lists no posed expressions")
	}
	source := func(e emotion.Entry) string {
		if e.Source != "" {
			return e.Source
		}
		return e.ID
	}
	first, _, _, err := readGIF(filepath.Join(gifs, source(posed[0])+".gif"))
	if err != nil {
		return fmt.Errorf("%s: %w", source(posed[0]), err)
	}
	neutral := newPlate(first[0])
	neutral.strip()
	neutral.deband()
	grade := match(neutral, idle)
	from, to := neutral.stand(), idle.stand()
	scale := (to.bottom - to.top) / (from.bottom - from.top)
	// The beak sits this far from the feet on the neutral drawing; it anchors
	// the speech bubble for every pose.
	mouth := emotion.Anchor{X: math.Round(to.feet + scale*(146-from.feet)), Y: math.Round(groundY + scale*(152-from.bottom))}
	fmt.Printf("scale %.3f; tone gains %.2f %.2f %.2f; mouth anchor (%.0f, %.0f)\n", scale, grade[0][0], grade[1][0], grade[2][0], mouth.X, mouth.Y)

	type result struct {
		entry emotion.Entry
		peak  *image.NRGBA
		note  string
		err   error
	}
	results := make([]result, len(posed))
	var wait sync.WaitGroup
	for i, entry := range posed {
		wait.Add(1)
		go func() {
			defer wait.Done()
			r := &results[i]
			r.entry = entry
			frames, delays, sum, err := readGIF(filepath.Join(gifs, source(entry)+".gif"))
			if err != nil {
				r.err = err
				return
			}
			if len(frames) < 2 {
				r.err = errors.New("needs a neutral frame and a pose")
				return
			}
			// The pose is the frame the GIF dwells on; ties go to the one
			// furthest from neutral.
			peak := 1
			for k := 2; k < len(frames); k++ {
				if delays[k] > delays[peak] || delays[k] == delays[peak] && difference(frames[k], frames[0]) > difference(frames[peak], frames[0]) {
					peak = k
				}
			}
			// Only the pose is imported. The frames before it are either face
			// morphs that ghost the eyes or drawings that sit somewhere else on
			// the canvas, and the rig's own settle reads better than either.
			p := newPlate(frames[peak])
			p.strip()
			// The drawings drift between frames, so each pose is stood on the
			// idle pose's spot by its own feet rather than by a shared offset.
			at := p.stand()
			put := placement{scale, to.feet - scale*at.feet, groundY - scale*at.bottom}
			p.deband()
			p.grade(grade)
			pose, clipped := p.place(put)
			if clipped {
				r.err = fmt.Errorf("frame %d does not fit the canvas", peak)
				return
			}
			main := clip{name: entry.ID, emotion: entry.ID, fps: 12, loop: true, frames: []*image.NRGBA{pose}}
			if err := main.write(out, sum, entry.Priority, mouth); err != nil {
				r.err = err
				return
			}
			r.peak = pose
			r.note = fmt.Sprintf("pose = frame %d of %d, feet moved %+.0f px from neutral", peak, len(frames), at.feet-from.feet)
		}()
	}
	wait.Wait()
	var failed []error
	for _, r := range results {
		if r.err != nil {
			failed = append(failed, fmt.Errorf("%s: %w", r.entry.ID, r.err))
			continue
		}
		fmt.Printf("%-13s %s\n", r.entry.ID, r.note)
	}
	if len(failed) > 0 {
		return errors.Join(failed...)
	}
	if sheetPath == "" {
		return nil
	}

	// The sheet opens with the idle frame so each pose can be judged against it.
	cells := []*image.NRGBA{idleImage}
	for _, r := range results {
		cells = append(cells, r.peak)
	}
	const columns = 9
	cw, ch := width/2, height/2
	rows := (len(cells) + columns - 1) / columns
	page := image.NewRGBA(image.Rect(0, 0, columns*cw, rows*ch))
	draw.Draw(page, page.Bounds(), image.NewUniform(color.RGBA{70, 90, 120, 255}), image.Point{}, draw.Src)
	for n, cell := range cells {
		for y := 0; y < ch; y++ {
			for x := 0; x < cw; x++ {
				var r, g, b, a int
				for _, d := range [4][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
					c := cell.NRGBAAt(x*2+d[0], y*2+d[1])
					r, g, b, a = r+int(c.R)*int(c.A), g+int(c.G)*int(c.A), b+int(c.B)*int(c.A), a+int(c.A)
				}
				at := image.Pt(n%columns*cw+x, n/columns*ch+y)
				under := page.RGBAAt(at.X, at.Y)
				keep := 4*255 - a
				page.SetRGBA(at.X, at.Y, color.RGBA{uint8((r + int(under.R)*keep) / (4 * 255)), uint8((g + int(under.G)*keep) / (4 * 255)), uint8((b + int(under.B)*keep) / (4 * 255)), 255})
			}
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, page); err != nil {
		return err
	}
	return os.WriteFile(sheetPath, b.Bytes(), 0644)
}
