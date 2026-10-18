// Package rig deforms the pet's rendered frames with a bone-driven skinned mesh.
//
// Nothing is cut apart or repainted. Every layer samples the same source frame,
// layers overlap beneath soft masks, and the rest pose reproduces the frame, so
// a motion can start from the idle clip without a visible switch.
package rig

import (
	"errors"
	"image"
	"math"
)

type Vec struct{ X, Y float64 }

// Affine maps (x, y) to (A*x + C*y + E, B*x + D*y + F).
type Affine struct{ A, B, C, D, E, F float64 }

var identity = Affine{A: 1, D: 1}

func (m Affine) apply(p Vec) Vec {
	return Vec{m.A*p.X + m.C*p.Y + m.E, m.B*p.X + m.D*p.Y + m.F}
}

// mul returns the transform that applies n first and m second.
func (m Affine) mul(n Affine) Affine {
	return Affine{
		A: m.A*n.A + m.C*n.B, B: m.B*n.A + m.D*n.B,
		C: m.A*n.C + m.C*n.D, D: m.B*n.C + m.D*n.D,
		E: m.A*n.E + m.C*n.F + m.E, F: m.B*n.E + m.D*n.F + m.F,
	}
}

// about re-centres a linear map on p.
func about(p Vec, m Affine) Affine {
	m.E = p.X - m.A*p.X - m.C*p.Y
	m.F = p.Y - m.B*p.X - m.D*p.Y
	return m
}

// Positive angles turn a downward-hanging limb towards -x, the way the pet faces.
func rotation(angle float64) Affine {
	s, c := math.Sincos(angle)
	return Affine{A: c, B: s, C: -s, D: c}
}

// stretch scales by along on the given axis and by across perpendicular to it.
func stretch(axis, along, across float64) Affine {
	s, c := math.Sincos(axis)
	return Affine{A: along*c*c + across*s*s, B: (along - across) * c * s, C: (along - across) * c * s, D: along*s*s + across*c*c}
}

// Bone is a joint in the rest pose. Tip gives the bone its direction.
type Bone struct {
	Name       string
	Parent     int
	Pivot, Tip Vec
}

// Joint is a bone's change from rest; its zero value leaves the bone alone.
// Stretch lengthens the bone along its axis and Bulge widens it across.
type Joint struct{ Rot, Stretch, Bulge, TX, TY float64 }

type Pose []Joint

type vertex struct {
	p    Vec
	bone [4]uint8
	w    [4]float32
}

// Layer is one overlapping sheet of the mesh. A nil mask covers the canvas.
type Layer struct {
	Name  string
	mask  []uint8
	verts []vertex
	tris  [][3]int32
	// cells holds, for each triangle, the mesh cell it was cut from.
	cells []int32
}

// LayerSpec describes a layer in rest-pose canvas coordinates. Mask returns
// coverage in 0..1 and Weights the influence of each bone at a point.
type LayerSpec struct {
	Name    string
	Mask    func(x, y float64) float64
	Weights func(p Vec) []float64
}

// Puppet is an immutable rig; Render may be called from one goroutine at a time.
type Puppet struct {
	w, h   int
	bones  []Bone
	layers []Layer
	world  []Affine
	moved  []Vec
	sheet  []uint8
	out    []uint8
}

const cell = 10

