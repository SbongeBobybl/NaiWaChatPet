package play

import (
	"math"
	"testing"
)

func TestBattleTuningAndTimeoutResults(t *testing.T) {
	c := NewCombat(false)
	for _, f := range c.Fighters {
		closeValue(t, f.HP, 80)
		closeValue(t, f.MaxHP, 80)
	}
	closeValue(t, c.Arena.Clock, 120)
	for i, want := range []float64{48, 64, 80, 112} {
		closeValue(t, RivalFor(i+1).HP, want)
	}
	c.Arena.Phase = Fighting
	ticks(c, 3600, [2]Input{})
	if !c.Arena.Live() {
		t.Fatal("round still ended at the old 60 second limit")
	}
	closeValue(t, c.Arena.Clock, 60)
	for _, winner := range []int{-1, 0, 1} {
		c = NewCombat(false)
		c.Arena.Phase = Fighting
		c.Arena.Clock = CombatDT
		if winner >= 0 {
			c.Fighters[1-winner].HP = 40
		}
		c.syncHealth()
		c.Step([2]Input{})
		if c.Winner != winner {
			t.Fatalf("timeout winner %d want %d", c.Winner, winner)
		}
		ticks(c, 160, [2]Input{})
		if c.Arena.Phase != Over || c.Winner != winner {
			t.Fatal("final result changed or failed to finish")
		}
	}
}
func TestCombatMovementResponseAndPrediction(t *testing.T) {
	c := NewCombat(false)
	c.Arena.Phase = Fighting
	for _, in := range []Input{{Right: true}, {Left: true}, {Right: true, Down: true, Run: true}, {}, {Up: true}} {
		before := c.Fighters[0]
		x, y := PredictCombatPosition(before, in, CombatDT)
		c.Step([2]Input{in, {}})
		closeValue(t, c.Fighters[0].X, x)
		closeValue(t, c.Fighters[0].Y, y)
	}
	placeCombat(c, 0, ArenaWidth-126, ArenaDepth-1)
	before := c.Fighters[0]
	px, py := PredictCombatPosition(before, Input{Right: true, Down: true}, CombatDT)
	c.Step([2]Input{{Right: true, Down: true}, {}})
	closeValue(t, c.Fighters[0].X, px)
	closeValue(t, c.Fighters[0].Y, py)
	placeCombat(c, 0, 600, 150)
	for i := 0; i < 30; i++ {
		c.Step([2]Input{{Right: true}, {}})
	}
	x := c.Fighters[0].X
	c.Step([2]Input{{Left: true}, {}})
	if c.Fighters[0].X >= x {
		t.Fatal("reverse input still drifts in old direction")
	}
	for i := 0; i < 10; i++ {
		c.Step([2]Input{})
	}
	if math.Hypot(c.Fighters[0].VX, c.Fighters[0].VY) > 3 {
		t.Fatal("release did not settle within 167ms")
	}
}
func TestCombatRecoveryAndLateInput(t *testing.T) {
	for _, id := range []SkillID{FanSkill, RollSkill, PowerSkill} {
		c := NewCombat(false)
		c.Arena.Phase = Fighting
		if !c.startSkill(0, id, Input{}) {
			t.Fatal("skill start")
		}
		d := c.Definition(id)
		unlock := max(d.Recovery, d.Effect+1, d.ActiveEnd+1)
		ticks(c, unlock-3, [2]Input{})
		c.Step([2]Input{{AttackPressed: true}, {}})
		ticks(c, 5, [2]Input{})
		if c.Fighters[0].Action.Skill != NoSkill || !c.Fighters[0].controller.Busy() {
			t.Fatal("late normal attack lost at skill recovery", id)
		}
		n := 0
		for _, e := range c.recent {
			if e.Kind == "skill_end" {
				n++
			}
		}
		if n != 1 {
			t.Fatal("duplicate/missing end event", n)
		}
	}
	c := NewCombat(false)
	c.Arena.Phase = Fighting
	c.Fighters[0].StunUntil = 8
	ticks(c, 3, [2]Input{})
	c.Step([2]Input{{DodgePressed: true}, {}})
	if c.Fighters[0].Action.Skill != NoSkill {
		t.Fatal("dodge bypassed stun")
	}
	ticks(c, 4, [2]Input{})
	if c.Fighters[0].Action.Skill != DodgeSkill {
		t.Fatal("last-moment dodge was discarded")
	}
}

func TestRetunedHitsKeepOriginalDamage(t *testing.T) {
	for _, tc := range []struct {
		id        SkillID
		pct, want float64
		burn      bool
	}{{NoSkill, .01, 1, false}, {FanSkill, .05, 5, false}, {RollSkill, .07, 7, false}, {NoSkill, .002, .2, true}} {
		c := NewCombat(false)
		c.Arena.Phase = Fighting
		if !c.damage(0, 1, c.Fighters[1].MaxHP*tc.pct, tc.id, false, tc.burn) {
			t.Fatal("hit rejected")
		}
		closeValue(t, c.Fighters[1].HP, 80-tc.want)
	}
	r := DefaultCombatRules()
	r.DamageScale = 0
	c, err := NewConfiguredCombat(false, &r)
	if err != nil || c.Rules.DamageScale != 1.25 {
		t.Fatal("older rules lost retuned damage scale", err)
	}
}

func TestHitRecoveryKeepsQueuedJump(t *testing.T) {
	c := NewCombat(false)
	c.Arena.Phase = Fighting
	c.damage(1, 0, 1, NoSkill, false, false)
	ticks(c, 8, [2]Input{})
	c.Step([2]Input{{JumpPressed: true}, {}})
	ticks(c, 3, [2]Input{})
	if c.Fighters[0].controller.crouch <= 0 {
		t.Fatal("jump buffered at end of actual hitstun was discarded")
	}
}
