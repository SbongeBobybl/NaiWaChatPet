//go:build windows

package main

import (
	"fmt"
	"image"
	"image/draw"
	"math"
	"math/rand"
	"runtime"
	"time"
	"unsafe"

	"codexpet/reconstruction/animation"
	"codexpet/reconstruction/compositor"
	"codexpet/reconstruction/play"
	"codexpet/reconstruction/rig"
)

const (
	menuFight = 328
	// menuWeapon plus 0 is bare hands, plus 1 and up the weapons in order.
	menuWeapon = 700
)

// rivalState is the opponent of the current round: a recoloured pet in a
// window of its own, driven by the same controller as the pet.
type rivalState struct {
	override *play.FighterState
	ctl      *play.Controller
	brain    *play.Brain
	rival    play.Rival
	win      *sprite
	shadow   *sprite
	textures map[string]*rig.Texture
	flash    float64
	fade     float64
	down     bool
	drawn    play.View
	drawnAt  time.Duration
	look     [3]float64
}

// dropState is a weapon a beaten rival left, bouncing to rest until the pet
// walks over it.
type dropState struct {
	weapon play.Weapon
	x, y   float64
	z, vz  float64
	sprite *sprite
	born   time.Duration
}

type fightState struct {
	upgraded *combatPresentation
	arena    *play.Arena
	foe      *rivalState
	rng      *rand.Rand
	hitstop  float64
	slow     float64
	// Who got knocked out, until they land and lie still.
	koPet  bool
	endAt  time.Duration
	hud    *sprite
	hudKey string

	banner      *sprite
	bannerText  string
	bannerStyle bannerStyle
	bannerBorn  time.Duration
	bannerLife  time.Duration

	combo     *sprite
	comboText string
	comboBorn time.Duration

	drops []*dropState
	// auto fights for the pet, for watching a bout rather than playing one.
	auto *play.Brain
}

// scale is how much fight time passes for dt of real time: none during the
// hit-stop of a blow, a fraction during a knockout.
func (f *fightState) scale(dt float64) float64 {
	switch {
	case f.hitstop > 0:
		return 0
	case f.slow > 0:
		return dt * .3
	}
	return dt
}

// weapon is what the pet has in hand.
func (p *playState) weapon() play.Weapon { return play.FindWeapon(p.settings.Equipped) }

// autoInput is the pet's keys when it fights on its own.
func (a *desktopHost) autoInput(f *fightState) play.Input {
	if f.auto == nil || f.foe == nil || !f.arena.Live() {
		return play.Input{}
	}
	fx, fy := f.foe.ctl.Position()
	view := f.foe.ctl.View()
	facing := -1
	if a.play.view.FacingRight {
		facing = 1
	}
	return f.auto.Think(1.0/60, play.Sense{X: a.feetX(), Y: a.groundLine(), FoeX: fx, FoeY: fy, Facing: facing, FoeHeight: view.Height,
		FoeStriking: view.Stance && view.Strike > 0 && view.Strike < rig.StrikeImpact+.05, Busy: a.play.view.Stance, Unit: a.factor()})
}

// startFight opens the arena: control mode, a rival from the far side of the
// screen and the round announcement.
func (a *desktopHost) startFight() {
	p := a.play
	if p == nil || p.fight != nil || p.class == nil {
		return
	}
	p.swarm.Reset()
	core := play.NewCombat(true)
	if combatRulesPath != "" {
		rules, err := play.ReadCombatRules(combatRulesPath)
		if err != nil {
			a.combatStartError(fmt.Errorf("读取战斗配置失败：%w", err))
			return
		}
		core, err = play.NewConfiguredCombat(true, &rules)
		if err != nil {
			a.combatStartError(fmt.Errorf("战斗配置无效：%w", err))
			return
		}
	}
	core.SetWeapon(0, p.settings.Equipped)
	core.SetAI(1, play.NewCombatAI(time.Now().UnixNano(), p.settings.Game.Difficulty))
	f := &fightState{arena: core.Arena, rng: rand.New(rand.NewSource(time.Now().UnixNano())), upgraded: newCombatPresentation(core)}
	p.fight = f
	a.suspendAmbientForCombat(a.now())
	a.setControl(true)
	a.startCombatClock(f.upgraded)
	a.warmCombatVFX(f.upgraded)
}

