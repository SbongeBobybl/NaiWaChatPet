//go:build windows

package main

import (
	"fmt"
	"math"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const lobbyWidth, lobbyHeight = 900, 560
const lobbyReflowMessage = 0x8000 + 107

// Header navigation and two-column room layout on an eight-pixel grid.
const (
	lobbyRailWidth                  = 80
	lobbyContentX, lobbyContentW    = 32, 836
	lobbyBodyY, lobbyBodyH          = 120, 384
	lobbyBodyEndY                   = lobbyBodyY + lobbyBodyH
	lobbyFooterY, lobbyFooterH      = 518, 26
	lobbyLeftX, lobbyLeftW          = 32, 516
	lobbyRightX, lobbyRightW        = 584, 284
	lobbyNavX, lobbyNavW, lobbyNavH = 286, 84, 36
	lobbyNavTop, lobbyNavStep       = 34, 0
	lobbyPondHeight                 = 88
)

// lobbyControl records a native child window together with the layout slot it
// belongs to, so page switches can reposition and show it in one pass.
type lobbyControl struct {
	window, id              uintptr
	x, y, w, h, pages, font int
}

type lobbyButton struct {
	role           string
	hover, pressed bool
	motion, press  float64
}

type lobbyStyle struct {
	building                   bool
	page                       int
	scale                      float64
	fonts                      map[int]uintptr
	brushes                    map[uint32]uintptr
	buttons                    map[uintptr]*lobbyButton
	enabled, shown             map[uintptr]bool
	controls                   []lobbyControl
	byID                       map[uintptr]uintptr
	chrome                     map[int]map[string]*lobbyLayer
	cache                      map[string]*lobbySurface
	navTargetY                 int
	bindAction                 string
	lastState                  string
	room, phase                string
	result                     string
	winner                     int
	ready, connected           [2]bool
	localPlayer, countdown     int
	connecting, online, active bool
	lastStatus                 string
}

func (g *gamePanel) px(v int) int { return int(math.Round(float64(v) * g.skin.scale)) }

func (g *gamePanel) buildLobby() error {
	g.skin = &lobbyStyle{scale: 1, fonts: map[int]uintptr{}, brushes: map[uint32]uintptr{}, buttons: map[uintptr]*lobbyButton{}, enabled: map[uintptr]bool{}, shown: map[uintptr]bool{}, byID: map[uintptr]uintptr{}, chrome: map[int]map[string]*lobbyLayer{}, cache: map[string]*lobbySurface{}, building: true}
	work, ok := workAreaAt(int(g.owner.feetX()), int(g.owner.groundLine())-2, true)
	if !ok {
		work = rectWin{0, 0, 1280, 720}
	}
	dpi := g.owner.dpi
	if dpi < 48 {
		dpi = 96
	}
	g.skin.scale = max(.65, min(float64(dpi)/96, float64(work.Right-work.Left-24)/(lobbyWidth+20), float64(work.Bottom-work.Top-24)/(lobbyHeight+44)))
	g.setLobbyFonts()
	classes := struct{ Size, Classes uint32 }{8, 4}
	if ok, err := call(chatControls.NewProc("InitCommonControlsEx"), uintptr(unsafe.Pointer(&classes))); ok == 0 {
		return failure("InitCommonControlsEx", err)
	}
	var firstError error
	control := func(class, text string, id uintptr, x, y, w, h, pages, font int, style uintptr) uintptr {
		if firstError != nil {
			return 0
		}
		c, t := utf(class), utf(text)
		window, err := call(createWindow, 0, uintptr(unsafe.Pointer(c)), uintptr(unsafe.Pointer(t)), 0x50000000|style, uintptr(g.px(x)), uintptr(g.px(y)), uintptr(g.px(w)), uintptr(g.px(h)), g.hwnd, id, g.owner.play.class.module, 0)
		runtime.KeepAlive(c)
		runtime.KeepAlive(t)
		if window == 0 {
			firstError = fmt.Errorf("control %d (%s): %w", id, class, failure("CreateWindowExW", err))
			return 0
		}
		g.skin.controls = append(g.skin.controls, lobbyControl{window, id, x, y, w, h, pages, font})
		g.skin.byID[id] = window
		call(chatSend, window, 0x30, g.skin.fonts[font], 1)
		return window
	}
	button := func(text string, id uintptr, x, y, w, h, pages int, role string) uintptr {
		window := control("BUTTON", text, id, x, y, w, h, pages, lobbyBody2, 0x10000|11)
		if window != 0 {
			g.skin.buttons[window] = &lobbyButton{role: role}
			call(chatSubclass, window, syscall.NewCallback(lobbyButtonProc), 7, 0)
		}
		return window
	}
	button("联机", 30, 286, 34, 84, 36, 0, "nav")
	button("设置", 31, 386, 34, 84, 36, 0, "nav")
	g.manualButton = button("高级", 18, 486, 34, 84, 36, 0, "nav")
	button("单机挑战", 14, 674, 34, 96, 36, 0, "secondary")
	button("桌宠", 16, 786, 34, 82, 36, 0, "quiet")
	button("继续对战", 15, 724, 518, 144, 32, 0, "secondary")
	g.roomList = control("LISTBOX", "房间列表", 3, 32, 166, 516, 252, 1, lobbyBody2, 0x00210151)
	call(chatSend, g.roomList, 0x01A0, 0, uintptr(g.px(58)))
	if g.roomList != 0 {
		call(chatSubclass, g.roomList, syscall.NewCallback(lobbyListProc), 8, 0)
	}
	g.code = control("EDIT", "", 2, 42, 462, 180, 28, 5, lobbyBody2, 0x10080)
	call(chatSend, g.code, 0x00C5, 6, 0)
	placeholder := utf("6 位房间码")
	call(chatSend, g.code, 0x1501, 0, uintptr(unsafe.Pointer(placeholder)))
	runtime.KeepAlive(placeholder)
	g.server = control("EDIT", g.owner.play.settings.Game.Server, 1, 42, 174, 496, 28, 4, lobbyBody2, 0x10080)
	call(chatSend, g.server, 0x00C5, 240, 0)
	g.probeButton = button("刷新", 17, 444, 120, 104, 30, 5, "quiet")
	button("加入房间", 11, 248, 454, 300, 40, 5, "primary")
	button("创建房间", 10, 584, 350, 284, 44, 5, "primary")
	button("准备", 12, 584, 350, 284, 44, 5, "primary")
	button("再来一局", 13, 584, 350, 284, 44, 5, "primary")
	button("复制房间码", 23, 584, 406, 136, 34, 5, "quiet")
	button("离开", 24, 732, 406, 136, 34, 5, "quiet")
	button("网络权限", 19, 584, 458, 284, 34, 5, "quiet")
	g.difficulty = button("普通", 20, 32, 180, 236, 40, 2, "secondary")
	g.volume = control("msctls_trackbar32", "音量", 21, 312, 184, 236, 30, 2, lobbyBody2, 0x10000)
	g.effects = control("msctls_trackbar32", "特效", 22, 592, 184, 276, 30, 2, lobbyBody2, 0x10000)
	for _, slider := range []uintptr{g.volume, g.effects} {
		call(chatSend, slider, 0x0406, 1, 100<<16)
	}
	call(chatSend, g.effects, 0x0406, 1, (100<<16)|25)
	call(chatSend, g.volume, 0x0405, 1, uintptr(math.Round(g.owner.play.settings.Game.Volume*100)))
	call(chatSend, g.effects, 0x0405, 1, uintptr(math.Round(g.owner.play.settings.Game.Effects*100)))
	button("恢复默认", 25, 732, 272, 136, 32, 2, "quiet")
	actions := []struct{ id, name string }{{"left", "左移"}, {"right", "右移"}, {"up", "纵深上"}, {"down", "纵深下"}, {"run", "奔跑"}, {"attack", "普攻"}, {"jump", "跳跃"}, {"fan", "飞扇"}, {"roll", "翻滚"}, {"power", "强化"}, {"dodge", "闪避"}}
	for i, action := range actions {
		id := uintptr(100 + i)
		g.keys[id] = action.id
		g.keyNames[action.id] = action.name
		g.keyHandles[id] = button(fmt.Sprintf("%s   %s", action.name, keyLabel(g.owner.play.settings.Game.Keys[action.id])), id, 32+i%4*208, 330+i/4*52, 200, 40, 2, "key")
	}
	g.status = control("STATIC", "", 4, 32, 526, 672, 20, 0, lobbyCap, 0)
	if firstError != nil {
		return firstError
	}
	r := rectWin{0, 0, int32(g.px(lobbyWidth)), int32(g.px(lobbyHeight))}
	call(userDLL.NewProc("AdjustWindowRectEx"), uintptr(unsafe.Pointer(&r)), 0x02CA0000, 0, 0)
	w, h := int(r.Right-r.Left), int(r.Bottom-r.Top)
	x, y := int(work.Left)+(int(work.Right-work.Left)-w)/2, int(work.Top)+(int(work.Bottom-work.Top)-h)/2
	call(setWindowPos, g.hwnd, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0x0014)
	g.frame = newLobbySurface(g.px(lobbyWidth), g.px(lobbyHeight))
	if g.frame == nil {
		return fmt.Errorf("lobby frame allocation failed")
	}
	g.ensureAnim()
	g.updateLANList(nil)
	g.skin.building = false
	g.switchLobbyPage(0)
	g.startLobbyClock()
	return nil
}

func (g *gamePanel) setLobbyFonts() {
	face := utf("Microsoft YaHei UI")
	sizes := []int{lobbyMicro, lobbyLabel, lobbyCap, lobbyBody, lobbyBody2, lobbySec, lobbyNum, lobbyHero, lobbyTitle}
	for _, size := range sizes {
		weight := 400
		if size >= lobbySec {
			weight = 600
		}
		height := -g.px(size)
		font, _ := call(chatCreateFont, uintptr(height), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(face)))
		g.skin.fonts[size] = font
	}
	runtime.KeepAlive(face)
}

