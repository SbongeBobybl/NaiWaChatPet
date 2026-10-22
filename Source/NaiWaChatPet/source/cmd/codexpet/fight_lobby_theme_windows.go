//go:build windows

package main

// Shared palette, typography and native drawing surfaces for the compact lobby.

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/draw"
	png "image/png"
	"math"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

// ---------------------------------------------------------------------------
// Palette
// ---------------------------------------------------------------------------

// Warm white, dark ink and restrained gold keep the lobby close to the mascot.
const (
	// Paper.
	lobbyBG       uint32 = 0xFAF9F4
	lobbyBGDeep   uint32 = 0xFAF9F4
	lobbyRail     uint32 = 0xFAF9F4
	lobbyCard     uint32 = 0xFAF9F4
	lobbyCardSoft uint32 = 0xEEEFE8
	lobbyRowSoft  uint32 = 0xF6E8BE
	lobbyWell     uint32 = 0xFFFFFF
	lobbyWellEdge uint32 = 0xC7CCC1
	lobbyHover    uint32 = 0xF0ECD9

	// Ink.
	lobbyInk   uint32 = 0x30382F
	lobbyMuted uint32 = 0x626B5F
	lobbyFaint uint32 = 0x7C8379

	// Rules.
	lobbyLineColor uint32 = 0xCDD2C7
	lobbyLineSoft  uint32 = 0xE1E5DA

	// Honey gold: the mascot's body and the app's action colour.
	lobbyGold      uint32 = 0xE9B537
	lobbyGoldLight uint32 = 0xF2C762
	lobbyGoldDeep  uint32 = 0x8B5A08
	lobbyGoldSoft  uint32 = 0xF8E4AE
	lobbyGoldInk   uint32 = 0x302812

	// Sage: the mascot's eyes and the lily pads.
	lobbySage      uint32 = 0x527653
	lobbySageDeep  uint32 = 0x386149
	lobbySageSoft  uint32 = 0xE7EEDD
	lobbyPadTop    uint32 = 0x6E9153
	lobbyPadBottom uint32 = 0x4B6738

	// Alert, matching the app's existing red.
	lobbyPeach     uint32 = 0xCF4A31
	lobbyClay      uint32 = 0xC0A040
	lobbyWhite     uint32 = 0xFFFFFF
	lobbyShadowInk uint32 = 0x6B5327
)

// Typography scale, in device-independent points before the lobby scale.
const (
	lobbyMicro = 10 // 400 tracking / labels
	lobbyLabel = 11 // 400 captions
	lobbyCap   = 12 // 400 secondary copy
	lobbyBody  = 13 // 400 body copy
	lobbyBody2 = 14 // 400 controls
	lobbySec   = 16 // 600 section headings
	lobbyNum   = 20 // 600 numerals
	lobbyHero  = 24 // 600 hero numeral
	lobbyTitle = 22 // 600 page title
)

func lobbyColor(value uint32) color.NRGBA {
	return color.NRGBA{byte(value >> 16), byte(value >> 8), byte(value), 255}
}

// lobbyRef converts an 0xRRGGBB literal into a GDI COLORREF.
func lobbyRef(value uint32) uintptr { return uintptr((value&255)<<16 | value&0xff00 | value>>16) }

// lobbyTint returns a palette colour with a different alpha (0..255).
func lobbyTint(value uint32, alpha int) color.NRGBA {
	c := lobbyColor(value)
	c.A = byte(max(0, min(255, alpha)))
	return c
}

func lobbyMix(a, b color.NRGBA, t float64) color.NRGBA {
	t = lobbyClamp01(t)
	return color.NRGBA{
		byte(float64(a.R) + (float64(b.R)-float64(a.R))*t),
		byte(float64(a.G) + (float64(b.G)-float64(a.G))*t),
		byte(float64(a.B) + (float64(b.B)-float64(a.B))*t),
		byte(float64(a.A) + (float64(b.A)-float64(a.A))*t),
	}
}

// ---------------------------------------------------------------------------
// Motion helpers
// ---------------------------------------------------------------------------

func lobbyClamp01(t float64) float64 { return math.Max(0, math.Min(1, t)) }

