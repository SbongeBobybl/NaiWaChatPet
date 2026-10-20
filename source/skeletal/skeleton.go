// Package skeletal is an optional, minimal Spine 4.2 JSON runtime for the
// skill rig in skeletal/. It reads the subset the rig uses (bones with normal
// inheritance, region and point attachments, rotate/translate/scale bone
// timelines, attachment/rgba slot timelines, linear/stepped/bezier curves,
// events) and samples poses the way spine-core does.
//
// It is not wired into emotion scheduling: skills keep playing their frame
// clips, and this package is an alternative source for the same poses.
package skeletal

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
)

// Skeleton is the parsed setup pose plus animations.
type Skeleton struct {
	Spine      string
	FPS        int
	X, Y       float64
	Width      float64
	Height     float64
	Bones      []BoneData
	Slots      []SlotData
	Animations map[string]*Animation

	boneIndex map[string]int
	slotIndex map[string]int
	skin      map[string]map[string]Attachment
}

type BoneData struct {
	Name     string
	Parent   int // -1 for the root
	X, Y     float64
	Rotation float64
	ScaleX   float64
	ScaleY   float64
}

type SlotData struct {
	Name       string
	Bone       int
	Attachment string
	Color      [4]float64
}

// Attachment is a region (quad) or a point (e.g. the mouth anchor).
type Attachment struct {
	Name   string
	Point  bool
	Path   string // atlas region name
	X, Y   float64
	Width  float64
	Height float64
}

// Event is a named marker on an animation's timeline.
type Event struct {
	Time float64
	Name string
}

type Animation struct {
	Name     string
	Duration float64
	Events   []Event
	bones    []boneTimeline
	slots    []slotTimeline
}

type timelineKind int

const (
	kindRotate timelineKind = iota
	kindTranslate
	kindScale
	kindRGBA
)

// curveTimeline stores keys like spine-core: times, per-key values, and per
// key either linear, stepped, or a precomputed bezier per value channel.
type curveTimeline struct {
	kind   timelineKind
	target int
	times  []float64
	values [][]float64
	curves []curve
}

type curveType int

const (
	curveLinear curveType = iota
	curveStepped
	curveBezier
)

type curve struct {
	typ    curveType
	bezier [][bezierSize]float64 // per channel
}

type boneTimeline = curveTimeline

type slotTimeline struct {
	curveTimeline
	attachment bool
	names      []string
	hasName    []bool
}

// Load reads a Spine JSON skeleton file.
func Load(path string) (*Skeleton, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(raw)
}

type jsonFile struct {
	Skeleton struct {
		Spine  string  `json:"spine"`
		FPS    int     `json:"fps"`
		X      float64 `json:"x"`
		Y      float64 `json:"y"`
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	} `json:"skeleton"`
	Bones []struct {
		Name     string   `json:"name"`
		Parent   string   `json:"parent"`
		X        float64  `json:"x"`
		Y        float64  `json:"y"`
		Rotation float64  `json:"rotation"`
		ScaleX   *float64 `json:"scaleX"`
		ScaleY   *float64 `json:"scaleY"`
		ShearX   float64  `json:"shearX"`
		ShearY   float64  `json:"shearY"`
		Inherit  string   `json:"inherit"`
	} `json:"bones"`
	Slots []struct {
		Name       string `json:"name"`
		Bone       string `json:"bone"`
		Attachment string `json:"attachment"`
		Color      string `json:"color"`
		Blend      string `json:"blend"`
	} `json:"slots"`
	Skins []struct {
		Name        string                               `json:"name"`
		Attachments map[string]map[string]jsonAttachment `json:"attachments"`
	} `json:"skins"`
	Animations map[string]struct {
		Bones  map[string]map[string][]jsonKey `json:"bones"`
		Slots  map[string]map[string][]jsonKey `json:"slots"`
		Events []struct {
			Time float64 `json:"time"`
			Name string  `json:"name"`
		} `json:"events"`
	} `json:"animations"`
}

type jsonAttachment struct {
	Type     string   `json:"type"`
	Path     string   `json:"path"`
	X        float64  `json:"x"`
	Y        float64  `json:"y"`
	Width    float64  `json:"width"`
	Height   float64  `json:"height"`
	Rotation float64  `json:"rotation"`
	ScaleX   *float64 `json:"scaleX"`
	ScaleY   *float64 `json:"scaleY"`
}

