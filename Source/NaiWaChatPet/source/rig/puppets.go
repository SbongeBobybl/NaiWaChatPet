package rig

// The three rigs below are bound to the pet's 420x560 body canvas. Joint and
// mask coordinates were measured on idle_000.png and fight_012.png.

const (
	CanvasWidth  = 420
	CanvasHeight = 560
	// GroundY is the line the pet stands on; squash and lean pivot here.
	GroundY = 540
)

const (
	standRoot = iota
	standPelvis
	standChest
	standHead
	standFarThigh
	standFarShin
	standFarFoot
	standNearThigh
	standNearShin
	standNearFoot
	standBones
)

// Stand rigs the idle pose for walking, running and jumping. Each leg is its
// own layer drawn beneath the body, so the legs can cross and the hips stay
// covered by the belly however far a thigh swings.
func Stand() (*Puppet, error) {
	bones := []Bone{
		standRoot:      {"root", -1, Vec{240, GroundY}, Vec{240, 400}},
		standPelvis:    {"pelvis", standRoot, Vec{250, 400}, Vec{250, 300}},
		standChest:     {"chest", standPelvis, Vec{250, 300}, Vec{232, 175}},
		standHead:      {"head", standChest, Vec{230, 175}, Vec{215, 50}},
		standFarThigh:  {"farThigh", standPelvis, Vec{205, 395}, Vec{212, 440}},
		standFarShin:   {"farShin", standFarThigh, Vec{212, 440}, Vec{216, 478}},
		standFarFoot:   {"farFoot", standFarShin, Vec{216, 480}, Vec{150, 505}},
		standNearThigh: {"nearThigh", standPelvis, Vec{290, 395}, Vec{300, 445}},
		standNearShin:  {"nearShin", standNearThigh, Vec{300, 445}, Vec{305, 492}},
		standNearFoot:  {"nearFoot", standNearShin, Vec{305, 494}, Vec{258, 528}},
	}
	body := func(p Vec) []float64 {
		w := make([]float64, standBones)
		head := ramp(p.Y, 215, 135)
		chest := ramp(p.Y, 335, 250) * (1 - head)
		w[standHead], w[standChest], w[standPelvis] = head, chest, 1-head-chest
		return w
	}
	// A leg follows the body above its hip and takes over gradually below it,
	// which keeps the seam under the belly closed.
	leg := func(thigh, shin, foot int, hip, knee, ankle float64) func(Vec) []float64 {
		return func(p Vec) []float64 {
			w := body(p)
			own := ramp(p.Y, hip-3, hip+37)
			f := ramp(p.Y, ankle-10, ankle+6)
			s := ramp(p.Y, knee-22, knee+22) * (1 - f)
			for i := range w {
				w[i] *= 1 - own
			}
			w[thigh] += own * (1 - s - f)
			w[shin] += own * s
			w[foot] += own * f
			return w
		}
	}
	// belly is the lower rim of the body by x; split is the gap between the
	// legs by y. The split is a hard partition so touching feet stay opaque.
	belly := [][2]float64{{150, 372}, {165, 381}, {185, 392}, {205, 399}, {245, 405}, {275, 402}, {300, 396}, {330, 388}, {345, 380}}
	split := [][2]float64{{380, 246}, {410, 247}, {430, 250}, {455, 258}, {475, 264}, {490, 262}, {500, 250}, {506, 246}, {560, 246}}
	below := func(x, y, from, to float64) float64 { return ramp(y-along(belly, x), from, to) }
	return NewPuppet(CanvasWidth, CanvasHeight, bones, []LayerSpec{
		{"farLeg", func(x, y float64) float64 {
			if x > along(split, y) {
				return 0
			}
			return below(x, y, -34, -22)
		}, leg(standFarThigh, standFarShin, standFarFoot, 395, 440, 478)},
		{"nearLeg", func(x, y float64) float64 {
			if x <= along(split, y) {
				return 0
			}
			return below(x, y, -34, -22)
		}, leg(standNearThigh, standNearShin, standNearFoot, 395, 445, 492)},
		{"body", func(x, y float64) float64 { return 1 - below(x, y, -4, 8) }, body},
	})
}

const (
	stanceRoot = iota
	stanceTorso
	stanceHead
	stanceArm
	stanceHand
	stanceBones
)

