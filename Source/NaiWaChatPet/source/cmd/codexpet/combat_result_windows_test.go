//go:build windows

package main

import (
	"codexpet/reconstruction/fightnet"
	"codexpet/reconstruction/play"
	"context"
	"encoding/json"
	"net"
	"runtime"
	"testing"
	"time"
	"unsafe"
)

func TestNativeOnlineResultsWithoutKOEvents(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	call(dpiContext, ^uintptr(3))
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	assets, err := loadAssets(bundledAssets, "runtime.json")
	if err != nil {
		t.Fatal(err)
	}
	module, _ := call(moduleHandle, 0)
	settings := play.DefaultGameSettings()
	core := play.NewCombat(false)
	u := newCombatPresentation(core)
	defer u.cancel()
	a := &desktopHost{assets: assets, epoch: time.Now(), dpi: 96, w: 210, h: 280, play: &playState{class: &spriteClass{module: module}, settings: playSettings{Game: &settings}, fight: &fightState{arena: core.Arena, upgraded: u}}}
	defer func() {
		if a.play.gamePanel != nil {
			a.play.gamePanel.close()
		}
	}()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	updates := make(chan fightnet.Update)
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, e := listener.Accept()
		if e != nil {
			return
		}
		defer conn.Close()
		encoder := json.NewEncoder(conn)
		for update := range updates {
			if encoder.Encode(update) != nil {
				return
			}
		}
	}()
	defer func() { close(updates); listener.Close(); <-done }()
	client, err := fightnet.Dial(context.Background(), listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	u.client, u.address = client, listener.Addr().String()
	for match, winner := range []int{0, 1, -1} {
		s := core.Snapshot()
		s.Tick = 8000
		s.Phase = play.Over
		s.Winner = winner
		s.Events = nil
		updates <- fightnet.Update{Version: fightnet.ProtocolVersion, Type: "state", Match: uint64(match + 1), Room: "ABC123", State: "over", Snapshot: &s, Connected: [2]bool{true, true}}
		until := time.Now().Add(2 * time.Second)
		for time.Now().Before(until) {
			a.updateCombatNetwork(time.Since(a.epoch), play.Input{})
			if u.resultShown && u.networkMatch == uint64(match+1) {
				break
			}
			time.Sleep(time.Millisecond)
		}
		label, banner := combatResult(s, 0)
		g := a.play.gamePanel
		if g == nil || !g.visible || g.skin.result != label || u.status != banner {
			t.Fatal("persistent final result missing", label, u.status)
		}
		// Run the messages a real application loop dispatches after ShowWindow.
		for n := 0; n < 128; n++ {
			var message messageWin
			if ok, _ := call(userDLL.NewProc("PeekMessageW"), uintptr(unsafe.Pointer(&message)), 0, 0, 0, 1); ok == 0 {
				break
			}
			if message.Message != 0x12 {
				call(translateMessage, uintptr(unsafe.Pointer(&message)))
				call(dispatchMessage, uintptr(unsafe.Pointer(&message)))
			}
		}
		captureLobby(t, g, "lobby-result-"+banner)
		g.resume()
		a.updateCombatNetwork(time.Since(a.epoch), play.Input{})
		if g.visible {
			t.Fatal("repeated final snapshot reopened lobby")
		}
		s.Tick = 0
		s.Phase = play.Ready
		s.Winner = -1
		updates <- fightnet.Update{Version: fightnet.ProtocolVersion, Type: "state", Match: uint64(match + 2), Room: "ABC123", State: "countdown", Snapshot: &s}
		until = time.Now().Add(time.Second)
		for time.Now().Before(until) {
			a.updateCombatNetwork(time.Since(a.epoch), play.Input{})
			if u.networkState == "countdown" {
				break
			}
			time.Sleep(time.Millisecond)
		}
		g.visible = true
		g.refresh()
		if g.skin.result != "" || u.resultShown {
			t.Fatal("old result survived new match")
		}
	}
}
