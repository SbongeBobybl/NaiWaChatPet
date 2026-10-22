//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"codexpet/reconstruction/play"
)

func TestLobbyStableFramesAndResourceLifecycle(t *testing.T) {
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
	var handles [][2]uintptr
	var paintMS []float64
	var cacheCounts []int
	peek := userDLL.NewProc("PeekMessageW")
	pump := func(g *gamePanel, duration time.Duration) {
		deadline := time.Now().Add(duration)
		for time.Now().Before(deadline) {
			var message messageWin
			for n := 0; n < 128; n++ {
				if ok, _ := call(peek, uintptr(unsafe.Pointer(&message)), 0, 0, 0, 1); ok == 0 {
					break
				}
				if message.Message != 0x12 {
					call(translateMessage, uintptr(unsafe.Pointer(&message)))
					call(dispatchMessage, uintptr(unsafe.Pointer(&message)))
				}
			}
			g.refresh()
			time.Sleep(time.Millisecond)
		}
	}
	for cycle := 0; cycle < 6; cycle++ {

		a.openGamePanel()

		g := a.play.gamePanel
		if g == nil || g.hwnd == 0 {
			t.Fatal("lobby unavailable")
		}
		if cycle == 0 {
			defer g.close()
		}
		if g.searchCancel != nil {
			g.searchCancel()
		}
		g.searchVersion++
		g.searching = false
		g.nextSearch = time.Now().Add(time.Hour)
		g.listInitialized = true
		g.refresh()
		pump(g, 30*time.Millisecond)
		if g.lobbyTimerRunning {
			t.Fatal("decorative timer still repaints the whole window")
		}
		grabLobby(t, g)
		before := append([]byte(nil), g.frame.pixels...)
		positions := map[uintptr]rectWin{}
		for _, control := range g.skin.controls {
			var r rectWin
			call(lobbyGetWindowRect, control.window, uintptr(unsafe.Pointer(&r)))
			positions[control.window] = r
		}
		pump(g, 260*time.Millisecond)
		grabLobby(t, g)
		// Hover and caret changes are expected; the page and control bounds stay fixed.
		if !bytes.Equal(before, g.frame.pixels) {
			t.Fatal("idle page background or text moves")
		}
		for window, expected := range positions {
			var r rectWin
			call(lobbyGetWindowRect, window, uintptr(unsafe.Pointer(&r)))
			if r != expected {
				t.Fatal("control bounds moved without input")
			}
		}
		start := time.Now()
		for n := 0; n < 10; n++ {
			grabLobby(t, g)
		}
		paintMS = append(paintMS, float64(time.Since(start))/float64(10*time.Millisecond))

		// Re-entrant DPI notifications during setup must be deferred.
		g.skin.building = true
		r := rectWin{40, 40, 1000, 680}
		gamePanelProc(g.hwnd, wmDPIChanged, 144, uintptr(unsafe.Pointer(&r)))
		g.skin.building = false
		pump(g, 10*time.Millisecond)
		g.reflowLobby(96, 40, 40)
		pump(g, 10*time.Millisecond)
		for _, page := range []int{1, 2, 0} {
			g.switchLobbyPage(page)
			pump(g, 10*time.Millisecond)
		}
		frame := g.frame

		g.reflowLobby(96, 40, 40)
		if frame != g.frame {
			t.Fatal("unchanged DPI rebuilt the frame buffer")
		}
		g.resume()
		pump(g, 10*time.Millisecond)
		gdi, user := guiResources()
		handles = append(handles, [2]uintptr{gdi, user})
		count := len(g.skin.cache) + len(g.skin.fonts) + len(g.skin.brushes)
		for _, layers := range g.skin.chrome {
			count += len(layers)
		}
		cacheCounts = append(cacheCounts, count)
	}
	// GetGuiResources also includes other test threads' OS/theme caches.
	// Assert only the resources this panel owns, and retain global telemetry.
	for _, count := range cacheCounts[1:] {
		if count != cacheCounts[0] {
			t.Fatalf("owned rendering caches grow after reopen: %v", cacheCounts)
		}
	}
	panel := a.play.gamePanel
	owned := panel.hwnd
	panel.close()
	if alive, _ := call(userDLL.NewProc("IsWindow"), owned); alive != 0 {
		t.Fatal("lobby survived final close")
	}
	if a.play.gamePanel != nil || panel.frame != nil || panel.skin != nil {
		t.Fatal("owned rendering resources survived final close")
	}
	result := map[string]any{"passed": true, "cycles": len(handles), "idlePageFramesByteIdentical": true, "controlBoundsStable": true, "gdiUserAfterHide": handles, "ownedCacheEntriesAfterHide": cacheCounts, "captureAverageMSByCycle": paintMS, "deferredDPI": true, "continuousDecorationTimer": false, "scope": "actual native window message pump; reopen/hide, page changes, DPI and final destruction"}
	t.Logf("native lifecycle: %v; capture costs %v ms", handles, paintMS)
	if output := os.Getenv("NAIWA_COMBAT_ARTIFACTS"); output != "" {
		os.MkdirAll(filepath.Dir(output), 0700)
		b, _ := json.MarshalIndent(result, "", "  ")
		if err := os.WriteFile(filepath.Join(filepath.Dir(output), "lobby-stability.json"), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
