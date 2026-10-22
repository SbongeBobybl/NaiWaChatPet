package rig

import (
	"math"
	"testing"
)

func TestSuppliedSkillSkeletonIntegration(t *testing.T) {
	l, err := DefaultSkillAnimations()
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"skill_fan_throw", "skill_roll", "skill_power_up"} {
		duration := l.GetAnimationDuration(n)
		if duration < 1 || duration > 1.1 {
			t.Fatalf("duration %s=%v", n, duration)
		}
		for _, at := range []float64{0, .25, .5, duration} {
			im, e := l.RenderSkill(n, at, 168, 224, true)
			if e != nil {
				t.Fatal(e)
			}
			opaque := 0
			for i := 3; i < len(im.Pix); i += 4 {
				if im.Pix[i] > 0 {
					opaque++
				}
			}
			if opaque < 500 {
				t.Fatalf("empty %s at %f", n, at)
			}
		}
	}
	p, ok := l.GetBoneAnchor("skill_fan_throw", "hand_near", .25, true)
	if !ok || math.IsNaN(p.X) || p.Y < 0 || p.Y > 560 {
		t.Fatalf("anchor %+v", p)
	}
	if math.Abs(l.RootTranslation("skill_roll", 1).X+110) > .001 {
		t.Fatal("root motion not preserved")
	}
	pb, _ := l.PlaySkillAnimation("skill_fan_throw")
	count := 0
	for i := 0; i < 120; i++ {
		for _, e := range pb.Advance(1.0 / 60) {
			if e.Name == "fan_release" {
				count++
			}
		}
	}
	if count != 1 || pb.Playing {
		t.Fatalf("event count %d", count)
	}
	pb, _ = l.PlaySkillAnimation("skill_fan_throw")
	pb.StopSkillAnimation()
	if len(pb.Advance(1)) != 0 {
		t.Fatal("interrupted events")
	}
}
func TestSpineAbsoluteBezierAndStepped(t *testing.T) {
	keys := []spineKey{{Time: 0, Value: 0, Curve: []byte(`[0.2,0,0.8,10]`)}, {Time: 1, Value: 10}}
	if math.Abs(sampleTrack(keys, .5, "", 0)-5) > .0001 {
		t.Fatal("bezier")
	}
	keys[0].Curve = []byte(`"stepped"`)
	if sampleTrack(keys, .9, "", 0) != 0 {
		t.Fatal("stepped")
	}
}
func BenchmarkAuthoredSkillRender(b *testing.B) {
	l, e := DefaultSkillAnimations()
	if e != nil {
		b.Fatal(e)
	}
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		_, e = l.RenderSkill("skill_power_up", float64(n%65)/60, 210, 280, true)
		if e != nil {
			b.Fatal(e)
		}
	}
}