func (g *gamePanel) navYForPage(page int) int {
	switch page {
	case 1:
		return lobbyNavTop + lobbyNavStep
	case 2:
		return lobbyNavTop + lobbyNavStep*2
	}
	return lobbyNavTop
}

func (g *gamePanel) switchLobbyPage(page int) {
	if g.skin == nil {
		return
	}
	g.skin.page = page
	g.skin.navTargetY = g.navYForPage(page)
	g.manual = page == 2
	g.skin.bindAction = ""
	if g.anim != nil {
		g.anim.pageStart = g.lobbyNow()
		g.anim.page = page
	}
	if page != 0 && g.searchCancel != nil {
		g.searchCancel()
		g.searchVersion++
		g.searching = false
		g.joinAfterSearch = false
	}
	if page == 0 {
		g.nextSearch = time.Time{}
	}
	for _, c := range g.skin.controls {
		x, y, w, h := c.x, c.y, c.w, c.h
		if page == 2 {
			switch c.id {
			case 2: // the room code moves under the server field
				x, y, w, h = 42, 270, 188, 28
			case 11: // join with server + code
				x, y, w, h = 32, 352, 516, 42
			case 17: // probe becomes "test connection"
				x, y, w, h = 264, 264, 284, 34
			}
		}
		call(setWindowPos, c.window, 0, uintptr(g.px(x)), uintptr(g.px(y)), uintptr(g.px(w)), uintptr(g.px(h)), 0x0014)
		show := uintptr(0)
		if c.pages == 0 || c.pages&(1<<page) != 0 {
			show = 5
		}
		call(showWindow, c.window, show)
		if show != 0 {
			call(skinInvalidate, c.window, 0, 0)
		}
	}
	clear(g.skin.shown)
	for _, id := range []uintptr{18, 30, 31} {
		call(skinInvalidate, g.skin.byID[id], 0, 0)
	}
	g.skin.lastState = ""
	g.refresh()
	g.startLobbyClock()
	g.noteLobbyInput()
	call(skinInvalidate, g.hwnd, 0, 0)
}

