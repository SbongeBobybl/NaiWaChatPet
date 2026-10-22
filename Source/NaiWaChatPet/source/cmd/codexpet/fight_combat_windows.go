//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"math"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"codexpet/reconstruction/animation"
	"codexpet/reconstruction/compositor"
	"codexpet/reconstruction/fightnet"
	"codexpet/reconstruction/play"
	"codexpet/reconstruction/rig"
)

const (
	menuOnline       = 760
	menuGameSettings = 761
	menuExitFight    = 762
	menuReady        = 763
	menuRematch      = 764
	menuReconnect    = 765
)

type dialResult struct {
	client *fightnet.Client
	err    error
	resume bool
}
type combatPresentation struct {
	lanHost                    *fightnet.LANHost
	lanSession                 bool
	reconnectUntil             time.Time
	networkState               string
	networkMatch               uint64
	resultShown                bool
	dialMu                     sync.Mutex
	networkInput               play.Input
	networkDetail              string
	originalScale              float64
	originalDPI                uint32
	windowReduced              bool
	vfx                        *combatVFXCache
	blendFrom                  *image.NRGBA
	blendSince                 time.Duration
	blendID                    uint64
	lastDrawTick               uint64
	statusKey                  [2]string
	clock                      *combatClock
	core                       *play.Combat
	runner                     play.CombatRunner
	snapshot, display          play.CombatSnapshot
	viewport                   play.CombatViewport
	localPlayer                int
	lastEvent                  uint64
	lastRound                  int
	status                     string
	paused, connecting         bool
	client                     *fightnet.Client
	connected                  chan dialResult
	cancel                     context.CancelFunc
	context                    context.Context
	address, room, token, join string
	nextReconnect              time.Time
	sentAt                     time.Duration
	projectiles                map[uint64]*sprite
	lastTrail                  uint64
	frameCache                 map[string]*image.NRGBA
	frameUsed                  map[string]uint64
	frameClock                 uint64
	statusSprites              [2]*sprite
}

func newCombatPresentation(c *play.Combat) *combatPresentation {
	ctx, cancel := context.WithCancel(context.Background())
	return &combatPresentation{core: c, snapshot: c.Snapshot(), context: ctx, cancel: cancel, connected: make(chan dialResult, 1), projectiles: map[uint64]*sprite{}, frameCache: map[string]*image.NRGBA{}}
}
func (f *fightState) closeCombat() {
	if f == nil || f.upgraded == nil {
		return
	}
	u := f.upgraded
	u.clock.close()
	u.cancel()
	u.lanHost.Close()
	u.dialMu.Lock()
	select {
	case result := <-u.connected:
		if result.client != nil {
			result.client.Close()
		}
	default:
	}
	u.dialMu.Unlock()
	if u.vfx != nil {
		<-u.vfx.done
		u.vfx.frames = nil
	}
	if u.client != nil {
		client := u.client
		go func() { _ = client.Send(fightnet.Message{Type: "leave"}); client.Close() }()
	}
	for _, p := range u.projectiles {
		p.close()
	}
	clear(u.projectiles)
	clear(u.frameCache)
	for _, s := range u.statusSprites {
		s.close()
	}
	_, _, _ = combatPlaySound.Call(0, 0, 0)
}
func (a *desktopHost) combatViewport() play.CombatViewport {
	work, ok := workAreaAt(int(a.feetX()), int(a.groundLine())-2, true)
	if !ok {
		return play.CombatViewport{X: 40, Y: 500, Scale: a.factor()}
	}
	scale := a.factor()
	return play.CombatViewport{X: float64(work.Left) + (float64(work.Right-work.Left)-play.ArenaWidth*scale)/2, Y: float64(work.Bottom) - play.ArenaDepth*scale - 8, Scale: scale}
}