// spawnRival brings in this round's opponent from the side away from the pet.
func (a *desktopHost) spawnRival() {
	p, f := a.play, a.play.fight
	if f.foe != nil {
		a.dropRival()
	}
	area := a.standArea()
	x := area.X + area.W*.9
	if a.feetX() > area.X+area.W/2 {
		x = area.X + area.W*.1
	}
	r := f.arena.Rival
	foe := &rivalState{rival: r, brain: play.NewBrain(f.rng.Int63(), r), textures: map[string]*rig.Texture{}}
	foe.look = r.Palette[1]
	foe.ctl = play.New(play.Config{Unit: a.factor() * r.Speed, Free: a.standable})
	facing := 1
	if x > a.feetX() {
		facing = -1
	}
	foe.ctl.Place(x, a.groundLine(), facing)
	var err error
	if foe.win, err = p.class.open(); err != nil {
		return
	}
	foe.shadow, _ = p.class.open()
	f.foe = foe
	a.say(fmt.Sprintf("第 %d 回合：%s 来了！", f.arena.Round, r.Name))
}

func (a *desktopHost) dropRival() {
	f := a.play.fight
	if f.foe != nil {
		f.foe.win.close()
		f.foe.shadow.close()
		f.foe = nil
	}
}

// endFight closes the arena and says how it went.
func (a *desktopHost) endFight() {
	p := a.play
	f := p.fight
	if f == nil {
		return
	}
	a.dropRival()
	f.closeCombat()
	p.fight = nil
	a.resumeAmbientAfterCombat(a.now())
	for _, e := range p.effects {
		e.sprite.close()
	}
	p.effects = nil
	p.ctl.Halt()
	p.view = play.View{Still: true, FacingRight: p.flip}
	if f.upgraded != nil && f.upgraded.windowReduced && !a.closing {
		x, y := a.feetX(), a.groundLine()
		if err := a.resize(f.upgraded.originalScale, f.upgraded.originalDPI); err != nil {
			_, _ = logDiagnostic(err)
		}
		x, y = clampTo(a.standArea(), x, y)
		a.placeFeet(x, y, 0, p.flip)
	}
	f.hud.close()
	f.banner.close()
	f.combo.close()
	// Anything still on the floor is the pet's anyway.
	for _, d := range f.drops {
		a.collect(d)
	}
	p.ctl.Down(false)
	p.flash, a.jolt = 0, 0
	beaten, round := f.arena.Beaten, f.arena.Round
	if f.arena.Phase != play.Over {
		round--
	}
	p.settings.Wins += beaten
	best := ""
	if round > p.settings.BestRound {
		p.settings.BestRound = round
		best = "，新纪录！"
	}
	p.save()
	if beaten > 0 || f.arena.Phase == play.Over {
		a.say(fmt.Sprintf("这次打倒了 %d 个对手，坚持到第 %d 回合%s\n累计胜利 %d 场，最好成绩第 %d 回合。", beaten, max(1, f.arena.Round), best, p.settings.Wins, p.settings.BestRound))
	}
	a.haveKey = false
}

