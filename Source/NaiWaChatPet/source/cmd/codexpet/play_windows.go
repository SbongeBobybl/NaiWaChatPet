//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"time"
	"unsafe"

	"codexpet/reconstruction/animation"
	"codexpet/reconstruction/companion"
	"codexpet/reconstruction/emotion"
	"codexpet/reconstruction/play"
	"codexpet/reconstruction/rig"
)

const (
	wmHotkey = 0x0312

	vkReturn, vkShift, vkEscape, vkSpace         = 0x0D, 0x10, 0x1B, 0x20
	vkLeft, vkUp, vkRight, vkDown        uintptr = 0x25, 0x26, 0x27, 0x28

	menuControl = 320
	menuSummon  = 321
	menuFlies   = 322
	menuStroll  = 323
	menuFocus   = 324
	menuShow    = 325
	// menuEmote plus a catalog index plays that expression.
	menuEmote = 600

	hotkeyControl = 0x4701
	hotkeySummon  = 0x4702
	hotkeyTalk    = 0x4703

	focusLength = 25 * time.Minute
	// feetAnchor is the canvas column midway between the pet's feet. The pet
	// turns around this column, so mirroring it does not move it sideways.
	feetAnchor = 240.0
)

var (
	playForeground = userDLL.NewProc("GetForegroundWindow")
	playAsyncKey   = userDLL.NewProc("GetAsyncKeyState")
	playRegister   = userDLL.NewProc("RegisterHotKey")
	playUnregister = userDLL.NewProc("UnregisterHotKey")
	playMonitorAt  = userDLL.NewProc("MonitorFromPoint")
)

// routine is the short show the pet puts on when asked: each step holds its
// keys from its start time until the next step, and its presses fire once.
var routine = []struct {
	at float64
	in play.Input
}{
	{0, play.Input{Right: true}},
	{1.3, play.Input{Right: true, Run: true}},
	{2.5, play.Input{Jump: true, JumpPressed: true}},
	{2.95, play.Input{Jump: true, JumpPressed: true}},
	{3.2, play.Input{Jump: true}},
	{3.9, play.Input{Left: true}},
	{5.0, play.Input{}},
	{5.2, play.Input{AttackPressed: true}},
	{5.55, play.Input{AttackPressed: true}},
	{5.9, play.Input{AttackPressed: true}},
	{7.0, play.Input{Left: true, Up: true}},
	{7.8, play.Input{Right: true, Down: true}},
	{8.6, play.Input{Jump: true, JumpPressed: true}},
	{8.85, play.Input{Jump: true, AttackPressed: true}},
	{9.6, play.Input{}},
	{10.6, play.Input{}},
}

// quickEmotes are the expressions on the number keys while steering.
var quickEmotes = [10]string{"dizzy", "greet", "cheer", "celebrate", "proud", "refuse", "hug", "ponder", "annoyed", "scared"}

// playSettings is what control mode remembers between runs.
type playSettings struct {
	Flies  bool `json:"flies"`
	Stroll bool `json:"stroll"`
	Best   int  `json:"best"`
	Hinted bool `json:"hinted"`
	// The arena: weapons owned by ID, the one in hand, and the record.
	Weapons   map[string]int `json:"weapons,omitempty"`
	Equipped  string         `json:"equipped,omitempty"`
	Wins      int            `json:"wins,omitempty"`
	BestRound int            `json:"bestRound,omitempty"`
}

type effect struct {
	sprite *sprite
	x, y   float64
	born   time.Duration
	// A burst has a size and no text; hard marks the third strike of a combo,
	// and ring the shockwave of a heavy blow.
	text       string
	size       int
	hard, ring bool
}

// playState is everything the host keeps for steering the pet, the fly game
// and the rigs that draw both.
type playState struct {
	carrying              []string
	carrySquash           float64
	ctl                   *play.Controller
	stand, stance, figure *rig.Puppet
	textures              map[string]*rig.Texture
	textureOrder          []string

	view, drawn          play.View
	last, drawnAt        time.Duration
	seq                  uint64
	control, flip        bool
	lift                 float64
	jumpEdge, attackEdge bool
	hotControl, hotSum   bool
	hotTalk              bool
	summoned             bool

	// A held pose is swapped in at the bottom of a short squash, so the pet
	// settles into it instead of cutting to it.
	shown                 string
	swapPhase, sceneShift int
	swapSince, shownSince time.Duration
	heldBody              *image.NRGBA

	settings playSettings
	// fight is the arena while one is running; flash whitens the pet the
	// moment it is hit.
	fight    *fightState
	flash    float64
	swarm    *play.Swarm
	stroll   *play.Stroll
	class    *spriteClass
	shadow   *sprite
	shadowAt [5]int
	flies    []*sprite
	flyArt   map[[3]int][]byte
	effects  []effect
	hud      *sprite
	hudUntil time.Duration

	focusEnd              time.Time
	greeted, pondering    bool
	ponderID              string
	lastTouch, lastSleepy time.Time
	shakes, shakeDir      int
	shakeAt               time.Duration
	dragX                 int32
	emotes                int
	// showing runs the routine; step is the next of its steps to start.
	showing  bool
	showFrom time.Duration
	step     int
}