type jsonKey struct {
	Time  float64         `json:"time"`
	Value *float64        `json:"value"`
	X     *float64        `json:"x"`
	Y     *float64        `json:"y"`
	Color string          `json:"color"`
	Name  *string         `json:"name"`
	Curve json.RawMessage `json:"curve"`
}

// Parse reads Spine 4.2 JSON. Features outside the rig's subset are rejected
// rather than silently ignored.
func Parse(raw []byte) (*Skeleton, error) {
	var f jsonFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	s := &Skeleton{
		Spine: f.Skeleton.Spine, FPS: f.Skeleton.FPS,
		X: f.Skeleton.X, Y: f.Skeleton.Y, Width: f.Skeleton.Width, Height: f.Skeleton.Height,
		Animations: map[string]*Animation{},
		boneIndex:  map[string]int{}, slotIndex: map[string]int{}, skin: map[string]map[string]Attachment{},
	}
	if s.FPS <= 0 {
		s.FPS = 30
	}
	for i, b := range f.Bones {
		if b.ShearX != 0 || b.ShearY != 0 || (b.Inherit != "" && b.Inherit != "normal") {
			return nil, fmt.Errorf("bone %s: shear and non-normal inherit are not supported", b.Name)
		}
		parent := -1
		if b.Parent != "" {
			p, ok := s.boneIndex[b.Parent]
			if !ok {
				return nil, fmt.Errorf("bone %s: parent %s must come first", b.Name, b.Parent)
			}
			parent = p
		} else if i != 0 {
			return nil, fmt.Errorf("bone %s has no parent", b.Name)
		}
		s.boneIndex[b.Name] = i
		s.Bones = append(s.Bones, BoneData{
			Name: b.Name, Parent: parent, X: b.X, Y: b.Y, Rotation: b.Rotation,
			ScaleX: orOne(b.ScaleX), ScaleY: orOne(b.ScaleY),
		})
	}
	for i, sl := range f.Slots {
		bi, ok := s.boneIndex[sl.Bone]
		if !ok {
			return nil, fmt.Errorf("slot %s: unknown bone %s", sl.Name, sl.Bone)
		}
		if sl.Blend != "" && sl.Blend != "normal" {
			return nil, fmt.Errorf("slot %s: blend %s is not supported", sl.Name, sl.Blend)
		}
		col := [4]float64{1, 1, 1, 1}
		if sl.Color != "" {
			c, err := parseColor(sl.Color)
			if err != nil {
				return nil, fmt.Errorf("slot %s: %w", sl.Name, err)
			}
			col = c
		}
		s.slotIndex[sl.Name] = i
		s.Slots = append(s.Slots, SlotData{Name: sl.Name, Bone: bi, Attachment: sl.Attachment, Color: col})
	}
	for _, sk := range f.Skins {
		if sk.Name != "default" {
			continue
		}
		for slot, atts := range sk.Attachments {
			if _, ok := s.slotIndex[slot]; !ok {
				return nil, fmt.Errorf("skin: unknown slot %s", slot)
			}
			m := map[string]Attachment{}
			for name, a := range atts {
				if a.Rotation != 0 || orOne(a.ScaleX) != 1 || orOne(a.ScaleY) != 1 {
					return nil, fmt.Errorf("attachment %s/%s: rotation and scale are not supported", slot, name)
				}
				switch a.Type {
				case "", "region":
					path := a.Path
					if path == "" {
						path = name
					}
					m[name] = Attachment{Name: name, Path: path, X: a.X, Y: a.Y, Width: a.Width, Height: a.Height}
				case "point":
					m[name] = Attachment{Name: name, Point: true, X: a.X, Y: a.Y}
				default:
					return nil, fmt.Errorf("attachment %s/%s: type %s is not supported", slot, name, a.Type)
				}
			}
			s.skin[slot] = m
		}
	}
	for name, ja := range f.Animations {
		a := &Animation{Name: name}
		for bone, tls := range ja.Bones {
			bi, ok := s.boneIndex[bone]
			if !ok {
				return nil, fmt.Errorf("%s: unknown bone %s", name, bone)
			}
			for kind, keys := range tls {
				var k timelineKind
				var channels []string
				switch kind {
				case "rotate":
					k, channels = kindRotate, []string{"value"}
				case "translate":
					k, channels = kindTranslate, []string{"x", "y"}
				case "scale":
					k, channels = kindScale, []string{"x", "y"}
				default:
					return nil, fmt.Errorf("%s/%s: bone timeline %s is not supported", name, bone, kind)
				}
				tl, err := readCurveTimeline(k, bi, keys, channels)
				if err != nil {
					return nil, fmt.Errorf("%s/%s/%s: %w", name, bone, kind, err)
				}
				a.bones = append(a.bones, tl)
			}
		}
		for slot, tls := range ja.Slots {
			si, ok := s.slotIndex[slot]
			if !ok {
				return nil, fmt.Errorf("%s: unknown slot %s", name, slot)
			}
			for kind, keys := range tls {
				switch kind {
				case "attachment":
					st := slotTimeline{attachment: true, curveTimeline: curveTimeline{target: si}}
					for _, key := range keys {
						st.times = append(st.times, key.Time)
						if key.Name != nil {
							st.names = append(st.names, *key.Name)
							st.hasName = append(st.hasName, true)
						} else {
							st.names = append(st.names, "")
							st.hasName = append(st.hasName, false)
						}
					}
					a.slots = append(a.slots, st)
				case "rgba":
					tl, err := readCurveTimeline(kindRGBA, si, keys, []string{"r", "g", "b", "a"})
					if err != nil {
						return nil, fmt.Errorf("%s/%s/rgba: %w", name, slot, err)
					}
					a.slots = append(a.slots, slotTimeline{curveTimeline: tl})
				default:
					return nil, fmt.Errorf("%s/%s: slot timeline %s is not supported", name, slot, kind)
				}
			}
		}
		for _, e := range ja.Events {
			a.Events = append(a.Events, Event{Time: e.Time, Name: e.Name})
		}
		sort.SliceStable(a.Events, func(i, j int) bool { return a.Events[i].Time < a.Events[j].Time })
		a.Duration = a.computeDuration()
		// Bone timelines are applied in a fixed order so results never depend on map iteration.
		sort.SliceStable(a.bones, func(i, j int) bool {
			if a.bones[i].target != a.bones[j].target {
				return a.bones[i].target < a.bones[j].target
			}
			return a.bones[i].kind < a.bones[j].kind
		})
		sort.SliceStable(a.slots, func(i, j int) bool { return a.slots[i].target < a.slots[j].target })
		s.Animations[name] = a
	}
	return s, nil
}