// stepFight runs the arena for one tick. dt is real time and scaled the
// fight's own, slowed or frozen by the effects of a blow.
func (a *desktopHost) stepFight(now time.Duration, dt, scaled float64) {
	p := a.play
	f := p.fight
	if f == nil {
		return
	}
	f.hitstop = math.Max(0, f.hitstop-dt)
	f.slow = math.Max(0, f.slow-dt)
	p.flash = math.Max(0, p.flash-dt)
	unit := a.factor()
	// The one who was hit shakes while the blow holds time still.
	a.jolt = 0
	if f.hitstop > 0 && p.flash > 0 {
		a.jolt = int(5*unit*(1+math.Sin(float64(now)/float64(16*time.Millisecond)*math.Pi))) - int(5*unit)
	}
	f.arena.Step(scaled)
	for _, e := range f.arena.Events() {
		a.announce(e, now)
	}
	if f.endAt != 0 && now >= f.endAt {
		a.endFight()
		a.setControl(false)
		return
	}
	if foe := f.foe; foe != nil {
		foe.flash = math.Max(0, foe.flash-dt)
		foe.ctl.Configure(play.Config{Unit: unit * foe.rival.Speed, Free: a.standable})
		fx, fy := foe.ctl.Position()
		view := foe.ctl.View()
		var in play.Input
		if f.arena.Live() && !foe.down {
			facing := -1
			if view.FacingRight {
				facing = 1
			}
			in = foe.brain.Think(scaled, play.Sense{X: fx, Y: fy, FoeX: a.feetX(), FoeY: a.groundLine(), Facing: facing, FoeHeight: p.view.Height,
				FoeStriking: p.view.Stance && p.view.Strike > 0 && p.view.Strike < rig.StrikeImpact+.05, Busy: view.Stance, Unit: unit})
		}
		if scaled > 0 {
			foe.ctl.Step(scaled, in)
		}
		for _, e := range foe.ctl.Events() {
			switch e {
			case play.Struck, play.StruckHard:
				a.rivalStrikes(e == play.StruckHard, now)
			case play.Landed:
				if f.arena.Phase == play.KnockedOut && f.arena.Won() {
					foe.down = true
					foe.ctl.Down(true)
				}
			}
		}
		if foe.down {
			foe.fade = math.Min(1, foe.fade+dt/.9)
		}
		a.drawRival(now)
	}
	a.stepDrops(now, scaled)
	a.drawArena(now)
}

// announce turns the arena's events into banners and lines.
func (a *desktopHost) announce(e play.ArenaEvent, now time.Duration) {
	f := a.play.fight
	switch e {
	case play.AnnounceRound:
		if f.upgraded == nil {
			a.spawnRival()
		}
		a.banner(fmt.Sprintf("ROUND %d", f.arena.Round), goldBanner, now, 1300*time.Millisecond)
	case play.AnnounceFight:
		a.banner("FIGHT!", fireBanner, now, 900*time.Millisecond)
	case play.AnnounceKO:
		f.slow = 1.1
		a.banner("K.O.", fireBanner, now, 2*time.Second)
	case play.AnnounceTimeUp:
		a.banner("TIME UP", iceBanner, now, 1500*time.Millisecond)
	case play.PetWon:
		a.emote("cheer", 3*time.Second)
	case play.PetLost:
		a.banner("YOU LOSE", iceBanner, now, 1800*time.Millisecond)
		f.endAt = now + 1800*time.Millisecond
		a.emote("dizzy", 5*time.Second)
	}
}

func (a *desktopHost) banner(text string, style bannerStyle, now, life time.Duration) {
	f := a.play.fight
	f.bannerText, f.bannerStyle, f.bannerBorn, f.bannerLife = text, style, now, life
}

