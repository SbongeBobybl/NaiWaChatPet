package play

import (
	"math"
	"testing"
)

const tick = 1.0 / 60

func run(c *Controller, seconds float64, in Input) {
	for t := 0.0; t < seconds; t += tick {
		c.Step(tick, in)
		in.JumpPressed, in.AttackPressed = false, false
	}
}

func count(events []Event, want Event) int {
	n := 0
	for _, e := range events {
		if e == want {
			n++
		}
	}
	return n
}

func TestWalkingMovesTheFeetAndTurnsThePet(t *testing.T) {
	c := New(Config{Unit: .5})
	c.Place(500, 500, 0)
	run(c, 1, Input{Right: true})
	x, y := c.Position()
	if want := 500 + walkSpeed*.5; math.Abs(x-want) > 25 || y != 500 {
		t.Fatalf("walked to (%.1f, %.1f), want about (%.1f, 500)", x, y, want)
	}
	if v := c.View(); !v.FacingRight || v.Still || v.Gait.Stride < .95 || v.Gait.Stride > 1.05 {
		t.Fatalf("walking view = %+v", v)
	}
	run(c, 1, Input{Left: true, Run: true})
	if v := c.View(); v.FacingRight || v.Gait.Stride < 1.9 {
		t.Fatalf("running left view = %+v", v)
	}
}

func TestDiagonalIsNoFasterThanStraight(t *testing.T) {
	c := New(Config{})
	run(c, 1, Input{Right: true, Down: true})
	x, y := c.Position()
	if speed := math.Hypot(x, y/climb); speed > walkSpeed*1.02 {
		t.Fatalf("diagonal covered %.1f in a second, straight covers %.1f", speed, walkSpeed)
	}
}

func TestStopsCompletely(t *testing.T) {
	c := New(Config{})
	run(c, .5, Input{Right: true})
	run(c, 1, Input{})
	if v := c.View(); !v.Still || c.Busy() {
		t.Fatalf("pet still moving a second after release: %+v", v)
	}
	before, _ := c.Position()
	run(c, 1, Input{})
	if after, _ := c.Position(); after != before {
		t.Fatalf("resting pet drifted from %v to %v", before, after)
	}
}

func TestScreenEdgeSlidesInsteadOfSticking(t *testing.T) {
	c := New(Config{Free: func(x, y float64) bool { return x <= 100 }})
	run(c, 1, Input{Right: true, Down: true})
	x, y := c.Position()
	if x > 100 {
		t.Fatalf("pet left the screen: x=%.1f", x)
	}
	if y < 100 {
		t.Fatalf("pet stuck on the edge instead of sliding: y=%.1f", y)
	}
}

func TestJumpRisesToItsApexAndLands(t *testing.T) {
	c := New(Config{Unit: 2})
	in := Input{Jump: true, JumpPressed: true}
	peak := 0.0
	var events []Event
	for step := 0; step < 120; step++ {
		c.Step(tick, in)
		in.JumpPressed = false
		peak = math.Max(peak, c.View().Height)
		events = append(events, c.Events()...)
	}
	want := jumpSpeed * jumpSpeed / (2 * gravity) * 2
	if math.Abs(peak-want) > want*.08 {
		t.Fatalf("apex %.1f, want about %.1f", peak, want)
	}
	if count(events, Jumped) != 1 || count(events, Landed) != 1 {
		t.Fatalf("events = %v", events)
	}
	if v := c.View(); v.Height != 0 || !v.Still {
		t.Fatalf("not standing after the jump: %+v", v)
	}
}

func TestReleasingEarlyMakesAShortHop(t *testing.T) {
	height := func(hold float64) float64 {
		c := New(Config{})
		in := Input{Jump: true, JumpPressed: true}
		peak := 0.0
		for elapsed := 0.0; elapsed < 2; elapsed += tick {
			in.Jump = elapsed < hold
			c.Step(tick, in)
			in.JumpPressed = false
			peak = math.Max(peak, c.View().Height)
		}
		return peak
	}
	if tap, held := height(.05), height(1); tap > held*.5 {
		t.Fatalf("tap rose %.1f, hold rose %.1f", tap, held)
	}
}

func TestOnlyOneJumpInTheAir(t *testing.T) {
	c := New(Config{})
	var events []Event
	in := Input{Jump: true}
	for step := 0; step < 200; step++ {
		in.JumpPressed = step%8 == 0
		c.Step(tick, in)
		events = append(events, c.Events()...)
		if count(events, Landed) > 0 {
			break
		}
	}
	if count(events, DoubleJumped) != 1 {
		t.Fatalf("double jumps = %d, want 1 per flight", count(events, DoubleJumped))
	}
}

