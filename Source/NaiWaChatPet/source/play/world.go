package play

import (
	"math"
	"math/rand"
)

// Area is a rectangle on the screen, in pixels.
type Area struct{ X, Y, W, H float64 }

func (a Area) contains(x, y, margin float64) bool {
	return x >= a.X-margin && x <= a.X+a.W+margin && y >= a.Y-margin && y <= a.Y+a.H+margin
}

// Fly is one target of the fly-catching game.
type Fly struct {
	ID   int
	X, Y float64
	// Wing is the wing-beat phase in turns; Heading is +1 flying right, -1 left.
	Wing    float64
	Heading int

	vx, vy       float64
	orbit, drift float64
	side, height float64
	life         float64
}

// Swarm keeps a few flies hovering within reach of the pet. Some hang low
// enough for a standing strike; the rest need a jump.
type Swarm struct {
	rng    *rand.Rand
	flies  []Fly
	wait   float64
	nextID int

	// Score counts flies caught this session. Streak counts catches made in
	// quick succession and resets when the pet goes too long without one.
	Score, Streak, BestStreak int
	streakLeft                float64
}

const (
	maxFlies   = 3
	streakTime = 3.0
)

func NewSwarm(seed int64) *Swarm {
	return &Swarm{rng: rand.New(rand.NewSource(seed)), wait: 1.2}
}

func (s *Swarm) Flies() []Fly { return s.flies }

// Reset removes every fly and clears the score.
func (s *Swarm) Reset() {
	s.flies, s.Score, s.Streak, s.BestStreak, s.streakLeft, s.wait = nil, 0, 0, 0, 0, 1.2
}

// Step moves the flies. The lure is the pet's feet; unit converts canvas
// pixels to the screen so the game scales with the pet.
func (s *Swarm) Step(dt float64, area Area, lureX, lureY, unit float64) {
	if dt <= 0 || unit <= 0 || area.W <= 0 || area.H <= 0 {
		return
	}
	dt = math.Min(dt, .05)
	if s.streakLeft > 0 {
		if s.streakLeft -= dt; s.streakLeft <= 0 {
			s.Streak = 0
		}
	}
	if s.wait -= dt; s.wait <= 0 && len(s.flies) < maxFlies {
		s.spawn(area, unit)
		s.wait = 2.5 + 4*s.rng.Float64()
	}
	kept := s.flies[:0]
	for _, f := range s.flies {
		f.life -= dt
		f.Wing += dt * 22
		f.Wing -= math.Floor(f.Wing)
		f.orbit += dt * f.drift
		// Aim for a point beside and above the pet, or straight up once the
		// fly has had enough.
		tx := lureX + f.side*unit*math.Cos(f.orbit)
		ty := lureY - f.height*unit + 46*unit*math.Sin(f.orbit*1.7)
		if f.life <= 0 {
			tx, ty = f.X, area.Y-200*unit
		}
		tx = math.Max(area.X+20*unit, math.Min(area.X+area.W-20*unit, tx))
		if f.life > 0 {
			ty = math.Max(area.Y+20*unit, ty)
		}
		ax := (tx-f.X)*5 + (s.rng.Float64()-.5)*900*unit
		ay := (ty-f.Y)*5 + (s.rng.Float64()-.5)*900*unit
		f.vx += (ax - 2.4*f.vx) * dt
		f.vy += (ay - 2.4*f.vy) * dt
		if top := 520 * unit; math.Hypot(f.vx, f.vy) > top {
			scale := top / math.Hypot(f.vx, f.vy)
			f.vx, f.vy = f.vx*scale, f.vy*scale
		}
		f.X += f.vx * dt
		f.Y += f.vy * dt
		if math.Abs(f.vx) > 12*unit {
			f.Heading = sign(f.vx)
		}
		if f.life > 0 || area.contains(f.X, f.Y, 60*unit) {
			kept = append(kept, f)
		}
	}
	s.flies = kept
}

func (s *Swarm) spawn(area Area, unit float64) {
	s.nextID++
	f := Fly{ID: s.nextID, Heading: 1, life: 16 + 12*s.rng.Float64()}
	// Enter from the left, right or top edge.
	switch s.rng.Intn(3) {
	case 0:
		f.X, f.Y = area.X-30*unit, area.Y+area.H*(.1+.5*s.rng.Float64())
	case 1:
		f.X, f.Y = area.X+area.W+30*unit, area.Y+area.H*(.1+.5*s.rng.Float64())
	default:
		f.X, f.Y = area.X+area.W*s.rng.Float64(), area.Y-30*unit
	}
	f.side = 150 + 330*s.rng.Float64()
	if s.rng.Intn(2) == 0 {
		f.side = -f.side
	}
	// The palm sits about 350 above the feet; anything over 480 takes a jump.
	f.height = 280 + 560*s.rng.Float64()
	f.orbit = 2 * math.Pi * s.rng.Float64()
	f.drift = (.5 + .7*s.rng.Float64()) * float64(sign(s.rng.Float64()-.5))
	s.flies = append(s.flies, f)
}

// Catch removes the flies within radius of a point and returns how many
// there were.
func (s *Swarm) Catch(x, y, radius float64) int {
	kept, caught := s.flies[:0], 0
	for _, f := range s.flies {
		if math.Hypot(f.X-x, f.Y-y) <= radius {
			caught++
			continue
		}
		kept = append(kept, f)
	}
	s.flies = kept
	if caught > 0 {
		s.Score += caught
		s.Streak += caught
		s.BestStreak = max(s.BestStreak, s.Streak)
		s.streakLeft = streakTime
	}
	return caught
}

// Stroll decides when and where an unattended pet wanders.
type Stroll struct {
	rng  *rand.Rand
	rest float64
}

func NewStroll(seed int64) *Stroll {
	s := &Stroll{rng: rand.New(rand.NewSource(seed))}
	s.Rest()
	return s
}

// Rest starts a pause before the next walk.
func (s *Stroll) Rest() { s.rest = 7 + 14*s.rng.Float64() }

// Due counts the pause down and reports when it is over.
func (s *Stroll) Due(dt float64) bool {
	s.rest -= dt
	return s.rest <= 0
}

// Pick chooses a destination near the pet that keeps it inside the area.
func (s *Stroll) Pick(area Area, x, y, unit float64) (float64, float64) {
	dx := (160 + 520*s.rng.Float64()) * unit * float64(sign(s.rng.Float64()-.5))
	dy := (s.rng.Float64() - .5) * 220 * unit
	// Turn around rather than walk into a wall.
	if x+dx < area.X || x+dx > area.X+area.W {
		dx = -dx
	}
	tx := math.Max(area.X, math.Min(area.X+area.W, x+dx))
	ty := math.Max(area.Y, math.Min(area.Y+area.H, y+dy))
	return tx, ty
}
