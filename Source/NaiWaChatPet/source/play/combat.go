package play

// Combat is the only authority for combat rules, on desktop and headless
// servers alike. Coordinates are logical canvas units; simulation is 60 Hz.
import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"

	"codexpet/reconstruction/rig"
)

const CombatHz = 60
const CombatDT = 1.0 / CombatHz
const ArenaWidth = 1800.0
const ArenaDepth = 300.0

const ComboProtectionHits = 6
const ComboProtectionTicks = 30
const ReceivedChainGapTicks = 66

type SkillID int

const (
	NoSkill SkillID = iota
	FanSkill
	RollSkill
	PowerSkill
	DodgeSkill
)

type SkillDefinition struct {
	ID             string  `json:"id"`
	Clip           string  `json:"clip"`
	Cost           float64 `json:"cost"`
	Cooldown       int     `json:"cooldown"`
	Duration       int     `json:"duration"`
	Effect         int     `json:"effect"`
	ActiveEnd      int     `json:"activeEnd"`
	Recovery       int     `json:"recovery"`
	Damage         float64 `json:"damage"`
	Anchor         string  `json:"anchor,omitempty"`
	DisplayScale   float64 `json:"displayScale,omitempty"`
	DisplayOffsetY float64 `json:"displayOffsetY,omitempty"`
}

// Presentation aligns the supplied full-body skill canvas with the existing
// standing rig. Missing values in older sidecars retain the corrected defaults.
func (d SkillDefinition) Presentation() (float64, float64) {
	if d.DisplayScale > 0 {
		return d.DisplayScale, d.DisplayOffsetY
	}
	if d.ID == "fan" {
		return 1.12, 0
	}
	return 1, 0
}

type CombatRules struct {
	Version      int               `json:"version"`
	MaxMP        float64           `json:"maxMP"`
	RegenMP      float64           `json:"regenMP"`
	HitMP        float64           `json:"hitMP"`
	DamageScale  float64           `json:"damageScale"`
	NormalDamage float64           `json:"normalDamage"`
	BurnDamage   float64           `json:"burnDamage"`
	BurnTicks    int               `json:"burnTicks"`
	StunTicks    int               `json:"stunTicks"`
	Heal         float64           `json:"heal"`
	BuffPerStack float64           `json:"buffPerStack"`
	Skills       []SkillDefinition `json:"skills"`
}

//go:embed combat.json
var defaultRulesJSON []byte

func DefaultCombatRules() CombatRules {
	var r CombatRules
	if err := json.Unmarshal(defaultRulesJSON, &r); err != nil {
		panic(err)
	}
	return r
}
func ParseCombatRules(b []byte) (CombatRules, error) {
	var r CombatRules
	err := json.Unmarshal(b, &r)
	if err != nil {
		return r, err
	}
	// Older sidecars inherit this release's retuned damage rather than reducing every hit with HP.
	if r.DamageScale == 0 {
		r.DamageScale = DefaultCombatRules().DamageScale
	}
	if r.DamageScale < .1 || r.DamageScale > 4 || r.Version != 1 || r.MaxMP != 100 || r.RegenMP < 0 || r.RegenMP > 100 || r.HitMP < 0 || r.HitMP > 100 || r.NormalDamage <= 0 || r.NormalDamage > 1 || r.BurnDamage < 0 || r.BurnDamage > 1 || r.BurnTicks < 1 || r.BurnTicks > 60 || r.StunTicks < 1 || r.StunTicks > 300 || r.Heal < 0 || r.Heal > 1 || r.BuffPerStack < 0 || r.BuffPerStack > 1 || len(r.Skills) != 4 {
		return r, fmt.Errorf("invalid combat rules")
	}
	for _, s := range r.Skills {
		if s.Cost < 0 || s.Cost > r.MaxMP || s.Duration < 1 || s.Duration > 600 || s.Effect < 0 || s.Effect > s.Duration || s.ActiveEnd < s.Effect || s.ActiveEnd > s.Duration || s.Cooldown < 0 || s.Cooldown > 36000 || s.Damage < 0 || s.Damage > 1 || s.DisplayScale != 0 && (s.DisplayScale < .75 || s.DisplayScale > 1.5) || math.Abs(s.DisplayOffsetY) > 80 {
			return r, fmt.Errorf("invalid skill %s", s.ID)
		}
	}
	return r, nil
}

