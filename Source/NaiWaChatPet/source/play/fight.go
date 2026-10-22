package play

import (
	"math"
	"math/rand"
)

// Weapon is a drop the pet keeps and fights with. Its numbers multiply the
// bare-handed palm.
type Weapon struct {
	ID, Name             string
	Damage, Reach, Knock float64
	// Weight is how often it drops relative to the others.
	Weight int
}

// Weapons lists every drop, commonest first.
var Weapons = []Weapon{
	{"stick", "木棍", 1.15, 1.35, 1.1, 40},
	{"lotus", "荷叶扇", 1.10, 1.20, 2.0, 26},
	{"gloves", "蜂蜜拳套", 1.40, 1.00, 1.25, 20},
	{"pan", "平底锅", 1.65, 1.05, 1.5, 11},
	{"swatter", "金苍蝇拍", 1.95, 1.45, 1.7, 3},
}

// Bare is the empty-handed pet.
var Bare = Weapon{ID: "", Name: "空手", Damage: 1, Reach: 1, Knock: 1}

func FindWeapon(id string) Weapon {
	for _, w := range Weapons {
		if w.ID == id {
			return w
		}
	}
	return Bare
}

// RollDrop decides what a beaten rival leaves behind. Later rounds drop more
// often and favour the rarer weapons; the boss always drops something.
func RollDrop(rng *rand.Rand, round int, boss bool) (Weapon, bool) {
	chance := math.Min(.85, .55+.06*float64(round-1))
	if !boss && rng.Float64() >= chance {
		return Weapon{}, false
	}
	total := 0.0
	weights := make([]float64, len(Weapons))
	for i, w := range Weapons {
		weights[i] = float64(w.Weight) * (1 + .25*float64(i)*float64(round-1))
		if boss && i < 2 {
			weights[i] = 0
		}
		total += weights[i]
	}
	pick := rng.Float64() * total
	for i, w := range weights {
		if pick < w {
			return Weapons[i], true
		}
		pick -= w
	}
	return Weapons[len(Weapons)-1], true
}

// Rival is an opponent: a recoloured pet with its own temper.
type Rival struct {
	ID, Name, Tag string
	HP            float64
	// Speed scales its steps, Aggression how readily it attacks and Agility
	// how often it jumps and sidesteps.
	Speed, Aggression, Agility float64
	// Palette maps the pet's shading, darkest to lightest, to new colours.
	Palette [3][3]float64
	Boss    bool
}

var Rivals = []Rival{
	{"shadow", "暗影奶蛙", "SHADOW", 48, .85, .45, .15, [3][3]float64{{.08, .04, .14}, {.36, .18, .55}, {.80, .64, .98}}, false},
	{"flame", "火焰奶蛙", "FLAME", 64, .95, .60, .20, [3][3]float64{{.22, .03, .02}, {.86, .25, .06}, {1, .84, .40}}, false},
	{"frost", "寒冰奶蛙", "FROST", 80, 1.0, .65, .35, [3][3]float64{{.02, .10, .22}, {.22, .58, .86}, {.86, .97, 1}}, false},
	{"gold", "黄金奶蛙", "GOLD", 112, 1.1, .80, .25, [3][3]float64{{.24, .14, .02}, {.86, .64, .12}, {1, .97, .80}}, true},
}

// RivalFor is the opponent of a round. After the boss the order repeats,
// tougher each time around.
func RivalFor(round int) Rival {
	round = max(1, round)
	r := Rivals[(round-1)%len(Rivals)]
	lap := float64((round - 1) / len(Rivals))
	r.HP *= 1 + .3*lap
	r.Speed = math.Min(1.4, r.Speed*(1+.08*lap))
	r.Aggression = math.Min(.95, r.Aggression+.08*lap)
	return r
}

// Damage is what a hit does before the target's own toughness.
const (
	JabDamage   = 7.0
	HeavyDamage = 15.0
	// Rivals hit a little softer than the pet: it is the hero.
	RivalJab   = 5.0
	RivalHeavy = 11.0
	PetHP      = 80.0
	RoundTime  = 120.0
)

