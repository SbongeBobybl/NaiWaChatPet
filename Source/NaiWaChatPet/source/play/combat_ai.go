package play

import "math"

type AIDifficulty string

const (
	EasyAI   AIDifficulty = "easy"
	NormalAI AIDifficulty = "normal"
	HardAI   AIDifficulty = "hard"
)

type AIStats struct{ Decisions, Attacks, Fans, Rolls, Powers, Dodges, Retreats, Adjustments int }
type CombatAI struct {
	pressure                   float64
	Difficulty                 AIDifficulty
	Stats                      AIStats
	seed                       uint64
	next                       uint64
	held                       Input
	lastFoeSkill               SkillID
	observedFans, observedRush float64
	chain                      uint64
	lastHP                     float64
}

func NewCombatAI(seed int64, d AIDifficulty) *CombatAI {
	if d != EasyAI && d != HardAI {
		d = NormalAI
	}
	return &CombatAI{Difficulty: d, seed: uint64(seed) | 1, lastHP: 1}
}
func (a *CombatAI) random() float64 {
	a.seed ^= a.seed << 13
	a.seed ^= a.seed >> 7
	a.seed ^= a.seed << 17
	return float64(a.seed>>11) / float64(uint64(1)<<53)
}
func (a *CombatAI) Think(c *Combat, i int) Input {
	f, o := &c.Fighters[i], &c.Fighters[1-i]
	if !c.Arena.Live() || c.Tick < f.StunUntil || c.Tick < f.HitUntil || f.HP <= 0 {
		a.held = Input{}
		return Input{}
	}
	if c.Tick < a.next {
		in := a.held
		in.ClearEdges()
		return in
	}
	delay := 12
	quality := .76
	if a.Difficulty == EasyAI {
		delay = 24
		quality = .42
	}
	if a.Difficulty == HardAI {
		delay = 7
		quality = .94
	}
	a.next = c.Tick + uint64(delay)
	a.Stats.Decisions++
	dx, dy := o.X-f.X, o.Y-f.Y
	distance := math.Abs(dx)
	toward := sign(dx)
	lane := SameLane(f.Y, o.Y, 1)
	a.observedFans *= .985
	a.observedRush *= .985
	if o.Action.Skill != a.lastFoeSkill {
		if o.Action.Skill == FanSkill {
			a.observedFans++
		}
		a.lastFoeSkill = o.Action.Skill
	}
	if o.View.Stance && distance < 440 {
		a.observedRush += .1
	}
	var in Input
	defer func() { a.held = in; a.held.ClearEdges() }()
	if f.Action.Skill != NoSkill {
		return in
	}
	danger := o.View.Stance && o.View.Strike > 0 && o.View.Strike < .6 && distance < 430 && lane
	for _, p := range c.Projectiles {
		if p.Owner == i || !SameLane(p.Lane, f.Y, 1) {
			continue
		}
		arrival := (f.X - p.X) / p.VX
		if arrival >= 0 && arrival < .35 && math.Abs(p.Y-(f.Y-f.Height-260)) < 240 {
			danger = true
		}
	}
	if danger && c.CanUse(i, DodgeSkill) && a.random() < quality {
		in.DodgePressed = true
		in.Up = f.Y > ArenaDepth/2
		in.Down = !in.Up
		if f.Y < 60 || f.Y > ArenaDepth-60 {
			in.Left = toward > 0
			in.Right = toward < 0
		}
		a.Stats.Dodges++
		return in
	}
	if f.Facing != toward && lane {
		in.Left = dx < 0
		in.Right = dx > 0
		return in
	}
	health := f.HP / f.MaxHP
	if health < a.lastHP {
		a.pressure += (a.lastHP - health) * 30
	}
	a.lastHP = health
	a.pressure *= .92
	if c.CanUse(i, PowerSkill) && f.MP >= 50 && (health < .78 || f.BuffStacks < 2 && distance > 650) && (distance > 570 || c.Tick < o.StunUntil || o.Action.Skill == PowerSkill) {
		in.PowerPressed = true
		a.Stats.Powers++
		return in
	}
	if lane && distance >= 420 && distance < 1450 && f.MP >= 30 && c.CanUse(i, FanSkill) && a.random() < quality {
		in.FanPressed = true
		a.Stats.Fans++
		return in
	}
	if lane && distance > 205 && distance < 355 && f.MP >= 40 && o.Height < 40 && c.CanUse(i, RollSkill) && a.random() < quality*.7 {
		in.RollPressed = true
		a.Stats.Rolls++
		return in
	}
	if lane && distance >= 185 && distance < 405 && o.Height < 270 {
		if c.Tick >= a.chain {
			in.AttackPressed = true
			a.Stats.Attacks++
			a.chain = c.Tick + uint64(15+int(a.random()*7))
		}
		return in
	}
	retreat := (health < .3 && health < o.HP/o.MaxHP && f.MP < 30) || (lane && distance < 180)
	if retreat {
		in.Left = toward > 0
		in.Right = toward < 0
		a.Stats.Retreats++
	} else {
		in.Left = dx < 0
		in.Right = dx > 0
		in.Run = distance > 650
	}
	in.Up = dy < -28
	in.Down = dy > 28
	if a.pressure > 1.5 && distance < 460 {
		in.Up = f.Y > ArenaDepth/2
		in.Down = !in.Up
		a.Stats.Adjustments++
	}
	if a.observedFans > 1.5 && distance > 400 {
		in.Up = f.Y > ArenaDepth/2
		in.Down = !in.Up
		a.Stats.Adjustments++
	}
	if f.X <= 145 && in.Left || f.X >= ArenaWidth-145 && in.Right {
		in.Left, in.Right = !in.Left, !in.Right
		in.Up = f.Y > ArenaDepth/2
		in.Down = !in.Up
		a.Stats.Adjustments++
	}
	return in
}