type SkillAction struct {
	Skill  SkillID `json:"skill"`
	Age    int     `json:"age"`
	ID     uint64  `json:"id"`
	Hit    bool    `json:"hit"`
	Facing int     `json:"facing"`
	DX, DY float64
}
type FighterState struct {
	X, Y, Height        float64
	Facing              int
	HP, MaxHP, MP       float64
	VX, VY              float64
	MoveLocked          bool
	BuffStacks          int
	StunUntil, HitUntil uint64
	ProtectionUntil     uint64
	ReceivedHits        int
	lastReceivedTick    uint64
	BurnLeft            int
	BurnNext            uint64
	BurnSource          int
	Cooldown            [4]uint64
	Action              SkillAction
	View                View
	Combo               int
	controller          *Controller
	buffered            SkillID
	bufferUntil         uint64
	queuedControl       Input
	controlUntil        uint64
}
type Projectile struct {
	ID             uint64
	Owner          int
	X, Y, Lane, VX float64
	Life           int
}
type CombatEvent struct {
	ID            uint64
	Tick          uint64
	Kind          string
	Actor, Target int
	X, Y, Amount  float64
	Skill         SkillID
	Hard          bool
}
type CombatSnapshot struct {
	HPShown       [2]float64
	Rules         *CombatRules `json:"rules,omitempty"`
	Tick          uint64
	Fighters      [2]FighterState
	Projectiles   []Projectile
	Phase         Phase
	Clock         float64
	Round, Winner int
	Events        []CombatEvent
	Ack           [2]uint64
}
type Combat struct {
	Rules         CombatRules
	Fighters      [2]FighterState
	Projectiles   []Projectile
	Arena         *Arena
	Tick          uint64
	Challenge     bool
	Winner        int
	resultSettled bool
	library       *rig.SkillAnimations
	stance        *rig.Puppet
	nextID        uint64
	events        []CombatEvent
	recent        []CombatEvent
	comboUntil    [2]uint64
	ai            [2]*CombatAI
}

func NewCombat(challenge bool) *Combat {
	c := &Combat{Rules: DefaultCombatRules(), Arena: NewArena(), Challenge: challenge, Winner: -1}
	c.library, _ = rig.DefaultSkillAnimations()
	if c.library != nil && c.library.GetAnimationDuration("skill_dodge") > 0 {
		c.Rules.Skills[3].Clip = "skill_dodge"
	}
	c.stance, _ = rig.Stance()
	// Authored events take precedence over sidecar fallback timings.
	if c.library != nil {
		for i := range c.Rules.Skills {
			d := &c.Rules.Skills[i]
			if d.Clip == "" {
				continue
			}
			d.Duration = int(math.Ceil(c.library.GetAnimationDuration(d.Clip) * CombatHz))
			for _, e := range c.library.GetAnimationEvents(d.Clip) {
				switch e.Name {
				case "fan_release", "roll_start", "power_apply", "invulnerable_start":
					d.Effect = int(math.Round(e.Time * CombatHz))
				case "roll_land", "invulnerable_end":
					d.ActiveEnd = int(math.Round(e.Time * CombatHz))
				}
			}
		}
	}
	if !challenge {
		c.Arena.Rival.HP = PetHP
		c.Arena.FoeHP = PetHP
		c.Arena.FoeShown = PetHP
		c.Arena.Rival.Tag = "PLAYER 2"
	}
	c.resetFighter(0, PetHP)
	c.resetFighter(1, c.Arena.Rival.HP)
	return c
}
func (c *Combat) resetFighter(i int, hp float64) {
	x, facing := 450.0, 1
	if i == 1 {
		x, facing = 1350, -1
	}
	ctl := New(Config{Unit: 1, Combat: true, Free: func(x, y float64) bool { return x >= 125 && x <= ArenaWidth-125 && y >= 0 && y <= ArenaDepth }})
	ctl.Place(x, 150, facing)
	c.Fighters[i] = FighterState{X: x, Y: 150, Facing: facing, HP: hp, MaxHP: hp, MP: c.Rules.MaxMP, controller: ctl}
}
func (c *Combat) SetAI(player int, ai *CombatAI) {
	if player >= 0 && player < 2 {
		c.ai[player] = ai
	}
}

