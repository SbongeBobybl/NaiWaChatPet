//go:build windows

package main

// The lobby is drawn rather than composed from themed native widgets, so every
// moving part lives here: the clock that drives the window timer, the drifting
// ambient backdrop, the little ponds the mascot stands in, and the eased motion
// values that make hover, press and page changes feel physical.

import (
	"image"
	"image/color"
	"math"
	"time"
)

const (
	lobbyTimerID      = 2
	lobbyFrameMillis  = 33 // 30 fps: smooth, and cheap enough for a desktop pet
	lobbyIdleMillis   = 90 // slow down when the window is not focused
	lobbyAmbientDiv   = 2  // the backdrop renders at half resolution and is scaled
	lobbyEntranceMS   = 420
	lobbyEntranceStep = 70
)

// lobbyAnim keeps all time-varying lobby state in one place.
type lobbyAnim struct {
	started   time.Time
	now       float64 // seconds since the lobby opened
	dt        float64 // seconds since the previous frame
	last      time.Time
	lastInput time.Time
	pageStart float64
	page      int
	ambient   *lobbySurface
	ambientDX [3][]float64
	ponds     map[string]*lobbyPond
	hoverKey  string
}

// lobbyPond is a small opaque animation stage: a cached static base plus a
// working buffer that is repainted every frame and alpha-blended into place.
type lobbyPond struct {
	key     string
	width   int
	height  int
	base    *image.NRGBA
	work    *image.NRGBA
	surface *lobbySurface
	x, y    int
}

func newLobbyPond(key string, width, height int, background uint32, build func(*image.NRGBA)) *lobbyPond {
	width, height = max(8, width), max(8, height)
	p := &lobbyPond{key: key, width: width, height: height}
	p.base = image.NewNRGBA(image.Rect(0, 0, width, height))
	p.work = image.NewNRGBA(image.Rect(0, 0, width, height))
	fill := lobbyColor(background)
	for i := 0; i < len(p.base.Pix); i += 4 {
		p.base.Pix[i], p.base.Pix[i+1], p.base.Pix[i+2], p.base.Pix[i+3] = fill.R, fill.G, fill.B, 255
	}
	if build != nil {
		build(p.base)
	}
	copy(p.work.Pix, p.base.Pix)
	p.surface = newLobbySurface(width, height)
	return p
}

func (p *lobbyPond) reset() { copy(p.work.Pix, p.base.Pix) }

func (p *lobbyPond) close() {
	if p == nil {
		return
	}
	if p.surface != nil {
		p.surface.close()
		p.surface = nil
	}
	p.base, p.work = nil, nil
}

// present converts the working buffer into the DIB and copies it into the window.
func (p *lobbyPond) present(dc uintptr) {
	if p == nil || p.surface == nil || dc == 0 {
		return
	}
	p.surface.copyOpaque(p.work)
	p.surface.paint(dc, p.x, p.y)
}

// ---------------------------------------------------------------------------
// Clock
// ---------------------------------------------------------------------------

func (g *gamePanel) lobbyNow() float64 {
	a := g.anim
	if a == nil || a.started.IsZero() {
		return 0
	}
	return time.Since(a.started).Seconds()
}

func (g *gamePanel) ensureAnim() *lobbyAnim {
	if g.anim == nil {
		g.anim = &lobbyAnim{ponds: map[string]*lobbyPond{}}
	}
	return g.anim
}

// startLobbyClock prepares the view without a permanent animation timer.
// Stable cards and controls repaint only when data or pointer state changes.
func (g *gamePanel) startLobbyClock() {
	a := g.ensureAnim()
	if a.started.IsZero() {
		a.started = time.Now()
	}
	a.last = time.Now()
	g.stopLobbyClock()
	g.navPillY = float64(g.skin.navTargetY)
	g.navPillReady = true
}

func (g *gamePanel) stopLobbyClock() {
	if g.hwnd == 0 || !g.lobbyTimerRunning {
		return
	}
	g.lobbyTimerRunning = false
	call(killTimer, g.hwnd, lobbyTimerID)
}

// noteLobbyInput records user activity so the frame rate can idle down later.
func (g *gamePanel) noteLobbyInput() {
	if g.anim != nil {
		g.anim.lastInput = time.Now()
	}
}

// A previously queued native timer may arrive after it was killed.
func (g *gamePanel) lobbyTick() { g.stopLobbyClock() }

