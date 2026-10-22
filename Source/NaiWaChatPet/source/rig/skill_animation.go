package rig

// Read-only adapter for the region/point subset used by the supplied Spine
// 4.2 export. It does not rewrite, bake or create skeletal keyframes.
import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"io/fs"
	"math"
	"path"
	"sort"
	"strings"
)

type AnimationEvent struct {
	Time float64 `json:"time"`
	Name string  `json:"name"`
}
type spineKey struct {
	controls []float64
	alpha    float64
	Time     float64         `json:"time"`
	Value    float64         `json:"value"`
	X        *float64        `json:"x"`
	Y        *float64        `json:"y"`
	Name     *string         `json:"name"`
	Color    string          `json:"color"`
	Curve    json.RawMessage `json:"curve"`
}
type spineBone struct {
	Name           string `json:"name"`
	Parent         string `json:"parent"`
	X, Y, Rotation float64
	ScaleX, ScaleY *float64
}
type spineSlot struct {
	Name       string `json:"name"`
	Bone       string `json:"bone"`
	Attachment string `json:"attachment"`
	Color      string `json:"color"`
}
type spineAttachment struct {
	Type, Path                    string
	X, Y, Width, Height, Rotation float64
	ScaleX, ScaleY                *float64
}
type spineAnimation struct {
	Bones  map[string]map[string][]spineKey `json:"bones"`
	Slots  map[string]map[string][]spineKey `json:"slots"`
	Events []AnimationEvent                 `json:"events"`
}
type spineDocument struct {
	Skeleton struct {
		Spine         string `json:"spine"`
		Width, Height float64
	} `json:"skeleton"`
	Bones []spineBone `json:"bones"`
	Slots []spineSlot `json:"slots"`
	Skins []struct {
		Name        string
		Attachments map[string]map[string]spineAttachment
	} `json:"skins"`
	Animations map[string]spineAnimation `json:"animations"`
}

// SkillAnimations shares rig's affine and canvas coordinate system but keeps
// the supplied skeleton independent of the older Stand/Stance mesh topology.
type SkillAnimations struct {
	premult   map[string]*image.RGBA
	doc       spineDocument
	index     map[string]int
	parents   []int
	images    map[string]*image.NRGBA
	durations map[string]float64
}

