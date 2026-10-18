package main

import (
	"image"
	"image/color"
	"math"
)

// Render the entire moving face over its matching skin crop. Native BUTTON
// windows still own hit testing, keyboard navigation and accessible captions.
func buttonFace(bg *image.NRGBA, origin image.Point, w, h int, scale, press float64, gold, hover, focus bool) ([]byte, int) {
	press = math.Max(0, math.Min(1, press))
	pixels := make([]byte, w*h*4)
	shift := 2 * scale * press
	left, top := 2*scale+float64(w)*.012*press, 2*scale+shift
	right, bottom := float64(w)-2*scale-float64(w)*.012*press, float64(h)-5*scale+shift-float64(h)*.04*press
	radius := math.Min((bottom-top)/2, 14*scale)
	distance := func(x, y, dy float64) float64 {
		qx := math.Abs(x-(left+right)/2) - ((right-left)/2 - radius)
		qy := math.Abs(y-(top+bottom)/2-dy) - ((bottom-top)/2 - radius)
		return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - radius
	}
	blend := func(a, b color.NRGBA, t float64) color.NRGBA {
		t = math.Max(0, math.Min(1, t))
		return color.NRGBA{uint8(float64(a.R)*(1-t) + float64(b.R)*t), uint8(float64(a.G)*(1-t) + float64(b.G)*t), uint8(float64(a.B)*(1-t) + float64(b.B)*t), 255}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := color.NRGBA{255, 242, 202, 255}
			if bg != nil {
				p = bg.NRGBAAt(x+origin.X, y+origin.Y)
				if p.A < 16 {
					p.R, p.G, p.B = 1, 0, 1
				}
			}
			fx, fy := float64(x)+.5, float64(y)+.5
			d := distance(fx, fy, 0)
			sd := distance(fx, fy, (2.5-1.8*press)*scale)
			if sd < 2*scale {
				a := .23 * (1 - .6*press) * math.Max(0, math.Min(1, (2*scale-sd)/(3*scale)))
				p = blend(p, color.NRGBA{192, 116, 17, 255}, a)
			}
			if d < .6 {
				t := math.Max(0, math.Min(1, (fy-top)/(bottom-top)))
				face := blend(color.NRGBA{255, 255, 244, 255}, color.NRGBA{255, 235, 182, 255}, t)
				if gold {
					face = blend(color.NRGBA{255, 242, 157, 255}, color.NRGBA{255, 199, 71, 255}, t)
				}
				if hover {
					face = blend(face, color.NRGBA{255, 255, 237, 255}, .14)
				}
				face = blend(face, color.NRGBA{229, 176, 69, 255}, .10*press)
				if d > -1.4*scale {
					face = color.NRGBA{245, 182, 55, 255}
				}
				if d <= -1.4*scale && d > -2.8*scale && fy < (top+bottom)*.54 {
					face = color.NRGBA{255, 255, 239, 255}
				}
				if focus && d < -3.2*scale && d > -4.5*scale {
					face = color.NRGBA{141, 92, 12, 255}
				}
				p = blend(p, face, math.Max(0, math.Min(1, .6-d)))
			}
			i := (y*w + x) * 4
			pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = p.B, p.G, p.R, 255
		}
	}
	return pixels, int(math.Round(shift - 1*scale))
}