func (c *Combat) ControllerFor(i int) *Controller {
	if i < 0 || i > 1 {
		return nil
	}
	return c.Fighters[i].controller
}
func (c *Combat) Definition(id SkillID) SkillDefinition {
	if id < FanSkill || id > DodgeSkill {
		return SkillDefinition{}
	}
	return c.Rules.Skills[int(id)-1]
}
func (c *Combat) emit(kind string, actor, target int, x, y, amount float64, skill SkillID, hard bool) {
	c.nextID++
	e := CombatEvent{c.nextID, c.Tick, kind, actor, target, x, y, amount, skill, hard}
	c.events = append(c.events, e)
	c.recent = append(c.recent, e)
	if len(c.recent) > 96 {
		c.recent = c.recent[len(c.recent)-96:]
	}
}
func (c *Combat) Events() []CombatEvent { out := c.events; c.events = nil; return out }
func (c *Combat) Snapshot() CombatSnapshot {
	s := CombatSnapshot{Tick: c.Tick, Fighters: c.Fighters, Projectiles: append([]Projectile(nil), c.Projectiles...), Phase: c.Arena.Phase, Clock: c.Arena.Clock, Round: c.Arena.Round, Winner: c.Winner, Events: append([]CombatEvent(nil), c.recent...)}
	// Keep a one-second delivery overlap, without transmitting an entire old
	// fight's effects twenty times per second after action has stopped.
	for len(s.Events) > 0 && c.Tick-s.Events[0].Tick > CombatHz {
		s.Events = s.Events[1:]
	}
	s.Rules = &c.Rules
	s.HPShown = [2]float64{c.Arena.PetShown, c.Arena.FoeShown}
	return s
}
func (c *Combat) CanUse(i int, id SkillID) bool {
	if i < 0 || i > 1 || id < FanSkill || id > DodgeSkill || !c.Arena.Live() {
		return false
	}
	f := &c.Fighters[i]
	d := c.Definition(id)
	return f.HP > 0 && f.MP+1e-9 >= d.Cost && c.Tick >= f.StunUntil && c.Tick >= f.HitUntil && c.Tick >= f.Cooldown[id-1] && (f.Action.Skill == NoSkill || c.canRecoverSkill(i)) && f.controller.CanCancel() && (id != RollSkill || f.Height == 0)
}

// Recovery may be cancelled only after all authored effects and active hit frames.
func (c *Combat) canRecoverSkill(i int) bool {
	a := c.Fighters[i].Action
	if a.Skill == NoSkill || a.Skill == DodgeSkill {
		return false
	}
	d := c.Definition(a.Skill)
	return a.Age >= max(d.Recovery, d.Effect+1, d.ActiveEnd+1)
}
func (c *Combat) endSkill(i int) {
	f := &c.Fighters[i]
	if f.Action.Skill != NoSkill {
		c.emit("skill_end", i, -1, f.X, f.Y, 0, f.Action.Skill, false)
		f.Action = SkillAction{}
	}
}
func (c *Combat) Invulnerable(i int) bool {
	if i < 0 || i >= len(c.Fighters) {
		return false
	}
	if c.Tick < c.Fighters[i].ProtectionUntil {
		return true
	}
	a := c.Fighters[i].Action
	return a.Skill == DodgeSkill && a.Age >= c.Definition(DodgeSkill).Effect && a.Age < c.Definition(DodgeSkill).ActiveEnd
}
func (c *Combat) startSkill(i int, id SkillID, in Input) bool {
	if !c.CanUse(i, id) {
		return false
	}
	f := &c.Fighters[i]
	d := c.Definition(id)
	f.MP = math.Max(0, f.MP-d.Cost)
	f.Cooldown[id-1] = c.Tick + uint64(d.Cooldown)
	if id == DodgeSkill {
		f.Cooldown[id-1] = 0
	}
	c.endSkill(i)
	c.nextID++
	f.Action = SkillAction{Skill: id, ID: c.nextID, Facing: f.Facing}
	if id == DodgeSkill {
		dx, dy := axis(in.Left, in.Right), axis(in.Up, in.Down)
		if dx == 0 && dy == 0 {
			dx = -float64(f.Facing)
		}
		length := math.Hypot(dx, dy)
		f.Action.DX, f.Action.DY = dx/length, dy/length
	}
	f.controller.CancelAction()
	f.buffered = NoSkill
	c.emit("skill_start", i, -1, f.X, f.Y-f.Height-260, 0, id, false)
	return true
}
func (c *Combat) interrupt(i int) {
	f := &c.Fighters[i]
	f.Action = SkillAction{}
	f.buffered = NoSkill
	f.bufferUntil = 0
	f.queuedControl = Input{}
	f.controlUntil = 0
	f.controller.CancelAction()
}