func TestComboLandsThreeStrikesAndLungesForward(t *testing.T) {
	c := New(Config{})
	c.Place(1000, 0, 0)
	var events []Event
	in := Input{}
	for step := 0; step < 240; step++ {
		// Three presses a third of a second apart, then hands off.
		in.AttackPressed = step%20 == 0 && step < 60
		c.Step(tick, in)
		events = append(events, c.Events()...)
	}
	if count(events, Struck) != 2 || count(events, StruckHard) != 1 {
		t.Fatalf("events = %v, want two jabs and a heavy strike", events)
	}
	x, _ := c.Position()
	if want := 1000 - (2*jabLunge + heavyLunge); math.Abs(x-want) > 6 {
		t.Fatalf("combo carried the pet to %.1f, want about %.1f", x, want)
	}
	if v := c.View(); !v.Still || v.Stance {
		t.Fatalf("pet did not return to standing: %+v", v)
	}
}

func TestStrikeOnTheGroundHoldsThePetInPlace(t *testing.T) {
	c := New(Config{})
	c.Step(tick, Input{AttackPressed: true})
	run(c, dropTime+.02, Input{})
	start, _ := c.Position()
	if !c.View().Stance {
		t.Fatal("expected the stance after the drop")
	}
	run(c, .1, Input{Right: true, Run: true})
	if x, _ := c.Position(); x > start {
		t.Fatalf("steering moved the pet mid-strike: %.1f to %.1f", start, x)
	}
	if c.View().FacingRight {
		t.Fatal("steering turned the pet mid-strike")
	}
}

func TestStrikeInTheAirKeepsFlying(t *testing.T) {
	c := New(Config{})
	c.Step(tick, Input{Jump: true, JumpPressed: true})
	run(c, .15, Input{Jump: true})
	c.Step(tick, Input{Jump: true, AttackPressed: true})
	run(c, dropTime+.05, Input{Jump: true})
	if v := c.View(); !v.Stance || v.Height <= 0 {
		t.Fatalf("expected a strike in flight: %+v", v)
	}
}

func TestGotoWalksThereAndReportsArrival(t *testing.T) {
	c := New(Config{})
	c.Goto(600, -200, true)
	var events []Event
	for step := 0; step < 600 && c.Travelling(); step++ {
		c.Step(tick, Input{})
		events = append(events, c.Events()...)
	}
	x, y := c.Position()
	if math.Hypot(x-600, y+200) > 30 || count(events, Arrived) != 1 {
		t.Fatalf("stopped at (%.1f, %.1f) with events %v", x, y, events)
	}
}

func TestGotoGivesUpAtAWall(t *testing.T) {
	c := New(Config{Free: func(x, y float64) bool { return x <= 50 }})
	c.Goto(900, 0, false)
	for step := 0; step < 600 && c.Travelling(); step++ {
		c.Step(tick, Input{})
	}
	if c.Travelling() {
		t.Fatal("pet kept walking into the wall")
	}
}

func TestSteeringCancelsGoto(t *testing.T) {
	c := New(Config{})
	c.Goto(900, 0, false)
	c.Step(tick, Input{Left: true})
	if c.Travelling() {
		t.Fatal("manual steering did not take over")
	}
}

func TestSwarmStaysSmallAndCatchingBuildsAStreak(t *testing.T) {
	s := NewSwarm(7)
	area := Area{0, 0, 1920, 1040}
	most := 0
	for step := 0; step < 60*40; step++ {
		s.Step(tick, area, 960, 900, .5)
		most = max(most, len(s.Flies()))
	}
	if most == 0 || most > maxFlies {
		t.Fatalf("flies on screen peaked at %d", most)
	}
	for _, f := range s.Flies() {
		if f.Y > 900 {
			t.Fatalf("fly %d hovers below the pet's feet at y=%.0f", f.ID, f.Y)
		}
	}
	f := s.Flies()[0]
	if got := s.Catch(f.X+3, f.Y-3, 20); got != 1 || s.Score != 1 || s.Streak != 1 {
		t.Fatalf("catch = %d, score %d, streak %d", got, s.Score, s.Streak)
	}
	if got := s.Catch(-9999, -9999, 20); got != 0 || s.Streak != 1 {
		t.Fatalf("a miss changed the streak: caught %d, streak %d", got, s.Streak)
	}
	for elapsed := 0.0; elapsed < streakTime+.2; elapsed += tick {
		s.Step(tick, area, 960, 900, .5)
	}
	if s.Streak != 0 || s.Score != 1 || s.BestStreak != 1 {
		t.Fatalf("after the window: streak %d, score %d, best %d", s.Streak, s.Score, s.BestStreak)
	}
}

func TestStrollPicksDestinationsOnScreen(t *testing.T) {
	s := NewStroll(3)
	area := Area{100, 100, 800, 600}
	for i := 0; i < 200; i++ {
		x, y := s.Pick(area, 120, 650, .5)
		if !area.contains(x, y, 0) {
			t.Fatalf("destination (%.0f, %.0f) is off screen", x, y)
		}
	}
	waited := 0.0
	for !s.Due(tick) {
		waited += tick
	}
	if waited < 6 || waited > 22 {
		t.Fatalf("rested %.1fs before strolling", waited)
	}
}
