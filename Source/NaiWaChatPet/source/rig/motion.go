package rig

import "math"

// Gait drives the Stand rig. The zero value is the rest pose.
type Gait struct {
	Stride float64 // 0 standing, 1 walking, 2 running
	Phase  float64 // turns; one turn is a left and a right step
	Squash float64 // above 0 shorter and wider, below 0 taller and thinner
	Lean   float64 // radians towards the facing side
	Tuck   float64 // 0..1 legs gathered under the body in the air
	Reach  float64 // 0..1 toes pointed at the ground before landing
	Topple float64 // 0..1 slumped over backwards, knocked out
}

// StandPose turns a gait into joint angles for the Stand rig.
func StandPose(g Gait) Pose {
	pose := make(Pose, standBones)
	walk, run := clamp01(g.Stride), clamp01(g.Stride-1)
	sin, cos := math.Sincos(2 * math.Pi * g.Phase)
	swing := .30*walk + .14*run
	lean := g.Lean + .03*walk + .05*run

	// The body rides highest when the legs pass each other and rocks over the
	// supporting foot, which is most of what makes a round body read as walking.
	bob := (2.5 + 3*run) * walk * math.Cos(4*math.Pi*g.Phase)
	pose[standPelvis] = Joint{Rot: -lean + .03*walk*sin, TY: -bob}
	pose[standChest].Rot = -.012 * walk * sin
	pose[standHead].Rot = lean*.5 - .01*walk*sin
	// A knocked-out pet tips back on its heels and sinks; the canvas has no
	// room to lay it flat, and slumping reads just as clearly.
	topple := clamp01(g.Topple)
	squash := g.Squash + .12*topple
	pose[standRoot] = Joint{Stretch: -squash, Bulge: .6 * squash, Rot: .42 * topple, TY: 14 * topple}

	leg := func(thigh, shin, foot int, sin, cos, tuck float64) {
		// lift is the half of the cycle in which this leg swings forward.
		lift := math.Max(0, cos) * walk
		// The legs are short and the mesh folds if a knee bends far, so a tuck
		// mostly draws the leg up and only eases the knee.
		pose[thigh] = Joint{
			// Cancel the body's lean so the legs stay under it.
			Rot: lean + swing*sin + .10*lift + .40*tuck*g.Tuck,
			// Keep the feet on the ground while the pelvis bobs.
			Stretch: -bob/95 - .10*lift - .22*tuck*g.Tuck + .06*g.Reach,
		}
		pose[shin] = Joint{Rot: -(.42+.18*run)*lift - .30*tuck*g.Tuck, Stretch: -.15 * tuck * g.Tuck}
		pose[foot].Rot = .30*lift - .18*math.Max(0, -sin)*walk + .30*g.Tuck - .35*g.Reach
	}
	leg(standNearThigh, standNearShin, standNearFoot, sin, cos, 1)
	leg(standFarThigh, standFarShin, standFarFoot, -sin, -cos, .7)
	return pose
}

// StrikeImpact is the point in a strike at which the palm lands.
const StrikeImpact = .40

// StrikePose poses the Stance rig for a palm strike. Progress runs 0..1 and
// power deepens the wind-up; 1 is a jab.
//
// The stance already reaches to within a hand's width of the canvas edge, so
// the palm travels only a little past rest here. A strike's real reach comes
// from the host moving the pet forward as the palm drives out.
func StrikePose(progress, power float64) Pose {
	pose := make(Pose, stanceBones)
	t := clamp01(progress)
	const windup, overshoot = .28, .06
	draw := math.Min(.56, .36+.12*(power-1))
	var extend, coil float64
	switch {
	case t < windup:
		coil = ramp(t, 0, windup)
		extend = -draw * coil
	case t < StrikeImpact:
		drive := easeOut((t - windup) / (StrikeImpact - windup))
		coil = 1 - drive
		extend = -draw + (draw+overshoot)*drive
	default:
		extend = overshoot * (1 - ramp(t, StrikeImpact, .85))
	}
	// follow is the body's commitment after the hit, fading as it recovers.
	follow := 0.0
	if t >= windup {
		follow = easeOut((t-windup)/(StrikeImpact-windup)) * (1 - ramp(t, .5, 1))
	}
	pose[stanceArm] = Joint{Stretch: extend, Bulge: -.35 * extend, Rot: .08*coil - .02*follow}
	pose[stanceHand].Rot = .25*coil - .08*follow
	pose[stanceTorso] = Joint{Rot: .05*power*coil - .035*follow, TX: 6*power*coil - 5*follow}
	pose[stanceHead].Rot = -.5 * pose[stanceTorso].Rot
	pose[stanceRoot] = Joint{Stretch: -.03*coil + .02*follow, Bulge: .02 * coil}
	return pose
}

