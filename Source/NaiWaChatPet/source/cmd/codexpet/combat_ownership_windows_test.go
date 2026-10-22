//go:build windows

package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codexpet/reconstruction/animation"
	"codexpet/reconstruction/companion"
	"codexpet/reconstruction/emotion"
	"codexpet/reconstruction/play"
	"codexpet/reconstruction/rig"
)

func TestCombatAmbientOwnershipAndSilentAnnouncements(t *testing.T) {
	assets, err := loadAssets(bundledAssets, "runtime.json")
	if err != nil {
		t.Fatal(err)
	}
	assets.Engine.Fire(0, animation.DoubleClick)
	core := play.NewCombat(false)
	u := newCombatPresentation(core)
	defer u.cancel()
	c := emotion.New(func() time.Duration { return 0 })
	a := &desktopHost{assets: assets, emotions: c, epoch: time.Now(), play: &playState{fight: &fightState{upgraded: u}}}
	a.chat = &nativeChat{replyVisible: true, local: &companion.ReplyView{Text: "old bubble"}}
	a.suspendAmbientForCombat(0)
	assets.Engine.Advance(time.Minute)
	if assets.Engine.Snapshot().State != animation.ToLaugh {
		t.Fatal("ambient action not preserved")
	}
	if assets.Engine.IdleSnapshot().State != animation.Idle {
		t.Fatal("combat inherited ambient animation")
	}
	if accepted, _ := assets.Engine.Fire(time.Minute, animation.Contact); accepted {
		t.Fatal("ambient contact interrupted combat")
	}
	c.Apply("late-company", emotion.Analysis{Emotion: "happy", Intensity: 1, Confidence: 1, DurationMS: 15000}, "local-companionship")
	a.syncEmotions(time.Minute)
	if !a.emotionView.Blocked || a.emotionView.Clip != "" || a.emote("happy", time.Second) {
		t.Fatal("company owns combat pose")
	}
	a.say("round starts")
	if a.chat.replyVisible || a.chat.local.Text != "old bubble" {
		t.Fatal("game opened an announcement bubble")
	}
	a.play.fight = nil
	a.resumeAmbientAfterCombat(time.Minute)
	assets.Engine.Advance(time.Minute + 100*time.Millisecond)
	if assets.Engine.Snapshot().Frame == 0 {
		t.Fatal("ambient action failed to resume")
	}
}

func TestRollDisplayScale(t *testing.T) {
	lib, err := rig.DefaultSkillAnimations()
	if err != nil {
		t.Fatal(err)
	}
	before, err := lib.RenderSkill("skill_roll", .5, 210, 280, true)
	if err != nil {
		t.Fatal(err)
	}
	after, err := lib.RenderSkillScaled("skill_roll", .5, 210, 280, true, combatSkillZoom(play.RollSkill, 30))
	if err != nil {
		t.Fatal(err)
	}
	area := func(im *image.NRGBA) int {
		n := 0
		for i := 3; i < len(im.Pix); i += 4 {
			if im.Pix[i] > 64 {
				n++
			}
		}
		return n
	}
	if area(after) < area(before)*14/10 {
		t.Fatal("roll did not visibly enlarge")
	}
	if combatSkillZoom(play.RollSkill, 0) != 1 || combatSkillZoom(play.RollSkill, 60) != 1 {
		t.Fatal("roll entry/exit scale changed")
	}
	if out := os.Getenv("NAIWA_COMBAT_ARTIFACTS"); out != "" {
		os.MkdirAll(out, 0700)
		for name, im := range map[string]*image.NRGBA{"roll-before.png": before, "roll-after.png": after} {
			f, e := os.Create(filepath.Join(out, name))
			if e != nil {
				t.Fatal(e)
			}
			e = png.Encode(f, im)
			f.Close()
			if e != nil {
				t.Fatal(e)
			}
		}
	}
}
