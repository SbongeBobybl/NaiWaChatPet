// Command import-scene turns a filmed clip of the pet into an entry, a loop
// and an exit the pet can play, for example carrying a box while it tidies.
//
// The clip must be shot on a plain, evenly lit backdrop with a locked camera.
// Extract its frames first, for example:
//
//	ffmpeg -i boxes.mp4 frames/%03d.png
//
// The backdrop is modelled as a smooth gradient and keyed out, the character
// is scaled to the pet's canvas and stood on its ground line, and its position
// on the first frame is recorded so the pet does not jump sideways when the
// scene begins.
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
	// keyed is the colour distance from the backdrop beyond which a pixel
	// certainly belongs to the character; soft is where the edge fades out.
	keyed, soft = 30.0, 6.0
)

type frame struct {
	w, h int
	rgb  []float64
}

func readFrame(path string) (*frame, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	im, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	r := im.Bounds()
	f := &frame{w: r.Dx(), h: r.Dy(), rgb: make([]float64, r.Dx()*r.Dy()*3)}
	for y := 0; y < f.h; y++ {
		for x := 0; x < f.w; x++ {
			c := color.NRGBAModel.Convert(im.At(r.Min.X+x, r.Min.Y+y)).(color.NRGBA)
			i := (y*f.w + x) * 3
			f.rgb[i], f.rgb[i+1], f.rgb[i+2] = float64(c.R), float64(c.G), float64(c.B)
		}
	}
	return f, nil
}

// backdrop is a quadratic surface per channel over normalised coordinates.
type backdrop [3][6]float64

func terms(x, y float64) [6]float64 { return [6]float64{1, x, y, x * x, x * y, y * y} }

func (b *backdrop) at(x, y float64) [3]float64 {
	t := terms(x, y)
	var out [3]float64
	for c := range out {
		for k, v := range t {
			out[c] += b[c][k] * v
		}
	}
	return out
}

// fitBackdrop fits the backdrop to pale, grey pixels, then refits without
// the ones the first fits explain poorly, so the character never pulls it.
func fitBackdrop(frames []*frame) (*backdrop, error) {
	type sample struct {
		x, y float64
		c    [3]float64
	}
	var samples []sample
	for _, f := range frames {
		for y := 0; y < f.h; y += 6 {
			for x := 0; x < f.w; x += 6 {
				i := (y*f.w + x) * 3
				c := [3]float64{f.rgb[i], f.rgb[i+1], f.rgb[i+2]}
				lo, hi := math.Min(c[0], math.Min(c[1], c[2])), math.Max(c[0], math.Max(c[1], c[2]))
				if lo > 185 && hi-lo < 14 {
					samples = append(samples, sample{float64(x) / float64(f.w), float64(y) / float64(f.h), c})
				}
			}
		}
	}
	if len(samples) < 500 {
		return nil, errors.New("too little plain backdrop to key against")
	}
	var b backdrop
	keep := make([]bool, len(samples))
	for i := range keep {
		keep[i] = true
	}
	for round := 0; round < 3; round++ {
		for c := 0; c < 3; c++ {
			var m [6][7]float64
			for i, s := range samples {
				if !keep[i] {
					continue
				}
				t := terms(s.x, s.y)
				for r := 0; r < 6; r++ {
					for k := 0; k < 6; k++ {
						m[r][k] += t[r] * t[k]
					}
					m[r][6] += t[r] * s.c[c]
				}
			}
			solved, err := solve(m)
			if err != nil {
				return nil, err
			}
			b[c] = solved
		}
		for i, s := range samples {
			p := b.at(s.x, s.y)
			keep[i] = math.Hypot(math.Hypot(s.c[0]-p[0], s.c[1]-p[1]), s.c[2]-p[2]) < 9
		}
	}
	return &b, nil
}