func (g *gamePanel) refreshLobbyState(status string) {
	s := g.skin
	if s == nil {
		return
	}
	s.active, s.online, s.connecting = false, false, false
	s.room, s.phase = "", ""
	s.result, s.winner = "", -1
	s.ready, s.connected = [2]bool{}, [2]bool{}
	s.countdown = 0
	if f := g.owner.play.fight; f != nil && f.upgraded != nil {
		u := f.upgraded
		s.active = true
		s.online = u.address != ""
		s.connecting = u.connecting
		s.room = u.room
		s.phase = u.networkState
		s.localPlayer = u.localPlayer
		if s.online && s.phase == "over" {
			s.result, _ = combatResult(u.snapshot, u.localPlayer)
			s.winner = u.snapshot.Winner
		}
		if u.client != nil {
			state := u.client.State()
			s.ready, s.connected, s.countdown = state.Ready, state.Connected, state.Countdown
		}
	}
	if s.bindAction != "" && g.statusError == "" {
		status = "设置\u201c" + g.keyNames[s.bindAction] + "\u201d：按新键 · Esc 取消"
	}
	if status == "" {
		status = "创建或加入房间"
	}
	if s.page == 1 && s.bindAction == "" && g.statusError == "" {
		status = ""
	}
	g.updateLabel(g.status, status)
	show := func(id uintptr, visible bool) {
		if s.byID[id] == 0 {
			return
		}
		visible = visible && s.page != 1
		if old, ok := s.shown[id]; ok && old == visible {
			return
		}
		s.shown[id] = visible
		v := uintptr(0)
		if visible {
			v = 5
		}
		call(showWindow, s.byID[id], v)
		if visible {
			call(skinInvalidate, s.byID[id], 0, 0)
		}
	}
	enable := func(id uintptr, ok bool) {
		if s.byID[id] == 0 {
			return
		}
		if old, exists := s.enabled[id]; exists && old == ok {
			return
		}
		s.enabled[id] = ok
		v := uintptr(0)
		if ok {
			v = 1
		}
		call(userDLL.NewProc("EnableWindow"), s.byID[id], v)
		call(skinInvalidate, s.byID[id], 0, 0)
	}
	show(3, s.page == 0 && len(g.rooms) > 0)
	show(10, s.room == "")
	show(12, s.room != "" && s.phase != "over")
	show(13, s.room != "" && s.phase == "over")
	show(23, s.room != "")
	show(24, s.online)
	busy := s.connecting || s.room != ""
	enable(10, !busy)
	enable(11, !busy && (len(g.rooms) > 0 || validLobbyCode(stringsTrim(controlText(g.code)))))
	enable(12, s.room != "" && s.phase == "waiting" && !s.ready[s.localPlayer])
	enable(13, s.phase == "over" && !s.ready[s.localPlayer])
	enable(23, s.room != "")
	enable(24, s.online)
	enable(15, s.active && (!s.online || s.phase == "fighting"))
	enable(17, !g.searching && !g.probing)
	ready := "准备"
	if s.ready[s.localPlayer] {
		ready = "等待好友"
	}
	if s.phase == "countdown" {
		ready = fmt.Sprintf("%d 秒后开战", (s.countdown+59)/60)
	}
	g.updateLabel(s.byID[12], ready)
	rematch := "再来一局"
	if s.ready[s.localPlayer] {
		rematch = "等待好友"
	}
	g.updateLabel(s.byID[13], rematch)
	if s.connecting {
		g.updateLabel(s.byID[10], "连接中…")
	} else {
		g.updateLabel(s.byID[10], "创建房间")
	}
	for id, action := range g.keys {
		label := fmt.Sprintf("%s   %s", g.keyNames[action], keyLabel(g.owner.play.settings.Game.Keys[action]))
		if s.bindAction == action {
			label = "请按新键…"
		}
		g.updateLabel(g.keyHandles[id], label)
	}
	key := fmt.Sprint(s.page, s.room, s.phase, s.ready, s.connected, s.localPlayer, s.countdown/60, s.connecting, g.searching, len(g.rooms), g.owner.play.settings.Game.Volume, g.owner.play.settings.Game.Effects, s.bindAction, s.result, s.winner, status)
	if key != s.lastState {
		s.lastState = key
		call(skinInvalidate, g.hwnd, 0, 0)
	}
}

