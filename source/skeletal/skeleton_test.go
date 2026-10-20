package skeletal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"codexpet/reconstruction/emotion"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func loadRig(t *testing.T) (*Skeleton, *Atlas) {
	t.Helper()
	root := repoRoot(t)
	s, err := Load(filepath.Join(root, "skeletal", "naiwa.json"))
	if err != nil {
		t.Fatal(err)
	}
	a, err := LoadAtlas(filepath.Join(root, "skeletal", "naiwa.atlas"))
	if err != nil {
		t.Fatal(err)
	}
	return s, a
}

type golden struct {
	Source     string `json:"source"`
	Animations map[string][]struct {
		T     float64              `json:"t"`
		Bones map[string][]float64 `json:"bones"`
		Slots [][]any              `json:"slots"`
	} `json:"animations"`
}

// The poses must agree with the official spine-core 4.2 runtime, which wrote
// skeletal/validation/spine-core-poses.json. spine-core keeps keys in float32,
// hence the tolerances.
func TestPosesMatchSpineCore(t *testing.T) {
	s, _ := loadRig(t)
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "skeletal", "validation", "spine-core-poses.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g golden
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	if len(g.Animations) < 4 {
		t.Fatalf("golden has %d animations", len(g.Animations))
	}
	for name, frames := range g.Animations {
		anim := name
		if name == "setup" {
			anim = ""
		}
		for _, f := range frames {
			p, err := s.Sample(anim, f.T+FrameEpsilon)
			if err != nil {
				t.Fatal(err)
			}
			for bone, want := range f.Bones {
				got, ok := p.Bone(bone)
				if !ok {
					t.Fatalf("%s t=%v: no bone %s", name, f.T, bone)
				}
				gv := []float64{got.A, got.B, got.C, got.D, got.X, got.Y}
				for i := range want {
					tol := 2e-3
					if i >= 4 {
						tol = 0.02
					}
					if math.Abs(gv[i]-want[i]) > tol {
						t.Fatalf("%s t=%v bone %s[%d] = %.4f, spine-core %.4f", name, f.T, bone, i, gv[i], want[i])
					}
				}
			}
			if len(p.Slots) != len(f.Slots) {
				t.Fatalf("%s t=%v: %d visible slots, spine-core %d", name, f.T, len(p.Slots), len(f.Slots))
			}
			for i, ws := range f.Slots {
				gs := p.Slots[i]
				slot, att := ws[0].(string), ws[1].(string)
				nums := make([]float64, 0, len(ws)-2)
				for _, v := range ws[2:] {
					nums = append(nums, v.(float64))
				}
				if gs.Slot != slot {
					t.Fatalf("%s t=%v draw order[%d] = %s, spine-core %s", name, f.T, i, gs.Slot, slot)
				}
				if gs.Point {
					if gs.Attachment != att || len(nums) != 2 || math.Abs(gs.PX-nums[0]) > 0.02 || math.Abs(gs.PY-nums[1]) > 0.02 {
						t.Fatalf("%s t=%v point %s/%s = (%.3f,%.3f), spine-core %v", name, f.T, slot, gs.Attachment, gs.PX, gs.PY, ws)
					}
					continue
				}
				if gs.Region != att || len(nums) != 12 {
					t.Fatalf("%s t=%v slot %s region %s, spine-core %v", name, f.T, slot, gs.Region, ws[1])
				}
				for c := 0; c < 4; c++ {
					if math.Abs(gs.Color[c]-nums[c]) > 2e-3 {
						t.Fatalf("%s t=%v slot %s color %v, spine-core %v", name, f.T, slot, gs.Color, nums[:4])
					}
				}
				for v := 0; v < 8; v++ {
					if math.Abs(gs.Verts[v]-nums[4+v]) > 0.05 {
						t.Fatalf("%s t=%v slot %s vert %d = %.3f, spine-core %.3f", name, f.T, slot, v, gs.Verts[v], nums[4+v])
					}
				}
			}
		}
	}
}

