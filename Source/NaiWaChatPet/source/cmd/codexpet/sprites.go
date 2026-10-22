package main

import (
	"math"
	"math/bits"
	"strconv"
	"sync"
)

// painter draws the small procedural sprites of control mode: the ground
// shadow, flies, impact bursts and score digits. It accumulates premultiplied
// colour so the result can go straight to a layered window.
type painter struct {
	poolBucket int
	w, h       int
	pix        []float64
}

func newPainter(w, h int) *painter {
	w, h = max(1, w), max(1, h)
	n := w * h * 4
	bucket := bits.Len(uint(n - 1))
	var pix []float64
	if bucket < len(painterPools) {
		if cached := painterPools[bucket].Get(); cached != nil {
			pix = cached.([]float64)
		}
	}
	if pix == nil {
		pix = make([]float64, 1<<bucket)
	}
	pix = pix[:n]
	clear(pix)
	return &painter{w: w, h: h, pix: pix, poolBucket: bucket}
}

var painterPools [32]sync.Pool

func (p *painter) recycle() {
	if p.pix != nil && p.poolBucket < len(painterPools) {
		painterPools[p.poolBucket].Put(p.pix[:cap(p.pix)])
	}
	p.pix = nil
}

// over lays one colour over a pixel with the given coverage.
func (p *painter) over(x, y int, r, g, b, a float64) {
	if x < 0 || y < 0 || x >= p.w || y >= p.h || a <= 0 {
		return
	}
	a = math.Min(1, a)
	o := (y*p.w + x) * 4
	keep := 1 - a
	p.pix[o], p.pix[o+1], p.pix[o+2], p.pix[o+3] = r*a+p.pix[o]*keep, g*a+p.pix[o+1]*keep, b*a+p.pix[o+2]*keep, a+p.pix[o+3]*keep
}

// shade paints every pixel with the coverage a function reports for it.
func (p *painter) shade(r, g, b float64, cover func(x, y float64) float64) {
	for y := 0; y < p.h; y++ {
		for x := 0; x < p.w; x++ {
			p.over(x, y, r, g, b, cover(float64(x)+.5, float64(y)+.5))
		}
	}
}

// ellipse reports coverage for a rotated ellipse with an edge about soft
// pixels wide.
func ellipse(cx, cy, rx, ry, angle, soft float64) func(x, y float64) float64 {
	sin, cos := math.Sincos(angle)
	return func(x, y float64) float64 {
		dx, dy := x-cx, y-cy
		u, v := (dx*cos+dy*sin)/rx, (dy*cos-dx*sin)/ry
		return math.Max(0, math.Min(1, (1-math.Sqrt(u*u+v*v))*math.Min(rx, ry)/soft+.5))
	}
}

// bgra returns the picture as premultiplied BGRA rows, top first.
func (p *painter) bgra() []byte {
	defer p.recycle()
	out := make([]byte, len(p.pix))
	for i := 0; i < len(p.pix); i += 4 {
		a := p.pix[i+3]
		if a <= 0 {
			continue
		}
		if a > 1 {
			a = 1
		}
		// Colour may never exceed alpha in a layered window.
		b, g, r := p.pix[i+2], p.pix[i+1], p.pix[i]
		if b > a {
			b = a
		}
		if g > a {
			g = a
		}
		if r > a {
			r = a
		}
		if b < 0 {
			b = 0
		}
		if g < 0 {
			g = 0
		}
		if r < 0 {
			r = 0
		}
		out[i] = byte(b*255 + .5)
		out[i+1] = byte(g*255 + .5)
		out[i+2] = byte(r*255 + .5)
		out[i+3] = byte(a*255 + .5)
	}
	return out
}

// shadowSprite is the patch of shade under the pet. A gold rim marks that the
// keyboard is steering it.
func shadowSprite(w, h int, strength float64, rim bool) []byte {
	p := newPainter(w, h)
	cx, cy, rx, ry := float64(w)/2, float64(h)/2, float64(w)/2-1, float64(h)/2-1
	p.shade(0, 0, 0, func(x, y float64) float64 {
		u, v := (x-cx)/rx, (y-cy)/ry
		return .42 * strength * math.Max(0, 1-(u*u+v*v))
	})
	if rim {
		outer, inner := ellipse(cx, cy, rx*.94, ry*.90, 0, 1.2), ellipse(cx, cy, rx*.94-2.4, ry*.90-2.4, 0, 1.2)
		p.shade(1, .80, .26, func(x, y float64) float64 { return .85 * strength * (outer(x, y) - inner(x, y)) })
	}
	return p.bgra()
}

// flySprite draws a fly. Wing is the beat phase in turns; heading is +1 when
// it faces right.
func flySprite(size int, wing float64, heading int) []byte {
	p := newPainter(size, size)
	s := float64(size)
	side := float64(heading)
	cx, cy := s/2, s*.56
	beat := .55 * math.Sin(2*math.Pi*wing)
	for _, lift := range []float64{-1, 1} {
		angle := side * (-.35 + lift*(.5+beat))
		wingAt := ellipse(cx-side*s*.04-math.Sin(angle)*s*.02, cy-s*.20, s*.27, s*.11, angle-side*.9, 1.2)
		p.shade(.86, .93, 1, func(x, y float64) float64 { return .55 * wingAt(x, y) })
	}
	p.shade(.10, .11, .13, ellipse(cx-side*s*.05, cy, s*.20, s*.13, side*.12, 1.1))
	p.shade(.18, .30, .26, ellipse(cx-side*s*.07, cy-s*.03, s*.11, s*.05, side*.12, 1.4))
	p.shade(.12, .12, .14, ellipse(cx+side*s*.17, cy-s*.02, s*.10, s*.10, 0, 1.1))
	p.shade(.78, .16, .12, ellipse(cx+side*s*.21, cy-s*.05, s*.045, s*.05, 0, 1))
	return p.bgra()
}