func stringsTrim(s string) string { return strings.TrimSpace(s) }

func (g *gamePanel) lobbyMessage(m *messageWin) bool {
	if g == nil || !g.visible || g.skin == nil {
		return false
	}
	if m.Window != g.hwnd {
		child, _ := call(userDLL.NewProc("IsChild"), g.hwnd, m.Window)
		if child == 0 {
			return false
		}
	}
	g.noteLobbyInput()
	if m.Message == 0x0100 && g.skin.bindAction != "" {
		if m.LParam&(1<<30) != 0 {
			return true
		}
		if m.WParam == 27 {
			g.skin.bindAction = ""
			g.statusError = "已取消"
		} else {
			err := g.owner.play.settings.Game.Bind(g.skin.bindAction, uint16(m.WParam))
			if err != nil {
				g.statusError = "按键已占用，请换一个"
			} else {
				g.skin.bindAction = ""
				g.owner.play.save()
				g.statusError = "已保存"
			}
		}
		g.refresh()
		return true
	}
	result, _ := call(userDLL.NewProc("IsDialogMessageW"), g.hwnd, uintptr(unsafe.Pointer(m)))
	return result != 0
}

func (g *gamePanel) changeLobbySlider(window uintptr, code uintptr) {
	g.statusError = ""
	value, _ := call(chatSend, window, 0x0400, 0, 0)
	if window == g.volume {
		g.owner.play.settings.Game.Volume = float64(value) / 100
	}
	if window == g.effects {
		g.owner.play.settings.Game.Effects = float64(value) / 100
	}
	if code != 5 {
		g.owner.play.save()
	}
	g.refresh()
}

