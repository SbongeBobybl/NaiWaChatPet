package play

import (
	"codexpet/reconstruction/rig"
	"math"
	"testing"
)

func TestFanPresentationAnchorAndLegacySettings(t *testing.T) {
	c := NewCombat(false)
	d := c.Definition(FanSkill)
	legacy := d
	legacy.DisplayScale = 0
	scale, offset := d.Presentation()
	oldScale, oldOffset := legacy.Presentation()
	if oldScale != scale || oldOffset != offset {
		t.Fatal("old combat sidecar loses the corrected fan scale")
	}
	q, ok := c.library.GetBoneAnchor(d.Clip, d.Anchor, float64(d.Effect)/CombatHz, true)
	if !ok {
		t.Fatal("authored hand anchor missing")
	}
	for _, facing := range []int{-1, 1} {
		c.Projectiles = nil
		c.Fighters[0].Facing = facing
		c.spawnFan(0)
		p := c.Projectiles[0]
		f := c.Fighters[0]
		x := f.X + float64(facing)*(240-(240+(q.X-240)*scale))
		y := f.Y - f.Height - (rig.GroundY - (rig.GroundY + (q.Y-rig.GroundY)*scale + offset))
		if math.Abs(p.X-x) > 1e-8 || math.Abs(p.Y-y) > 1e-8 {
			t.Fatal("rendered hand and fan origin disagree", facing)
		}
	}
	c.library = nil
	for _, facing := range []int{-1, 1} {
		c.Projectiles = nil
		c.Fighters[0].Facing = facing
		c.spawnFan(0)
		if (c.Projectiles[0].X-c.Fighters[0].X)*float64(facing) <= 0 {
			t.Fatal("fallback fan starts behind the fighter", facing)
		}
	}
}