// solve is Gaussian elimination with partial pivoting on an augmented matrix.
func solve(m [6][7]float64) ([6]float64, error) {
	for col := 0; col < 6; col++ {
		pivot := col
		for r := col + 1; r < 6; r++ {
			if math.Abs(m[r][col]) > math.Abs(m[pivot][col]) {
				pivot = r
			}
		}
		if math.Abs(m[pivot][col]) < 1e-12 {
			return [6]float64{}, errors.New("backdrop fit is singular")
		}
		m[col], m[pivot] = m[pivot], m[col]
		for r := 0; r < 6; r++ {
			if r == col {
				continue
			}
			k := m[r][col] / m[col][col]
			for c := col; c < 7; c++ {
				m[r][c] -= k * m[col][c]
			}
		}
	}
	var out [6]float64
	for r := range out {
		out[r] = m[r][6] / m[r][r]
	}
	return out, nil
}

// matte is a frame with its backdrop removed: straight colour and coverage.
type matte struct {
	w, h  int
	rgb   []float64
	alpha []float64
}

// key separates the character from the backdrop. Pixels far from the backdrop
// colour seed the figure, enclosed holes such as eyes and tape are filled, and
// only a thin band around the figure gets partial coverage, from which the
// backdrop's colour is then unmixed.
func key(f *frame, b *backdrop) *matte {
	n := f.w * f.h
	m := &matte{w: f.w, h: f.h, rgb: append([]float64(nil), f.rgb...), alpha: make([]float64, n)}
	distance := make([]float64, n)
	shade := make([]bool, n)
	ground := make([][3]float64, n)
	for y := 0; y < f.h; y++ {
		for x := 0; x < f.w; x++ {
			i := y*f.w + x
			p := b.at(float64(x)/float64(f.w), float64(y)/float64(f.h))
			ground[i] = p
			c := [3]float64{f.rgb[i*3], f.rgb[i*3+1], f.rgb[i*3+2]}
			distance[i] = math.Hypot(math.Hypot(c[0]-p[0], c[1]-p[1]), c[2]-p[2])
			// Floor shadow and bounce light are the backdrop made darker or
			// lighter and tinted a little by the figure: pale, nearly
			// colourless and matching once the brightness is. The box's tape
			// and the olive feet carry far more colour than this.
			lum, back := (c[0]+c[1]+c[2])/3, (p[0]+p[1]+p[2])/3
			chroma := math.Max(c[0], math.Max(c[1], c[2])) - math.Min(c[0], math.Min(c[1], c[2]))
			k := lum / back
			residual := math.Hypot(math.Hypot(c[0]-k*p[0], c[1]-k*p[1]), c[2]-k*p[2])
			if shade[i] = chroma < 30 && lum > 140 && lum < back+40 && residual < 22; shade[i] {
				distance[i] = math.Min(distance[i], 2.5*residual)
			}
		}
	}
	// Reach the backdrop from the border; whatever stays unreached is figure.
	outside := make([]bool, n)
	var queue []int
	push := func(i int) {
		if !outside[i] && (distance[i] <= keyed || shade[i]) {
			outside[i] = true
			queue = append(queue, i)
		}
	}
	for x := 0; x < f.w; x++ {
		push(x)
		push((f.h-1)*f.w + x)
	}
	for y := 0; y < f.h; y++ {
		push(y * f.w)
		push(y*f.w + f.w - 1)
	}
	for len(queue) > 0 {
		i := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		x, y := i%f.w, i/f.w
		if x > 0 {
			push(i - 1)
		}
		if x < f.w-1 {
			push(i + 1)
		}
		if y > 0 {
			push(i - f.w)
		}
		if y < f.h-1 {
			push(i + f.w)
		}
	}
	// Floor seen between the legs is walled in by the figure, so the flood
	// cannot reach it; open up any sizeable pocket that looks like backdrop.
	pocket := make([]bool, n)
	for start := 0; start < n; start++ {
		if outside[start] || pocket[start] || !(distance[start] <= keyed || shade[start]) {
			continue
		}
		var blob []int
		stack := []int{start}
		pocket[start] = true
		for len(stack) > 0 {
			i := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			blob = append(blob, i)
			x, y := i%f.w, i/f.w
			for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				nx, ny := x+d[0], y+d[1]
				if nx < 0 || ny < 0 || nx >= f.w || ny >= f.h {
					continue
				}
				if j := ny*f.w + nx; !outside[j] && !pocket[j] && (distance[j] <= keyed || shade[j]) {
					pocket[j] = true
					stack = append(stack, j)
				}
			}
		}
		if len(blob) >= 250 {
			for _, i := range blob {
				outside[i] = true
			}
		}
	}
	// Drop specks: compression noise that happens to clear the threshold.
	seen := make([]bool, n)
	for start := 0; start < n; start++ {
		if outside[start] || seen[start] {
			continue
		}
		var blob []int
		stack := []int{start}
		seen[start] = true
		for len(stack) > 0 {
			i := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			blob = append(blob, i)
			x, y := i%f.w, i/f.w
			for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				nx, ny := x+d[0], y+d[1]
				if nx < 0 || ny < 0 || nx >= f.w || ny >= f.h {
					continue
				}
				if j := ny*f.w + nx; !outside[j] && !seen[j] {
					seen[j] = true
					stack = append(stack, j)
				}
			}
		}
		if len(blob) < 600 {
			for _, i := range blob {
				outside[i] = true
			}
		}
	}
	for i := 0; i < n; i++ {
		if !outside[i] {
			m.alpha[i] = 1
		}
	}
	// Soften two pixels either side of the outline.
	near := func(i int, inside bool) bool {
		x, y := i%f.w, i/f.w
		for dy := -2; dy <= 2; dy++ {
			for dx := -2; dx <= 2; dx++ {
				nx, ny := x+dx, y+dy
				if nx >= 0 && ny >= 0 && nx < f.w && ny < f.h && outside[ny*f.w+nx] != !inside {
					return true
				}
			}
		}
		return false
	}
	for i := 0; i < n; i++ {
		if !near(i, !outside[i]) {
			continue
		}
		a := math.Max(0, math.Min(1, (distance[i]-soft)/(keyed+20-soft)))
		if !outside[i] {
			a = math.Max(a, .35)
		}
		m.alpha[i] = a
		if a > 0 && a < 1 {
			// The pixel is a blend of figure and backdrop; recover the figure.
			for c := 0; c < 3; c++ {
				m.rgb[i*3+c] = math.Max(0, math.Min(255, ground[i][c]+(f.rgb[i*3+c]-ground[i][c])/a))
			}
		}
	}
	return m
}