// NewPuppet builds the mesh. Bones must list parents before children, and
// layers are drawn in the order given.
func NewPuppet(width, height int, bones []Bone, specs []LayerSpec) (*Puppet, error) {
	if width < cell || height < cell || width > 4096 || height > 4096 {
		return nil, errors.New("rig: invalid canvas")
	}
	if len(bones) == 0 || len(bones) > 255 {
		return nil, errors.New("rig: invalid bone count")
	}
	for i, b := range bones {
		if b.Parent >= i || b.Parent < -1 {
			return nil, errors.New("rig: bone " + b.Name + " must follow its parent")
		}
	}
	p := &Puppet{w: width, h: height, bones: bones, world: make([]Affine, len(bones)), sheet: make([]uint8, width*height*4), out: make([]uint8, width*height*4)}
	cols, rows := (width+cell-1)/cell, (height+cell-1)/cell
	for _, spec := range specs {
		if spec.Weights == nil {
			return nil, errors.New("rig: layer " + spec.Name + " has no weights")
		}
		layer := Layer{Name: spec.Name}
		if spec.Mask != nil {
			layer.mask = make([]uint8, width*height)
			for y := 0; y < height; y++ {
				for x := 0; x < width; x++ {
					layer.mask[y*width+x] = uint8(math.Round(255 * clamp01(spec.Mask(float64(x)+.5, float64(y)+.5))))
				}
			}
		}
		index := map[[2]int]int32{}
		node := func(i, j int) (int32, error) {
			if v, ok := index[[2]int{i, j}]; ok {
				return v, nil
			}
			at := Vec{math.Min(float64(i*cell), float64(width)), math.Min(float64(j*cell), float64(height))}
			v, err := skin(at, spec.Weights(at), len(bones))
			if err != nil {
				return 0, errors.New("rig: layer " + spec.Name + ": " + err.Error())
			}
			layer.verts = append(layer.verts, v)
			index[[2]int{i, j}] = int32(len(layer.verts) - 1)
			return index[[2]int{i, j}], nil
		}
		for j := 0; j < rows; j++ {
			for i := 0; i < cols; i++ {
				if !layer.covers(width, height, i*cell, j*cell) {
					continue
				}
				var q [4]int32
				for k, d := range [4][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
					v, err := node(i+d[0], j+d[1])
					if err != nil {
						return nil, err
					}
					q[k] = v
				}
				layer.tris = append(layer.tris, [3]int32{q[0], q[1], q[2]}, [3]int32{q[1], q[3], q[2]})
				layer.cells = append(layer.cells, int32(j*cols+i), int32(j*cols+i))
			}
		}
		if len(layer.verts) > len(p.moved) {
			p.moved = make([]Vec, len(layer.verts))
		}
		p.layers = append(p.layers, layer)
	}
	if len(p.layers) == 0 {
		return nil, errors.New("rig: no layers")
	}
	return p, nil
}

// covers reports whether a mesh cell, padded by a pixel, holds any mask.
func (l *Layer) covers(width, height, x, y int) bool {
	if l.mask == nil {
		return true
	}
	for py := max(0, y-1); py < min(height, y+cell+1); py++ {
		for px := max(0, x-1); px < min(width, x+cell+1); px++ {
			if l.mask[py*width+px] != 0 {
				return true
			}
		}
	}
	return false
}

// skin keeps the four strongest influences and normalises them.
func skin(at Vec, weights []float64, bones int) (vertex, error) {
	v := vertex{p: at}
	if len(weights) != bones {
		return v, errors.New("weights must name every bone")
	}
	var total float64
	for slot := 0; slot < 4; slot++ {
		best, value := -1, 0.0
		for i, w := range weights {
			if math.IsNaN(w) || math.IsInf(w, 0) || w < 0 {
				return v, errors.New("weights must be finite and non-negative")
			}
			taken := false
			for k := 0; k < slot; k++ {
				taken = taken || int(v.bone[k]) == i && v.w[k] > 0
			}
			if !taken && w > value {
				best, value = i, w
			}
		}
		if best < 0 {
			break
		}
		v.bone[slot], v.w[slot] = uint8(best), float32(value)
		total += value
	}
	if total <= 0 {
		return v, errors.New("a vertex has no influence")
	}
	for k := range v.w {
		v.w[k] = float32(float64(v.w[k]) / total)
	}
	return v, nil
}

func (p *Puppet) Bounds() image.Rectangle { return image.Rect(0, 0, p.w, p.h) }
func (p *Puppet) Bones() int              { return len(p.bones) }

// Rest returns a pose that leaves every bone alone.
func (p *Puppet) Rest() Pose { return make(Pose, len(p.bones)) }

