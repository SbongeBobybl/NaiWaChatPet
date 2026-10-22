package play

import "testing"

func TestSixHitEscapeAndUnrestrictedDodge(t *testing.T) {
	c := NewCombat(false)
	c.Arena.Phase = Fighting
	for i := 0; i < 6; i++ {
		c.Tick++
		if !c.damage(0, 1, 1, NoSkill, false, false) {
			t.Fatal("direct hit rejected")
		}
	}
	f := &c.Fighters[1]
	if f.HP != 72.5 || !c.Invulnerable(1) || f.ProtectionUntil != c.Tick+30 || !c.CanUse(1, DodgeSkill) {
		t.Fatal("six hits did not release stun and grant half-second escape", f)
	}
	if c.damage(0, 1, 1, NoSkill, false, false) || f.HP != 72.5 {
		t.Fatal("protected fighter took another direct hit")
	}
	c.Tick = f.ProtectionUntil - 1
	if !c.Invulnerable(1) {
		t.Fatal("protection ended early")
	}
	c.Tick++
	if c.Invulnerable(1) {
		t.Fatal("protection lasted longer than 30 ticks")
	}
	c = NewCombat(false)
	c.Arena.Phase = Fighting
	c.Step([2]Input{{DodgePressed: true}, {}})
	id := c.Fighters[0].Action.ID
	c.Step([2]Input{{DodgePressed: true}, {}})
	for i := 0; i < 17; i++ {
		c.Step([2]Input{})
	}
	if c.Fighters[0].Action.Skill != DodgeSkill || c.Fighters[0].Action.ID == id || c.Fighters[0].Cooldown[3] != 0 {
		t.Fatal("consecutive dodge input was lost or cooldown remained")
	}
}

