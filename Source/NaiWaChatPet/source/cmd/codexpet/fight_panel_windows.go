//go:build windows

package main

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"codexpet/reconstruction/fightnet"
	"codexpet/reconstruction/play"
)

type connectionProbeResult struct{ text string }

type gamePanel struct {
	skin                                                    *lobbyStyle
	joinAfterSearch                                         bool
	searchJoinCode                                          string
	roomList, connectionLabel, manualButton                 uintptr
	manual, visible, searching, listInitialized             bool
	rooms                                                   []fightnet.LANRoom
	roomListKey                                             string
	searchResults                                           chan lanSearchResult
	searchCancel                                            context.CancelFunc
	searchVersion                                           uint64
	nextSearch                                              time.Time
	probeResults                                            chan connectionProbeResult
	probeCancel                                             context.CancelFunc
	probing                                                 bool
	probeButton                                             uintptr
	labels                                                  map[uintptr]string
	keyHandles                                              map[uintptr]uintptr
	keyNames                                                map[string]string
	statusError                                             string
	owner                                                   *desktopHost
	hwnd, server, code, status, difficulty, volume, effects uintptr
	keys                                                    map[uintptr]string
	name                                                    *uint16
	// Animation bookkeeping for the redesigned lobby.
	anim                *lobbyAnim
	shiftX, shiftY      int
	fontDC              uintptr
	fontCurrent         uintptr
	lobbyTimerRunning   bool
	lobbyTimerPeriod    int
	navPillY            float64
	navPillReady        bool
	hoverRoomItem       int
	frame               *lobbySurface
	painting, reflowing bool
	pendingDPI          uint32
	pendingBounds       rectWin
}

var lobbyWindows sync.Map // HWND -> *gamePanel; never route through a global active panel.
var lobbyWindowProcedure uintptr

func init() { lobbyWindowProcedure = syscall.NewCallback(gamePanelProc) }

func lobbyPanel(hwnd uintptr) *gamePanel {
	value, _ := lobbyWindows.Load(hwnd)
	panel, _ := value.(*gamePanel)
	return panel
}
func lobbyChildPanel(hwnd uintptr) *gamePanel {
	parent, _ := call(userDLL.NewProc("GetParent"), hwnd)
	return lobbyPanel(parent)
}

func (a *desktopHost) lobbyOpenError(stage string, err error) {
	problem := fmt.Errorf("lobby %s: %w", stage, err)
	_, _ = logDiagnostic(problem)
	nativeMessage(a.hwnd, "大厅未打开", "打开失败，请重试。\n"+problem.Error(), 0x10)
}

func (a *desktopHost) openGamePanel() {
	if a.play == nil || a.play.class == nil {
		return
	}
	if f := a.play.fight; f != nil && f.upgraded != nil && f.upgraded.address == "" && !f.upgraded.connecting {
		f.upgraded.paused = true
	}
	if a.play.gamePanel != nil {
		g := a.play.gamePanel
		if alive, _ := call(userDLL.NewProc("IsWindow"), g.hwnd); alive != 0 && g.skin != nil {
			g.presentLobby()
			return
		}
		g.close()
	}
	g := &gamePanel{owner: a, keys: map[uintptr]string{}, keyHandles: map[uintptr]uintptr{}, keyNames: map[string]string{}, name: utf("NaiWaCombatLobby"), probeResults: make(chan connectionProbeResult, 1), searchResults: make(chan lanSearchResult, 4), visible: true, hoverRoomItem: -1, lobbyTimerPeriod: lobbyFrameMillis}
	module := a.play.class.module
	k := classWin{Size: uint32(unsafe.Sizeof(classWin{})), Procedure: lobbyWindowProcedure, Instance: module, ClassName: g.name, Background: 0}
	k.Cursor, _ = call(loadCursor, 0, 32512)
	if atom, err := call(registerClass, uintptr(unsafe.Pointer(&k))); atom == 0 {
		// A window class belongs to the process and is reusable after closing a
		// window. Never silently fail just because it was registered earlier.
		existing := classWin{Size: uint32(unsafe.Sizeof(classWin{}))}
		found, _ := call(userDLL.NewProc("GetClassInfoExW"), module, uintptr(unsafe.Pointer(g.name)), uintptr(unsafe.Pointer(&existing)))
		if found == 0 || existing.Procedure != lobbyWindowProcedure {
			a.lobbyOpenError("register window class", failure("RegisterClassExW", err))
			return
		}
	}
	a.play.gamePanel = g
	title := utf("奶蛙格斗")
	hwnd, err := call(createWindow, 0, uintptr(unsafe.Pointer(g.name)), uintptr(unsafe.Pointer(title)), 0x02CA0000, 100, 100, 920, 604, a.hwnd, 0, module, uintptr(unsafe.Pointer(g)))
	runtime.KeepAlive(title)
	runtime.KeepAlive(k)
	runtime.KeepAlive(g)
	if hwnd == 0 {
		g.close()
		a.lobbyOpenError("create window", failure("CreateWindowExW", err))
		return
	}
	g.hwnd = hwnd
	a.play.gamePanel = g
	if err := g.buildLobby(); err != nil {
		g.close()
		a.lobbyOpenError("create controls", err)
		return
	}
	g.presentLobby()
}