// petStrikes resolves the pet's palm landing at (x, y) during a fight. It
// reports whether it struck the rival.
func (a *desktopHost) petStrikes(x, y float64, hard bool, now time.Duration) bool {
	p := a.play
	f := p.fight
	if f.upgraded != nil {
		return false
	}
	foe := f.foe
	if !f.arena.Live() || foe == nil || foe.down {
		return false
	}
	unit := a.factor()
	w := p.weapon()
	fx, fy := foe.ctl.Position()
	if !play.SameLane(a.groundLine(), fy, unit) || !play.Reaches(x, y, fx, fy, foe.ctl.View().Height, unit, 105*unit*w.Reach) {
		return false
	}
	damage, push, stun, lift, stop := play.JabDamage, 1000.0, .32, 0.0, .075
	if hard {
		damage, push, stun, lift, stop = play.HeavyDamage, 1600, .5, 900, .13
	}
	damage *= w.Damage
	push *= w.Knock
	ko := f.arena.HitFoe(damage)
	if ko {
		push, stun, lift, stop = 2100*w.Knock, 99, 1500, .18
	}
	dir := -1
	if p.view.FacingRight {
		dir = 1
	}
	foe.ctl.Hit(dir, push, stun, lift)
	foe.flash = .15
	f.hitstop = stop
	if hard || ko {
		p.spawnRing(x, y, int(math.Min(360, 380*unit)), now)
	}
	if f.arena.Combo >= 2 {
		f.comboText, f.comboBorn = fmt.Sprintf("%d HITS", f.arena.Combo), now
	}
	if ko {
		if drop, ok := play.RollDrop(f.rng, f.arena.Round, f.arena.Rival.Boss); ok {
			f.drops = append(f.drops, &dropState{weapon: drop, x: fx, y: fy, z: 260 * unit, vz: 700 * unit, born: now})
		}
	}
	return true
}

// rivalStrikes resolves the rival's palm landing.
func (a *desktopHost) rivalStrikes(hard bool, now time.Duration) {
	p := a.play
	f := p.fight
	if f.upgraded != nil {
		return
	}
	foe := f.foe
	if !f.arena.Live() || foe == nil {
		return
	}
	unit := a.factor()
	view := foe.ctl.View()
	power := 1.0
	if hard {
		power = 1.9
	}
	palm := rig.Palm(p.stance, rig.StrikePose(rig.StrikeImpact, power))
	forward := -1.0
	if view.FacingRight {
		palm.X, forward = rig.CanvasWidth-palm.X, 1
	}
	fx, fy := foe.ctl.Position()
	wx, wy := a.rivalWindow(fx, fy, view)
	x, y := float64(wx)+(palm.X+forward*22)*unit, float64(wy)+palm.Y*unit
	p.spawn(effect{x: x, y: y, born: now, hard: hard, size: max(24, int(math.Min(200, 170*unit)))})
	if !play.SameLane(fy, a.groundLine(), unit) || !play.Reaches(x, y, a.feetX(), a.groundLine(), p.view.Height, unit, 95*unit) {
		return
	}
	damage, push, stun, lift, stop := play.RivalJab, 900.0, .3, 0.0, .07
	if hard {
		damage, push, stun, lift, stop = play.RivalHeavy, 1400, .45, 800, .12
	}
	if f.arena.HitPet(damage) {
		push, stun, lift, stop = 1900, 99, 1400, .18
		f.koPet = true
	}
	p.ctl.Hit(int(forward), push, stun, lift)
	p.flash = .15
	f.hitstop = stop
	if hard {
		p.spawnRing(x, y, int(math.Min(320, 340*unit)), now)
	}
}

// petLanded lays the pet down once a knockout throw comes to earth.
func (a *desktopHost) petLanded() {
	if f := a.play.fight; f != nil && f.koPet {
		a.play.ctl.Down(true)
	}
}

func (p *playState) spawnRing(x, y float64, size int, now time.Duration) {
	p.spawn(effect{x: x, y: y, born: now, size: size, ring: true})
}

// rivalWindow is where the rival's window goes for feet at (x, y).
func (a *desktopHost) rivalWindow(x, y float64, v play.View) (int, int) {
	anchor := feetAnchor
	if v.FacingRight {
		anchor = rig.CanvasWidth - feetAnchor
	}
	f := a.factor()
	return int(math.Round(x - anchor*f)), int(math.Round(y - rig.GroundY*f - v.Height))
}

func (r *rivalState) texture(key string, source func() (*image.NRGBA, error)) (*rig.Texture, error) {
	if t, ok := r.textures[key]; ok {
		return t, nil
	}
	frame, err := source()
	if err != nil {
		return nil, err
	}
	t := rig.NewTexture(recolour(frame, r.rival.Palette))
	r.textures[key] = t
	return t, nil
}

