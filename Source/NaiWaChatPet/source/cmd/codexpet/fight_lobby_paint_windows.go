//go:build windows

package main

import (
	"fmt"
	"image"
	"math"
	"runtime"
	"unsafe"
)

var (
	lobbyGetWindowRect  = userDLL.NewProc("GetWindowRect")
	lobbyScreenToClient = userDLL.NewProc("ScreenToClient")
)

const (
	keyRail   = "rail"
	keyLeft   = "left"
	keyRight  = "right"
	keyFooter = "footer"
)

type lobbyLayer struct {
	surface *lobbySurface
	origin  image.Point
}
type lobbyNotifyHeader struct {
	Window, ID uintptr
	Code       uint32
}
type lobbyCustomDraw struct {
	Header lobbyNotifyHeader
	Stage  uint32
	DC     uintptr
	Rect   rectWin
	Item   uintptr
	State  uint32
	Data   uintptr
}

const (
	dtLeft      uintptr = 0
	dtCenter    uintptr = 1
	dtRight     uintptr = 2
	dtVCenter   uintptr = 4
	dtWordBreak uintptr = 0x10
	dtSingle    uintptr = 0x20
	dtNoPrefix  uintptr = 0x800
	dtEllipsis  uintptr = 0x8000
)

func (g *gamePanel) cardOffset(string) (int, int) { return 0, 0 }
func (g *gamePanel) forgetFont()                  { g.fontDC, g.fontCurrent = 0, 0 }
func (g *gamePanel) selectFont(dc uintptr, size int) {
	font := g.skin.fonts[size]
	if font == 0 {
		font = g.skin.fonts[lobbyCap]
	}
	if font != 0 && (g.fontDC != dc || g.fontCurrent != font) {
		call(chatSelectObject, dc, font)
		g.fontDC, g.fontCurrent = dc, font
	}
}
func (g *gamePanel) lobbyText(dc uintptr, x, y, w, h int, text string, size int, ink uint32, flags uintptr) {
	if text == "" {
		return
	}
	g.selectFont(dc, size)
	call(chatSetTextColor, dc, lobbyRef(ink))
	call(chatSetBkMode, dc, 1)
	r := rectWin{int32(g.px(x)), int32(g.px(y)), int32(g.px(x + w)), int32(g.px(y + h))}
	p := utf(text)
	call(chatDrawText, dc, uintptr(unsafe.Pointer(p)), ^uintptr(0), uintptr(unsafe.Pointer(&r)), flags|dtNoPrefix)
	runtime.KeepAlive(p)
}
func (g *gamePanel) field(im *image.NRGBA, x, y, w, h int) {
	r := image.Rect(g.px(x), g.px(y), g.px(x+w), g.px(y+h))
	lobbyFill(im, r, 0, lobbyColor(lobbyWellEdge))
	lobbyFill(im, r.Inset(1), 0, lobbyColor(lobbyWell))
}
func (g *gamePanel) rule(im *image.NRGBA, x, y, w int) {
	lobbyFill(im, image.Rect(g.px(x), g.px(y), g.px(x+w), g.px(y)+max(1, g.px(1))), 0, lobbyColor(lobbyLineColor))
}

