// Package play turns key state into the pet's motion on the desktop: walking
// and running on a plane, jumping above it, and a three-hit palm combo. It owns
// no window and no clock, so the host feeds it time and draws what it reports.
package play

import (
	"math"

	"codexpet/reconstruction/rig"
)

// Speeds and distances are in body-canvas pixels, so the pet covers the same
// share of its own height at every size. Config.Unit converts to the screen.
const (
	walkSpeed  = 340.0
	runSpeed   = 760.0
	climb      = .8 // vertical travel relative to horizontal
	jumpSpeed  = 2300.0
	gravity    = 7400.0
	crouchTime = .07
	landTime   = .18
	dropTime   = .07 // standing into the stance
	guardTime  = .12 // stance held after the last strike
	riseTime   = .16 // stance back to standing
	jabTime    = .34
	heavyTime  = .50
	jabLunge   = 46.0
	heavyLunge = 104.0
	// stanceDrop is how far the standing pose squashes to meet the stance,
	// whose head sits that much lower.
	stanceDrop = .12
	chainFrom  = .62 // earliest point at which the next strike may start
	bufferTime = .28
)

// Input is the key state for one step. Pressed fields are true only on the
// step in which the key went down.
type Input struct {
	Left, Right, Up, Down, Run bool
	Jump, Attack               bool
	JumpPressed, AttackPressed bool
}

func (in Input) steering() bool { return in.Left || in.Right || in.Up || in.Down }

type Event int

const (
	Jumped Event = iota
	DoubleJumped
	Landed
	// Struck fires when the palm lands; StruckHard is the third hit of a combo.
	Struck
	StruckHard
	Arrived
)

// Config ties the controller to a screen. Free reports whether the pet may
// stand with its feet at a point; nil allows everything.
type Config struct {
	Unit float64
	Free func(x, y float64) bool
}

type action int

const (
	actNone action = iota
	actDrop
	actStrike
	actGuard
	actRise
)

// Controller holds the pet's feet position in screen pixels and its height
// above that point in canvas pixels.
type Controller struct {
	cfg          Config
	x, y, vx, vy float64
	z, vz        float64
	facing       int
	phase        float64
	stride       float64
	crouch, land float64
	airJumps     int
	act          action
	at           float64
	strikeTime   float64
	combo        int
	buffer       float64
	target       *goal
	events       []Event
	// stun ignores the keys after a blow, hurt fades the recoil it shows, and
	// down keeps a knocked-out fighter lying where it landed.
	stun, hurt float64
	// thrown marks a flight that began with a blow, not a jump.
	down, thrown bool
}

type goal struct {
	x, y         float64
	run          bool
	age, stalled float64
}

func New(cfg Config) *Controller {
	c := &Controller{facing: -1, airJumps: 1}
	c.Configure(cfg)
	return c
}

// Configure replaces the screen binding, for example after the pet is resized.
func (c *Controller) Configure(cfg Config) {
	if cfg.Unit <= 0 || math.IsNaN(cfg.Unit) || math.IsInf(cfg.Unit, 0) {
		cfg.Unit = 1
	}
	c.cfg = cfg
}

// Place moves the pet without animating, as when the user drags it. A facing
// of 0 keeps the current one.
func (c *Controller) Place(x, y float64, facing int) {
	c.x, c.y = x, y
	if facing != 0 {
		c.facing = sign(float64(facing))
	}
}

// Halt drops everything in progress and stands the pet where it is.
func (c *Controller) Halt() {
	*c = Controller{cfg: c.cfg, x: c.x, y: c.y, facing: c.facing, airJumps: 1}
}

// Goto walks or runs to a point unaided. Any steering input cancels it.
func (c *Controller) Goto(x, y float64, run bool) { c.target = &goal{x: x, y: y, run: run} }

func (c *Controller) Travelling() bool { return c.target != nil }

func (c *Controller) grounded() bool { return c.z == 0 && c.vz == 0 }

func (c *Controller) free(x, y float64) bool { return c.cfg.Free == nil || c.cfg.Free(x, y) }

// shift moves the feet, sliding along a screen edge instead of stopping dead.
func (c *Controller) shift(dx, dy float64) {
	switch {
	case c.free(c.x+dx, c.y+dy):
		c.x, c.y = c.x+dx, c.y+dy
	case dx != 0 && c.free(c.x+dx, c.y):
		c.x += dx
		c.vy = 0
	case dy != 0 && c.free(c.x, c.y+dy):
		c.y += dy
		c.vx = 0
	default:
		c.vx, c.vy = 0, 0
	}
}

