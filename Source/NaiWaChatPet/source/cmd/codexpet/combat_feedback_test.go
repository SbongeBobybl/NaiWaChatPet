package main

import (
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"codexpet/reconstruction/play"
	"codexpet/reconstruction/rig"
)

func feedbackImage(pix []byte, w, h int) *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(pix); i += 4 {
		im.Pix[i], im.Pix[i+1], im.Pix[i+2], im.Pix[i+3] = pix[i+2], pix[i+1], pix[i], pix[i+3]
	}
	return im
}
func TestCombatPremultipliedVFXAndHUD(t *testing.T) {
	for _, kind := range []string{"explosion", "power", "dodge", "rollhit"} {
		for _, progress := range []float64{0, .06, .2, .5, .85, 1} {
			pix := combatEffectSprite(kind, 128, progress, 1)
			for n := 0; n < len(pix); n += 4 {
				if pix[n] > pix[n+3] || pix[n+1] > pix[n+3] || pix[n+2] > pix[n+3] {
					t.Fatalf("invalid transparent pixel %s", kind)
				}
			}
		}
	}
	c := play.NewCombat(false)
	s := c.Snapshot()
	s.Fighters[0].MP = 7
	s.Fighters[1].BurnLeft = 5
	pix, h := combatHUDSprite(960, s, play.DefaultGameSettings().Keys, 0, "ROOM ABC123 READY")
	if len(pix) != 960*h*4 {
		t.Fatal("HUD dimensions")
	}
	for _, kind := range []string{"hit", "roll", "power", "dodge", "explosion"} {
		b := combatWAV(kind, .65)
		if len(b) < 44 || string(b[:4]) != "RIFF" {
			t.Fatal("invalid audio")
		}
	}
}
func TestCombatVisualArtifacts(t *testing.T) {
	out := os.Getenv("NAIWA_COMBAT_ARTIFACTS")
	if out == "" {
		t.Skip("artifact export not requested")
	}
	if err := os.MkdirAll(out, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(name string, im image.Image) {
		f, e := os.Create(filepath.Join(out, name))
		if e != nil {
			t.Fatal(e)
		}
		defer f.Close()
		if e = png.Encode(f, im); e != nil {
			t.Fatal(e)
		}
	}
	lib, e := rig.DefaultSkillAnimations()
	if e != nil {
		t.Fatal(e)
	}
	sheet := image.NewRGBA(image.Rect(0, 0, 1008, 784))
	draw.Draw(sheet, sheet.Rect, image.NewUniform(image.Black), image.Point{}, draw.Src)
	for row, name := range []string{"skill_fan_throw", "skill_roll", "skill_power_up"} {
		for col, at := range []float64{0, .166667, .25, .5, .833333, 1} {
			im, e := lib.RenderSkill(name, at, 168, 224, true)
			if e != nil {
				t.Fatal(e)
			}
			draw.Draw(sheet, image.Rect(col*168, row*224, (col+1)*168, (row+1)*224), im, image.Point{}, draw.Over)
		}
	}
	write("authored-skills-contact-sheet.png", sheet)
	vfx := image.NewRGBA(image.Rect(0, 0, 960, 192))
	draw.Draw(vfx, vfx.Rect, image.NewUniform(image.Black), image.Point{}, draw.Src)
	for i, at := range []float64{.02, .12, .3, .55, .8} {
		draw.Draw(vfx, image.Rect(i*192, 0, (i+1)*192, 192), feedbackImage(combatEffectSprite("explosion", 192, at, 1), 192, 192), image.Point{}, draw.Over)
	}
	write("explosion-five-phases.png", vfx)
	c := play.NewCombat(false)
	s := c.Snapshot()
	s.Fighters[0].MP = 63.5
	s.Fighters[1].MP = 27
	s.Fighters[1].BurnLeft = 5
	s.Fighters[0].BuffStacks = 2
	pix, h := combatHUDSprite(960, s, play.DefaultGameSettings().Keys, 0, "ROOM A1B2C3 FIGHTING")
	write("combat-hud.png", feedbackImage(pix, 960, h))
	for _, kind := range []string{"hit", "roll", "power", "dodge", "explosion"} {
		if e := os.WriteFile(filepath.Join(out, kind+".wav"), combatWAV(kind, .65), 0600); e != nil {
			t.Fatal(e)
		}
	}
}
func BenchmarkCombatExplosion(b *testing.B) {
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		combatEffectSprite("explosion", 180, float64(n%60)/60, 1)
	}
}
