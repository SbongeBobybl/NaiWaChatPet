package play

import (
	"math"
	"math/rand"
	"testing"
)

func TestDropsGetRarerTowardTheTop(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	seen := map[string]int{}
	none := 0
	for i := 0; i < 4000; i++ {
		w, ok := RollDrop(rng, 1, false)
		if !ok {
			none++
			continue
		}
		seen[w.ID]++
	}
	if none < 1400 || none > 2200 {
		t.Fatalf("round 1 dropped nothing %d times in 4000, want about 45%%", none)
	}
	if seen["stick"] <= seen["pan"] || seen["pan"] <= seen["swatter"] || seen["swatter"] == 0 {
		t.Fatalf("drop counts %v do not fall off with rarity", seen)
	}
	for i := 0; i < 500; i++ {
		w, ok := RollDrop(rng, 4, true)
		if !ok || w.ID == "stick" || w.ID == "lotus" {
			t.Fatalf("the boss dropped %v (%t); it should always drop something good", w.ID, ok)
		}
	}
}

func TestRivalsCycleAndToughenEachLap(t *testing.T) {
	first, again := RivalFor(1), RivalFor(1+len(Rivals))
	if first.ID != again.ID || again.HP <= first.HP || again.Aggression <= first.Aggression {
		t.Fatalf("lap two %+v is not a tougher %+v", again, first)
	}
	if !RivalFor(len(Rivals)).Boss {
		t.Fatal("the last rival of a lap should be the boss")
	}
	if FindWeapon("pan").Name != "平底锅" || FindWeapon("missing") != Bare {
		t.Fatal("weapon lookup")
	}
}

func TestReachesUsesTheBodyNotJustTheFeet(t *testing.T) {
	const unit = .5
	// A fighter with its feet at (1000, 800) stands about 240 pixels tall here.
	if !Reaches(1000-60, 700, 1000, 800, 0, unit, 10) {
		t.Fatal("a palm inside the body missed")
	}
	if !Reaches(1000-62-20, 700, 1000, 800, 0, unit, 25) {
		t.Fatal("a palm just short of the body missed despite the reach")
	}
	if Reaches(1000-62-60, 700, 1000, 800, 0, unit, 25) {
		t.Fatal("a palm well short of the body hit")
	}
	// A fighter high in a jump cannot be hit at its feet.
	if Reaches(1000, 790, 1000, 800, 400, unit, 10) {
		t.Fatal("a palm at ground level hit a fighter in the air")
	}
	if SameLane(800, 800+50, unit) || !SameLane(800, 830, unit) {
		t.Fatal("lane depth check")
	}
}

func TestArenaRunsRoundsToTheEnd(t *testing.T) {
	a := NewArena()
	if a.Round != 1 || a.Live() {
		t.Fatalf("a fresh arena is %+v", a)
	}
	for i := 0; i < 120 && !a.Live(); i++ {
		a.Step(tick)
	}
	if !a.Live() || a.HitFoe(10) {
		t.Fatal("the fight never started, or one jab knocked the rival out")
	}
	if a.Combo != 1 {
		t.Fatalf("combo = %d", a.Combo)
	}
	if !a.HitFoe(1000) || a.Live() || !a.Won() {
		t.Fatal("a knockout did not end the round in the pet's favour")
	}
	a.PetHP = 50
	for i := 0; i < 400 && a.Round == 1; i++ {
		a.Step(tick)
	}
	if a.Round != 2 || a.Beaten != 1 || a.PetHP != 80 || a.FoeHP != RivalFor(2).HP {
		t.Fatalf("after the win: round %d, beaten %d, pet %.0f, rival %.0f", a.Round, a.Beaten, a.PetHP, a.FoeHP)
	}
	var events []ArenaEvent
	for i := 0; i < 200 && !a.Live(); i++ {
		a.Step(tick)
	}
	a.HitPet(1000)
	for i := 0; i < 400 && a.Phase != Over; i++ {
		a.Step(tick)
		events = append(events, a.Events()...)
	}
	if a.Phase != Over || events[len(events)-1] != PetLost {
		t.Fatalf("losing did not end the run: phase %v, events %v", a.Phase, events)
	}
	// Health bars drain after the hit rather than snapping.
	b := NewArena()
	for !b.Live() {
		b.Step(tick)
	}
	b.HitPet(30)
	b.Step(tick)
	if b.PetShown <= b.PetHP || b.PetShown >= PetHP {
		t.Fatalf("shown health %.1f should trail real health %.1f", b.PetShown, b.PetHP)
	}
}

func TestTimeUpGoesToTheHealthierSide(t *testing.T) {
	a := NewArena()
	for !a.Live() {
		a.Step(tick)
	}
	a.HitFoe(a.Rival.HP * .5)
	a.HitPet(10)
	for i := 0; i < int((RoundTime+1)/tick) && a.Live(); i++ {
		a.Step(tick)
	}
	if a.Live() || !a.Won() {
		t.Fatal("time up should have gone to the pet")
	}
}

func TestBrainClosesInThenStrikes(t *testing.T) {
	b := NewBrain(2, RivalFor(1))
	s := Sense{X: 0, Y: 500, FoeX: 900, FoeY: 500, Facing: -1, Unit: 1}
	if in := b.Think(tick, s); !in.Right || in.Left {
		t.Fatalf("a distant rival should walk towards the pet: %+v", in)
	}
	s.FoeX = 260
	if in := b.Think(tick, s); !in.Right {
		t.Fatalf("a rival facing away should turn first: %+v", in)
	}
	s.Facing = 1
	struck := false
	for i := 0; i < 200 && !struck; i++ {
		struck = b.Think(tick, s).AttackPressed
	}
	if !struck {
		t.Fatal("a rival in range never attacked")
	}
	// Pressed right up against the pet between strikes, it backs off.
	s.FoeX = 80
	if in := b.Think(tick, s); !in.Left || in.Right {
		t.Fatalf("a rival in a clinch should step back: %+v", in)
	}
}

func TestAHitKnocksBackAndStuns(t *testing.T) {
	c := New(Config{})
	c.Hit(1, 1500, .3, 0)
	run(c, .2, Input{Left: true})
	x, _ := c.Position()
	if x < 60 {
		t.Fatalf("knockback carried the pet %.1f, and steering should not fight it while stunned", x)
	}
	if v := c.View(); v.Gait.Lean >= 0 || v.Still {
		t.Fatalf("a hurt pet should recoil: %+v", v)
	}
	run(c, 1, Input{})
	if !c.View().Still {
		t.Fatal("the pet did not recover from the blow")
	}

	c.Hit(-1, 1800, .5, 1400)
	peak := 0.0
	for i := 0; i < 60; i++ {
		c.Step(tick, Input{})
		peak = math.Max(peak, c.View().Height)
	}
	if peak < 80 {
		t.Fatalf("a launch only lifted the pet %.1f", peak)
	}
	c.Down(true)
	run(c, 1, Input{Right: true, JumpPressed: true})
	if v := c.View(); v.Gait.Topple != 1 || v.Still {
		t.Fatalf("a knocked-out pet should lie toppled: %+v", v)
	}
	c.Down(false)
	run(c, .5, Input{})
	if !c.View().Still {
		t.Fatal("the pet did not get back up")
	}
}
