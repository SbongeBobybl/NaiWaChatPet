package play

import (
	"testing"
)

func TestActualNormalComboUsesOnePercentForAllThreeHits(t *testing.T) {
	c := liveCombat()
	placeCombat(c, 0, 600, 150)
	placeCombat(c, 1, 930, 150)
	count := 0
	for n := 0; n < 100; n++ {
		var in Input
		in.AttackPressed = n == 0 || n == 18 || n == 36
		c.Step([2]Input{in, {}})
		for _, e := range c.Events() {
			if e.Kind == "hit" {
				count++
				closeValue(t, e.Amount, 1)
			}
		}
	}
	if count != 3 {
		t.Fatalf("combo hit %d times", count)
	}
	closeValue(t, c.Fighters[1].HP, 97)
}

func TestSimultaneousNormalAttacksTrade(t *testing.T) {
	c := liveCombat()
	placeCombat(c, 0, 600, 150)
	placeCombat(c, 1, 930, 150)
	ticks(c, 30, [2]Input{{AttackPressed: true}, {AttackPressed: true}})
	closeValue(t, c.Fighters[0].HP, 99)
	closeValue(t, c.Fighters[1].HP, 99)
}
func TestAIObservesProjectileAndHonorsStun(t *testing.T) {
	c := liveCombat()
	ai := NewCombatAI(1, HardAI)
	c.Projectiles = []Projectile{{Owner: 0, X: 1100, Y: -100, Lane: 150, VX: 1150, Life: 120}}
	if in := ai.Think(c, 1); !in.DodgePressed {
		t.Fatal("hard AI did not react to an observable incoming projectile")
	}
	c.Fighters[1].StunUntil = c.Tick + 30
	if in := ai.Think(c, 1); in != (Input{}) {
		t.Fatal("stunned AI produced input")
	}
}
func TestViewportAndInputBindings(t *testing.T) {
	for _, v := range []CombatViewport{{-1920, 500, .5}, {100, 1200, 1.5}, {-2500, -200, 2.25}} {
		x, y := v.Point(800, 150)
		wx, wy := v.World(x, y)
		closeValue(t, wx, 800)
		closeValue(t, wy, 150)
	}
	settings := DefaultGameSettings()
	if settings.Bind("fan", 'Q') != nil || settings.Keys["fan"] != 'Q' {
		t.Fatal("rebind")
	}
	if settings.Bind("roll", 'Q') == nil || settings.Bind("dodge", 0x1b) == nil {
		t.Fatal("duplicate/reserved key accepted")
	}
	if settings.Bind("fan", 0x20) == nil || settings.Bind("power", 0x26) == nil {
		t.Fatal("compatibility input aliases lost")
	}
}
func TestConfiguredRulesKeepAuthoredEventsAndDifferentMaxHealing(t *testing.T) {
	r := DefaultCombatRules()
	r.Skills[0].Cost = 12
	r.Skills[0].Effect = 40
	r.Skills[0].ActiveEnd = 40
	c, e := NewConfiguredCombat(false, &r)
	if e != nil {
		t.Fatal(e)
	}
	if c.Definition(FanSkill).Cost != 12 || c.Definition(FanSkill).Effect != 15 {
		t.Fatal("authored release event lost")
	}
	c.Arena.Phase = Fighting
	c.Fighters[0].MaxHP = 250
	c.Fighters[0].HP = 100
	c.startSkill(0, PowerSkill, Input{})
	ticks(c, 20, [2]Input{})
	closeValue(t, c.Fighters[0].HP, 112.5)
}
