package main

import "codexpet/reconstruction/play"

// The authored tucked roll is much smaller than the standing pose. Zoom the
// display 30% only during the compact phase; keep entry/exit sizes continuous.
func combatSkillZoom(skill play.SkillID, age int) float64 {
	if skill != play.RollSkill {
		return 1
	}
	weight := min(1.0, max(0, min(float64(age-7)/8, float64(58-age)/8)))
	return 1 + .30*weight*weight*(3-2*weight)
}