func LoadSkillAnimations(source fs.FS, filename string) (*SkillAnimations, error) {
	b, err := fs.ReadFile(source, filename)
	if err != nil {
		return nil, err
	}
	if len(b) > 16<<20 {
		return nil, fmt.Errorf("skill skeleton too large")
	}
	l := &SkillAnimations{index: map[string]int{}, images: map[string]*image.NRGBA{}, premult: map[string]*image.RGBA{}, durations: map[string]float64{}}
	if err = json.Unmarshal(b, &l.doc); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(l.doc.Skeleton.Spine, "4.2.") || len(l.doc.Bones) == 0 || len(l.doc.Bones) > 255 || len(l.doc.Skins) != 1 {
		return nil, fmt.Errorf("unsupported skeleton version/bones/skins: %s/%d/%d", l.doc.Skeleton.Spine, len(l.doc.Bones), len(l.doc.Skins))
	}
	for i, bone := range l.doc.Bones {
		parent := -1
		if bone.Parent != "" {
			var ok bool
			parent, ok = l.index[bone.Parent]
			if !ok {
				return nil, fmt.Errorf("bone %s: missing or out-of-order parent %s", bone.Name, bone.Parent)
			}
		}
		if _, exists := l.index[bone.Name]; exists {
			return nil, fmt.Errorf("duplicate bone %s", bone.Name)
		}
		l.index[bone.Name] = i
		l.parents = append(l.parents, parent)
	}
	for _, slot := range l.doc.Slots {
		if _, ok := l.index[slot.Bone]; !ok {
			return nil, fmt.Errorf("slot %s: missing bone %s", slot.Name, slot.Bone)
		}
	}
	for slot, attachments := range l.doc.Skins[0].Attachments {
		for name, a := range attachments {
			if a.Type == "point" {
				continue
			}
			if a.Type != "" && a.Type != "region" {
				return nil, fmt.Errorf("slot %s/%s: unsupported attachment %s", slot, name, a.Type)
			}
			key := a.Path
			if key == "" {
				key = name
			}
			if !fs.ValidPath(key) || strings.Contains(key, "..") {
				return nil, fmt.Errorf("unsafe attachment path")
			}
			if l.images[key] != nil {
				continue
			}
			f, e := source.Open(path.Join(path.Dir(filename), "images", key+".png"))
			if e != nil {
				return nil, fmt.Errorf("attachment %s: %w", key, e)
			}
			im, _, e := image.Decode(f)
			f.Close()
			if e != nil {
				return nil, e
			}
			if im.Bounds().Dx() > 4096 || im.Bounds().Dy() > 4096 {
				return nil, fmt.Errorf("attachment dimensions too large")
			}
			n := image.NewNRGBA(image.Rect(0, 0, im.Bounds().Dx(), im.Bounds().Dy()))
			for y := 0; y < n.Rect.Dy(); y++ {
				for x := 0; x < n.Rect.Dx(); x++ {
					n.Set(x, y, im.At(x+im.Bounds().Min.X, y+im.Bounds().Min.Y))
				}
			}
			l.images[key] = n
			pm := image.NewRGBA(n.Rect)
			for i := 0; i < len(n.Pix); i += 4 {
				a := uint32(n.Pix[i+3])
				pm.Pix[i] = uint8((uint32(n.Pix[i])*a + 127) / 255)
				pm.Pix[i+1] = uint8((uint32(n.Pix[i+1])*a + 127) / 255)
				pm.Pix[i+2] = uint8((uint32(n.Pix[i+2])*a + 127) / 255)
				pm.Pix[i+3] = n.Pix[i+3]
			}
			l.premult[key] = pm
		}
	}
	for name, a := range l.doc.Animations {
		for bone, tracks := range a.Bones {
			if _, ok := l.index[bone]; !ok {
				return nil, fmt.Errorf("animation %s missing bone %s", name, bone)
			}
			for track, keys := range tracks {
				if track != "rotate" && track != "translate" && track != "scale" {
					return nil, fmt.Errorf("unsupported bone track %s/%s", bone, track)
				}
				for i := range keys {
					k := &keys[i]
					if len(k.Curve) > 0 && string(k.Curve) != `"stepped"` {
						if err := json.Unmarshal(k.Curve, &k.controls); err != nil {
							return nil, fmt.Errorf("invalid curve %s/%s", bone, track)
						}
					}
				}
				for _, k := range keys {
					l.durations[name] = math.Max(l.durations[name], k.Time)
				}
			}
		}
		for slot, tracks := range a.Slots {
			for track, keys := range tracks {
				for i := range keys {
					k := &keys[i]
					if len(k.Curve) > 0 && string(k.Curve) != `"stepped"` {
						if err := json.Unmarshal(k.Curve, &k.controls); err != nil {
							return nil, fmt.Errorf("invalid slot curve %s/%s", slot, track)
						}
					}
					k.alpha = 1
					if len(k.Color) == 8 {
						var alpha uint8
						if _, err := fmt.Sscanf(k.Color[6:], "%02x", &alpha); err == nil {
							k.alpha = float64(alpha) / 255
						}
					}
				}
				if track != "attachment" && track != "rgba" && track != "color" && track != "alpha" {
					return nil, fmt.Errorf("unsupported slot track %s/%s", slot, track)
				}
				for _, k := range keys {
					l.durations[name] = math.Max(l.durations[name], k.Time)
				}
			}
		}
		for _, e := range a.Events {
			l.durations[name] = math.Max(l.durations[name], e.Time)
		}
	}
	return l, nil
}