// lobbyEaseOut is a cubic ease-out: fast start, calm settle.
func lobbyEaseOut(t float64) float64 {
	t = lobbyClamp01(t)
	return 1 - math.Pow(1-t, 3)
}

func lobbyEaseInOut(t float64) float64 {
	t = lobbyClamp01(t)
	if t < .5 {
		return 4 * t * t * t
	}
	f := 2*t - 2
	return .5*f*f*f + 1
}

// lobbyEaseOutBack overshoots slightly; the mascot hop uses it.
func lobbyEaseOutBack(t float64) float64 {
	t = lobbyClamp01(t)
	const c1, c3 = 1.70158, 2.70158
	return 1 + c3*math.Pow(t-1, 3) + c1*math.Pow(t-1, 2)
}

// lobbyApproach moves current toward target with frame-rate independent
// exponential smoothing. It is the workhorse for hover and press easing.
func lobbyApproach(current, target, rate, dt float64) float64 {
	if rate <= 0 {
		return target
	}
	k := 1 - math.Exp(-rate*dt)
	v := current + (target-current)*k
	if math.Abs(v-target) < 0.0015 {
		return target
	}
	return v
}

// lobbyStagger maps a clock plus a delay/duration to 0..1 progress.
func lobbyStagger(elapsed, delay, duration float64) float64 {
	if duration <= 0 {
		return 1
	}
	return lobbyClamp01((elapsed - delay) / duration)
}

func lobbySqrt(v float64) float64 {
	if v <= 0 {
		return 0
	}
	return math.Sqrt(v)
}

// ---------------------------------------------------------------------------
// Software rasteriser
// ---------------------------------------------------------------------------

// lobbySDF is the signed distance to a rounded rectangle, negative inside.
func lobbySDF(px, py float64, r image.Rectangle, radius float64) float64 {
	cx := (float64(r.Min.X) + float64(r.Max.X)) / 2
	cy := (float64(r.Min.Y) + float64(r.Max.Y)) / 2
	hx := (float64(r.Max.X) - float64(r.Min.X)) / 2
	hy := (float64(r.Max.Y) - float64(r.Min.Y)) / 2
	radius = math.Max(0, math.Min(radius, math.Min(hx, hy)))
	dx := math.Abs(px-cx) - (hx - radius)
	dy := math.Abs(py-cy) - (hy - radius)
	ax, ay := math.Max(dx, 0), math.Max(dy, 0)
	return lobbySqrt(ax*ax+ay*ay) + math.Min(math.Max(dx, dy), 0) - radius
}

// lobbyPut composites a straight-alpha colour over an NRGBA surface.
func lobbyPut(im *image.NRGBA, x, y int, c color.NRGBA) {
	if c.A == 0 || x < 0 || y < 0 || x >= im.Rect.Dx() || y >= im.Rect.Dy() {
		return
	}
	i := im.PixOffset(x, y)
	if c.A == 255 {
		im.Pix[i], im.Pix[i+1], im.Pix[i+2], im.Pix[i+3] = c.R, c.G, c.B, 255
		return
	}
	// The animated ponds are opaque and take this integer path.
	if im.Pix[i+3] == 255 {
		a := int(c.A)
		ia := 255 - a
		im.Pix[i] = byte((int(c.R)*a + int(im.Pix[i])*ia) / 255)
		im.Pix[i+1] = byte((int(c.G)*a + int(im.Pix[i+1])*ia) / 255)
		im.Pix[i+2] = byte((int(c.B)*a + int(im.Pix[i+2])*ia) / 255)
		return
	}
	sa := float64(c.A) / 255
	da := float64(im.Pix[i+3]) / 255
	oa := sa + da*(1-sa)
	if oa <= 0 {
		return
	}
	inv := 1 / oa
	im.Pix[i] = byte((float64(c.R)*sa + float64(im.Pix[i])*da*(1-sa)) * inv)
	im.Pix[i+1] = byte((float64(c.G)*sa + float64(im.Pix[i+1])*da*(1-sa)) * inv)
	im.Pix[i+2] = byte((float64(c.B)*sa + float64(im.Pix[i+2])*da*(1-sa)) * inv)
	im.Pix[i+3] = byte(oa * 255)
}