func (g *gamePanel) paintLobby(target uintptr) {
	if g.skin == nil || target == 0 || g.painting || g.reflowing || g.skin.building {
		return
	}
	g.painting = true
	defer func() { g.painting = false }()
	w, h := g.px(lobbyWidth), g.px(lobbyHeight)
	if g.frame == nil || g.frame.width != w || g.frame.height != h {
		g.frame.close()
		g.frame = newLobbySurface(w, h)
	}
	if g.frame == nil {
		return
	}
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	bg := lobbyColor(lobbyBG)
	for i := 0; i < len(im.Pix); i += 4 {
		im.Pix[i], im.Pix[i+1], im.Pix[i+2], im.Pix[i+3] = bg.R, bg.G, bg.B, 255
	}
	g.rule(im, 32, 104, 836)
	g.rule(im, 32, 518, 836)
	if g.skin.page != 1 {
		lobbyFill(im, image.Rect(g.px(564), g.px(120), g.px(564)+1, g.px(496)), 0, lobbyColor(lobbyLineColor))
		g.rule(im, 584, 214, 284)
		g.rule(im, 584, 334, 284)
		if g.skin.page == 0 {
			g.field(im, 32, 454, 200, 40)
		} else {
			g.field(im, 32, 168, 516, 40)
			g.field(im, 32, 264, 208, 40)
		}
	} else {
		g.rule(im, 32, 252, 836)
	}
	g.frame.copyOpaque(im)
	dc := g.frame.dc
	saved, _ := call(chatGDI.NewProc("SaveDC"), dc)
	defer func() { call(chatGDI.NewProc("RestoreDC"), dc, saved); g.forgetFont() }()
	g.forgetFont()
	g.paintRailPond(dc, 0)
	g.lobbyText(dc, 104, 22, 164, 30, "奶蛙格斗", lobbyTitle, lobbyInk, dtSingle|dtVCenter)
	g.lobbyText(dc, 104, 56, 164, 18, "局域网对战", lobbyCap, lobbyMuted, dtSingle)
	switch g.skin.page {
	case 1:
		g.paintSettingsText(dc)
	case 2:
		g.paintAdvancedText(dc)
	default:
		g.paintListText(dc)
	}
	if g.skin.page != 1 {
		g.paintRoomText(dc)
	}
	call(lobbyFlush)
	g.frame.paint(target, 0, 0)
	call(lobbyFlush)
}

func (g *gamePanel) paintRailPond(dc uintptr, now float64) {
	pond := g.pondFor("rail", g.px(80), g.px(88), lobbyBG, nil)
	pond.reset()
	mascotInto(pond.work, g.px(72), g.px(40), g.px(84), 255)
	pond.x, pond.y = g.px(20), g.px(4)
	pond.present(dc)
}
func (g *gamePanel) paintListText(dc uintptr) {
	g.lobbyText(dc, 32, 122, 180, 24, "附近房间", lobbySec, lobbyInk, dtSingle|dtVCenter)
	count := fmt.Sprintf("%d 个房间", len(g.rooms))
	if g.searching {
		count = "搜索中…"
	}
	g.lobbyText(dc, 244, 126, 180, 18, count, lobbyCap, lobbyMuted, dtRight|dtSingle)
	if len(g.rooms) == 0 {
		g.lobbyText(dc, 32, 258, 516, 28, "暂无房间", lobbyNum, lobbyInk, dtCenter|dtSingle|dtVCenter)
		g.lobbyText(dc, 32, 298, 516, 20, "让好友建房，或自己创建", lobbyCap, lobbyMuted, dtCenter|dtSingle)
	}
	g.lobbyText(dc, 32, 430, 180, 18, "房间码", lobbyCap, lobbyMuted, dtSingle)
}
func (g *gamePanel) paintAdvancedText(dc uintptr) {
	g.lobbyText(dc, 32, 126, 516, 24, "服务器地址", lobbySec, lobbyInk, dtSingle)
	g.lobbyText(dc, 32, 236, 208, 18, "房间码", lobbyCap, lobbyMuted, dtSingle)
	g.lobbyText(dc, 32, 318, 516, 20, "host:port  /  tls://host:port", lobbyCap, lobbyMuted, dtSingle)
	g.lobbyText(dc, 32, 424, 516, 24, "同一 Wi-Fi 对战，请用「联机」", lobbyCap, lobbyMuted, dtSingle)
}
func (g *gamePanel) paintSettingsText(dc uintptr) {
	game := g.owner.play.settings.Game
	for _, v := range []struct {
		x     int
		label string
	}{{32, "电脑难度"}, {312, "音量"}, {592, "特效"}} {
		g.lobbyText(dc, v.x, 140, 170, 24, v.label, lobbySec, lobbyInk, dtSingle)
	}
	g.lobbyText(dc, 450, 140, 98, 24, fmt.Sprintf("%.0f%%", game.Volume*100), lobbyBody2, lobbyMuted, dtRight|dtSingle)
	g.lobbyText(dc, 770, 140, 98, 24, fmt.Sprintf("%.0f%%", game.Effects*100), lobbyBody2, lobbyMuted, dtRight|dtSingle)
	g.lobbyText(dc, 32, 276, 80, 24, "按键", lobbySec, lobbyInk, dtSingle)
	g.lobbyText(dc, 114, 280, 380, 20, "点击改键 · Esc 取消", lobbyCap, lobbyMuted, dtSingle)
}
func (g *gamePanel) paintRoomText(dc uintptr) {
	s := g.skin
	room := s.room
	if room == "" {
		room = "—"
	}
	heading, ink := "当前房间", uint32(lobbyInk)
	if s.result != "" {
		heading, room = "本局结果 · "+s.room, s.result
		ink = lobbyGoldDeep
		if s.winner == s.localPlayer {
			ink = lobbySageDeep
		} else if s.winner >= 0 {
			ink = lobbyPeach
		}
	}
	g.lobbyText(dc, 584, 124, 284, 22, heading, lobbyCap, lobbyMuted, dtSingle)
	g.lobbyText(dc, 584, 156, 284, 40, room, lobbyHero, ink, dtSingle|dtVCenter)
	for row := 0; row < 2; row++ {
		player := s.localPlayer
		if row == 1 {
			player = 1 - player
		}
		player = max(0, min(1, player))
		name, state := "你", "未加入"
		if row == 1 {
			name, state = "好友", "等待加入"
		}
		switch {
		case s.ready[player]:
			state = "已准备"
		case s.connected[player]:
			state = "未准备"
		}
		if s.phase == "countdown" || s.phase == "fighting" {
			state = "就绪"
		}
		if s.result != "" {
			state = "平局"
			if s.winner == player {
				state = "胜利"
			} else if s.winner >= 0 {
				state = "失败"
			}
		}
		ink := lobbyMuted
		if s.ready[player] {
			ink = lobbySageDeep
		}
		y := 232 + row*52
		g.lobbyText(dc, 584, y, 44, 28, fmt.Sprintf("P%d", player+1), lobbyCap, lobbyGoldDeep, dtSingle|dtVCenter)
		g.lobbyText(dc, 634, y, 88, 28, name, lobbyBody2, lobbyInk, dtSingle|dtVCenter)
		g.lobbyText(dc, 726, y, 142, 28, state, lobbyCap, ink, dtRight|dtSingle|dtVCenter)
	}
	if s.phase == "countdown" {
		g.lobbyText(dc, 790, 156, 78, 40, fmt.Sprint((s.countdown+59)/60), lobbyHero, lobbyGoldDeep, dtRight|dtSingle|dtVCenter)
	}
}