// Step advances the pet by dt seconds.
func (c *Controller) Step(dt float64, in Input) {
	if math.IsNaN(dt) || dt <= 0 {
		return
	}
	// A stalled message loop must not launch the pet across the desktop.
	dt = math.Min(dt, .05)
	unit := c.cfg.Unit

	if c.target != nil {
		if in.steering() || in.JumpPressed || in.AttackPressed {
			c.target = nil
		} else {
			in = c.pilot(dt)
		}
	}

	if c.hurt > 0 {
		c.hurt = math.Max(0, c.hurt-dt/.4)
	}
	if c.stun > 0 || c.down {
		c.stun = math.Max(0, c.stun-dt)
		in = Input{}
	}
	grounded := c.grounded()
	dx, dy := axis(in.Left, in.Right), axis(in.Up, in.Down)
	if c.act != actNone && grounded {
		dx, dy = 0, 0
	}
	if dx != 0 && c.act == actNone {
		c.facing = sign(dx)
	}
	speed := walkSpeed
	if in.Run {
		speed = runSpeed
	}
	if c.crouch > 0 {
		speed *= .4
	}
	wantX, wantY := dx*speed, dy*speed*climb
	if dx != 0 && dy != 0 {
		wantX, wantY = wantX*math.Sqrt2/2, wantY*math.Sqrt2/2
	}
	settle := .07
	if !grounded {
		settle = .16
	}
	blend := 1 - math.Exp(-dt/settle)
	c.vx += (wantX - c.vx) * blend
	c.vy += (wantY - c.vy) * blend
	// Come to a real stop instead of creeping towards one forever.
	if wantX == 0 && wantY == 0 && math.Hypot(c.vx, c.vy) < 3 {
		c.vx, c.vy = 0, 0
	}
	c.shift(c.vx*dt*unit, c.vy*dt*unit)

	moving := math.Hypot(c.vx, c.vy/climb)
	if grounded {
		c.stride = math.Min(moving, walkSpeed)/walkSpeed + math.Max(0, moving-walkSpeed)/(runSpeed-walkSpeed)
		c.phase += moving * dt / (150 + 60*clamp01(c.stride-1))
		c.phase -= math.Floor(c.phase)
	} else {
		c.stride *= math.Exp(-dt / .08)
	}

	c.jump(dt, in)
	c.strike(dt, in)
	if c.land > 0 {
		c.land = math.Max(0, c.land-dt)
	}
}

// pilot steers towards the goal and reports arrival. A goal behind a screen
// edge counts as reached once the pet has stopped making progress.
func (c *Controller) pilot(dt float64) Input {
	g := c.target
	dx, dy := g.x-c.x, g.y-c.y
	reach := 10 * c.cfg.Unit
	if g.age += dt; g.age > .4 && math.Hypot(c.vx, c.vy) < 20 {
		g.stalled += dt
	} else {
		g.stalled = 0
	}
	if math.Hypot(dx, dy) <= reach || g.stalled > .5 {
		c.target = nil
		c.events = append(c.events, Arrived)
		return Input{}
	}
	// Walk the last stretch so the pet settles instead of overshooting.
	in := Input{Run: g.run && math.Hypot(dx, dy) > 140*c.cfg.Unit}
	in.Left, in.Right = dx < -reach*.6, dx > reach*.6
	in.Up, in.Down = dy < -reach*.6, dy > reach*.6
	return in
}

func (c *Controller) jump(dt float64, in Input) {
	if in.JumpPressed {
		switch {
		case c.grounded() && c.act == actNone && c.crouch == 0:
			c.crouch = crouchTime
		case !c.grounded() && c.airJumps > 0:
			c.airJumps--
			c.vz = jumpSpeed * .82
			c.events = append(c.events, DoubleJumped)
		}
	}
	if c.crouch > 0 {
		c.crouch -= dt
		if c.crouch <= 0 {
			c.crouch, c.vz, c.land = 0, jumpSpeed, 0
			c.events = append(c.events, Jumped)
		}
	}
	if c.grounded() {
		return
	}
	// Letting go early makes a short hop, unless a blow did the throwing.
	if !in.Jump && !c.thrown && c.vz > jumpSpeed*.45 {
		c.vz = jumpSpeed * .45
	}
	c.vz -= gravity * dt
	c.z += c.vz * dt
	if c.z <= 0 {
		c.z, c.vz, c.land, c.airJumps, c.thrown = 0, 0, landTime, 1, false
		c.events = append(c.events, Landed)
	}
}

func (c *Controller) strike(dt float64, in Input) {
	if in.AttackPressed {
		c.buffer = bufferTime
	} else if c.buffer > 0 {
		c.buffer = math.Max(0, c.buffer-dt)
	}
	begin := func() {
		c.combo = c.combo%3 + 1
		c.act, c.at, c.buffer = actStrike, 0, 0
		c.strikeTime = jabTime
		if c.combo == 3 {
			c.strikeTime = heavyTime
		}
	}
	switch c.act {
	case actNone:
		if c.buffer > 0 && c.crouch == 0 {
			c.act, c.at, c.combo = actDrop, 0, 0
			if c.grounded() {
				c.vx, c.vy = 0, 0
			}
		}
	case actDrop:
		if c.at += dt; c.at >= dropTime {
			begin()
		}
	case actStrike:
		before := c.at / c.strikeTime
		c.at += dt
		now := math.Min(1, c.at/c.strikeTime)
		reach := jabLunge
		if c.combo == 3 {
			reach = heavyLunge
		}
		if step := lunge(now) - lunge(before); step > 0 {
			c.shift(float64(c.facing)*step*reach*c.cfg.Unit, 0)
		}
		if before < rig.StrikeImpact && now >= rig.StrikeImpact {
			if c.combo == 3 {
				c.events = append(c.events, StruckHard)
			} else {
				c.events = append(c.events, Struck)
			}
		}
		// A buffered press chains early; holding the key chains at the end.
		switch {
		case c.combo < 3 && (c.buffer > 0 && now >= chainFrom || in.Attack && now >= .92):
			begin()
		case now >= 1:
			c.act, c.at = actGuard, 0
		}
	case actGuard:
		c.at += dt
		switch {
		case c.buffer > 0 || in.Attack && c.at >= guardTime:
			begin()
		case c.at >= guardTime:
			c.act, c.at, c.combo = actRise, 0, 0
		}
	case actRise:
		if c.at += dt; c.at >= riseTime {
			c.act, c.at = actNone, 0
		}
	}
}