func (a *desktopHost) stepCombatPlay(now time.Duration, dt float64, held bool) {
	p := a.play
	f := p.fight
	u := f.upgraded
	a.fitCombatWindow(u)
	u.viewport = a.combatViewport()
	f.hitstop = math.Max(0, f.hitstop-dt)
	p.flash = math.Max(0, p.flash-dt)
	a.jolt = 0
	var in play.Input
	if p.control && a.focused() && !held && (a.chat == nil || !a.chat.inputVisible) {
		in = p.poll()
	} else {
		u.runner.Pending[0] = play.Input{}
		u.networkInput = play.Input{}
	}
	p.jumpEdge, p.attackEdge = false, false
	p.skillEdges = [4]bool{}
	if combatSmokeReport != "" && combatOnlineSmoke && u.client != nil {
		in = onlineProbeInput(u.snapshot, u.localPlayer, uint64(now/(time.Second/60)))
	}
	if u.address != "" {
		a.updateCombatNetwork(now, in)
	} else {
		u.core.SetWeapon(0, p.settings.Equipped)
		if f.auto != nil && u.core != nil {
			u.core.SetAI(0, play.NewCombatAI(37, p.settings.Game.Difficulty))
			f.auto = nil
		}
		if !u.paused && !held {
			u.runner.Advance(u.core, dt, [2]play.Input{in, {}})
		}
		u.snapshot = u.core.Snapshot()
		if u.paused {
			u.status = "PAUSED"
		} else {
			u.status = ""
		}
		for _, e := range f.arena.Events() {
			a.announce(e, now)
		}
	}
	if p.fight == nil {
		return
	}
	s := u.snapshot
	if u.lastRound != s.Round {
		u.lastRound = s.Round
		u.lastEvent = 0
		if f.foe != nil {
			a.dropRival()
		}
		a.spawnRival()
		f.koPet = false
	}
	if f.foe == nil {
		a.spawnRival()
	}
	for _, e := range s.Events {
		if e.ID > u.lastEvent {
			u.lastEvent = e.ID
			a.combatFeedback(e, now)
		}
	}
	// Only the presentation freezes. The core/server continues at 60 Hz.
	if f.hitstop <= 0 || u.display.Tick == 0 {
		u.display = s
	}
	display := u.display
	local := display.Fighters[u.localPlayer]
	other := display.Fighters[1-u.localPlayer]
	if local.Action.ID != u.blendID {
		u.blendID = local.Action.ID
		u.blendSince = now
		u.blendFrom = nil
		if a.currentBody != nil && local.HP > 0 {
			source := copyCombatFrame(a.currentBody)
			if p.flip {
				mirror(source.Pix, source.Stride, source.Rect.Dx(), source.Rect.Dy())
			}
			if source.Rect.Dx() != a.w || source.Rect.Dy() != a.h {
				if scaled, err := compositor.Resize(source, a.w, a.h); err == nil {
					source = image.NewNRGBA(scaled.Rect)
					draw.Draw(source, source.Rect, scaled, image.Point{}, draw.Src)
				}
			}
			u.blendFrom = source
		}
	}
	p.view = local.View
	p.view.Height = local.Height * a.factor()
	p.view.FacingRight = local.Facing > 0
	if local.Action.Skill != play.NoSkill || u.blendFrom != nil && now-u.blendSince < 80*time.Millisecond {
		p.view.Still = false
	}
	x, y := u.viewport.Point(local.X, local.Y)
	a.placeFeet(x, y, p.view.Height, p.view.FacingRight)
	drawTick := u.snapshot.Tick
	if u.address != "" {
		drawTick = uint64(now / (time.Second / 60))
	}
	if drawTick != u.lastDrawTick || p.view.Still != p.drawn.Still || p.flash > 0 {
		u.lastDrawTick = drawTick
		p.seq++
		p.drawn, p.drawnAt = p.view, now
	}
	if f.foe != nil {
		f.foe.override = &other
		f.foe.flash = math.Max(0, f.foe.flash-dt)
		f.foe.down = other.HP <= 0
		f.foe.fade = 0
		a.drawRival(now)
	}
	if s.Fighters[u.localPlayer].Combo >= 2 {
		f.comboText = fmt.Sprintf("%d HITS", s.Fighters[u.localPlayer].Combo)
		f.comboBorn = now
	}
	a.drawCombatProjectiles(now)
	a.drawCombatStatus(now)
	if s.Tick-u.lastTrail >= 4 {
		u.lastTrail = s.Tick
		for _, fighter := range s.Fighters {
			kind := ""
			if fighter.Action.Skill == play.DodgeSkill {
				kind = "dodge"
			}
			if fighter.Action.Skill == play.RollSkill {
				kind = "roll"
			}
			if kind != "" {
				x, y := u.viewport.Point(fighter.X, fighter.Y-fighter.Height-180)
				p.spawn(effect{kind: kind, x: x, y: y, born: now, size: max(30, int(230*a.factor()))})
			}
		}
	}
	a.stepDrops(now, dt)
	a.drawArena(now)
	if f.endAt != 0 && now >= f.endAt {
		a.endFight()
		a.setControl(false)
	}
}