// drawRival renders the rival through the same rigs as the pet, recoloured,
// flashing on a hit and fading out once beaten.
func (a *desktopHost) drawRival(now time.Duration) {
	p := a.play
	foe := p.fight.foe
	v := foe.ctl.View()
	fx, fy := foe.ctl.Position()
	x, y := a.rivalWindow(fx, fy, v)
	if foe.override != nil {
		v = foe.override.View
		if foe.override.Action.Skill != play.NoSkill {
			v.Still = false
		}
		v.Height = foe.override.Height * a.factor()
		fx, fy = p.fight.upgraded.viewport.Point(foe.override.X, foe.override.Y)
		x, y = a.rivalWindow(fx, fy, v)
	}
	if foe.fade >= 1 {
		foe.win.hide()
		foe.shadow.hide()
		return
	}
	// Redraw at most thirty times a second while it moves, at once when it
	// starts, stops or changes rig, and whenever a flash or fade is running.
	changed := v != foe.drawn || foe.flash > 0 || foe.fade > 0 || !foe.win.shown || foe.override != nil && foe.override.Action.Skill != play.NoSkill
	if changed {
		var frame *image.NRGBA
		var err error
		if foe.override != nil && foe.override.Action.Skill >= play.FanSkill && foe.override.Action.Skill <= play.DodgeSkill && p.fight.upgraded.core.Definition(foe.override.Action.Skill).Clip != "" {
			frame, err = a.cachedCombatSkillFrame(foe.override.Action.Skill, foe.override.Action.Age)
			if err == nil {
				frame = recolour(frame, foe.rival.Palette)
			}
		} else if v.Stance {
			var tex *rig.Texture
			if tex, err = foe.texture("fight", func() (*image.NRGBA, error) { return a.assets.Bodies.BodyCopy(string(animation.Fight), 12) }); err == nil {
				frame, err = p.stance.RenderSized(tex, rig.StrikePose(v.Strike, v.Power).Add(rig.Settle(p.stance, v.Settle)), a.w, a.h)
			}
		} else {
			var tex *rig.Texture
			if tex, err = foe.texture("idle", func() (*image.NRGBA, error) { return a.assets.Bodies.BodyCopy(string(animation.Idle), 0) }); err == nil {
				frame, err = p.stand.RenderSized(tex, rig.StandPose(v.Gait), a.w, a.h)
			}
		}
		if err != nil {
			_, _ = logDiagnostic(fmt.Errorf("rival: %w", err))
			return
		}
		if foe.flash > 0 {
			whiten(frame, foe.flash/.15*.85)
		}
		if foe.fade > 0 {
			fadeOut(frame, 1-foe.fade)
		}
		canvas := image.NewRGBA(frame.Rect)
		draw.Draw(canvas, canvas.Rect, frame, image.Point{}, draw.Src)
		if v.FacingRight {
			mirror(canvas.Pix, canvas.Stride, canvas.Rect.Dx(), canvas.Rect.Dy())
		}
		resized, err := compositor.Resize(canvas, a.w, a.h)
		if err != nil {
			return
		}
		jolt := 0
		if p.fight.hitstop > 0 && foe.flash > 0 {
			jolt = int(6 * a.factor() * math.Sin(float64(now)/float64(16*time.Millisecond)*math.Pi))
		}
		if foe.win.draw(x+jolt, y, a.w, a.h, compositor.BGRA(resized)) == nil {
			foe.drawn, foe.drawnAt = v, now
		}
	} else if foe.win.shown {
		// Unchanged pixels, but the window may still be moving.
		_, _ = call(setWindowPos, foe.win.hwnd, 0, uintptr(x), uintptr(y), 0, 0, 0x0001|0x0004|0x0010)
	}
	// Its shadow, plain: only the pet's carries the gold rim of control.
	unit := a.factor()
	near := 1 / (1 + v.Height/(700*unit))
	w, h := max(8, int(300*unit*near)), max(4, int(66*unit*near))
	_ = foe.shadow.draw(int(fx)-w/2, int(fy-16*unit)-h/2, w, h, shadowSprite(w, h, near*(1-foe.fade), false))
	foe.shadow.under(foe.win.hwnd)
}

