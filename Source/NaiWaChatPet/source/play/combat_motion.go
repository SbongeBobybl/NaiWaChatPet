package play

import "math"

// The same movement response drives authority and bounded cosmetic prediction.
func combatSettle(grounded bool, vx, vy, wantX, wantY float64) float64 {
	if !grounded {
		return .10
	}
	if wantX == 0 && wantY == 0 {
		return .025
	}
	if vx*wantX+vy*wantY < 0 {
		return .020
	}
	return .035
}
func (c *Controller) MovementUnlocked() bool {
	return c.act == actNone || c.cfg.Combat && (c.act == actGuard || c.act == actRise || c.act == actStrike && c.at/c.strikeTime >= .85)
}

func PredictCombatPosition(f FighterState, in Input, dt float64) (float64, float64) {
	x, y := f.X, f.Y
	if f.MoveLocked || f.HP <= 0 || math.IsNaN(dt) || math.IsInf(dt, 0) {
		return x, y
	}
	dt = math.Max(0, math.Min(.05, dt))
	speed := walkSpeed
	if in.Run {
		speed = runSpeed
	}
	wantX, wantY := axis(in.Left, in.Right)*speed, axis(in.Up, in.Down)*speed*climb
	if wantX != 0 && wantY != 0 {
		wantX, wantY = wantX/math.Sqrt2, wantY/math.Sqrt2
	}
	vx, vy := f.VX, f.VY
	for dt > 1e-9 {
		step := math.Min(CombatDT, dt)
		blend := 1 - math.Exp(-step/combatSettle(f.Height == 0, vx, vy, wantX, wantY))
		vx, vy = vx+(wantX-vx)*blend, vy+(wantY-vy)*blend
		if wantX == 0 && wantY == 0 && math.Hypot(vx, vy) < 3 {
			vx, vy = 0, 0
		}
		nx, ny := x+vx*step, y+vy*step
		freeX, freeY := nx >= 125 && nx <= ArenaWidth-125, ny >= 0 && ny <= ArenaDepth
		switch {
		case freeX && freeY:
			x, y = nx, ny
		case vx != 0 && freeX:
			x = nx
			vy = 0
		case vy != 0 && freeY:
			y = ny
			vx = 0
		default:
			vx, vy = 0, 0
		}
		dt -= step
	}
	return x, y
}