func TestSkillsReferenceTheRig(t *testing.T) {
	s, _ := loadRig(t)
	skills, err := emotion.Skills()
	if err != nil {
		t.Fatal(err)
	}
	for _, sk := range skills {
		if sk.Skeletal == nil {
			t.Fatalf("%s has no skeletal entry", sk.ID)
		}
		if sk.Skeletal.Skeleton != "skeletal/naiwa.json" || sk.Skeletal.Atlas != "skeletal/naiwa.atlas" {
			t.Fatalf("%s skeletal paths %+v", sk.ID, sk.Skeletal)
		}
		frames := s.FrameCount(sk.Skeletal.Animation)
		if sk.Skeletal.LoopAnimation != "" {
			// The intro's last key closes the burn cycle and is not a clip frame.
			frames--
		}
		if frames != sk.FrameCount {
			t.Fatalf("%s: animation %s has %d frames at %d fps, clip has %d", sk.ID, sk.Skeletal.Animation, frames, s.FPS, sk.FrameCount)
		}
		if s.FPS != sk.FPS {
			t.Fatalf("%s fps %d, rig %d", sk.ID, sk.FPS, s.FPS)
		}
		if sk.Skeletal.LoopAnimation != "" {
			if got := s.FrameCount(sk.Skeletal.LoopAnimation); got != s.FPS+1 {
				t.Fatalf("%s loop animation %s has %d frames, want a one-second cycle", sk.ID, sk.Skeletal.LoopAnimation, got)
			}
		}
	}
	for _, id := range []string{"skill_fan_throw", "skill_roll"} {
		if len(s.Animations[id].Events) == 0 {
			t.Fatalf("%s has no events", id)
		}
	}
}

func TestSetupPoseRendersTheReference(t *testing.T) {
	s, a := loadRig(t)
	got, err := s.Render(s.Setup(), a)
	if err != nil {
		t.Fatal(err)
	}
	ref := readPNG(t, filepath.Join(repoRoot(t), "source", "emotion", "neutral-reference.png"))
	if got.Bounds() != ref.Bounds() {
		t.Fatalf("bounds %v, reference %v", got.Bounds(), ref.Bounds())
	}
	for i := 0; i < len(ref.Pix); i += 4 {
		r, g := ref.Pix[i:i+4], got.Pix[i:i+4]
		if r[3] == 0 && g[3] == 0 {
			continue
		}
		if !bytes.Equal(r, g) {
			x, y := (i/4)%ref.Bounds().Dx(), (i/4)/ref.Bounds().Dx()
			t.Fatalf("setup pixel %d,%d = %v, reference %v", x, y, g, r)
		}
	}
}

// Rendering any frame in Go must land on the shipped clip frame up to
// resampling noise; the clips were rendered from the same rig.
func TestRenderedFramesMatchClips(t *testing.T) {
	s, a := loadRig(t)
	root := repoRoot(t)
	skills, err := emotion.Skills()
	if err != nil {
		t.Fatal(err)
	}
	for _, sk := range skills {
		for k := 1; k < sk.FrameCount-1; k++ {
			p, err := s.SampleFrame(sk.Skeletal.Animation, k)
			if err != nil {
				t.Fatal(err)
			}
			got, err := s.Render(p, a)
			if err != nil {
				t.Fatal(err)
			}
			want := readPNG(t, filepath.Join(root, filepath.FromSlash(sk.Clip), framePNG(k)))
			var sum, n float64
			big := 0
			for i := 0; i < len(want.Pix); i += 4 {
				gw, ww := onWhite(got.Pix[i:i+4]), onWhite(want.Pix[i:i+4])
				for c := 0; c < 3; c++ {
					d := math.Abs(gw[c] - ww[c])
					sum += d
					n++
					if d > 24 {
						big++
					}
				}
			}
			if mean := sum / n; mean > 0.25 || big > 400 {
				t.Fatalf("%s frame %d: mean diff %.3f, %d channels off by more than 24", sk.ID, k, mean, big)
			}
		}
	}
}

func TestMouthPointTracksTheManifest(t *testing.T) {
	s, _ := loadRig(t)
	p := s.Setup()
	x, y, ok := p.Point("mouth")
	if !ok {
		t.Fatal("setup pose has no mouth point")
	}
	if math.Abs(x-153.5) > 0.01 || math.Abs(y-138) > 0.01 {
		t.Fatalf("setup mouth at %.2f,%.2f, want 153.5,138", x, y)
	}
}

func framePNG(k int) string {
	return fmt.Sprintf("%04d.png", k)
}

func onWhite(p []uint8) [3]float64 {
	a := float64(p[3]) / 255
	return [3]float64{float64(p[0])*a + 255*(1-a), float64(p[1])*a + 255*(1-a), float64(p[2])*a + 255*(1-a)}
}

func readPNG(t *testing.T, path string) *image.NRGBA {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	im, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if n, ok := im.(*image.NRGBA); ok {
		return n
	}
	out := image.NewNRGBA(im.Bounds())
	draw.Draw(out, out.Bounds(), im, im.Bounds().Min, draw.Src)
	return out
}