// Reaches reports whether a palm at (px, py) lands on a fighter standing with
// its feet at (fx, fy) and lifted by height, everything in screen pixels.
// reach is the slack around the body the strike still connects with.
func Reaches(px, py, fx, fy, height, unit, reach float64) bool {
	left, right := fx-125*unit, fx+125*unit
	top, bottom := fy-height-480*unit, fy-height
	dx := math.Max(0, math.Max(left-px, px-right))
	dy := math.Max(0, math.Max(top-py, py-bottom))
	return math.Hypot(dx, dy) <= reach
}

// SameLane reports whether two fighters stand close enough in depth for one
// to hit the other: the desktop is a floor, not a single line.
func SameLane(ay, by, unit float64) bool { return math.Abs(ay-by) <= 80*unit }

// Sense is what a rival's brain knows about the fight.
type Sense struct {
	X, Y, FoeX, FoeY float64
	Facing           int
	FoeHeight        float64
	FoeStriking      bool
	Busy             bool
	Unit             float64
}

// Distances in canvas pixels between two fighters' feet.
const (
	strikeRange = 330
	tooClose    = 190
)

// Brain steers a rival: close in, line up, strike, and now and then hop or
// back off.
type Brain struct {
	rng                     *rand.Rand
	rival                   Rival
	cool, think, dodge, hop float64
	strafe, chain           float64
}

func NewBrain(seed int64, rival Rival) *Brain {
	return &Brain{rng: rand.New(rand.NewSource(seed)), rival: rival, cool: 1.2}
}

// Think returns the rival's keys for this step.
func (b *Brain) Think(dt float64, s Sense) Input {
	b.cool -= dt
	b.dodge -= dt
	b.think -= dt
	var in Input
	// A jump is held for a moment so it is a full hop, not a twitch.
	if b.hop > 0 {
		b.hop -= dt
		in.Jump = true
	}
	jump := func() {
		in.Jump, in.JumpPressed = true, true
		b.hop, b.dodge = .35, 1.4
	}
	if b.chain > 0 {
		// Follow a strike with another while the chain window is open.
		if b.chain -= dt; b.chain <= 0 {
			in.AttackPressed = true
		}
	}
	dx, dy := s.FoeX-s.X, s.FoeY-s.Y
	// A palm lands about 300 ahead of the feet once the lunge is counted, so
	// fighters trade blows across a gap rather than standing inside each other.
	near := math.Abs(dx) < strikeRange*s.Unit && SameLane(s.Y, s.FoeY, s.Unit)
	toward := sign(dx)
	if math.Abs(dx) < tooClose*s.Unit && SameLane(s.Y, s.FoeY, s.Unit) && !s.Busy && b.cool > .2 {
		// Step back out of a clinch while waiting to strike again.
		in.Left, in.Right = toward > 0, toward < 0
		return in
	}
	switch {
	case s.Busy:
	case s.FoeStriking && near && b.dodge <= 0 && b.rng.Float64() < b.rival.Agility*3*dt:
		// Read the incoming strike and hop out of it.
		jump()
	case near && s.Facing != toward:
		in.Left, in.Right = toward < 0, toward > 0
	case near && b.cool <= 0:
		in.AttackPressed = true
		b.cool = (1.5 - b.rival.Aggression) / b.rival.Speed * (.7 + .6*b.rng.Float64())
		if b.rng.Float64() < b.rival.Aggression*.7 {
			b.chain = .30
		}
	case near:
		// Waiting out the cooldown: shuffle in and out of range.
		if b.think <= 0 {
			b.strafe = b.rng.Float64()*2 - 1
			b.think = .4 + .5*b.rng.Float64()
		}
		if b.strafe < -.5 {
			in.Left, in.Right = toward > 0, toward < 0
		}
	default:
		in.Left, in.Right = dx < 0, dx > 0
		in.Up, in.Down = dy < -30*s.Unit, dy > 30*s.Unit
		in.Run = math.Abs(dx) > 600*s.Unit
		if b.dodge <= 0 && b.rng.Float64() < b.rival.Agility*.6*dt {
			jump()
		}
	}
	return in
}

// Phase is where a round stands.
type Phase int

const (
	Ready Phase = iota
	Fighting
	KnockedOut
	Over
)

// ArenaEvent tells the host what to announce.
type ArenaEvent int

