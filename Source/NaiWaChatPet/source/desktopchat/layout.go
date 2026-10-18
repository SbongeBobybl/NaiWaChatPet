package desktopchat

import (
	"math"
	"time"
)

type Point struct{ X, Y int }
type Rect struct{ X, Y, W, H int }

func (r Rect) Right() int  { return r.X + r.W }
func (r Rect) Bottom() int { return r.Y + r.H }
func (r Rect) Contains(b Rect) bool {
	return b.W > 0 && b.H > 0 && b.X >= r.X && b.Y >= r.Y && b.Right() <= r.Right() && b.Bottom() <= r.Bottom()
}

type Placement struct {
	Rect Rect
	Tail string
	Tip  Point
}

func dip(v int, dpi uint32) int { return max(1, int(math.Round(float64(v)*float64(dpi)/96))) }
func clamp(r, work Rect) Rect {
	r.W = min(r.W, work.W)
	r.H = min(r.H, work.H)
	r.X = max(work.X, min(r.X, work.Right()-r.W))
	r.Y = max(work.Y, min(r.Y, work.Bottom()-r.H))
	return r
}
func Input(pet, work Rect, ground int, dpi uint32) Rect {
	w, h, gap := min(dip(340, dpi), work.W), min(dip(116, dpi), work.H), dip(12, dpi)
	r := Rect{pet.X + (pet.W-w)/2, ground + gap, w, h}
	return clamp(r, work)
}

// Reserve room beneath the pet instead of moving its input to a screen edge.
func PetForInput(pet, work Rect, ground int, dpi uint32) Rect {
	w, h, gap := min(dip(340, dpi), work.W), min(dip(116, dpi), work.H), dip(12, dpi)
	center := max(work.X+w/2, min(pet.X+pet.W/2, work.Right()-(w+1)/2))
	pet.X = center - pet.W/2
	groundOffset := ground - pet.Y
	pet.Y = max(work.Y, min(pet.Y, work.Bottom()-h-gap-groundOffset))
	return pet
}
func Bubble(pet, work Rect, mouth Point, dpi uint32, lines int, expanded bool) Placement {
	width := 400
	height := max(144, min(240, 108+lines*20))
	if expanded {
		width = 500
		height = 340
	}
	w, h, gap := min(dip(width, dpi), work.W), min(dip(height, dpi), work.H), dip(10, dpi)
	right := Rect{pet.Right() + gap, mouth.Y - dip(45, dpi), w, h}
	left := Rect{pet.X - w - gap, right.Y, w, h}
	var p Placement
	if right.Right() <= work.Right() {
		p = Placement{right, "left", mouth}
	} else if left.X >= work.X {
		p = Placement{left, "right", mouth}
	} else {
		// At large DPI, fit the bubble into actual free space instead of clamping
		// a full-height rectangle over the pet's face.
		above, below := pet.Y-work.Y-gap, work.Bottom()-pet.Bottom()-gap
		if above >= below && above > 0 {
			h = min(h, above)
			p = Placement{Rect{mouth.X - w/2, pet.Y - h - gap, w, h}, "bottom", mouth}
		} else if below > 0 {
			h = min(h, below)
			p = Placement{Rect{mouth.X - w/2, pet.Bottom() + gap, w, h}, "top", mouth}
		} else {
			w = max(1, max(pet.X-work.X-gap, work.Right()-pet.Right()-gap))
			if pet.X-work.X >= work.Right()-pet.Right() {
				p = Placement{Rect{pet.X - w - gap, right.Y, w, h}, "right", mouth}
			} else {
				p = Placement{Rect{pet.Right() + gap, right.Y, w, h}, "left", mouth}
			}
		}
	}
	p.Rect = clamp(p.Rect, work)
	return p
}

type Anchor struct {
	Frame int `json:"frame"`
	X, Y  float64
}

func Mouth(state string, frame, count int) Point {
	anchors := []Anchor{{0, 125, 110}}
	switch state {
	case "toLaugh", "video", "fromLaugh":
		anchors = []Anchor{{0, 125, 110}, {max(1, count/3), 205, 100}, {max(2, count-1), 190, 120}}
	case "toBack", "poop", "fromBack":
		anchors = []Anchor{{0, 125, 110}, {max(1, count/2), 118, 143}}
	case "fight", "toFight", "fromFight":
		anchors = []Anchor{{0, 134, 124}}
	}
	return Interpolate(anchors, frame)
}
func Interpolate(a []Anchor, frame int) Point {
	if len(a) == 0 {
		return Point{125, 110}
	}
	if frame <= a[0].Frame {
		return Point{int(a[0].X), int(a[0].Y)}
	}
	for i := 1; i < len(a); i++ {
		if frame <= a[i].Frame {
			d := a[i].Frame - a[i-1].Frame
			if d < 1 {
				d = 1
			}
			t := float64(frame-a[i-1].Frame) / float64(d)
			return Point{int(math.Round(a[i-1].X + (a[i].X-a[i-1].X)*t)), int(math.Round(a[i-1].Y + (a[i].Y-a[i-1].Y)*t))}
		}
	}
	last := a[len(a)-1]
	return Point{int(last.X), int(last.Y)}
}
func DismissAfter(text string) time.Duration {
	n := len([]rune(text))
	if n > 180 {
		return 0
	}
	return time.Duration(max(12, min(35, 8+n/5))) * time.Second
}
