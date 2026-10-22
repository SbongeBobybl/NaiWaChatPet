//go:build windows

package main

import (
	"context"
	"image"
	"image/png"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"codexpet/reconstruction/fightnet"
	"codexpet/reconstruction/play"
)

// settleLobby advances the lobby animation clock so a capture shows the design
// in its resting state rather than mid-entrance.
func settleLobby(g *gamePanel) { g.refresh() }

// grabLobby forces a synchronous repaint and copies the client area.
func grabLobby(t *testing.T, g *gamePanel) *image.NRGBA {
	t.Helper()
	call(userDLL.NewProc("RedrawWindow"), g.hwnd, 0, 0, 0x0001|0x0004|0x0080|0x0100)
	call(userDLL.NewProc("UpdateWindow"), g.hwnd)
	call(chatGDI.NewProc("GdiFlush"))
	var r rectWin
	call(chatGetClientRect, g.hwnd, uintptr(unsafe.Pointer(&r)))
	w, h := int(r.Right), int(r.Bottom)
	dc, _ := call(skinGetDC, g.hwnd)
	defer call(skinReleaseDC, g.hwnd, dc)
	copyDC, _ := call(skinCreateDC, dc)
	defer call(skinDeleteDC, copyDC)
	header := skinDIBHeader{Size: 40, Width: int32(w), Height: -int32(h), Planes: 1, Bits: 32}
	var bits uintptr
	bitmap, _ := call(skinCreateDIB, dc, uintptr(unsafe.Pointer(&header)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bitmap == 0 || bits == 0 {
		t.Fatal("native lobby capture failed")
	}
	defer call(chatDeleteObject, bitmap)
	old, _ := call(chatSelectObject, copyDC, bitmap)
	defer call(chatSelectObject, copyDC, old)
	if result, _ := call(userDLL.NewProc("PrintWindow"), g.hwnd, copyDC, 1|2); result == 0 {
		t.Fatal("PrintWindow client capture failed")
	}
	call(chatGDI.NewProc("GdiFlush"))
	pix := unsafe.Slice((*byte)(unsafe.Pointer(bits)), w*h*4)
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(pix); i += 4 {
		im.Pix[i], im.Pix[i+1], im.Pix[i+2], im.Pix[i+3] = pix[i+2], pix[i+1], pix[i], 255
	}
	return im
}

// mascotPixels checks that the existing mascot is composited into the header.
func mascotPixels(g *gamePanel) int {
	g.paintRailPond(0, g.lobbyNow())
	if g.anim == nil {
		return 0
	}
	pond := g.anim.ponds["rail"]
	if pond == nil || pond.work == nil {
		return 0
	}
	pix := pond.work.Pix
	gold := 0
	for i := 0; i+3 < len(pix); i += 4 {
		r, gg, b := int(pix[i]), int(pix[i+1]), int(pix[i+2])
		if r > 185 && gg > 120 && b < 150 && r > b+60 {
			gold++
		}
	}
	return gold
}

func captureLobby(t *testing.T, g *gamePanel, name string) {
	t.Helper()
	settleLobby(g)
	im := grabLobby(t, g)
	// Save first, so a failing check still leaves a picture to look at.
	if output := os.Getenv("NAIWA_COMBAT_ARTIFACTS"); output != "" {
		os.MkdirAll(output, 0700)
		file, err := os.Create(filepath.Join(output, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(file, im)
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	// Both pages and status use the same flat canvas, without a separate card.
	for _, point := range []image.Point{{880, 546}, {8, 300}} {
		pixel := im.NRGBAAt(g.px(point.X), g.px(point.Y))
		if pixel.R != byte(lobbyBG>>16) || pixel.G != byte((lobbyBG>>8)&255) || pixel.B != byte(lobbyBG&255) {
			t.Fatal("flat lobby canvas was not painted", point, pixel)
		}
	}
	if gold := mascotPixels(g); gold < 100 {
		t.Fatal("mascot artwork missing from the header", gold)
	}
}

func TestNativeLobbyWorkflowAndAppearance(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	call(dpiContext, ^uintptr(3))
	assets, err := loadAssets(bundledAssets, "runtime.json")
	if err != nil {
		t.Fatal(err)
	}
	module, _ := call(moduleHandle, 0)
	settings := play.DefaultGameSettings()
	a := &desktopHost{assets: assets, epoch: time.Now(), dpi: 96, w: 210, h: 280, play: &playState{class: &spriteClass{module: module}, settings: playSettings{Game: &settings}}}
	a.openGamePanel()
	g := a.play.gamePanel
	if g == nil || g.hwnd == 0 {
		t.Fatal("lobby did not open")
	}
	defer g.close()
	stopSearch := func() {
		if g.searchCancel != nil {
			g.searchCancel()
		}
		g.searchVersion++
		g.searching = false
		g.nextSearch = time.Now().Add(time.Hour)
	}
	stopSearch()
	g.refresh()
	if enabled, _ := call(userDLL.NewProc("IsWindowEnabled"), g.skin.byID[11]); enabled != 0 {
		t.Fatal("empty room list leaves join enabled")
	}
	for _, control := range g.skin.controls {
		if control.window == 0 {
			t.Fatal("native control missing", control.id)
		}
	}
	captureLobby(t, g, "lobby-empty")
	g.updateLANList([]fightnet.LANRoom{{Code: "A7C204", Players: 1, Address: "127.0.0.1:28760"}, {Code: "E2941B", Players: 1, Address: "127.0.0.1:28762"}})
	g.refresh()
	if enabled, _ := call(userDLL.NewProc("IsWindowEnabled"), g.skin.byID[11]); enabled == 0 {
		t.Fatal("selected room cannot be joined")
	}
	captureLobby(t, g, "lobby-room-list")
	g.switchLobbyPage(1)
	g.command(105)
	m := messageWin{Window: g.hwnd, Message: 0x0100, WParam: 'H'}
	if !g.lobbyMessage(&m) || settings.Keys["attack"] != 'H' {
		t.Fatal("in-place key binding failed")
	}
	g.command(25)
	if settings.Keys["attack"] != 'J' {
		t.Fatal("key reset failed")
	}
	call(chatSend, g.volume, 0x0405, 1, 42)
	g.changeLobbySlider(g.volume, 8)
	if settings.Volume != .42 {
		t.Fatal("volume slider failed")
	}
	g.statusError = ""
	g.refresh()
	captureLobby(t, g, "lobby-settings")
	g.switchLobbyPage(2)
	captureLobby(t, g, "lobby-advanced")
	g.switchLobbyPage(0)
	stopSearch()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := fightnet.NewServer(fightnet.ServerConfig{})
	done := make(chan struct{})
	go func() { defer close(done); server.Serve(ctx, listener) }()
	defer func() { cancel(); listener.Close(); server.Close(); <-done; server.Wait() }()
	owner, err := fightnet.Dial(ctx, listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	owner.Send(fightnet.Message{Type: "create"})
	wait := func(client *fightnet.Client, predicate func(fightnet.Update) bool) fightnet.Update {
		until := time.Now().Add(2 * time.Second)
		for time.Now().Before(until) {
			u := client.State()
			if predicate(u) {
				return u
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatal("room state timeout")
		return fightnet.Update{}
	}
	state := wait(owner, func(u fightnet.Update) bool { return u.Room != "" && u.State == "waiting" })
	guest, err := fightnet.Dial(ctx, listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer guest.Close()
	guest.Send(fightnet.Message{Type: "join", Room: state.Room})
	wait(owner, func(u fightnet.Update) bool { return u.Connected[1] })
	u := newCombatPresentation(play.NewCombat(false))
	defer u.cancel()
	u.client, u.room, u.address, u.networkState = owner, state.Room, listener.Addr().String(), "waiting"
	a.play.fight = &fightState{upgraded: u}
	defer func() { a.play.fight = nil }()
	g.statusError = ""
	g.refresh()
	captureLobby(t, g, "lobby-connected")
	g.command(12)
	wait(owner, func(u fightnet.Update) bool { return u.Ready[0] })
	g.refresh()
	if enabled, _ := call(userDLL.NewProc("IsWindowEnabled"), g.skin.byID[12]); enabled != 0 {
		t.Fatal("ready button can be repeated")
	}
	captureLobby(t, g, "lobby-ready")
	guest.Send(fightnet.Message{Type: "ready"})
	state = wait(owner, func(u fightnet.Update) bool { return u.State == "countdown" })
	u.networkState = state.State
	g.refresh()

	captureLobby(t, g, "lobby-countdown")
	g.reflowLobby(144, 40, 40)
	stopSearch()
	captureLobby(t, g, "lobby-scaled")
	hwnd := g.hwnd
	g.close()
	if valid, _ := call(userDLL.NewProc("IsWindow"), hwnd); valid != 0 {
		t.Fatal("lobby window leaked")
	}
	t.Log("native pages rendered; real TCP room joined and readied; binding, slider, DPI reflow and close checked")
}
