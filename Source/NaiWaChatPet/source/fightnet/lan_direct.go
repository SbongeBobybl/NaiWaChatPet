package fightnet

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"strconv"
	"sync"
	"time"
)

func validLANQuery(p lanPacket) bool {
	return p.Magic == lanMagic && p.Version == ProtocolVersion && len(p.Nonce) == 24 && p.Room == nil && p.Port == 0 && p.Instance == ""
}

func (h *LANHost) advertisement(nonce string, room LANRoom) lanPacket {
	return lanPacket{Instance: h.instance, Magic: lanMagic, Version: ProtocolVersion, Nonce: nonce, Port: h.listener.Addr().(*net.TCPAddr).Port, Room: &room}
}

// A known TCP discovery port is a second transport, independent of broadcasts.
// Only a small LAN-only room advertisement is served, never game/profile data.
func (h *LANHost) respondDirect(ctx context.Context) {
	var workers sync.WaitGroup
	defer workers.Wait()
	slots := make(chan struct{}, 16)
	networks := localIPv4Networks()
	updated, window := time.Now(), time.Now()
	accepted := 0
	for {
		conn, err := h.directDiscovery.Accept()
		if err != nil {
			return
		}
		if time.Since(updated) > 20*time.Second {
			networks, updated = localIPv4Networks(), time.Now()
		}
		if time.Since(window) >= time.Second {
			window, accepted = time.Now(), 0
		}
		if ctx.Err() != nil || !onLocalNetwork(conn.RemoteAddr().(*net.TCPAddr).IP, networks) || accepted >= 64 {
			conn.Close()
			continue
		}
		select {
		case slots <- struct{}{}:
			accepted++
		default:
			conn.Close()
			continue
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			defer func() { <-slots }()
			defer conn.Close()
			stop := context.AfterFunc(ctx, func() { conn.Close() })
			defer stop()
			conn.SetDeadline(time.Now().Add(500 * time.Millisecond))
			data, err := bufio.NewReader(io.LimitReader(conn, 1025)).ReadBytes('\n')
			var request lanPacket
			if err != nil || len(data) > 1024 || json.Unmarshal(data, &request) != nil || !validLANQuery(request) {
				return
			}
			for _, room := range h.waitingRooms() {
				if json.NewEncoder(conn).Encode(h.advertisement(request.Nonce, room)) != nil {
					return
				}
			}
		}()
	}
}

type lanProbe struct{ source, target net.IP }

// Bound work even on a large corporate subnet or a machine with many adapters.
// Phone hotspots (/24 or smaller) are covered completely. Larger subnets get
// the local /24 and subnet gateway; they are never swept in their entirety.
func directDiscoveryTargets(networks []*net.IPNet) []lanProbe {
	var groups [][]lanProbe
	seen := map[string]bool{}
	for _, network := range networks {
		ip := network.IP.To4()
		if ip == nil || len(network.Mask) != 4 {
			continue
		}
		if ip.IsLoopback() {
			groups = append(groups, []lanProbe{{source: ip, target: net.IPv4(127, 0, 0, 1)}})
			continue
		}
		value, mask := binary.BigEndian.Uint32(ip), binary.BigEndian.Uint32(network.Mask)
		low, high := value&mask, value&mask|^mask
		var group []lanProbe
		add := func(candidate uint32) {
			if candidate < low || candidate > high || (high-low > 1 && (candidate == low || candidate == high)) {
				return
			}
			target := make(net.IP, 4)
			binary.BigEndian.PutUint32(target, candidate)
			key := ip.String() + "/" + target.String()
			if !seen[key] {
				seen[key] = true
				group = append(group, lanProbe{source: ip, target: target})
			}
		}
		add(low + 1) // The hotspot host/gateway is often also the room owner.
		add(value)
		base := value & 0xffffff00
		for n := uint32(1); n < 255; n++ {
			add(base | n)
		}
		groups = append(groups, group)
		if len(groups) >= 8 {
			break
		}
	}
	// Round-robin keeps VPN/virtual adapters from starving the Wi-Fi adapter.
	var result []lanProbe
	for i := 0; i < 256 && len(result) < 1024; i++ {
		for _, group := range groups {
			if i < len(group) && len(result) < 1024 {
				result = append(result, group[i])
			}
		}
	}
	return result
}

type lanReply struct {
	packet lanPacket
	source net.IP
}

func discoverDirect(ctx context.Context, networks []*net.IPNet, nonce string, replies chan<- lanReply) {
	targets := directDiscoveryTargets(networks)
	jobs := make(chan lanProbe)
	var workers sync.WaitGroup
	for i := 0; i < min(48, len(targets)); i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for target := range jobs {
				probeDirect(ctx, target, nonce, replies)
			}
		}()
	}
	defer workers.Wait()
	defer close(jobs)
	for _, target := range targets {
		select {
		case jobs <- target:
		case <-ctx.Done():
			return
		}
	}
}

func probeDirect(ctx context.Context, target lanProbe, nonce string, replies chan<- lanReply) {
	dialer := net.Dialer{Timeout: 200 * time.Millisecond, LocalAddr: &net.TCPAddr{IP: target.source}}
	conn, err := dialer.DialContext(ctx, "tcp4", net.JoinHostPort(target.target.String(), strconv.Itoa(LANDiscoveryPort)))
	if err != nil {
		return
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	conn.SetDeadline(time.Now().Add(250 * time.Millisecond))
	if json.NewEncoder(conn).Encode(lanPacket{Magic: lanMagic, Version: ProtocolVersion, Nonce: nonce}) != nil {
		return
	}
	decoder := json.NewDecoder(io.LimitReader(conn, 8192))
	for i := 0; i < 8; i++ {
		var packet lanPacket
		if decoder.Decode(&packet) != nil {
			return
		}
		select {
		case replies <- lanReply{packet: packet, source: target.target}:
		case <-ctx.Done():
			return
		}
	}
}