// Step consumes exactly one simulation tick. Press edges must appear once;
// held fields never initiate a skill. Rendering and hitstop cannot stop time.
func (c *Combat) Step(inputs [2]Input) {
	c.Tick++
	c.events = c.events[:0]
	oldRound := c.Arena.Round
	if !c.Challenge && c.Arena.Phase == KnockedOut {
		c.Arena.Since += CombatDT
		if c.Arena.Since >= koTime {
			c.Arena.Phase = Over
		}
	} else {
		c.Arena.Step(CombatDT)
	}
	if c.Arena.Round != oldRound {
		c.Winner = -1
		c.resultSettled = false
		keep := c.Fighters[0].BuffStacks
		mp := c.Fighters[0].MP
		weapon := c.Fighters[0].controller.weapon.ID
		c.resetFighter(0, PetHP)
		c.SetWeapon(0, weapon)
		c.Fighters[0].HP = c.Arena.PetHP
		c.Fighters[0].MP = mp
		c.Fighters[0].BuffStacks = keep
		c.resetFighter(1, c.Arena.Rival.HP)
		c.Projectiles = nil
		c.emit("round", 0, 1, 0, 0, 0, NoSkill, false)
	}
	if !c.Arena.Live() {
		if c.Arena.Phase == KnockedOut && !c.resultSettled {
			c.finish()
		}
		for i := range c.Fighters {
			c.Fighters[i].controller.Step(CombatDT, Input{})
			c.syncPosition(i)
		}
		return
	}
	for i := range c.Fighters {
		f := &c.Fighters[i]
		f.MP = math.Min(c.Rules.MaxMP, f.MP+c.Rules.RegenMP*CombatDT)
		if f.BurnLeft > 0 && c.Tick >= f.BurnNext {
			f.BurnLeft--
			f.BurnNext += CombatHz
			c.damage(f.BurnSource, i, f.MaxHP*c.Rules.BurnDamage, NoSkill, false, true)
			if !c.Arena.Live() {
				return
			}
		}
		if c.Tick >= c.comboUntil[i] {
			f.Combo = 0
		}
		if f.ReceivedHits > 0 && c.Tick-f.lastReceivedTick > ReceivedChainGapTicks {
			f.ReceivedHits = 0
		}
		if c.ai[i] != nil {
			inputs[i] = c.ai[i].Think(c, i)
		}
	}
	// Prepare both fighters before hit detection, making simultaneous attacks
	// use the same tick's poses and avoiding player-order movement advantage.
	for i, in := range inputs {
		f := &c.Fighters[i]
		blocked := c.Tick < f.StunUntil || c.Tick < f.HitUntil
		// Tick deadlines are authoritative; avoid the controller consuming a
		// queued press on its final floating-point stun countdown frame.
		if !blocked {
			f.controller.stun = 0
		}
		unlock := max(f.StunUntil, f.HitUntil)
		if blocked && unlock-c.Tick > 6 {
			in = Input{}
			f.buffered = NoSkill
		}
		id := NoSkill
		switch {
		case in.DodgePressed:
			id = DodgeSkill
		case in.PowerPressed:
			id = PowerSkill
		case in.RollPressed:
			id = RollSkill
		case in.FanPressed:
			id = FanSkill
		}
		if id != NoSkill {
			f.buffered = id
			f.bufferUntil = c.Tick + 12
			if id == DodgeSkill && f.Action.Skill == DodgeSkill {
				f.bufferUntil = c.Tick + uint64(max(0, c.Definition(DodgeSkill).Duration-f.Action.Age)+1)
			}
		}
		if f.buffered != NoSkill && c.Tick <= f.bufferUntil {
			c.startSkill(i, f.buffered, in)
		} else if c.Tick > f.bufferUntil {
			f.buffered = NoSkill
		}
		if (f.Action.Skill != NoSkill || blocked && unlock-c.Tick <= 6) && (in.AttackPressed || in.JumpPressed) {
			f.queuedControl.MergeEdges(Input{AttackPressed: in.AttackPressed, JumpPressed: in.JumpPressed})
			f.controlUntil = c.Tick + 9
		}
		if c.Tick > f.controlUntil {
			f.queuedControl = Input{}
		}
		if !blocked && c.canRecoverSkill(i) && (in.steering() || in.AttackPressed || in.JumpPressed || f.queuedControl.AttackPressed || f.queuedControl.JumpPressed) {
			c.endSkill(i)
		}
		if f.Action.Skill != NoSkill {
			a := &f.Action
			d := c.Definition(a.Skill)
			before := float64(a.Age) / CombatHz
			a.Age++
			f.controller.Step(CombatDT, Input{})
			dx, dy := 0.0, 0.0
			if a.Skill == RollSkill && c.library != nil {
				p, q := c.library.RootTranslation(d.Clip, before), c.library.RootTranslation(d.Clip, float64(a.Age)/CombatHz)
				dx = -(q.X - p.X) * float64(a.Facing)
			}
			if a.Skill == DodgeSkill {
				v := smooth(float64(a.Age)/float64(d.Duration)) - smooth(float64(a.Age-1)/float64(d.Duration))
				dx, dy = a.DX*300*v, a.DY*210*v
			}
			c.moveSkill(i, dx, dy)
			c.syncPosition(i)
			if a.Age == d.Effect || d.Effect == 0 && a.Age == 1 {
				switch a.Skill {
				case FanSkill:
					c.spawnFan(i)
				case PowerSkill:
					f.BuffStacks++
					heal := math.Min(f.MaxHP-f.HP, f.MaxHP*c.Rules.Heal)
					f.HP += heal
					c.syncHealth()
					c.emit("power", i, i, f.X, f.Y-240, heal, PowerSkill, false)
				}
			}
		} else {
			if blocked {
				in = Input{}
			}
			if !blocked {
				in.MergeEdges(f.queuedControl)
				f.queuedControl = Input{}
			}
			in.Attack = false
			f.controller.Step(CombatDT, in)
			c.syncPosition(i)
		}
	}
	var pendingEvents [2][]Event
	for i := range c.Fighters {
		pendingEvents[i] = c.Fighters[i].controller.Events()
	}
	for i := range c.Fighters {
		f := &c.Fighters[i]
		for _, event := range pendingEvents[i] {
			if event == Struck || event == StruckHard {
				c.melee(i, event == StruckHard)
			}
		}
		a := &f.Action
		if a.Skill == RollSkill && a.Age >= c.Definition(RollSkill).Effect && a.Age <= c.Definition(RollSkill).ActiveEnd && !a.Hit {
			other := &c.Fighters[1-i]
			if SameLane(f.Y, other.Y, 1) && math.Abs(f.X-other.X) <= 265 && math.Abs(f.Height-other.Height) < 210 {
				if c.damage(i, 1-i, other.MaxHP*c.Definition(RollSkill).Damage, RollSkill, true, false) {
					a.Hit = true
				}
			}
		}
		if a.Skill != NoSkill && a.Age >= c.Definition(a.Skill).Duration {
			c.endSkill(i)
		}
	}
	c.stepProjectiles()
	if c.Arena.Clock <= 0 || !c.Arena.Live() {
		c.finish()
	}
}
func (c *Combat) syncPosition(i int) {
	f := &c.Fighters[i]
	f.X, f.Y = f.controller.Position()
	f.VX, f.VY = f.controller.vx, f.controller.vy
	f.MoveLocked = !f.controller.MovementUnlocked()
	f.View = f.controller.View()
	f.Height = f.View.Height
	f.Facing = -1
	if f.View.FacingRight {
		f.Facing = 1
	}
}
func (c *Combat) syncHealth() { c.Arena.PetHP = c.Fighters[0].HP; c.Arena.FoeHP = c.Fighters[1].HP }
func (c *Combat) moveSkill(i int, dx, dy float64) {
	f := &c.Fighters[i]
	x, y := f.controller.Position()
	other := c.Fighters[1-i]
	nx, ny := math.Max(125, math.Min(ArenaWidth-125, x+dx)), math.Max(0, math.Min(ArenaDepth, y+dy))
	if SameLane(ny, other.Y, 1) && math.Abs(x-other.X) >= 190 && math.Abs(nx-other.X) < 190 {
		nx = other.X + float64(sign(x-other.X))*190
	}
	f.controller.Place(nx, ny, f.Action.Facing)
}
func (c *Combat) spawnFan(i int) {
	f := &c.Fighters[i]
	d := c.Definition(FanSkill)
	p := rig.Vec{X: 145, Y: 280}
	if c.library != nil {
		if q, ok := c.library.GetBoneAnchor(d.Clip, d.Anchor, float64(d.Effect)/CombatHz, true); ok {
			p = q
		}
	}
	scale, offset := d.Presentation()
	p.X = 240 + (p.X-240)*scale
	p.Y = rig.GroundY + (p.Y-rig.GroundY)*scale + offset
	dx := 240 - p.X
	c.nextID++
	c.Projectiles = append(c.Projectiles, Projectile{ID: c.nextID, Owner: i, X: f.X + float64(f.Facing)*dx, Y: f.Y - f.Height - (rig.GroundY - p.Y), Lane: f.Y, VX: float64(f.Facing) * 1150, Life: 120})
	c.emit("fan_release", i, -1, c.Projectiles[len(c.Projectiles)-1].X, c.Projectiles[len(c.Projectiles)-1].Y, 0, FanSkill, false)
}
func (c *Combat) melee(i int, hard bool) {
	f, other := &c.Fighters[i], &c.Fighters[1-i]
	power := 1.0
	if hard {
		power = 1.9
	}
	p := rig.Palm(c.stance, rig.StrikePose(rig.StrikeImpact, power))
	x := f.X + float64(f.Facing)*(240-p.X+22)
	y := f.Y - f.Height - (rig.GroundY - p.Y)
	w := f.controller.weapon
	reach := 105 * w.Reach
	if SameLane(f.Y, other.Y, 1) && Reaches(x, y, other.X, other.Y, other.Height, 1, reach) {
		c.damage(i, 1-i, other.MaxHP*c.Rules.NormalDamage*w.Damage, NoSkill, hard, false)
	}
}
func (c *Combat) SetWeapon(i int, id string) {
	if i >= 0 && i < 2 {
		c.Fighters[i].controller.weapon = FindWeapon(id)
	}
}
func (c *Combat) stepProjectiles() {
	kept := c.Projectiles[:0]
	for _, p := range c.Projectiles {
		if !c.Arena.Live() {
			break
		}
		before := p.X
		p.X += p.VX * CombatDT
		p.Life--
		other := &c.Fighters[1-p.Owner]
		left, right := math.Min(before, p.X)-28, math.Max(before, p.X)+28
		if SameLane(p.Lane, other.Y, 1) && right >= other.X-125 && left <= other.X+125 && p.Y >= other.Y-other.Height-480 && p.Y <= other.Y-other.Height+28 {
			if c.damage(p.Owner, 1-p.Owner, other.MaxHP*c.Definition(FanSkill).Damage, FanSkill, true, false) && c.Arena.Live() {
				other.BurnLeft = c.Rules.BurnTicks
				other.BurnNext = c.Tick + CombatHz
				other.BurnSource = p.Owner
			}
			continue
		}
		if p.Life > 0 && p.X >= -100 && p.X <= ArenaWidth+100 {
			kept = append(kept, p)
		}
	}
	c.Projectiles = kept
}
func (c *Combat) damage(actor, target int, base float64, skill SkillID, hard, burn bool) bool {
	if !c.Arena.Live() || c.Fighters[target].HP <= 0 || (!burn && c.Invulnerable(target)) {
		return false
	}
	src, dst := &c.Fighters[actor], &c.Fighters[target]
	damage := base * c.Rules.DamageScale
	if !burn {
		damage *= 1 + c.Rules.BuffPerStack*float64(src.BuffStacks)
		src.MP = math.Min(c.Rules.MaxMP, src.MP+c.Rules.HitMP)
		src.Combo++
		c.comboUntil[actor] = c.Tick + 66
	}
	dst.HP = math.Max(0, dst.HP-damage)
	c.syncHealth()
	kind := "hit"
	if burn {
		kind = "burn"
	}
	c.emit(kind, actor, target, dst.X, dst.Y-dst.Height-260, damage, skill, hard)
	if !burn {
		c.interrupt(target)
		stun := 12
		force := 650.0
		if hard {
			stun = 18
			force = 950
		}
		if skill == NoSkill {
			force *= src.controller.weapon.Knock
		}
		if skill == RollSkill {
			stun = c.Rules.StunTicks
			dst.StunUntil = max(dst.StunUntil, c.Tick+uint64(stun))
		}
		dst.HitUntil = c.Tick + uint64(stun)
		dst.controller.Hit(src.Facing, force, float64(stun)/CombatHz, 0)
		if skill == FanSkill {
			c.emit("explosion", actor, target, dst.X, dst.Y-dst.Height-260, damage, skill, true)
		}
		if dst.HP > 0 {
			if dst.ReceivedHits == 0 || c.Tick-dst.lastReceivedTick > ReceivedChainGapTicks {
				dst.ReceivedHits = 0
			}
			dst.lastReceivedTick = c.Tick
			dst.ReceivedHits++
			if dst.ReceivedHits >= ComboProtectionHits {
				dst.ReceivedHits = 0
				dst.ProtectionUntil = c.Tick + ComboProtectionTicks
				dst.StunUntil, dst.HitUntil = c.Tick, c.Tick
				dst.controller.stun = 0
				c.interrupt(target)
				c.emit("combo_guard", target, target, dst.X, dst.Y-dst.Height-260, .5, NoSkill, false)
			}
		}
	}
	if dst.HP <= 0 {
		c.Arena.events = append(c.Arena.events, AnnounceKO)
		c.Arena.finish(target == 1)
		c.Winner = actor
		c.emit("ko", actor, target, dst.X, dst.Y, 0, skill, true)
		c.finish()
	}
	return true
}
func (c *Combat) finish() {
	if c.resultSettled {
		return
	}
	c.resultSettled = true
	// Online timeout compares remaining health fractions; an exact tie is a draw.
	if !c.Challenge && c.Arena.Clock <= 0 {
		left := c.Fighters[0].HP / c.Fighters[0].MaxHP
		right := c.Fighters[1].HP / c.Fighters[1].MaxHP
		switch {
		case math.Abs(left-right) < 1e-9:
			c.Winner = -1
		case left > right:
			c.Winner = 0
		default:
			c.Winner = 1
		}
	} else if c.Winner < 0 {
		if c.Arena.Won() {
			c.Winner = 0
		} else {
			c.Winner = 1
		}
	}
	c.Projectiles = nil
	for i := range c.Fighters {
		c.interrupt(i)
		f := &c.Fighters[i]
		f.BurnLeft = 0
		f.StunUntil = 0
		f.ProtectionUntil, f.ReceivedHits = 0, 0
		f.HitUntil = 0
		f.controller.Down(f.HP <= 0)
	}
}

// Advance batches variable host time without changing the simulation step.
// Pending edges survive a render frame that contains no simulation tick.
type CombatRunner struct {
	Accumulator float64
	Pending     [2]Input
}

func (r *CombatRunner) Advance(c *Combat, dt float64, in [2]Input) {
	if dt <= 0 || math.IsNaN(dt) || math.IsInf(dt, 0) {
		return
	}
	for i := range in {
		in[i].MergeEdges(r.Pending[i])
	}
	r.Pending = in
	r.Accumulator += math.Min(dt, .25)
	for r.Accumulator+1e-12 >= CombatDT {
		c.Step(r.Pending)
		for i := range r.Pending {
			r.Pending[i].ClearEdges()
		}
		r.Accumulator -= CombatDT
	}
}
