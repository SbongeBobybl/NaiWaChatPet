package rig

import (
	"image"
	"math"
)

// Premultiplied, fixed-point bilinear rasterization avoids converting every
// overlapping region back to straight alpha. The original PNGs stay immutable.
func blitSkillRegion(out, src *image.RGBA, m Affine, opacity float64) {
	det := m.A*m.D - m.B*m.C
	if src == nil || math.Abs(det) < 1e-10 || opacity <= 0 {
		return
	}
	inv := Affine{A: m.D / det, B: -m.B / det, C: -m.C / det, D: m.A / det}
	inv.E = -inv.A*m.E - inv.C*m.F
	inv.F = -inv.B*m.E - inv.D*m.F
	x0, y0, x1, y1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range [4]Vec{{0, 0}, {float64(src.Rect.Dx()), 0}, {0, float64(src.Rect.Dy())}, {float64(src.Rect.Dx()), float64(src.Rect.Dy())}} {
		q := m.apply(p)
		x0 = math.Min(x0, q.X)
		y0 = math.Min(y0, q.Y)
		x1 = math.Max(x1, q.X)
		y1 = math.Max(y1, q.Y)
	}
	left, top, right, bottom := max(0, int(math.Floor(x0))), max(0, int(math.Floor(y0))), min(out.Rect.Dx(), int(math.Ceil(x1))), min(out.Rect.Dy(), int(math.Ceil(y1)))
	alpha := uint32(math.Min(255, opacity*255+.5))
	w, h := src.Rect.Dx(), src.Rect.Dy()
	for y := top; y < bottom; y++ {
		px, py := float64(left)+.5, float64(y)+.5
		u := (inv.A*px + inv.C*py + inv.E - .5) * 256
		v := (inv.B*px + inv.D*py + inv.F - .5) * 256
		for x := left; x < right; x, u, v = x+1, u+inv.A*256, v+inv.B*256 {
			fx, fy := int(math.Floor(u)), int(math.Floor(v))
			sx, sy := fx>>8, fy>>8
			if sx < -1 || sy < -1 || sx >= w || sy >= h {
				continue
			}
			ax, ay := uint32(fx&255), uint32(fy&255)
			weights := [4]uint32{(256 - ax) * (256 - ay), ax * (256 - ay), (256 - ax) * ay, ax * ay}
			var rgba [4]uint32
			for n, d := range [4][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
				xx, yy := sx+d[0], sy+d[1]
				if xx < 0 || yy < 0 || xx >= w || yy >= h {
					continue
				}
				o := yy*src.Stride + xx*4
				weight := weights[n]
				rgba[0] += uint32(src.Pix[o]) * weight
				rgba[1] += uint32(src.Pix[o+1]) * weight
				rgba[2] += uint32(src.Pix[o+2]) * weight
				rgba[3] += uint32(src.Pix[o+3]) * weight
			}
			a := (((rgba[3]+32768)>>16)*alpha + 127) / 255
			if a == 0 {
				continue
			}
			o := y*out.Stride + x*4
			keep := 255 - a
			for ch := 0; ch < 3; ch++ {
				value := (((rgba[ch]+32768)>>16)*alpha + 127) / 255
				out.Pix[o+ch] = uint8(min(255, value+(uint32(out.Pix[o+ch])*keep+127)/255))
			}
			out.Pix[o+3] = uint8(min(255, a+(uint32(out.Pix[o+3])*keep+127)/255))
		}
	}
}
func straightSkillFrame(src *image.RGBA) *image.NRGBA {
	out := image.NewNRGBA(src.Rect)
	for i := 0; i < len(src.Pix); i += 4 {
		a := uint32(src.Pix[i+3])
		if a == 0 {
			continue
		}
		out.Pix[i+3] = uint8(a)
		for ch := 0; ch < 3; ch++ {
			out.Pix[i+ch] = uint8(min(255, (uint32(src.Pix[i+ch])*255+a/2)/a))
		}
	}
	return out
}