func (a *desktopHost) combatSkillFrame() (*image.NRGBA, bool, error) {
	u := a.play.fight.upgraded
	fighter := u.display.Fighters[u.localPlayer]
	skill := fighter.Action.Skill
	if (skill == play.NoSkill || u.core.Definition(skill).Clip == "") && u.blendFrom != nil && a.now()-u.blendSince < 80*time.Millisecond {
		base, err := a.assets.Bodies.BodyCopy(string(animation.Idle), 0)
		if err != nil {
			return nil, true, err
		}
		target, err := a.play.stand.RenderSized(a.play.texture("idle/0", base), rig.StandPose(a.play.view.Gait), a.w, a.h)
		if err != nil {
			return nil, true, err
		}
		return rig.BlendToExistingPose(u.blendFrom, target, float64(a.now()-u.blendSince)/float64(80*time.Millisecond)), true, nil
	}
	if skill < play.FanSkill || skill > play.DodgeSkill || u.core.Definition(skill).Clip == "" {
		return nil, false, nil
	}
	frame, err := a.cachedCombatSkillFrame(skill, fighter.Action.Age)
	return a.blendCombatSkill(copyCombatFrame(frame)), true, err
}

// Both fighters share immutable skill frames. Evict one old entry instead of
// emptying the entire cache whenever repeated skills fill it.
func (a *desktopHost) cachedCombatSkillFrame(skill play.SkillID, age int) (*image.NRGBA, error) {
	u := a.play.fight.upgraded
	d := u.core.Definition(skill)
	key := fmt.Sprintf("%s/%d/%d/%d/%g/%g", d.Clip, age, a.w, a.h, d.DisplayScale, d.DisplayOffsetY)
	if u.frameUsed == nil {
		u.frameUsed = map[string]uint64{}
	}
	u.frameClock++
	if frame := u.frameCache[key]; frame != nil {
		u.frameUsed[key] = u.frameClock
		return frame, nil
	}
	lib, err := rig.DefaultSkillAnimations()
	if err != nil {
		return nil, err
	}
	scale, offset := d.Presentation()
	frame, err := lib.RenderSkillPlaced(d.Clip, float64(age)/play.CombatHz, min(a.w, rig.CanvasWidth), min(a.h, rig.CanvasHeight), true, scale*combatSkillZoom(skill, age), offset)
	if err == nil {
		if len(u.frameCache) >= 80 {
			oldest, used := "", ^uint64(0)
			for k, tick := range u.frameUsed {
				if tick < used {
					oldest, used = k, tick
				}
			}
			delete(u.frameCache, oldest)
			delete(u.frameUsed, oldest)
		}
		u.frameCache[key] = frame
		u.frameUsed[key] = u.frameClock
	}
	return frame, err
}

func (a *desktopHost) blendCombatSkill(frame *image.NRGBA) *image.NRGBA {
	u := a.play.fight.upgraded
	if u.blendFrom != nil && a.now()-u.blendSince < 80*time.Millisecond {
		return rig.BlendToExistingPose(u.blendFrom, frame, float64(a.now()-u.blendSince)/float64(80*time.Millisecond))
	}
	u.blendFrom = nil
	return frame
}

