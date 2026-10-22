package play

import (
	"encoding/json"
	"math"
	"testing"
)

func liveCombat() *Combat {
	c := NewCombat(false)
	c.Rules.DamageScale = 1 // original percentage examples use an explicit unscaled 100 HP fixture
	for i := range c.Fighters {
		c.Fighters[i].HP, c.Fighters[i].MaxHP = 100, 100
	}
	c.Arena.Rival.HP = 100
	c.Arena.PetShown, c.Arena.FoeShown = 100, 100
	c.syncHealth()
	c.Arena.Phase = Fighting
	c.Arena.events = nil
	return c
}
func ticks(c *Combat, n int, in [2]Input) {
	for i := 0; i < n; i++ {
		c.Step(in)
		for j := range in {
			in[j].ClearEdges()
		}
	}
}
func closeValue(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-7 {
		t.Fatalf("got %.10f want %.10f", got, want)
	}
}
func placeCombat(c *Combat, i int, x, y float64) {
	c.Fighters[i].controller.Place(x, y, 1-2*i)
	c.syncPosition(i)
}
func TestCombatManaAndCosts(t *testing.T) {
	c := liveCombat()
	closeValue(t, c.Fighters[0].MP, 100)
	c.Fighters[0].MP = 0
	ticks(c, 60, [2]Input{})
	closeValue(t, c.Fighters[0].MP, 2.5)
	c.Fighters[0].MP = 99
	ticks(c, 60, [2]Input{})
	closeValue(t, c.Fighters[0].MP, 100)
	for _, id := range []SkillID{FanSkill, RollSkill, PowerSkill, DodgeSkill} {
		c = liveCombat()
		if !c.startSkill(0, id, Input{}) {
			t.Fatalf("start %v", id)
		}
		closeValue(t, c.Fighters[0].MP, 100-c.Definition(id).Cost)
		if c.startSkill(0, id, Input{}) {
			t.Fatal("duplicate start")
		}
		closeValue(t, c.Fighters[0].MP, 100-c.Definition(id).Cost)
	}
	c = liveCombat()
	c.Fighters[0].MP = 9
	if c.startSkill(0, FanSkill, Input{}) {
		t.Fatal("insufficient mana")
	}
	closeValue(t, c.Fighters[0].MP, 9)
}
func TestCombatPercentDamageBuffHealingAndBurn(t *testing.T) {
	for _, maxHP := range []float64{100, 250} {
		for _, tc := range []struct {
			id  SkillID
			pct float64
		}{{NoSkill, .01}, {FanSkill, .05}, {RollSkill, .07}} {
			c := liveCombat()
			c.Fighters[1].HP = maxHP
			c.Fighters[1].MaxHP = maxHP
			c.Fighters[0].MP = 50
			c.damage(0, 1, maxHP*tc.pct, tc.id, false, false)
			closeValue(t, c.Fighters[1].HP, maxHP*(1-tc.pct))
			closeValue(t, c.Fighters[0].MP, 51)
		}
	}
	c := liveCombat()
	c.Fighters[0].HP = 50
	c.startSkill(0, PowerSkill, Input{})
	ticks(c, 20, [2]Input{})
	closeValue(t, c.Fighters[0].HP, 55)
	if c.Fighters[0].BuffStacks != 1 {
		t.Fatal("buff")
	}
	c.damage(0, 1, 5, FanSkill, false, false)
	closeValue(t, c.Fighters[1].HP, 94.95)
	c = liveCombat()
	c.Fighters[0].MP = 0
	c.Fighters[0].BuffStacks = 100
	c.Fighters[1].BurnLeft = 5
	c.Fighters[1].BurnSource = 0
	c.Fighters[1].BurnNext = 60
	ticks(c, 299, [2]Input{})
	closeValue(t, c.Fighters[1].HP, 99.2)
	ticks(c, 1, [2]Input{})
	closeValue(t, c.Fighters[1].HP, 99)
	closeValue(t, c.Fighters[0].MP, 12.5)
	if c.Fighters[1].BurnLeft != 0 {
		t.Fatal("burn count")
	}
}
func TestFanSweptCollisionSingleExplosionAndRefresh(t *testing.T) {
	c := liveCombat()
	placeCombat(c, 0, 450, 150)
	placeCombat(c, 1, 950, 150)
	c.Fighters[0].MP = 50
	c.startSkill(0, FanSkill, Input{})
	explosions, hits := 0, 0
	for n := 0; n < 100; n++ {
		c.Step([2]Input{})
		for _, e := range c.Events() {
			if e.Kind == "explosion" {
				explosions++
			}
			if e.Kind == "hit" {
				hits++
			}
		}
	}
	if explosions != 1 || hits != 1 {
		t.Fatalf("explosions=%d hits=%d", explosions, hits)
	}
	closeValue(t, c.Fighters[1].HP, 94.8)
	closeValue(t, c.Fighters[0].MP, 50-10+100*CombatDT*2.5+1)
	// High-speed movement crosses the entire target in one step.
	c = liveCombat()
	placeCombat(c, 1, 900, 150)
	c.Projectiles = []Projectile{{ID: 1, Owner: 0, X: 600, Y: -100, Lane: 150, VX: 30000, Life: 10}}
	c.Step([2]Input{})
	closeValue(t, c.Fighters[1].HP, 95)
	if len(c.Projectiles) != 0 {
		t.Fatal("projectile survived collision")
	}
}
func TestRollStunAndInvulnerability(t *testing.T) {
	c := liveCombat()
	placeCombat(c, 0, 600, 150)
	placeCombat(c, 1, 840, 150)
	c.startSkill(0, RollSkill, Input{})
	hits := 0
	for n := 0; n < 60; n++ {
		c.Step([2]Input{})
		for _, e := range c.Events() {
			if e.Kind == "hit" {
				hits++
			}
		}
	}
	if hits != 1 {
		t.Fatalf("roll hit %d times", hits)
	}
	closeValue(t, c.Fighters[1].HP, 93)
	c = liveCombat()
	c.damage(0, 1, 7, RollSkill, true, false)
	until := c.Fighters[1].StunUntil
	if until-c.Tick != 30 {
		t.Fatal("stun duration")
	}
	if c.CanUse(1, DodgeSkill) {
		t.Fatal("stunned dodge")
	}
	ticks(c, 29, [2]Input{{}, {Right: true, JumpPressed: true, FanPressed: true}})
	if c.CanUse(1, PowerSkill) {
		t.Fatal("early recovery")
	}
	ticks(c, 1, [2]Input{})
	if !c.CanUse(1, PowerSkill) {
		t.Fatal("stun did not end at 0.5s")
	}
	c = liveCombat()
	c.startSkill(1, DodgeSkill, Input{})
	if c.damage(0, 1, 5, FanSkill, false, false) {
		t.Fatal("dodge did not evade")
	}
	c.Fighters[1].BurnLeft = 1
	c.Fighters[1].BurnNext = c.Tick + 1
	c.Fighters[1].BurnSource = 0
	c.Step([2]Input{})
	closeValue(t, c.Fighters[1].HP, 99.8)
	ticks(c, 10, [2]Input{})
	if c.Invulnerable(1) {
		t.Fatal("invulnerability exceeded 11 ticks")
	}
}
func TestInterruptKOAndNewMatchCleanup(t *testing.T) {
	c := liveCombat()
	c.startSkill(0, FanSkill, Input{})
	c.damage(1, 0, 1, NoSkill, false, false)
	ticks(c, 70, [2]Input{})
	if len(c.Projectiles) > 0 || c.Fighters[0].Action.Skill != NoSkill {
		t.Fatal("interrupted skill left active hitbox")
	}
	c.Fighters[1].HP = .1
	c.Fighters[1].BurnLeft = 1
	c.Fighters[1].BurnNext = c.Tick + 1
	c.Step([2]Input{})
	if c.Winner != 0 || c.Arena.Live() {
		t.Fatal("burn KO")
	}
	for _, f := range c.Fighters {
		if f.BurnLeft != 0 || f.Action.Skill != NoSkill {
			t.Fatal("KO cleanup")
		}
	}
	if NewCombat(false).Fighters[0].BuffStacks != 0 {
		t.Fatal("buff leaked to new match")
	}
}
func TestHeldInputAndFrameRateDeterminism(t *testing.T) {
	c := liveCombat()
	ticks(c, 300, [2]Input{{Attack: true}})
	closeValue(t, c.Fighters[1].HP, 100)
	var snapshots []CombatSnapshot
	for _, fps := range []int{30, 60, 144} {
		c = liveCombat()
		r := CombatRunner{}
		for n := 0; n < fps*2; n++ {
			in := [2]Input{{Right: true}}
			if n == 0 {
				in[0].PowerPressed = true
			}
			r.Advance(c, 1/float64(fps), in)
		}
		snapshots = append(snapshots, c.Snapshot())
	}
	for _, s := range snapshots {
		if s.Tick != 120 {
			t.Fatalf("tick %d", s.Tick)
		}
		closeValue(t, s.Fighters[0].MP, snapshots[0].Fighters[0].MP)
		closeValue(t, s.Fighters[0].X, snapshots[0].Fighters[0].X)
	}
	encoded, _ := json.Marshal(snapshots[0])
	if len(encoded) > 24000 {
		t.Fatalf("snapshot too large: %d", len(encoded))
	}
}
func TestAIUsesLegalTacticsForSeededScenarios(t *testing.T) {
	for _, difficulty := range []AIDifficulty{EasyAI, NormalAI, HardAI} {
		for scenario := 0; scenario < 8; scenario++ {
			c := liveCombat()
			c.Arena.Clock = 300
			ai := NewCombatAI(int64(20+scenario), difficulty)
			c.SetAI(1, ai)
			if scenario == 4 {
				c.Fighters[1].HP = 20
			}
			if scenario == 5 {
				c.Fighters[1].MP = 5
			}
			if scenario == 6 {
				placeCombat(c, 1, 1670, 30)
			}
			for n := 0; n < 3600 && c.Arena.Live(); n++ {
				var input Input
				switch scenario {
				case 1:
					input.Right = n%240 < 120
					input.Left = !input.Right
				case 2:
					input.AttackPressed = n%20 == 0
					input.Right = true
				case 3:
					input.FanPressed = n%110 == 0
				case 7:
					input.DodgePressed = n%90 == 0
				}
				c.Step([2]Input{input, {}})
				for _, f := range c.Fighters {
					if f.MP < 0 || f.MP > 100 || math.IsNaN(f.X) || f.X < 125 || f.X > ArenaWidth-125 {
						t.Fatalf("illegal AI state %+v", f)
					}
				}
			}
			if ai.Stats.Attacks+ai.Stats.Fans+ai.Stats.Rolls == 0 {
				t.Fatalf("AI never attacked %s/%d: %+v", difficulty, scenario, ai.Stats)
			}
		}
	}
}
func BenchmarkCombat60Hz(b *testing.B) {
	c := liveCombat()
	c.Arena.Clock = 1e9
	c.SetAI(0, NewCombatAI(1, HardAI))
	c.SetAI(1, NewCombatAI(2, HardAI))
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		if !c.Arena.Live() {
			c = liveCombat()
			c.Arena.Clock = 1e9
		}
		c.Step([2]Input{})
	}
}
