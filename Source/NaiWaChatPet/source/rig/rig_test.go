package rig

import (
	"image"
	"image/color"
	"math"
	"testing"
)

// figureFrame paints an upright blob with a soft edge and a colour gradient,
// enough to expose a seam, a shifted sample or a doubled edge.
func figureFrame() *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, CanvasWidth, CanvasHeight))
	for y := 0; y < CanvasHeight; y++ {
		for x := 0; x < CanvasWidth; x++ {
			dx, dy := (float64(x)-235)/135, (float64(y)-290)/255
			edge := clamp01((1 - math.Sqrt(dx*dx+dy*dy)) * 40)
			if edge == 0 {
				continue
			}
			im.SetNRGBA(x, y, color.NRGBA{uint8(120 + x/4), uint8(60 + y/4), uint8((x + y) % 200), uint8(math.Round(255 * edge))})
		}
	}
	return im
}

func rigs(t *testing.T) map[string]*Puppet {
	t.Helper()
	out := map[string]*Puppet{}
	for name, build := range map[string]func() (*Puppet, error){"stand": Stand, "stance": Stance, "figure": Figure} {
		p, err := build()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out[name] = p
	}
	return out
}

func TestRestPoseReproducesTheFrame(t *testing.T) {
	frame := figureFrame()
	tex := NewTexture(frame)
	for name, p := range rigs(t) {
		got, err := p.Render(tex, p.Rest())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for i := 0; i < len(frame.Pix); i += 4 {
			for k := 0; k < 4; k++ {
				g, w := int(got.Pix[i+k]), int(frame.Pix[i+k])
				if k < 3 {
					// Compare colour as displayed, weighted by its alpha.
					g, w = g*int(got.Pix[i+3])/255, w*int(frame.Pix[i+3])/255
				}
				if g-w > 2 || w-g > 2 {
					t.Fatalf("%s: pixel (%d,%d) channel %d = %d, want %d", name, i/4%CanvasWidth, i/4/CanvasWidth, k, g, w)
				}
			}
		}
	}
}

func TestRenderKeepsTransparentPixelsBlack(t *testing.T) {
	tex := NewTexture(figureFrame())
	p := rigs(t)["stand"]
	got, err := p.Render(tex, StandPose(Gait{Stride: 2, Phase: .3, Squash: -.05}))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(got.Pix); i += 4 {
		if got.Pix[i+3] == 0 && (got.Pix[i] != 0 || got.Pix[i+1] != 0 || got.Pix[i+2] != 0) {
			t.Fatalf("pixel %d has colour under zero alpha", i/4)
		}
	}
}

func TestRenderRejectsAMismatchedTexture(t *testing.T) {
	small := NewTexture(image.NewNRGBA(image.Rect(0, 0, 64, 64)))
	p := rigs(t)["figure"]
	if _, err := p.Render(small, p.Rest()); err == nil {
		t.Fatal("expected an error for a texture of the wrong size")
	}
	if _, err := p.Render(nil, p.Rest()); err == nil {
		t.Fatal("expected an error for a missing texture")
	}
}

func TestChildBonesFollowTheirParents(t *testing.T) {
	p := rigs(t)["stand"]
	toe := Vec{258, 528}
	pose := p.Rest()
	if got := p.Point(pose, standNearFoot, toe); got != toe {
		t.Fatalf("rest pose moved the toe to %v", got)
	}
	// Turning the thigh a quarter turn forward carries the whole leg with it.
	pose[standNearThigh].Rot = math.Pi / 2
	got := p.Point(pose, standNearFoot, toe)
	hip := Vec{290, 395}
	want := Vec{hip.X - (toe.Y - hip.Y), hip.Y + (toe.X - hip.X)}
	if math.Hypot(got.X-want.X, got.Y-want.Y) > 1e-9 {
		t.Fatalf("toe at %v, want %v", got, want)
	}
}

func TestWalkSwingsTheLegsAgainstEachOther(t *testing.T) {
	p := rigs(t)["stand"]
	near, far := Vec{305, 492}, Vec{216, 478}
	a := StandPose(Gait{Stride: 1, Phase: .25})
	b := StandPose(Gait{Stride: 1, Phase: .75})
	nearSwing := p.Point(a, standNearShin, near).X - p.Point(b, standNearShin, near).X
	farSwing := p.Point(a, standFarShin, far).X - p.Point(b, standFarShin, far).X
	if nearSwing > -20 || farSwing < 20 {
		t.Fatalf("ankles travel %.1f (near) and %.1f (far); want opposite strides", nearSwing, farSwing)
	}
	if still := StandPose(Gait{Phase: .25}); p.Point(still, standNearShin, near) != near {
		t.Fatal("a standing gait moved the legs")
	}
}