// bounds is the box around everything at least half covered.
func (m *matte) bounds() image.Rectangle {
	r := image.Rectangle{}
	for y := 0; y < m.h; y++ {
		for x := 0; x < m.w; x++ {
			if m.alpha[y*m.w+x] >= .5 {
				r = r.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return r
}

// feet is the middle of the character's feet: the bottom rows of the
// largest yellow region, which keeps a prop standing beside it out.
func (m *matte) feet() float64 {
	n := m.w * m.h
	yellow := func(i int) bool {
		r, g, b := m.rgb[i*3], m.rgb[i*3+1], m.rgb[i*3+2]
		return m.alpha[i] >= .5 && r > 170 && g > 120 && b < 120 && r-b > 90
	}
	seen := make([]bool, n)
	var best []int
	for start := 0; start < n; start++ {
		if seen[start] || m.alpha[start] < .5 {
			continue
		}
		var blob []int
		yellows := 0
		stack := []int{start}
		seen[start] = true
		for len(stack) > 0 {
			i := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			blob = append(blob, i)
			if yellow(i) {
				yellows++
			}
			x, y := i%m.w, i/m.w
			for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				nx, ny := x+d[0], y+d[1]
				if nx < 0 || ny < 0 || nx >= m.w || ny >= m.h {
					continue
				}
				if j := ny*m.w + nx; !seen[j] && m.alpha[j] >= .5 {
					seen[j] = true
					stack = append(stack, j)
				}
			}
		}
		if yellows*3 > len(blob) && len(blob) > len(best) {
			best = blob
		}
	}
	if len(best) == 0 {
		return float64(m.w) / 2
	}
	bottom := 0
	for _, i := range best {
		bottom = max(bottom, i/m.w)
	}
	var sum, count float64
	for _, i := range best {
		if i/m.w >= bottom-m.h/40 {
			sum += float64(i % m.w)
			count++
		}
	}
	return sum / count
}

// place scales the matte by s, shifts it by (dx, dy) and samples it onto the
// pet's canvas in premultiplied colour.
func (m *matte) place(s, dx, dy float64) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, width, height))
	// Each canvas pixel averages the source pixels it covers, which is a
	// proper minification for the large downscale from video size.
	reach := math.Max(1, 1/s)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			cx, cy := (float64(x)+.5-dx)/s, (float64(y)+.5-dy)/s
			x0, x1 := int(math.Floor(cx-reach/2)), int(math.Ceil(cx+reach/2))
			y0, y1 := int(math.Floor(cy-reach/2)), int(math.Ceil(cy+reach/2))
			var r, g, b, a, total float64
			for sy := max(0, y0); sy < min(m.h, y1); sy++ {
				for sx := max(0, x0); sx < min(m.w, x1); sx++ {
					i := sy*m.w + sx
					w := m.alpha[i]
					r, g, b, a = r+w*m.rgb[i*3], g+w*m.rgb[i*3+1], b+w*m.rgb[i*3+2], a+w
					total++
				}
			}
			if total == 0 || a/total < .02 {
				continue
			}
			out.SetNRGBA(x, y, color.NRGBA{uint8(math.Round(r / a)), uint8(math.Round(g / a)), uint8(math.Round(b / a)), uint8(math.Round(255 * a / total))})
		}
	}
	return out
}

