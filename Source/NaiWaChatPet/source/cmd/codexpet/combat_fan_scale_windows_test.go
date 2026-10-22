//go:build windows

package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"codexpet/reconstruction/animation"
	"codexpet/reconstruction/play"
	"codexpet/reconstruction/rig"
)

func visibleSkillBounds(im *image.NRGBA) image.Rectangle {
	r := image.Rectangle{Min: image.Pt(im.Rect.Dx(), im.Rect.Dy())}
	for y := 0; y < im.Rect.Dy(); y++ {
		for x := 0; x < im.Rect.Dx(); x++ {
			if im.NRGBAAt(x, y).A >= 96 {
				r.Min.X, r.Min.Y = min(r.Min.X, x), min(r.Min.Y, y)
				r.Max.X, r.Max.Y = max(r.Max.X, x+1), max(r.Max.Y, y+1)
			}
		}
	}
	return r
}

func TestFanPresentationReference(t *testing.T) {
	assets, err := loadAssets(bundledAssets, "runtime.json")
	if err != nil {
		t.Fatal(err)
	}
	base, err := assets.Bodies.BodyCopy(string(animation.Idle), 0)
	if err != nil {
		t.Fatal(err)
	}
	stand, _ := rig.Stand()
	idle, err := stand.RenderSized(rig.NewTexture(base), rig.StandPose(rig.Gait{}), 210, 280)
	if err != nil {
		t.Fatal(err)
	}
	lib, err := rig.DefaultSkillAnimations()
	if err != nil {
		t.Fatal(err)
	}
	frames := map[string]*image.NRGBA{"idle-reference": idle}
	d := play.DefaultCombatRules().Skills[0]
	scale, offset := d.Presentation()
	for _, tick := range []int{0, 15, 32, 64} {
		im, err := lib.RenderSkill("skill_fan_throw", float64(tick)/60, 210, 280, true)
		if err != nil {
			t.Fatal(err)
		}
		name := map[int]string{0: "fan-start-before", 15: "fan-release-before", 32: "fan-middle-before", 64: "fan-end-before"}[tick]
		frames[name] = im
		t.Log(name, visibleSkillBounds(im), "idle", visibleSkillBounds(idle))
		fixed, err := lib.RenderSkillPlaced(d.Clip, float64(tick)/60, 210, 280, true, scale, offset)
		if err != nil {
			t.Fatal(err)
		}
		frames[name+"-fixed"] = fixed
		if visibleSkillBounds(fixed).Dy() < visibleSkillBounds(idle).Dy()*94/100 {
			t.Fatal("fan still visibly shrinks the fighter", tick)
		}
		if difference := visibleSkillBounds(fixed).Max.Y - visibleSkillBounds(idle).Max.Y; difference > 3 || difference < -3 {
			t.Fatal("fan feet leave the common ground line", tick, difference)
		}
	}
	if out := os.Getenv("NAIWA_COMBAT_ARTIFACTS"); out != "" {
		os.MkdirAll(out, 0700)
		for name, im := range frames {
			file, err := os.Create(filepath.Join(out, name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(file, im)
			file.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}