// burstSprite is the star that flashes where a strike lands. Progress runs
// from 0 at the hit to 1 when it has faded.
func burstSprite(size int, progress float64, hard bool) []byte {
	p := newPainter(size, size)
	c := float64(size) / 2
	t := math.Max(0, math.Min(1, progress))
	grow := 1 - math.Pow(1-t, 3)
	fade := (1 - t) * (1 - t)
	points, reach := 8.0, c*(.45+.5*grow)
	if hard {
		points = 10
	}
	star := func(scale float64) func(x, y float64) float64 {
		return func(x, y float64) float64 {
			dx, dy := x-c, y-c
			spike := math.Pow(math.Abs(math.Cos(points/2*math.Atan2(dy, dx)+t*1.2)), 2.4)
			return math.Max(0, math.Min(1, reach*scale*(.42+.58*spike)-math.Hypot(dx, dy)+.5))
		}
	}
	rim, outer, middle, core := star(1.1), star(1), star(.74), star(.42)
	// A dark rim keeps the flash readable over white windows as well as dark ones.
	p.shade(.45, .14, .02, func(x, y float64) float64 { return .7 * fade * rim(x, y) })
	if hard {
		p.shade(1, .36, .06, func(x, y float64) float64 { return fade * outer(x, y) })
	} else {
		p.shade(1, .56, .08, func(x, y float64) float64 { return fade * outer(x, y) })
	}
	p.shade(1, .90, .30, func(x, y float64) float64 { return fade * middle(x, y) })
	p.shade(1, 1, .92, func(x, y float64) float64 { return fade * core(x, y) })
	return p.bgra()
}

// glyphs is a 5x7 face for the few characters the score needs.
var glyphs = map[rune][7]string{
	'0': {"01110", "10001", "10011", "10101", "11001", "10001", "01110"},
	'1': {"00100", "01100", "00100", "00100", "00100", "00100", "01110"},
	'2': {"01110", "10001", "00001", "00010", "00100", "01000", "11111"},
	'3': {"11110", "00001", "00001", "01110", "00001", "00001", "11110"},
	'4': {"00010", "00110", "01010", "10010", "11111", "00010", "00010"},
	'5': {"11111", "10000", "11110", "00001", "00001", "10001", "01110"},
	'6': {"00110", "01000", "10000", "11110", "10001", "10001", "01110"},
	'7': {"11111", "00001", "00010", "00100", "01000", "01000", "01000"},
	'8': {"01110", "10001", "10001", "01110", "10001", "10001", "01110"},
	'9': {"01110", "10001", "10001", "01111", "00001", "00010", "01100"},
	'+': {"00000", "00100", "00100", "11111", "00100", "00100", "00000"},
	'x': {"00000", "10001", "01010", "00100", "01010", "10001", "00000"},
	' ': {"00000", "00000", "00000", "00000", "00000", "00000", "00000"},
}

// textSize is the pixel size textSprite needs for a string.
func textSize(text string, dot int) (w, h int) {
	return (len([]rune(text))*6+1)*dot + 2*dot, 9 * dot
}

// textSprite renders digits and signs in a chunky outlined face that stays
// readable over any wallpaper. Gold marks a streak. Opacity fades the whole
// sprite.
func textSprite(text string, dot int, gold bool, opacity float64) (pix []byte, w, h int) {
	dot = max(1, dot)
	w, h = textSize(text, dot)
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
	block := func(cx, cy int, r, g, b, a float64) {
		for y := cy * dot; y < (cy+1)*dot; y++ {
			for x := cx * dot; x < (cx+1)*dot; x++ {
				p.over(x+dot, y, r, g, b, a)
			}
		}
	}
	for at := range lit {
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				if !lit[[2]int{at[0] + dx, at[1] + dy}] {
					block(at[0]+dx, at[1]+dy, .12, .08, .04, .9*opacity)
				}
			}
		}
	}
	for at := range lit {
		if gold {
			block(at[0], at[1], 1, .82, .22, opacity)
		} else {
			block(at[0], at[1], 1, 1, 1, opacity)
		}
	}
	return p.bgra(), w, h
}

// scoreText formats the fly count, with the streak once it is worth showing.
func scoreText(score, streak int) string {
	if streak >= 2 {
		return strconv.Itoa(score) + " x" + strconv.Itoa(streak)
	}
	return strconv.Itoa(score)
}

// mirror flips straight-alpha pixels left to right in place.
func mirror(pix []uint8, stride, width, height int) {
	for y := 0; y < height; y++ {
		row := pix[y*stride : y*stride+width*4]
		for l, r := 0, (width-1)*4; l < r; l, r = l+4, r-4 {
			row[l], row[l+1], row[l+2], row[l+3], row[r], row[r+1], row[r+2], row[r+3] = row[r], row[r+1], row[r+2], row[r+3], row[l], row[l+1], row[l+2], row[l+3]
		}
	}
}