func (g *gamePanel) blit(itemDC uintptr, pixels []byte, w, h, x, y int) {
	if len(pixels) == 0 {
		return
	}
	header := skinDIBHeader{Size: 40, Width: int32(w), Height: -int32(h), Planes: 1, Bits: 32}
	call(buttonStretch, itemDC, uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0, 0, uintptr(w), uintptr(h), uintptr(unsafe.Pointer(&pixels[0])), uintptr(unsafe.Pointer(&header)), 0, lobbySRCCopy)
	call(lobbyFlush)
	runtime.KeepAlive(pixels)
}

// Owner-drawn controls need the same atomic presentation as the page: paint
// background and text into their own DIB before exposing either to Windows.
func (g *gamePanel) bufferLobbyControl(item drawItem, kind string) (drawItem, func()) {
	w, h := int(item.Rect.Right-item.Rect.Left), int(item.Rect.Bottom-item.Rect.Top)
	if w <= 0 || h <= 0 {
		return item, func() {}
	}
	key := fmt.Sprintf("%s/%d/%d/%d", kind, item.Window, w, h)
	surface := g.skin.cache[key]
	if surface == nil {
		surface = newLobbySurface(w, h)
		if surface == nil {
			return item, func() {}
		}
		g.skin.cache[key] = surface
	}
	target, at := item.DC, item.Rect
	call(lobbyFlush)
	saved, _ := call(chatGDI.NewProc("SaveDC"), surface.dc)
	item.DC, item.Rect = surface.dc, rectWin{0, 0, int32(w), int32(h)}
	g.forgetFont()
	return item, func() {
		call(lobbyFlush)
		call(chatGDI.NewProc("RestoreDC"), surface.dc, saved)
		g.forgetFont()
		surface.paint(target, int(at.Left), int(at.Top))
		call(lobbyFlush)
	}
}

