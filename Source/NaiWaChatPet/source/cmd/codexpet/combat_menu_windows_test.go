//go:build windows

package main

import (
	"runtime"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"codexpet/reconstruction/play"
)

func nativeMenuLabels(menu uintptr) []string {
	count, _ := call(userDLL.NewProc("GetMenuItemCount"), menu)
	labels := make([]string, 0, int(count))
	for i := uintptr(0); i < count; i++ {
		var text [256]uint16
		call(userDLL.NewProc("GetMenuStringW"), menu, i, uintptr(unsafe.Pointer(&text[0])), 256, 0x400)
		if label := syscall.UTF16ToString(text[:]); label != "" {
			labels = append(labels, label)
		}
	}
	return labels
}

func TestNativeCombatMenuGrouping(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	root, _ := call(createMenu)
	interact, _ := call(createMenu)
	defer call(destroyMenu, root)
	defer call(destroyMenu, interact)
	settings := play.DefaultGameSettings()
	a := &desktopHost{play: &playState{settings: playSettings{Game: &settings}}}
	a.playMenu(root, interact)
	game, _ := call(userDLL.NewProc("GetSubMenu"), root, 0)
	if got := strings.Join(nativeMenuLabels(game), "|"); got != "单机挑战|联机大厅|游戏设置|武器" {
		t.Fatal(got)
	}
	if got := strings.Join(nativeMenuLabels(interact), "|"); !strings.Contains(got, "桌宠玩法") {
		t.Fatal(got)
	}
	active, _ := call(createMenu)
	defer call(destroyMenu, active)
	a.play.fight = &fightState{upgraded: newCombatPresentation(play.NewCombat(false))}
	defer a.play.fight.upgraded.cancel()
	a.combatMenu(active)
	if got := strings.Join(nativeMenuLabels(active), "|"); got != "联机大厅|游戏设置|结束对局，返回桌宠" {
		t.Fatal(got)
	}
}