func TestStrikeDrawsBackThenDrivesPastRest(t *testing.T) {
	p := rigs(t)["stance"]
	rest := Palm(p, p.Rest()).X
	back := Palm(p, StrikePose(.28, 1)).X
	deep := Palm(p, StrikePose(.28, 1.9)).X
	end := Palm(p, StrikePose(1, 1)).X
	if back < rest+35 || deep < back+10 {
		t.Fatalf("wind-up palm at %.1f (heavy %.1f), rest at %.1f", back, deep, rest)
	}
	if math.Abs(end-rest) > .5 {
		t.Fatalf("strike ends with the palm at %.1f, rest at %.1f", end, rest)
	}
	// The open hand is 23 wide of the palm's centre and must stay on the canvas.
	for _, power := range []float64{1, 1.9} {
		for progress := 0.0; progress <= 1; progress += .02 {
			if x := Palm(p, StrikePose(progress, power)).X; x < 26 {
				t.Fatalf("power %.1f at %.2f puts the palm at %.1f, clipping the hand", power, progress, x)
			}
		}
		if hit := Palm(p, StrikePose(StrikeImpact, power)).X; hit > rest-12 {
			t.Fatalf("power %.1f lands the palm at %.1f, rest at %.1f", power, hit, rest)
		}
	}
}

func TestGesturesStartNearRestAndStayFinite(t *testing.T) {
	for _, g := range Gestures {
		for _, s := range []float64{0, .4, 1.3, 9} {
			for _, j := range GesturePose(g, s) {
				for _, v := range []float64{j.Rot, j.Stretch, j.Bulge, j.TX, j.TY} {
					if math.IsNaN(v) || math.Abs(v) > 16 {
						t.Fatalf("%s at %.1fs: joint value %v", g, s, v)
					}
				}
			}
		}
		for _, j := range GesturePose(g, 0) {
			if math.Abs(j.Rot) > .07 || math.Abs(j.Stretch) > .03 || math.Abs(j.TX) > 1 || math.Abs(j.TY) > 1 {
				t.Fatalf("%s snaps away from rest on its first frame: %+v", g, j)
			}
		}
	}
}

func TestSkinKeepsTheStrongestFourInfluences(t *testing.T) {
	v, err := skin(Vec{}, []float64{.1, .4, 0, .2, .05, .25}, 6)
	if err != nil {
		t.Fatal(err)
	}
	var total float32
	seen := map[uint8]bool{}
	for k := range v.w {
		total += v.w[k]
		seen[v.bone[k]] = true
	}
	if math.Abs(float64(total)-1) > 1e-6 || seen[4] || seen[2] {
		t.Fatalf("weights %v on bones %v", v.w, v.bone)
	}
	if _, err := skin(Vec{}, []float64{0, 0}, 2); err == nil {
		t.Fatal("expected an error for a vertex with no influence")
	}
	if _, err := skin(Vec{}, []float64{1}, 2); err == nil {
		t.Fatal("expected an error for a short weight list")
	}
}

func TestNewPuppetRejectsBadSkeletons(t *testing.T) {
	weights := func(Vec) []float64 { return []float64{1, 0} }
	spec := []LayerSpec{{"all", nil, weights}}
	if _, err := NewPuppet(100, 100, []Bone{{"a", 1, Vec{}, Vec{}}, {"b", -1, Vec{}, Vec{}}}, spec); err == nil {
		t.Fatal("expected an error for a child listed before its parent")
	}
	if _, err := NewPuppet(100, 100, []Bone{{"a", -1, Vec{}, Vec{}}, {"b", 0, Vec{}, Vec{}}}, nil); err == nil {
		t.Fatal("expected an error for a rig without layers")
	}
}

func BenchmarkStandRender(b *testing.B) {
	p, err := Stand()
	if err != nil {
		b.Fatal(err)
	}
	tex := NewTexture(figureFrame())
	pose := StandPose(Gait{Stride: 2, Phase: .3})
	for b.Loop() {
		if _, err := p.Render(tex, pose); err != nil {
			b.Fatal(err)
		}
	}
}