func (g *gamePanel) layerPixels(im *image.NRGBA) []byte {
	pix := make([]byte, len(im.Pix))
	for i := 0; i < len(pix); i += 4 {
		pix[i], pix[i+1], pix[i+2], pix[i+3] = im.Pix[i+2], im.Pix[i+1], im.Pix[i], 255
	}
	return pix
}

func (g *gamePanel) drawLobbyButton(item drawItem) {
	s := g.skin
	b := s.buttons[item.Window]
	if b == nil {
		return
	}
	item, finish := g.bufferLobbyControl(item, "button")
	defer finish()
	w, h := int(item.Rect.Right), int(item.Rect.Bottom)
	disabled := item.State&4 != 0
	if on, _ := call(userDLL.NewProc("IsWindowEnabled"), item.Window); on == 0 {
		disabled = true
	}
	focus := item.State&0x10 != 0
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	fill, edge, ink := lobbyBG, lobbyLineColor, lobbyInk
	selected := b.role == "nav" && (item.ID == 30 && s.page == 0 || item.ID == 31 && s.page == 1 || item.ID == 18 && s.page == 2)
	switch {
	case b.role == "nav":
		edge = lobbyBG
		if selected {
			ink = lobbyGoldDeep
		}
	case disabled:
		fill, edge, ink = lobbyCardSoft, lobbyCardSoft, lobbyMuted
	case b.role == "primary":
		fill, edge, ink = lobbyGold, lobbyGold, lobbyGoldInk
	case b.role == "secondary" || b.role == "key":
		fill = lobbyWell
	}
	if b.hover && !disabled {
		if b.role == "primary" {
			fill = lobbyGoldLight
		} else {
			fill = lobbyHover
		}
	}
	if b.pressed && !disabled {
		fill = lobbyRowSoft
		if b.role == "primary" {
			fill = lobbyGoldSoft
		}
	}
	lobbyFill(im, im.Bounds(), 0, lobbyColor(edge))
	lobbyFill(im, im.Bounds().Inset(1), 0, lobbyColor(fill))
	if selected {
		lobbyFill(im, image.Rect(0, h-g.px(3), w, h), 0, lobbyColor(lobbyGold))
	}
	g.blit(item.DC, g.layerPixels(im), w, h, 0, 0)
	g.lobbyText(item.DC, 0, 0, int(math.Round(float64(w)/s.scale)), int(math.Round(float64(h)/s.scale)), controlText(item.Window), lobbyBody2, ink, dtCenter|dtSingle|dtVCenter)
	if focus && !disabled {
		r := rectWin{3, 3, int32(w - 3), int32(h - 3)}
		call(chatDrawFocus, item.DC, uintptr(unsafe.Pointer(&r)))
	}
}
func (g *gamePanel) drawLobbyRoom(item drawItem) {
	if int(item.Item) >= len(g.rooms) {
		return
	}
	room := g.rooms[item.Item]
	item, finish := g.bufferLobbyControl(item, "room")
	defer finish()
	w, h := int(item.Rect.Right), int(item.Rect.Bottom)
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	fill := lobbyBG
	selected := item.State&1 != 0
	if selected {
		fill = lobbyRowSoft
	} else if g.hoverRoomItem == int(item.Item) {
		fill = lobbyCardSoft
	}
	lobbyFill(im, im.Bounds(), 0, lobbyColor(fill))
	lobbyFill(im, image.Rect(0, h-1, w, h), 0, lobbyColor(lobbyLineSoft))
	if selected {
		lobbyFill(im, image.Rect(0, 0, g.px(3), h), 0, lobbyColor(lobbyGold))
	}
	g.blit(item.DC, g.layerPixels(im), w, h, 0, 0)
	g.lobbyText(item.DC, 16, 16, 280, 26, room.Code, lobbySec, lobbyInk, dtSingle|dtVCenter)
	g.lobbyText(item.DC, 328, 16, 64, 26, fmt.Sprintf("%d / 2", room.Players), lobbyCap, lobbyMuted, dtSingle|dtVCenter)
	g.lobbyText(item.DC, 410, 16, 90, 26, "可加入", lobbyCap, lobbySageDeep, dtRight|dtSingle|dtVCenter)
}
func (g *gamePanel) drawLobbySlider(window, dc uintptr) {
	var r, thumb, channel rectWin
	call(chatGetClientRect, window, uintptr(unsafe.Pointer(&r)))
	call(chatSend, window, 0x0419, 0, uintptr(unsafe.Pointer(&thumb)))
	call(chatSend, window, 0x041a, 0, uintptr(unsafe.Pointer(&channel)))
	w, h := int(r.Right), int(r.Bottom)
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	lobbyFill(im, im.Bounds(), 0, lobbyColor(lobbyBG))
	x, y := float64(thumb.Left+thumb.Right)/2, float64(channel.Top+channel.Bottom)/2
	lobbyStroke(im, float64(channel.Left), y, float64(channel.Right), y, 3*g.skin.scale, lobbyColor(lobbyLineColor))
	lobbyStroke(im, float64(channel.Left), y, x, y, 3*g.skin.scale, lobbyColor(lobbyGold))
	lobbyDisc(im, x, y, 6*g.skin.scale, 6*g.skin.scale, lobbyColor(lobbyGold))
	g.blit(dc, g.layerPixels(im), w, h, 0, 0)
	if focus, _ := call(chatGetFocus); focus == window {
		fr := rectWin{1, 1, int32(w - 1), int32(h - 1)}
		call(chatDrawFocus, dc, uintptr(unsafe.Pointer(&fr)))
	}
}