// stepDrops lets dropped weapons bounce to rest and hands them to the pet
// when it walks over one.
func (a *desktopHost) stepDrops(now time.Duration, dt float64) {
	p := a.play
	f := p.fight
	unit := a.factor()
	kept := f.drops[:0]
	for _, d := range f.drops {
		if d.z > 0 || d.vz != 0 {
			d.vz -= 2600 * unit * dt
			d.z += d.vz * dt
			if d.z <= 0 {
				d.z, d.vz = 0, -d.vz*.42
				if d.vz < 120*unit {
					d.vz = 0
				}
			}
		}
		if d.z < 20*unit && p.view.Height < 40*unit && math.Abs(a.feetX()-d.x) < 120*unit && play.SameLane(a.groundLine(), d.y, unit) {
			a.collect(d)
			continue
		}
		if d.sprite == nil {
			d.sprite, _ = p.class.open()
		}
		size := max(40, int(170*unit))
		glow := .6 + .4*math.Sin(float64(now-d.born)/float64(time.Second)*5)
		_ = d.sprite.draw(int(d.x)-size/2, int(d.y-d.z-40*unit)-size/2, size, size, weaponIcon(d.weapon.ID, size, glow))
		kept = append(kept, d)
	}
	f.drops = kept
}

// collect puts a dropped weapon in the pet's bag, equipping it if the pet's
// hands were empty.
func (a *desktopHost) collect(d *dropState) {
	p := a.play
	d.sprite.close()
	if p.settings.Weapons == nil {
		p.settings.Weapons = map[string]int{}
	}
	p.settings.Weapons[d.weapon.ID]++
	text := fmt.Sprintf("捡到了「%s」！", d.weapon.Name)
	if p.settings.Equipped == "" {
		p.settings.Equipped = d.weapon.ID
		text += "已经拿在手上了。"
	} else {
		text += "右键 → 武器 可以换上它。"
	}
	p.save()
	a.say(text)
}

// drawArena draws the bar across the top of the screen, the banner and the
// combo counter.
func (a *desktopHost) drawArena(now time.Duration) {
	p := a.play
	f := p.fight
	work, ok := workAreaAt(int(a.feetX()), int(a.groundLine())-2, true)
	if !ok {
		return
	}
	width := int(work.Right - work.Left)
	hudW := min(960, width*62/100)
	left := int(work.Left) + (width-hudW)/2
	r := f.arena.Rival
	colour := r.Palette[1]
	key := fmt.Sprintf("%d|%.0f|%.0f|%.0f|%.0f|%d|%d", hudW, f.arena.PetHP, f.arena.PetShown, f.arena.FoeHP, f.arena.FoeShown, int(math.Ceil(f.arena.Clock)), f.arena.Round)
	if f.upgraded != nil {
		key += fmt.Sprintf("|%d|%s", f.upgraded.snapshot.Tick/3, f.upgraded.status)
	}
	if f.hud == nil {
		f.hud, _ = p.class.open()
	}
	if key != f.hudKey {
		var pix []byte
		var h int
		if f.upgraded != nil {
			pix, h = combatHUDSprite(hudW, f.upgraded.snapshot, p.settings.Game.Keys, f.upgraded.localPlayer, f.upgraded.status)
		} else {
			pix, h = hudSprite(hudW, f.arena.PetHP/play.PetHP, f.arena.PetShown/play.PetHP, f.arena.FoeHP/r.HP, f.arena.FoeShown/r.HP, int(math.Ceil(f.arena.Clock)), f.arena.Round, r.Tag, colour)
		}
		if f.hud.draw(left, int(work.Top)+10, hudW, h, pix) == nil {
			f.hudKey = key
		}
	}
	dot := max(4, hudW/70)
	if age := now - f.bannerBorn; f.bannerText != "" && age < f.bannerLife {
		if f.banner == nil {
			f.banner, _ = p.class.open()
		}
		// Slam in large, settle, then fade at the end.
		t := float64(age) / float64(160*time.Millisecond)
		size := dot
		if t < 1 {
			size = int(float64(dot) * (1.8 - .8*t))
		}
		fade := math.Min(1, float64(f.bannerLife-age)/float64(300*time.Millisecond))
		pix, w, h := bannerSprite(f.bannerText, size, f.bannerStyle, fade)
		_ = f.banner.draw(int(work.Left)+width/2-w/2, int(work.Top)+int(work.Bottom-work.Top)*22/100-h/2, w, h, pix)
	} else {
		f.banner.hide()
	}
	if age := now - f.comboBorn; f.comboText != "" && age < time.Second && f.arena.Combo >= 2 {
		if f.combo == nil {
			f.combo, _ = p.class.open()
		}
		fade := math.Min(1, float64(time.Second-age)/float64(250*time.Millisecond))
		pix, w, h := bannerSprite(f.comboText, max(3, dot*2/3), iceBanner, fade)
		_ = f.combo.draw(left, int(work.Top)+int(float64(hudW)/240*22)+30, w, h, pix)
	} else {
		f.combo.hide()
	}
}

