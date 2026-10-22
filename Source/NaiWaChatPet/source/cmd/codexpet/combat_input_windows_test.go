//go:build windows

package main

import (
	"codexpet/reconstruction/play"
	"testing"
)

func TestCombatKeysRespectRepeatChatAndInactiveMode(t *testing.T) {
	settings := play.DefaultGameSettings()
	p := &playState{control: true, settings: playSettings{Game: &settings}, fight: &fightState{upgraded: &combatPresentation{}}}
	a := &desktopHost{play: p}
	if err := settings.Bind("attack", 'Z'); err != nil {
		t.Fatal(err)
	}
	if a.playKey('J', false) || p.attackEdge {
		t.Fatal("old primary binding still triggered after remapping")
	}
	if !a.playKey('Z', false) || !p.attackEdge {
		t.Fatal("new primary binding ignored")
	}
	p.attackEdge = false
	if !a.playKey('U', false) || !p.skillEdges[0] {
		t.Fatal("skill edge not captured")
	}
	p.skillEdges[0] = false
	a.playKey('U', true)
	if p.skillEdges[0] {
		t.Fatal("repeat generated another skill edge")
	}
	a.playKey(vkEscape, true)
	if !p.control {
		t.Fatal("held Esc exited game")
	}
	a.chat = &nativeChat{inputVisible: true}
	a.playKey('I', false)
	if p.skillEdges[1] {
		t.Fatal("chat typing triggered skill")
	}
	a.chat = nil
	p.fight = nil
	p.control = false
	if a.playKey('U', false) || p.control {
		t.Fatal("inactive skill became a global combat hotkey")
	}
}