func lobbyButtonProc(hwnd, msg, wp, lp, id, data uintptr) uintptr {
	if g := lobbyChildPanel(hwnd); g != nil && g.skin != nil {
		if b := g.skin.buttons[hwnd]; b != nil {
			hover, pressed := b.hover, b.pressed
			switch msg {
			case wmMouseMove:
				g.noteLobbyInput()
				if !b.hover {
					b.hover = true
					t := struct {
						Size, Flags uint32
						Window      uintptr
						Time        uint32
					}{Flags: 2, Window: hwnd}
					t.Size = uint32(unsafe.Sizeof(t))
					call(buttonTrack, uintptr(unsafe.Pointer(&t)))
				}
			case wmLeftDown:
				b.pressed = true
				g.noteLobbyInput()
			case wmLeftUp, wmCaptureChanged:
				b.pressed = false
			case 0x02A3: // WM_MOUSELEAVE
				b.hover = false
			case 0x0100, 0x0101:
				g.noteLobbyInput()
			}
			b.motion, b.press = 0, 0
			if b.hover {
				b.motion = 1
			}
			if b.pressed {
				b.press = 1
			}
			if hover != b.hover || pressed != b.pressed {
				call(skinInvalidate, hwnd, 0, 0)
			}
		}
	}
	r, _ := call(chatDefSubclass, hwnd, msg, wp, lp)
	return r
}

// lobbyListProc tracks the hovered row without moving its bounds.
func lobbyListProc(hwnd, msg, wp, lp, id, data uintptr) uintptr {
	g := lobbyChildPanel(hwnd)
	if g != nil && g.skin != nil && hwnd == g.roomList {
		switch msg {
		case wmMouseMove:
			g.noteLobbyInput()
			index, _ := call(chatSend, hwnd, 0x01A9, wp, lp) // LB_ITEMFROMPOINT
			row := int(index & 0xFFFF)
			if int16(index>>16) != 0 || row >= len(g.rooms) {
				row = -1
			}
			if row != g.hoverRoomItem {
				g.hoverRoomItem = row
				call(skinInvalidate, hwnd, 0, 0)
			}
		case 0x02A3:
			if g.hoverRoomItem != -1 {
				g.hoverRoomItem = -1
				call(skinInvalidate, hwnd, 0, 0)
			}
		}
	}
	r, _ := call(chatDefSubclass, hwnd, msg, wp, lp)
	return r
}
