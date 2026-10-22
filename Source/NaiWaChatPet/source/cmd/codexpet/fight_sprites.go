package main

import (
	"image"
	"math"
	"strconv"
)

// capitals complete the pixel face for the arena's announcements.
var capitals = map[rune][7]string{
	'A': {"01110", "10001", "10001", "11111", "10001", "10001", "10001"},
	'B': {"11110", "10001", "10001", "11110", "10001", "10001", "11110"},
	'C': {"01110", "10001", "10000", "10000", "10000", "10001", "01110"},
	'D': {"11110", "10001", "10001", "10001", "10001", "10001", "11110"},
	'E': {"11111", "10000", "10000", "11110", "10000", "10000", "11111"},
	'F': {"11111", "10000", "10000", "11110", "10000", "10000", "10000"},
	'G': {"01110", "10001", "10000", "10111", "10001", "10001", "01111"},
	'H': {"10001", "10001", "10001", "11111", "10001", "10001", "10001"},
	'I': {"01110", "00100", "00100", "00100", "00100", "00100", "01110"},
	'J': {"00111", "00010", "00010", "00010", "00010", "10010", "01100"},
	'K': {"10001", "10010", "10100", "11000", "10100", "10010", "10001"},
	'L': {"10000", "10000", "10000", "10000", "10000", "10000", "11111"},
	'M': {"10001", "11011", "10101", "10101", "10001", "10001", "10001"},
	'N': {"10001", "10001", "11001", "10101", "10011", "10001", "10001"},
	'O': {"01110", "10001", "10001", "10001", "10001", "10001", "01110"},
	'P': {"11110", "10001", "10001", "11110", "10000", "10000", "10000"},
	'Q': {"01110", "10001", "10001", "10001", "10101", "10010", "01101"},
	'R': {"11110", "10001", "10001", "11110", "10100", "10010", "10001"},
	'S': {"01111", "10000", "10000", "01110", "00001", "00001", "11110"},
	'T': {"11111", "00100", "00100", "00100", "00100", "00100", "00100"},
	'U': {"10001", "10001", "10001", "10001", "10001", "10001", "01110"},
	'V': {"10001", "10001", "10001", "10001", "10001", "01010", "00100"},
	'W': {"10001", "10001", "10001", "10101", "10101", "10101", "01010"},
	'X': {"10001", "10001", "01010", "00100", "01010", "10001", "10001"},
	'Y': {"10001", "10001", "01010", "00100", "00100", "00100", "00100"},
	'Z': {"11111", "00001", "00010", "00100", "01000", "10000", "11111"},
	'!': {"00100", "00100", "00100", "00100", "00100", "00000", "00100"},
	'.': {"00000", "00000", "00000", "00000", "00000", "01100", "01100"},
	':': {"00000", "01100", "01100", "00000", "01100", "01100", "00000"},
}

func init() {
	for r, g := range capitals {
		glyphs[r] = g
	}
}

// bannerStyle colours an announcement: a top-to-bottom fill and an outline.
type bannerStyle struct{ top, bottom, edge [3]float64 }

var (
	fireBanner = bannerStyle{[3]float64{1, .96, .55}, [3]float64{1, .34, .05}, [3]float64{.32, .04, .02}}
	goldBanner = bannerStyle{[3]float64{1, 1, .85}, [3]float64{1, .72, .12}, [3]float64{.22, .12, .02}}
	iceBanner  = bannerStyle{[3]float64{1, 1, 1}, [3]float64{.35, .75, 1}, [3]float64{.02, .10, .26}}
)

