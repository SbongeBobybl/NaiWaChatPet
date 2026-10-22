//go:build windows

package main

import (
	"codexpet/reconstruction/fightnet"
	"context"
	"strings"
	"time"
)

type lanSearchResult struct {
	rooms    []fightnet.LANRoom
	err      error
	version  uint64
	joinCode string
	join     bool
}

func validLobbyCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	for _, c := range code {
		if !strings.ContainsRune("0123456789ABCDEF", c) {
			return false
		}
	}
	return true
}

func (a *desktopHost) createLANRoom() {
	u := a.prepareOnline()
	if u == nil {
		return
	}
	host, err := fightnet.StartLANHost(u.context, &u.core.Rules)
	if err != nil {
		a.endFight()
		a.combatStartError(err)
		return
	}
	u.lanHost, u.lanSession = host, true
	u.address = host.LocalAddress()
	u.status = "CONNECTING"
	u.networkDetail = "正在创建局域网房间…"
	u.startDial(false)
	a.openGamePanel()
}

func (g *gamePanel) startLANSearch(code string, join bool) {
	if join {
		g.joinAfterSearch = true
		g.searchJoinCode = code
	}
	if g.searching {
		return
	}
	g.searchVersion++
	version := g.searchVersion
	ctx, cancel := context.WithCancel(context.Background())
	g.searchCancel = cancel
	g.searching = true
	g.nextSearch = time.Now().Add(8 * time.Second)
	go func() {
		defer cancel()
		rooms, err := fightnet.DiscoverLANRooms(ctx, 2800*time.Millisecond)
		result := lanSearchResult{rooms: rooms, err: err, version: version, joinCode: code, join: join}
		select {
		case g.searchResults <- result:
		default:
		}
	}()
}

func (g *gamePanel) selectedLANRoom() (fightnet.LANRoom, bool) {
	index, _, _ := chatSend.Call(g.roomList, 0x0188, 0, 0)
	if int(index) >= 0 && int(index) < len(g.rooms) {
		return g.rooms[int(index)], true
	}
	return fightnet.LANRoom{}, false
}

func (g *gamePanel) joinLANRoom(code string, search bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code != "" && len(code) == 6 && !validLobbyCode(code) {
		g.statusError = "房间码只能包含数字与 A–F，请检查后重试。"
		return
	}
	if code != "" && len(code) != 6 {
		g.statusError = "房间码为 6 位，也可以直接在列表选择房间。"
		return
	}
	room, found := g.selectedLANRoom()
	if code != "" {
		found = false
		for _, candidate := range g.rooms {
			if candidate.Code == code {
				room, found = candidate, true
				break
			}
		}
	}
	if !found {
		if search {
			g.statusError = "正在查找好友的房间…"
			g.startLANSearch(code, true)
		} else {
			g.statusError = "广播和热点兼容搜索均未收到房间回应。\n请房主检查“联机权限”；若仍失败，热点可能禁止设备互访。"
		}
		return
	}
	g.statusError = ""
	if g.searchCancel != nil {
		g.searchCancel()
	}
	g.searchVersion++
	g.searching = false
	g.joinAfterSearch = false
	g.nextSearch = time.Now().Add(4 * time.Second)
	g.owner.beginOnlineAt(room.Code, room.Address)
	if g.owner.play.fight != nil {
		g.owner.play.fight.upgraded.lanSession = true
		g.owner.play.fight.upgraded.networkDetail = "正在加入局域网房间 " + room.Code + "…"
	}
}

func (g *gamePanel) refreshLAN() {
	for {
		select {
		case result := <-g.searchResults:
			if result.version != g.searchVersion {
				continue
			}
			g.searching = false
			if g.manual {
				continue
			}
			if result.err != nil {
				g.joinAfterSearch = false
				g.statusError = "无法搜索局域网房间：" + result.err.Error()
				continue
			}
			g.updateLANList(result.rooms)
			if result.join || g.joinAfterSearch {
				code := result.joinCode
				if g.joinAfterSearch {
					code = g.searchJoinCode
				}
				g.joinAfterSearch = false
				g.joinLANRoom(code, false)
			}
		default:
			if !g.manual && (g.skin == nil || g.skin.page == 0) && g.visible && !g.searching && time.Now().After(g.nextSearch) {
				g.startLANSearch("", false)
			}
			return
		}
	}
}