func copyCombatFrame(frame *image.NRGBA) *image.NRGBA {
	if frame == nil {
		return nil
	}
	out := image.NewNRGBA(frame.Rect)
	copy(out.Pix, frame.Pix)
	return out
}

func (a *desktopHost) drawCombatStatus(now time.Duration) {
	u := a.play.fight.upgraded
	for i, f := range u.snapshot.Fighters {
		protected := u.snapshot.Tick < f.ProtectionUntil
		if f.HP <= 0 || f.BurnLeft == 0 && u.snapshot.Tick >= f.StunUntil && f.BuffStacks == 0 && !protected {
			u.statusSprites[i].hide()
			continue
		}
		s := u.statusSprites[i]
		if s == nil {
			s, _ = a.play.class.open()
			u.statusSprites[i] = s
		}
		if s == nil {
			continue
		}
		size := max(40, int(500*a.factor()))
		x, y := u.viewport.Point(f.X, f.Y-f.Height-245)
		key := fmt.Sprintf("%d/%d/%t/%t/%d/%t", size, u.snapshot.Tick/4, f.BurnLeft > 0, u.snapshot.Tick < f.StunUntil, f.BuffStacks, protected)
		if u.statusKey[i] == key && s.shown {
			_, _ = call(setWindowPos, s.hwnd, 0, uintptr(int(x)-size/2), uintptr(int(y)-size/2), 0, 0, 0x0001|0x0004|0x0010)
			continue
		}
		u.statusKey[i] = key
		pix := combatStatusSprite(size, float64(u.snapshot.Tick)/60, f.BurnLeft > 0, u.snapshot.Tick < f.StunUntil, f.BuffStacks, protected)
		_ = s.draw(int(x)-size/2, int(y)-size/2, size, size, pix)
	}
}
func (a *desktopHost) combatFeedback(e play.CombatEvent, now time.Duration) {
	p, f := a.play, a.play.fight
	u := f.upgraded
	x, y := u.viewport.Point(e.X, e.Y)
	size := max(28, int(280*a.factor()))
	switch e.Kind {
	case "combo_guard":
		p.spawn(effect{x: x, y: y, born: now, text: "GUARD"})
	case "hit", "burn":
		p.spawn(effect{x: x, y: y, born: now, text: fmt.Sprintf("%.1f", e.Amount)})
		if e.Kind == "hit" {
			if e.Skill != play.FanSkill {
				p.spawn(effect{x: x, y: y, born: now, hard: e.Hard, size: size})
			}
			if e.Target == u.localPlayer {
				p.flash = .15
			} else if f.foe != nil {
				f.foe.flash = .15
			}
			f.hitstop = .045
			if e.Hard {
				f.hitstop = .075
			}
			a.combatSound("hit")
			if e.Skill == play.RollSkill {
				p.spawn(effect{kind: "rollhit", x: x, y: y, born: now, size: size * 2})
				a.combatSound("roll")
			}
		}
	case "explosion":
		p.spawn(effect{kind: "explosion", x: x, y: y, born: now, size: size * 2})
		a.combatSound("explosion")
	case "power":
		p.spawn(effect{kind: "power", x: x, y: y, born: now, size: size * 2})
		if e.Amount > 0 {
			p.spawn(effect{x: x, y: y, born: now, text: fmt.Sprintf("+%.1f", e.Amount)})
		}
		a.combatSound("power")
	case "skill_start":
		if e.Skill == play.DodgeSkill {
			a.combatSound("dodge")
		}
	case "ko":
		text := "YOU WIN"
		if e.Target == u.localPlayer {
			text = "YOU LOSE"
		}
		a.banner(text, fireBanner, now, 2300*time.Millisecond)
		f.hitstop = .12
		if u.address == "" && e.Target == 1 {
			if drop, ok := play.RollDrop(f.rng, f.arena.Round, f.arena.Rival.Boss); ok {
				dx, dy := u.viewport.Point(e.X, e.Y)
				f.drops = append(f.drops, &dropState{weapon: drop, x: dx, y: dy, z: 150 * a.factor(), vz: 500 * a.factor(), born: now})
			}
		}
	}
}
func (a *desktopHost) drawCombatProjectiles(now time.Duration) {
	u := a.play.fight.upgraded
	seen := map[uint64]bool{}
	for _, p := range u.snapshot.Projectiles {
		seen[p.ID] = true
		s := u.projectiles[p.ID]
		if s == nil && len(u.projectiles) < 8 {
			s, _ = a.play.class.open()
			u.projectiles[p.ID] = s
		}
		if s == nil {
			continue
		}
		x, y := u.viewport.Point(p.X, p.Y)
		size := max(24, int(150*a.factor()))
		_ = s.draw(int(x)-size/2, int(y)-size/2, size, size, combatFanSprite(size, float64(u.snapshot.Tick)*.22))
	}
	for id, s := range u.projectiles {
		if !seen[id] {
			s.close()
			delete(u.projectiles, id)
		}
	}
}

