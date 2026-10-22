//go:build windows

package main

import (
	"codexpet/reconstruction/fightnet"
	"fmt"
	"runtime"
	"strings"
	"unsafe"
)

func (g *gamePanel) updateLANList(rooms []fightnet.LANRoom) {
	var keys []string
	for _, room := range rooms {
		keys = append(keys, room.Code+"/"+room.Address)
	}
	key := strings.Join(keys, "|")
	if key == g.roomListKey && g.listInitialized {
		return
	}
	selected, _ := g.selectedLANRoom()
	g.rooms = rooms
	g.roomListKey = key
	g.listInitialized = true
	_, _, _ = chatSend.Call(g.roomList, 0x0184, 0, 0)
	add := func(text string) {
		p := utf(text)
		_, _, _ = chatSend.Call(g.roomList, 0x0180, 0, uintptr(unsafe.Pointer(p)))
		runtime.KeepAlive(p)
	}

	choice := 0
	for i, room := range rooms {
		add(fmt.Sprintf("房间 %s   ·   %d/2 人", room.Code, room.Players))
		if room.Code == selected.Code && room.Address == selected.Address {
			choice = i
		}
	}
	_, _, _ = chatSend.Call(g.roomList, 0x0186, uintptr(choice), 0)
	if controlText(g.code) == selected.Code {
		code := ""
		if len(rooms) > 0 {
			code = rooms[choice].Code
		}
		setText(g.code, code)
	}
}

func (g *gamePanel) toggleConnectionMode() {
	if g.manual {
		g.switchLobbyPage(0)
	} else {
		g.switchLobbyPage(2)
	}
}