func (l *SkillAnimations) GetAnimationDuration(name string) float64 { return l.durations[name] }
func (l *SkillAnimations) GetAnimationEvents(name string) []AnimationEvent {
	return append([]AnimationEvent(nil), l.doc.Animations[name].Events...)
}
func (l *SkillAnimations) Names() []string {
	names := make([]string, 0, len(l.durations))
	for n := range l.durations {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
func number(p *float64, fallback float64) float64 {
	if p != nil {
		return *p
	}
	return fallback
}
func keyValue(k spineKey, axis string, fallback float64) float64 {
	switch axis {
	case "x":
		return number(k.X, fallback)
	case "y":
		return number(k.Y, fallback)
	case "alpha":
		return k.alpha
	}
	return k.Value
}

// Spine 4.2 Bezier control points contain absolute time/value pairs, with
// separate four-number groups for X and Y. Never replace them with an easing.
func sampleTrack(keys []spineKey, t float64, axis string, fallback float64) float64 {
	if len(keys) == 0 || t < keys[0].Time {
		return fallback
	}
	i := sort.Search(len(keys), func(i int) bool { return keys[i].Time > t }) - 1
	a := keys[i]
	av := keyValue(a, axis, fallback)
	if i+1 == len(keys) {
		return av
	}
	b := keys[i+1]
	bv := keyValue(b, axis, fallback)
	if string(a.Curve) == `"stepped"` {
		return av
	}
	controls := a.controls
	if controls == nil && len(a.Curve) > 0 {
		_ = json.Unmarshal(a.Curve, &controls)
	}
	if len(controls) >= 4 {
		offset := 0
		if axis == "y" && len(controls) >= 8 {
			offset = 4
		}
		if axis == "alpha" && len(controls) >= 16 {
			offset = 12
		}
		c := controls[offset : offset+4]
		cubic := func(u, p0, p1, p2, p3 float64) float64 {
			v := 1 - u
			return v*v*v*p0 + 3*v*v*u*p1 + 3*v*u*u*p2 + u*u*u*p3
		}
		// The supplied spine-core reference samples Bezier time/value curves
		// at ten uniform parameter intervals, then interpolates between them.
		// Match that sampling convention instead of solving a different curve.
		previousTime, previousValue := a.Time, av
		for sample := 1; sample <= 10; sample++ {
			u := float64(sample) / 10
			nextTime := cubic(u, a.Time, c[0], c[2], b.Time)
			nextValue := cubic(u, av, c[1], c[3], bv)
			if t <= nextTime || sample == 10 {
				if nextTime <= previousTime {
					return nextValue
				}
				return previousValue + (nextValue-previousValue)*(t-previousTime)/(nextTime-previousTime)
			}
			previousTime, previousValue = nextTime, nextValue
		}
		return bv
	}
	if b.Time <= a.Time {
		return bv
	}
	return av + (bv-av)*(t-a.Time)/(b.Time-a.Time)
}

func (l *SkillAnimations) transforms(name string, t float64, stripRoot bool) []Affine {
	world := make([]Affine, len(l.doc.Bones))
	a := l.doc.Animations[name]
	for i, b := range l.doc.Bones {
		tracks := a.Bones[b.Name]
		x, y := b.X+sampleTrack(tracks["translate"], t, "x", 0), b.Y+sampleTrack(tracks["translate"], t, "y", 0)
		if stripRoot && i == 0 {
			x, y = b.X, b.Y
		}
		m := rotation((b.Rotation + sampleTrack(tracks["rotate"], t, "", 0)) * math.Pi / 180)
		sx, sy := number(b.ScaleX, 1)*sampleTrack(tracks["scale"], t, "x", 1), number(b.ScaleY, 1)*sampleTrack(tracks["scale"], t, "y", 1)
		m.A *= sx
		m.B *= sx
		m.C *= sy
		m.D *= sy
		m.E = x
		m.F = y
		if l.parents[i] >= 0 {
			m = world[l.parents[i]].mul(m)
		}
		world[i] = m
	}
	return world
}

// GetBoneAnchor returns authored canvas coordinates (Y down), with root motion
// removed when it is already applied by Combat to the fighter's world position.
func (l *SkillAnimations) GetBoneAnchor(name, bone string, t float64, stripRoot bool) (Vec, bool) {
	i, ok := l.index[bone]
	if !ok {
		return Vec{}, false
	}
	p := l.transforms(name, t, stripRoot)[i].apply(Vec{})
	return Vec{240 + p.X, GroundY - p.Y}, true
}
func (l *SkillAnimations) RootTranslation(name string, t float64) Vec {
	tr := l.doc.Animations[name].Bones["root"]["translate"]
	return Vec{sampleTrack(tr, t, "x", 0), sampleTrack(tr, t, "y", 0)}
}

// RenderSkill uses the user's original region images. Detached fan and demo
// explosion slots are excluded by the host so authoritative projectiles/VFX
// appear at the actual collision instead of the asset's demonstration position.
func (l *SkillAnimations) RenderSkill(name string, t float64, width, height int, gameplay bool) (*image.NRGBA, error) {
	return l.RenderSkillScaled(name, t, width, height, gameplay, 1)
}

// RenderSkillScaled changes only the presentation camera around the ground
// anchor. Authored transforms, keys, root motion and collision data stay intact.
func (l *SkillAnimations) RenderSkillScaled(name string, t float64, width, height int, gameplay bool, zoom float64) (*image.NRGBA, error) {
	return l.RenderSkillPlaced(name, t, width, height, gameplay, zoom, 0)
}

// RenderSkillPlaced adapts the display canvas only; no authored key is changed.
func (l *SkillAnimations) RenderSkillPlaced(name string, t float64, width, height int, gameplay bool, zoom, offsetY float64) (*image.NRGBA, error) {
	duration := l.durations[name]
	if duration <= 0 || width <= 0 || height <= 0 || width > 4096 || height > 4096 || math.IsNaN(zoom) || zoom < .5 || zoom > 2 || math.IsNaN(offsetY) || math.Abs(offsetY) > 80 {
		return nil, fmt.Errorf("invalid animation or dimensions: %s", name)
	}
	t = math.Max(0, math.Min(duration, t))
	out := image.NewRGBA(image.Rect(0, 0, width, height))
	world := l.transforms(name, t, gameplay)
	a := l.doc.Animations[name]
	canvas := Affine{A: float64(width) / CanvasWidth, D: -float64(height) / CanvasHeight, E: 240 * float64(width) / CanvasWidth, F: (GroundY + offsetY) * float64(height) / CanvasHeight}
	canvas.A *= zoom
	canvas.D *= zoom
	for _, s := range l.doc.Slots {
		if gameplay && (s.Bone == "fx" || s.Bone == "fan_fly" || s.Bone == "fan_trail" || s.Bone == "swoosh" || s.Bone == "boom" || strings.HasPrefix(s.Bone, "boom_") || strings.HasPrefix(s.Bone, "spark_") || strings.HasPrefix(s.Bone, "debris_") || strings.HasPrefix(s.Bone, "smoke_")) {
			continue
		}
		attachment := s.Attachment
		for _, key := range a.Slots[s.Name]["attachment"] {
			if key.Time > t {
				break
			}
			attachment = ""
			if key.Name != nil {
				attachment = *key.Name
			}
		}
		if attachment == "" {
			continue
		}
		region, ok := l.doc.Skins[0].Attachments[s.Name][attachment]
		if !ok || region.Type == "point" {
			continue
		}
		key := region.Path
		if key == "" {
			key = attachment
		}
		im := l.images[key]
		if im == nil {
			continue
		}
		m := rotation(region.Rotation * math.Pi / 180)
		sx, sy := number(region.ScaleX, 1), number(region.ScaleY, 1)
		m.A *= sx
		m.B *= sx
		m.C *= sy
		m.D *= sy
		m.E = region.X
		m.F = region.Y
		m = canvas.mul(world[l.index[s.Bone]]).mul(m).mul(Affine{A: region.Width / float64(im.Rect.Dx()), D: -region.Height / float64(im.Rect.Dy()), E: -region.Width / 2, F: region.Height / 2})
		opacity := 1.0
		colorKeys := a.Slots[s.Name]["rgba"]
		if colorKeys == nil {
			colorKeys = a.Slots[s.Name]["color"]
		}
		c := s.Color
		for _, k := range colorKeys {
			if k.Time > t {
				break
			}
			c = k.Color
		}
		if len(c) == 8 {
			var alpha uint8
			if _, err := fmt.Sscanf(c[6:], "%02x", &alpha); err == nil {
				opacity = float64(alpha) / 255
			}
		}
		if len(colorKeys) > 0 {
			opacity = sampleTrack(colorKeys, t, "alpha", opacity)
		}
		if ks := a.Slots[s.Name]["alpha"]; len(ks) > 0 {
			opacity = sampleTrack(ks, t, "", 1)
		}
		blitSkillRegion(out, l.premult[key], m, opacity)
	}
	return straightSkillFrame(out), nil
}

func blitRegion(out, src *image.NRGBA, m Affine, opacity float64) {
	det := m.A*m.D - m.B*m.C
	if math.Abs(det) < 1e-10 || opacity <= 0 {
		return
	}
	inv := Affine{A: m.D / det, B: -m.B / det, C: -m.C / det, D: m.A / det}
	inv.E = -inv.A*m.E - inv.C*m.F
	inv.F = -inv.B*m.E - inv.D*m.F
	x0, y0, x1, y1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range []Vec{{0, 0}, {float64(src.Rect.Dx()), 0}, {0, float64(src.Rect.Dy())}, {float64(src.Rect.Dx()), float64(src.Rect.Dy())}} {
		q := m.apply(p)
		x0 = math.Min(x0, q.X)
		y0 = math.Min(y0, q.Y)
		x1 = math.Max(x1, q.X)
		y1 = math.Max(y1, q.Y)
	}
	for y := max(0, int(math.Floor(y0))); y < min(out.Rect.Dy(), int(math.Ceil(y1))); y++ {
		for x := max(0, int(math.Floor(x0))); x < min(out.Rect.Dx(), int(math.Ceil(x1))); x++ {
			p := inv.apply(Vec{float64(x) + .5, float64(y) + .5})
			sx, sy := int(math.Floor(p.X)), int(math.Floor(p.Y))
			if sx < 0 || sy < 0 || sx >= src.Rect.Dx() || sy >= src.Rect.Dy() {
				continue
			}
			// Bilinear sample in premultiplied space to prevent dark atlas fringes.
			fx, fy := p.X-.5, p.Y-.5
			ix, iy := int(math.Floor(fx)), int(math.Floor(fy))
			tx, ty := fx-float64(ix), fy-float64(iy)
			var r, g, b, a float64
			for dy := 0; dy < 2; dy++ {
				for dx := 0; dx < 2; dx++ {
					xx, yy := ix+dx, iy+dy
					if xx < 0 || yy < 0 || xx >= src.Rect.Dx() || yy >= src.Rect.Dy() {
						continue
					}
					c := src.NRGBAAt(xx, yy)
					w := (1 - tx) * (1 - ty)
					if dx == 1 {
						w = tx * (1 - ty)
					}
					if dy == 1 {
						w = (1 - tx) * ty
						if dx == 1 {
							w = tx * ty
						}
					}
					aa := float64(c.A) / 255 * w * opacity
					a += aa
					r += float64(c.R) * aa
					g += float64(c.G) * aa
					b += float64(c.B) * aa
				}
			}
			if a <= 0 {
				continue
			}
			old := out.NRGBAAt(x, y)
			oa := float64(old.A) / 255
			na := a + oa*(1-a)
			out.SetNRGBA(x, y, color.NRGBA{uint8(math.Min(255, (r+float64(old.R)*oa*(1-a))/na)), uint8(math.Min(255, (g+float64(old.G)*oa*(1-a))/na)), uint8(math.Min(255, (b+float64(old.B)*oa*(1-a))/na)), uint8(math.Min(255, na*255))})
		}
	}
}

// SkillPlayback is a once-only event cursor; Stop removes every pending event.
type SkillPlayback struct {
	Name    string
	Time    float64
	Playing bool
	library *SkillAnimations
}

func (l *SkillAnimations) PlaySkillAnimation(name string) (*SkillPlayback, error) {
	if l.GetAnimationDuration(name) <= 0 {
		return nil, fmt.Errorf("unknown skill %s", name)
	}
	return &SkillPlayback{Name: name, Playing: true, library: l}, nil
}
func (p *SkillPlayback) StopSkillAnimation() { p.Playing = false }
func (p *SkillPlayback) GetAnimationProgress() float64 {
	return math.Min(1, p.Time/p.library.GetAnimationDuration(p.Name))
}
func (p *SkillPlayback) Advance(dt float64) []AnimationEvent {
	if !p.Playing || dt <= 0 {
		return nil
	}
	before := p.Time
	p.Time = math.Min(p.Time+dt, p.library.GetAnimationDuration(p.Name))
	var events []AnimationEvent
	for _, e := range p.library.GetAnimationEvents(p.Name) {
		if (e.Time > before || e.Time == 0 && before == 0) && e.Time <= p.Time {
			events = append(events, e)
		}
	}
	if p.GetAnimationProgress() >= 1 {
		p.Playing = false
	}
	return events
}
