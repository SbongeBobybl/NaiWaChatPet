//go:build windows

package main

import (
	"runtime"
	"testing"
	"time"
	"unsafe"

	"codexpet/reconstruction/play"
)

// TestLobbyFrameBudget keeps the animated lobby honest about its cost. A frame
// has to leave plenty of headroom: the lobby shares one GUI thread with the
// desktop pet, so a frame that takes longer than its 33 ms slot is a bug.
func TestLobbyFrameBudget(t *testing.T) {
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
	host := &desktopHost{assets: assets, epoch: time.Now(), dpi: 96, w: 210, h: 280, play: &playState{class: &spriteClass{module: module}, settings: playSettings{Game: &settings}}}
	host.openGamePanel()
	g := host.play.gamePanel
	if g == nil || g.hwnd == 0 {
		t.Fatal("lobby did not open")
	}
	defer g.close()
	g.ensureAnim().started = time.Now().Add(-2 * time.Second)

	var ps paintStruct
	dc, _ := call(chatBeginPaint, g.hwnd, uintptr(unsafe.Pointer(&ps)))
	if dc == 0 {
		dc, _ = call(skinGetDC, g.hwnd)
		defer call(skinReleaseDC, g.hwnd, dc)
	} else {
		defer call(chatEndPaint, g.hwnd, uintptr(unsafe.Pointer(&ps)))
	}

	// Warm every cache: chrome layers, ponds, the mascot sprite.
	for i := 0; i < 12; i++ {
		g.paintLobby(dc)
	}
	const frames = 30
	start := time.Now()
	for i := 0; i < frames; i++ {
		g.paintLobby(dc)
	}
	perFrame := time.Since(start) / frames
	t.Logf("lobby frame cost: %v for %d frames (budget %d ms)", perFrame, frames, lobbyFrameMillis)
	if perFrame > 14*time.Millisecond {
		t.Fatalf("lobby frame costs %v, too close to the %d ms slot", perFrame, lobbyFrameMillis)
	}
}