// lobbyFill paints an anti-aliased rounded rectangle onto an NRGBA surface.
func lobbyFill(im *image.NRGBA, r image.Rectangle, radius float64, c color.NRGBA) {
	if c.A == 0 {
		return
	}
	area := r.Intersect(im.Bounds())
	if radius <= 0 {
		op := draw.Over
		if c.A == 255 {
			op = draw.Src
		}
		draw.Draw(im, area, image.NewUniform(c), image.Point{}, op)
		return
	}
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			cov := .5 - lobbySDF(float64(x)+.5, float64(y)+.5, r, radius)
			if cov <= 0 {
				continue
			}
			paint := c
			if cov < 1 {
				paint.A = byte(float64(c.A) * cov)
			}
			lobbyPut(im, x, y, paint)
		}
	}
}

// lobbyFillVGradient paints a rounded rectangle with a vertical gradient.
func lobbyFillVGradient(im *image.NRGBA, r image.Rectangle, radius float64, top, bottom color.NRGBA) {
	area := r.Intersect(im.Bounds())
	h := math.Max(1, float64(r.Dy())-1)
	for y := area.Min.Y; y < area.Max.Y; y++ {
		row := lobbyMix(top, bottom, float64(y-r.Min.Y)/h)
		for x := area.Min.X; x < area.Max.X; x++ {
			cov := .5 - lobbySDF(float64(x)+.5, float64(y)+.5, r, radius)
			if cov <= 0 {
				continue
			}
			paint := row
			if cov < 1 {
				paint.A = byte(float64(row.A) * cov)
			}
			lobbyPut(im, x, y, paint)
		}
	}
}

// lobbyShadow paints a soft drop shadow behind a rounded rectangle.
func lobbyShadow(im *image.NRGBA, r image.Rectangle, radius, blur float64, c color.NRGBA) {
	if blur <= 0 || c.A == 0 {
		return
	}
	b := int(math.Ceil(blur))
	area := image.Rect(r.Min.X-b, r.Min.Y-b, r.Max.X+b+1, r.Max.Y+b+1).Intersect(im.Bounds())
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			d := lobbySDF(float64(x)+.5, float64(y)+.5, r, radius)
			if d >= blur {
				continue
			}
			k := lobbyClamp01(1 - d/blur)
			k = k * k * (3 - 2*k)
			if k <= .003 {
				continue
			}
			paint := c
			paint.A = byte(float64(c.A) * k)
			lobbyPut(im, x, y, paint)
		}
	}
}

// lobbyStroke paints an anti-aliased line segment.
func lobbyStroke(im *image.NRGBA, x1, y1, x2, y2, width float64, c color.NRGBA) {
	dx, dy := x2-x1, y2-y1
	length := dx*dx + dy*dy
	minX := int(math.Min(x1, x2) - width - 1)
	maxX := int(math.Max(x1, x2) + width + 1)
	minY := int(math.Min(y1, y2) - width - 1)
	maxY := int(math.Max(y1, y2) + width + 1)
	for y := max(0, minY); y < min(im.Rect.Dy(), maxY); y++ {
		for x := max(0, minX); x < min(im.Rect.Dx(), maxX); x++ {
			t := 0.0
			if length > 0 {
				t = lobbyClamp01(((float64(x)+.5-x1)*dx + (float64(y)+.5-y1)*dy) / length)
			}
			qx, qy := float64(x)+.5-x1-t*dx, float64(y)+.5-y1-t*dy
			a := lobbyClamp01(width/2 + .5 - lobbySqrt(qx*qx+qy*qy))
			if a <= 0 {
				continue
			}
			paint := c
			paint.A = byte(float64(c.A) * a)
			lobbyPut(im, x, y, paint)
		}
	}
}

