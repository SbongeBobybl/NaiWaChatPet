package skeletal

import (
	"fmt"
	"math"
)

// FrameEpsilon is added to frame times when sampling by frame index. The rig's
// key times are rounded to microseconds, so k/fps can land a hair before a key
// that belongs to frame k.
const FrameEpsilon = 1e-5

// BoneWorld is a bone's world matrix in Spine's y-up skeleton space.
type BoneWorld struct {
	A, B, C, D float64
	X, Y       float64
}

// SlotPose is one visible attachment in draw order.
type SlotPose struct {
	Slot       string
	Attachment string
	Region     string // atlas region; empty for points
	Point      bool
	Color      [4]float64
	// Verts is the region quad in skeleton space: bottom-left, top-left,
	// top-right, bottom-right, as spine-core's computeWorldVertices.
	Verts [8]float64
	// PX, PY is a point attachment's position in skeleton space.
	PX, PY float64
}

// Pose is a sampled skeleton.
type Pose struct {
	skel  *Skeleton
	Bones []BoneWorld
	Slots []SlotPose
}

type local struct {
	x, y, rot, sx, sy float64
}

// Setup returns the setup pose.
func (s *Skeleton) Setup() *Pose {
	p, _ := s.Sample("", 0)
	return p
}

// SampleFrame samples an animation at frame k of the rig's fps.
func (s *Skeleton) SampleFrame(anim string, k int) (*Pose, error) {
	return s.Sample(anim, float64(k)/float64(s.FPS)+FrameEpsilon)
}

// Sample poses the skeleton at time t (seconds) of anim, applied over the
// setup pose with full weight and no looping. anim "" is the setup pose.
func (s *Skeleton) Sample(anim string, t float64) (*Pose, error) {
	locals := make([]local, len(s.Bones))
	for i, b := range s.Bones {
		locals[i] = local{b.X, b.Y, b.Rotation, b.ScaleX, b.ScaleY}
	}
	attach := make([]string, len(s.Slots))
	colors := make([][4]float64, len(s.Slots))
	for i, sl := range s.Slots {
		attach[i] = sl.Attachment
		colors[i] = sl.Color
	}
	if anim != "" {
		a, ok := s.Animations[anim]
		if !ok {
			return nil, fmt.Errorf("no animation %q", anim)
		}
		for i := range a.bones {
			tl := &a.bones[i]
			if t < tl.times[0] {
				continue
			}
			v := tl.value(t)
			b, l := s.Bones[tl.target], &locals[tl.target]
			switch tl.kind {
			case kindRotate:
				l.rot = b.Rotation + v[0]
			case kindTranslate:
				l.x, l.y = b.X+v[0], b.Y+v[1]
			case kindScale:
				l.sx, l.sy = b.ScaleX*v[0], b.ScaleY*v[1]
			}
		}
		for i := range a.slots {
			st := &a.slots[i]
			if t < st.times[0] {
				continue
			}
			if st.attachment {
				if name, ok := st.attachmentAt(t); ok {
					attach[st.target] = name
				} else {
					attach[st.target] = ""
				}
				continue
			}
			v := st.value(t)
			colors[st.target] = [4]float64{clamp01(v[0]), clamp01(v[1]), clamp01(v[2]), clamp01(v[3])}
		}
	}
	p := &Pose{skel: s, Bones: make([]BoneWorld, len(s.Bones))}
	for i, b := range s.Bones {
		l := locals[i]
		r := l.rot * math.Pi / 180
		ry := (l.rot + 90) * math.Pi / 180
		la, lb := math.Cos(r)*l.sx, math.Cos(ry)*l.sy
		lc, ld := math.Sin(r)*l.sx, math.Sin(ry)*l.sy
		if b.Parent < 0 {
			p.Bones[i] = BoneWorld{la, lb, lc, ld, l.x, l.y}
			continue
		}
		pw := p.Bones[b.Parent]
		p.Bones[i] = BoneWorld{
			A: pw.A*la + pw.B*lc, B: pw.A*lb + pw.B*ld,
			C: pw.C*la + pw.D*lc, D: pw.C*lb + pw.D*ld,
			X: pw.A*l.x + pw.B*l.y + pw.X, Y: pw.C*l.x + pw.D*l.y + pw.Y,
		}
	}
	for i, sl := range s.Slots {
		if attach[i] == "" {
			continue
		}
		att, ok := s.skin[sl.Name][attach[i]]
		if !ok {
			return nil, fmt.Errorf("slot %s has no attachment %q", sl.Name, attach[i])
		}
		w := p.Bones[sl.Bone]
		sp := SlotPose{Slot: sl.Name, Attachment: att.Name, Region: att.Path, Point: att.Point, Color: colors[i]}
		if att.Point {
			sp.Region = ""
			sp.PX, sp.PY = w.apply(att.X, att.Y)
		} else {
			x0, x1 := att.X-att.Width/2, att.X+att.Width/2
			y0, y1 := att.Y-att.Height/2, att.Y+att.Height/2
			for k, c := range [4][2]float64{{x0, y0}, {x0, y1}, {x1, y1}, {x1, y0}} {
				sp.Verts[k*2], sp.Verts[k*2+1] = w.apply(c[0], c[1])
			}
		}
		p.Slots = append(p.Slots, sp)
	}
	return p, nil
}

func (w BoneWorld) apply(x, y float64) (float64, float64) {
	return w.A*x + w.B*y + w.X, w.C*x + w.D*y + w.Y
}

// Bone returns a bone's world matrix by name.
func (p *Pose) Bone(name string) (BoneWorld, bool) {
	i, ok := p.skel.boneIndex[name]
	if !ok {
		return BoneWorld{}, false
	}
	return p.Bones[i], true
}

// Point returns a visible point attachment's position in canvas pixels
// (origin top-left, y down), e.g. "mouth".
func (p *Pose) Point(name string) (x, y float64, ok bool) {
	for _, s := range p.Slots {
		if s.Point && s.Attachment == name {
			x, y = p.skel.ToCanvas(s.PX, s.PY)
			return x, y, true
		}
	}
	return 0, 0, false
}

// ToCanvas maps skeleton space to the clip canvas (top-left origin, y down).
func (s *Skeleton) ToCanvas(x, y float64) (float64, float64) {
	return x - s.X, s.Y + s.Height - y
}

// FrameCount is the number of frames a clip of anim has at the rig's fps,
// counting both ends.
func (s *Skeleton) FrameCount(anim string) int {
	a, ok := s.Animations[anim]
	if !ok {
		return 0
	}
	return int(math.Round(a.Duration*float64(s.FPS))) + 1
}

// EventsBetween returns anim's events with from < time <= to.
func (a *Animation) EventsBetween(from, to float64) []Event {
	var out []Event
	for _, e := range a.Events {
		if e.Time > from && e.Time <= to {
			out = append(out, e)
		}
	}
	return out
}

func clamp01(v float64) float64 {
	return math.Max(0, math.Min(1, v))
}