// bannerSprite renders arcade lettering: slanted, outlined, shaded from top to
// bottom and dropped onto a hard shadow, the way fighting games announce.
func bannerSprite(text string, dot int, style bannerStyle, opacity float64) (pix []byte, w, h int) {
	dot = max(1, dot)
	slant := .35
	w, h = textSize(text, dot)
	w += int(math.Ceil(slant*9*float64(dot))) + dot
	h += dot
	p := newPainter(w, h)
	lit := map[[2]int]bool{}
	for i, r := range []rune(text) {
		rows, ok := glyphs[r]
		if !ok {
			continue
		}
		for y, row := range rows {
			for x := range row {
				if row[x] == '1' {
					lit[[2]int{1 + i*6 + x, 1 + y}] = true
				}
			}
		}
	}
	block := func(cx, cy int, c [3]float64, a float64) {
		shift := int(slant * float64((8-cy)*dot))
		for y := cy * dot; y < (cy+1)*dot; y++ {
			for x := cx * dot; x < (cx+1)*dot; x++ {
				p.over(x+dot+shift, y, c[0], c[1], c[2], a)
			}
		}
	}
	for at := range lit {
		block(at[0]+1, at[1]+1, [3]float64{0, 0, 0}, .55*opacity)
	}
	for at := range lit {
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				block(at[0]+dx, at[1]+dy, style.edge, opacity)
			}
		}
	}
	for at := range lit {
		t := float64(at[1]-1) / 6
		var c [3]float64
		for k := range c {
			c[k] = style.top[k] + (style.bottom[k]-style.top[k])*t
		}
		block(at[0], at[1], c, opacity)
	}
	return p.bgra(), w, h
}

// blit lays premultiplied BGRA pixels over the painter at (x, y).
func (p *painter) blit(pix []byte, w, h, x, y int) {
	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			o := (j*w + i) * 4
			a := float64(pix[o+3]) / 255
			if a == 0 {
				continue
			}
			px, py := x+i, y+j
			if px < 0 || py < 0 || px >= p.w || py >= p.h {
				continue
			}
			d := (py*p.w + px) * 4
			keep := 1 - a
			p.pix[d] = float64(pix[o+2])/255 + p.pix[d]*keep
			p.pix[d+1] = float64(pix[o+1])/255 + p.pix[d+1]*keep
			p.pix[d+2] = float64(pix[o])/255 + p.pix[d+2]*keep
			p.pix[d+3] = a + p.pix[d+3]*keep
		}
	}
}

// box paints an axis-aligned rectangle.
func (p *painter) box(x0, y0, x1, y1 float64, c [3]float64, a float64) {
	for y := max(0, int(y0)); y < min(p.h, int(math.Ceil(y1))); y++ {
		for x := max(0, int(x0)); x < min(p.w, int(math.Ceil(x1))); x++ {
			// Coverage of the edge pixels keeps the bars crisp but smooth.
			cx := math.Min(1, math.Min(float64(x+1)-x0, x1-float64(x)))
			cy := math.Min(1, math.Min(float64(y+1)-y0, y1-float64(y)))
			p.over(x, y, c[0], c[1], c[2], a*math.Max(0, cx)*math.Max(0, cy))
		}
	}
}