type clip struct {
	name   string
	fps    float64
	loop   bool
	frames []*image.NRGBA
}

func write(root string, c clip, in, out, source string, priority int, feet float64) error {
	dir := filepath.Join(root, c.name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	stale, _ := filepath.Glob(filepath.Join(dir, "[0-9][0-9][0-9][0-9].png"))
	for _, path := range stale {
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	m := emotion.Manifest{Name: c.name, Emotion: c.name, FPS: c.fps, FrameCount: len(c.frames), Loop: c.loop, Priority: priority,
		MouthAnchor: emotion.Anchor{X: math.Round(feet), Y: 160}, TransitionIn: in, TransitionOut: out,
		Canvas: emotion.Canvas{Width: width, Height: height}, GroundY: groundY, FeetX: math.Round(feet*10) / 10, Reviewed: true, SourceSHA256: source}
	for i, f := range c.frames {
		var b bytes.Buffer
		if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&b, f); err != nil {
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
	frames := flag.String("frames", "", "directory of numbered PNG frames extracted from the clip")
	video := flag.String("video", "", "the source clip, recorded by checksum in the manifests")
	name := flag.String("name", "organize", "catalog ID of the scene")
	from := flag.Int("from", 61, "first frame (1-based file number) of the entry")
	hold := flag.Int("hold", 111, "first frame of the loop that holds while the job runs")
	to := flag.Int("to", 150, "last frame of the loop")
	fps := flag.Float64("fps", 20, "playback rate; frames are resampled from 30 per second")
	out := flag.String("out", "../animations", "animations directory to write into")
	sheet := flag.String("sheet", "", "optional contact sheet for review")
	flag.Parse()
	if err := run(*frames, *video, *name, *from, *hold, *to, *fps, *out, *sheet); err != nil {
		fmt.Fprintln(os.Stderr, "import-scene:", err)
		os.Exit(1)
	}
}

func run(dir, video, name string, from, hold, to int, fps float64, out, sheet string) error {
	entry, ok := emotion.Find(name)
	if !ok || !entry.Scene {
		return fmt.Errorf("%q is not a scene in the emotion catalog", name)
	}
	if dir == "" || !(from < hold && hold < to) || fps < 1 || fps > 30 {
		return errors.New("need -frames and from < hold < to with fps in 1..30")
	}
	source := ""
	if video != "" {
		b, err := os.ReadFile(video)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(b)
		source = hex.EncodeToString(digest[:])
	}
	// Resample the source's thirty frames a second to the playback rate.
	pick := func(a, b int) []int {
		var list []int
		for t := 0.0; ; t += 30 / fps {
			i := a + int(math.Round(t))
			if i > b {
				return list
			}
			list = append(list, i)
		}
	}
	entryFrames, loopFrames := pick(from, hold-1), pick(hold, to)
	all := append(append([]int(nil), entryFrames...), loopFrames...)

	raw := make([]*frame, len(all))
	for i, n := range all {
		f, err := readFrame(filepath.Join(dir, fmt.Sprintf("%03d.png", n)))
		if err != nil {
			return err
		}
		raw[i] = f
	}
	ground, err := fitBackdrop(raw)
	if err != nil {
		return err
	}
	mattes := make([]*matte, len(raw))
	var wait sync.WaitGroup
	for i := range raw {
		wait.Add(1)
		go func() {
			defer wait.Done()
			mattes[i] = key(raw[i], ground)
		}()
	}
	wait.Wait()

	// One placement for the whole scene: the camera does not move, so the
	// character's own movement must survive. Fit everything it ever covers.
	union := image.Rectangle{}
	for _, m := range mattes {
		union = union.Union(m.bounds())
	}
	scale := math.Min(404/float64(union.Dx()), (groundY-12)/float64(union.Dy()))
	dx := width/2 - scale*float64(union.Min.X+union.Max.X)/2
	dy := groundY - scale*float64(union.Max.Y)
	feet := mattes[0].feet()*scale + dx
	fmt.Printf("scene spans %v in the clip; scale %.3f; the pet starts with its feet at x=%.1f\n", union, scale, feet)

	placed := make([]*image.NRGBA, len(mattes))
	for i := range mattes {
		wait.Add(1)
		go func() {
			defer wait.Done()
			placed[i] = mattes[i].place(scale, dx, dy)
		}()
	}
	wait.Wait()
	in := placed[:len(entryFrames)]
	loop := placed[len(entryFrames):]
	// The loop runs forwards and back so it never jumps at the seam.
	var held []*image.NRGBA
	held = append(held, loop...)
	for i := len(loop) - 2; i > 0; i-- {
		held = append(held, loop[i])
	}
	// Leaving plays the pick-up backwards: the box goes down, the pet walks back.
	var leave []*image.NRGBA
	for i := len(in) - 1; i >= 0; i-- {
		leave = append(leave, in[i])
	}
	clips := []struct {
		c       clip
		in, out string
	}{
		{clip{name + "_in", fps, false, in}, "", ""},
		{clip{name, fps, true, held}, name + "_in", name + "_out"},
		{clip{name + "_out", fps, false, leave}, "", ""},
	}
	for _, c := range clips {
		if err := write(out, c.c, c.in, c.out, source, entry.Priority, feet); err != nil {
			return err
		}
		fmt.Printf("%-14s %3d frames\n", c.c.name, len(c.c.frames))
	}
	if sheet == "" {
		return nil
	}
	// Review sheet: every fourth frame of the entry and loop on a dark backdrop.
	var cells []*image.NRGBA
	for i := 0; i < len(placed); i += 4 {
		cells = append(cells, placed[i])
	}
	const columns = 8
	cw, ch := width/2, height/2
	page := image.NewRGBA(image.Rect(0, 0, columns*cw, (len(cells)+columns-1)/columns*ch))
	draw.Draw(page, page.Bounds(), image.NewUniform(color.RGBA{60, 72, 96, 255}), image.Point{}, draw.Src)
	for k, cell := range cells {
		for y := 0; y < ch; y++ {
			for x := 0; x < cw; x++ {
				var r, g, b, a int
				for _, d := range [4][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
					c := cell.NRGBAAt(x*2+d[0], y*2+d[1])
					r, g, b, a = r+int(c.R)*int(c.A), g+int(c.G)*int(c.A), b+int(c.B)*int(c.A), a+int(c.A)
				}
				px, py := k%columns*cw+x, k/columns*ch+y
				under := page.RGBAAt(px, py)
				keep := 4*255 - a
				page.SetRGBA(px, py, color.RGBA{uint8((r + int(under.R)*keep) / 1020), uint8((g + int(under.G)*keep) / 1020), uint8((b + int(under.B)*keep) / 1020), 255})
			}
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, page); err != nil {
		return err
	}
	return os.WriteFile(sheet, b.Bytes(), 0644)
}