// ---------------------------------------------------------------------------
// Ambient backdrop
// ---------------------------------------------------------------------------

func (g *gamePanel) ensureAmbient(width, height int) *lobbySurface {
	a := g.ensureAnim()
	w := max(2, (width+lobbyAmbientDiv-1)/lobbyAmbientDiv)
	h := max(2, (height+lobbyAmbientDiv-1)/lobbyAmbientDiv)
	if a.ambient != nil && a.ambient.width == w && a.ambient.height == h {
		return a.ambient
	}
	if a.ambient != nil {
		a.ambient.close()
	}
	a.ambient = newLobbySurface(w, h)
	return a.ambient
}

// renderAmbient paints the slow aurora plus rising bubbles. It writes BGRA
// straight into the small DIB, so the whole backdrop costs well under a
// millisecond at this resolution.
func (g *gamePanel) renderAmbient(now float64) {
	s := g.anim.ambient
	if s == nil || len(s.pixels) == 0 {
		return
	}
	w, h := s.width, s.height
	pixels := s.pixels
	top := lobbyColor(lobbyBG)
	bottom := lobbyColor(lobbyBGDeep)
	// Only colours the rest of the app already uses: the mascot's honey body,
	// the apricot of warm skin, and the olive of her eyes.
	honey := color.NRGBA{232, 168, 40, 255}
	apricot := color.NRGBA{234, 158, 92, 255}
	sage := color.NRGBA{114, 150, 84, 255}

	// Three large, very slow blobs drift on different Lissajous paths.
	type blob struct {
		x, y, radius, weight float64
		c                    color.NRGBA
	}
	blobs := [3]blob{
		{0.30 + 0.10*math.Sin(now*0.11), 0.18 + 0.07*math.Sin(now*0.09+1.1), 0.62, 0.24, honey},
		{0.78 + 0.08*math.Sin(now*0.08+2.4), 0.72 + 0.08*math.Cos(now*0.12), 0.52, 0.20, apricot},
		{0.14 + 0.07*math.Cos(now*0.07+0.6), 0.86 + 0.05*math.Sin(now*0.10+3.1), 0.46, 0.15, sage},
	}
	aspect := float64(w) / float64(h)
	invW := 1 / float64(w)
	invH := 1 / float64(h)

	// Horizontal offsets only depend on x, so they are computed once per frame.
	a := g.anim
	for i := range blobs {
		if cap(a.ambientDX[i]) < w {
			a.ambientDX[i] = make([]float64, w)
		}
		dx := a.ambientDX[i][:w]
		for x := 0; x < w; x++ {
			d := ((float64(x)+.5)*invW - blobs[i].x) * aspect
			dx[x] = d * d
		}
	}

	for y := 0; y < h; y++ {
		v := (float64(y) + .5) * invH
		base := lobbyMix(top, bottom, lobbyClamp01(v*0.9))
		br, bg, bb := float64(base.R), float64(base.G), float64(base.B)
		var rowDy [3]float64
		var rowR2 [3]float64
		for i := range blobs {
			dy := v - blobs[i].y
			rowDy[i] = dy * dy
			rowR2[i] = blobs[i].radius * blobs[i].radius
		}
		d0, d1, d2 := a.ambientDX[0][:w], a.ambientDX[1][:w], a.ambientDX[2][:w]
		out := y * w * 4
		for x := 0; x < w; x++ {
			r, gr, bl := br, bg, bb
			if d2v := d0[x] + rowDy[0]; d2v < rowR2[0] {
				k := 1 - d2v/rowR2[0]
				k = k * k * blobs[0].weight
				r += (float64(blobs[0].c.R) - r) * k
				gr += (float64(blobs[0].c.G) - gr) * k
				bl += (float64(blobs[0].c.B) - bl) * k
			}
			if d2v := d1[x] + rowDy[1]; d2v < rowR2[1] {
				k := 1 - d2v/rowR2[1]
				k = k * k * blobs[1].weight
				r += (float64(blobs[1].c.R) - r) * k
				gr += (float64(blobs[1].c.G) - gr) * k
				bl += (float64(blobs[1].c.B) - bl) * k
			}
			if d2v := d2[x] + rowDy[2]; d2v < rowR2[2] {
				k := 1 - d2v/rowR2[2]
				k = k * k * blobs[2].weight
				r += (float64(blobs[2].c.R) - r) * k
				gr += (float64(blobs[2].c.G) - gr) * k
				bl += (float64(blobs[2].c.B) - bl) * k
			}
			o := out + x*4
			pixels[o], pixels[o+1], pixels[o+2], pixels[o+3] = byte(bl), byte(gr), byte(r), 255
		}
	}

	// Bubbles: soft bokeh rising with gentle parallax.
	const bubbleCount = 26
	for i := 0; i < bubbleCount; i++ {
		seed := float64(i) * 2.399963
		speed := 0.018 + 0.02*math.Abs(math.Sin(seed*1.7))
		cycle := math.Mod(now*speed+float64(i)/bubbleCount, 1.0)
		bx := (0.5 + 0.5*math.Sin(seed+cycle*3.1)) * float64(w)
		by := (1 - cycle) * float64(h)
		radius := 1.2 + 2.6*math.Abs(math.Cos(seed*2.3)) + 1.6*cycle
		alpha := int(70 * math.Sin(math.Pi*lobbyClamp01(cycle)))
		if alpha <= 3 {
			continue
		}
		drawAmbientBubble(pixels, w, h, bx, by, radius, alpha)
	}
}