func (a *desktopHost) beginOnline(join string) {
	if a.play == nil {
		return
	}
	a.beginOnlineAt(join, a.play.settings.Game.Server)
}

func (a *desktopHost) prepareOnline() *combatPresentation {
	if a.play == nil {
		return nil
	}
	a.endFight()
	a.startFight()
	if a.play.fight == nil {
		return nil
	}
	f := a.play.fight
	u := f.upgraded
	core, err := play.NewConfiguredCombat(false, &u.core.Rules)
	if err != nil {
		a.endFight()
		a.combatStartError(err)
		return nil
	}
	u.core = core
	f.arena = u.core.Arena
	u.snapshot = u.core.Snapshot()
	return u
}

func (a *desktopHost) beginOnlineAt(join, address string) {
	u := a.prepareOnline()
	if u == nil {
		return
	}
	u.join = join
	u.address = address
	u.status = "CONNECTING"
	u.networkDetail = "正在连接 " + u.address + "…"
	u.startDial(false)
	a.openGamePanel()
}
func (u *combatPresentation) startDial(resume bool) {
	if u.connecting {
		return
	}
	u.connecting = true
	go func() {
		client, err := fightnet.Dial(u.context, u.address)
		u.dialMu.Lock()
		defer u.dialMu.Unlock()
		if u.context.Err() != nil {
			if client != nil {
				client.Close()
			}
			return
		}
		select {
		case <-u.context.Done():
			if client != nil {
				client.Close()
			}
		case u.connected <- dialResult{client, err, resume}:
		}
	}()
}
func (a *desktopHost) updateCombatNetwork(now time.Duration, in play.Input) {
	p := a.play
	f := p.fight
	u := f.upgraded
	in.MergeEdges(u.networkInput)
	u.networkInput = in
	select {
	case result := <-u.connected:
		u.connecting = false
		if result.err != nil {
			u.status = "CONNECT FAILED"
			u.networkDetail = u.connectionError(result.err)
			u.nextReconnect = time.Now().Add(2 * time.Second)
			break
		}
		u.client = result.client
		u.networkDetail = "已连接，正在处理房间请求…"
		message := fightnet.Message{Type: "create"}
		if result.resume {
			message = fightnet.Message{Type: "resume", Room: u.room, Token: u.token}
		} else if u.join != "" {
			message = fightnet.Message{Type: "join", Room: u.join}
		}
		client := u.client
		go func() {
			if client.Send(message) != nil {
				client.Close()
			}
		}()
	default:
	}
	if u.client == nil {
		if !u.reconnectUntil.IsZero() && time.Now().After(u.reconnectUntil) {
			u.token = ""
			u.status = "ROOM CLOSED"
			u.networkDetail = "未能恢复连接，房主可能已离开。可在房间列表重新加入。"
			u.reconnectUntil = time.Time{}
		}
		if u.token != "" && !u.connecting && time.Now().After(u.nextReconnect) {
			u.startDial(true)
		}
		return
	}
	// Process at most the buffered updates per frame. Network traffic must not
	// keep the UI inside this loop while keyboard messages are waiting.
	for pending := len(u.client.Updates()); pending > 0; pending-- {
		select {
		case update := <-u.client.Updates():
			if update.Match > u.networkMatch {
				u.networkMatch = update.Match
				u.resultShown = false
				f.bannerText = ""
				u.lastEvent, u.lastTrail = 0, 0
				u.display = play.CombatSnapshot{}
				f.hitstop = 0
			}
			if update.Room != "" {
				u.room = update.Room
			}
			if update.Token != "" {
				u.token = update.Token
			}
			if update.Player >= 0 && update.Player < 2 {
				u.localPlayer = update.Player
			}
			if update.Type == "error" {
				u.status = update.Error
				u.networkDetail = combatProtocolError(update.Error)
				continue
			}
			if update.Type == "left" {
				u.token = ""
				u.room = ""
				u.networkState = "closed"
				u.reconnectUntil = time.Time{}
				u.status = "ROOM CLOSED"
				u.networkDetail = "房间已关闭，请重新创建或加入。"
				continue
			}
			if update.Type == "welcome" {
				u.room, u.token, u.localPlayer = update.Room, update.Token, update.Player
				u.reconnectUntil = time.Time{}
			}
			if update.State != "" {
				u.status = "ROOM " + u.room + " " + update.State
				u.networkDetail = ""
				if update.State == "fighting" && u.networkState != "fighting" && p.gamePanel != nil {
					p.gamePanel.resume()
				}
				u.networkState = update.State
			}
			if update.State == "countdown" {
				u.status = fmt.Sprintf("ROOM %s START %d", u.room, (update.Countdown+59)/60)
				if update.Snapshot != nil && update.Snapshot.Tick == 0 {
					u.lastEvent = 0
					u.display = play.CombatSnapshot{}
				}
			}
			if update.Snapshot != nil {
				if update.Snapshot.Rules != nil {
					u.core.Rules = *update.Snapshot.Rules
				}
				u.snapshot = *update.Snapshot
				f.arena.PetHP = update.Snapshot.Fighters[u.localPlayer].HP
				f.arena.FoeHP = update.Snapshot.Fighters[1-u.localPlayer].HP
				f.arena.Clock = update.Snapshot.Clock
				f.arena.Phase = update.Snapshot.Phase
				f.arena.Combo = update.Snapshot.Fighters[u.localPlayer].Combo
			}
		default:
			goto drained
		}
	}
drained:
	select {
	case <-u.client.Done():
		err := u.client.Error()
		u.client = nil
		u.status = "DISCONNECTED - RECONNECTING"
		u.networkDetail = u.connectionError(err)
		if u.token != "" {
			u.networkDetail = "连接已中断，正在重连。\n" + u.networkDetail
			if u.reconnectUntil.IsZero() {
				u.reconnectUntil = time.Now().Add(30 * time.Second)
			}
		}
		u.nextReconnect = time.Now().Add(time.Second)
		return
	default:
	}
	if u.client.State().State != "fighting" {
		u.networkInput.ClearEdges()
	} else if now-u.sentAt >= 16*time.Millisecond {
		u.sentAt = now
		if u.client.SendInput(u.networkInput) == nil {
			u.networkInput.ClearEdges()
		}
	}
	if snapshot := u.client.DisplaySnapshot(time.Now(), in); snapshot != nil {
		u.snapshot = *snapshot
	}
	if u.networkState == "over" {
		_, result := combatResult(u.snapshot, u.localPlayer)
		if result != "" {
			u.status = result
			if !u.resultShown {
				u.resultShown = true
				style := goldBanner
				if u.snapshot.Winner != u.localPlayer {
					style = iceBanner
				}
				a.banner(result, style, now, 3*time.Second)
				// Apply both final state and snapshot before the lobby first paints.
				a.openGamePanel()
			}
		}
	}
}

