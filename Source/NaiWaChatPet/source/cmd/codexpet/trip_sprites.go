package main

import (
	"image"
	"math"
)

// docLook is how a kind of file looks when the pet carries it: a coloured
// band and a short label on a white page.
type docLook struct {
	band  [3]float64
	label string
}

var docLooks = map[string]docLook{
	"PDF 文档":   {[3]float64{.86, .16, .14}, "PDF"},
	"Word 文档":  {[3]float64{.16, .38, .80}, "W"},
	"Excel 表格": {[3]float64{.10, .56, .28}, "X"},
	"PPT 演示":   {[3]float64{.92, .42, .14}, "P"},
	"文本笔记":     {[3]float64{.45, .48, .52}, "TXT"},
	"图片":       {[3]float64{.18, .62, .78}, ""},
	"视频":       {[3]float64{.50, .26, .74}, ""},
	"音频":       {[3]float64{.10, .60, .62}, ""},
	"压缩包":      {[3]float64{.92, .72, .12}, "ZIP"},
	"安装程序":     {[3]float64{.26, .26, .30}, "EXE"},
	"代码":       {[3]float64{.14, .20, .30}, "CODE"},
	"快捷方式":     {[3]float64{.20, .44, .90}, ""},
}

// paintDoc draws a page of the given kind centred at (cx, cy), size tall.
func paintDoc(p *painter, kind string, cx, cy, size, tilt float64) {
	look, ok := docLooks[kind]
	if !ok {
		look = docLook{[3]float64{.55, .55, .58}, ""}
	}
	w, h := size*.76, size
	sin, cos := math.Sincos(tilt)
	at := func(x, y float64) (float64, float64) { return cx + x*cos - y*sin, cy + x*sin + y*cos }
	soft := math.Max(1, size/40)
	page := func(x, y, along, cross float64) func(float64, float64) float64 {
		px, py := at(x, y)
		return rect(px, py, along, cross, tilt)
	}
	edge := page(0, 0, w/2+soft, h/2+soft)
	p.shade(.25, .22, .18, func(x, y float64) float64 { return .8 * edge(x, y) })
	body := page(0, 0, w/2, h/2)
	p.shade(.99, .99, .97, body)
	// Folded corner, top right.
	fold := page(w/2-w*.14, -h/2+w*.14, w*.13, w*.13)
	p.shade(.82, .82, .80, fold)
	band := page(0, h*.12, w/2, h*.17)
	p.shade(look.band[0], look.band[1], look.band[2], band)
	switch kind {
	case "图片":
		hill := ellipse(cx, cy+h*.18, w*.38, h*.14, tilt, soft)
		p.shade(.20, .55, .25, hill)
		sx, sy := at(w*.18, -h*.16)
		p.shade(1, .80, .20, ellipse(sx, sy, w*.1, w*.1, 0, soft))
	case "视频":
		tx, ty := at(0, h*.12)
		p.shade(1, 1, 1, func(x, y float64) float64 {
			// A right-pointing play triangle: inside all three edges.
			dx, dy := (x-tx)*cos+(y-ty)*sin, (y-ty)*cos-(x-tx)*sin
			r := w * .13
			return math.Max(0, math.Min(1, math.Min(dx+r*.6, (r*.9-dx)-math.Abs(dy)*1.6)/soft))
		})
	case "音频":
		nx, ny := at(-w*.08, h*.2)
		p.shade(1, 1, 1, ellipse(nx, ny, w*.09, w*.07, tilt-.3, soft))
		sx, sy := at(w*.0, h*.06)
		p.shade(1, 1, 1, rect(sx, sy, w*.02, h*.12, tilt))
	case "快捷方式":
		ax, ay := at(0, h*.12)
		p.shade(1, 1, 1, rect(ax, ay, w*.22, w*.045, tilt-.75))
		hx, hy := at(w*.13, h*.12-w*.13)
		p.shade(1, 1, 1, rect(hx, hy, w*.09, w*.045, tilt))
		p.shade(1, 1, 1, rect(hx+w*.04, hy+w*.06, w*.045, w*.09, tilt))
	}
	// Lines of "text" above the band.
	for i := 0; i < 3; i++ {
		lx, ly := at(-w*.06, -h*.32+float64(i)*h*.09)
		p.shade(.70, .70, .72, rect(lx, ly, w*.28, math.Max(.6, h*.018), tilt))
	}
	if look.label != "" {
		dot := max(1, int(size/(9*float64(len(look.label))+30)))
		pix, lw, lh := textSprite(look.label, dot, false, 1)
		lx, ly := at(0, h*.12)
		p.blit(pix, lw, lh, int(lx)-lw/2, int(ly)-lh/2)
	}
}

// docSprite is one file in flight between the desktop and the pet's box.
func docSprite(kind string, size int, tilt float64) []byte {
	p := newPainter(size, size)
	paintDoc(p, kind, float64(size)/2, float64(size)/2, float64(size)*.82, tilt)
	return p.bgra()
}

// Cardboard, matching the box in the filmed scene.
var (
	boxFront = [3]float64{.64, .47, .32}
	boxSide  = [3]float64{.52, .37, .24}
	boxTop   = [3]float64{.74, .57, .40}
	boxTape  = [3]float64{.86, .78, .62}
	boxEdge  = [3]float64{.30, .20, .12}
)

// holdBox draws an open cardboard box into a straight-alpha body frame, its
// front face centred at (cx, cy), with the carried files standing up out of
// it. It goes in front of the belly, over the hands that hold it.
func holdBox(body *image.NRGBA, cx, cy float64, kinds []string) {
	const w, h, depth = 190.0, 128.0, 40.0
	x0, y0 := max(0, int(cx-w/2-30)), max(0, int(cy-h/2-depth-110))
	x1, y1 := min(body.Rect.Dx(), int(cx+w/2+depth+30)), min(body.Rect.Dy(), int(cy+h/2+30))
	if x1 <= x0 || y1 <= y0 {
		return
	}
	p := newPainter(x1-x0, y1-y0)
	ox, oy := cx-float64(x0), cy-float64(y0)
	left, right, top, bottom := ox-w/2, ox+w/2, oy-h/2, oy+h/2
	// The inside back wall, then the files, then the front over them.
	p.box(left+depth*.4, top-depth*.6, right+depth*.4, top+2, [3]float64{.36, .25, .15}, 1)
	for i, kind := range kinds {
		if i >= 6 {
			break
		}
		// Fan the files across the opening, the newest in front.
		n := float64(min(len(kinds), 6))
		fx := ox - w*.28 + w*.56*(float64(i)+.5)/n
		paintDoc(p, kind, fx, top-30-float64(i%2)*10, 74, (float64(i)-n/2)*.12)
	}
	// Side face, a parallelogram going back and up to the right.
	p.shade(boxSide[0], boxSide[1], boxSide[2], func(x, y float64) float64 {
		if x < right || x > right+depth {
			return 0
		}
		lift := (x - right) * .6
		return math.Max(0, math.Min(1, math.Min(y-(top-lift), (bottom-lift)-y)+.5))
	})
	p.box(left-2, top-2, right+2, bottom+2, boxEdge, 1)
	p.box(left, top, right, bottom, boxFront, 1)
	// The front rim catches the light; a strip of tape runs down the middle.
	p.box(left, top, right, top+10, boxTop, 1)
	p.box(ox-12, top, ox+12, bottom, boxTape, .9)
	p.box(left, bottom-8, right, bottom, boxSide, .6)
	p.onto(body, x0, y0)
}