func (p *Puppet) solve(pose Pose) {
	for i, b := range p.bones {
		local := identity
		if i < len(pose) {
			j := pose[i]
			axis := math.Atan2(b.Tip.Y-b.Pivot.Y, b.Tip.X-b.Pivot.X)
			local = about(b.Pivot, rotation(j.Rot).mul(stretch(axis, 1+j.Stretch, 1+j.Bulge)))
			local.E += j.TX
			local.F += j.TY
		}
		if b.Parent >= 0 {
			local = p.world[b.Parent].mul(local)
		}
		p.world[i] = local
	}
}

// Point reports where a rest-pose point attached to one bone ends up.
func (p *Puppet) Point(pose Pose, bone int, at Vec) Vec {
	if bone < 0 || bone >= len(p.bones) {
		return at
	}
	p.solve(pose)
	return p.world[bone].apply(at)
}

// Texture is a source frame held as premultiplied RGBA for filtering.
type Texture struct {
	w, h int
	pix  []uint8
	// empty marks the mesh cells in which the frame is fully transparent.
	empty []bool
}

func NewTexture(frame *image.NRGBA) *Texture {
	b := frame.Bounds()
	t := &Texture{w: b.Dx(), h: b.Dy(), pix: make([]uint8, b.Dx()*b.Dy()*4)}
	for y := 0; y < t.h; y++ {
		row := frame.Pix[frame.PixOffset(b.Min.X, b.Min.Y+y):]
		for x := 0; x < t.w; x++ {
			s, d := row[x*4:x*4+4], t.pix[(y*t.w+x)*4:]
			a := uint32(s[3])
			d[0], d[1], d[2], d[3] = uint8((uint32(s[0])*a+127)/255), uint8((uint32(s[1])*a+127)/255), uint8((uint32(s[2])*a+127)/255), s[3]
		}
	}
	// Most of the canvas is empty around the pet; note which cells are, with a
	// texel of padding for the filter, so Render can skip them.
	cols, rows := (t.w+cell-1)/cell, (t.h+cell-1)/cell
	t.empty = make([]bool, cols*rows)
	for j := 0; j < rows; j++ {
		for i := 0; i < cols; i++ {
			blank := true
			for y := max(0, j*cell-1); blank && y < min(t.h, (j+1)*cell+1); y++ {
				for x := max(0, i*cell-1); x < min(t.w, (i+1)*cell+1); x++ {
					if t.pix[(y*t.w+x)*4+3] != 0 {
						blank = false
						break
					}
				}
			}
			t.empty[j*cols+i] = blank
		}
	}
	return t
}

// Render draws the posed frame as straight-alpha pixels with zero RGB wherever
// alpha is zero, matching the release contract for body frames.
func (p *Puppet) Render(tex *Texture, pose Pose) (*image.NRGBA, error) {
	if tex == nil || tex.w != p.w || tex.h != p.h {
		return nil, errors.New("rig: texture does not match the puppet canvas")
	}
	p.solve(pose)
	clear(p.out)
	for li := range p.layers {
		layer := &p.layers[li]
		for i, v := range layer.verts {
			var x, y float64
			for k := 0; k < 4; k++ {
				if v.w[k] == 0 {
					continue
				}
				q := p.world[v.bone[k]].apply(v.p)
				x += q.X * float64(v.w[k])
				y += q.Y * float64(v.w[k])
			}
			p.moved[i] = Vec{x, y}
		}
		dirty := image.Rectangle{}
		for i, t := range layer.tris {
			if tex.empty[layer.cells[i]] {
				continue
			}
			r := p.fill(tex, layer.mask, p.moved[t[0]], p.moved[t[1]], p.moved[t[2]], layer.verts[t[0]].p, layer.verts[t[1]].p, layer.verts[t[2]].p)
			dirty = dirty.Union(r)
		}
		p.flatten(dirty)
	}
	out := image.NewNRGBA(image.Rect(0, 0, p.w, p.h))
	for i := 0; i < len(p.out); i += 4 {
		a := uint32(p.out[i+3])
		switch a {
		case 0:
		case 255:
			copy(out.Pix[i:i+4], p.out[i:i+4])
		default:
			out.Pix[i] = uint8(min(255, (uint32(p.out[i])*255+a/2)/a))
			out.Pix[i+1] = uint8(min(255, (uint32(p.out[i+1])*255+a/2)/a))
			out.Pix[i+2] = uint8(min(255, (uint32(p.out[i+2])*255+a/2)/a))
			out.Pix[i+3] = uint8(a)
		}
	}
	return out, nil
}