func (u *combatPresentation) connectionError(err error) string {
	if u.lanSession {
		return "未能连接到房间。请确认房主仍在游戏中，双方在同一网络。\n房主可点“联机权限”检查当前网络；热点也可能属于公用网络。"
	}
	return combatConnectionError(err, u.address)
}

func combatConnectionError(err error, address string) string {
	detail := "连接已关闭"
	if err != nil {
		detail = err.Error()
	}
	if errors.Is(err, syscall.Errno(10061)) || strings.Contains(detail, "10061") || strings.Contains(detail, "refused") {
		return "连接被拒绝：该地址没有可用的独立服务器。\n局域网联机请返回默认页面并点击创建房间。"
	}
	if strings.Contains(detail, "certificate") || strings.Contains(detail, "tls:") {
		return "TLS 连接失败，请检查证书及服务器域名：" + detail
	}
	if strings.Contains(detail, "protocol version mismatch") {
		return "服务端与客户端协议版本不一致，请使用同一发行包中的服务端。"
	}
	if strings.Contains(detail, "not a NaiWa fight server") {
		return "该端口不是奶蛙房间服务器，请检查服务端程序和端口。"
	}
	if strings.Contains(detail, "timeout") || strings.Contains(detail, "deadline") {
		return "连接超时：请核对服务器地址、端口和服务器防火墙。\n" + address
	}
	return "连接失败：" + detail
}

