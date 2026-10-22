package fightnet

import (
	"context"
	"encoding/json"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DiscoverLANRooms uses interface-bound broadcasts and bounded TCP discovery.
// The second transport also works when a hotspot drops broadcast datagrams.
func DiscoverLANRooms(parent context.Context, duration time.Duration) ([]LANRoom, error) {
	return discoverLANRooms(parent, duration, localIPv4Networks(), true)
}

func discoverLANRooms(parent context.Context, duration time.Duration, networks []*net.IPNet, broadcast bool) ([]LANRoom, error) {
	if duration <= 0 || duration > 5*time.Second {
		duration = 2800 * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(parent, duration)
	nonce := randomID(12)
	replies := make(chan lanReply, 128)
	var workers sync.WaitGroup
	defer workers.Wait()
	// Cancel before waiting, including early result-limit/caller-cancellation exits.
	defer cancel()
	if broadcast {
		for _, network := range networks {
			workers.Add(1)
			go func() {
				defer workers.Done()
				discoverBroadcast(ctx, network, nonce, replies)
			}()
		}
	}
	workers.Add(1)
	go func() {
		defer workers.Done()
		discoverDirect(ctx, networks, nonce, replies)
	}()
	found := map[string]LANRoom{}
	for {
		select {
		case <-ctx.Done():
			if parent.Err() != nil {
				return nil, parent.Err()
			}
			return sortedLANRooms(found), nil
		case result := <-replies:
			reply, source := result.packet, result.source
			if !onLocalNetwork(source, networks) || reply.Magic != lanMagic || reply.Version != ProtocolVersion || reply.Nonce != nonce || len(reply.Instance) != 16 || reply.Room == nil || reply.Port < 1 || reply.Port > 65535 {
				continue
			}
			room := *reply.Room
			room.Code = strings.ToUpper(room.Code)
			if !validRoomCode(room.Code) || room.Players < 0 || room.Players >= 2 {
				continue
			}
			room.Address = net.JoinHostPort(source.String(), strconv.Itoa(reply.Port))
			key := reply.Instance + "/" + room.Code
			if _, ok := found[key]; !ok || !source.IsLoopback() {
				found[key] = room
			}
			if len(found) >= 64 {
				return sortedLANRooms(found), nil
			}
		}
	}
}

func sortedLANRooms(found map[string]LANRoom) []LANRoom {
	rooms := make([]LANRoom, 0, len(found))
	for _, room := range found {
		rooms = append(rooms, room)
	}
	sort.Slice(rooms, func(i, j int) bool {
		if rooms[i].Code == rooms[j].Code {
			return rooms[i].Address < rooms[j].Address
		}
		return rooms[i].Code < rooms[j].Code
	})
	return rooms
}

func discoverBroadcast(ctx context.Context, network *net.IPNet, nonce string, replies chan<- lanReply) {
	ip := network.IP.To4()
	if ip == nil || len(network.Mask) != 4 {
		return
	}
	// Binding the sender fixes Windows choosing the VPN/default adapter for
	// wildcard broadcast sockets. Replies return to that same physical interface.
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: ip})
	if err != nil {
		return // TCP discovery remains available when UDP is unavailable.
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	broadcast := make(net.IP, 4)
	for i := range broadcast {
		broadcast[i] = ip[i] | ^network.Mask[i]
	}
	targets := []net.IP{broadcast, net.IPv4bcast}
	if ip.IsLoopback() {
		targets = []net.IP{ip}
	}
	query, _ := json.Marshal(lanPacket{Magic: lanMagic, Version: ProtocolVersion, Nonce: nonce})
	nextSend := time.Time{}
	buf := make([]byte, 1025)
	for ctx.Err() == nil {
		if !time.Now().Before(nextSend) {
			for _, target := range targets {
				conn.SetWriteDeadline(time.Now().Add(50 * time.Millisecond))
				conn.WriteToUDP(query, &net.UDPAddr{IP: target, Port: LANDiscoveryPort})
			}
			nextSend = time.Now().Add(350 * time.Millisecond)
		}
		conn.SetReadDeadline(time.Now().Add(80 * time.Millisecond))
		n, source, err := conn.ReadFromUDP(buf)
		if err != nil {
			if e, ok := err.(net.Error); ok && e.Timeout() {
				continue
			}
			return
		}
		var packet lanPacket
		if n > 1024 || json.Unmarshal(buf[:n], &packet) != nil {
			continue
		}
		select {
		case replies <- lanReply{packet: packet, source: source.IP}:
		case <-ctx.Done():
			return
		}
	}
}