func playSettingsPath() (string, error) {
	dir, err := companion.ProfileDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "play.json"), nil
}

func loadPlaySettings() playSettings {
	s := playSettings{Flies: true}
	if path, err := playSettingsPath(); err == nil {
		if b, err := os.ReadFile(path); err == nil && len(b) < 4096 {
			_ = json.Unmarshal(b, &s)
		}
	}
	s.Best = max(0, s.Best)
	return s
}

func (p *playState) save() {
	path, err := playSettingsPath()
	if err != nil {
		return
	}
	b, err := json.Marshal(p.settings)
	if err == nil && os.MkdirAll(filepath.Dir(path), 0700) == nil {
		err = os.WriteFile(path, b, 0600)
	}
	if err != nil {
		_, _ = logDiagnostic(fmt.Errorf("play settings not saved: %w", err))
	}
}

// initPlay builds the rigs and the controller. The pet runs without them if
// anything here fails; steering is an extra, not a requirement.
func (a *desktopHost) initPlay(module uintptr) {
	stand, e1 := rig.Stand()
	stance, e2 := rig.Stance()
	figure, e3 := rig.Figure()
	if err := errors.Join(e1, e2, e3); err != nil {
		_, _ = logDiagnostic(fmt.Errorf("control mode unavailable: %w", err))
		return
	}
	if stand.Bounds() != a.assets.Canvas {
		_, _ = logDiagnostic(errors.New("control mode unavailable: the rigs are bound to a 420x560 body canvas"))
		return
	}
	seed := time.Now().UnixNano()
	p := &playState{stand: stand, stance: stance, figure: figure, textures: map[string]*rig.Texture{}, flyArt: map[[3]int][]byte{},
		view: play.View{Still: true}, settings: loadPlaySettings(), swarm: play.NewSwarm(seed), stroll: play.NewStroll(seed + 1), lastTouch: time.Now()}
	a.play = p
	p.ctl = play.New(play.Config{Unit: a.factor(), Free: a.standable})
	if class, err := registerSpriteClass(module); err == nil {
		p.class = class
	} else {
		_, _ = logDiagnostic(fmt.Errorf("control mode sprites unavailable: %w", err))
	}
	// MOD_ALT | MOD_CONTROL | MOD_NOREPEAT. Another program may own either
	// combination already; the menu then offers the action without a shortcut.
	hot := func(id, key uintptr) bool {
		ok, _ := call(playRegister, a.hwnd, id, 0x4003, key)
		return ok != 0
	}
	p.hotControl, p.hotSum, p.hotTalk = hot(hotkeyControl, 'G'), hot(hotkeySummon, 'H'), hot(hotkeyTalk, 'T')
}

func (p *playState) close(hwnd uintptr) {
	if p == nil {
		return
	}
	if p.hotControl {
		_, _ = call(playUnregister, hwnd, hotkeyControl)
	}
	if p.hotSum {
		_, _ = call(playUnregister, hwnd, hotkeySummon)
	}
	if p.hotTalk {
		_, _ = call(playUnregister, hwnd, hotkeyTalk)
	}
	p.shadow.close()
	p.hud.close()
	for _, s := range p.flies {
		s.close()
	}
	for _, e := range p.effects {
		e.sprite.close()
	}
	p.class.close()
}

// factor converts body-canvas pixels to screen pixels at the pet's current size.
func (a *desktopHost) factor() float64 { return float64(a.w) / float64(a.assets.Canvas.Dx()) }

func (a *desktopHost) feetX() float64 {
	anchor := feetAnchor
	if a.play.flip {
		anchor = rig.CanvasWidth - feetAnchor
	}
	return float64(a.x) + anchor*a.factor()
}

// groundLine is the screen row the pet stands on, however high it has jumped.
func (a *desktopHost) groundLine() float64 {
	return float64(a.y) + rig.GroundY*a.factor() + a.play.lift
}

// placeFeet moves the window so the pet's feet are at a point on the ground,
// raised by height and facing the given way.
func (a *desktopHost) placeFeet(x, y, height float64, right bool) {
	p := a.play
	p.flip, p.lift = right, height
	anchor := feetAnchor
	if right {
		anchor = rig.CanvasWidth - feetAnchor
	}
	f := a.factor()
	a.x, a.y = int(math.Round(x-anchor*f)), int(math.Round(y-rig.GroundY*f-height))
}