// hudSprite is the bar across the top of the screen during a fight: both
// health bars meeting at the round clock, as in the games it borrows from.
// Health and its trailing drain are fractions of full.
func hudSprite(w int, pet, petTrail, foe, foeTrail float64, clock, round int, foeTag string, foeColour [3]float64) (pix []byte, h int) {
	dot := max(2, w/240)
	h = dot * 22
	p := newPainter(w, h)
	clockW := float64(dot * 20)
	mid := float64(w) / 2
	barTop, barH := float64(dot*8), float64(dot*7)
	edge := [3]float64{.10, .07, .03}
	bar := func(from, to, health, trail float64, fill [3]float64, rightward bool) {
		p.box(from-2, barTop-2, to+2, barTop+barH+2, edge, .92)
		p.box(from, barTop, to, barTop+barH, [3]float64{.22, .16, .08}, .9)
		span := to - from
		// The bars are anchored at the clock and drain towards the screen edge.
		cut := func(f float64) (float64, float64) {
			if rightward {
				return from, from + span*f
			}
			return to - span*f, to
		}
		a, b := cut(trail)
		p.box(a, barTop, b, barTop+barH, [3]float64{1, .92, .82}, .95)
		a, b = cut(health)
		p.box(a, barTop, b, barTop+barH*.55, [3]float64{math.Min(1, fill[0]*1.15), math.Min(1, fill[1]*1.15), math.Min(1, fill[2]*1.15)}, 1)
		p.box(a, barTop+barH*.55, b, barTop+barH, fill, 1)
	}
	margin := float64(dot * 2)
	bar(margin, mid-clockW/2-margin, pet, petTrail, [3]float64{1, .74, .10}, false)
	bar(mid+clockW/2+margin, float64(w)-margin, foe, foeTrail, foeColour, true)
	p.box(mid-clockW/2, float64(dot*3), mid+clockW/2, float64(dot*19), edge, .95)
	p.box(mid-clockW/2+float64(dot), float64(dot*4), mid+clockW/2-float64(dot), float64(dot*18), [3]float64{.20, .14, .05}, .95)
	digits, dw, dh := bannerSprite(strconv.Itoa(clock), dot, goldBanner, 1)
	p.blit(digits, dw, dh, int(mid)-dw/2, dot*5)
	small := max(1, dot/2)
	label, lw, lh := textSprite("NAIWA", small, false, 1)
	p.blit(label, lw, lh, int(margin), int(barTop)-lh-dot/2)
	label, lw, lh = textSprite(foeTag, small, false, 1)
	p.blit(label, lw, lh, w-int(margin)-lw, int(barTop)-lh-dot/2)
	label, lw, lh = textSprite("R"+strconv.Itoa(round), small, true, 1)
	p.blit(label, lw, lh, int(mid)-lw/2, int(barTop+barH)+dot*2)
	return p.bgra(), h
}

// ringSprite is the shockwave of a heavy blow, spreading and thinning out.
func ringSprite(size int, progress float64, colour [3]float64) []byte {
	p := newPainter(size, size)
	t := math.Max(0, math.Min(1, progress))
	c := float64(size) / 2
	r := c * (.25 + .72*(1-math.Pow(1-t, 3)))
	width := float64(size) * (.07*(1-t) + .012)
	fade := (1 - t) * (1 - t)
	p.shade(colour[0], colour[1], colour[2], func(x, y float64) float64 {
		d := math.Abs(math.Hypot(x-c, y-c) - r)
		return fade * math.Max(0, math.Min(1, width/2-d+.5))
	})
	return p.bgra()
}

// part is one shape of a weapon, in its own frame: x runs from the palm out
// along the weapon, y across it.
type part struct {
	round              bool
	x, y, along, cross float64
	turn               float64
	colour             [3]float64
	alpha              float64
}