func (g *gamePanel) releaseLobby() {
	if g.skin == nil {
		return
	}
	g.stopLobbyClock()
	g.frame.close()
	g.frame = nil
	for _, font := range g.skin.fonts {
		call(chatDeleteObject, font)
	}
	for _, brush := range g.skin.brushes {
		call(chatDeleteObject, brush)
	}
	for _, page := range g.skin.chrome {
		for _, layer := range page {
			if layer != nil {
				layer.surface.close()
			}
		}
	}
	for _, surface := range g.skin.cache {
		surface.close()
	}
	g.releasePonds()
	g.skin = nil
	g.anim = nil
}

func (g *gamePanel) reflowLobby(dpi uint32, x, y int) {
	if g.skin == nil || g.skin.building || g.reflowing || g.painting || dpi < 48 || dpi > 768 {
		return
	}
	g.reflowing = true
	defer func() { g.reflowing = false }()
	work, ok := workAreaAt(x, y, true)
	if !ok {
		return
	}
	s := g.skin
	scale := max(.65, min(float64(dpi)/96, float64(work.Right-work.Left-24)/(lobbyWidth+20), float64(work.Bottom-work.Top-24)/(lobbyHeight+44)))
	if math.Abs(scale-s.scale) < .001 {
		return
	}
	s.scale = scale
	g.frame.close()
	g.frame = nil
	oldFonts := s.fonts
	s.fonts = map[int]uintptr{}
	g.setLobbyFonts()
	for _, control := range s.controls {
		call(chatSend, control.window, 0x30, s.fonts[control.font], 1)
	}
	for _, font := range oldFonts {
		call(chatDeleteObject, font)
	}
	for _, page := range s.chrome {
		for _, layer := range page {
			if layer != nil {
				layer.surface.close()
			}
		}
	}
	s.chrome = map[int]map[string]*lobbyLayer{}
	for _, surface := range s.cache {
		surface.close()
	}
	s.cache = map[string]*lobbySurface{}
	g.releasePonds()
	call(chatSend, g.roomList, 0x01A0, 0, uintptr(g.px(58)))
	r := rectWin{0, 0, int32(g.px(lobbyWidth)), int32(g.px(lobbyHeight))}
	call(userDLL.NewProc("AdjustWindowRectEx"), uintptr(unsafe.Pointer(&r)), 0x02CA0000, 0, 0)
	w, h := int(r.Right-r.Left), int(r.Bottom-r.Top)
	x = max(int(work.Left), min(x, int(work.Right)-w))
	y = max(int(work.Top), min(y, int(work.Bottom)-h))
	call(setWindowPos, g.hwnd, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0x0014)
	g.switchLobbyPage(s.page)
}