func (a *Animation) computeDuration() float64 {
	d := 0.0
	for _, t := range a.bones {
		d = math.Max(d, t.times[len(t.times)-1])
	}
	for _, t := range a.slots {
		d = math.Max(d, t.times[len(t.times)-1])
	}
	for _, e := range a.Events {
		d = math.Max(d, e.Time)
	}
	return d
}

func readCurveTimeline(kind timelineKind, target int, keys []jsonKey, channels []string) (curveTimeline, error) {
	tl := curveTimeline{kind: kind, target: target}
	if len(keys) == 0 {
		return tl, fmt.Errorf("no keys")
	}
	def := 0.0
	if kind == kindScale {
		def = 1
	}
	for _, k := range keys {
		var vals []float64
		switch kind {
		case kindRotate:
			vals = []float64{orDefault(k.Value, def)}
		case kindTranslate, kindScale:
			vals = []float64{orDefault(k.X, def), orDefault(k.Y, def)}
		case kindRGBA:
			c, err := parseColor(k.Color)
			if err != nil {
				return tl, err
			}
			vals = c[:]
		}
		tl.times = append(tl.times, k.Time)
		tl.values = append(tl.values, vals)
	}
	for i, k := range keys {
		c := curve{typ: curveLinear}
		if len(k.Curve) > 0 && i < len(keys)-1 {
			var name string
			var nums []float64
			switch {
			case json.Unmarshal(k.Curve, &name) == nil:
				if name != "stepped" {
					return tl, fmt.Errorf("curve %q is not supported", name)
				}
				c.typ = curveStepped
			case json.Unmarshal(k.Curve, &nums) == nil:
				if len(nums) != 4*len(channels) {
					return tl, fmt.Errorf("bezier has %d numbers, want %d", len(nums), 4*len(channels))
				}
				c.typ = curveBezier
				t1, t2 := tl.times[i], tl.times[i+1]
				for ch := range channels {
					v1, v2 := tl.values[i][ch], tl.values[i+1][ch]
					c.bezier = append(c.bezier, setBezier(t1, v1, nums[ch*4], nums[ch*4+1], nums[ch*4+2], nums[ch*4+3], t2, v2))
				}
			default:
				return tl, fmt.Errorf("bad curve %s", string(k.Curve))
			}
		}
		if i == len(keys)-1 {
			c.typ = curveStepped
		}
		tl.curves = append(tl.curves, c)
	}
	return tl, nil
}