var weaponArt = map[string][]part{
	"stick": {
		{false, 52, 0, 66, 10, 0, [3]float64{.25, .14, .05}, 1},
		{false, 52, 0, 64, 7.5, 0, [3]float64{.62, .40, .18}, 1},
		{true, 90, -2, 6, 4, .3, [3]float64{.38, .22, .08}, 1},
		{true, 30, 2, 5, 3, -.2, [3]float64{.38, .22, .08}, 1},
	},
	"pan": {
		{false, 24, 0, 36, 7, 0, [3]float64{.08, .07, .07}, 1},
		{true, 100, 0, 48, 46, 0, [3]float64{.30, .30, .32}, 1},
		{true, 100, 0, 42, 40, 0, [3]float64{.12, .12, .13}, 1},
		{true, 88, -14, 15, 8, -.5, [3]float64{1, 1, 1}, .28},
	},
	"lotus": {
		{false, 20, 0, 28, 4.5, 0, [3]float64{.32, .40, .12}, 1},
		{true, 98, 0, 60, 42, 0, [3]float64{.10, .34, .12}, 1},
		{true, 98, 0, 56, 38, 0, [3]float64{.30, .70, .30}, 1},
		{false, 98, 0, 52, 1.8, 0, [3]float64{.70, .95, .55}, .9},
		{false, 106, -13, 26, 1.5, .55, [3]float64{.70, .95, .55}, .8},
		{false, 106, 13, 26, 1.5, -.55, [3]float64{.70, .95, .55}, .8},
	},
	// Gloves sit over the hand, a little behind it so the stance's
	// outstretched palm does not push them off the canvas.
	"gloves": {
		{true, -4, 12, 33, 30, 0, [3]float64{.42, .10, .04}, 1},
		{true, -4, 12, 30, 27, 0, [3]float64{1, .62, .14}, 1},
		{true, -32, 12, 9, 26, 0, [3]float64{.86, .18, .10}, 1},
		{true, 4, 0, 12, 7, -.4, [3]float64{1, 1, .85}, .45},
	},
	"swatter": {
		{false, 36, 0, 48, 6, 0, [3]float64{.45, .30, .04}, 1},
		{false, 36, 0, 46, 4, 0, [3]float64{1, .80, .22}, 1},
		{false, 122, 0, 42, 36, 0, [3]float64{.45, .30, .04}, 1},
		{false, 122, 0, 38, 32, 0, [3]float64{1, .86, .38}, .4},
		{false, 122, -16, 38, 1.6, 0, [3]float64{1, .80, .22}, 1},
		{false, 122, 0, 38, 1.6, 0, [3]float64{1, .80, .22}, 1},
		{false, 122, 16, 38, 1.6, 0, [3]float64{1, .80, .22}, 1},
		{false, 106, 0, 1.6, 32, 0, [3]float64{1, .80, .22}, 1},
		{false, 122, 0, 1.6, 32, 0, [3]float64{1, .80, .22}, 1},
		{false, 138, 0, 1.6, 32, 0, [3]float64{1, .80, .22}, 1},
	},
}

// rect reports coverage for a rotated rectangle given by its half sizes.
func rect(cx, cy, along, cross, angle float64) func(x, y float64) float64 {
	sin, cos := math.Sincos(angle)
	return func(x, y float64) float64 {
		dx, dy := x-cx, y-cy
		u, v := dx*cos+dy*sin, dy*cos-dx*sin
		return math.Max(0, math.Min(1, .5-math.Max(math.Abs(u)-along, math.Abs(v)-cross)))
	}
}

// paintWeapon draws a weapon into p with its grip at (x, y), pointing at
// angle, at the given scale.
func paintWeapon(p *painter, id string, x, y, angle, scale float64) {
	sin, cos := math.Sincos(angle)
	for _, s := range weaponArt[id] {
		cx, cy := x+(s.x*cos-s.y*sin)*scale, y+(s.x*sin+s.y*cos)*scale
		var cover func(float64, float64) float64
		if s.round {
			cover = ellipse(cx, cy, s.along*scale, s.cross*scale, angle+s.turn, 1)
		} else {
			cover = rect(cx, cy, s.along*scale, s.cross*scale, angle+s.turn)
		}
		a := s.alpha
		p.shade(s.colour[0], s.colour[1], s.colour[2], func(x, y float64) float64 { return a * cover(x, y) })
	}
}

// weaponIcon is a weapon lying at an angle, for a drop on the ground.
func weaponIcon(id string, size int, glow float64) []byte {
	p := newPainter(size, size)
	c := float64(size) / 2
	p.shade(1, .92, .45, func(x, y float64) float64 {
		d := math.Hypot(x-c, y-c) / c
		return .35 * glow * math.Max(0, 1-d*d)
	})
	scale := float64(size) / 260
	paintWeapon(p, id, c-70*scale, c+45*scale, -.6, scale)
	return p.bgra()
}

// holdWeapon draws a weapon into a straight-alpha body frame, gripped at
// (x, y) and pointing at angle, in front of the hand.
func holdWeapon(body *image.NRGBA, id string, x, y, angle float64) {
	if _, ok := weaponArt[id]; !ok {
		return
	}
	scale := float64(body.Rect.Dx()) / 420
	x *= scale
	y *= scale
	reach := int(math.Ceil(170 * scale))
	x0, y0 := max(0, int(x)-reach), max(0, int(y)-reach)
	x1, y1 := min(body.Rect.Dx(), int(x)+reach), min(body.Rect.Dy(), int(y)+reach)
	if x1 <= x0 || y1 <= y0 {
		return
	}
	p := newPainter(x1-x0, y1-y0)
	paintWeapon(p, id, x-float64(x0), y-float64(y0), angle, scale)
	p.onto(body, x0, y0)
}