func drawAmbientBubble(pixels []byte, w, h int, cx, cy, radius float64, alpha int) {
	minX, maxX := max(0, int(cx-radius)), min(w-1, int(cx+radius))
	minY, maxY := max(0, int(cy-radius)), min(h-1, int(cy+radius))
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			dx, dy := float64(x)+.5-cx, float64(y)+.5-cy
			d := math.Sqrt(dx*dx + dy*dy)
			if d >= radius {
				continue
			}
			k := math.Pow(1-d/radius, 1.6)
			a := int(float64(alpha) * k)
			if a <= 0 {
				continue
			}
			i := (y*w + x) * 4
			ia := 255 - a
			pixels[i] = byte((236*a + int(pixels[i])*ia) / 255)
			pixels[i+1] = byte((252*a + int(pixels[i+1])*ia) / 255)
			pixels[i+2] = byte((255*a + int(pixels[i+2])*ia) / 255)
		}
	}
}

// ---------------------------------------------------------------------------
// Ponds
// ---------------------------------------------------------------------------

// pondFor returns a sized pond, rebuilding the cache if the requested size or
// background changed. Static artwork is baked into the base once, so a frame only
// paints what actually moves.
func (g *gamePanel) pondFor(key string, width, height int, background uint32, build func(*image.NRGBA)) *lobbyPond {
	a := g.ensureAnim()
	if p, ok := a.ponds[key]; ok && p.width == width && p.height == height && p.base != nil {
		return p
	}
	if p, ok := a.ponds[key]; ok {
		p.close()
		delete(a.ponds, key)
	}
	p := newLobbyPond(key, width, height, background, build)
	a.ponds[key] = p
	return p
}

func (g *gamePanel) releasePonds() {
	if g.anim == nil {
		return
	}
	for key, p := range g.anim.ponds {
		p.close()
		delete(g.anim.ponds, key)
	}
	if g.anim.ambient != nil {
		g.anim.ambient.close()
		g.anim.ambient = nil
	}
}

// drawRipples paints expanding rings from a focus point. Each ring starts small
// and fades out as it grows, like a drop landing in the pond.
func drawRipples(im *image.NRGBA, cx, cy, maxRadius, period float64, count int, phase float64, c color.NRGBA) {
	for i := 0; i < count; i++ {
		k := math.Mod(phase/period+float64(i)/float64(count), 1.0)
		r := maxRadius * lobbyEaseOut(k)
		a := int(float64(c.A) * (1 - k) * (1 - k))
		if a <= 4 || r < 1 {
			continue
		}
		paint := c
		paint.A = byte(a)
		lobbyRing(im, cx, cy, r, 1.15, paint)
	}
}

// mascotInto blits the mascot onto an opaque surface with the given centre.
func mascotInto(im *image.NRGBA, height int, cx, bottom int, alpha int) *image.NRGBA {
	sprite := lobbyMascotScaled(height)
	if sprite == nil {
		return nil
	}
	w, h := sprite.Rect.Dx(), sprite.Rect.Dy()
	blitOpaque(im, sprite, cx-w/2, bottom-h, alpha)
	return sprite
}