// Settle squashes a Stance or Figure rig about the ground, for dropping into
// and rising out of a pose.
func Settle(p *Puppet, squash float64) Pose {
	pose := p.Rest()
	pose[0] = Joint{Stretch: -squash, Bulge: .6 * squash}
	return pose
}

// Gestures name the small loops that keep a held pose alive.
var Gestures = []string{"breathe", "nod", "shake", "sway", "rock", "bounce", "tremble", "sigh", "bow", "tilt", "puff", "sob", "huff"}

// GesturePose animates the Figure rig. Seconds counts from the start of the
// hold; an unknown gesture breathes.
func GesturePose(gesture string, seconds float64) Pose {
	pose := make(Pose, figureBones)
	wave := func(hz float64) float64 { return math.Sin(2 * math.Pi * hz * seconds) }
	// opening plays a gesture's accent once, then lets the pose rest.
	opening := func(length float64) float64 { return 1 - ramp(seconds, length*.7, length) }
	breath := .012 * wave(.3)
	root, spine, head := &pose[figureRoot], &pose[figureSpine], &pose[figureHead]
	root.Stretch, root.Bulge = breath, -.6*breath
	switch gesture {
	case "nod":
		head.Rot = -.15 * math.Max(0, wave(1.7)) * opening(1.5)
	case "shake":
		head.TX = 7 * wave(3) * opening(1.4)
		head.Rot = .04 * wave(3) * opening(1.4)
	case "sway":
		root.Rot = .05 * wave(.55)
		head.Rot = .06 * math.Sin(2*math.Pi*.55*seconds-1)
	case "rock":
		root.Rot = .022 * wave(1.6)
		head.Rot = -.02 * wave(1.6)
	case "bounce":
		hop := math.Abs(wave(1.5))
		root.Stretch, root.Bulge = .05*hop-.02, -.03*hop+.012
		root.TY = -10 * hop
	case "tremble":
		root.TX = 2.2 * wave(13)
		head.Rot = .012 * wave(11)
	case "sigh":
		sink := .5 - .5*math.Cos(2*math.Pi*.22*seconds)
		root.Stretch, root.Bulge = -.035*sink, .02*sink
		head.Rot = -.05 * sink
	case "bow":
		dip := ramp(seconds, 0, .35) * (1 - ramp(seconds, 1.1, 1.6))
		spine.Rot = -.13 * dip
		head.Rot = -.10 * dip
	case "tilt":
		head.Rot = .07 * wave(.4)
	case "puff":
		swell := .5 + .5*wave(.45)
		spine.Stretch, spine.Bulge = .03*swell, .02*swell
		head.Rot = .05 * swell
	case "sob":
		head.Rot = -.06 + .03*wave(3.2)
		root.Stretch = breath - .012*math.Abs(wave(3.2))
	case "huff":
		pulse := math.Max(0, wave(1.1))
		root.Stretch, root.Bulge = -.03*pulse, .025*pulse
	}
	return pose
}

// Add layers one pose over another, joint by joint.
func (p Pose) Add(q Pose) Pose {
	out := make(Pose, max(len(p), len(q)))
	copy(out, p)
	for i, j := range q {
		out[i].Rot += j.Rot
		out[i].Stretch += j.Stretch
		out[i].Bulge += j.Bulge
		out[i].TX += j.TX
		out[i].TY += j.TY
	}
	return out
}

func easeOut(t float64) float64 {
	t = 1 - clamp01(t)
	return 1 - t*t*t
}