// Stance rigs the fighting pose. Its feet stay planted; the outstretched arm
// is a layer of its own so a strike can draw it back and drive it forward.
func Stance() (*Puppet, error) {
	bones := []Bone{
		stanceRoot:  {"root", -1, Vec{230, GroundY}, Vec{230, 420}},
		stanceTorso: {"torso", stanceRoot, Vec{250, 420}, Vec{250, 250}},
		stanceHead:  {"head", stanceTorso, Vec{236, 215}, Vec{240, 105}},
		stanceArm:   {"arm", stanceTorso, Vec{172, 242}, Vec{50, 190}},
		stanceHand:  {"hand", stanceArm, Vec{80, 208}, Vec{45, 184}},
	}
	body := func(p Vec) []float64 {
		w := make([]float64, stanceBones)
		head := ramp(p.Y, 240, 190)
		torso := ramp(p.Y, 480, 400) * (1 - head)
		w[stanceHead], w[stanceTorso], w[stanceRoot] = head, torso, 1-head-torso
		return w
	}
	shoulder, palm := bones[stanceArm].Pivot, bones[stanceArm].Tip
	// reach runs from 0 at the shoulder to 1 at the palm.
	reach := func(p Vec) float64 {
		dx, dy := palm.X-shoulder.X, palm.Y-shoulder.Y
		return ((p.X-shoulder.X)*dx + (p.Y-shoulder.Y)*dy) / (dx*dx + dy*dy)
	}
	arm := func(p Vec) []float64 {
		w := body(p)
		own, hand := ramp(reach(p), 0, .42), ramp(reach(p), .62, .85)
		for i := range w {
			w[i] *= 1 - own
		}
		w[stanceArm] += own * (1 - hand)
		w[stanceHand] += own * hand
		return w
	}
	// lower follows the underside of the arm into the armpit. The body fades
	// out across that line and the arm layer runs on beneath the fade, so the
	// boundary never shows as an edge when the arm moves.
	lower := [][2]float64{{0, 262}, {60, 262}, {100, 272}, {135, 284}, {150, 281}, {165, 270}, {190, 262}, {240, 262}}
	return NewPuppet(CanvasWidth, CanvasHeight, bones, []LayerSpec{
		{"arm", func(x, y float64) float64 {
			if y <= 140 || y >= along(lower, x)+14 {
				return 0
			}
			return ramp(x, 236, 214)
		}, arm},
		{"body", func(x, y float64) float64 {
			if y <= 140 {
				return 1
			}
			return 1 - ramp(x, 186, 160)*ramp(y-along(lower, x), 6, -6)
		}, body},
	})
}

const (
	figureRoot = iota
	figureSpine
	figureHead
	figureBones
)

// Figure is a spine-only rig for any upright frame: it can squash, lean, bow
// and nod a pose without knowing where its limbs are.
func Figure() (*Puppet, error) {
	bones := []Bone{
		figureRoot:  {"root", -1, Vec{240, GroundY}, Vec{240, 420}},
		figureSpine: {"spine", figureRoot, Vec{240, 410}, Vec{240, 260}},
		figureHead:  {"head", figureSpine, Vec{232, 195}, Vec{226, 60}},
	}
	return NewPuppet(CanvasWidth, CanvasHeight, bones, []LayerSpec{{"figure", nil, func(p Vec) []float64 {
		head := ramp(p.Y, 235, 165)
		spine := ramp(p.Y, 470, 390) * (1 - head)
		return []float64{figureRoot: 1 - head - spine, figureSpine: spine, figureHead: head}
	}}})
}

// PalmRest is where the stance's open palm sits before a strike.
var PalmRest = Vec{50, 192}

// Palm reports where the palm is for a stance pose, for hit tests and effects.
func Palm(p *Puppet, pose Pose) Vec { return p.Point(pose, stanceHand, PalmRest) }

// Arm reports where a stance pose puts the shoulder and the palm, so a held
// weapon can follow the arm through a strike.
func Arm(p *Puppet, pose Pose) (shoulder, palm Vec) {
	return p.Point(pose, stanceArm, p.bones[stanceArm].Pivot), Palm(p, pose)
}

// CarryRest is where a box held against the belly sits on the idle pose.
var CarryRest = Vec{200, 300}

// Carry reports where a Stand pose holds that box, so it rides with the body.
func Carry(p *Puppet, pose Pose) Vec { return p.Point(pose, standChest, CarryRest) }