func (g *gamePanel) presentLobby() {
	g.visible = true
	g.refresh()
	g.startLobbyClock()
	mode := uintptr(5)
	if iconic, _ := call(userDLL.NewProc("IsIconic"), g.hwnd); iconic != 0 {
		mode = 9
	}
	call(showWindow, g.hwnd, mode)
	var r rectWin
	if ok, _ := call(lobbyGetWindowRect, g.hwnd, uintptr(unsafe.Pointer(&r))); ok != 0 {
		if work, ok := workAreaAt(int(r.Left), int(r.Top), true); ok {
			x := max(int(work.Left), min(int(r.Left), int(work.Right-r.Right+r.Left)))
			y := max(int(work.Top), min(int(r.Top), int(work.Bottom-r.Bottom+r.Top)))
			if x != int(r.Left) || y != int(r.Top) {
				call(setWindowPos, g.hwnd, 0, uintptr(x), uintptr(y), 0, 0, 0x0015)
			}
		}
	}
	call(userDLL.NewProc("BringWindowToTop"), g.hwnd)
	call(foregroundWindow, g.hwnd)
	call(chatSetFocus, g.hwnd)
	// Finish the first paint before returning from an in-frame result transition.
	call(userDLL.NewProc("UpdateWindow"), g.hwnd)
}
func keyLabel(key uint16) string {
	if key == 0x10 {
		return "SHIFT"
	}
	if key >= 32 && key < 127 {
		return string(rune(key))
	}
	return fmt.Sprintf("VK%02X", key)
}
func (g *gamePanel) refresh() {
	if g == nil || g.hwnd == 0 || !g.visible {
		return
	}
	if g.skin != nil && g.skin.building {
		return
	}
	g.refreshLAN()
	select {
	case result := <-g.probeResults:
		g.probing = false
		g.statusError = result.text
	default:
	}
	probeLabel := "刷新"
	if g.searching {
		probeLabel = "搜索中…"
	}
	if g.manual {
		probeLabel = "检测"
	}
	if g.manual && g.probing {
		probeLabel = "检测中…"
	}
	g.updateLabel(g.probeButton, probeLabel)
	p := g.owner.play
	s := p.settings.Game
	level := map[play.AIDifficulty]string{play.EasyAI: "简单", play.NormalAI: "普通", play.HardAI: "困难"}[s.Difficulty]
	g.updateLabel(g.difficulty, level+"  ›")
	status := "创建房间，或加入好友"
	if !g.manual && g.listInitialized && len(g.rooms) == 0 {
		status = "暂无房间，可先创建"
		if g.searching {
			status = "正在查找附近房间…"
		}
	}
	if p.fight != nil {
		u := p.fight.upgraded
		status = u.status
		if u.address == "" {
			status = "单机挑战"
			if u.paused {
				status += " · 已暂停"
			}
		} else if u.room != "" {
			switch u.networkState {
			case "waiting":
				status = "等待双方准备"
			case "countdown":
				status = "即将开战"
			case "fighting":
				status = "对战中"
			case "disconnected":
				status = "重连中…"
			case "over":
				result, _ := combatResult(u.snapshot, u.localPlayer)
				if result == "" {
					result = "对局结束"
				}
				status = result + " · 再来一局"
			default:
				status = "已加入房间"
			}
		}
		if u.networkDetail != "" {
			status = u.networkDetail
		}
	}
	if g.statusError != "" {
		status = g.statusError
	}
	g.refreshLobbyState(status)
}

