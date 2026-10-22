//go:build windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"codexpet/reconstruction/play"
)

// Exercise the reported "pet works, lobby does not open" window states.
func TestLobbyReopensFromMinimizedAndStaleWindows(t *testing.T) {
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
	defer func() {
		if a.play.gamePanel != nil {
			a.play.gamePanel.close()
		}
	}()
	opened := 0
	for cycle := 0; cycle < 4; cycle++ {
		a.openGamePanel()
		g := a.play.gamePanel
		if g == nil || g.hwnd == 0 || lobbyPanel(g.hwnd) != g {
			t.Fatal("lobby not created or routed", cycle)
		}
		opened++
		original := g.hwnd
		call(showWindow, original, 6)
		if iconic, _ := call(userDLL.NewProc("IsIconic"), original); iconic == 0 {
			t.Fatal("fixture did not minimize")
		}
		a.openGamePanel()
		if a.play.gamePanel != g || g.hwnd != original {
			t.Fatal("minimized lobby was duplicated")
		}
		if iconic, _ := call(userDLL.NewProc("IsIconic"), original); iconic != 0 {
			t.Fatal("minimized lobby not restored")
		}
		if cycle%2 == 0 {
			// An OS-owned destruction also has to clear the cached panel and buffers.
			if ok, _ := call(destroyWindow, original); ok == 0 {
				t.Fatal("fixture did not destroy")
			}
			if a.play.gamePanel != nil || g.hwnd != 0 || g.frame != nil || g.skin != nil || lobbyPanel(original) != nil {
				t.Fatal("externally destroyed lobby retained stale state")
			}
			a.play.gamePanel = g
		} else {
			g.close()
			if a.play.gamePanel != nil || lobbyPanel(original) != nil {
				t.Fatal("close retained lobby routing")
			}
		}
		if alive, _ := call(userDLL.NewProc("IsWindow"), original); alive != 0 {
			t.Fatal("destroyed lobby window leaked")
		}
	}
	a.openGamePanel()
	if g := a.play.gamePanel; g == nil || g.hwnd == 0 || g.skin == nil {
		t.Fatal("registered class could not be reused")
	}
	opened++
	result := map[string]any{"passed": true, "windowCreations": opened, "minimizedRestored": 4, "staleCacheRebuilt": 2, "externalDestructionCleaned": 2, "registeredClassReused": true, "scope": "actual Win32 window lifecycle; user field crash dump not available"}
	if output := os.Getenv("NAIWA_COMBAT_ARTIFACTS"); output != "" {
		os.MkdirAll(filepath.Dir(output), 0700)
		b, _ := json.MarshalIndent(result, "", "  ")
		if err := os.WriteFile(filepath.Join(filepath.Dir(output), "lobby-open.json"), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("lobby recovery: %v", result)
}