// weaponMenu lists the pet's weapons for equipping.
func (a *desktopHost) weaponMenu(menu uintptr) {
	p := a.play
	handle, _ := call(createMenu)
	if handle == 0 {
		return
	}
	item := func(id uintptr, label string, checked bool) {
		flags := uintptr(0)
		if checked {
			flags = 8
		}
		text := utf(label)
		_, _ = call(appendMenu, handle, flags, id, uintptr(unsafe.Pointer(text)))
		runtime.KeepAlive(text)
	}
	item(menuWeapon, "空手", p.settings.Equipped == "")
	owned := 0
	for i, w := range play.Weapons {
		if n := p.settings.Weapons[w.ID]; n > 0 {
			owned++
			item(menuWeapon+1+uintptr(i), fmt.Sprintf("%s ×%d　伤害 ×%.2f · 距离 ×%.2f · 击退 ×%.1f", w.Name, n, w.Damage, w.Reach, w.Knock), p.settings.Equipped == w.ID)
		}
	}
	if owned == 0 {
		text := utf("还没有武器：在擂台打倒对手会掉落")
		_, _ = call(appendMenu, handle, 1, 0, uintptr(unsafe.Pointer(text)))
		runtime.KeepAlive(text)
	}
	text := utf("武器")
	if ok, _ := call(appendMenu, menu, 0x10, handle, uintptr(unsafe.Pointer(text))); ok == 0 {
		_, _ = call(destroyMenu, handle)
	}
	runtime.KeepAlive(text)
}

// fightCommand handles the arena's menu entries.
func (a *desktopHost) fightCommand(command uintptr) bool {
	p := a.play
	if a.combatCommand(command) {
		return true
	}
	switch {
	case command == menuFight:
		if p.fight != nil && p.fight.upgraded.address != "" {
			a.endFight()
		}
		if p.fight == nil {
			a.startFight()
		} else if p.gamePanel != nil {
			p.gamePanel.resume()
		} else {
			p.fight.upgraded.paused = false
			_, _ = call(foregroundWindow, a.hwnd)
			_, _ = call(chatSetFocus, a.hwnd)
		}
	case command == menuWeapon:
		p.settings.Equipped = ""
		p.save()
	case command > menuWeapon && command <= menuWeapon+uintptr(len(play.Weapons)):
		w := play.Weapons[command-menuWeapon-1]
		if p.settings.Weapons[w.ID] > 0 {
			p.settings.Equipped = w.ID
			p.save()
			a.say(fmt.Sprintf("换上了「%s」。", w.Name))
		}
	default:
		return false
	}
	return true
}