const (
	AnnounceRound ArenaEvent = iota
	AnnounceFight
	AnnounceKO
	AnnounceTimeUp
	PetWon
	PetLost
)

// Arena keeps the score of a run of rounds.
type Arena struct {
	Round        int
	Rival        Rival
	PetHP, FoeHP float64
	// Shown health trails the real value, the way fighting games drain bars.
	PetShown, FoeShown float64
	Clock              float64
	Phase              Phase
	Since              float64
	// Combo counts consecutive hits by one side; it lapses after a pause.
	Combo, FoeCombo int
	Beaten          int
	comboLeft       float64
	foeComboLeft    float64
	petWon          bool
	events          []ArenaEvent
}

const (
	readyTime = 1.6
	koTime    = 2.6
	comboGap  = 1.1
)

func NewArena() *Arena {
	a := &Arena{PetHP: PetHP, PetShown: PetHP}
	a.next()
	return a
}

func (a *Arena) next() {
	a.Round++
	a.Rival = RivalFor(a.Round)
	a.FoeHP, a.FoeShown = a.Rival.HP, a.Rival.HP
	a.Clock, a.Phase, a.Since = RoundTime, Ready, 0
	a.Combo, a.FoeCombo = 0, 0
	a.events = append(a.events, AnnounceRound)
}

// Step runs the round clock. During a knockout the host is expected to slow
// time down; the arena only counts it.
func (a *Arena) Step(dt float64) {
	if dt <= 0 || a.Phase == Over {
		return
	}
	a.Since += dt
	drain := func(shown *float64, real float64) {
		if *shown > real {
			*shown = math.Max(real, *shown-dt*40)
		} else {
			*shown = real
		}
	}
	drain(&a.PetShown, a.PetHP)
	drain(&a.FoeShown, a.FoeHP)
	if a.comboLeft -= dt; a.comboLeft <= 0 {
		a.Combo = 0
	}
	if a.foeComboLeft -= dt; a.foeComboLeft <= 0 {
		a.FoeCombo = 0
	}
	switch a.Phase {
	case Ready:
		if a.Since >= readyTime {
			a.Phase, a.Since = Fighting, 0
			a.events = append(a.events, AnnounceFight)
		}
	case Fighting:
		if a.Clock -= dt; a.Clock <= 0 {
			a.Clock = 0
			a.events = append(a.events, AnnounceTimeUp)
			a.finish(a.PetHP/PetHP >= a.FoeHP/a.Rival.HP)
		}
	case KnockedOut:
		if a.Since >= koTime {
			if a.petWon {
				a.Beaten++
				a.events = append(a.events, PetWon)
				// Catch a breath between rounds.
				a.PetHP = math.Min(PetHP, a.PetHP+30)
				a.next()
			} else {
				a.Phase = Over
				a.events = append(a.events, PetLost)
			}
		}
	}
}

func (a *Arena) finish(petWon bool) {
	a.Phase, a.Since, a.petWon = KnockedOut, 0, petWon
}

// Live reports whether hits count right now.
func (a *Arena) Live() bool { return a.Phase == Fighting }

// HitFoe applies the pet's hit and reports a knockout.
func (a *Arena) HitFoe(damage float64) bool {
	if !a.Live() {
		return false
	}
	a.FoeHP = math.Max(0, a.FoeHP-damage)
	a.Combo++
	a.comboLeft = comboGap
	if a.FoeHP == 0 {
		a.events = append(a.events, AnnounceKO)
		a.finish(true)
		return true
	}
	return false
}

// HitPet applies the rival's hit and reports a knockout.
func (a *Arena) HitPet(damage float64) bool {
	if !a.Live() {
		return false
	}
	a.PetHP = math.Max(0, a.PetHP-damage)
	a.FoeCombo++
	a.foeComboLeft = comboGap
	if a.PetHP == 0 {
		a.events = append(a.events, AnnounceKO)
		a.finish(false)
		return true
	}
	return false
}

// Won reports, during a knockout, whether it was the pet's.
func (a *Arena) Won() bool { return a.petWon }

func (a *Arena) Events() []ArenaEvent {
	out := a.events
	a.events = nil
	return out
}