func (g *gamePanel) updateLabel(hwnd uintptr, text string) {
	if hwnd == 0 {
		return
	}
	if g.labels == nil {
		g.labels = map[uintptr]string{}
	}
	if old, ok := g.labels[hwnd]; ok && old == text {
		return
	}
	g.labels[hwnd] = text
	setText(hwnd, text)
}
func (g *gamePanel) saveServer() bool {
	s, err := fightnet.NormalizeAddress(controlText(g.server))
	if err != nil {
		g.statusError = "服务器地址格式应为 host:port 或 tls://host:port"
		g.refresh()
		return false
	}
	g.owner.play.settings.Game.Server = s
	g.owner.play.save()
	return true
}
func (g *gamePanel) command(id uintptr) {
	g.statusError = ""
	a, p := g.owner, g.owner.play
	switch id {
	case 30:
		g.switchLobbyPage(0)
	case 31:
		g.switchLobbyPage(1)
	case 23:
		if p.fight != nil && p.fight.upgraded.room != "" {
			if err := copyChatURL(p.fight.upgraded.room); err != nil {
				g.statusError = "复制失败，请重试。"
			} else {
				g.statusError = "已复制房间码"
			}
		}
	case 24:
		a.endFight()
		a.setControl(false)
		g.statusError = "已离开房间"
		g.switchLobbyPage(0)
	case 25:
		p.settings.Game.Keys = play.DefaultGameSettings().Keys
		p.save()
		g.statusError = "已恢复默认按键"
	case 3:
		if room, ok := g.selectedLANRoom(); ok {
			setText(g.code, room.Code)
		}
	case 10:
		if a.combatActive() && p.fight.upgraded.address != "" && (p.fight.upgraded.connecting || p.fight.upgraded.room != "") {
			g.statusError = "已在房间中，请准备"
			break
		}
		if !g.manual {
			setText(g.code, "")
			a.createLANRoom()
		} else if g.saveServer() {
			a.beginOnline("")
		}
	case 11:
		if a.combatActive() && p.fight.upgraded.address != "" && (p.fight.upgraded.connecting || p.fight.upgraded.room != "") {
			g.statusError = "正在连接或已在房间中，点击准备即可。"
			break
		}
		code := strings.ToUpper(strings.TrimSpace(controlText(g.code)))
		if !g.manual {
			if a.combatActive() && p.fight.upgraded.lanHost != nil {
				g.statusError = "你已创建房间，等待好友加入并准备即可。"
				break
			}
			g.joinLANRoom(code, true)
			break
		}
		if len(code) != 6 {
			g.statusError = "请输入 6 位房间代码。"
			g.refresh()
			return
		}
		if g.saveServer() {
			a.beginOnline(code)
		}
	case 12:
		if p.fight == nil || p.fight.upgraded.client == nil || p.fight.upgraded.room == "" {
			g.statusError = "请先创建或加入房间。"
		} else {
			a.combatCommand(menuReady)
		}
	case 13:
		a.combatCommand(menuRematch)
	case 14:
		a.endFight()
		a.startFight()
		g.resume()
	case 15:
		g.resume()
	case 16:
		a.combatCommand(menuExitFight)
		g.resume()
	case 17:
		if !g.manual {
			g.nextSearch = time.Time{}
			g.startLANSearch("", false)
		} else if !g.probing && g.saveServer() {
			g.startProbe(p.settings.Game.Server)
		}
	case 18:
		g.toggleConnectionMode()
	case 19:
		g.openLANPermissions()
	case 20:
		s := p.settings.Game
		switch s.Difficulty {
		case play.EasyAI:
			s.Difficulty = play.NormalAI
		case play.NormalAI:
			s.Difficulty = play.HardAI
		default:
			s.Difficulty = play.EasyAI
		}
		if p.fight != nil && p.fight.upgraded.address == "" {
			p.fight.upgraded.core.SetAI(1, play.NewCombatAI(19, s.Difficulty))
		}
		p.save()
	case 21:
		s := p.settings.Game
		s.Volume += .25
		if s.Volume > 1 {
			s.Volume = 0
		}
		p.save()
	case 22:
		s := p.settings.Game
		s.Effects -= .25
		if s.Effects < .25 {
			s.Effects = 1
		}
		p.save()
	default:
		if action, ok := g.keys[id]; ok {
			g.skin.bindAction = action
			_, _ = call(chatSetFocus, g.hwnd)
		}
	}
	g.refresh()
}
func (g *gamePanel) resume() {
	if g.skin != nil {
		g.skin.bindAction = ""
	}
	g.stopLobbyClock()
	g.visible = false
	if g.searchCancel != nil {
		g.searchCancel()
	}
	g.searchVersion++
	g.searching = false
	g.joinAfterSearch = false
	g.nextSearch = time.Time{}
	if f := g.owner.play.fight; f != nil {
		f.upgraded.paused = false
	}
	_, _ = call(showWindow, g.hwnd, 0)
	_, _ = call(foregroundWindow, g.owner.hwnd)
	_, _ = call(chatSetFocus, g.owner.hwnd)
}
func (g *gamePanel) close() {
	if g == nil {
		return
	}
	if g.probeCancel != nil {
		g.probeCancel()
	}
	if g.searchCancel != nil {
		g.searchCancel()
	}
	g.visible = false
	if g.hwnd != 0 {
		hwnd := g.hwnd
		if alive, _ := call(userDLL.NewProc("IsWindow"), hwnd); alive != 0 {
			if ok, err := call(destroyWindow, hwnd); ok == 0 {
				_, _ = logDiagnostic(failure("DestroyWindow(lobby)", err))
				return // A live window must retain its fonts and drawing surfaces.
			}
		}
		lobbyWindows.Delete(hwnd)
	}
	g.disposeLobby()
}