// lobbyRing paints an anti-aliased circle outline. It walks the annulus, not the
// whole disc, because thin rings are drawn several times per frame.
func lobbyRing(im *image.NRGBA, cx, cy, radius, width float64, c color.NRGBA) {
	if c.A == 0 || radius <= 0 {
		return
	}
	half := width/2 + 1
	outer := radius + half
	yMin := max(0, int(cy-outer))
	yMax := min(im.Rect.Dy()-1, int(cy+outer))
	for y := yMin; y <= yMax; y++ {
		dy := float64(y) + .5 - cy
		h2 := radius*radius - dy*dy
		if h2 <= 0 {
			continue
		}
		root := math.Sqrt(h2)
		for _, centre := range [2]float64{cx - root, cx + root} {
			x0 := max(0, int(centre-half))
			x1 := min(im.Rect.Dx()-1, int(centre+half)+1)
			for x := x0; x <= x1; x++ {
				dx, dy2 := float64(x)+.5-cx, float64(y)+.5-cy
				d := math.Abs(lobbySqrt(dx*dx+dy2*dy2) - radius)
				a := lobbyClamp01(width/2 + .5 - d)
				if a <= 0 {
					continue
				}
				paint := c
				paint.A = byte(float64(c.A) * a)
				lobbyPut(im, x, y, paint)
			}
		}
	}
}

// lobbyArc paints a stroked arc, used by the search radar and countdown ring.
func lobbyArc(im *image.NRGBA, cx, cy, radius, width, from, sweep float64, c color.NRGBA) {
	if c.A == 0 || radius <= 0 {
		return
	}
	half := width/2 + 1
	outer := radius + half
	yMin := max(0, int(cy-outer))
	yMax := min(im.Rect.Dy()-1, int(cy+outer))
	for y := yMin; y <= yMax; y++ {
		dy := float64(y) + .5 - cy
		h2 := radius*radius - dy*dy
		if h2 <= 0 {
			continue
		}
		root := math.Sqrt(h2)
		for _, centre := range [2]float64{cx - root, cx + root} {
			x0 := max(0, int(centre-half))
			x1 := min(im.Rect.Dx()-1, int(centre+half)+1)
			for x := x0; x <= x1; x++ {
				dx := float64(x) + .5 - cx
				d := math.Abs(lobbySqrt(dx*dx+dy*dy) - radius)
				a := lobbyClamp01(width/2 + .5 - d)
				if a <= 0 {
					continue
				}
				ang := math.Atan2(dy, dx)
				delta := math.Mod(ang-from+4*math.Pi, 2*math.Pi)
				if delta > sweep {
					continue
				}
				if delta > sweep-.10 || delta < .10 {
					a *= .5
				}
				paint := c
				paint.A = byte(float64(c.A) * a)
				lobbyPut(im, x, y, paint)
			}
		}
	}
}

// lobbyDisc paints a soft filled circle with a radial falloff.
func lobbyDisc(im *image.NRGBA, cx, cy, radius, softness float64, c color.NRGBA) {
	if c.A == 0 || radius <= 0 {
		return
	}
	outer := radius + softness
	for y := max(0, int(cy-outer)); y < min(im.Rect.Dy(), int(cy+outer)+1); y++ {
		for x := max(0, int(cx-outer)); x < min(im.Rect.Dx(), int(cx+outer)+1); x++ {
			dx, dy := float64(x)+.5-cx, float64(y)+.5-cy
			d := lobbySqrt(dx*dx + dy*dy)
			var a float64
			switch {
			case d <= radius-softness:
				a = 1
			case d >= radius:
				continue
			default:
				a = (radius - d) / math.Max(softness, .001)
				a = a * a * (3 - 2*a)
			}
			paint := c
			paint.A = byte(float64(c.A) * a)
			lobbyPut(im, x, y, paint)
		}
	}
}

// lobbyLilyPad paints the notched lily pad the mascot stands on.
func lobbyLilyPad(im *image.NRGBA, cx, cy, rx, ry, notch float64, top, bottom color.NRGBA) {
	outer := math.Max(rx, ry) + 2
	for y := max(0, int(cy-outer)); y < min(im.Rect.Dy(), int(cy+outer)+1); y++ {
		for x := max(0, int(cx-outer)); x < min(im.Rect.Dx(), int(cx+outer)+1); x++ {
			px, py := float64(x)+.5-cx, float64(y)+.5-cy
			d := lobbySqrt((px/rx)*(px/rx) + (py/ry)*(py/ry))
			if d >= 1 {
				continue
			}
			// One narrow radial slit, angled away from the mascot's feet.
			if wedge := math.Mod(math.Atan2(py, px)-0.95+math.Pi, 2*math.Pi); math.Abs(wedge-math.Pi) < notch/2 {
				continue
			}
			a := lobbyClamp01((1 - d) * math.Min(rx, ry) * .9)
			if a <= 0 {
				continue
			}
			shade := lobbyMix(top, bottom, lobbyClamp01(d*.8+(py/ry)*.35+.25))
			shade.A = byte(float64(shade.A) * a)
			lobbyPut(im, x, y, shade)
		}
	}
}