const bezierSize = 18

// setBezier samples a bezier segment with forward differences, exactly like
// spine-core's CurveTimeline.setBezier.
func setBezier(time1, value1, cx1, cy1, cx2, cy2, time2, value2 float64) [bezierSize]float64 {
	var out [bezierSize]float64
	tmpx, tmpy := (time1-cx1*2+cx2)*0.03, (value1-cy1*2+cy2)*0.03
	dddx, dddy := ((cx1-cx2)*3-time1+time2)*0.006, ((cy1-cy2)*3-value1+value2)*0.006
	ddx, ddy := tmpx*2+dddx, tmpy*2+dddy
	dx, dy := (cx1-time1)*0.3+tmpx+dddx*0.16666667, (cy1-value1)*0.3+tmpy+dddy*0.16666667
	x, y := time1+dx, value1+dy
	for i := 0; i < bezierSize; i += 2 {
		out[i], out[i+1] = x, y
		dx += ddx
		dy += ddy
		ddx += dddx
		ddy += dddy
		x += dx
		y += dy
	}
	return out
}

func (tl *curveTimeline) value(t float64) []float64 {
	i := len(tl.times) - 1
	for k := 1; k < len(tl.times); k++ {
		if tl.times[k] > t {
			i = k - 1
			break
		}
	}
	c := tl.curves[i]
	switch c.typ {
	case curveStepped:
		return tl.values[i]
	case curveLinear:
		t0, t1 := tl.times[i], tl.times[i+1]
		f := (t - t0) / (t1 - t0)
		out := make([]float64, len(tl.values[i]))
		for ch := range out {
			out[ch] = tl.values[i][ch] + f*(tl.values[i+1][ch]-tl.values[i][ch])
		}
		return out
	}
	out := make([]float64, len(tl.values[i]))
	for ch := range out {
		out[ch] = bezierValue(c.bezier[ch], t, tl.times[i], tl.values[i][ch], tl.times[i+1], tl.values[i+1][ch])
	}
	return out
}

func bezierValue(b [bezierSize]float64, t, t0, v0, t1, v1 float64) float64 {
	if b[0] > t {
		return v0 + (t-t0)/(b[0]-t0)*(b[1]-v0)
	}
	for i := 2; i < bezierSize; i += 2 {
		if b[i] >= t {
			x, y := b[i-2], b[i-1]
			return y + (t-x)/(b[i]-x)*(b[i+1]-y)
		}
	}
	x, y := b[bezierSize-2], b[bezierSize-1]
	return y + (t-x)/(t1-x)*(v1-y)
}

func (st *slotTimeline) attachmentAt(t float64) (string, bool) {
	i := len(st.times) - 1
	for k := 1; k < len(st.times); k++ {
		if st.times[k] > t {
			i = k - 1
			break
		}
	}
	return st.names[i], st.hasName[i]
}

func orOne(v *float64) float64 { return orDefault(v, 1) }

func orDefault(v *float64, def float64) float64 {
	if v == nil {
		return def
	}
	return *v
}

func parseColor(hex string) ([4]float64, error) {
	var c [4]float64
	if len(hex) != 8 && len(hex) != 6 {
		return c, fmt.Errorf("bad color %q", hex)
	}
	c[3] = 1
	for i := 0; i < len(hex)/2; i++ {
		v, err := strconv.ParseUint(hex[i*2:i*2+2], 16, 8)
		if err != nil {
			return c, fmt.Errorf("bad color %q", hex)
		}
		c[i] = float64(v) / 255
	}
	return c, nil
}