// fill rasterises one triangle into the layer sheet. Where triangles of a layer
// overlap, the more opaque sample wins: shared edges never blend twice and a
// folded joint cannot punch a hole with the empty texels beside the limb.
func (p *Puppet) fill(tex *Texture, mask []uint8, a, b, c, ua, ub, uc Vec) image.Rectangle {
	area := (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
	if math.Abs(area) < 1e-9 {
		return image.Rectangle{}
	}
	x0 := max(0, int(math.Floor(math.Min(a.X, math.Min(b.X, c.X)))))
	y0 := max(0, int(math.Floor(math.Min(a.Y, math.Min(b.Y, c.Y)))))
	x1 := min(p.w-1, int(math.Ceil(math.Max(a.X, math.Max(b.X, c.X)))))
	y1 := min(p.h-1, int(math.Ceil(math.Max(a.Y, math.Max(b.Y, c.Y)))))
	if x0 > x1 || y0 > y1 {
		return image.Rectangle{}
	}
	inv := 1 / area
	// Barycentric weights, and so the source position, are affine in x: find
	// them at the start of each row and step across it. Source positions are
	// kept in 1/256 texel, offset so that they index texel centres.
	w0x, w1x := (b.Y-c.Y)*inv, (c.Y-a.Y)*inv
	ux := (w0x*(ua.X-uc.X) + w1x*(ub.X-uc.X)) * 256
	vx := (w0x*(ua.Y-uc.Y) + w1x*(ub.Y-uc.Y)) * 256
	const slack = -1e-6
	w, h, pix := p.w, p.h, tex.pix
	for y := y0; y <= y1; y++ {
		px, py := float64(x0)+.5, float64(y)+.5
		w0 := ((b.X-px)*(c.Y-py) - (b.Y-py)*(c.X-px)) * inv
		w1 := ((c.X-px)*(a.Y-py) - (c.Y-py)*(a.X-px)) * inv
		// The nudge keeps a rest-pose sample on its own texel despite rounding.
		u := (w0*ua.X+w1*ub.X+(1-w0-w1)*uc.X)*256 - 128 + 1e-4
		v := (w0*ua.Y+w1*ub.Y+(1-w0-w1)*uc.Y)*256 - 128 + 1e-4
		o := (y*w + x0) * 4
		for x := x0; x <= x1; x, w0, w1, u, v, o = x+1, w0+w0x, w1+w1x, u+ux, v+vx, o+4 {
			if w0 < slack || w1 < slack || 1-w0-w1 < slack {
				continue
			}
			fx, fy := int(math.Floor(u)), int(math.Floor(v))
			sx, sy := fx>>8, fy>>8
			ax, ay := uint32(fx&255), uint32(fy&255)
			if sx < 0 || sy < 0 || sx >= w-1 || sy >= h-1 {
				p.border(tex, mask, sx, sy, ax, ay, o)
				continue
			}
			s := (sy*w + sx) * 4
			top, low := pix[s:s+8:s+8], pix[s+w*4:s+w*4+8:s+w*4+8]
			if top[3]|top[7]|low[3]|low[7] == 0 {
				continue
			}
			k00, k10, k01, k11 := (256-ax)*(256-ay), ax*(256-ay), (256-ax)*ay, ax*ay
			m := uint32(255)
			if mask != nil {
				i := sy*w + sx
				m = (uint32(mask[i])*k00 + uint32(mask[i+1])*k10 + uint32(mask[i+w])*k01 + uint32(mask[i+w+1])*k11 + 1<<15) >> 16
				if m == 0 {
					continue
				}
			}
			alpha := (((uint32(top[3])*k00+uint32(top[7])*k10+uint32(low[3])*k01+uint32(low[7])*k11+1<<15)>>16)*m + 127) / 255
			if alpha <= uint32(p.sheet[o+3]) {
				continue
			}
			p.sheet[o] = uint8((((uint32(top[0])*k00+uint32(top[4])*k10+uint32(low[0])*k01+uint32(low[4])*k11+1<<15)>>16)*m + 127) / 255)
			p.sheet[o+1] = uint8((((uint32(top[1])*k00+uint32(top[5])*k10+uint32(low[1])*k01+uint32(low[5])*k11+1<<15)>>16)*m + 127) / 255)
			p.sheet[o+2] = uint8((((uint32(top[2])*k00+uint32(top[6])*k10+uint32(low[2])*k01+uint32(low[6])*k11+1<<15)>>16)*m + 127) / 255)
			p.sheet[o+3] = uint8(alpha)
		}
	}
	return image.Rect(x0, y0, x1+1, y1+1)
}

// border is fill's slow path for a sample touching the canvas edge, where the
// texels beyond it count as transparent.
func (p *Puppet) border(tex *Texture, mask []uint8, x, y int, ax, ay uint32, o int) {
	if x < -1 || y < -1 || x >= p.w || y >= p.h {
		return
	}
	weight := [4]uint32{(256 - ax) * (256 - ay), ax * (256 - ay), (256 - ax) * ay, ax * ay}
	var sum [4]uint32
	cover := uint32(255 << 16)
	if mask != nil {
		cover = 0
	}
	for k, d := range [4][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
		tx, ty := x+d[0], y+d[1]
		if tx < 0 || ty < 0 || tx >= p.w || ty >= p.h {
			continue
		}
		s := (ty*p.w + tx) * 4
		for c := range sum {
			sum[c] += uint32(tex.pix[s+c]) * weight[k]
		}
		if mask != nil {
			cover += uint32(mask[ty*p.w+tx]) * weight[k]
		}
	}
	m := (cover + 1<<15) >> 16
	if alpha := (((sum[3]+1<<15)>>16)*m + 127) / 255; alpha > uint32(p.sheet[o+3]) {
		for c := range sum {
			p.sheet[o+c] = uint8((((sum[c]+1<<15)>>16)*m + 127) / 255)
		}
	}
}

// flatten merges the layer sheet onto the output and clears the sheet.
func (p *Puppet) flatten(r image.Rectangle) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			o := (y*p.w + x) * 4
			a := uint32(p.sheet[o+3])
			if a == 0 {
				continue
			}
			// Both layers show the same surface, so their coverage nests rather
			// than stacks: a soft edge drawn twice keeps the larger coverage.
			// Against an opaque layer beneath this is the usual "over".
			if under := uint32(p.out[o+3]); a >= under {
				copy(p.out[o:o+4], p.sheet[o:o+4])
			} else {
				for k := 0; k < 3; k++ {
					p.out[o+k] = uint8(min(under, uint32(p.sheet[o+k])+(uint32(p.out[o+k])*(under-a)+under/2)/under))
				}
			}
			p.sheet[o], p.sheet[o+1], p.sheet[o+2], p.sheet[o+3] = 0, 0, 0, 0
		}
	}
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

// ramp is a smoothstep from 0 at from to 1 at to; from may exceed to.
func ramp(v, from, to float64) float64 {
	t := clamp01((v - from) / (to - from))
	return t * t * (3 - 2*t)
}

// along interpolates a polyline of (key, value) pairs sorted by key.
func along(points [][2]float64, key float64) float64 {
	if key <= points[0][0] {
		return points[0][1]
	}
	for i := 1; i < len(points); i++ {
		if key <= points[i][0] {
			t := (key - points[i-1][0]) / (points[i][0] - points[i-1][0])
			return points[i-1][1] + (points[i][1]-points[i-1][1])*t
		}
	}
	return points[len(points)-1][1]
}