func combatProtocolError(code string) string {
	translations := map[string]string{
		"room_not_found": "房间不存在或已关闭，请确认双方使用同一服务器和房间代码。",
		"room_full":      "房间已满，请创建新房间。", "version_mismatch": "协议版本不一致，请双方更新客户端和服务端。",
		"not_in_room": "尚未进入房间，请先创建或加入。", "invalid_resume": "重连凭证失效，请重新创建或加入房间。",
	}
	if text := translations[code]; text != "" {
		return text
	}
	return "服务器返回：" + code
}

func (a *desktopHost) combatStartError(err error) {
	_, _ = logDiagnostic(err)
	a.openGamePanel()
	if a.play.gamePanel != nil {
		a.play.gamePanel.statusError = err.Error()
		a.play.gamePanel.refresh()
	}
}
func (a *desktopHost) combatMenu(parent uintptr) {
	for _, item := range []struct {
		id    uintptr
		label string
	}{{menuOnline, "联机大厅"}, {menuGameSettings, "游戏设置"}} {
		label := utf(item.label)
		_, _ = call(appendMenu, parent, 0, item.id, uintptr(unsafe.Pointer(label)))
		runtime.KeepAlive(label)
	}
	if a.play.fight != nil {
		_, _ = call(appendMenu, parent, 0x800, 0, 0)
		label := utf("结束对局，返回桌宠")
		_, _ = call(appendMenu, parent, 0, menuExitFight, uintptr(unsafe.Pointer(label)))
		runtime.KeepAlive(label)
	}
}
func (a *desktopHost) combatCommand(command uintptr) bool {
	switch command {
	case menuOnline:
		a.openGamePanel()
		if a.play.gamePanel != nil {
			a.play.gamePanel.switchLobbyPage(0)
		}
	case menuGameSettings:
		a.openGamePanel()
		if a.play.gamePanel != nil {
			a.play.gamePanel.switchLobbyPage(1)
		}
	case menuExitFight:
		a.endFight()
		a.setControl(false)
	case menuReady, menuRematch:
		if a.play.fight != nil && a.play.fight.upgraded.client != nil {
			kind := "ready"
			if command == menuRematch {
				kind = "rematch"
			}
			go a.play.fight.upgraded.client.Send(fightnet.Message{Type: kind})
		}
	case menuReconnect:
		if a.play.fight != nil {
			a.play.fight.upgraded.startDial(true)
		}
	default:
		return false
	}
	return true
}

func combatFanSprite(size int, angle float64) []byte {
	p := newPainter(size, size)
	c := float64(size) / 2
	for i := 0; i < 7; i++ {
		theta := angle + float64(i)*.45
		cover := ellipse(c+math.Cos(theta)*c*.18, c+math.Sin(theta)*c*.18, c*.60, c*.17, theta, .9)
		p.shade(.4, .95, .78, func(x, y float64) float64 { return .9 * cover(x, y) })
	}
	p.shade(1, .98, .68, ellipse(c, c, c*.12, c*.12, 0, 1))
	return p.bgra()
}