// lunge is the share of a strike's forward travel covered by a given progress;
// the body commits as the palm drives out.
func lunge(progress float64) float64 {
	return smooth((progress - .26) / (rig.StrikeImpact + .06 - .26))
}

// View is everything the host needs to draw one frame.
type View struct {
	// Still means the pet is standing at rest and needs no deformation.
	Still bool
	// Stance selects the fighting rig; otherwise Gait drives the standing rig.
	Stance bool
	Gait   rig.Gait
	// Strike and Power pose the stance; Settle squashes it on landing.
	Strike, Power, Settle float64
	// FacingRight asks for the frame mirrored; the art faces left.
	FacingRight bool
	// Height is how far the feet are above the ground, in screen pixels.
	Height float64
}

func (c *Controller) Position() (x, y float64) { return c.x, c.y }

func (c *Controller) View() View {
	v := View{FacingRight: c.facing > 0, Height: c.z * c.cfg.Unit}
	landing := 0.0
	if c.land > 0 {
		// A squash that rebounds a little past rest before it settles.
		t := 1 - c.land/landTime
		landing = .15 * math.Exp(-5*t) * math.Cos(9*t)
	}
	switch c.act {
	case actStrike:
		v.Stance, v.Strike, v.Power, v.Settle = true, math.Min(1, c.at/c.strikeTime), 1, landing
		if c.combo == 3 {
			v.Power = 1.9
		}
		return v
	case actGuard:
		v.Stance, v.Settle = true, landing
		return v
	}
	g := rig.Gait{Stride: c.stride, Phase: c.phase, Squash: landing}
	g.Lean = .05*math.Abs(c.vx)/runSpeed - .28*c.hurt
	g.Squash += .05 * c.hurt
	if c.down {
		g.Topple = 1
	}
	switch {
	case c.act == actDrop:
		g.Squash += stanceDrop * smooth(c.at/dropTime)
	case c.act == actRise:
		t := c.at / riseTime
		g.Squash += stanceDrop * (1 - smooth(t*1.6)) * math.Cos(4*t)
	case c.crouch > 0:
		g.Squash += .13 * smooth(1-c.crouch/crouchTime)
	case !c.grounded():
		rise := c.vz / jumpSpeed
		g.Squash -= .07 * clamp01(rise)
		g.Tuck = clamp01(1 - math.Abs(rise)*1.3)
		g.Reach = clamp01(-rise - .35)
		g.Lean += .05 * float64(c.facing) * c.vx / runSpeed
	}
	v.Gait = g
	v.Still = c.act == actNone && c.grounded() && c.crouch == 0 && c.land == 0 && c.stride < .004 && c.hurt == 0 && !c.down
	return v
}

// Hit knocks the fighter back from a blow. dir is the way it is pushed, force
// its speed in canvas pixels a second, stun how long its keys are ignored, and
// lift, when above zero, throws it into the air. Whatever it was doing stops.
func (c *Controller) Hit(dir int, force, stun, lift float64) {
	c.act, c.at, c.combo, c.buffer, c.crouch, c.target = actNone, 0, 0, 0, 0, nil
	c.vx = float64(sign(float64(dir))) * force
	c.stun = math.Max(c.stun, stun)
	c.hurt = 1
	if lift > 0 {
		c.vz = math.Max(c.vz, lift)
		c.z = math.Max(c.z, .01)
		c.thrown = true
	}
}

// Down lays a knocked-out fighter on the ground, or gets it back up.
func (c *Controller) Down(on bool) {
	c.down = on
	if !on {
		c.stun, c.hurt = 0, 0
	}
}

// Busy reports whether the pet is doing anything other than standing.
func (c *Controller) Busy() bool { return !c.View().Still || c.target != nil }

// Events returns what happened since the last call.
func (c *Controller) Events() []Event {
	out := c.events
	c.events = nil
	return out
}

func axis(negative, positive bool) float64 {
	switch {
	case negative && !positive:
		return -1
	case positive && !negative:
		return 1
	}
	return 0
}

func sign(v float64) int {
	if v > 0 {
		return 1
	}
	return -1
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func smooth(t float64) float64 {
	t = clamp01(t)
	return t * t * (3 - 2*t)
}