// workAreaAt returns the work area of the monitor showing a point, or of the
// nearest monitor when asked. The point is packed into one argument as the
// 64-bit calling convention passes a POINT by value.
func workAreaAt(x, y int, nearest bool) (rectWin, bool) {
	flag := uintptr(0)
	if nearest {
		flag = 2
	}
	monitor, _ := call(playMonitorAt, uintptr(uint32(int32(x)))|uintptr(uint32(int32(y)))<<32, flag)
	if monitor == 0 {
		return rectWin{}, false
	}
	info := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	ok, _ := call(chatMonitorInfo, monitor, uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(&info)
	return info.Work, ok != 0
}

// standable reports whether the pet fits on screen with its feet at a point.
// Each corner is tested against whichever monitor shows it, so the pet can
// cross between monitors but not leave the desktop or sink behind the taskbar.
func (a *desktopHost) standable(x, y float64) bool {
	f := a.factor()
	for _, at := range [3][2]float64{{x - 120*f, y - 2}, {x + 120*f, y - 2}, {x, y - 500*f}} {
		px, py := int(at[0]), int(at[1])
		work, ok := workAreaAt(px, py, false)
		if !ok || px < int(work.Left) || px >= int(work.Right) || py < int(work.Top) || py >= int(work.Bottom) {
			return false
		}
	}
	return true
}

// standArea is where the feet may be on the pet's own monitor.
func (a *desktopHost) standArea() play.Area {
	f := a.factor()
	work, ok := workAreaAt(int(a.feetX()), int(a.groundLine())-2, true)
	if !ok {
		w, _ := call(getSystemMetrics, 0)
		h, _ := call(getSystemMetrics, 1)
		work = rectWin{0, 0, int32(w), int32(h)}
	}
	return play.Area{X: float64(work.Left) + 125*f, Y: float64(work.Top) + 505*f, W: math.Max(1, float64(work.Right-work.Left)-250*f), H: math.Max(1, float64(work.Bottom-work.Top)-505*f-3)}
}

func clampTo(area play.Area, x, y float64) (float64, float64) {
	return math.Max(area.X, math.Min(area.X+area.W, x)), math.Max(area.Y, math.Min(area.Y+area.H, y))
}

func (a *desktopHost) focused() bool {
	front, _ := call(playForeground)
	return front == a.hwnd
}

// quiet reports that nothing else is using the pet: no expression is playing
// and no chat window is open.
func (a *desktopHost) quiet() bool {
	return a.emotionView.Clip == "" && (a.chat == nil || !a.chat.inputVisible && !a.chat.replyVisible)
}

func (p *playState) poll() play.Input {
	down := func(keys ...uintptr) bool {
		for _, key := range keys {
			if state, _ := call(playAsyncKey, key); state&0x8000 != 0 {
				return true
			}
		}
		return false
	}
	return play.Input{Left: down(vkLeft, 'A'), Right: down(vkRight, 'D'), Up: down(vkUp, 'W'), Down: down(vkDown, 'S'), Run: down(vkShift),
		Jump: down('K', vkSpace), Attack: down('J'), JumpPressed: p.jumpEdge, AttackPressed: p.attackEdge}
}

// playKey handles a key pressed while the pet has the keyboard. It reports
// whether the key was one of its own.
func (a *desktopHost) playKey(key uintptr, repeat bool) bool {
	p := a.play
	if p == nil {
		return false
	}
	steer := key == vkLeft || key == vkRight || key == vkUp || key == vkDown || key == 'W' || key == 'A' || key == 'S' || key == 'D'
	act := key == 'J' || key == 'K' || key == vkSpace
	if !p.control {
		// Reaching for the movement keys is itself the request to steer.
		if repeat || !steer && !act {
			return false
		}
		a.setControl(true)
	}
	p.lastTouch = time.Now()
	switch {
	case key == vkEscape:
		a.setControl(false)
	case key == vkReturn || key == 'T':
		// Talk without stopping: the keys come back once the message is sent.
		if a.chat != nil {
			if err := a.chat.ShowInput(); err != nil {
				_, _ = logDiagnostic(err)
			}
		}
	case key == 'J':
		p.attackEdge = p.attackEdge || !repeat
	case key == 'K' || key == vkSpace:
		p.jumpEdge = p.jumpEdge || !repeat
	case steer:
	case key == 'F':
		if !repeat && p.fight == nil {
			a.startFight()
		}
	case key >= '0' && key <= '9':
		if !repeat && !p.ctl.Busy() {
			a.emote(quickEmotes[key-'0'], 6*time.Second)
		}
	default:
		return false
	}
	return true
}

// setControl turns keyboard steering on or off.
func (a *desktopHost) setControl(on bool) {
	p := a.play
	if p == nil || p.control == on {
		return
	}
	p.control, p.jumpEdge, p.attackEdge = on, false, false
	a.haveKey = false
	if on {
		if a.chat != nil {
			a.chat.HideInput()
		}
		if a.emotions != nil {
			a.emotions.InterruptAnimation("control")
		}
		// Bring a pet that was dragged half off the desktop back onto it.
		x, y := clampTo(a.standArea(), a.feetX(), a.groundLine())
		a.placeFeet(x, y, 0, p.flip)
		p.ctl.Halt()
		p.swarm.Reset()
		_, _ = call(foregroundWindow, a.hwnd)
		_, _ = call(chatSetFocus, a.hwnd)
		if !p.settings.Hinted {
			p.settings.Hinted = true
			p.save()
			a.say("方向键或 WASD 移动，按住 Shift 跑；J 出掌，K 跳，空中还能再跳一次。\n数字键 1-9 做表情，Enter 聊天，Esc 退出操控。")
		}
		return
	}
	a.endFight()
	if score := p.swarm.Score; score > 0 {
		text := fmt.Sprintf("这次抓到 %d 只苍蝇", score)
		if p.swarm.BestStreak >= 3 {
			text += fmt.Sprintf("，最高 %d 连击", p.swarm.BestStreak)
		}
		if score > p.settings.Best {
			p.settings.Best = score
			p.save()
			text += "，新纪录！"
			a.emote("proud", 6*time.Second)
		} else {
			text += fmt.Sprintf("。最好成绩是 %d 只。", p.settings.Best)
		}
		a.say(text)
	}
	p.swarm.Reset()
	p.hudUntil = 0
}

// emote plays an expression on the pet's own initiative.
func (a *desktopHost) emote(id string, hold time.Duration) bool {
	if a.emotions == nil || a.play == nil {
		return false
	}
	a.play.emotes++
	return a.emotions.Apply(fmt.Sprintf("pet-%d", a.play.emotes), emotion.Analysis{Emotion: id, Intensity: 1, Confidence: 1, DurationMS: int(hold / time.Millisecond)}, "manual-action").Accepted
}

// say shows a line of the pet's own in the reply bubble.
func (a *desktopHost) say(text string) {
	if a.trip != nil && a.trip.external != nil {
		return
	}
	if a.chat != nil {
		a.chat.say(text)
	}
}

// summon runs the pet over to the mouse pointer.
func (a *desktopHost) summon() {
	p := a.play
	if p == nil || a.assets.Engine.Snapshot().State != animation.Idle {
		return
	}
	at, err := cursor()
	if err != nil {
		return
	}
	f := a.factor()
	// Stop beside the pointer, on whichever side the pet comes from.
	side := 150 * f
	if a.feetX() < float64(at.X) {
		side = -side
	}
	work, ok := workAreaAt(int(at.X), int(at.Y), true)
	if !ok {
		return
	}
	area := play.Area{X: float64(work.Left) + 125*f, Y: float64(work.Top) + 505*f, W: math.Max(1, float64(work.Right-work.Left)-250*f), H: math.Max(1, float64(work.Bottom-work.Top)-505*f-3)}
	x, y := clampTo(area, float64(at.X)+side, float64(at.Y)+260*f)
	if a.emotions != nil {
		a.emotions.InterruptAnimation("summon")
	}
	if a.chat != nil {
		a.chat.HideInput()
	}
	p.ctl.Place(a.feetX(), a.groundLine(), 0)
	p.ctl.Goto(x, y, true)
	p.summoned = true
	p.lastTouch = time.Now()
}

// stepPlay advances steering, the fly game and the pet's own habits by one tick.
func (a *desktopHost) stepPlay(now time.Duration) {
	p := a.play
	if p == nil {
		return
	}
	dt := (now - p.last).Seconds()
	p.last = now
	snap := a.assets.Engine.Snapshot()
	// While the user holds the pet or a filmed action plays, the controller
	// waits and follows wherever the window goes.
	held := a.drag || a.pointerDown || a.menuOpen || snap.State != animation.Idle
	p.ctl.Configure(play.Config{Unit: a.factor(), Free: a.standable})
	if held || !p.ctl.Busy() {
		if held && p.ctl.Busy() {
			p.ctl.Halt()
			p.summoned = false
		}
		if held {
			p.lift = 0
		}
		facing := -1
		if p.flip {
			facing = 1
		}
		p.ctl.Place(a.feetX(), a.groundLine(), facing)
		if !p.view.Still {
			p.view = play.View{Still: true, FacingRight: p.flip}
			p.drawn = p.view
			p.seq++
		}
	}
	focused := a.focused()
	var in play.Input
	switch {
	case p.fight != nil && p.fight.auto != nil && !held:
		in = a.autoInput(p.fight)
	case p.showing && !held:
		in = a.perform(now)
	case p.control && focused && !held:
		in = p.poll()
	}
	// A fight freezes time for the hit-stop of a blow and slows it for a
	// knockout. A key pressed during the freeze waits for time to move again.
	scaled := dt
	if p.fight != nil {
		scaled = p.fight.scale(dt)
	}
	if scaled > 0 {
		p.jumpEdge, p.attackEdge = false, false
	}
	if !held {
		a.wander(dt)
		if (in != (play.Input{}) || p.ctl.Busy()) && scaled > 0 {
			if a.emotions != nil && a.emotionView.Clip != "" {
				a.emotions.InterruptAnimation("steering")
			}
			p.ctl.Step(scaled, in)
			p.view = p.ctl.View()
			// The body is redrawn at most thirty times a second; the window
			// itself moves on every tick, which is what keeps motion smooth.
			// Starting, stopping and switching rigs are drawn at once.
			if p.view != p.drawn && (now-p.drawnAt >= 30*time.Millisecond || p.view.Still != p.drawn.Still || p.view.Stance != p.drawn.Stance) {
				p.seq++
				p.drawn, p.drawnAt = p.view, now
			}
			x, y := p.ctl.Position()
			a.placeFeet(x, y, p.view.Height, p.view.FacingRight)
			for _, e := range p.ctl.Events() {
				a.playEvent(e, now)
			}
		}
	}
	a.stepFight(now, dt, scaled)
	a.stepGame(now, dt, focused)
	a.stepHabits(now)
}

// wander sends an unattended pet for a short walk now and then.
func (a *desktopHost) wander(dt float64) {
	p := a.play
	if a.trip != nil || !p.settings.Stroll || p.control || p.ctl.Busy() || !a.quiet() || !p.focusEnd.IsZero() {
		return
	}
	if p.stroll.Due(dt) {
		p.stroll.Rest()
		x, y := p.stroll.Pick(a.standArea(), a.feetX(), a.groundLine(), a.factor())
		p.ctl.Goto(x, y, false)
	}
}

func (a *desktopHost) playEvent(e play.Event, now time.Duration) {
	p := a.play
	switch e {
	case play.Struck, play.StruckHard:
		hard := e == play.StruckHard
		power, reach, size := 1.0, 105.0, 190.0
		if hard {
			power, reach, size = 1.9, 160, 280
		}
		f := a.factor()
		palm := rig.Palm(p.stance, rig.StrikePose(rig.StrikeImpact, power))
		forward := -1.0
		if p.flip {
			palm.X, forward = rig.CanvasWidth-palm.X, 1
		}
		x, y := float64(a.x)+(palm.X+forward*22)*f, float64(a.y)+palm.Y*f
		p.spawn(effect{x: x, y: y, born: now, hard: hard, size: max(24, int(math.Min(220, size*f)))})
		if p.fight != nil {
			a.petStrikes(x, y, hard, now)
		} else if p.playing() {
			a.caught(p.swarm.Catch(x, y, reach*f), x, y, now)
		}
	case play.Landed:
		a.petLanded()
	case play.Arrived:
		if p.summoned {
			p.summoned = false
			if at, err := cursor(); err == nil {
				// Turn to face whoever called.
				a.placeFeet(a.feetX(), a.groundLine(), 0, float64(at.X) > a.feetX())
				a.haveKey = false
			}
			a.emote("greet", 5*time.Second)
		}
	}
}

// playing reports whether the fly game is running.
func (p *playState) playing() bool {
	return p.control && p.settings.Flies && p.class != nil && p.fight == nil
}

func (a *desktopHost) caught(count int, x, y float64, now time.Duration) {
	if count == 0 {
		return
	}
	p := a.play
	p.spawn(effect{x: x, y: y - 34*a.factor(), born: now, text: "+" + fmt.Sprint(count)})
	p.hudUntil = now + 2600*time.Millisecond
}

// spawn starts a burst or a rising number in a sprite window of its own.
func (p *playState) spawn(e effect) {
	if p.class == nil || len(p.effects) >= 6 {
		return
	}
	s, err := p.class.open()
	if err != nil {
		return
	}
	e.sprite = s
	p.effects = append(p.effects, e)
}

// stepGame moves the flies and redraws every sprite of control mode.
func (a *desktopHost) stepGame(now time.Duration, dt float64, focused bool) {
	p := a.play
	if p.class == nil {
		return
	}
	f := a.factor()
	var flies []play.Fly
	if p.playing() {
		work, ok := workAreaAt(int(a.feetX()), int(a.groundLine())-2, true)
		if ok {
			p.swarm.Step(dt, play.Area{X: float64(work.Left), Y: float64(work.Top), W: float64(work.Right - work.Left), H: float64(work.Bottom - work.Top)}, a.feetX(), a.groundLine(), f)
		}
		// A jumping frog catches flies with its mouth.
		if p.view.Height > 24*f && !p.view.Stance {
			mouth := 150.0
			if p.flip {
				mouth = rig.CanvasWidth - mouth
			}
			x, y := float64(a.x)+mouth*f, float64(a.y)+112*f
			a.caught(p.swarm.Catch(x, y, 78*f), x, y, now)
		}
		flies = p.swarm.Flies()
	}
	size := max(18, int(math.Round(84*f)))
	for len(p.flies) < len(flies) {
		s, err := p.class.open()
		if err != nil {
			break
		}
		p.flies = append(p.flies, s)
	}
	for i, s := range p.flies {
		if i >= len(flies) {
			s.hide()
			continue
		}
		fly := flies[i]
		// Eight wing positions per heading are plenty and are drawn once each.
		key := [3]int{size, int(fly.Wing*8) % 8, fly.Heading}
		art, ok := p.flyArt[key]
		if !ok {
			if len(p.flyArt) > 64 {
				clear(p.flyArt)
			}
			art = flySprite(size, float64(key[1])/8, fly.Heading)
			p.flyArt[key] = art
		}
		_ = s.draw(int(fly.X)-size/2, int(fly.Y)-size/2, size, size, art)
	}

	if p.control {
		// The shadow stays on the ground and shrinks as the pet rises.
		near := 1 / (1 + p.lift/(700*f))
		w, h := max(8, int(300*f*near)), max(4, int(66*f*near))
		at := [5]int{int(a.feetX()) - w/2, int(a.groundLine()-16*f) - h/2, w, h, 0}
		if focused {
			at[4] = 1
		}
		if p.shadow == nil {
			p.shadow, _ = p.class.open()
		}
		if p.shadow != nil && (at != p.shadowAt || !p.shadow.shown) {
			if p.shadow.draw(at[0], at[1], w, h, shadowSprite(w, h, near, focused)) == nil {
				p.shadowAt = at
			}
		}
		p.shadow.under(a.hwnd)
	} else {
		p.shadow.hide()
	}

	kept := p.effects[:0]
	for _, e := range p.effects {
		age := (now - e.born).Seconds()
		life := .30
		switch {
		case e.text != "":
			life = .85
		case e.hard:
			life = .42
		case e.ring:
			life = .45
		}
		if age >= life {
			e.sprite.close()
			continue
		}
		t := age / life
		if e.text != "" {
			pix, w, h := textSprite(e.text, max(2, int(math.Round(7*f))), true, math.Min(1, (1-t)*2.5))
			_ = e.sprite.draw(int(e.x)-w/2, int(e.y-70*f*t)-h/2, w, h, pix)
		} else if e.ring {
			_ = e.sprite.draw(int(e.x)-e.size/2, int(e.y)-e.size/2, e.size, e.size, ringSprite(e.size, t, [3]float64{1, .86, .45}))
		} else {
			_ = e.sprite.draw(int(e.x)-e.size/2, int(e.y)-e.size/2, e.size, e.size, burstSprite(e.size, t, e.hard))
		}
		kept = append(kept, e)
	}
	for i := len(kept); i < len(p.effects); i++ {
		p.effects[i] = effect{}
	}
	p.effects = kept

	if p.playing() && now < p.hudUntil {
		if p.hud == nil {
			p.hud, _ = p.class.open()
		}
		if p.hud != nil {
			pix, w, h := textSprite(scoreText(p.swarm.Score, p.swarm.Streak), max(2, int(math.Round(6*f))), p.swarm.Streak >= 2, 1)
			_ = p.hud.draw(a.x+a.w/2-w/2, a.y-h-4, w, h, pix)
		}
	} else {
		p.hud.hide()
	}
}

// stepHabits runs the things the pet does unprompted.
func (a *desktopHost) stepHabits(now time.Duration) {
	p := a.play
	if !p.greeted && now > 1500*time.Millisecond {
		p.greeted = true
		a.emote("greet", 4*time.Second)
	}
	if showOnLaunch && now > 3500*time.Millisecond {
		showOnLaunch = false
		a.startShow()
	}
	if tidyOnLaunch && now > 3500*time.Millisecond {
		tidyOnLaunch = false
		a.startTidy()
	}
	if fightOnLaunch && now > 3500*time.Millisecond {
		fightOnLaunch = false
		a.startFight()
		if p.fight != nil {
			p.fight.auto = play.NewBrain(time.Now().UnixNano(), play.Rival{Speed: 1, Aggression: .8, Agility: .3})
		}
	}
	if !p.focusEnd.IsZero() && !time.Now().Before(p.focusEnd) {
		p.focusEnd = time.Time{}
		a.say("专注时间到啦！起来走一走、喝口水吧。")
		a.emote("celebrate", 6*time.Second)
	}
	// Hand on chin while the model is working on a reply.
	if companionService != nil && a.emotions != nil {
		v := companionService.View()
		switch {
		case v.Stage == "waiting" && v.RequestID != p.ponderID:
			p.ponderID = v.RequestID
			p.pondering = a.emote("ponder", 20*time.Second)
		case p.pondering && v.Stage != "waiting":
			p.pondering = false
			if a.emotions.Scheduling().Current == "ponder" {
				a.emotions.ReturnToIdle()
			}
		}
	}
	if hour := time.Now().Hour(); (hour >= 23 || hour < 6) && !p.control && a.quiet() && time.Since(p.lastTouch) > 10*time.Minute && time.Since(p.lastSleepy) > 10*time.Minute {
		p.lastSleepy = time.Now()
		a.emote("sleepy", 8*time.Second)
	}
}

// shaken counts quick reversals while the pet is dragged; enough of them and
// it comes out dizzy.
func (a *desktopHost) shaken(dx int, now time.Duration) {
	p := a.play
	// Ignore the tremor of a steady hand.
	if p == nil || dx > -3 && dx < 3 {
		return
	}
	if now-p.shakeAt > 700*time.Millisecond {
		p.shakes = 0
	}
	dir := 1
	if dx < 0 {
		dir = -1
	}
	if dir != p.shakeDir {
		p.shakeDir, p.shakeAt = dir, now
		p.shakes++
	}
}

func (a *desktopHost) dropped() {
	p := a.play
	if p == nil {
		return
	}
	p.lastTouch = time.Now()
	if p.shakes >= 6 {
		a.emote("dizzy", 5*time.Second)
	}
	p.shakes = 0
}

// motionKey identifies what the rigs would draw right now, so the host
// redraws when it changes and only then.
func (a *desktopHost) motionKey() (bool, uint64) {
	p := a.play
	switch {
	case p == nil:
		return false, 0
	case p.carrying != nil:
		return p.flip, uint64(a.now()/(33*time.Millisecond))<<1 | 1
	case !p.view.Still || p.flash > 0:
		return p.flip, p.seq<<1 | 1 + uint64(p.flash*1000)<<20
	case p.swapPhase != 0 || p.shown != "" && !scene(p.shown):
		// Held poses breathe at thirty frames a second.
		return p.flip, uint64(a.now()/(33*time.Millisecond)) << 1
	}
	return p.flip, 0
}

func (p *playState) texture(key string, frame *image.NRGBA) *rig.Texture {
	if t, ok := p.textures[key]; ok {
		return t
	}
	if len(p.textureOrder) == 8 {
		delete(p.textures, p.textureOrder[0])
		p.textureOrder = p.textureOrder[1:]
	}
	t := rig.NewTexture(frame)
	p.textures[key] = t
	p.textureOrder = append(p.textureOrder, key)
	return t
}

// playFrame returns the body as a rig draws it, or nil when the frame should
// be shown untouched. Clip names the expression frame in body, if it is one.
func (a *desktopHost) playFrame(snap animation.Snapshot, body *image.NRGBA, clip string) (*image.NRGBA, error) {
	p := a.play
	if p == nil {
		return nil, nil
	}
	now := a.now()
	if p.carrying != nil {
		frameIndex := snap.Frame
		if snap.State != animation.Idle {
			frameIndex = 0
		}
		idle, err := a.assets.Bodies.BodyCopy(string(animation.Idle), frameIndex)
		if err != nil {
			return nil, err
		}
		pose := rig.StandPose(p.view.Gait).Add(rig.Settle(p.stand, p.carrySquash))
		frame, err := p.stand.Render(p.texture(fmt.Sprint("idle/", frameIndex), idle), pose)
		if err == nil {
			at := rig.Carry(p.stand, pose)
			holdBox(frame, at.X, at.Y, p.carrying)
		}
		return frame, err
	}
	if snap.State == animation.Idle && !p.view.Still {
		p.shown, p.swapPhase, p.heldBody = "", 0, nil
		if p.view.Stance {
			frame, err := a.assets.Bodies.BodyCopy(string(animation.Fight), 12)
			if err != nil {
				return nil, err
			}
			pose := rig.StrikePose(p.view.Strike, p.view.Power).Add(rig.Settle(p.stance, p.view.Settle))
			body, err := p.stance.Render(p.texture("fight/12", frame), pose)
			if err == nil && p.settings.Equipped != "" {
				_, palm := rig.Arm(p.stance, pose)
				holdWeapon(body, p.settings.Equipped, palm.X, palm.Y, gripAngle(p.view.Strike))
			}
			return body, err
		}
		if clip != "" {
			// An expression frame was still showing on the tick the pet set off.
			idle, err := a.assets.Bodies.BodyCopy(string(animation.Idle), snap.Frame)
			if err != nil {
				return nil, err
			}
			body = idle
		}
		// The idle clip keeps playing under the rig, so the pet still breathes
		// while it walks.
		return p.stand.Render(p.texture(fmt.Sprint("idle/", snap.Frame), body), rig.StandPose(p.view.Gait))
	}

	// A held pose shows one clip; a scene runs through its entry, loop and
	// exit, which all belong to the same expression.
	want := ""
	if e, ok := emotion.Find(a.emotionView.Emotion); ok && clip != "" && (e.Posed && clip == e.ID || e.Scene) {
		want = e.ID
	}
	squash := 0.0
	const down, up = .07, .22
	if p.swapPhase == 0 && want != p.shown {
		p.swapPhase, p.swapSince = 1, now
	}
	if p.swapPhase == 1 {
		if t := (now - p.swapSince).Seconds() / down; t < 1 && p.heldBody != nil {
			// Still sinking: keep showing what was there.
			squash, body = .09*t*t*(3-2*t), p.heldBody
		} else {
			p.shown, p.shownSince, p.swapPhase, p.swapSince = want, now, 2, now
			a.standScene(clip)
		}
	}
	if p.swapPhase == 2 {
		switch t := (now - p.swapSince).Seconds() / up; {
		case want != p.shown:
			p.swapPhase, p.swapSince = 1, now
			if p.heldBody != nil {
				body = p.heldBody
			}
		case t >= 1:
			p.swapPhase = 0
		default:
			ease := math.Min(1, 1.6*t)
			squash = .09 * (1 - ease*ease*(3-2*ease)) * math.Cos(5*t)
		}
	}
	if p.swapPhase != 1 {
		p.heldBody = body
	}
	if p.swapPhase == 0 && p.shown == "" {
		return nil, nil
	}
	pose := rig.Settle(p.figure, squash)
	if e, _ := emotion.Find(p.shown); p.shown == "" || e.Scene {
		// Settling into or out of a pose on an ordinary frame. A scene plays
		// untouched once it has settled; its own footage is the motion.
		if p.swapPhase == 0 {
			return nil, nil
		}
		return p.figure.Render(rig.NewTexture(body), pose)
	}
	// While a pose is on screen, body is its one frame and the gesture keeps
	// it moving, including through the squash that takes it away again.
	if e, ok := emotion.Find(p.shown); ok {
		pose = rig.GesturePose(e.Gesture, (now - p.shownSince).Seconds()).Add(pose)
	}
	return p.figure.Render(p.texture("pose/"+p.shown, body), pose)
}

// playMenu adds control mode's entries to the pet's menu and returns the
// submenus it created, which the caller's menu now owns.
func (a *desktopHost) playMenu(menu, interact uintptr) {
	p := a.play
	if p == nil {
		return
	}
	item := func(parent, id uintptr, label string, checked bool) {
		flags := uintptr(0)
		if checked {
			flags = 8
		}
		text := utf(label)
		_, _ = call(appendMenu, parent, flags, id, uintptr(unsafe.Pointer(text)))
		runtime.KeepAlive(text)
	}
	sub := func(parent uintptr, label string) uintptr {
		handle, _ := call(createMenu)
		if handle == 0 {
			return 0
		}
		text := utf(label)
		if ok, _ := call(appendMenu, parent, 0x10, handle, uintptr(unsafe.Pointer(text))); ok == 0 {
			_, _ = call(destroyMenu, handle)
			handle = 0
		}
		runtime.KeepAlive(text)
		return handle
	}
	_, _ = call(appendMenu, interact, 0x800, 0, 0)
	if faces := sub(interact, "表情"); faces != 0 {
		for i, e := range emotion.Catalog {
			if e.Posed {
				item(faces, uintptr(menuEmote+i), e.Label, false)
			}
		}
	}
	games := sub(menu, "游戏（实验性玩法）")
	if games == 0 {
		return
	}
	label := "操控模式（方向键移动 · J 出掌 · K 跳）"
	if p.hotControl {
		label += "\tCtrl+Alt+G"
	}
	item(games, menuControl, label, p.control)
	label = "召唤到鼠标旁"
	if p.hotSum {
		label += "\tCtrl+Alt+H"
	}
	item(games, menuSummon, label, false)
	item(games, menuShow, "表演一段", p.showing)
	item(games, menuFight, "擂台挑战（打败对手掉落武器）", p.fight != nil)
	a.weaponMenu(games)
	if extras := sub(games, "玩法与实用"); extras != 0 {
		item(extras, menuFlies, "操控时出现苍蝇", p.settings.Flies)
		item(extras, menuStroll, "没事时自己散步", p.settings.Stroll)
		if left := time.Until(p.focusEnd); !p.focusEnd.IsZero() && left > 0 {
			item(extras, menuFocus, fmt.Sprintf("结束专注（还剩 %d 分钟）", int(math.Ceil(left.Minutes()))), true)
		} else {
			item(extras, menuFocus, "专注 25 分钟", false)
		}
	}
}

// playCommand handles a menu choice or a hotkey; it reports whether the
// command was one of control mode's.
func (a *desktopHost) playCommand(command uintptr) bool {
	p := a.play
	if p == nil {
		return false
	}
	switch {
	case command == menuControl || command == hotkeyControl:
		a.setControl(!p.control)
	case command == menuSummon || command == hotkeySummon:
		a.summon()
	case command == hotkeyTalk:
		// Chat from anywhere, whatever the pet is doing.
		if a.chat != nil {
			if err := a.chat.ShowInput(); err != nil {
				_, _ = logDiagnostic(err)
			}
		}
	case command == menuFlies:
		p.settings.Flies = !p.settings.Flies
		p.swarm.Reset()
		p.save()
	case command == menuStroll:
		p.settings.Stroll = !p.settings.Stroll
		p.stroll.Rest()
		p.save()
	case a.fightCommand(command):
	case command == menuShow:
		a.startShow()
	case command == menuFocus:
		if p.focusEnd.IsZero() {
			p.focusEnd = time.Now().Add(focusLength)
			a.say("开始专注 25 分钟，我在旁边安静陪着你。")
			a.emote("focused", 8*time.Second)
		} else {
			p.focusEnd = time.Time{}
			a.say("好，这次专注先到这里。")
		}
	case command >= menuEmote && command < menuEmote+uintptr(len(emotion.Catalog)):
		a.emote(emotion.Catalog[command-menuEmote].ID, 8*time.Second)
	default:
		return false
	}
	p.lastTouch = time.Now()
	return true
}

// startShow begins the routine from wherever the pet stands.
func (a *desktopHost) startShow() {
	p := a.play
	if p == nil || p.showing || a.assets.Engine.Snapshot().State != animation.Idle {
		return
	}
	a.setControl(true)
	p.showing, p.showFrom, p.step = true, a.now(), 0
}

// perform returns the routine's keys for this tick, and ends the show with a
// cheer once the last step has run out.
func (a *desktopHost) perform(now time.Duration) play.Input {
	p := a.play
	at := (now - p.showFrom).Seconds()
	if p.step >= len(routine) || at >= routine[len(routine)-1].at {
		p.showing = false
		a.setControl(false)
		a.emote("cheer", 4*time.Second)
		return play.Input{}
	}
	current := 0
	for i, s := range routine {
		if s.at <= at {
			current = i
		}
	}
	in := routine[current].in
	// Presses fire on the first tick of their step only.
	if current < p.step {
		in.JumpPressed, in.AttackPressed = false, false
	}
	p.step = current + 1
	return in
}

func scene(id string) bool {
	e, ok := emotion.Find(id)
	return ok && e.Scene
}

// standScene keeps the pet's feet where they were as a scene starts or ends.
// A scene can be framed with the pet off to one side, to leave room for a
// prop; its manifest says where the feet stand, and the window slides by the
// difference, out of sight at the bottom of the settle.
func (a *desktopHost) standScene(clip string) {
	p := a.play
	a.x -= p.sceneShift
	p.sceneShift = 0
	m, ok := a.emotionLibrary.Manifest(clip)
	if !ok || m.FeetX == 0 || !scene(a.emotionView.Emotion) {
		return
	}
	idle, feet := feetAnchor, m.FeetX
	if p.flip {
		idle, feet = rig.CanvasWidth-idle, rig.CanvasWidth-feet
	}
	p.sceneShift = int(math.Round((idle - feet) * a.factor()))
	a.x += p.sceneShift
}