// ---------------------------------------------------------------------------
// GDI surfaces
// ---------------------------------------------------------------------------

var (
	lobbyMSImg      = syscall.NewLazyDLL("msimg32.dll")
	lobbyAlphaBlend = lobbyMSImg.NewProc("AlphaBlend")
	lobbyCreateDC   = chatGDI.NewProc("CreateCompatibleDC")
	lobbyCreateDIB  = chatGDI.NewProc("CreateDIBSection")
	lobbyDeleteDC   = chatGDI.NewProc("DeleteDC")
	lobbyStretchBlt = chatGDI.NewProc("StretchDIBits")
	lobbyBlitMode   = chatGDI.NewProc("SetStretchBltMode")
	lobbyBrushOrg   = chatGDI.NewProc("SetBrushOrgEx")
	lobbyFlush      = chatGDI.NewProc("GdiFlush")
)

const (
	lobbyHalftone     = 4
	lobbyColorOnColor = 3
	lobbySRCCopy      = 0x00CC0020
)

type lobbyBlendFunction struct {
	BlendOp, BlendFlags, SourceConstantAlpha, AlphaFormat byte
}

// lobbySurface owns a 32-bit top-down DIB plus the memory DC GDI needs to blend
// it. Pixel data is written straight into the DIB, so animating costs no copies.
type lobbySurface struct {
	width, height int
	dc, bitmap    uintptr
	bitmapOld     uintptr
	pixels        []byte
}