func (g *gamePanel) disposeLobby() {
	g.hwnd, g.visible = 0, false
	if g.probeCancel != nil {
		g.probeCancel()
	}
	if g.searchCancel != nil {
		g.searchCancel()
	}
	g.releaseLobby()
	if g.owner != nil && g.owner.play != nil && g.owner.play.gamePanel == g {
		g.owner.play.gamePanel = nil
	}
}

func (g *gamePanel) startProbe(address string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	g.probeCancel = cancel
	g.probing = true
	g.statusError = "正在检测 " + address + " 的连接与协议版本…"
	go func() {
		defer cancel()
		result, err := fightnet.Probe(ctx, address)
		text := fmt.Sprintf("%s 连接成功，协议 v%d，往返 %.0fms。\n现在可以创建房间；另一位玩家使用同一地址加入房间。", address, result.Version, float64(result.RoundTrip)/float64(time.Millisecond))
		if err != nil {
			text = combatConnectionError(err, address)
		}
		select {
		case g.probeResults <- connectionProbeResult{text}:
		default:
		}
	}()
}
func gamePanelProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	if msg == 0x0081 && lparam != 0 { // WM_NCCREATE, CREATESTRUCTW.lpCreateParams
		parameter := *(*uintptr)(unsafe.Pointer(lparam))
		if parameter != 0 {
			g := (*gamePanel)(unsafe.Pointer(parameter))
			g.hwnd = hwnd
			lobbyWindows.Store(hwnd, g)
		}
	}
	g := lobbyPanel(hwnd)
	if g != nil {
		switch msg {
		case 0x0082: // WM_NCDESTROY, including destruction outside close().
			lobbyWindows.Delete(hwnd)
			g.disposeLobby()
		case wmEraseBackground:
			return 1
		case wmTimer:
			if g.skin != nil && wparam == lobbyTimerID && g.visible {
				g.lobbyTick()
				return 0
			}
		case 0x0318:
			if g.skin != nil {
				g.paintLobby(wparam)
				return 0
			}
		case wmDPIChanged:
			if g.skin != nil && lparam != 0 {
				r := *(*rectWin)(unsafe.Pointer(lparam))
				g.pendingDPI, g.pendingBounds = uint32(wparam&0xffff), r
				// Creation, layout and painting can dispatch nested messages.
				// Apply the latest DPI only after the current operation returns.
				call(postMessage, hwnd, lobbyReflowMessage, 0, 0)
				return 0
			}
		case lobbyReflowMessage:
			if g.skin != nil && !g.skin.building && !g.reflowing && !g.painting && g.pendingDPI != 0 {
				dpi, r := g.pendingDPI, g.pendingBounds
				g.pendingDPI = 0
				g.reflowLobby(dpi, int(r.Left), int(r.Top))
			}
			return 0
		case wmPaint:
			if g.skin != nil {
				var ps paintStruct
				dc, _ := call(chatBeginPaint, hwnd, uintptr(unsafe.Pointer(&ps)))
				g.paintLobby(dc)
				call(chatEndPaint, hwnd, uintptr(unsafe.Pointer(&ps)))
				return 0
			}
		case 0x002B:
			if g.skin != nil && lparam != 0 {
				item := *(*drawItem)(unsafe.Pointer(lparam))
				if item.Type == 2 {
					g.drawLobbyRoom(item)
				} else {
					g.drawLobbyButton(item)
				}
				return 1
			}
		case 0x002C:
			if g.skin != nil && lparam != 0 {
				item := (*struct {
					Type, ID, Item, Width, Height uint32
					Data                          uintptr
				})(unsafe.Pointer(lparam))
				if item.Type == 2 {
					item.Height = uint32(g.px(58))
					return 1
				}
			}
		case 0x0133, 0x0134, 0x0135, 0x0138:
			if g.skin != nil {
				bg := uint32(lobbyBG)
				ink := uint32(lobbyInk)
				switch lparam {
				case g.server, g.code:
					bg = lobbyWell
				case g.status:
					bg = lobbyBG
					ink = lobbyMuted
				case g.roomList:
					bg = lobbyBG
				}
				call(chatSetTextColor, wparam, lobbyRef(ink))
				call(chatSetBkColor, wparam, lobbyRef(bg))
				brush := g.skin.brushes[bg]
				if brush == 0 {
					brush, _ = call(chatCreateBrush, lobbyRef(bg))
					g.skin.brushes[bg] = brush
				}
				return brush
			}
		case 0x0114:
			if g.skin != nil && (lparam == g.volume || lparam == g.effects) {
				g.changeLobbySlider(lparam, wparam&0xffff)
				return 0
			}
		case 0x004e:
			if g.skin != nil && lparam != 0 {
				n := (*lobbyCustomDraw)(unsafe.Pointer(lparam))
				if (n.Header.Window == g.volume || n.Header.Window == g.effects) && n.Header.Code == 0xfffffff4 && n.Stage == 1 {
					g.drawLobbySlider(n.Header.Window, n.DC)
					return 4
				}
			}
		case 0x0400: // DM_GETDEFID: Enter follows the current room action.
			id := uintptr(10)
			if g.skin != nil {
				if g.skin.room != "" {
					id = 12
				} else if len(g.rooms) > 0 || len(controlText(g.code)) == 6 {
					id = 11
				}
			}
			return 0x534b0000 | id
		case 0x0111:
			id, notice := wparam&0xffff, wparam>>16
			if lparam == 0 && id == 2 {
				g.resume()
				return 0
			}
			if id == 1 || id == 2 {
				if notice == 0x300 {
					g.refresh()
				}
				return 0
			}
			if id == 3 {
				g.command(3)
				if notice == 2 {
					g.command(11)
				}
				return 0
			}
			if notice == 0 {
				g.command(id)
			}
			return 0
		case wmClose:
			g.resume()
			return 0
		}
	}
	r, _, _ := defWindowProc.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}