// onto lays the painter over a straight-alpha frame with its top left at
// (x0, y0).
func (p *painter) onto(body *image.NRGBA, x0, y0 int) {
	defer p.recycle()
	for j := 0; j < p.h; j++ {
		for i := 0; i < p.w; i++ {
			s := (j*p.w + i) * 4
			a := math.Min(1, p.pix[s+3])
			if a <= 0 || !image.Pt(x0+i, y0+j).In(body.Rect) {
				continue
			}
			o := body.PixOffset(x0+i, y0+j)
			under := float64(body.Pix[o+3]) / 255
			out := a + under*(1-a)
			for k := 0; k < 3; k++ {
				// The painter is premultiplied; the body frame is not.
				c := p.pix[s+k] + float64(body.Pix[o+k])/255*under*(1-a)
				body.Pix[o+k] = uint8(math.Round(255 * math.Min(1, c/out)))
			}
			body.Pix[o+3] = uint8(math.Round(255 * out))
		}
	}
}

// recolour maps a frame's shading onto a rival's palette, darkest stop to
// lightest, keeping its outline, light and shadow.
func recolour(src *image.NRGBA, palette [3][3]float64) *image.NRGBA {
	out := image.NewNRGBA(src.Rect)
	for i := 0; i < len(src.Pix); i += 4 {
		a := src.Pix[i+3]
		if a == 0 {
			continue
		}
		l := (.299*float64(src.Pix[i]) + .587*float64(src.Pix[i+1]) + .114*float64(src.Pix[i+2])) / 255
		// The pet spans a narrow band of brightness; spread it over the palette.
		l = math.Max(0, math.Min(1, (l-.12)/.82))
		lo, hi, t := palette[0], palette[1], l*2
		if l > .5 {
			lo, hi, t = palette[1], palette[2], l*2-1
		}
		for k := 0; k < 3; k++ {
			out.Pix[i+k] = uint8(math.Round(255 * (lo[k] + (hi[k]-lo[k])*t)))
		}
		out.Pix[i+3] = a
	}
	return out
}

// whiten flashes a frame towards white, the instant of a hit.
func whiten(img *image.NRGBA, amount float64) {
	amount = math.Max(0, math.Min(1, amount))
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i+3] == 0 {
			continue
		}
		for k := 0; k < 3; k++ {
			img.Pix[i+k] = uint8(float64(img.Pix[i+k]) + (255-float64(img.Pix[i+k]))*amount)
		}
	}
}

// fadeOut scales a frame's opacity, for a beaten rival leaving the ring.
func fadeOut(img *image.NRGBA, opacity float64) {
	opacity = math.Max(0, math.Min(1, opacity))
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = uint8(float64(img.Pix[i]) * opacity)
		if img.Pix[i] == 0 {
			img.Pix[i-3], img.Pix[i-2], img.Pix[i-1] = 0, 0, 0
		}
	}
}

// gripAngle is how a held weapon is carried through a strike. The stance's
// palm is a hand's width from the canvas edge, so a weapon pointed along the
// arm would be cut off; instead it is held up and chopped forward, tipped back
// on the wind-up and over at the hit.
func gripAngle(progress float64) float64 {
	const windup, impact = .28, .40
	var swing float64
	switch {
	case progress <= 0:
		swing = -.08
	case progress < windup:
		swing = -.08 + (.35+.08)*progress/windup
	case progress < impact:
		swing = .35 - .6*(progress-windup)/(impact-windup)
	default:
		swing = -.25 + .17*math.Min(1, (progress-impact)/(1-impact))
	}
	// Straight up in canvas coordinates, then the swing; positive tips the
	// weapon back, away from the way the pet faces.
	return -math.Pi/2 + swing
}