func newLobbySurface(width, height int) *lobbySurface {
	width, height = max(1, width), max(1, height)
	s := &lobbySurface{width: width, height: height}
	s.dc, _ = call(lobbyCreateDC, 0)
	if s.dc == 0 {
		return nil
	}
	header := skinDIBHeader{Size: 40, Width: int32(width), Height: -int32(height), Planes: 1, Bits: 32}
	var bits uintptr
	s.bitmap, _ = call(lobbyCreateDIB, s.dc, uintptr(unsafe.Pointer(&header)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	runtime.KeepAlive(header)
	if s.bitmap == 0 || bits == 0 {
		call(lobbyDeleteDC, s.dc)
		s.dc = 0
		return nil
	}
	s.bitmapOld, _ = call(chatSelectObject, s.dc, s.bitmap)
	if s.bitmapOld == 0 || s.bitmapOld == ^uintptr(0) {
		call(chatDeleteObject, s.bitmap)
		call(lobbyDeleteDC, s.dc)
		return nil
	}
	s.pixels = unsafe.Slice((*byte)(unsafe.Pointer(bits)), width*height*4)
	return s
}

func (s *lobbySurface) close() {
	if s == nil {
		return
	}
	call(lobbyFlush)
	if s.dc != 0 {
		if s.bitmapOld != 0 {
			call(chatSelectObject, s.dc, s.bitmapOld)
		}
		call(lobbyDeleteDC, s.dc)
		s.dc = 0
	}
	if s.bitmap != 0 {
		call(chatDeleteObject, s.bitmap)
		s.bitmap = 0
	}
	s.pixels = nil
}

// upload premultiplies an NRGBA image into the DIB so AlphaBlend can use it.
func (s *lobbySurface) upload(im *image.NRGBA) {
	if s == nil || len(s.pixels) == 0 {
		return
	}
	call(lobbyFlush)
	clear(s.pixels)
	if im == nil {
		return
	}
	sw := min(s.width, im.Rect.Dx())
	sh := min(s.height, im.Rect.Dy())
	for y := 0; y < sh; y++ {
		row := y * im.Stride
		out := y * s.width * 4
		for x := 0; x < sw; x++ {
			i := row + x*4
			r, g, b, a := im.Pix[i], im.Pix[i+1], im.Pix[i+2], im.Pix[i+3]
			if a != 255 && a != 0 {
				r = byte(int(r) * int(a) / 255)
				g = byte(int(g) * int(a) / 255)
				b = byte(int(b) * int(a) / 255)
			}
			o := out + x*4
			s.pixels[o], s.pixels[o+1], s.pixels[o+2], s.pixels[o+3] = b, g, r, a
		}
	}
}

// copyOpaque writes an opaque NRGBA buffer into the DIB without premultiplying.
// The animated ponds are fully opaque, so this is the cheapest upload path.
func (s *lobbySurface) copyOpaque(im *image.NRGBA) {
	if s == nil || len(s.pixels) == 0 || im == nil {
		return
	}
	call(lobbyFlush)
	sw := min(s.width, im.Rect.Dx())
	sh := min(s.height, im.Rect.Dy())
	for y := 0; y < sh; y++ {
		src := y * im.Stride
		out := y * s.width * 4
		for x := 0; x < sw; x++ {
			i := src + x*4
			o := out + x*4
			s.pixels[o], s.pixels[o+1], s.pixels[o+2], s.pixels[o+3] = im.Pix[i+2], im.Pix[i+1], im.Pix[i], 255
		}
	}
}

// pack returns the BLENDFUNCTION as a single register-sized value. AlphaBlend
// takes it by value, not by pointer.
func (b lobbyBlendFunction) pack() uintptr {
	return uintptr(b.BlendOp) | uintptr(b.BlendFlags)<<8 | uintptr(b.SourceConstantAlpha)<<16 | uintptr(b.AlphaFormat)<<24
}

// paint copies the surface onto a device context. Ponds are opaque, so BitBlt
// is both correct and much cheaper than an alpha blend.
func (s *lobbySurface) paint(dst uintptr, x, y int) {
	if s == nil || s.dc == 0 || dst == 0 {
		return
	}
	call(skinBitBlt, dst, uintptr(int32(x)), uintptr(int32(y)), uintptr(s.width), uintptr(s.height), s.dc, 0, 0, lobbySRCCopy)
}

// blend composites the surface onto a device context at the given position.
func (s *lobbySurface) blend(dst uintptr, x, y int, alpha int) {
	if s == nil || s.dc == 0 || dst == 0 {
		return
	}
	bf := lobbyBlendFunction{SourceConstantAlpha: byte(max(0, min(255, alpha))), AlphaFormat: 1}
	call(lobbyAlphaBlend, dst, uintptr(int32(x)), uintptr(int32(y)), uintptr(s.width), uintptr(s.height),
		s.dc, 0, 0, uintptr(s.width), uintptr(s.height), bf.pack())
}

// stretchOpaque scales an opaque DIB up to the window with smooth filtering.
func (s *lobbySurface) stretchOpaque(dst uintptr, width, height int) {
	if s == nil || s.dc == 0 || dst == 0 || len(s.pixels) == 0 {
		return
	}
	old, _ := call(lobbyBlitMode, dst, lobbyColorOnColor)
	header := skinDIBHeader{Size: 40, Width: int32(s.width), Height: -int32(s.height), Planes: 1, Bits: 32}
	call(lobbyStretchBlt, dst, 0, 0, uintptr(int32(width)), uintptr(int32(height)),
		0, 0, uintptr(int32(s.width)), uintptr(int32(s.height)),
		uintptr(unsafe.Pointer(&s.pixels[0])), uintptr(unsafe.Pointer(&header)), 0, lobbySRCCopy)
	runtime.KeepAlive(header)
	call(lobbyBlitMode, dst, old)
}

// ---------------------------------------------------------------------------
// Mascot artwork
// ---------------------------------------------------------------------------

//go:embed ui_assets/lobby_naiwa.png
var lobbyMascotPNG []byte

var (
	lobbyMascotOnce  sync.Once
	lobbyMascotSrc   *image.NRGBA
	lobbyMascotCache = map[int]*image.NRGBA{}
	lobbyMascotMu    sync.Mutex
)

// lobbyMascotImage loads the released mascot artwork exactly once.
func lobbyMascotImage() *image.NRGBA {
	lobbyMascotOnce.Do(func() {
		decoded, err := png.Decode(bytes.NewReader(lobbyMascotPNG))
		if err != nil {
			lobbyMascotSrc = image.NewNRGBA(image.Rect(0, 0, 1, 1))
			return
		}
		b := decoded.Bounds()
		out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		for y := 0; y < b.Dy(); y++ {
			for x := 0; x < b.Dx(); x++ {
				out.Set(x, y, decoded.At(b.Min.X+x, b.Min.Y+y))
			}
		}
		lobbyMascotSrc = out
	})
	return lobbyMascotSrc
}

// lobbyMascotScaled resizes the mascot to a pixel height and caches the result.
func lobbyMascotScaled(height int) *image.NRGBA {
	height = max(8, min(720, height))
	lobbyMascotMu.Lock()
	defer lobbyMascotMu.Unlock()
	if cached, ok := lobbyMascotCache[height]; ok {
		return cached
	}
	src := lobbyMascotImage()
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	width := max(1, int(math.Round(float64(sw)*float64(height)/float64(sh))))
	out := resamplePremultiplied(src, width, height)
	if len(lobbyMascotCache) > 14 {
		lobbyMascotCache = map[int]*image.NRGBA{}
	}
	lobbyMascotCache[height] = out
	return out
}

// resamplePremultiplied performs a box-filtered resize in premultiplied space so
// edges keep their soft alpha without dark halos.
func resamplePremultiplied(src *image.NRGBA, width, height int) *image.NRGBA {
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	out := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		y0 := y * sh / height
		y1 := max(y0+1, (y+1)*sh/height)
		for x := 0; x < width; x++ {
			x0 := x * sw / width
			x1 := max(x0+1, (x+1)*sw/width)
			var sr, sg, sb, sa, n float64
			for sy := y0; sy < y1; sy++ {
				base := sy*src.Stride + x0*4
				for sx := x0; sx < x1; sx++ {
					a := float64(src.Pix[base+3]) / 255
					sr += float64(src.Pix[base]) * a
					sg += float64(src.Pix[base+1]) * a
					sb += float64(src.Pix[base+2]) * a
					sa += a
					base += 4
					n++
				}
			}
			if n == 0 || sa <= 0 {
				continue
			}
			o := y*out.Stride + x*4
			out.Pix[o] = byte(clamp255(sr / sa))
			out.Pix[o+1] = byte(clamp255(sg / sa))
			out.Pix[o+2] = byte(clamp255(sb / sa))
			out.Pix[o+3] = byte(clamp255(sa / n * 255))
		}
	}
	return out
}

func clamp255(v float64) float64 { return math.Max(0, math.Min(255, v)) }

// blitOpaque draws a straight-alpha sprite over an opaque NRGBA surface with
// integer blending, which keeps the animated ponds cheap.
func blitOpaque(dst *image.NRGBA, src *image.NRGBA, x, y, alpha int) {
	if src == nil || dst == nil || alpha <= 0 {
		return
	}
	dw, dh := dst.Rect.Dx(), dst.Rect.Dy()
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	for sy := 0; sy < sh; sy++ {
		dy := y + sy
		if dy < 0 || dy >= dh {
			continue
		}
		for sx := 0; sx < sw; sx++ {
			dx := x + sx
			if dx < 0 || dx >= dw {
				continue
			}
			si := sy*src.Stride + sx*4
			a := int(src.Pix[si+3]) * alpha / 255
			if a <= 0 {
				continue
			}
			di := dy*dst.Stride + dx*4
			if a >= 255 {
				dst.Pix[di], dst.Pix[di+1], dst.Pix[di+2], dst.Pix[di+3] = src.Pix[si], src.Pix[si+1], src.Pix[si+2], 255
				continue
			}
			ia := 255 - a
			dst.Pix[di] = byte((int(src.Pix[si])*a + int(dst.Pix[di])*ia) / 255)
			dst.Pix[di+1] = byte((int(src.Pix[si+1])*a + int(dst.Pix[di+1])*ia) / 255)
			dst.Pix[di+2] = byte((int(src.Pix[si+2])*a + int(dst.Pix[di+2])*ia) / 255)
			dst.Pix[di+3] = 255
		}
	}
}
