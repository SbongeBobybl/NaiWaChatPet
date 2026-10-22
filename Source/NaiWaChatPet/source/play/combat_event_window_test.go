package play

import "testing"

func TestSnapshotEffectsExpireWithoutGrowingHistory(t *testing.T) {
	c := NewCombat(false)
	for n := 0; n < 96; n++ {
		c.emit("hit", 0, 1, 0, 0, 1, NoSkill, false)
	}
	if len(c.Snapshot().Events) != 96 {
		t.Fatal("fresh events lost")
	}
	c.Tick = CombatHz
	if len(c.Snapshot().Events) != 96 {
		t.Fatal("delivery overlap too short")
	}
	c.Tick++
	if len(c.Snapshot().Events) != 0 {
		t.Fatal("expired effects keep crossing the network")
	}
	if len(c.recent) != 96 {
		t.Fatal("snapshot modified combat history")
	}
}
